package controller

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// installerStages are the stages of a Join that redeems a NodeAdmission: the
// machine is installed by the installer an operator's typed command runs.
var installerStages = []string{"Preflight", "Uplink", "Push", "Install", "VerifyNode", "VerifyRLark", "VerifyAddons", "RotateEntryHostKey"}

const (
	installerStage = "/var/lib/rlark/bootstrap"
	// EntryHostKeyAnnotation is, on a NodeAdmission and on the member Node,
	// the SSH host key the machine's entry device got when the machine joined.
	EntryHostKeyAnnotation = "rlark.io/entry-host-key"
)

// addonNameLabel is LabelAddonName of apps/rlark/pkg/addons: the cluster
// agent puts it on every object an Addon renders, its DaemonSets among them.
// An Addon's pods carry what its own manifests give them, so they are found
// through the DaemonSet's selector.
const addonNameLabel = "rlark.io/addon-name"

var (
	ErrInstallerFailed       = errors.New("the installer failed on the machine")
	ErrJoinOutcomeUnknown    = errors.New("join attempt exists but node identity cannot be verified")
	ErrNodeAlreadyConfigured = errors.New("node already has Kubernetes configuration")
)

// InstallerJoinDriver joins a machine of a NodeClass without anybody at the
// machine: it logs in through the machine's entry device with the accounts
// of the class, lets the machine out (the class's uplink setup), sends the
// installer and runs it with a code it gives the machine's NodeAdmission.
// From the redemption on the machine is admitted exactly as one whose
// operator typed the command.
type InstallerJoinDriver struct {
	Reader  client.Reader
	Writer  client.Client
	Access  NodeClassAccess
	Members NodeMemberClientSource
	// BundleFile is the installer as a tar (bootstrap/build.sh).
	BundleFile string
	// AddonsDirs hold what the addons publish, one directory per addon: the built-in addons' as this program's
	// image carries them, then the site's own. A class's uplink.setup names a file of the first that has it.
	AddonsDirs []string
	// StationAPI is the URL the machine reaches Station API at before it has
	// a tunnel, CAFile the certificate it verifies it with.
	StationAPI, CAFile string
	Now                func() time.Time

	mu     sync.Mutex
	digest string
	stamp  time.Time
}

func (d *InstallerJoinDriver) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *InstallerJoinDriver) Execute(ctx context.Context, op *stationv1alpha1.NodeOperation, stage string) (bool, string, error) {
	class, accounts, err := d.Access.class(ctx, op.Spec.NodeClass)
	if err != nil {
		return false, "", err
	}
	defer accounts.clear()
	at := MachineAddress{Entry: op.Spec.SSH.Jump.Address, EntryHostKey: op.Spec.SSH.Jump.HostKeyFingerprint, Via: op.Spec.SSH.Jump.Via, NodeHostKey: op.Spec.SSH.HostKeyFingerprint}
	switch stage {
	case "Preflight":
		node, sudo, _, err := dialNode(ctx, class, accounts, at)
		if err != nil {
			return false, "", err
		}
		defer node.Close()
		if _, err := RunNodeSudoScript(ctx, node, "command -v tar >/dev/null && command -v systemd-run >/dev/null && modprobe wireguard", sudo); err != nil {
			return false, "", fmt.Errorf("the machine needs tar, systemd and a kernel with wireguard: %w", err)
		}
		return true, "", nil
	case "Uplink":
		if class.Spec.Uplink == nil {
			return true, "", nil
		}
		setup := class.Spec.Uplink.Setup
		script, err := d.published(setup.Addon, setup.Path)
		if err != nil {
			return false, "", fmt.Errorf("read the uplink setup %s of addon %s: %w", setup.Path, setup.Addon, err)
		}
		entry, sudo, err := dialEntry(ctx, class, accounts, at)
		if err != nil {
			return false, "", err
		}
		defer entry.Close()
		if _, err := RunNodeSudoScript(ctx, entry, string(script), sudo); err != nil {
			return false, "", fmt.Errorf("run the uplink setup on the entry device: %w", err)
		}
		return true, "", nil
	case "Push":
		digest, err := d.bundleDigest()
		if err != nil {
			return false, "", err
		}
		node, sudo, _, err := dialNode(ctx, class, accounts, at)
		if err != nil {
			return false, "", err
		}
		defer node.Close()
		have, err := RunNodeSudoScript(ctx, node, installObservationScript+"echo \"installer=$(sha256sum $B/installer.tar 2>/dev/null | cut -d' ' -f1)\"\n", sudo)
		if err != nil {
			return false, "", fmt.Errorf("read the installer on the machine: %w", err)
		}
		seen := parseInstallObservation(have)
		if seen.Installer == digest {
			return true, "", nil
		}
		// A machine that is this node already is sent nothing: its join installs nothing.
		if seen.Node == op.Spec.NodeName && seen.Operation != string(op.UID) {
			joined, err := d.nodeExists(ctx, op)
			if joined || err != nil {
				return joined, "", err
			}
		}
		bundle, err := os.Open(d.BundleFile)
		if err != nil {
			return false, "", fmt.Errorf("open the installer: %w", err)
		}
		defer bundle.Close()
		receive := fmt.Sprintf(`set -eu; B=%s; mkdir -p $B; cat > $B/installer.tar.new; echo "%s  $B/installer.tar.new" | sha256sum -c - >/dev/null; tar -xf $B/installer.tar.new -C $B; mv $B/installer.tar.new $B/installer.tar`, installerStage, digest)
		if _, err := RunNodeSudoStream(ctx, node, receive, sudo, bundle); err != nil {
			return false, "", fmt.Errorf("send the installer to the machine: %w", err)
		}
		return false, "", nil
	case "Install":
		return d.install(ctx, op, class, accounts, at)
	case "VerifyAddons":
		done, err := d.addonsReady(ctx, op, class.Spec.Addons)
		return done, op.Status.NodeUID, err
	case "RotateEntryHostKey":
		return d.rotateEntryHostKey(ctx, op, class, accounts, at)
	}
	return false, "", fmt.Errorf("%w: %s", ErrNodeStageUnavailable, stage)
}

// addonsReady says whether every Addon the class names has a ready pod of
// one of its DaemonSets on the node.
func (d *InstallerJoinDriver) addonsReady(ctx context.Context, op *stationv1alpha1.NodeOperation, addons []string) (bool, error) {
	if len(addons) == 0 {
		return true, nil
	}
	member, err := d.Members.Client(ctx, op.Spec.ClusterID)
	if err != nil {
		return false, fmt.Errorf("connect member cluster: %w", err)
	}
	for _, addon := range addons {
		sets, err := member.AppsV1().DaemonSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: addonNameLabel + "=" + addon})
		if err != nil {
			return false, fmt.Errorf("list the DaemonSets of Addon %s: %w", addon, err)
		}
		ready := false
		for _, set := range sets.Items {
			pods, err := member.CoreV1().Pods(set.Namespace).List(ctx, metav1.ListOptions{LabelSelector: metav1.FormatLabelSelector(set.Spec.Selector), FieldSelector: "spec.nodeName=" + op.Spec.NodeName})
			if err != nil {
				return false, fmt.Errorf("list the pods of Addon %s: %w", addon, err)
			}
			for _, pod := range pods.Items {
				// The field selector again: not every API in front of a cluster filters by it.
				if pod.Spec.NodeName != op.Spec.NodeName || pod.DeletionTimestamp != nil {
					continue
				}
				for _, condition := range pod.Status.Conditions {
					ready = ready || condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue
				}
			}
		}
		if !ready {
			return false, nil
		}
	}
	return true, nil
}

// published reads a file an addon publishes, in the first of the directories that holds it.
func (d *InstallerJoinDriver) published(addon, name string) ([]byte, error) {
	for _, directory := range d.AddonsDirs {
		root, err := os.OpenRoot(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		data, err := root.ReadFile(filepath.Join(addon, name))
		_ = root.Close()
		if !errors.Is(err, os.ErrNotExist) {
			return data, err
		}
	}
	return nil, os.ErrNotExist
}

func (d *InstallerJoinDriver) bundleDigest() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	info, err := os.Stat(d.BundleFile)
	if err != nil {
		return "", fmt.Errorf("the installer: %w", err)
	}
	if d.digest != "" && info.ModTime().Equal(d.stamp) {
		return d.digest, nil
	}
	file, err := os.Open(d.BundleFile)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	d.digest, d.stamp = hex.EncodeToString(sum.Sum(nil)), info.ModTime()
	return d.digest, nil
}

// installObservation is what the machine says of its installation.
type installObservation struct {
	// Operation is the UID of the operation whose installer ran last; Running
	// whether it still runs; Result its exit status, empty until it ended.
	Operation, Result string
	Running           bool
	// Node is the name the machine is a node under, empty if it is none.
	Node string
	// Failed is the installer's own last word on why it stopped: its FAILED line, or the last lines of its log
	// when it stopped without one (a command of its own that failed).
	Failed string
	// Installer is the digest of the installer the machine holds, where it was asked for.
	Installer string
}

const installObservationScript = `B=` + installerStage + `
echo "operation=$(cat $B/operation 2>/dev/null)"
echo "unit=$(systemctl is-active rlark-node-install 2>/dev/null)"
echo "result=$(cat $B/install.result 2>/dev/null)"
echo "certificate=$(sed -n '/BEGIN CERTIFICATE/,/END CERTIFICATE/p' /var/lib/kubelet/pki/kubelet-client-current.pem 2>/dev/null | base64 -w0)"
echo "failed=$({ grep -a 'FAILED\|refused\|required' $B/install.log || tail -n 3 $B/install.log; } 2>/dev/null | tail -3 | tr '\n' ' ' | cut -c1-300)"
`

// staleNodeResetScript resets a machine whose node was removed while it could
// not be reached. A control plane is never reset.
const staleNodeResetScript = `set -eu
PATH=/opt/rlark/bin:$PATH
[ ! -e /etc/kubernetes/manifests/kube-apiserver.yaml ]
socket=$(sed -n 's|.*--container-runtime-endpoint=\(unix://[^ "]*\).*|\1|p' /var/lib/kubelet/kubeadm-flags.env 2>/dev/null)
systemctl stop kubelet 2>/dev/null || true
kubeadm reset --force ${socket:+--cri-socket "$socket"}
rm -rf /etc/cni/net.d/*
`

// nodeExists says whether the operation's cluster has its node.
func (d *InstallerJoinDriver) nodeExists(ctx context.Context, op *stationv1alpha1.NodeOperation) (bool, error) {
	member, err := d.Members.Client(ctx, op.Spec.ClusterID)
	if err != nil {
		return false, fmt.Errorf("connect member cluster: %w", err)
	}
	if _, err := member.CoreV1().Nodes().Get(ctx, op.Spec.NodeName, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("read member node: %w", err)
	}
	return true, nil
}

func parseInstallObservation(output []byte) installObservation {
	var seen installObservation
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		key, value, _ := strings.Cut(scanner.Text(), "=")
		switch key {
		case "operation":
			seen.Operation = value
		case "unit":
			seen.Running = value == "active" || value == "activating"
		case "result":
			seen.Result = value
		case "certificate":
			// The kubelet's own certificate says which node the machine is: its kubeconfig names no node.
			encoded, _ := base64.StdEncoding.DecodeString(value)
			if block, _ := pem.Decode(encoded); block != nil {
				if certificate, err := x509.ParseCertificate(block.Bytes); err == nil {
					seen.Node = strings.TrimPrefix(certificate.Subject.CommonName, "system:node:")
				}
			}
		case "failed":
			seen.Failed = value
		case "installer":
			seen.Installer = value
		}
	}
	return seen
}

// install runs the installer once. What the machine says of an installer
// that has run is read; a run whose end is unknown is never started again.
func (d *InstallerJoinDriver) install(ctx context.Context, op *stationv1alpha1.NodeOperation, class *stationv1alpha1.NodeClass, accounts classAccounts, at MachineAddress) (bool, string, error) {
	node, sudo, _, err := dialNode(ctx, class, accounts, at)
	if err != nil {
		return false, "", err
	}
	defer node.Close()
	output, err := RunNodeSudoScript(ctx, node, installObservationScript, sudo)
	if err != nil {
		return false, "", fmt.Errorf("read the machine's installation: %w", err)
	}
	seen := parseInstallObservation(output)
	switch {
	case seen.Running:
		return false, "", nil
	case seen.Node != "" && seen.Node != op.Spec.NodeName:
		return false, "", fmt.Errorf("%w: it is node %s", ErrNodeAlreadyConfigured, seen.Node)
	case seen.Node != "" && (seen.Operation != string(op.UID) || seen.Result == "0"):
		// A node under this name already, or made one by this operation.
		member, err := d.Members.Client(ctx, op.Spec.ClusterID)
		if err != nil {
			return false, "", fmt.Errorf("connect member cluster: %w", err)
		}
		joined, err := member.CoreV1().Nodes().Get(ctx, op.Spec.NodeName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) && seen.Operation != string(op.UID) {
			// The node was removed while the machine could not be reached:
			// what it kept of the node goes, and it is installed anew.
			if _, err := RunNodeSudoScript(ctx, node, staleNodeResetScript, sudo); err != nil {
				return false, "", fmt.Errorf("reset what the machine kept of removed node %s: %w", seen.Node, err)
			}
			return false, "", nil
		}
		if apierrors.IsNotFound(err) {
			return false, "", fmt.Errorf("%w: its cluster has no node %s", ErrNodeAlreadyConfigured, seen.Node)
		}
		if err != nil {
			return false, "", fmt.Errorf("read member node: %w", err)
		}
		return true, string(joined.UID), nil
	case seen.Operation == string(op.UID) && seen.Result != "":
		return false, "", fmt.Errorf("%w: %s", ErrInstallerFailed, seen.Failed)
	case seen.Operation == string(op.UID):
		return false, "", ErrJoinOutcomeUnknown
	}

	ca, err := os.ReadFile(d.CAFile)
	if err != nil {
		return false, "", fmt.Errorf("read Station API's CA: %w", err)
	}
	code, hash, err := newAdmissionCode()
	if err != nil {
		return false, "", err
	}
	admission := &stationv1alpha1.NodeAdmission{}
	if err := d.Reader.Get(ctx, types.NamespacedName{Namespace: op.Namespace, Name: op.Spec.Admission}, admission); err != nil {
		return false, "", fmt.Errorf("read NodeAdmission %s: %w", op.Spec.Admission, err)
	}
	now := metav1.NewTime(d.now())
	admission.Spec.CodeHash, admission.Spec.ExpiresAt = hash, metav1.NewTime(now.Add(time.Hour))
	admission.Spec.PublicKey, admission.Spec.RedeemedAt = "", nil
	admission.Spec.IssuedBy, admission.Spec.IssuedAt = op.Spec.RequestedBy, &now
	if err := d.Writer.Update(ctx, admission); err != nil {
		return false, "", fmt.Errorf("give NodeAdmission %s its code: %w", op.Spec.Admission, err)
	}
	// The code reaches the machine inside this script, on the connection's
	// stdin, and stays there in a file only root reads until the installer
	// has started: it is in no command line and no log.
	// The installer runs on all of the bundle it was sent: a run that ended
	// deleted the images it imported, and a machine joins again with them.
	start := fmt.Sprintf(`set -eu
B=%[1]s
tar -xf $B/installer.tar -C $B
rm -f $B/install.result $B/install.log
printf '%%s' '%[2]s' > $B/operation
cat > $B/station-api-ca.crt <<'RLARK_CA'
%[3]sRLARK_CA
umask 077
cat > /run/rlark-node-install.env <<'RLARK_ENV'
RLARK_SERVER=%[4]s
RLARK_CODE=%[5]s
RLARK_CA_FILE=%[1]s/station-api-ca.crt
RLARK_ENV
systemd-run --unit rlark-node-install --collect --quiet -p EnvironmentFile=/run/rlark-node-install.env \
  /bin/sh -c 'rm -f /run/rlark-node-install.env; %[1]s/station-bootstrap > %[1]s/install.log 2>&1; echo $? > %[1]s/install.result'
`, installerStage, op.UID, strings.TrimRight(string(ca), "\n")+"\n", d.StationAPI, code)
	if _, err := RunNodeSudoScript(ctx, node, start, sudo); err != nil {
		return false, "", fmt.Errorf("start the installer: %w", err)
	}
	return false, "", nil
}

func newAdmissionCode() (code, hash string, err error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	text := make([]byte, 0, 24)
	buffer := make([]byte, 48)
	for len(text) < cap(text) {
		if _, err := rand.Read(buffer); err != nil {
			return "", "", err
		}
		for _, b := range buffer {
			// 252 is the largest multiple of 36 that fits a byte.
			if b < 252 && len(text) < cap(text) {
				text = append(text, alphabet[int(b)%len(alphabet)])
			}
		}
	}
	digest := sha256.Sum256(text)
	return string(text), hex.EncodeToString(digest[:]), nil
}

const entryHostKeyScript = `set -eu
d=/etc/ssh/rlark-next
if [ ! -e $d/ssh_host_ed25519_key ]; then
  rm -rf $d; mkdir -m 700 $d
  for t in ed25519 ecdsa rsa; do ssh-keygen -q -t $t -N '' -f $d/ssh_host_${t}_key; done
fi
ssh-keygen -lf $d/ssh_host_ed25519_key.pub | cut -d' ' -f2
`

const entryHostKeyActivateScript = `set -eu
d=/etc/ssh/rlark-next
rm -f /etc/ssh/ssh_host_*_key /etc/ssh/ssh_host_*_key.pub
mv $d/ssh_host_* /etc/ssh/
rmdir $d
systemctl restart ssh 2>/dev/null || systemctl restart sshd
`

// rotateEntryHostKey gives the entry device host keys of its own. The new
// key is written down (on the NodeAdmission and the member Node) before the
// device shows it, so a device that no longer shows the factory's key shows
// one that is known.
func (d *InstallerJoinDriver) rotateEntryHostKey(ctx context.Context, op *stationv1alpha1.NodeOperation, class *stationv1alpha1.NodeClass, accounts classAccounts, at MachineAddress) (bool, string, error) {
	if !class.Spec.Access.Entry.RotateHostKey {
		return true, op.Status.NodeUID, nil
	}
	admission := &stationv1alpha1.NodeAdmission{}
	if err := d.Reader.Get(ctx, types.NamespacedName{Namespace: op.Namespace, Name: op.Spec.Admission}, admission); err != nil {
		return false, "", fmt.Errorf("read NodeAdmission %s: %w", op.Spec.Admission, err)
	}
	entry, sudo, err := dialEntry(ctx, class, accounts, at)
	var mismatch *SSHHostKeyMismatchError
	if errors.As(err, &mismatch) && mismatch.Received == admission.Annotations[EntryHostKeyAnnotation] {
		return true, op.Status.NodeUID, nil
	}
	if err != nil {
		return false, "", err
	}
	defer entry.Close()
	output, err := RunNodeSudoScript(ctx, entry, entryHostKeyScript, sudo)
	if err != nil {
		return false, "", fmt.Errorf("make the entry device's host keys: %w", err)
	}
	next := strings.TrimSpace(string(output))
	if !strings.HasPrefix(next, "SHA256:") {
		return false, "", errors.New("the entry device did not name its new host key")
	}
	if admission.Annotations[EntryHostKeyAnnotation] != next {
		patch := client.MergeFrom(admission.DeepCopy())
		if admission.Annotations == nil {
			admission.Annotations = map[string]string{}
		}
		admission.Annotations[EntryHostKeyAnnotation] = next
		if err := d.Writer.Patch(ctx, admission, patch); err != nil {
			return false, "", fmt.Errorf("record the entry device's host key: %w", err)
		}
	}
	member, err := d.Members.Client(ctx, op.Spec.ClusterID)
	if err != nil {
		return false, "", fmt.Errorf("connect member cluster: %w", err)
	}
	annotation := fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, EntryHostKeyAnnotation, next)
	if _, err := member.CoreV1().Nodes().Patch(ctx, op.Spec.NodeName, types.MergePatchType, []byte(annotation), metav1.PatchOptions{}); err != nil {
		return false, "", fmt.Errorf("record the entry device's host key on the node: %w", err)
	}
	if _, err := RunNodeSudoScript(ctx, entry, entryHostKeyActivateScript, sudo); err != nil {
		return false, "", fmt.Errorf("put the entry device's host keys in place: %w", err)
	}
	// The next pass sees the device show the key that was written down.
	return false, op.Status.NodeUID, nil
}

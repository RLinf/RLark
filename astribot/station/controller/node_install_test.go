package controller

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

type installMembers struct{ member kubernetes.Interface }

func (m installMembers) Client(context.Context, string) (kubernetes.Interface, error) {
	return m.member, nil
}

// installRobot is a robot as the driver meets it: an entry device and, behind
// it, the computer that becomes the node.
type installRobot struct {
	entry, node *testSSHHost
	// entryScripts and nodeScripts are what each ran as root; commands every command line.
	entryScripts, nodeScripts, commands []string
	// observation is what the node says of its installation.
	observation string
	// received is the installer file the node holds.
	received []byte
}

func newInstallRobot(t *testing.T) *installRobot {
	robot := &installRobot{entry: newTestSSHHost(t, "entry-secret"), node: newTestSSHHost(t, "node-secret")}
	robot.entry.behind["192.168.0.10:22"] = robot.node.address()
	robot.entry.run = func(command string, stdin []byte) (string, uint32) {
		robot.commands = append(robot.commands, command)
		switch {
		case command == "sudo -n true":
			return "", 0 // the entry account's sudo asks for nothing
		case strings.Contains(string(stdin), "ssh-keygen -lf"):
			robot.entryScripts = append(robot.entryScripts, string(stdin))
			return "SHA256:" + strings.Repeat("n", 43) + "\n", 0
		}
		robot.entryScripts = append(robot.entryScripts, string(stdin))
		return "", 0
	}
	robot.node.run = func(command string, stdin []byte) (string, uint32) {
		robot.commands = append(robot.commands, command)
		if command == "sudo -n true" {
			return "", 1 // the node's sudo asks for the password
		}
		text := strings.TrimPrefix(string(stdin), "node-secret\n")
		switch {
		case strings.Contains(command, "cat > $B/installer.tar.new"):
			robot.received = []byte(text)
		case strings.Contains(text, "sha256sum $B/installer.tar"):
			if robot.received == nil {
				return robot.observation + "installer=\n", 0
			}
			digest := sha256.Sum256(robot.received)
			return robot.observation + "installer=" + hex.EncodeToString(digest[:]) + "\n", 0
		case text == installObservationScript:
			return robot.observation, 0
		default:
			robot.nodeScripts = append(robot.nodeScripts, text)
		}
		return "", 0
	}
	return robot
}

func TestInstallerJoinRunsTheInstallerOnceWithACodeOfItsOwn(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = clientscheme.AddToScheme(scheme)
	_ = stationv1alpha1.AddToScheme(scheme)
	robot := newInstallRobot(t)
	directory := t.TempDir()
	_ = os.MkdirAll(filepath.Join(directory, "addons/s1u-robot/bootstrap"), 0755)
	_ = os.WriteFile(filepath.Join(directory, "addons/s1u-robot/bootstrap/orin-uplink.sh"), []byte("echo let the x86 out\n"), 0644)
	_ = os.WriteFile(filepath.Join(directory, "installer.tar"), []byte("the installer"), 0644)
	_ = os.WriteFile(filepath.Join(directory, "ca.crt"), []byte("-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----\n"), 0644)

	class := &stationv1alpha1.NodeClass{ObjectMeta: metav1.ObjectMeta{Name: "s1u"}, Spec: stationv1alpha1.NodeClassSpec{
		Access: &stationv1alpha1.NodeClassAccess{
			Entry: stationv1alpha1.NodeClassEntry{Port: robot.entry.port(), User: "orin", SerialFile: "/proc/device-tree/serial-number", RotateHostKey: true, Password: stationv1alpha1.SecretValue{Secret: "s1u-access", Key: "password"}},
			Node:  stationv1alpha1.NodeClassHost{Address: "192.168.0.10", Port: 22, User: "robot", Password: stationv1alpha1.SecretValue{Secret: "s1u-access", Key: "x86_password"}}},
		Uplink: &stationv1alpha1.NodeClassUplink{Setup: stationv1alpha1.NodeClassUplinkSetup{Addon: "s1u-robot", Path: "bootstrap/orin-uplink.sh"}}}}
	accounts := rlarkSecret("s1u-access", map[string]string{"password": "entry-secret", "x86_password": "node-secret"})
	admission := &stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "s1u-03-x86"},
		Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "cluster-a", CodeHash: strings.Repeat("0", 64), NodeClass: "s1u", PublicKey: "old", Serial: "1794925030610"}}
	entryAddress, _, _ := strings.Cut(robot.entry.address(), ":")
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "join", UID: "op-uid"},
		Spec: stationv1alpha1.NodeOperationSpec{Action: "Join", ClusterID: "cluster-a", NodeName: "s1u-03-x86", Admission: "s1u-03-x86", NodeClass: "s1u", RequestedBy: "alice",
			SSH: &stationv1alpha1.NodeSSHConnection{Address: "192.168.0.10", Port: 22, User: "robot", HostKeyFingerprint: robot.node.fingerprint(),
				Jump: &stationv1alpha1.NodeSSHHop{Address: entryAddress, Port: robot.entry.port(), User: "orin", HostKeyFingerprint: robot.entry.fingerprint()}}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(class, accounts, admission).Build()
	member := kubefake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "s1u-03-x86", UID: "node-uid"}})
	driver := &InstallerJoinDriver{Reader: c, Writer: c, Access: NodeClassAccess{Reader: c, Namespace: "default"}, Members: installMembers{member},
		BundleFile: filepath.Join(directory, "installer.tar"), AddonsDirs: []string{filepath.Join(directory, "no-such-directory"), filepath.Join(directory, "addons")}, StationAPI: "https://10.11.14.128:18443", CAFile: filepath.Join(directory, "ca.crt")}
	stage := func(name string) (bool, string, error) { return driver.Execute(ctx, op, name) }

	if done, _, err := stage("Preflight"); !done || err != nil {
		t.Fatalf("Preflight: %v %v", done, err)
	}
	if done, _, err := stage("Uplink"); !done || err != nil || len(robot.entryScripts) != 1 || robot.entryScripts[0] != "echo let the x86 out\n" {
		t.Fatalf("Uplink runs the class's setup on the entry device: %v %v %q", done, err, robot.entryScripts)
	}
	if done, _, err := stage("Push"); done || err != nil || string(robot.received) != "the installer" {
		t.Fatalf("Push sends the installer: %v %v %q", done, err, robot.received)
	}
	if done, _, err := stage("Push"); !done || err != nil {
		t.Fatalf("Push is done once the machine holds the file: %v %v", done, err)
	}

	// The installer is started with a code that only the machine and the admission's hash know.
	if done, _, err := stage("Install"); done || err != nil || len(robot.nodeScripts) != 2 {
		t.Fatalf("Install starts the installer: %v %v %d scripts", done, err, len(robot.nodeScripts))
	}
	start := robot.nodeScripts[1]
	code := regexp.MustCompile(`RLARK_CODE=([a-z0-9]{24})\n`).FindStringSubmatch(start)
	stored := &stationv1alpha1.NodeAdmission{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(admission), stored); err != nil || code == nil {
		t.Fatalf("the code in the start script: %v %v", code, err)
	}
	digest := sha256.Sum256([]byte(code[1]))
	if stored.Spec.CodeHash != hex.EncodeToString(digest[:]) || stored.Spec.PublicKey != "" || stored.Spec.IssuedBy != "alice" || !stored.Spec.ExpiresAt.After(stored.Spec.IssuedAt.Time) {
		t.Fatalf("the admission is open for the code it was given: %+v", stored.Spec)
	}
	for _, command := range robot.commands {
		if strings.Contains(command, code[1]) || strings.Contains(command, "secret") {
			t.Fatalf("a command line carries a secret: %s", command)
		}
	}
	if !strings.Contains(start, "tar -xf $B/installer.tar -C $B\n") || !strings.Contains(start, "RLARK_SERVER=https://10.11.14.128:18443\n") || !strings.Contains(start, "printf '%s' 'op-uid' > $B/operation") || !strings.Contains(start, "-----BEGIN CERTIFICATE-----\nCA\n") {
		t.Fatalf("the start script: %s", start)
	}

	// While it runs, and when its end is not known, nothing is started again.
	robot.observation = "operation=op-uid\nunit=active\nresult=\ncertificate=\nfailed=\n"
	if done, _, err := stage("Install"); done || err != nil {
		t.Fatalf("a running installer: %v %v", done, err)
	}
	robot.observation = "operation=op-uid\nunit=inactive\nresult=\ncertificate=\nfailed=\n"
	if _, _, err := stage("Install"); !errors.Is(err, ErrJoinOutcomeUnknown) {
		t.Fatalf("an installer whose end is unknown: %v", err)
	}
	robot.observation = "operation=op-uid\nunit=inactive\nresult=1\ncertificate=\nfailed=12:00:00 FAILED: no peer has answered through the tunnel after 2 min\n"
	if _, _, err := stage("Install"); !errors.Is(err, ErrInstallerFailed) || !strings.Contains(err.Error(), "no peer has answered") {
		t.Fatalf("a failed installer says why: %v", err)
	}
	robot.observation = "operation=op-uid\nunit=inactive\nresult=0\ncertificate=" + kubeletCertificate(t, "s1u-03-x86") + "\nfailed=\n"
	if done, uid, err := stage("Install"); !done || uid != "node-uid" || err != nil || len(robot.nodeScripts) != 2 {
		t.Fatalf("a finished installer: %v %q %v, %d scripts", done, uid, err, len(robot.nodeScripts))
	}
	// A machine that is this node already is not installed: its join only finishes what is left.
	robot.observation = "operation=\nunit=inactive\nresult=\ncertificate=" + kubeletCertificate(t, "s1u-03-x86") + "\nfailed=\n"
	if done, uid, err := stage("Install"); !done || uid != "node-uid" || err != nil || len(robot.nodeScripts) != 2 {
		t.Fatalf("a machine that is the node already: %v %q %v", done, uid, err)
	}
	robot.received = nil
	if done, _, err := stage("Push"); !done || err != nil || robot.received != nil {
		t.Fatalf("a machine that is the node already is sent no installer: %v %v %q", done, err, robot.received)
	}
	robot.observation = "operation=\nunit=inactive\nresult=\ncertificate=" + kubeletCertificate(t, "another") + "\nfailed=\n"
	if _, _, err := stage("Install"); !errors.Is(err, ErrNodeAlreadyConfigured) {
		t.Fatalf("a machine that is another node: %v", err)
	}

	// The join waits for the Addons of the class: each with a ready pod of its DaemonSet on this node.
	op.Status.NodeUID = "node-uid"
	class.Spec.Addons = []string{"s1u-robot"}
	_ = c.Update(ctx, class)
	if done, _, err := stage("VerifyAddons"); done || err != nil {
		t.Fatalf("an Addon that is not installed: %v %v", done, err)
	}
	selector := map[string]string{"app.kubernetes.io/name": "s1u-robot"}
	_, _ = member.AppsV1().DaemonSets("rlark-system").Create(ctx, &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "s1u-robot", Labels: map[string]string{addonNameLabel: "s1u-robot"}},
		Spec: appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: selector}}}, metav1.CreateOptions{})
	pod := func(name, node string, ready corev1.ConditionStatus) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-system", Name: name, Labels: selector}, Spec: corev1.PodSpec{NodeName: node},
			Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: ready}}}}
	}
	_, _ = member.CoreV1().Pods("rlark-system").Create(ctx, pod("on-another-robot", "s1u-01-x86", corev1.ConditionTrue), metav1.CreateOptions{})
	_, _ = member.CoreV1().Pods("rlark-system").Create(ctx, pod("starting", "s1u-03-x86", corev1.ConditionFalse), metav1.CreateOptions{})
	if done, _, err := stage("VerifyAddons"); done || err != nil {
		t.Fatalf("an Addon whose pod on this node is not ready, and is ready on another: %v %v", done, err)
	}
	_, _ = member.CoreV1().Pods("rlark-system").UpdateStatus(ctx, pod("starting", "s1u-03-x86", corev1.ConditionTrue), metav1.UpdateOptions{})
	if done, uid, err := stage("VerifyAddons"); !done || uid != "node-uid" || err != nil {
		t.Fatalf("an Addon with a ready pod on this node: %v %v", done, err)
	}
	class.Spec.Addons = nil
	_ = c.Update(ctx, class)
	if done, _, err := stage("VerifyAddons"); !done || err != nil {
		t.Fatalf("a class that names no Addon: %v %v", done, err)
	}

	// The entry device's new key is written down before the device shows it.
	next := "SHA256:" + strings.Repeat("n", 43)
	if done, _, err := stage("RotateEntryHostKey"); done || err != nil {
		t.Fatalf("RotateEntryHostKey: %v %v", done, err)
	}
	_ = c.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: "s1u-03-x86"}, stored)
	node, _ := member.CoreV1().Nodes().Get(ctx, "s1u-03-x86", metav1.GetOptions{})
	if stored.Annotations[EntryHostKeyAnnotation] != next || node.Annotations[EntryHostKeyAnnotation] != next || !strings.Contains(robot.entryScripts[len(robot.entryScripts)-1], "systemctl restart ssh") {
		t.Fatalf("the new key on the admission %q and the node %q", stored.Annotations[EntryHostKeyAnnotation], node.Annotations[EntryHostKeyAnnotation])
	}
	// The device now shows a key; only the one that was written down ends the stage.
	robot.entry.setKey(newTestSSHKey(t))
	var mismatch *SSHHostKeyMismatchError
	if _, _, err := stage("RotateEntryHostKey"); !errors.As(err, &mismatch) {
		t.Fatalf("an entry device with a key nobody wrote down: %v", err)
	}
	stored.Annotations[EntryHostKeyAnnotation] = robot.entry.fingerprint()
	_ = c.Update(ctx, stored)
	if done, uid, err := stage("RotateEntryHostKey"); !done || uid != "node-uid" || err != nil {
		t.Fatalf("an entry device that shows its written-down key: %v %v", done, err)
	}

	// The node was removed while the machine could not be reached: admitted
	// again, it is sent the installer, and what it kept of the node is reset.
	_ = member.CoreV1().Nodes().Delete(ctx, "s1u-03-x86", metav1.DeleteOptions{})
	op.Spec.SSH.Jump.HostKeyFingerprint = robot.entry.fingerprint()
	robot.observation = "operation=an-earlier-join\nunit=inactive\nresult=0\ncertificate=" + kubeletCertificate(t, "s1u-03-x86") + "\nfailed=\n"
	robot.received = nil
	if done, _, err := stage("Push"); done || err != nil || string(robot.received) != "the installer" {
		t.Fatalf("a machine whose node was removed is sent the installer: %v %v %q", done, err, robot.received)
	}
	scripts := len(robot.nodeScripts)
	if done, _, err := stage("Install"); done || err != nil || len(robot.nodeScripts) != scripts+1 || robot.nodeScripts[scripts] != staleNodeResetScript {
		t.Fatalf("a machine whose node was removed is reset first: %v %v %q", done, err, robot.nodeScripts[scripts:])
	}
}

func TestInstallerJoinNamesARefusedAccount(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = clientscheme.AddToScheme(scheme)
	_ = stationv1alpha1.AddToScheme(scheme)
	robot := newInstallRobot(t)
	robot.node.password = "somebody changed it"
	class := &stationv1alpha1.NodeClass{ObjectMeta: metav1.ObjectMeta{Name: "s1u"}, Spec: stationv1alpha1.NodeClassSpec{Access: &stationv1alpha1.NodeClassAccess{
		Entry: stationv1alpha1.NodeClassEntry{Port: robot.entry.port(), User: "orin", SerialFile: "/serial", Password: stationv1alpha1.SecretValue{Secret: "s1u-access", Key: "password"}},
		Node:  stationv1alpha1.NodeClassHost{Address: "192.168.0.10", Port: 22, User: "robot", Password: stationv1alpha1.SecretValue{Secret: "s1u-access", Key: "x86_password"}}}}}
	accounts := rlarkSecret("s1u-access", map[string]string{"password": "entry-secret", "x86_password": "node-secret"})
	entryAddress, _, _ := strings.Cut(robot.entry.address(), ":")
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "join", UID: "op-uid", Generation: 1},
		Spec: stationv1alpha1.NodeOperationSpec{RequestID: "r", Action: "Join", ClusterID: "cluster-a", NodeName: "s1u-03-x86", Admission: "s1u-03-x86", NodeClass: "s1u", RequestedBy: "alice",
			SSH: &stationv1alpha1.NodeSSHConnection{Address: "192.168.0.10", Port: 22, User: "robot", HostKeyFingerprint: robot.node.fingerprint(),
				Jump: &stationv1alpha1.NodeSSHHop{Address: entryAddress, Port: robot.entry.port(), User: "orin", HostKeyFingerprint: robot.entry.fingerprint()}}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(op).WithObjects(class, accounts, op).Build()
	executor := &NodeLifecycleExecutor{Installer: &InstallerJoinDriver{Reader: c, Writer: c, Access: NodeClassAccess{Reader: c, Namespace: "default"}}}
	r := NewNodeOperationReconciler(c, c, testRegistrations{testNamespace: {ClusterID: "cluster-a", Namespace: testNamespace}}, executor)
	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "join"}}); !errors.Is(err, ErrSSHCredentialRejected) {
		t.Fatalf("a node that refuses the class's account: %v", err)
	}
	stored := &stationv1alpha1.NodeOperation{}
	_ = c.Get(ctx, client.ObjectKeyFromObject(op), stored)
	if stored.Status.Phase != "Blocked" || len(stored.Status.Conditions) != 1 || stored.Status.Conditions[0].Reason != stationv1alpha1.MachineCredentialRejected {
		t.Fatalf("the operation says the account was refused: %+v", stored.Status)
	}
}

// kubeletCertificate is what the observation script prints of a kubelet's client certificate.
func kubeletCertificate(t *testing.T, node string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{Organization: []string{"system:nodes"}, CommonName: "system:node:" + node}, NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// rlarkSecret is the declaration of a secret kept in RLark, as its Gateway writes it.
func rlarkSecret(name string, values map[string]string) *corev1.Secret {
	delivered, _ := yaml.Marshal([]corev1.Secret{{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "rlark-system"}, StringData: values}})
	delivery, _ := yaml.Marshal([]corev1.Secret{{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: metav1.ObjectMeta{Name: "js-" + name + "-delivery"}, Data: map[string][]byte{"manifests": delivered}}})
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "js-" + name + "-replication"}, Data: map[string][]byte{"manifests": delivery}}
}

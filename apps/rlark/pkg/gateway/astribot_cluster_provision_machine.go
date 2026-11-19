package gateway

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// provisionMachine is a logged-in session on the first machine.
type provisionMachine interface {
	// Run runs a script of RLark's own as root. stdin, when not nil, is what the script reads; each line it
	// prints goes to line, when not nil. It answers what the script printed; a script that fails answers an error
	// that carries its last lines.
	Run(ctx context.Context, script string, stdin io.Reader, line func(string)) (string, error)
	Close() error
}

// provisionDialFunc logs in to the machine with its password. A hostKey (SHA256 fingerprint) that is not empty is
// the key the machine must show; an empty one takes the key it shows, which is answered.
type provisionDialFunc func(ctx context.Context, machine rlarkv1alpha1.ClusterProvisionMachine, password []byte, hostKey string) (provisionMachine, string, error)

const (
	// provisionBootstrapDir is where the installer is unpacked on the machine, as Station API's join puts it.
	provisionBootstrapDir = "/var/lib/rlark/bootstrap"
	// provisionDiskFloor is the free space /var/lib needs: the installer, the images of the control plane and of
	// RLark's components.
	provisionDiskFloor = 50 << 30
	// provisionCacheVolume is the site cache's volume, made when the machine has room for it beside the floor.
	provisionCacheVolume = "site-cache=300G:/var/lib/rlark/site-cache"
	provisionCacheFloor  = 300<<30 + provisionDiskFloor
	// provisionTunnelPort is the WireGuard port of a site's hub that has an address of its own.
	provisionTunnelPort = 51820
)

// precheckScript reads what the stages need to know of the machine; it changes nothing but loading the
// wireguard module.
const precheckScript = `set -u
echo "hostname=$(hostname | tr '[:upper:]' '[:lower:]')"
. /etc/os-release && echo "os=$PRETTY_NAME"
echo "kernel=$(uname -r)"
[ -d /run/systemd/system ] && echo systemd=yes
command -v tar >/dev/null && echo tar=yes
modprobe wireguard 2>/dev/null && echo wireguard=yes
command -v nvidia-smi >/dev/null && nvidia-smi --query-gpu=name --format=csv,noheader | sed 's/^/gpu=/'
echo "diskfree=$(df -B1 --output=avail /var/lib | tail -1 | tr -d ' ')"
dev=$(ip -4 route show default | awk '{for (i = 1; i < NF; i++) if ($i == "dev") {print $(i + 1); exit}}')
[ -n "$dev" ] && ip -4 -o addr show dev "$dev" | awk '{split($4, a, "/"); print "address=" a[1]; exit}'
ip -4 route show | awk '$1 != "default" {print "route=" $1}'
ip -4 -o addr show | awk '{print "route=" $4}'
echo "bundle=$(cat ` + provisionBootstrapDir + `/bundle.sha256 2>/dev/null)"
[ -s /var/lib/station/wg/private.key ] && [ -x /opt/rlark/bin/station-gateway ] && echo "publickey=$(/opt/rlark/bin/station-gateway --public-key)"
`

func (r *provisionRun) precheck(ctx context.Context, p *rlarkv1alpha1.ClusterProvision) (string, error) {
	machine, err := r.needMachine(ctx, p)
	if err != nil {
		return "", err
	}
	facts, readback, err := readMachine(ctx, machine, provisionDiskFloor)
	p.Status.Facts = facts
	if err != nil {
		return readback, err
	}
	if facts.NodeName == "" || len(facts.NodeName) > 63 || strings.ContainsAny(facts.NodeName, "._ ") || strings.Trim(facts.NodeName, "-") != facts.NodeName {
		return readback, fmt.Errorf("the machine's hostname %q is not a node name (lower-case letters, digits and -)", facts.NodeName)
	}
	if taken, err := r.g.nodeNameTaken(ctx, facts.NodeName); err != nil {
		return readback, err
	} else if taken != "" {
		return readback, fmt.Errorf("a node named %s is already %s: rename the machine (hostnamectl set-hostname) and retry", facts.NodeName, taken)
	}
	return readback, nil
}

// readMachine reads what a machine that becomes a node of a site needs, and refuses one that lacks it: systemd, tar,
// the wireguard module, an address of its own network, floor bytes free in /var/lib. It reads too the sha256 of the
// installer the machine holds and its tunnel key, when it has them.
func readMachine(ctx context.Context, machine provisionMachine, floor int64) (rlarkv1alpha1.ClusterProvisionFacts, string, error) {
	var facts rlarkv1alpha1.ClusterProvisionFacts
	out, err := machine.Run(ctx, precheckScript, nil, nil)
	if err != nil {
		return facts, "", err
	}
	seen := map[string]bool{}
	gpus := map[string]int32{}
	var free int64
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		seen[key] = true
		switch key {
		case "hostname":
			facts.NodeName = value
		case "os":
			facts.OS = value
		case "kernel":
			facts.Kernel = value
		case "gpu":
			gpus[strings.TrimSpace(value)]++
		case "diskfree":
			free, _ = strconv.ParseInt(value, 10, 64)
		case "address":
			facts.PrivateAddress = value
		case "bundle":
			facts.BundleSHA256 = value
		case "publickey":
			facts.PublicKey = value
		case "route":
			if prefix, err := netip.ParsePrefix(value); err == nil && prefix.Addr().Is4() && !prefix.Addr().IsLoopback() {
				facts.Routes = append(facts.Routes, prefix.Masked().String())
			}
		}
	}
	for _, model := range slices.Sorted(maps.Keys(gpus)) {
		facts.GPUs = append(facts.GPUs, rlarkv1alpha1.ClusterProvisionGPU{Model: model, Count: gpus[model]})
	}
	slices.Sort(facts.Routes)
	facts.Routes = slices.Compact(facts.Routes)
	facts.DiskFree = fmt.Sprintf("%d GiB", free>>30)
	facts.CacheVolume = free >= provisionCacheFloor
	readback := fmt.Sprintf("%s, %s, kernel %s; %s; /var/lib %s free; private address %s", facts.NodeName, facts.OS, facts.Kernel, describeGPUs(facts.GPUs), facts.DiskFree, facts.PrivateAddress)

	var missing []string
	for key, what := range map[string]string{"systemd": "systemd", "tar": "tar", "wireguard": "the wireguard kernel module"} {
		if !seen[key] {
			missing = append(missing, what)
		}
	}
	slices.Sort(missing)
	switch {
	case len(missing) > 0:
		return facts, readback, fmt.Errorf("the machine has no %s", strings.Join(missing, ", "))
	case facts.PrivateAddress == "":
		return facts, readback, errors.New("the machine has no default route, so no address of its own network")
	case free < floor:
		return facts, readback, fmt.Errorf("/var/lib has %s free; the installer and the images need %d GiB", facts.DiskFree, floor>>30)
	}
	return facts, readback, nil
}

func describeGPUs(gpus []rlarkv1alpha1.ClusterProvisionGPU) string {
	if len(gpus) == 0 {
		return "no NVIDIA GPU"
	}
	parts := make([]string, 0, len(gpus))
	for _, gpu := range gpus {
		parts = append(parts, fmt.Sprintf("%d × %s", gpu.Count, gpu.Model))
	}
	return strings.Join(parts, ", ")
}

// nodeNameTaken says where a node of that name already is, if anywhere: a node of a cluster, or one the
// SiteNetwork declares.
func (g *Gateway) nodeNameTaken(ctx context.Context, name string) (string, error) {
	nodes, err := g.kubeClient.RlinfV1alpha1().Nodes("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list nodes: %w", err)
	}
	for _, node := range nodes.Items {
		if node.Name == name {
			return "a node of " + node.Namespace, nil
		}
	}
	network, err := g.provisionNetwork(ctx)
	if err != nil {
		return "", err
	}
	for _, node := range network.nodes() {
		if node.NodeName == name {
			return "declared in the SiteNetwork for " + node.ClusterID, nil
		}
	}
	return "", nil
}

// bundleScript takes the installer of sha256 expect from stdin, unpacks it where the join's installer is and marks it
// with its sha256 (bundle.sha256, which the code's line reads too), then makes the machine's tunnel key (tunnelKeyScript).
func bundleScript(registry, expect string) string {
	return `set -eu
D=` + provisionBootstrapDir + `
install -d /var/lib/rlark /opt/rlark/bin /etc/rlark/station
cat > /var/lib/rlark/installer.tar.new
sum=$(sha256sum /var/lib/rlark/installer.tar.new | cut -d' ' -f1)
echo "sha256=$sum"
[ "$sum" = "` + expect + `" ] || { rm -f /var/lib/rlark/installer.tar.new; echo "the installer received is not of sha256 ` + expect + `" >&2; exit 1; }
rm -rf $D.new && mkdir $D.new && tar -x -C $D.new -f /var/lib/rlark/installer.tar.new
rm -rf $D && mv $D.new $D && rm /var/lib/rlark/installer.tar.new
echo "$sum" > $D/bundle.sha256
` + tunnelKeyScript(registry)
}

// tunnelKeyScript makes the machine's tunnel key from the installer it holds, which never leaves it. It prints the CA
// of the site registry the installer trusts, which the cluster's registry Addons are given.
func tunnelKeyScript(registry string) string {
	return `set -eu
D=` + provisionBootstrapDir + `
install -d /opt/rlark/bin /etc/rlark/station
install -m 755 $D/bin/station-gateway /opt/rlark/bin/
echo "publickey=$(/opt/rlark/bin/station-gateway --public-key)"
echo "ca=$(base64 -w0 $D/certs.d/` + registry + `/ca.crt)"
`
}

func (r *provisionRun) bundle(ctx context.Context, p *rlarkv1alpha1.ClusterProvision) (string, error) {
	release, err := r.g.currentRelease(ctx)
	if err != nil {
		return "", err
	}
	machine, err := r.needMachine(ctx, p)
	if err != nil {
		return "", err
	}
	installed, readback, err := r.g.sendInstaller(ctx, machine, release.Registry, p.Status.Facts.BundleSHA256, func(line string) { r.line(ctx, stageBundle, line) })
	facts := &p.Status.Facts
	facts.BundleSHA256, facts.PublicKey, facts.RegistryCA = installed.sha256, installed.publicKey, installed.registryCA
	return readback, err
}

// installedMachine is what sendInstaller leaves on a machine: Station API's installer, of sha256, and the machine's
// tunnel key, made there; registryCA is the CA of the site registry the installer trusts.
type installedMachine struct {
	sha256, publicKey, registryCA string
}

// sendInstaller gives the machine Station API's installer, unless the one it holds (of sha256 have) is that one
// already, and has the machine make its tunnel key, which never leaves it. While it streams, progress is told how
// much has gone every 5 s.
func (g *Gateway) sendInstaller(ctx context.Context, machine provisionMachine, registry, have string, progress func(string)) (installedMachine, string, error) {
	var installed installedMachine
	want, err := g.stationBootstrap.BundleDigest(ctx)
	if err != nil {
		return installed, "", fmt.Errorf("read the installer's sha256 from Station API: %w", err)
	}
	var out, readback string
	if have == want {
		if out, err = machine.Run(ctx, tunnelKeyScript(registry), nil, nil); err != nil {
			return installed, "", err
		}
		readback = fmt.Sprintf("the installer of sha256 %s… is on the machine already, not sent again", short(want))
	} else {
		installer, size, err := g.stationBootstrap.Open(ctx, "/v1/bootstrap/bundle")
		if err != nil {
			return installed, "", fmt.Errorf("take the installer from Station API: %w", err)
		}
		defer installer.Close()
		sent := &sentCounter{reader: installer, size: size, progress: progress, started: time.Now()}
		if out, err = machine.Run(ctx, bundleScript(registry, want), sent, nil); err != nil {
			return installed, "", err
		}
		readback = fmt.Sprintf("installer sha256 %s… sent (%s in %s)", short(provisionValues(out)["sha256"]), megabytes(sent.count), time.Since(sent.started).Round(time.Second))
	}
	values := provisionValues(out)
	ca, _ := base64.StdEncoding.DecodeString(values["ca"])
	installed = installedMachine{sha256: want, publicKey: values["publickey"], registryCA: string(ca)}
	readback += "; tunnel public key " + installed.publicKey
	if values["sha256"] != "" && values["sha256"] != want {
		return installed, readback, fmt.Errorf("the machine holds an installer of sha256 %s, Station API's is %s", values["sha256"], want)
	}
	if !strings.Contains(installed.registryCA, "BEGIN CERTIFICATE") {
		return installed, readback, fmt.Errorf("the installer holds no CA of %s (certs.d/%s/ca.crt)", registry, registry)
	}
	if len(installed.publicKey) != 44 {
		return installed, readback, fmt.Errorf("station-gateway answered no WireGuard public key (%q)", installed.publicKey)
	}
	return installed, readback, nil
}

// sentCounter counts what the installer's stream has given, and tells progress every 5 s.
type sentCounter struct {
	reader   io.Reader
	size     int64
	count    int64
	progress func(string)
	started  time.Time
	told     time.Time
}

func (s *sentCounter) Read(p []byte) (int, error) {
	n, err := s.reader.Read(p)
	s.count += int64(n)
	if now := time.Now(); now.Sub(s.told) >= 5*time.Second {
		s.told = now
		rate := float64(s.count) / max(now.Sub(s.started).Seconds(), 1) / (1 << 20)
		s.progress(fmt.Sprintf("sent %s of %s, %.1f MB/s", megabytes(s.count), megabytes(s.size), rate))
	}
	return n, err
}

func megabytes(n int64) string {
	if n >= 1<<30 {
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%d MB", n>>20)
}

// provisionValues reads the key=value lines a script printed.
func provisionValues(out string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			values[key] = value
		}
	}
	return values
}

func short(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}

// controlPlaneScript puts the tunnel document the SiteNetwork published for the node (stdin) where the installer
// reads it, and makes the machine the cluster's control plane; run again, the installer goes on where it stopped.
func controlPlaneScript(facts rlarkv1alpha1.ClusterProvisionFacts) string {
	// WORKLOADS: the first machine is the cluster's control plane and its worker too (its GPUs run the site's Jobs).
	env := fmt.Sprintf("CONTROL_PLANE_NAME=%s POD_SUBNET=%s SERVICE_SUBNET=%s NODE_NAME=%s WORKLOADS=yes", facts.ControlPlaneName, facts.PodSubnet, facts.ServiceSubnet, facts.NodeName)
	if facts.CacheVolume {
		env += " NODE_VOLUMES=" + provisionCacheVolume
	}
	return `set -eu
umask 077
cat > /etc/rlark/station/bootstrap.json
cd ` + provisionBootstrapDir + `
` + env + ` ./station-bootstrap control-plane 2>&1
K="` + provisionBootstrapDir + `/bin/kubectl --kubeconfig /etc/kubernetes/admin.conf"
$K wait --for=condition=Ready node/` + facts.NodeName + ` --timeout=300s >/dev/null || true
$K get node ` + facts.NodeName + ` -o jsonpath='ready={.status.conditions[?(@.type=="Ready")].status}{"\n"}'
`
}

func (r *provisionRun) controlPlane(ctx context.Context, p *rlarkv1alpha1.ClusterProvision) (string, error) {
	machine, err := r.needMachine(ctx, p)
	if err != nil {
		return "", err
	}
	facts := p.Status.Facts
	network, err := r.g.provisionNetwork(ctx)
	if err != nil {
		return "", err
	}
	document := network.declared(p.Name, facts.NodeName)
	if document == "" {
		return "", fmt.Errorf("the SiteNetwork publishes no tunnel document for %s (status.declared): station-controller has not taken the declaration", facts.NodeName)
	}
	// The machine's tunnel first, waited for on its peers' reports (awaitTunnel): the installer's own wait for a
	// handshake (station-gateway --await-peers) has no Station API to ask on a declared hub.
	if _, err := machine.Run(ctx, tunnelScript, strings.NewReader(document), nil); err != nil {
		return "", err
	}
	var seen rlarkv1alpha1.NodeJoinFacts
	tunnel, err := r.g.awaitTunnel(ctx, machine, p.Name+"/"+facts.NodeName, document, p.Spec.Machine.TunnelEndpoint, joinClock(), &seen,
		func(message string) { r.line(ctx, stageControlPlane, message) })
	if err != nil {
		return tunnel, err
	}
	out, err := machine.Run(ctx, controlPlaneScript(facts), strings.NewReader(document), func(line string) { r.line(ctx, stageControlPlane, line) })
	if err != nil {
		return tunnel, err
	}
	if ready := provisionValues(out)["ready"]; ready != "True" {
		return tunnel + "; node " + facts.NodeName + " Ready=" + ready, fmt.Errorf("the control plane is made, but node %s is not Ready", facts.NodeName)
	}
	// The management plane reaches the API through the tunnel: the first handshake shows the hub's UDP port is open.
	address := net.JoinHostPort(facts.TunnelAddress, "6443")
	var dialErr error
	for deadline := time.Now().Add(2 * time.Minute); time.Now().Before(deadline); time.Sleep(3 * time.Second) {
		var connection net.Conn
		if connection, dialErr = (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", address); dialErr == nil {
			_ = connection.Close()
			break
		}
	}
	readback := fmt.Sprintf("%s; node %s Ready; %s", tunnel, facts.NodeName, lastLine(out, "Done"))
	if dialErr != nil {
		return readback, fmt.Errorf("the management plane does not reach the API at %s through the tunnel (does UDP %s reach the machine?): %v", address, p.Spec.Machine.TunnelEndpoint, dialErr)
	}
	return readback + "; the API answers at " + address + " through the tunnel", nil
}

// lastLine is the last line of out that holds want, or its last line.
func lastLine(out, want string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], want) {
			return strings.TrimSpace(lines[i])
		}
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

// needAdmin reads the cluster's administrator credential from the machine, once in a run, into memory only. The
// API is reached at the control plane's tunnel address, by the name its certificate holds.
func (r *provisionRun) needAdmin(ctx context.Context, p *rlarkv1alpha1.ClusterProvision) (*rest.Config, error) {
	if r.admin != nil {
		return r.admin, nil
	}
	machine, err := r.needMachine(ctx, p)
	if err != nil {
		return nil, err
	}
	kubeconfig, err := machine.Run(ctx, "cat /etc/kubernetes/admin.conf", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("read the cluster's admin.conf: %w", err)
	}
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("the cluster's admin.conf: %w", err)
	}
	config.Host = "https://" + net.JoinHostPort(p.Status.Facts.TunnelAddress, "6443")
	config.ServerName = p.Status.Facts.ControlPlaneName
	r.admin = config
	return config, nil
}

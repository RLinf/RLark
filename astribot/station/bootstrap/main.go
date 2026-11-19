// station-bootstrap turns a machine into a node. It is the entry point of the
// installer image an operator runs once, privileged and in the host's PID and
// network namespaces:
//
//	docker run --rm --privileged --net=host --pid=host -v CA:/ca.crt:ro \
//	  -e RLARK_SERVER=https://station-api:18443 -e RLARK_CODE=<one-time code> rlark/node-bootstrap
//
// The same program runs without a container where there is no docker: the
// installer's tar (build.sh, OUT.bundle.tar) unpacked in /var/lib/rlark/bootstrap
// holds it beside the bundle, and RLark starts it there over SSH with
// RLARK_SERVER, RLARK_CODE and RLARK_CA_FILE.
//
// It copies its bundle to the host, creates the machine's WireGuard key,
// redeems the code at Station API for the tunnel configuration and a join
// token, and hands over to install.sh, which runs as a host process.
//
// With the argument "tunnel" it runs the site tunnel on the document an
// administrator put on the machine (tunnel.sh): a machine the SiteNetwork
// declares with its key reaches Station API through it, and its join command
// redeems the code there.
//
// With the argument "control-plane" it makes the machine the control plane of
// a cluster instead (control-plane.sh); that asks Station API for nothing and
// carries no credential:
//
//	docker run --rm --privileged --net=host --pid=host \
//	  -e CONTROL_PLANE_NAME=cp.<cluster-id>.rlark.internal rlark/node-bootstrap control-plane
package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"time"

	"sigs.k8s.io/yaml"
)

const (
	hostRoot = "/proc/1/root" // the host's filesystem, visible because of --pid=host
	stage    = "/var/lib/rlark/bootstrap"
)

type answer struct {
	State         string          `json:"state"`
	Reason        string          `json:"reason"`
	ClusterID     string          `json:"cluster_id"`
	NodeName      string          `json:"node_name"`
	TunnelAddress string          `json:"tunnel_address"`
	Config        json.RawMessage `json:"config"`
	Join          struct {
		APIServer  string `json:"api_server"`
		Token      string `json:"token"`
		CACertHash string `json:"ca_cert_hash"`
	} `json:"join"`
	Labels string `json:"labels"`
	Taints string `json:"taints"`
	// The node's volumes, "NAME=SIZE:PATH,..." (lib.sh, volumes).
	Volumes string `json:"volumes"`
}

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) == 4 && os.Args[1] == "kubelet-overrides" {
		kubeletOverrides(os.Args[2], os.Args[3])
		return
	}
	if len(os.Args) > 3 && os.Args[1] == "cert-sans" {
		certSANs(os.Args[2], os.Args[3], os.Args[4:])
		return
	}
	if len(os.Args) == 5 && os.Args[1] == "controller-manager-arg" {
		controllerManagerArg(os.Args[2], os.Args[3], os.Args[4])
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "cert-addresses" {
		certAddresses(os.Args[2], os.Args[3:])
		return
	}
	if len(os.Args) == 6 && os.Args[1] == "snapshot-serve" {
		snapshotServe(os.Args[2], os.Args[3], os.Args[4], os.Args[5])
		return
	}
	if len(os.Args) == 5 && os.Args[1] == "snapshot-fetch" {
		if err := snapshotFetch(os.Args[2], os.Args[3], os.Args[4]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if os.Geteuid() != 0 {
		log.Fatal("this must run as root: start the command with sudo, and give the container no other user")
	}
	if os.Getenv("PHASE") != "" {
		onHost("upgrade.sh")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "control-plane" {
		onHost("control-plane.sh", os.Args[2:]...)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "tunnel" {
		onHost("tunnel.sh")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "volumes" {
		onHost("volumes.sh", os.Args[2:]...)
		return
	}
	server, code := os.Getenv("RLARK_SERVER"), os.Getenv("RLARK_CODE")
	caFile := "/ca.crt"
	if named := os.Getenv("RLARK_CA_FILE"); named != "" {
		caFile = named
	}
	ca, err := os.ReadFile(caFile)
	if err != nil && os.Getenv("RLARK_CA") != "" {
		// A Job that moves a node hands the CA over in the environment.
		ca, err = []byte(os.Getenv("RLARK_CA")), nil
	}
	if server == "" || code == "" || err != nil {
		log.Fatal("RLARK_SERVER, RLARK_CODE and the CA (/ca.crt, RLARK_CA_FILE or RLARK_CA) are required")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		log.Fatal("/ca.crt holds no certificate")
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}

	// A wrong clock fails later in ways that do not name it: certificates
	// that are "not yet valid", a join token that has "expired".
	response, err := client.Get(server + "/healthz")
	if err != nil {
		log.Fatalf("Station API is not reachable at %s: %v", server, err)
	}
	response.Body.Close()
	if at, err := http.ParseTime(response.Header.Get("Date")); err == nil {
		if skew := time.Since(at); skew > time.Minute || skew < -time.Minute {
			log.Fatalf("this machine's clock is %s off the server's; set the clock and run again", skew.Round(time.Second))
		}
	}

	// Before anything is put on the machine and before the code is spent: it
	// is also what the device before this machine fetches its uplink setup with.
	if err := uplinkCheck(server, hostRoot); err != nil {
		log.Fatal(err)
	}

	if !onMachine() {
		log.Print("copying the bundle to the host")
		if err := copyTree("/bundle", hostRoot+stage); err != nil {
			log.Fatalf("copy the bundle: %v", err)
		}
	}
	if err := os.WriteFile(hostRoot+stage+"/ca.crt", ca, 0644); err != nil {
		log.Fatal(err)
	}
	if err := syscall.Chroot(hostRoot); err != nil {
		log.Fatalf("enter the host filesystem (the container must be privileged, with --pid=host): %v", err)
	}
	if err := os.Chdir(stage); err != nil {
		log.Fatal(err)
	}

	key, err := exec.Command(stage+"/bin/station-gateway", "--public-key").Output()
	if err != nil {
		log.Fatalf("create the WireGuard key: %v", err)
	}
	request, _ := json.Marshal(map[string]string{"code": code, "public_key": strings.TrimSpace(string(key))})
	// Station API says what the machine waits for (the controller; the site's
	// registry cache being filled, which takes its time on a new site), and
	// the machine waits as long as it is told, up to an hour.
	var granted answer
	for began, heard, said := time.Now(), time.Now(), ""; ; time.Sleep(2 * time.Second) {
		response, err := client.Post(server+"/v1/bootstrap/redeem", "application/json", bytes.NewReader(request))
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			response.Body.Close()
			_ = json.Unmarshal(body, &granted)
			if granted.State == "ready" {
				break
			}
			if response.StatusCode != http.StatusAccepted {
				log.Fatalf("the code was refused (%d): %s", response.StatusCode, body)
			}
			if heard = time.Now(); granted.Reason != said {
				said = granted.Reason
				log.Printf("waiting: %s", said)
			}
		}
		if time.Since(heard) > 2*time.Minute {
			log.Fatalf("Station API has not answered for two minutes: %v", err)
		}
		if time.Since(began) > time.Hour {
			log.Fatalf("not admitted after an hour: %s", granted.Reason)
		}
	}
	log.Printf("admitted as %s in %s, tunnel address %s", granted.NodeName, granted.ClusterID, granted.TunnelAddress)

	var tunnel struct {
		Interface string `json:"interface"`
		Peers     []struct {
			AllowedIPs []string `json:"allowedIPs"`
		} `json:"peers"`
	}
	if err := json.Unmarshal(granted.Config, &tunnel); err != nil || tunnel.Interface == "" || len(tunnel.Peers) == 0 {
		log.Fatalf("the tunnel configuration names no interface or no peer: %v", err)
	}
	if err := os.MkdirAll("/etc/rlark/station", 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("/etc/rlark/station/bootstrap.json", granted.Config, 0644); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("/etc/rlark/station/ca.crt", ca, 0644); err != nil {
		log.Fatal(err)
	}
	// Where the tunnel service asks for the machine's document from now on,
	// with the machine's key and before any pod of it runs (station-gateway,
	// document.go). The keeper in the node's pod keeps the same two files.
	pointer, _ := json.Marshal(struct {
		URL string `json:"url"`
		CA  string `json:"ca"`
	}{server, "ca.crt"})
	if err := os.WriteFile("/etc/rlark/station/station-api.json", append(pointer, '\n'), 0644); err != nil {
		log.Fatal(err)
	}
	settings := append(os.Environ(),
		"PATH=/opt/rlark/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"NODE_NAME="+granted.NodeName, "TUNNEL_ADDRESS="+granted.TunnelAddress, "TUNNEL_INTERFACE="+tunnel.Interface,
		"JOIN_API_SERVER="+granted.Join.APIServer, "JOIN_TOKEN="+granted.Join.Token, "JOIN_CA_HASH="+granted.Join.CACertHash,
		"NODE_LABELS="+granted.Labels, "NODE_TAINTS="+granted.Taints, "NODE_VOLUMES="+granted.Volumes)
	if os.Getenv("RLARK_MIGRATE") != "" {
		asUnit("rlark-node-migrate", stage+"/install.sh", stage+"/install.log", settings, "RLARK_MIGRATE", "NODE_", "TUNNEL_", "JOIN_", "PATH")
		return
	}
	install := hostCommand(stage + "/install.sh")
	install.Stdout, install.Stderr = os.Stdout, os.Stderr
	install.Env = settings
	if err := install.Run(); err != nil {
		log.Fatalf("install: %v", err)
	}
}

// asUnit runs a script of the bundle as a unit of the host's systemd and
// shows its log: this program runs in a pod of the node, which the script
// ends (the node leaves its cluster, or its control plane stops). The unit
// goes on. It is given the settings whose names begin as listed, and no more,
// each name once (the last value stands): the systemd of Ubuntu 20.04 refuses
// a unit whose environment names a variable twice (PATH: the container's and
// ours) or holds a line break (RLARK_CA) with "Invalid environment block".
func asUnit(unit, command, logFile string, settings []string, begin ...string) {
	arguments := []string{"--unit", unit, "--collect", "--quiet"}
	chosen, order := map[string]string{}, []string{}
	for _, setting := range settings {
		name := strings.SplitN(setting, "=", 2)[0]
		if slices.ContainsFunc(begin, func(prefix string) bool { return strings.HasPrefix(name, prefix) }) {
			if _, seen := chosen[name]; !seen {
				order = append(order, name)
			}
			chosen[name] = setting
		}
	}
	for _, name := range order {
		arguments = append(arguments, "--setenv="+chosen[name])
	}
	_ = os.Remove(logFile)
	arguments = append(arguments, "/bin/sh", "-c", "exec /bin/sh "+command+" > "+logFile+" 2>&1")
	if output, err := exec.Command("systemd-run", arguments...).CombinedOutput(); err != nil {
		log.Fatalf("start %s (settings %s): %v: %s", unit, strings.Join(order, " "), err, output)
	}
	log.Printf("running as %s.service; its log is %s", unit, logFile)
	for shown := 0; ; time.Sleep(time.Second) {
		if data, err := os.ReadFile(logFile); err == nil && len(data) > shown {
			os.Stdout.Write(data[shown:])
			shown = len(data)
		}
		if state, _ := exec.Command("systemctl", "is-active", unit).Output(); strings.TrimSpace(string(state)) != "active" && strings.TrimSpace(string(state)) != "activating" {
			return
		}
	}
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(to, strings.TrimPrefix(path, from))
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()
		temporary := target + ".new"
		destination, err := os.OpenFile(temporary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(destination, source); err != nil {
			destination.Close()
			return err
		}
		if err := destination.Close(); err != nil {
			return err
		}
		if err := os.Rename(temporary, target); err != nil {
			return fmt.Errorf("replace %s: %w", target, err)
		}
		return nil
	})
}

// onHost is the other use of the image: a script of the bundle that needs
// nothing from Station API runs as a host process. upgrade.sh, from a Job
// pinned to a node, takes that node's Kubernetes one minor version up;
// control-plane.sh makes the machine a cluster's control plane; volumes.sh
// moves a node's data into its volumes.
func onHost(script string, arguments ...string) {
	if !onMachine() {
		if err := copyTree("/bundle", hostRoot+stage); err != nil {
			log.Fatalf("copy the bundle: %v", err)
		}
		// The scripts call this program again on the host (kubelet-overrides, cert-sans).
		self, err := os.ReadFile("/station-bootstrap")
		if err != nil {
			log.Fatal(err)
		}
		// In its place by a rename: the file may be running (the snapshot service
		// of a control plane that was handed over).
		if err := os.WriteFile(hostRoot+stage+"/station-bootstrap.new", self, 0755); err != nil {
			log.Fatal(err)
		}
		if err := os.Rename(hostRoot+stage+"/station-bootstrap.new", hostRoot+stage+"/station-bootstrap"); err != nil {
			log.Fatal(err)
		}
	}
	if err := syscall.Chroot(hostRoot); err != nil {
		log.Fatalf("enter the host filesystem (the container must be privileged, with --pid=host): %v", err)
	}
	if err := os.Chdir(stage); err != nil {
		log.Fatal(err)
	}
	settings := append(os.Environ(), "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	// From a Job on the node (RLARK_DETACH): the script ends the Job's own pod.
	if os.Getenv("RLARK_DETACH") != "" {
		asUnit("rlark-"+strings.TrimSuffix(script, ".sh"), strings.Join(append([]string{stage + "/" + script}, arguments...), " "), stage+"/"+strings.TrimSuffix(script, ".sh")+".log", settings,
			"CONTROL_PLANE_", "POD_", "SERVICE_", "FROM_", "SUCCESSOR", "ETCD_", "SYSTEM_RESERVED_", "NODE_NAME", "NODE_VOLUMES", "TUNNEL_INTERFACE", "PATH")
		return
	}
	run := hostCommand(stage+"/"+script, arguments...)
	run.Stdout, run.Stderr = os.Stdout, os.Stderr
	run.Env = settings
	if err := run.Run(); err != nil {
		log.Fatalf("%s: %v", strings.TrimSuffix(script, ".sh"), err)
	}
}

// kubeletOverrides prints, as a kubeadm patch of the KubeletConfiguration,
// the settings in which this node's kubelet configuration differs from the
// cluster's. kubeadm writes the node's file anew from the cluster's on every
// upgrade; the patch carries what was set on this node only (swap handling on
// a workstation, CPU reservation on a robot) across it.
func kubeletOverrides(nodeFile, clusterFile string) {
	read := func(path string) map[string]interface{} {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		var config map[string]interface{}
		if err := yaml.Unmarshal(data, &config); err != nil {
			log.Fatalf("%s: %v", path, err)
		}
		return config
	}
	node, cluster := read(nodeFile), read(clusterFile)
	overrides := map[string]interface{}{}
	for key, value := range node {
		if key != "apiVersion" && key != "kind" && !reflect.DeepEqual(value, cluster[key]) {
			overrides[key] = value
		}
	}
	if len(overrides) == 0 {
		return
	}
	overrides["apiVersion"], overrides["kind"] = "kubelet.config.k8s.io/v1beta1", "KubeletConfiguration"
	patch, err := yaml.Marshal(overrides)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(patch)
}

// certSANs serves a control-plane node whose address is no longer the one its
// cluster was made with. For the API server's certificate and etcd's two it
// prints, on stderr, a line "<file> lacks <names>" when the certificate does
// not name every entry of its list in the ClusterConfiguration or one of the
// addresses the node is reached at now; on stdout the ClusterConfiguration
// whose lists hold what they held, what the certificates name and those
// addresses, for kubeadm to issue the certificates from.
func certSANs(configFile, pki string, addresses []string) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatal(err)
	}
	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("%s: %v", configFile, err)
	}
	// An address is compared in one spelling, as a certificate holds it.
	spelling := func(name string) string {
		if ip := net.ParseIP(name); ip != nil {
			return ip.String()
		}
		return name
	}
	for _, list := range [][]string{{"apiserver.crt", "apiServer", "certSANs"}, {"etcd/server.crt", "etcd", "local", "serverCertSANs"}, {"etcd/peer.crt", "etcd", "local", "peerCertSANs"}} {
		encoded, err := os.ReadFile(filepath.Join(pki, list[0]))
		if err != nil {
			log.Fatal(err)
		}
		block, _ := pem.Decode(encoded)
		if block == nil {
			log.Fatalf("%s holds no certificate", list[0])
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			log.Fatalf("%s: %v", list[0], err)
		}
		at := config
		for _, key := range list[1 : len(list)-1] {
			next, _ := at[key].(map[string]interface{})
			if next == nil {
				next = map[string]interface{}{}
				at[key] = next
			}
			at = next
		}
		named, all, lacks := map[string]bool{}, []interface{}{}, []string{}
		add := func(name string) {
			if name = spelling(name); !named[name] {
				named[name] = true
				all = append(all, name)
			}
		}
		for _, name := range certificate.DNSNames {
			add(name)
		}
		for _, ip := range certificate.IPAddresses {
			add(ip.String())
		}
		wanted, _ := at[list[len(list)-1]].([]interface{})
		for _, address := range addresses {
			wanted = append(wanted, address)
		}
		for _, name := range wanted {
			if text := fmt.Sprint(name); !named[spelling(text)] {
				lacks = append(lacks, spelling(text))
				add(text)
			}
		}
		if len(lacks) > 0 {
			fmt.Fprintf(os.Stderr, "%s lacks %s\n", list[0], strings.Join(lacks, " "))
		}
		at[list[len(list)-1]] = all
	}
	out, err := yaml.Marshal(config)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(out)
}

// controllerManagerArg prints the ClusterConfiguration of configFile with the
// controller manager given the argument, in the form the configuration's
// version has (a list of name and value since v1beta4, a map before).
func controllerManagerArg(configFile, name, value string) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatal(err)
	}
	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("%s: %v", configFile, err)
	}
	manager, _ := config["controllerManager"].(map[string]interface{})
	if manager == nil {
		manager = map[string]interface{}{}
		config["controllerManager"] = manager
	}
	if config["apiVersion"] == "kubeadm.k8s.io/v1beta3" {
		arguments, _ := manager["extraArgs"].(map[string]interface{})
		if arguments == nil {
			arguments = map[string]interface{}{}
		}
		arguments[name] = value
		manager["extraArgs"] = arguments
	} else {
		arguments, _ := manager["extraArgs"].([]interface{})
		arguments = slices.DeleteFunc(arguments, func(argument interface{}) bool {
			held, _ := argument.(map[string]interface{})
			return held["name"] == name
		})
		manager["extraArgs"] = append(arguments, map[string]interface{}{"name": name, "value": value})
	}
	out, err := yaml.Marshal(config)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(out)
}

// certAddresses prints a line for every certificate under pki that names one
// of the addresses: a control plane that is to move with its machine holds
// none of the machine's own (control-plane.sh).
func certAddresses(pki string, addresses []string) {
	files, _ := filepath.Glob(filepath.Join(pki, "*.crt"))
	etcd, _ := filepath.Glob(filepath.Join(pki, "etcd", "*.crt"))
	for _, file := range append(files, etcd...) {
		encoded, err := os.ReadFile(file)
		if err != nil {
			log.Fatal(err)
		}
		block, _ := pem.Decode(encoded)
		if block == nil {
			log.Fatalf("%s holds no certificate", file)
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			log.Fatalf("%s: %v", file, err)
		}
		for _, ip := range certificate.IPAddresses {
			for _, address := range addresses {
				if ip.Equal(net.ParseIP(address)) {
					fmt.Printf("the certificate %s names %s\n", file, address)
				}
			}
		}
	}
}

// onMachine says whether this program was started on the machine itself,
// from the bundle as it was unpacked there (the installer's tar, sent over
// SSH), not as the entry point of the installer image: the bundle then lies
// where it belongs already.
func onMachine() bool {
	self, err := os.Executable()
	return err == nil && filepath.Dir(self) == stage
}

// hostCommand runs a script of the bundle as a process of the host. After the
// chroot this program sees the host's files but is still in its container's
// namespaces; kubeadm reads the mount table to find the cgroup hierarchy, so
// the script is started through the host's nsenter in the namespaces of PID 1.
func hostCommand(script string, arguments ...string) *exec.Cmd {
	for _, nsenter := range []string{"/usr/bin/nsenter", "/bin/nsenter", "/usr/sbin/nsenter", "/sbin/nsenter"} {
		if _, err := os.Stat(nsenter); err == nil {
			return exec.Command(nsenter, append([]string{"-t", "1", "-m", "-u", "-i", "-n", "-p", "--", "/bin/sh", script}, arguments...)...)
		}
	}
	log.Print("the host has no nsenter; running in the container's namespaces")
	return exec.Command("/bin/sh", append([]string{script}, arguments...)...)
}

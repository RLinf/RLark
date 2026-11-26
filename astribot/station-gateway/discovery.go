package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vishvananda/netlink"
	"golang.org/x/crypto/ssh"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// discovery is what a hub looks for on its site's own network, and what it
// lets the management plane reach there (astribot/station/network, Discovery).
type discovery struct {
	Ranges              []string `json:"ranges"`
	IntervalSeconds     int      `json:"intervalSeconds"`
	Ports               []int    `json:"ports"`
	HostKeys            []string `json:"hostKeys"`
	IncludeUnclassified bool     `json:"includeUnclassified,omitempty"`
	SSHPorts            []int    `json:"sshPorts,omitempty"`
	RelayToken          string   `json:"relayToken,omitempty"`
	RelayPort           int      `json:"relayPort"`
	RelayFrom           string   `json:"relayFrom"`
}

// discoveredHost is a host that showed the host key of a machine that has
// not joined. Nothing but its key was asked of it.
type discoveredHost struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	// MAC is the host's hardware address: what stays when its lease changes.
	MAC              string `json:"mac"`
	HostKey          string `json:"host_key"`
	HostKeyAlgorithm string `json:"host_key_algorithm,omitempty"`
	Banner           string `json:"ssh_banner,omitempty"`
	Unclassified     bool   `json:"unclassified,omitempty"`
}

// virtualLinks are the beginnings of the names of the interfaces whose
// networks are not the site's: its tunnel, and those of containers and of
// the cluster's own pod and service networks.
var virtualLinks = []string{"st-", "docker", "br-", "cali", "cni", "flannel", "vxlan", "tunl", "veth", "kube-ipvs"}

// attachedOf returns the private IPv4 networks the interfaces' addresses are
// on, interface by name: a robot that comes up on any network the hub is on
// itself is found without that network being declared. Robots are on private
// networks; a public one is strangers' and is not looked at. A network wider
// than /22 is looked at in the /22 around the hub's own address.
func attachedOf(interfaces map[string][]netip.Prefix) []netip.Prefix {
	var networks []netip.Prefix
	for name, assigned := range interfaces {
		if slices.ContainsFunc(virtualLinks, func(start string) bool { return strings.HasPrefix(name, start) }) {
			continue
		}
		for _, prefix := range assigned {
			if address := prefix.Addr(); address.Is4() && address.IsPrivate() {
				networks = append(networks, netip.PrefixFrom(address, max(prefix.Bits(), 22)).Masked())
			}
		}
	}
	slices.SortFunc(networks, func(a, b netip.Prefix) int { return a.Addr().Compare(b.Addr()) })
	return slices.Compact(networks)
}

// attached returns the IPv4 networks of the interfaces that are up.
func attached() []netip.Prefix {
	links, _ := net.Interfaces()
	interfaces := map[string][]netip.Prefix{}
	for _, link := range links {
		if link.Flags&net.FlagUp == 0 {
			continue
		}
		assigned, _ := link.Addrs()
		for _, entry := range assigned {
			if network, ok := entry.(*net.IPNet); ok {
				address, _ := netip.AddrFromSlice(network.IP)
				bits, _ := network.Mask.Size()
				interfaces[link.Name] = append(interfaces[link.Name], netip.PrefixFrom(address.Unmap(), bits))
			}
		}
	}
	return attachedOf(interfaces)
}

// networks are where the hub looks and what its relay reaches: the declared
// ranges and the networks it is attached to.
func (d *discovery) networks(attached []netip.Prefix) ([]netip.Prefix, error) {
	all := slices.Clone(attached)
	for _, text := range d.Ranges {
		prefix, err := netip.ParsePrefix(text)
		if err != nil || !prefix.Addr().Is4() || prefix.Bits() < 22 ||
			!(prefix.Addr().IsPrivate() || prefix.Addr().IsLoopback() || prefix.Addr().IsLinkLocalUnicast()) {
			return nil, fmt.Errorf("discovery range %q is not an IPv4 prefix of /22 or smaller", text)
		}
		all = append(all, prefix.Masked())
	}
	return all, nil
}

func (d *discovery) validRelayToken() bool {
	decoded, err := base64.StdEncoding.DecodeString(d.RelayToken)
	return err == nil && len(decoded) == 32 && base64.StdEncoding.EncodeToString(decoded) == d.RelayToken
}

// addresses are the host addresses of the networks, each once.
func addresses(networks []netip.Prefix) []netip.Addr {
	var all []netip.Addr
	for _, prefix := range networks {
		var hosts []netip.Addr
		for address := prefix.Addr(); prefix.Contains(address); address = address.Next() {
			hosts = append(hosts, address)
		}
		if prefix.Bits() < 31 { // the network's own and its broadcast address
			hosts = hosts[1 : len(hosts)-1]
		}
		all = append(all, hosts...)
	}
	slices.SortFunc(all, netip.Addr.Compare)
	return slices.Compact(all)
}

// errKeyRead ends an SSH handshake as soon as the host has shown its key:
// before the client would name an account or send anything to prove one.
var errKeyRead = errors.New("the host key is read")

// hostKey returns the fingerprint of the ED25519 host key the host at the
// address shows, empty if none answers there in time. The host is not asked
// to authenticate anybody.
type sshObservation struct {
	Banner      string
	Algorithm   string
	Fingerprint string
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

func hostKey(ctx context.Context, address string, algorithms []string) sshObservation {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return sshObservation{}
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	reader := bufio.NewReaderSize(conn, 512)
	banner, err := reader.ReadSlice('\n')
	if err != nil || len(banner) > 255 || !strings.HasPrefix(string(banner), "SSH-") {
		return sshObservation{}
	}
	observed := sshObservation{Banner: strings.TrimSpace(string(banner))}
	if len(observed.Banner) > 200 {
		observed.Banner = observed.Banner[:200]
	}
	replay := bufio.NewReader(io.MultiReader(strings.NewReader(string(banner)), reader))
	_, _, _, _ = ssh.NewClientConn(bufferedConn{Conn: conn, reader: replay}, address, &ssh.ClientConfig{HostKeyAlgorithms: algorithms,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			observed.Algorithm = key.Type()
			observed.Fingerprint = ssh.FingerprintSHA256(key)
			return errKeyRead
		}})
	return observed
}

// scan looks at every address of the ranges and of the attached networks
// and returns the hosts that show one of the host keys looked for. An
// address where nothing answers takes a slot for the dial's second: 128
// slots look at three empty /22 in at most about 25 s, and keep the
// kernel's unresolved neighbours under its table's default 1024.
func (d *discovery) scan(ctx context.Context, attached []netip.Prefix, hardware func(string) string) ([]discoveredHost, error) {
	networks, err := d.networks(attached)
	if err != nil {
		return nil, err
	}
	found := []discoveredHost{}
	var mu sync.Mutex
	var group sync.WaitGroup
	slots := make(chan struct{}, 128)
	ports := slices.Clone(d.Ports)
	if d.IncludeUnclassified {
		sshPorts := d.SSHPorts
		if len(sshPorts) == 0 {
			sshPorts = []int{22}
		}
		for _, port := range sshPorts {
			if !slices.Contains(ports, port) {
				ports = append(ports, port)
			}
		}
	}
	algorithms := []string{ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	for _, address := range addresses(networks) {
		for _, port := range ports {
			group.Add(1)
			slots <- struct{}{}
			go func() {
				defer func() { <-slots; group.Done() }()
				observed := hostKey(ctx, net.JoinHostPort(address.String(), strconv.Itoa(port)), algorithms)
				known := slices.Contains(d.HostKeys, observed.Fingerprint)
				if observed.Fingerprint == "" || !known && !d.IncludeUnclassified {
					return
				}
				// Without a hardware address the host is not on this network itself.
				if mac := hardware(address.String()); mac != "" {
					mu.Lock()
					found = append(found, discoveredHost{Address: address.String(), Port: port, MAC: mac, HostKey: observed.Fingerprint,
						HostKeyAlgorithm: observed.Algorithm, Banner: observed.Banner, Unclassified: !known})
					mu.Unlock()
				}
			}()
		}
	}
	group.Wait()
	slices.SortFunc(found, func(a, b discoveredHost) int {
		if a.Unclassified != b.Unclassified {
			if a.Unclassified {
				return 1
			}
			return -1
		}
		if order := strings.Compare(a.Address, b.Address); order != 0 {
			return order
		}
		return a.Port - b.Port
	})
	return found, nil
}

// neighbour is the hardware address the kernel has learned for an address.
func neighbour(address string) string {
	neighbours, err := netlink.NeighList(0, netlink.FAMILY_V4)
	if err != nil {
		return ""
	}
	for _, entry := range neighbours {
		if entry.IP.String() == address && len(entry.HardwareAddr) == 6 {
			return entry.HardwareAddr.String()
		}
	}
	return ""
}

// relay connects a peer of the tunnel to a host of the discovery ranges or
// the attached networks: the management plane logs in to a machine that was
// found from where the accounts are kept, and no account ever comes to this
// node. The peer names the host in one line, "address:port"; anything
// outside those networks and the ports of the document is refused, and so
// is every peer but the one node the document says the management plane
// dials from.
func (d *discovery) relay(conn net.Conn, attached []netip.Prefix) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReaderSize(conn, 256)
	line, err := reader.ReadSlice('\n')
	if err != nil || len(line) > 256 {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	fields := strings.Fields(string(line))
	if len(fields) == 3 && fields[0] == "gateway" {
		if !d.validRelayToken() || subtle.ConstantTimeCompare([]byte(fields[1]), []byte(d.RelayToken)) != 1 {
			return
		}
		target, err := d.relayTarget(fields[2], attached, true)
		if err != nil {
			return
		}
		d.pipeRelay(conn, reader, target, true)
		return
	}
	// The legacy protocol is still restricted to the configured controller
	// tunnel address. Its wire format and response strings stay unchanged.
	if remote, ok := conn.RemoteAddr().(*net.TCPAddr); !ok || remote.IP.String() != d.RelayFrom {
		return
	}
	if len(fields) != 1 {
		return
	}
	target, err := d.relayTarget(fields[0], attached, false)
	if err != nil {
		_, _ = io.WriteString(conn, "refused\n")
		return
	}
	d.pipeRelay(conn, reader, target, false)
}

func (d *discovery) relayTarget(raw string, attached []netip.Prefix, authenticated bool) (string, error) {
	target, err := netip.ParseAddrPort(raw)
	if err != nil || !target.Addr().Is4() || !slices.Contains(d.Ports, int(target.Port())) {
		if authenticated {
			ports := d.SSHPorts
			if len(ports) == 0 {
				ports = []int{22}
			}
			if err != nil || !target.Addr().Is4() || !slices.Contains(ports, int(target.Port())) {
				return "", errors.New("target is outside the discovery ports")
			}
		} else {
			return "", errors.New("target is outside the discovery ports")
		}
	}
	networks, err := d.networks(attached)
	if err != nil || !slices.ContainsFunc(networks, func(prefix netip.Prefix) bool { return prefix.Contains(target.Addr()) }) {
		return "", errors.New("target is outside the discovery networks")
	}
	return target.String(), nil
}

func (d *discovery) pipeRelay(conn net.Conn, reader *bufio.Reader, target string, silent bool) {
	far, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		if !silent {
			_, _ = io.WriteString(conn, "unreachable\n")
		}
		return
	}
	defer far.Close()
	if _, err := io.WriteString(conn, "ok\n"); err != nil {
		return
	}
	go func() { _, _ = io.Copy(far, reader); _ = far.Close() }()
	_, _ = io.Copy(conn, far)
}

// discover runs for as long as the node does: while the node's document
// declares discovery it keeps the relay on the tunnel address, looks at the
// ranges and the attached networks every interval and reports what it found
// to Station API. A round starts an interval after the last one started, or
// when it ends if it took longer, so a hub reports at least every interval
// its scan fits in (station-controller calls it silent after three).
func discover(read func() *desired, directory string, key wgtypes.Key) {
	var serving *discovery
	var listener net.Listener
	said := ""
	for {
		started := time.Now()
		wait := 30 * time.Second
		d := read()
		var declared *discovery
		at := ""
		if d != nil && d.Discovery != nil {
			declared = d.Discovery
			at = net.JoinHostPort(d.address.IP.String(), strconv.Itoa(declared.RelayPort))
			wait = time.Duration(declared.IntervalSeconds) * time.Second
		}
		if listener != nil && (declared == nil || listener.Addr().String() != at || !reflect.DeepEqual(serving, declared)) {
			_ = listener.Close()
			listener = nil
		}
		state := ""
		if declared != nil {
			if listener == nil {
				opened, err := net.Listen("tcp", at)
				if err != nil {
					state = fmt.Sprintf("the relay cannot listen at %s: %v", at, err)
				} else {
					listener, serving = opened, declared
					go func() {
						for {
							conn, err := opened.Accept()
							if err != nil {
								return
							}
							go declared.relay(conn, attached())
						}
					}()
				}
			}
			found, err := declared.scan(context.Background(), attached(), neighbour)
			if err == nil {
				var status int
				var url string
				var ca []byte
				// A hub whose tunnel a pod runs has no station-api.json: its pod says where Station API is.
				if url, ca, err = stationAPIOf(directory); errors.Is(err, errNoStationAPI) && os.Getenv("STATION_API_URL") != "" {
					url = os.Getenv("STATION_API_URL")
					ca, err = os.ReadFile(os.Getenv("STATION_API_CA_FILE"))
				}
				if err == nil {
					// Deliver an empty heartbeat as well as non-empty batches.
					for start := 0; ; start += 256 {
						end := min(start+256, len(found))
						status, _, err = askStationAPI(url, ca, key, "/v1/network/discovered", map[string]any{"hosts": found[start:end]})
						if err != nil || status != http.StatusOK || end == len(found) {
							break
						}
					}
				}
				if err == nil && status != http.StatusOK {
					err = fmt.Errorf("Station API answered %d to what was found", status)
				}
			}
			if err != nil {
				state = err.Error()
			} else if state == "" {
				state = fmt.Sprintf("observed %d SSH endpoints", len(found))
			}
		}
		if state != said {
			said = state
			if state == "" {
				state = "not declared for this node"
			}
			log.Printf("discovery: %s", state)
		}
		time.Sleep(wait - time.Since(started))
	}
}

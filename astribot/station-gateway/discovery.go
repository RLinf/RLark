package main

import (
	"bufio"
	"context"
	"encoding/json"
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
	Ranges          []string `json:"ranges"`
	IntervalSeconds int      `json:"intervalSeconds"`
	Ports           []int    `json:"ports"`
	HostKeys        []string `json:"hostKeys"`
	RelayPort       int      `json:"relayPort"`
	RelayFrom       string   `json:"relayFrom"`
}

// discoveredHost is a host that showed the host key of a machine that has
// not joined. Nothing but its key was asked of it.
type discoveredHost struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	// MAC is the host's hardware address: what stays when its lease changes.
	MAC     string `json:"mac"`
	HostKey string `json:"host_key"`
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
		if err != nil || !prefix.Addr().Is4() || prefix.Bits() < 22 {
			return nil, fmt.Errorf("discovery range %q is not an IPv4 prefix of /22 or smaller", text)
		}
		all = append(all, prefix.Masked())
	}
	return all, nil
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
func hostKey(ctx context.Context, address string) string {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return ""
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	shown := ""
	_, _, _, _ = ssh.NewClientConn(conn, address, &ssh.ClientConfig{HostKeyAlgorithms: []string{ssh.KeyAlgoED25519},
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			shown = ssh.FingerprintSHA256(key)
			return errKeyRead
		}})
	return shown
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
	for _, address := range addresses(networks) {
		for _, port := range d.Ports {
			group.Add(1)
			slots <- struct{}{}
			go func() {
				defer func() { <-slots; group.Done() }()
				key := hostKey(ctx, net.JoinHostPort(address.String(), strconv.Itoa(port)))
				if !slices.Contains(d.HostKeys, key) {
					return
				}
				// Without a hardware address the host is not on this network itself.
				if mac := hardware(address.String()); mac != "" {
					mu.Lock()
					found = append(found, discoveredHost{Address: address.String(), Port: port, MAC: mac, HostKey: key})
					mu.Unlock()
				}
			}()
		}
	}
	group.Wait()
	slices.SortFunc(found, func(a, b discoveredHost) int { return strings.Compare(a.Address, b.Address) })
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
	// Only the node the management plane dials from: any other peer of the
	// tunnel is closed before a line of it is read.
	if remote, ok := conn.RemoteAddr().(*net.TCPAddr); !ok || remote.IP.String() != d.RelayFrom {
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := bufio.NewReaderSize(conn, 64).ReadString('\n')
	if err != nil {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	target, err := netip.ParseAddrPort(strings.TrimSpace(line))
	networks, _ := d.networks(attached) // none for a document with a bad range, which scan says
	if err != nil || !slices.Contains(d.Ports, int(target.Port())) || !slices.ContainsFunc(networks, func(prefix netip.Prefix) bool { return prefix.Contains(target.Addr()) }) {
		_, _ = io.WriteString(conn, "refused\n")
		return
	}
	far, err := net.DialTimeout("tcp", target.String(), 5*time.Second)
	if err != nil {
		_, _ = io.WriteString(conn, "unreachable\n")
		return
	}
	defer far.Close()
	if _, err := io.WriteString(conn, "ok\n"); err != nil {
		return
	}
	go func() { _, _ = io.Copy(far, conn); _ = far.Close() }()
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
					status, _, err = askStationAPI(url, ca, key, "/v1/network/discovered", map[string]any{"hosts": found})
				}
				if err == nil && status != http.StatusOK {
					err = fmt.Errorf("Station API answered %d to what was found", status)
				}
			}
			if err != nil {
				state = err.Error()
			} else if state == "" {
				names, _ := json.Marshal(found)
				state = "machines that have not joined: " + string(names)
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

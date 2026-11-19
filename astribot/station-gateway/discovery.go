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

func (d *discovery) addresses() ([]netip.Addr, error) {
	var all []netip.Addr
	for _, text := range d.Ranges {
		prefix, err := netip.ParsePrefix(text)
		if err != nil || !prefix.Addr().Is4() || prefix.Bits() < 22 {
			return nil, fmt.Errorf("discovery range %q is not an IPv4 prefix of /22 or smaller", text)
		}
		prefix = prefix.Masked()
		for address := prefix.Addr(); prefix.Contains(address); address = address.Next() {
			all = append(all, address)
		}
		if prefix.Bits() < 31 { // the network's own and its broadcast address
			all = slices.DeleteFunc(all, func(address netip.Addr) bool { return address == prefix.Addr() })
			all = all[:len(all)-1]
		}
	}
	return all, nil
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

// scan looks at every address of the ranges and returns the hosts that show
// one of the host keys looked for.
func (d *discovery) scan(ctx context.Context, hardware func(string) string) ([]discoveredHost, error) {
	addresses, err := d.addresses()
	if err != nil {
		return nil, err
	}
	found := []discoveredHost{}
	var mu sync.Mutex
	var group sync.WaitGroup
	slots := make(chan struct{}, 64)
	for _, address := range addresses {
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

// relay connects a peer of the tunnel to a host of the discovery ranges: the
// management plane logs in to a machine that was found from where the
// accounts are kept, and no account ever comes to this node. The peer names
// the host in one line, "address:port"; anything outside the ranges and
// ports of the document is refused, and so is every peer but the one node
// the document says the management plane dials from.
func (d *discovery) relay(conn net.Conn) {
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
	if err != nil || !slices.Contains(d.Ports, int(target.Port())) || !slices.ContainsFunc(d.Ranges, func(text string) bool {
		prefix, err := netip.ParsePrefix(text)
		return err == nil && prefix.Contains(target.Addr())
	}) {
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
// ranges every interval and reports what it found to Station API.
func discover(read func() *desired, directory string, key wgtypes.Key) {
	var serving *discovery
	var listener net.Listener
	said := ""
	for {
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
							go declared.relay(conn)
						}
					}()
				}
			}
			found, err := declared.scan(context.Background(), neighbour)
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
		time.Sleep(wait)
	}
}

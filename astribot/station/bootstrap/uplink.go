package main

import (
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"slices"
	"strings"
	"syscall"
)

// uplinkCheck refuses a machine that dials Station API from an address it
// does not have in the network of the gateway it dials through (a robot's
// computer that holds an address of the venue's network behind the robot's
// router): nothing has given it that address, and whatever is given it later
// takes the machine's answers. The machine is to dial from its own address,
// and the device before it to let it out (the site's leafUplink).
func uplinkCheck(server, root string) error {
	target, err := url.Parse(server)
	if err != nil {
		return err
	}
	// What the kernel chooses for a packet to the server; nothing is sent.
	conn, err := net.Dial("udp4", net.JoinHostPort(target.Hostname(), "9"))
	if err != nil {
		return fmt.Errorf("no route to Station API at %s: %w", target.Hostname(), err)
	}
	source, remote := conn.LocalAddr().(*net.UDPAddr).IP, conn.RemoteAddr().(*net.UDPAddr).IP
	conn.Close()
	// The machine's own ip, in the machine's files (root): the image has none.
	lookup := exec.Command("/bin/sh", "-c", "PATH=/usr/sbin:/sbin:/usr/bin:/bin ip -4 -o route get "+remote.String())
	lookup.SysProcAttr = &syscall.SysProcAttr{Chroot: root}
	route, err := lookup.Output()
	if err != nil {
		return fmt.Errorf("ip route get %s: %w", remote, err)
	}
	fields := strings.Fields(string(route))
	after := func(word string) string {
		if i := slices.Index(fields, word); i >= 0 && i+1 < len(fields) {
			return fields[i+1]
		}
		return ""
	}
	gateway := net.ParseIP(after("via"))
	if gateway == nil {
		return nil // the server is in a network this machine is in itself
	}
	device, err := net.InterfaceByName(after("dev"))
	if err != nil {
		return fmt.Errorf("the interface of the route to %s: %w", remote, err)
	}
	addresses, err := device.Addrs()
	if err != nil {
		return err
	}
	own := "no address"
	for _, address := range addresses {
		if network, ok := address.(*net.IPNet); ok && network.Contains(gateway) {
			if network.IP.Equal(source) {
				return nil
			}
			own = network.IP.String()
		}
	}
	return fmt.Errorf("this machine dials out from %s, which is not an address in the network of its gateway %s (there it has %s): nothing has given it that address, and whatever is given it later takes this machine's answers. Remove the address and the routes that name it as their source, so that the machine dials from its own address; the device at %s lets it out once the first line of the add-node command has run there",
		source, gateway, own, gateway)
}

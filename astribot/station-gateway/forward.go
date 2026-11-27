package main

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"slices"
	"strings"

	"github.com/vishvananda/netlink"
)

// forward is a network beyond this node that it carries the tunnel's traffic
// on to (SiteNetwork, advertise with forward): what comes in from the tunnel
// from one of Sources, to Destination, leaves from the node's own address on
// Destination's network, and its answers go back.
type forward struct {
	Destination string   `json:"destination"`
	Sources     []string `json:"sources"`
}

// The chains this program owns, one in each table, each entered from the top
// of its hook; nothing else of the tables is touched.
const (
	forwardChain = "RLARK-STATION-FORWARD"
	snatChain    = "RLARK-STATION-SNAT"
)

// retiredForwarder tags the rules station-forwarder left in FORWARD and
// POSTROUTING: it never removed its rules. A gateway that forwards itself
// removes them; once every site has had such a gateway this goes.
const retiredForwarder = "--comment rlark-station-forwarder-v1"

// egress is where the node sends what it forwards to destination: the
// interface and source address of its own route there.
type egress struct {
	link   string
	source netip.Addr
}

// egressOf asks the kernel for the node's route to destination. A route back
// into the tunnel, or one with no source address, is no way on.
func egressOf(destination netip.Prefix, tunnel string) (egress, error) {
	routes, err := netlink.RouteGet(net.IP(destination.Addr().AsSlice()))
	if err != nil || len(routes) == 0 {
		return egress{}, fmt.Errorf("no route to forwarded network %s: %v", destination, err)
	}
	link, err := netlink.LinkByIndex(routes[0].LinkIndex)
	if err != nil {
		return egress{}, fmt.Errorf("interface of the route to %s: %w", destination, err)
	}
	source, ok := netip.AddrFromSlice(routes[0].Src.To4())
	if link.Attrs().Name == tunnel || !ok {
		return egress{}, fmt.Errorf("forwarded network %s is routed through %s from %v, not through a network of this node", destination, link.Attrs().Name, routes[0].Src)
	}
	return egress{link: link.Attrs().Name, source: source}, nil
}

// forwardRules are the two chains as `iptables -S` lists them.
func forwardRules(forwards []forward, ways []egress, tunnel string) (filter, nat []string) {
	for i, f := range forwards {
		way := ways[i]
		destination := netip.MustParsePrefix(f.Destination).Masked().String()
		for _, source := range f.Sources {
			filter = append(filter, fmt.Sprintf("-A %s -s %s/32 -d %s -i %s -o %s -j ACCEPT", forwardChain, source, destination, tunnel, way.link))
			nat = append(nat, fmt.Sprintf("-A %s -s %s/32 -d %s -o %s -j SNAT --to-source %s", snatChain, source, destination, way.link, way.source))
		}
		filter = append(filter, fmt.Sprintf("-A %s -s %s -i %s -o %s -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT", forwardChain, destination, way.link, tunnel))
	}
	return filter, nat
}

// tableScript is what iptables-restore --noflush is given to make one table's
// chain the stated rules, entered once from the top of hook; empty when the
// table, as listed, already is. With no rules the chain and its entry go.
// Rules of the retired forwarder go as the chain comes.
func tableScript(table, hook, chain string, rules, listed []string) string {
	var has, retired []string
	exists, entries := false, 0
	for _, line := range listed {
		switch {
		case line == "-N "+chain:
			exists = true
		case strings.HasPrefix(line, "-A "+chain+" "):
			has = append(has, line)
		case line == "-A "+hook+" -j "+chain:
			entries++
		case strings.HasPrefix(line, "-A "+hook+" ") && strings.Contains(line, retiredForwarder) && len(rules) > 0:
			retired = append(retired, "-D"+strings.TrimPrefix(line, "-A"))
		}
	}
	var script []string
	switch {
	case len(rules) == 0 && (exists || entries > 0):
		for range entries {
			script = append(script, "-D "+hook+" -j "+chain)
		}
		if exists {
			script = append(script, "-F "+chain, "-X "+chain)
		}
	case len(rules) > 0 && (!exists || !slices.Equal(has, rules) || entries != 1 || len(retired) > 0):
		// Declaring the chain empties it; the rules follow.
		script = append(append(script, ":"+chain+" - [0:0]"), rules...)
		for ; entries > 1; entries-- {
			script = append(script, "-D "+hook+" -j "+chain)
		}
		if entries == 0 {
			script = append(script, "-I "+hook+" 1 -j "+chain)
		}
		script = append(script, retired...)
	default:
		return ""
	}
	return "*" + table + "\n" + strings.Join(script, "\n") + "\nCOMMIT\n"
}

// iptables runs iptables, or iptables-restore with stdin, and returns what it
// printed; what it says on stderr (a backend's warnings) is only for its errors.
func iptables(stdin string, args ...string) (string, error) {
	command := exec.Command("iptables", args...)
	if stdin != "" {
		command = exec.Command("iptables-restore", args...)
		command.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", command.Path, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// applyForwards keeps the node's two chains what forwards states: rules for
// these sources only, the packets leaving from the node's address on the way
// to each network. A node that forwards nothing has neither chain.
func applyForwards(forwards []forward, tunnel string) error {
	ways := make([]egress, len(forwards))
	for i, f := range forwards {
		var err error
		if ways[i], err = egressOf(netip.MustParsePrefix(f.Destination), tunnel); err != nil {
			return err
		}
	}
	filter, nat := forwardRules(forwards, ways, tunnel)
	var errs []error
	for _, t := range []struct {
		table, hook, chain string
		rules              []string
	}{
		{"filter", "FORWARD", forwardChain, filter},
		{"nat", "POSTROUTING", snatChain, nat},
	} {
		listed, err := iptables("", "-w", "5", "-t", t.table, "-S")
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if script := tableScript(t.table, t.hook, t.chain, t.rules, strings.Split(strings.TrimSpace(listed), "\n")); script != "" {
			if _, err := iptables(script, "-w", "5", "--noflush"); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

package main

import (
	"net/netip"
	"slices"
	"strings"
	"testing"
)

var nas = []forward{{Destination: "10.11.5.12/32", Sources: []string{"10.254.240.2", "10.254.240.4"}}}

func TestForwardRulesAreWhatIptablesLists(t *testing.T) {
	filter, nat := forwardRules(nas, []egress{{link: "enp5s0", source: netip.MustParseAddr("10.11.14.128")}}, "st-wg0")
	wantFilter := []string{
		"-A RLARK-STATION-FORWARD -s 10.254.240.2/32 -d 10.11.5.12/32 -i st-wg0 -o enp5s0 -j ACCEPT",
		"-A RLARK-STATION-FORWARD -s 10.254.240.4/32 -d 10.11.5.12/32 -i st-wg0 -o enp5s0 -j ACCEPT",
		"-A RLARK-STATION-FORWARD -s 10.11.5.12/32 -i enp5s0 -o st-wg0 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT",
	}
	wantNAT := []string{
		"-A RLARK-STATION-SNAT -s 10.254.240.2/32 -d 10.11.5.12/32 -o enp5s0 -j SNAT --to-source 10.11.14.128",
		"-A RLARK-STATION-SNAT -s 10.254.240.4/32 -d 10.11.5.12/32 -o enp5s0 -j SNAT --to-source 10.11.14.128",
	}
	if !slices.Equal(filter, wantFilter) || !slices.Equal(nat, wantNAT) {
		t.Fatalf("filter\n%s\nnat\n%s", strings.Join(filter, "\n"), strings.Join(nat, "\n"))
	}
}

func TestTableScript(t *testing.T) {
	rules := []string{"-A RLARK-STATION-FORWARD -s 10.254.240.2/32 -d 10.11.5.12/32 -i st-wg0 -o enp5s0 -j ACCEPT"}
	kept := []string{"-P FORWARD DROP", "-N RLARK-STATION-FORWARD", "-A FORWARD -j cali-FORWARD", "-A FORWARD -j RLARK-STATION-FORWARD", rules[0]}
	retired := "-A FORWARD -s 10.254.240.4/32 -d 10.11.5.12/32 -i st-wg0 -o enp5s0 -p tcp -m tcp --dport 445 -m comment --comment rlark-station-forwarder-v1 -j ACCEPT"
	for name, c := range map[string]struct {
		rules, listed []string
		want          string
	}{
		"as stated":                          {rules, kept, ""},
		"nothing, none":                      {nil, []string{"-P FORWARD ACCEPT", "-A FORWARD -j cali-FORWARD"}, ""},
		"first time":                         {rules, []string{"-P FORWARD DROP"}, ":RLARK-STATION-FORWARD - [0:0]\n" + rules[0] + "\n-I FORWARD 1 -j RLARK-STATION-FORWARD"},
		"a source less":                      {rules, append(slices.Clone(kept), "-A RLARK-STATION-FORWARD -s 10.254.240.9/32 -d 10.11.5.12/32 -i st-wg0 -o enp5s0 -j ACCEPT"), ":RLARK-STATION-FORWARD - [0:0]\n" + rules[0]},
		"entered twice":                      {rules, append(slices.Clone(kept), "-A FORWARD -j RLARK-STATION-FORWARD"), ":RLARK-STATION-FORWARD - [0:0]\n" + rules[0] + "\n-D FORWARD -j RLARK-STATION-FORWARD"},
		"no longer":                          {nil, kept, "-D FORWARD -j RLARK-STATION-FORWARD\n-F RLARK-STATION-FORWARD\n-X RLARK-STATION-FORWARD"},
		"the forwarder's":                    {rules, append(slices.Clone(kept), retired), ":RLARK-STATION-FORWARD - [0:0]\n" + rules[0] + "\n-D" + strings.TrimPrefix(retired, "-A")},
		"the forwarder's, nothing forwarded": {nil, []string{retired}, ""},
	} {
		script := tableScript("filter", "FORWARD", forwardChain, c.rules, c.listed)
		if want := "*filter\n" + c.want + "\nCOMMIT\n"; c.want == "" && script != "" || c.want != "" && script != want {
			t.Errorf("%s: %q", name, script)
		}
	}
}

func TestParseForwards(t *testing.T) {
	const base = `{"interface":"st-wg0","address":"10.254.240.1/32","listenPort":51820,"mtu":1380,"peers":[],"routes":[],"forwards":`
	if d, err := parse([]byte(base + `[{"destination":"10.11.5.12/32","sources":["10.254.240.2"]}]}`)); err != nil || len(d.Forwards) != 1 {
		t.Fatalf("%+v %v", d, err)
	}
	for _, refused := range []string{
		`[{"destination":"0.0.0.0/0","sources":["10.254.240.2"]}]`,
		`[{"destination":"10.11.5.12","sources":["10.254.240.2"]}]`,
		`[{"destination":"10.11.5.12/32","sources":[]}]`,
		`[{"destination":"10.11.5.12/32","sources":["10.254.240.2/32"]}]`,
		`[{"destination":"10.11.5.12/32","sources":["10.254.240.2"],"port":445}]`,
	} {
		if _, err := parse([]byte(base + refused + `}`)); err == nil {
			t.Errorf("accepted %s", refused)
		}
	}
}

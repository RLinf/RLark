package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A machine that joins under another tunnel address than the one its
// interface holds from an earlier join (staging 11-17: a node removed, then
// joined under another name, went on over the tunnel of before while the site
// network routed it at the new address) has that interface removed before
// the tunnel service starts; one that holds this join's address, or none,
// is left as it is. The join then checks the interface holds its address.
func TestTheTunnelInterfaceOfAnEarlierJoinIsRemoved(t *testing.T) {
	for name, test := range map[string]struct{ held, joins, removed, check string }{
		"another address":  {"198.18.99.7/32", "198.18.99.8", "st-wg0", "does not hold"},
		"the same address": {"198.18.99.8/32", "198.18.99.8", "", ""},
		"no interface":     {"", "198.18.99.8", "", "does not hold"},
		"a longer address": {"198.18.99.80/32", "198.18.99.8", "st-wg0", "does not hold"},
	} {
		t.Run(name, func(t *testing.T) {
			bin := t.TempDir()
			calls := filepath.Join(bin, "calls")
			stub := func(command, body string) {
				if err := os.WriteFile(filepath.Join(bin, command), []byte("#!/bin/sh\n"+body), 0755); err != nil {
					t.Fatal(err)
				}
			}
			stub("ip", `case "$*" in
"-4 -o addr show dev st-wg0") [ -z "$HELD" ] || echo "4: st-wg0    inet $HELD scope global st-wg0" ;;
"link del st-wg0") echo "st-wg0" >> `+calls+` ;;
esac
`)
			stub("systemctl", "exit 0\n")
			run := func(script string) (string, error) {
				command := exec.Command("sh", "-c", "started=$(date +%s); . ./lib.sh; TUNNEL_INTERFACE=st-wg0; "+script)
				command.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "HELD="+test.held)
				output, err := command.CombinedOutput()
				return string(output), err
			}
			if output, err := run("tunnel_of_before " + test.joins); err != nil {
				t.Fatalf("tunnel_of_before: %v %s", err, output)
			}
			removed, _ := os.ReadFile(calls)
			if strings.TrimSpace(string(removed)) != test.removed {
				t.Fatalf("removed %q, want %q", removed, test.removed)
			}
			output, err := run("tunnel_address_is " + test.joins)
			if test.check == "" && err != nil || test.check != "" && (err == nil || !strings.Contains(output, test.check)) {
				t.Fatalf("tunnel_address_is: %v %s", err, output)
			}
		})
	}
}

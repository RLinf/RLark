package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A node registers with the address it reaches its cluster's API from: the
// office's LAN address when the API is on the office's LAN, its tunnel
// address when the API is the name of a control plane of the tunnel.
func TestANodeIsKnownWhereItReachesItsClusterAPI(t *testing.T) {
	bin := t.TempDir()
	stub := func(name, body string) {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// The machine: 10.11.107.154 on the office LAN, 10.254.240.9 on the tunnel;
	// the site's names put the robots' control plane on the tunnel.
	stub("getent", `case $2 in
cp.rlark-robot-s1u-01.rlark.internal) echo "10.254.240.5    STREAM $2" ;;
nowhere.rlark.internal) exit 2 ;;
*) echo "$2    STREAM" ;;
esac
`)
	stub("ip", `case $4 in
10.254.240.*) echo "$4 dev st-wg0 src 10.254.240.9 uid 0" ;;
*) echo "$4 via 10.11.107.1 dev enp3s0 src 10.11.107.154 uid 0" ;;
esac
`)
	for host, want := range map[string]string{
		"10.11.14.128":                         "10.11.107.154",
		"cp.rlark-robot-s1u-01.rlark.internal": "10.254.240.9",
		"nowhere.rlark.internal":               "",
	} {
		command := exec.Command("sh", "-c", `. ./lib.sh; node_address "$0"`, host)
		command.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
		output, err := command.CombinedOutput()
		if got := strings.TrimSpace(string(output)); want != "" && (err != nil || got != want) || want == "" && (err == nil || !strings.Contains(got, "cannot resolve nowhere.rlark.internal")) {
			t.Fatalf("%s: %q %v, want %q", host, got, err, want)
		}
	}
}

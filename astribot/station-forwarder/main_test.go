package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func validConfig() config {
	return config{Input: "wg-test", Output: "eth-test", Source: "10.254.240.4", Destination: "10.11.5.12", SNAT: "10.11.14.128", Port: 445}
}
func TestValidateRejectsBroadOrMalformedRules(t *testing.T) {
	valid := `{"input":"wg-test","output":"eth-test","source":"10.254.240.4","destination":"10.11.5.12","snat":"10.11.14.128","port":445}`
	if _, err := parse([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{`{"input":"x","output":"y","source":"0.0.0.0/0","destination":"10.11.5.12","snat":"10.11.14.128","port":445}`, `{"input":"x","output":"x","source":"10.0.0.1","destination":"10.11.5.12","snat":"10.11.14.128","port":445}`, `{"input":"x","output":"y","source":"10.0.0.1","destination":"10.11.5.12","snat":"10.11.14.128","port":0}`} {
		if _, err := parse([]byte(v)); err == nil {
			t.Fatalf("accepted %s", v)
		}
	}
}
func TestPinnedConfigurationSurvivesRestartAndRejectsChanges(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state", "config.sha256")
	c := validConfig()
	if err := verifyPinned(p, configDigest(c)); !os.IsNotExist(err) {
		t.Fatalf("missing state check: %v", err)
	}
	if err := pinConfig(p, configDigest(c)); err != nil {
		t.Fatal(err)
	}
	if err := verifyPinned(p, configDigest(c)); err != nil {
		t.Fatal(err)
	}
	if err := pinConfig(p, configDigest(c)); err != nil {
		t.Fatal(err)
	}
	c.Port = 446
	if err := pinConfig(p, configDigest(c)); err == nil {
		t.Fatal("accepted changed config")
	}
	info, err := os.Stat(p)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("state mode: %v %v", info, err)
	}
}
func TestFirewallIdempotence(t *testing.T) {
	if os.Getenv("STATION_NETNS_TEST") != "1" {
		t.Skip("requires isolated network namespace")
	}
	if err := exec.Command("iptables", "-P", "FORWARD", "DROP").Run(); err != nil {
		t.Fatal(err)
	}
	c := validConfig()
	if err := apply(context.Background(), c, command); err != nil {
		t.Fatal(err)
	}
	if err := apply(context.Background(), c, command); err != nil {
		t.Fatal(err)
	}
	for _, r := range rules(c) {
		args := append([]string{"-t", r.table, "-C", r.chain}, r.args...)
		if out, err := exec.Command("iptables", args...).CombinedOutput(); err != nil {
			t.Fatalf("missing rule %s: %v %s", r.chain, err, out)
		}
	}
}

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteNamesKeepsForeignLinesAndTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte("127.0.0.1 localhost\n10.0.0.9 cp.old # rlark names\n10.254.240.5 cp.c1.rlark.internal # rlark control-plane"), 0644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	names := []hostName{{Name: "cp.c1.rlark.internal", Address: "10.254.240.5"}, {Name: "mgmt.rlark.internal", Address: "10.254.240.1"}}
	for range 2 {
		if err := writeNames(path, names); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := os.ReadFile(path)
	want := "127.0.0.1 localhost\n10.254.240.5 cp.c1.rlark.internal # rlark control-plane\n10.254.240.5 cp.c1.rlark.internal # rlark names\n10.254.240.1 mgmt.rlark.internal # rlark names\n"
	if string(got) != want {
		t.Fatalf("hosts:\n%s", got)
	}
	if after, _ := os.Stat(path); !os.SameFile(before, after) {
		t.Fatal("the hosts file was replaced, not rewritten in place")
	}
	// A declaration taken back takes its lines along, and a shorter file leaves no tail.
	if err := writeNames(path, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "127.0.0.1 localhost\n10.254.240.5 cp.c1.rlark.internal # rlark control-plane\n" {
		t.Fatalf("hosts without names:\n%s", got)
	}
	if err := writeNames(filepath.Join(t.TempDir(), "none"), names); err == nil {
		t.Fatal("a hosts file that is not there must be an error, not a new file")
	}
}

func TestWriteResolvFollowsTheSiteAndElseTheMachine(t *testing.T) {
	dir := t.TempDir()
	// The directory is not there on a node the installer did not make.
	path, own := filepath.Join(dir, "rlark", "station", "resolv.conf"), filepath.Join(dir, "machine")
	if err := os.WriteFile(own, []byte("nameserver 10.11.0.1\nsearch lab\n"), 0644); err != nil {
		t.Fatal(err)
	}
	machine := []string{filepath.Join(dir, "absent"), own}
	for _, step := range []struct {
		servers []string
		want    string
	}{{nil, "nameserver 10.11.0.1\nsearch lab\n"}, {[]string{"192.168.31.1"}, "nameserver 192.168.31.1\n"}, {nil, "nameserver 10.11.0.1\nsearch lab\n"}} {
		if err := writeResolv(path, step.servers, machine); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); string(got) != step.want {
			t.Fatalf("resolv.conf for %v:\n%s", step.servers, got)
		}
	}
}

func TestParseTakesNamesAndNameServers(t *testing.T) {
	d, err := parse([]byte(`{"interface":"st-wg0","address":"10.254.240.3/32","listenPort":51820,"mtu":1380,"peers":[],"routes":[],"names":[{"name":"cp.c1.rlark.internal","address":"10.254.240.5"}],"dns":["192.168.31.1"]}`))
	if err != nil || len(d.Names) != 1 || d.Names[0].Address != "10.254.240.5" || len(d.DNS) != 1 {
		t.Fatalf("%+v %v", d, err)
	}
	for _, bad := range []string{`"names":[{"name":"a b","address":"10.0.0.1"}]`, `"names":[{"name":"a","address":"x"}]`, `"dns":["x"]`} {
		if _, err := parse([]byte(`{"interface":"st-wg0","address":"10.254.240.3/32","listenPort":51820,"mtu":1380,"peers":[],"routes":[],` + bad + `}`)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

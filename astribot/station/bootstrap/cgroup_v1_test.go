package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cgroup_v1, as dash runs it: a machine on cgroup v1 whose kubelet is 1.35 or later gets the drop-in that lets the
// kubelet start there, and kubeadm is told to skip its system verification; a machine on v2, or a kubelet before
// 1.35, gets neither.
func TestCgroupV1AllowanceForA135KubeletOnV1Only(t *testing.T) {
	for name, test := range map[string]struct {
		fs, version string
		allowed     bool
	}{
		"v1, 1.36":  {"tmpfs", "v1.36.0", true},
		"v1, 1.35":  {"tmpfs", "v1.35.2", true},
		"v1, 1.34":  {"tmpfs", "v1.34.3", false},
		"v2, 1.36":  {"cgroup2fs", "v1.36.0", false},
		"v1, 1.100": {"tmpfs", "v1.100.0", true},
	} {
		t.Run(name, func(t *testing.T) {
			stubs, dir := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(stubs, "stat"), []byte("#!/bin/sh\necho \"$FS\"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			conf := filepath.Join(dir, "kubelet.conf.d", "90-rlark-cgroup-v1.conf")
			command := exec.Command("dash", "-c", `set -eu; . ./lib.sh; CGROUP_V1_CONF=$CONF; preflight=$(cgroup_v1 "$VERSION"); echo "$preflight"`)
			command.Env = append(os.Environ(), "PATH="+stubs+":"+os.Getenv("PATH"), "FS="+test.fs, "VERSION="+test.version, "CONF="+conf)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
			data, err := os.ReadFile(conf)
			if !test.allowed {
				if got := strings.TrimSpace(string(output)); got != "" || err == nil {
					t.Fatalf("printed %q, drop-in written: %v; want neither", got, err == nil)
				}
				return
			}
			if got := strings.TrimSpace(string(output)); got != "SystemVerification" {
				t.Fatalf("printed %q, want SystemVerification", got)
			}
			if err != nil || !strings.Contains(string(data), "kind: KubeletConfiguration\nfailCgroupV1: false\n") {
				t.Fatalf("drop-in %q: %v", data, err)
			}
		})
	}
}

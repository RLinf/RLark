package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiskPreflightForWorkerJoin(t *testing.T) {
	bin := t.TempDir()
	df := `#!/bin/sh
printf '%s\n' 'Filesystem 1024-blocks Used Available Capacity Mounted on'
printf '/dev/test 1000000 0 %s 0%% /\n' "$TEST_FREE_KIB"
`
	if err := os.WriteFile(filepath.Join(bin, "df"), []byte(df), 0755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, freeKiB, check, wantError string
	}{
		{"worker with 9.9 percent free", "99999", "disk 10", "requires at least 10 % free"},
		{"worker with exactly 10 percent free", "100000", "disk 10", ""},
		{"worker with 16 percent free", "160000", "disk 10", ""},
		{"control plane with 16 percent free", "160000", "disk", "requires at least 20 % free"},
		{"control plane with exactly 20 percent free", "200000", "disk", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := exec.Command("sh", "-c", ". ./lib.sh; "+tc.check)
			command.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "TEST_FREE_KIB="+tc.freeKiB, "NODE_VOLUMES=")
			out, err := command.CombinedOutput()
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("preflight: %v\n%s", err, out)
				}
			} else if err == nil || !strings.Contains(string(out), tc.wantError) {
				t.Fatalf("preflight: got %v, %s, want %q", err, out, tc.wantError)
			}
		})
	}
}

//go:build linux

package netmac

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/sys/unix"
)

// TestIsAlreadyExists verifies the EEXIST detection used to make macvlan
// configuration idempotent (so reusing a leftover interface from a previous
// container instance does not fail on a duplicate addr/route). Netlink ACKs
// surface as a raw syscall.Errno, so errors.Is matches EEXIST directly (this
// replaces the former output-string "File exists" parsing).
func TestIsAlreadyExists(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"eexist", unix.EEXIST, true},
		{"wrapped_eexist", fmt.Errorf("add addr: %w", unix.EEXIST), true},
		{"enoent", unix.ENOENT, false},
		{"arbitrary", errors.New("something else"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := isAlreadyExists(c.err); got != c.want {
			t.Errorf("isAlreadyExists(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

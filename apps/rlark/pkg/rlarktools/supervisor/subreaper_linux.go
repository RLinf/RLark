//go:build linux

package supervisor

import "golang.org/x/sys/unix"

func enableSubreaper() error { return unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0) }

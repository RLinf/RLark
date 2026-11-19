package main

import "syscall"

// On a machine whose transparent huge pages are "always" (Baidu's GPU images), every region the Go runtime touches
// is a 2 MB page: this program stood at 45 MB instead of 8, and with its readiness check beside it passed the
// container's 96 Mi (instance-gt4l6j5v-1/-2, 11-19: anon_thp 88 MB of 94 at the kill). It takes ordinary pages.
func init() {
	const prSetTHPDisable = 41
	syscall.Syscall6(syscall.SYS_PRCTL, prSetTHPDisable, 1, 0, 0, 0, 0)
}

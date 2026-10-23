//go:build unix && !linux

package supervisor

func enableSubreaper() error { return nil }

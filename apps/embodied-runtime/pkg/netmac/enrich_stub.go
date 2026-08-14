//go:build !linux

package netmac

// EnrichMACVLANConfig is a no-op on non-Linux platforms where nsenter / the
// host network namespace are unavailable. The config keeps its original
// values; the caller must set HostNIC and IP explicitly.
func EnrichMACVLANConfig(cfg *MACVLANConfig, logPrefix string) {}

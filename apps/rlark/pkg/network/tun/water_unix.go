//go:build linux || darwin

package tun

import "github.com/songgao/water"

func newWater(name string) (ifce *water.Interface, err error) {
	config := water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: name,
		},
	}
	return water.New(config)
}

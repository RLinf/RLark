//go:build !linux && !darwin

package tun

import (
	"fmt"

	"github.com/songgao/water"
)

func newWater(name string) (ifce *water.Interface, err error) {
	return nil, fmt.Errorf("newWater is not implemented for this platform")
}

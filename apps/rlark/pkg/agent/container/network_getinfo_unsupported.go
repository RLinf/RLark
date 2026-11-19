//go:build !linux && !darwin

package container

import (
	"context"
	"fmt"
)

func (a *containerNetworkAdapter) getProcessNetworkInfo(ctx context.Context, pid int32) (string, string, error) {
	return "", "", fmt.Errorf("getProcessNetworkInfo is not supported on this platform")
}

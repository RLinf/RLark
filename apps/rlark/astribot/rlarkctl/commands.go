// Package rlarkctl holds our rlarkctl commands: release, registry, image and station.
package rlarkctl

import "github.com/spf13/cobra"

// Commands are added to rlarkctl's root.
func Commands() []*cobra.Command {
	return []*cobra.Command{releaseCommand(), registryCommand(), imageCommand(), stationCommand()}
}

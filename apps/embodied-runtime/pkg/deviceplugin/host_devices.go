package deviceplugin

import (
	"log"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// defaultHostDevicePermissions is the cgroup-style permission string applied
// when a HostDeviceConfig leaves Permissions empty. "rwm" grants read, write,
// and mknod — matching the historical default of the existing mounts.
const defaultHostDevicePermissions = "rwm"

// hostDeviceSpecs converts the configured HostDeviceConfig entries into the
// DeviceSpec messages returned by Allocate. Entries with an empty HostPath
// are skipped (and logged) so a stray blank line in the YAML does not break
// allocation. ContainerPath defaults to HostPath and Permissions defaults to
// "rwm" when left empty.
func hostDeviceSpecs(devices []HostDeviceConfig) []*pluginapi.DeviceSpec {
	out := make([]*pluginapi.DeviceSpec, 0, len(devices))
	for _, d := range devices {
		if d.HostPath == "" {
			log.Printf("[device-plugin] WARNING: host_devices entry with empty host_path — skipping")
			continue
		}
		containerPath := d.ContainerPath
		if containerPath == "" {
			containerPath = d.HostPath
		}
		permissions := d.Permissions
		if permissions == "" {
			permissions = defaultHostDevicePermissions
		}
		out = append(out, &pluginapi.DeviceSpec{
			HostPath:      d.HostPath,
			ContainerPath: containerPath,
			Permissions:   permissions,
		})
	}
	return out
}

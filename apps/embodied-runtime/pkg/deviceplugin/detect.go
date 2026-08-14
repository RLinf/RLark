package deviceplugin

import (
	"context"
	"fmt"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// detectDevices returns the device inventory for Kubernetes scheduling.
// Reports N unified devices, each representing a device runtime available
// on this node. N is configured via DeviceCount.
//
// Device health reflects whether the ros-controller, ros2-controller, and
// camera-controller subprocesses are alive. Device IDs are prefixed with the configured model
// (e.g. "franka-0") so different device types are distinguishable; when no
// model is set the prefix defaults to "device".
//
// When a controller is enabled and running, detectDevices also lists the
// robots/cameras from the controller via its Unix socket and caches the
// result for other components to consume.
func (p *Plugin) detectDevices() []*pluginapi.Device {
	ctx := context.Background()

	// Check controller health. A disabled manager (nil) does not affect
	// device health; an enabled but not-running manager marks devices as
	// Unhealthy.
	rosRunning := p.rosManager == nil || p.rosManager.IsRunning(ctx)
	ros2Running := p.ros2Manager == nil || p.ros2Manager.IsRunning(ctx)
	cameraRunning := p.cameraManager == nil || p.cameraManager.IsRunning(ctx)

	health := pluginapi.Healthy
	if !rosRunning || !ros2Running || !cameraRunning {
		health = pluginapi.Unhealthy
	}

	// Refresh the cached robot/camera inventory by listing through the
	// controllers' Unix sockets. Only attempt when the manager is enabled
	// and running so we don't spam logs with dial errors for controllers
	// that are disabled or not yet started.
	if p.rosManager != nil && rosRunning {
		p.refreshRobotInventory(ctx)
	}
	if p.ros2Manager != nil && ros2Running {
		p.refreshROS2RobotInventory(ctx)
	}
	if p.cameraManager != nil && cameraRunning {
		p.refreshCameraInventory(ctx)
	}

	prefix := p.cfg.Model
	if prefix == "" {
		prefix = "device"
	}

	devices := make([]*pluginapi.Device, p.cfg.DeviceCount)
	for i := 0; i < p.cfg.DeviceCount; i++ {
		devices[i] = &pluginapi.Device{
			ID:     fmt.Sprintf("%s-%d", prefix, i),
			Health: health,
		}
	}
	return devices
}

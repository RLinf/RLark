//go:build !linux

package cameracontroller

// EnrichV4L2Config is a no-op on non-Linux platforms where the V4L2 ioctl
// interface is unavailable. The config keeps its sysfs-only values
// (CameraType, Name, Params["device"]).
func EnrichV4L2Config(cfg *CameraConfig) {}

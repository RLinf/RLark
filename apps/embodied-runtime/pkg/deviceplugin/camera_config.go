package deviceplugin

import (
	"log"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cameracontroller"
	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/deviceplugin/camera"
	"gopkg.in/yaml.v3"
)

// generateCameraConfig builds the camera-controller YAML config from the
// inlined camera definitions of CameraConfig, optionally combining them
// with V4L2 cameras auto-detected from /sys/class/video4linux.
//
// Auto-detection runs unless AutoDetectV4L2 is explicitly false. It scans
// sysfs for every videoN node and produces a base CameraConfig per device
// (sysfs-only: ID, Name, CameraType=v4l2, Params["device"]). Querying each
// device's V4L2 capabilities — pixel formats, resolutions, framerates — is
// done by the camera-controller at startup (see EnrichV4L2Config), not here,
// since that step needs direct /dev/videoN access on the host.
//
// The auto-detected cameras form the base set; entries declared in
// Config.Cameras with the same ID override them (so the user can pin
// resolution, fps, or params for a specific node), while entries with new
// IDs are appended (e.g. RTSP or realsense cameras).
func generateCameraConfig(cfg CameraConfig) []byte {
	cameras := make([]cameracontroller.CameraConfig, 0)

	if cfg.AutoDetectV4L2 == nil || *cfg.AutoDetectV4L2 {
		sysCameras, err := camera.DetectV4L2Cameras(camera.Video4linuxSysPath)
		if err != nil {
			log.Printf("[device-plugin] WARNING: auto-detect V4L2 cameras: %v", err)
		}

		if len(sysCameras) > 0 {
			log.Printf("[device-plugin] auto-detected %d V4L2 camera(s)", len(sysCameras))
		}
		cameras = sysCameras
	}
	cameras = camera.MergeCameras(cameras, cfg.Cameras)

	camcfg := cameracontroller.ControllerConfig{
		HTTPAddr: cfg.HTTPAddr,
		Cameras:  cameras,
	}

	data, err := yaml.Marshal(&camcfg)
	if err != nil {
		log.Printf("[device-plugin] WARNING: marshal camera-controller config: %v", err)
		return nil
	}
	return data
}

package camera

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cameracontroller"
)

// Video4linuxSysPath is the sysfs root exposing one entry per V4L2 device
// node (video0, video1, ...).
const Video4linuxSysPath = "/sys/class/video4linux"

// DetectV4L2Cameras scans sysfs (typically /sys/class/video4linux) for V4L2
// video devices and returns a CameraConfig for each detected node.
//
// Each sysfs entry videoN contains a "name" file with the human-readable
// device name reported by the kernel driver. The corresponding character
// device node is /dev/videoN, stored in the config's Params.
//
// All videoN entries are reported as v4l2 cameras. Non-capture V4L2 nodes
// (VBI / metadata / radio do not start with "video" and are skipped; metadata
// siblings of a capture node share its name but get a distinct videoN entry).
// Nodes that fail to open at runtime are logged by the camera-controller.
//
// The sysPath argument makes the function testable with a fake sysfs tree.
func DetectV4L2Cameras(sysPath string) ([]cameracontroller.CameraConfig, error) {
	entries, err := os.ReadDir(sysPath)
	if err != nil {
		return nil, err
	}

	var cams []cameracontroller.CameraConfig
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "video") {
			continue
		}

		devName, ok := readSysfsName(filepath.Join(sysPath, name, "name"))
		if !ok {
			continue
		}

		cams = append(cams, cameracontroller.CameraConfig{
			ID:         name, // "videoN" — unique within a boot
			Name:       devName,
			CameraType: "v4l2",
			Params: map[string]string{
				"device": "/dev/" + name,
			},
		})
	}

	// Sort by ID for deterministic output (sysfs ordering is not guaranteed).
	sort.Slice(cams, func(i, j int) bool {
		return cams[i].ID < cams[j].ID
	})

	return cams, nil
}

// readSysfsName reads the "name" file of a video4linux sysfs entry and returns
// the trimmed value. Returns ok=false if the file is missing or empty.
func readSysfsName(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return "", false
	}
	return name, true
}

// MergeCameras combines auto-detected cameras with manually-configured ones.
// When a manual config shares an ID with an auto-detected camera, the manual
// entry overrides it — allowing the user to set resolution, fps, params, etc.
// for a specific auto-detected device. Manual entries with new IDs are
// appended (e.g. RTSP or realsense cameras declared in the device config).
func MergeCameras(auto, manual []cameracontroller.CameraConfig) []cameracontroller.CameraConfig {
	merged := make([]cameracontroller.CameraConfig, 0, len(auto)+len(manual))
	idx := make(map[string]int, len(auto)+len(manual))

	for _, c := range auto {
		idx[c.ID] = len(merged)
		merged = append(merged, c)
	}
	for _, c := range manual {
		if i, ok := idx[c.ID]; ok {
			merged[i] = c // override auto-detected entry
		} else {
			idx[c.ID] = len(merged)
			merged = append(merged, c)
		}
	}
	return merged
}

package camera

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cameracontroller"
)

// TestDetectV4L2Cameras builds a fake sysfs tree and verifies that only valid
// videoN entries with a non-empty "name" file are turned into CameraConfigs.
func TestDetectV4L2Cameras(t *testing.T) {
	sysPath := t.TempDir()

	mkdir := func(name string) {
		if err := os.MkdirAll(filepath.Join(sysPath, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeName := func(name, value string) {
		mkdir(name)
		if err := os.WriteFile(filepath.Join(sysPath, name, "name"), []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}

	writeName("video0", "USB Camera\n")
	writeName("video1", "Integrated Webcam")
	writeName("video2", "")       // empty name → skipped
	mkdir("video3")               // no name file → skipped
	writeName("notavideo", "Foo") // not a videoN entry → skipped

	cams, err := DetectV4L2Cameras(sysPath)
	if err != nil {
		t.Fatalf("DetectV4L2Cameras: %v", err)
	}

	if want := 2; len(cams) != want {
		t.Fatalf("got %d cameras, want %d: %+v", len(cams), want, cams)
	}

	// Sorted by ID → video0 first.
	c0 := cams[0]
	if c0.ID != "video0" || c0.Name != "USB Camera" || c0.CameraType != "v4l2" {
		t.Errorf("cam0 = %+v", c0)
	}
	if got := c0.Params["device"]; got != "/dev/video0" {
		t.Errorf("cam0 device param = %q, want /dev/video0", got)
	}

	c1 := cams[1]
	if c1.ID != "video1" || c1.Name != "Integrated Webcam" {
		t.Errorf("cam1 = %+v", c1)
	}
	if got := c1.Params["device"]; got != "/dev/video1" {
		t.Errorf("cam1 device param = %q, want /dev/video1", got)
	}
}

// TestDetectV4L2Cameras_MissingPath verifies that a missing sysfs path yields
// an error and no cameras (e.g. non-Linux hosts where the path is absent).
func TestDetectV4L2Cameras_MissingPath(t *testing.T) {
	cams, err := DetectV4L2Cameras(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing sysfs path")
	}
	if cams != nil {
		t.Fatalf("expected nil cameras, got %v", cams)
	}
}

// TestDetectV4L2Cameras_EmptyDir verifies an existing but empty sysfs dir
// returns no cameras and no error.
func TestDetectV4L2Cameras_EmptyDir(t *testing.T) {
	cams, err := DetectV4L2Cameras(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cams != nil {
		t.Fatalf("expected nil cameras, got %v", cams)
	}
}

// TestMergeCameras verifies that manual entries override auto-detected ones by
// ID and that new manual entries are appended.
func TestMergeCameras(t *testing.T) {
	auto := []cameracontroller.CameraConfig{
		{ID: "video0", Name: "USB Camera", CameraType: "v4l2"},
		{ID: "video1", Name: "Webcam", CameraType: "v4l2"},
	}
	manual := []cameracontroller.CameraConfig{
		{ID: "video0", Name: "Overridden", CameraType: "v4l2", Width: 1280, Height: 720, FPS: 30},
		{ID: "rtsp-1", Name: "IP Cam", CameraType: "rtsp"},
	}

	merged := MergeCameras(auto, manual)

	if want := 3; len(merged) != want {
		t.Fatalf("got %d merged, want %d: %+v", len(merged), want, merged)
	}

	byID := make(map[string]cameracontroller.CameraConfig, len(merged))
	for _, c := range merged {
		byID[c.ID] = c
	}
	if byID["video0"].Name != "Overridden" || byID["video0"].Width != 1280 {
		t.Errorf("video0 not overridden: %+v", byID["video0"])
	}
	if byID["video1"].Name != "Webcam" {
		t.Errorf("video1 should be untouched: %+v", byID["video1"])
	}
	if byID["rtsp-1"].Name != "IP Cam" {
		t.Errorf("rtsp-1 missing: %+v", byID["rtsp-1"])
	}
}

// TestMergeCameras_Order verifies auto-detected entries keep their relative
// order and overrides happen in place.
func TestMergeCameras_Order(t *testing.T) {
	auto := []cameracontroller.CameraConfig{
		{ID: "video0", Name: "A"},
		{ID: "video1", Name: "B"},
		{ID: "video2", Name: "C"},
	}
	manual := []cameracontroller.CameraConfig{
		{ID: "video2", Name: "C2"},
		{ID: "video0", Name: "A2"},
	}

	merged := MergeCameras(auto, manual)

	if want := 3; len(merged) != want {
		t.Fatalf("got %d merged, want %d", len(merged), want)
	}
	// In-place overrides preserve the auto order.
	if merged[0].ID != "video0" || merged[0].Name != "A2" {
		t.Errorf("merged[0] = %+v, want {video0 A2}", merged[0])
	}
	if merged[1].ID != "video1" || merged[1].Name != "B" {
		t.Errorf("merged[1] = %+v, want {video1 B}", merged[1])
	}
	if merged[2].ID != "video2" || merged[2].Name != "C2" {
		t.Errorf("merged[2] = %+v, want {video2 C2}", merged[2])
	}
}

// TestMergeCameras_EmptyAuto verifies that with no auto-detected cameras, the
// manual list is returned as-is.
func TestMergeCameras_EmptyAuto(t *testing.T) {
	manual := []cameracontroller.CameraConfig{
		{ID: "rtsp-1", CameraType: "rtsp"},
	}
	merged := MergeCameras(nil, manual)
	if len(merged) != 1 || merged[0].ID != "rtsp-1" {
		t.Fatalf("merged = %+v", merged)
	}
}

package imagepull

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/go-logr/logr/testr"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestRuntimeFor(t *testing.T) {
	cases := []struct {
		in   rlarkv1alpha1.AgentType
		want ContainerRuntime
	}{
		{rlarkv1alpha1.AgentTypeKubernetes, RuntimeContainerd},
		{rlarkv1alpha1.AgentTypeDocker, RuntimeDocker},
		{rlarkv1alpha1.AgentTypeRaw, RuntimeNone},
		{"", RuntimeNone},
		{"unknown", RuntimeNone},
	}
	for _, c := range cases {
		if got := RuntimeFor(c.in); got != c.want {
			t.Errorf("RuntimeFor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNewImagePuller(t *testing.T) {
	log := testr.New(t)
	if got := NewImagePuller(RuntimeNone, "", "", log); got != nil {
		t.Errorf("NewImagePuller(RuntimeNone) = %v, want nil", got)
	}
	// containerd.New eagerly dials the socket. Try the system socket and
	// the rootless socket; if neither is accessible, skip the containerd case.
	socketCandidates := []string{
		"/run/containerd/containerd.sock",
		fmt.Sprintf("/run/user/%d/containerd/containerd.sock", os.Getuid()),
	}
	containerdAvailable := false
	for _, socket := range socketCandidates {
		got := NewImagePuller(RuntimeContainerd, socket, "k8s.io", log)
		if got != nil {
			containerdAvailable = true
			break
		}
	}
	if !containerdAvailable {
		t.Skip("no containerd socket accessible, skipping containerd case")
	}
	if got := NewImagePuller(RuntimeDocker, "", "", log); got == nil {
		t.Error("NewImagePuller(RuntimeDocker) = nil, want non-nil")
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1048576, "1.0MB"},
		{5242880, "5.0MB"},
		{1073741824, "1.0GB"},
		{3221225472, "3.0GB"},
	}
	for _, c := range cases {
		got := formatBytes(c.bytes)
		if got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.bytes, got, c.want)
		}
	}
}

// fakeExecPuller is a fake ImagePuller that records pulls and returns canned
// existence results, so puller behavior can be asserted without invoking real
// container runtime CLIs.
type fakeExecPuller struct {
	exists map[string]bool
	pulled []string
	pullFn func(image string) error
}

func (f *fakeExecPuller) Pull(ctx context.Context, image string, reporter ProgressReporter) error {
	if f.pullFn != nil {
		if err := f.pullFn(image); err != nil {
			return err
		}
	}
	f.pulled = append(f.pulled, image)
	if reporter != nil {
		reporter(Progress{Image: image, Status: "completed", Total: 1})
	}
	return nil
}

func (f *fakeExecPuller) ImageExists(ctx context.Context, image string) (bool, error) {
	if f.exists == nil {
		return false, nil
	}
	return f.exists[image], nil
}

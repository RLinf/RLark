package inuse

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestReadJoinsTheSortedNamesAndTakesAnAbsentDirectoryAsEmpty(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "in-use")
	if value, err := Read(directory); err != nil || value != "" {
		t.Fatalf("absent directory: (%q, %v)", value, err)
	}
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if value, err := Read(directory); err != nil || value != "" {
		t.Fatalf("empty directory: (%q, %v)", value, err)
	}
	for _, name := range []string{"s1u-robot", "fleet-station"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("why\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if value, err := Read(directory); err != nil || value != "fleet-station,s1u-robot" {
		t.Fatalf("two components: (%q, %v)", value, err)
	}
}

// recordingNodes stands in for the Node client: it records each value written
// and refuses the first failures writes.
type recordingNodes struct {
	mu       sync.Mutex
	failures int
	values   []string
}

func (n *recordingNodes) Patch(_ context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subresources ...string) (*corev1.Node, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if name != "robot-a" || pt != types.MergePatchType || len(subresources) != 0 {
		return nil, errors.New("unexpected patch")
	}
	if n.failures > 0 {
		n.failures--
		return nil, errors.New("the API server is not reachable yet")
	}
	var patch struct {
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(data, &patch); err != nil {
		return nil, err
	}
	value, found := patch.Metadata.Annotations[rlarkv1alpha1.NodeInUseAnnotation]
	if !found || len(patch.Metadata.Annotations) != 1 {
		return nil, errors.New("the patch must set only the in-use annotation")
	}
	n.values = append(n.values, value)
	return &corev1.Node{}, nil
}

func (n *recordingNodes) written() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.values...)
}

func TestReporterWritesTheFirstStateAndEachChange(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "in-use")
	// A node that has just joined: the first writes fail, and the idle state
	// is still reported once the API server answers.
	nodes := &recordingNodes{failures: 2}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- (&Reporter{Directory: directory, NodeName: "robot-a", Nodes: nodes, Interval: 10 * time.Millisecond}).Run(ctx)
	}()
	wait := func(want ...string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			got := nodes.written()
			if len(got) == len(want) && (len(got) == 0 || got[len(got)-1] == want[len(want)-1]) {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("written=%q, want %q", nodes.written(), want)
	}
	wait("")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "fleet-station"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	wait("", "fleet-station")
	if err := os.Remove(filepath.Join(directory, "fleet-station")); err != nil {
		t.Fatal(err)
	}
	wait("", "fleet-station", "")
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

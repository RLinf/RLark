// Package inuse reports a machine's in-use directory on its Kubernetes Node.
//
// Whatever is busy on a machine keeps a file of its own in /run/rlark/in-use
// (a robot's driver, a workload in the middle of its work) and removes it when
// it is done; the machine is in use while the directory holds any file. Host
// programs read the directory itself. The node Agent writes its content to the
// Node's rlark.io/in-use annotation, so that what decides in the cluster, such
// as the update of a per-node Task, waits for the same signal.
package inuse

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

const (
	// Interval is how often the node Agent reads the directory: a machine that
	// became busy shows on its Node within this long.
	Interval = 3 * time.Second
	// refresh is how often an unchanged value is written again, which puts the
	// annotation back if it was removed. Writing an equal value changes nothing
	// on the Node.
	refresh = time.Minute
)

// NodePatcher is the part of the Kubernetes Node client the Reporter uses.
type NodePatcher interface {
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*corev1.Node, error)
}

// Reporter keeps one Node's in-use annotation equal to its machine's directory.
type Reporter struct {
	// Directory is the in-use directory as the Agent sees it.
	Directory string
	NodeName  string
	Nodes     NodePatcher
	// Interval is how often the directory is read.
	Interval time.Duration
}

// Read returns the annotation value for directory: the names in it, sorted and
// joined by commas; empty for an empty or absent directory, which whoever
// keeps files there may create and remove.
func Read(directory string) (string, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return strings.Join(names, ","), nil
}

// Run reports until ctx ends. A directory that cannot be read or a Node that
// cannot be written leaves the annotation as it was and is tried again: the
// last reported state stands, and a Node never reported counts as in use.
func (r *Reporter) Run(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("in-use").WithValues("node", r.NodeName, "directory", r.Directory)
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	reported, written, failing := "", time.Time{}, false
	for {
		value, err := Read(r.Directory)
		if err == nil && (written.IsZero() || value != reported || time.Since(written) >= refresh) {
			if err = r.write(ctx, value); err == nil {
				if written.IsZero() || value != reported {
					logger.Info("reported", "inUse", value)
				}
				reported, written = value, time.Now()
			}
		}
		if err != nil && !failing {
			logger.Error(err, "report the in-use directory; retrying")
		}
		failing = err != nil
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *Reporter) write(ctx context.Context, value string) error {
	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]string{rlarkv1alpha1.NodeInUseAnnotation: value}}})
	if err != nil {
		return err
	}
	_, err = r.Nodes.Patch(ctx, r.NodeName, types.MergePatchType, patch, metav1.PatchOptions{})
	return err
}

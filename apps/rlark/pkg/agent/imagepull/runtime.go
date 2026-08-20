// Package imagepull implements node-level image pull progress monitoring for the node-agent.
//
// The Puller watches management Tasks and monitors the progress of image pulls
// initiated by other components (e.g., kubelet, ctr, the CRI plugin). It does
// NOT actively pull images itself. Instead, it interacts with the container
// runtime (containerd or docker) to observe pull progress:
//
//   - Kubernetes -> containerd (via the containerd Go client over containerd.sock)
//   - Docker     -> docker     (via the `docker` CLI for existence checks)
//   - Raw        -> none       (skipped, no container runtime)
//
// For containerd, the Puller subscribes to containerd events and polls the
// content store to report download progress. Progress reporting is best-effort:
// it never writes back to the Task status and only updates Node status via the
// ProgressPatcher.
package imagepull

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/go-logr/logr"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// ContainerRuntime identifies the container runtime available on a node.
type ContainerRuntime string

const (
	RuntimeContainerd ContainerRuntime = "containerd"
	RuntimeDocker     ContainerRuntime = "docker"
	RuntimeNone       ContainerRuntime = "none"
)

const (
	// contentStorePollInterval is how often the containerd content store is
	// polled for active download progress.
	contentStorePollInterval = 1 * time.Second
	// imageExistCheckInterval is how often the docker puller polls for image
	// existence (docker has no content store API to monitor).
	imageExistCheckInterval = 2 * time.Second
)

// RuntimeFor maps an AgentType to the container runtime used for image pulling.
func RuntimeFor(agentType rlarkv1alpha1.AgentType) ContainerRuntime {
	switch agentType {
	case rlarkv1alpha1.AgentTypeKubernetes:
		return RuntimeContainerd
	case rlarkv1alpha1.AgentTypeDocker:
		return RuntimeDocker
	default:
		return RuntimeNone
	}
}

// Progress represents the current progress of an image pull operation.
type Progress struct {
	Image      string
	Downloaded int64
	Total      int64
	Speed      float64
	Status     string
	Message    string
}

// ProgressReporter is a callback function invoked during image pulling
// to report progress updates.
type ProgressReporter func(Progress)

// ImagePuller monitors image pull progress and reports whether an image is
// already present in the local container runtime. It does NOT initiate pulls;
// it only observes pulls initiated by other components.
type ImagePuller interface {
	// Pull waits for the given image to be pulled by another component (e.g.,
	// kubelet) and reports progress via the optional reporter. It returns nil
	// when the image is fully present locally.
	Pull(ctx context.Context, image string, reporter ProgressReporter) error
	// ImageExists reports whether the image is already present locally.
	// It is best-effort: a false negative triggers a harmless wait, while a
	// false positive returns immediately.
	ImageExists(ctx context.Context, image string) (bool, error)
}

// NewImagePuller constructs the ImagePuller for the given runtime.
// It returns nil for RuntimeNone; callers must handle the nil (skip) case.
func NewImagePuller(rt ContainerRuntime, socket, namespace string, log logr.Logger) ImagePuller {
	switch rt {
	case RuntimeContainerd:
		client, err := containerd.New(socket, containerd.WithDefaultNamespace(namespace))
		if err != nil {
			log.Error(err, "failed to create containerd client, image monitoring disabled", "socket", socket)
			return nil
		}
		return &containerdPuller{client: client, socket: socket, namespace: namespace, log: log}
	case RuntimeDocker:
		return &dockerPuller{log: log}
	default:
		return nil
	}
}

// --- containerd ---

// containerdPuller monitors containerd's content store to track image pull
// progress. It does NOT initiate image pulls; it only watches for pulls
// initiated by other components (e.g., kubelet, ctr, the CRI plugin).
//
// The Puller subscribes to containerd's event bus for /images/create/ events
// and polls ContentStore().ListStatuses() to report download progress.
type containerdPuller struct {
	client    *containerd.Client
	socket    string
	namespace string
	log       logr.Logger
}

func (p *containerdPuller) Pull(ctx context.Context, image string, reporter ProgressReporter) error {
	// Fast path: image already exists locally.
	exists, err := p.imageExists(ctx, image)
	if err != nil {
		return fmt.Errorf("check image existence for %s: %w", image, err)
	}
	if exists {
		if reporter != nil {
			reporter(Progress{Image: image, Status: "completed", Message: "image already present"})
		}
		return nil
	}

	if reporter != nil {
		reporter(Progress{Image: image, Status: "pulling", Message: "waiting for pull to start"})
	}

	p.log.V(1).Info("monitoring image pull progress", "image", image, "socket", p.socket, "namespace", p.namespace)

	// Subscribe to containerd events for image creation. This lets us detect
	// when the pull completes (image is created in the image store) without
	// excessive polling. The filter uses containerd's selector syntax
	// (topic=="<value>") rather than a bare topic string, since '/' is a
	// quote rune in the filter grammar and "/images/create/" fails to parse.
	eventCh, errCh := p.client.Subscribe(ctx, `topic=="/images/create"`)

	ticker := time.NewTicker(contentStorePollInterval)
	defer ticker.Stop()

	var (
		lastDownloaded int64 = -1
		lastTotal      int64 = -1
		lastBlobCount        = -1
		lastMsg        string
		startTime      time.Time
		monitorStart   = time.Now()
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-eventCh:
			// An image event was received; check if our target image is now available.
			if exists, _ := p.imageExists(ctx, image); exists {
				if reporter != nil {
					reporter(Progress{
						Image:   image,
						Status:  "completed",
						Message: "image pulled successfully",
					})
				}
				return nil
			}

		case err := <-errCh:
			if err != nil {
				p.log.V(1).Info("containerd event subscription error", "err", err)
			}

		case <-ticker.C:
			// Check if image now exists (covers the case where we missed the event).
			if exists, _ := p.imageExists(ctx, image); exists {
				if reporter != nil {
					reporter(Progress{
						Image:   image,
						Status:  "completed",
						Message: "image pulled successfully",
					})
				}
				return nil
			}

			if reporter == nil {
				continue
			}

			// Poll the content store for active downloads.
			statuses, err := p.client.ContentStore().ListStatuses(ctx)
			if err != nil {
				p.log.V(1).Info("failed to list content statuses", "err", err)
				continue
			}

			if len(statuses) > 0 {
				if startTime.IsZero() {
					startTime = time.Now()
				}

				var downloaded, total int64
				for _, s := range statuses {
					downloaded += s.Offset
					total += s.Total
				}

				// Only report when something changed to avoid flooding.
				var msg string
				if total > 0 {
					msg = fmt.Sprintf("downloading: %s / %s (%d blobs)", formatBytes(downloaded), formatBytes(total), len(statuses))
				} else {
					msg = fmt.Sprintf("downloading: %s (%d blobs)", formatBytes(downloaded), len(statuses))
				}

				if downloaded == lastDownloaded && total == lastTotal &&
					len(statuses) == lastBlobCount && msg == lastMsg {
					continue
				}

				lastDownloaded = downloaded
				lastTotal = total
				lastBlobCount = len(statuses)
				lastMsg = msg

				var speed float64
				elapsed := time.Since(startTime).Seconds()
				if elapsed > 0 && downloaded > 0 {
					speed = float64(downloaded) / elapsed
				}

				reporter(Progress{
					Image:      image,
					Downloaded: downloaded,
					Total:      total,
					Speed:      speed,
					Status:     "pulling",
					Message:    msg,
				})
			} else if lastDownloaded > 0 {
				// Downloads finished but image not yet created — extraction in progress.
				msg := "extracting layers"
				if msg != lastMsg {
					lastMsg = msg
					reporter(Progress{
						Image:   image,
						Status:  "pulling",
						Message: msg,
					})
				}
			} else {
				// No download activity yet — keep reporting so the operator
				// can see the agent is still polling. Include elapsed seconds
				// so the message changes each tick and bypasses the reporter
				// throttle.
				elapsed := int(time.Since(monitorStart).Seconds())
				msg := fmt.Sprintf("waiting for pull to start (%ds elapsed)", elapsed)
				if msg != lastMsg {
					lastMsg = msg
					reporter(Progress{
						Image:   image,
						Status:  "pulling",
						Message: msg,
					})
				}
			}
		}
	}
}

func (p *containerdPuller) ImageExists(ctx context.Context, image string) (bool, error) {
	return p.imageExists(ctx, image)
}

// imageExists checks whether the image is present in the containerd image store.
func (p *containerdPuller) imageExists(ctx context.Context, image string) (bool, error) {
	_, err := p.client.ImageService().Get(ctx, image)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// --- docker ---

// dockerPuller monitors docker image availability. Unlike containerd, docker
// does not expose a content store API for progress monitoring, so the Pull
// method simply waits for the image to appear (pulled by another component)
// and reports status transitions only.
type dockerPuller struct {
	log logr.Logger
}

func (p *dockerPuller) Pull(ctx context.Context, image string, reporter ProgressReporter) error {
	exists, err := p.imageExists(ctx, image)
	if err != nil {
		return fmt.Errorf("check image existence for %s: %w", image, err)
	}
	if exists {
		if reporter != nil {
			reporter(Progress{Image: image, Status: "completed", Message: "image already present"})
		}
		return nil
	}

	if reporter != nil {
		reporter(Progress{Image: image, Status: "pulling", Message: "waiting for image to be pulled"})
	}

	p.log.V(1).Info("monitoring image availability", "image", image)

	ticker := time.NewTicker(imageExistCheckInterval)
	defer ticker.Stop()

	monitorStart := time.Now()
	var lastMsg string
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			exists, err := p.imageExists(ctx, image)
			if err != nil {
				p.log.V(1).Info("failed to check image existence", "image", image, "err", err)
				continue
			}
			if exists {
				if reporter != nil {
					reporter(Progress{Image: image, Status: "completed", Message: "image pulled successfully"})
				}
				return nil
			}
			// Continuously re-report the waiting status so the progress map
			// stays warm and the operator can see the agent is still polling.
			// Include elapsed seconds so the message changes each tick and
			// bypasses the reporter throttle.
			if reporter != nil {
				elapsed := int(time.Since(monitorStart).Seconds())
				msg := fmt.Sprintf("waiting for image to be pulled (%ds elapsed)", elapsed)
				if msg != lastMsg {
					lastMsg = msg
					reporter(Progress{Image: image, Status: "pulling", Message: msg})
				}
			}
		}
	}
}

func (p *dockerPuller) ImageExists(ctx context.Context, image string) (bool, error) {
	return p.imageExists(ctx, image)
}

func (p *dockerPuller) imageExists(ctx context.Context, image string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}

// formatBytes formats a byte count into a human-readable string.
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

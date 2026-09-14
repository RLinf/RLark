package gateway

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const (
	imageHistoryScanLimit = 500
	imageHistoryListLimit = 10
)

type imageUsage struct {
	Image      string    `json:"image"`
	UseCount   int       `json:"useCount"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

func (g *Gateway) initializeImageUsage() error {
	jobs, err := g.kubeClient.RlinfV1alpha1().Jobs().List(context.Background(), metav1.ListOptions{Limit: imageHistoryScanLimit})
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}
	sort.Slice(jobs.Items, func(i, j int) bool {
		return jobs.Items[i].CreationTimestamp.After(jobs.Items[j].CreationTimestamp.Time)
	})

	g.imagesMu.Lock()
	defer g.imagesMu.Unlock()
	g.images = make(map[string]imageUsage)
	for i := range jobs.Items {
		g.recordJobImagesLocked(&jobs.Items[i])
	}
	return nil
}

func (g *Gateway) recordJobImages(job *rlarkiov1alpha1.Job) {
	g.imagesMu.Lock()
	defer g.imagesMu.Unlock()
	g.recordJobImagesLocked(job)
}

func (g *Gateway) recordJobImagesLocked(job *rlarkiov1alpha1.Job) {
	lastUsedAt := job.CreationTimestamp.Time
	if lastUsedAt.IsZero() {
		lastUsedAt = time.Now()
	}
	for _, image := range jobImages(job) {
		usage := g.images[image]
		usage.Image = image
		usage.UseCount++
		if usage.LastUsedAt.Before(lastUsedAt) {
			usage.LastUsedAt = lastUsedAt
		}
		g.images[image] = usage
	}
}

func jobImages(job *rlarkiov1alpha1.Job) []string {
	images := make(map[string]struct{})
	for _, task := range job.Spec.Tasks {
		if task.Kubernetes != nil && task.Kubernetes.Workload != nil {
			for _, container := range task.Kubernetes.Workload.Template.Spec.InitContainers {
				if container.Image != "" {
					images[container.Image] = struct{}{}
				}
			}
			for _, container := range task.Kubernetes.Workload.Template.Spec.Containers {
				if container.Image != "" {
					images[container.Image] = struct{}{}
				}
			}
		}
		if task.Docker != nil {
			for _, container := range task.Docker.Containers {
				if container.Image != "" {
					images[container.Image] = struct{}{}
				}
			}
		}
	}

	result := make([]string, 0, len(images))
	for image := range images {
		result = append(result, image)
	}
	return result
}

func (g *Gateway) listImages(c *gin.Context) {
	g.imagesMu.RLock()
	images := make([]imageUsage, 0, len(g.images))
	for _, image := range g.images {
		images = append(images, image)
	}
	g.imagesMu.RUnlock()

	sort.Slice(images, func(i, j int) bool {
		return images[i].LastUsedAt.After(images[j].LastUsedAt)
	})
	if len(images) > imageHistoryListLimit {
		images = images[:imageHistoryListLimit]
	}
	c.JSON(200, gin.H{"items": images})
}

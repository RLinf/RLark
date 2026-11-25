package task

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
)

// newTestKubernetesExecutionCoordinator bypasses complete assembly validation.
// Environment-default behavior is tested separately from explicit test wiring.
func newTestKubernetesExecutionCoordinator(kubeClient client.Client, image string) (*coordinator.Coordinator, error) {
	return newKubernetesExecutionCoordinator(context.Background(), kubeClient, KubernetesExecutionConfig{
		Image: image, TensorBoardImage: defaultTensorBoardImage, RestartableInitContainers: true,
	})
}

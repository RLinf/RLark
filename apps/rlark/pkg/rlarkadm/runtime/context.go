// Package runtime provides context helpers for passing runtime dependencies.
package runtime

import (
	"context"

	"k8s.io/client-go/kubernetes"
)

type contextKey string

const (
	contextKeyClientset  contextKey = "clientset"
	contextKeyKubeconfig contextKey = "kubeconfig"
)

// WithClientset injects a Kubernetes clientset into the context.
func WithClientset(ctx context.Context, clientset kubernetes.Interface) context.Context {
	return context.WithValue(ctx, contextKeyClientset, clientset)
}

// GetClientset retrieves a Kubernetes clientset from the context.
func GetClientset(ctx context.Context) (kubernetes.Interface, bool) {
	clientset, ok := ctx.Value(contextKeyClientset).(kubernetes.Interface)
	return clientset, ok
}

// WithKubeconfig injects a kubeconfig path into the context.
// This is optional and only needed for components that require kubectl exec (e.g., KCP).
func WithKubeconfig(ctx context.Context, kubeconfig string) context.Context {
	return context.WithValue(ctx, contextKeyKubeconfig, kubeconfig)
}

// GetKubeconfig retrieves a kubeconfig path from the context.
func GetKubeconfig(ctx context.Context) (string, bool) {
	kubeconfig, ok := ctx.Value(contextKeyKubeconfig).(string)
	return kubeconfig, ok
}

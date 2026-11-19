package sync

import "sigs.k8s.io/controller-runtime/pkg/client"

const (
	// SyncFinalizer is the finalizer added to resources to ensure
	// they are synced to PostgreSQL before deletion.
	SyncFinalizer = "sync.rlinf.io/persist"
)

// CheckSync checks the sync.
func CheckSync(obj client.Object) bool {
	if anno := obj.GetAnnotations(); anno != nil {
		if _, ok := anno["skip-sync"]; ok {
			return false
		}
	}
	return true
}

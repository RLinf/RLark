package upgrade

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func setupTestReconciler(objs ...runtime.Object) *Reconciler {
	scheme := runtime.NewScheme()
	_ = rlarkv1alpha1.AddToScheme(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(&rlarkv1alpha1.UpgradeJob{}).Build()

	reconciler := &Reconciler{
		Client: client,
		Scheme: scheme,
	}

	return reconciler
}

// reconcileN performs n reconcile iterations to allow multiple ClusterUpgradeJobs
// to be created (one per reconcile due to concurrency control).
func reconcileN(r *Reconciler, name string, n int) error {
	for i := 0; i < n; i++ {
		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// reconcileTwice performs two reconcile iterations (first adds finalizer, second does the work)
func reconcileTwice(r *Reconciler, name string) error {
	return reconcileN(r, name, 2)
}

func TestReconcile_CreateClusterUpgradeJobs(t *testing.T) {
	job := &rlarkv1alpha1.UpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0",
		},
		Spec: rlarkv1alpha1.UpgradeJobSpec{
			Image:        "registry.rlark.io/rlarkadm:v0.2.0",
			ClusterNames: []string{"beijing", "shanghai"},
			Strategy: rlarkv1alpha1.UpgradeStrategy{
				Concurrency: 2,
			},
		},
	}

	r := setupTestReconciler(job)

	// Need 4 reconciles: 1 for finalizer, 3 for creating 2 ClusterUpgradeJobs
	// (first reconcile creates 1, second detects active=1 and creates 1 more,
	// third confirms no more needed).
	err := reconcileN(r, job.Name, 4)
	require.NoError(t, err)

	// Check that ClusterUpgradeJobs were created
	var clusterUpgradeJobs rlarkv1alpha1.ClusterUpgradeJobList
	err = r.List(context.Background(), &clusterUpgradeJobs)
	require.NoError(t, err)
	assert.Len(t, clusterUpgradeJobs.Items, 2)

	// Check labels
	for _, cuj := range clusterUpgradeJobs.Items {
		assert.Equal(t, "upgrade-v0.2.0", cuj.Labels["rlark.io/upgrade-job"])
	}
}

func TestReconcile_AggregateStatus(t *testing.T) {
	job := &rlarkv1alpha1.UpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0",
		},
		Spec: rlarkv1alpha1.UpgradeJobSpec{
			Image:        "registry.rlark.io/rlarkadm:v0.2.0",
			ClusterNames: []string{"beijing", "shanghai"},
		},
	}

	clusterUpgradeJob1 := &rlarkv1alpha1.ClusterUpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0-beijing",
			Labels: map[string]string{
				"rlark.io/upgrade-job": "upgrade-v0.2.0",
				"rlark.io/cluster":     "beijing",
			},
		},
		Status: rlarkv1alpha1.ClusterUpgradeJobStatus{
			Phase: rlarkv1alpha1.ClusterUpgradeJobSucceeded,
		},
	}

	clusterUpgradeJob2 := &rlarkv1alpha1.ClusterUpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0-shanghai",
			Labels: map[string]string{
				"rlark.io/upgrade-job": "upgrade-v0.2.0",
				"rlark.io/cluster":     "shanghai",
			},
		},
		Status: rlarkv1alpha1.ClusterUpgradeJobStatus{
			Phase: rlarkv1alpha1.ClusterUpgradeJobUpgrading,
		},
	}

	r := setupTestReconciler(job, clusterUpgradeJob1, clusterUpgradeJob2)

	err := reconcileTwice(r, job.Name)
	require.NoError(t, err)

	// Check aggregated status
	var updatedJob rlarkv1alpha1.UpgradeJob
	err = r.Get(context.Background(), types.NamespacedName{Name: job.Name}, &updatedJob)
	require.NoError(t, err)

	assert.Equal(t, int32(2), updatedJob.Status.TotalClusters)
	assert.Equal(t, int32(1), updatedJob.Status.UpgradedClusters)
	assert.Equal(t, int32(0), updatedJob.Status.FailedClusters)
	assert.NotNil(t, updatedJob.Status.StartTime)
}

func TestReconcile_AllSucceeded(t *testing.T) {
	job := &rlarkv1alpha1.UpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0",
		},
		Spec: rlarkv1alpha1.UpgradeJobSpec{
			Image:        "registry.rlark.io/rlarkadm:v0.2.0",
			ClusterNames: []string{"beijing", "shanghai"},
		},
	}

	clusterUpgradeJob1 := &rlarkv1alpha1.ClusterUpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0-beijing",
			Labels: map[string]string{
				"rlark.io/upgrade-job": "upgrade-v0.2.0",
			},
		},
		Status: rlarkv1alpha1.ClusterUpgradeJobStatus{
			Phase: rlarkv1alpha1.ClusterUpgradeJobSucceeded,
		},
	}

	clusterUpgradeJob2 := &rlarkv1alpha1.ClusterUpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0-shanghai",
			Labels: map[string]string{
				"rlark.io/upgrade-job": "upgrade-v0.2.0",
			},
		},
		Status: rlarkv1alpha1.ClusterUpgradeJobStatus{
			Phase: rlarkv1alpha1.ClusterUpgradeJobSucceeded,
		},
	}

	r := setupTestReconciler(job, clusterUpgradeJob1, clusterUpgradeJob2)

	err := reconcileTwice(r, job.Name)
	require.NoError(t, err)

	// Check final status
	var updatedJob rlarkv1alpha1.UpgradeJob
	err = r.Get(context.Background(), types.NamespacedName{Name: job.Name}, &updatedJob)
	require.NoError(t, err)

	assert.Equal(t, int32(2), updatedJob.Status.UpgradedClusters)
	assert.Equal(t, int32(0), updatedJob.Status.FailedClusters)
	assert.Equal(t, rlarkv1alpha1.UpgradeJobSucceeded, updatedJob.Status.Phase)
	assert.NotNil(t, updatedJob.Status.CompletionTime)
}

func TestReconcile_OneFailed(t *testing.T) {
	job := &rlarkv1alpha1.UpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0",
		},
		Spec: rlarkv1alpha1.UpgradeJobSpec{
			Image:        "registry.rlark.io/rlarkadm:v0.2.0",
			ClusterNames: []string{"beijing", "shanghai"},
		},
	}

	clusterUpgradeJob1 := &rlarkv1alpha1.ClusterUpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0-beijing",
			Labels: map[string]string{
				"rlark.io/upgrade-job": "upgrade-v0.2.0",
			},
		},
		Status: rlarkv1alpha1.ClusterUpgradeJobStatus{
			Phase: rlarkv1alpha1.ClusterUpgradeJobSucceeded,
		},
	}

	clusterUpgradeJob2 := &rlarkv1alpha1.ClusterUpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0-shanghai",
			Labels: map[string]string{
				"rlark.io/upgrade-job": "upgrade-v0.2.0",
			},
		},
		Status: rlarkv1alpha1.ClusterUpgradeJobStatus{
			Phase: rlarkv1alpha1.ClusterUpgradeJobFailed,
		},
	}

	r := setupTestReconciler(job, clusterUpgradeJob1, clusterUpgradeJob2)

	err := reconcileTwice(r, job.Name)
	require.NoError(t, err)

	// Check final status
	var updatedJob rlarkv1alpha1.UpgradeJob
	err = r.Get(context.Background(), types.NamespacedName{Name: job.Name}, &updatedJob)
	require.NoError(t, err)

	assert.Equal(t, int32(1), updatedJob.Status.UpgradedClusters)
	assert.Equal(t, int32(1), updatedJob.Status.FailedClusters)
	assert.Equal(t, rlarkv1alpha1.UpgradeJobFailed, updatedJob.Status.Phase)
}

func TestReconcile_RolledBack(t *testing.T) {
	job := &rlarkv1alpha1.UpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0",
		},
		Spec: rlarkv1alpha1.UpgradeJobSpec{
			Image:        "registry.rlark.io/rlarkadm:v0.2.0",
			ClusterNames: []string{"beijing", "shanghai"},
		},
	}

	clusterUpgradeJob1 := &rlarkv1alpha1.ClusterUpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0-beijing",
			Labels: map[string]string{
				"rlark.io/upgrade-job": "upgrade-v0.2.0",
			},
		},
		Status: rlarkv1alpha1.ClusterUpgradeJobStatus{
			Phase: rlarkv1alpha1.ClusterUpgradeJobSucceeded,
		},
	}

	clusterUpgradeJob2 := &rlarkv1alpha1.ClusterUpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0-shanghai",
			Labels: map[string]string{
				"rlark.io/upgrade-job": "upgrade-v0.2.0",
			},
		},
		Status: rlarkv1alpha1.ClusterUpgradeJobStatus{
			Phase: rlarkv1alpha1.ClusterUpgradeJobRolledBack,
		},
	}

	r := setupTestReconciler(job, clusterUpgradeJob1, clusterUpgradeJob2)

	err := reconcileTwice(r, job.Name)
	require.NoError(t, err)

	// Check final status
	var updatedJob rlarkv1alpha1.UpgradeJob
	err = r.Get(context.Background(), types.NamespacedName{Name: job.Name}, &updatedJob)
	require.NoError(t, err)

	assert.Equal(t, int32(1), updatedJob.Status.UpgradedClusters)
	assert.Equal(t, int32(0), updatedJob.Status.FailedClusters)
	assert.Equal(t, int32(1), updatedJob.Status.RollbackClusters)
	assert.Equal(t, rlarkv1alpha1.UpgradeJobRollingBack, updatedJob.Status.Phase)
}

func TestReconcile_StartTimeInitialization(t *testing.T) {
	job := &rlarkv1alpha1.UpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0",
		},
		Spec: rlarkv1alpha1.UpgradeJobSpec{
			Image:        "registry.rlark.io/rlarkadm:v0.2.0",
			ClusterNames: []string{"beijing"},
		},
	}

	r := setupTestReconciler(job)

	err := reconcileTwice(r, job.Name)
	require.NoError(t, err)

	// Check that StartTime was initialized
	var updatedJob rlarkv1alpha1.UpgradeJob
	err = r.Get(context.Background(), types.NamespacedName{Name: job.Name}, &updatedJob)
	require.NoError(t, err)

	assert.NotNil(t, updatedJob.Status.StartTime)
}

func TestGetTargetClusters(t *testing.T) {
	tests := []struct {
		name     string
		job      *rlarkv1alpha1.UpgradeJob
		clusters []rlarkv1alpha1.Cluster
		want     []string
	}{
		{
			name: "by cluster names",
			job: &rlarkv1alpha1.UpgradeJob{
				Spec: rlarkv1alpha1.UpgradeJobSpec{
					ClusterNames: []string{"beijing", "shanghai"},
				},
			},
			want: []string{"beijing", "shanghai"},
		},
		{
			name: "by cluster selector",
			job: &rlarkv1alpha1.UpgradeJob{
				Spec: rlarkv1alpha1.UpgradeJobSpec{
					ClusterSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"region": "north",
						},
					},
				},
			},
			clusters: []rlarkv1alpha1.Cluster{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "beijing",
						Labels: map[string]string{"region": "north"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "shanghai",
						Labels: map[string]string{"region": "north"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "guangzhou",
						Labels: map[string]string{"region": "south"},
					},
				},
			},
			want: []string{"beijing", "shanghai"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{}
			for i := range tt.clusters {
				objs = append(objs, &tt.clusters[i])
			}

			r := setupTestReconciler(objs...)
			got, err := r.getTargetClusters(context.Background(), tt.job)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

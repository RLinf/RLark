package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	kubeclientsetfake "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func setupTestGateway(objs ...runtime.Object) *Gateway {
	scheme := runtime.NewScheme()
	_ = rlarkv1alpha1.AddToScheme(scheme)

	kubeClient := kubeclientsetfake.NewSimpleClientset(objs...)

	return &Gateway{
		kubeClient: kubeClient,
		accessors:  registerAccessors(kubeClient),
	}
}

func TestCreateUpgradeJob(t *testing.T) {
	g := setupTestGateway()

	router := gin.Default()
	router.POST("/api/v1/upgrade-jobs", g.rlinfv1alpha1CreateUpgradeJob)

	tests := []struct {
		name           string
		request        rlarkv1alpha1.UpgradeJob
		expectedStatus int
		validate       func(t *testing.T, g *Gateway, name string)
	}{
		{
			name: "create upgrade job with cluster names",
			request: rlarkv1alpha1.UpgradeJob{
				Spec: rlarkv1alpha1.UpgradeJobSpec{
					Image:        "registry.rlark.io/rlarkadm:v0.2.0",
					ClusterNames: []string{"beijing", "shanghai"},
					Strategy: rlarkv1alpha1.UpgradeStrategy{
						Concurrency:     2,
						FailureTolerate: 0.1,
						TimeoutSeconds:  600,
						AutoRollback:    true,
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validate: func(t *testing.T, g *Gateway, name string) {
				job, err := g.kubeClient.RlinfV1alpha1().UpgradeJobs().Get(t.Context(), name, metav1.GetOptions{})
				require.NoError(t, err)
				assert.Equal(t, "registry.rlark.io/rlarkadm:v0.2.0", job.Spec.Image)
				assert.Equal(t, []string{"beijing", "shanghai"}, job.Spec.ClusterNames)
				assert.Equal(t, int32(2), job.Spec.Strategy.Concurrency)
				assert.Equal(t, 0.1, job.Spec.Strategy.FailureTolerate)
				assert.Equal(t, int64(600), job.Spec.Strategy.TimeoutSeconds)
				assert.True(t, job.Spec.Strategy.AutoRollback)
			},
		},
		{
			name: "create upgrade job with cluster selector",
			request: rlarkv1alpha1.UpgradeJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "upgrade-selector",
				},
				Spec: rlarkv1alpha1.UpgradeJobSpec{
					Image: "registry.rlark.io/rlarkadm:v0.2.0",
					ClusterSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"region": "north",
						},
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validate: func(t *testing.T, g *Gateway, name string) {
				job, err := g.kubeClient.RlinfV1alpha1().UpgradeJobs().Get(t.Context(), name, metav1.GetOptions{})
				require.NoError(t, err)
				assert.NotNil(t, job.Spec.ClusterSelector)
				assert.Equal(t, "north", job.Spec.ClusterSelector.MatchLabels["region"])
			},
		},
		{
			name: "create upgrade job with generate name",
			request: rlarkv1alpha1.UpgradeJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "upgrade-generated",
				},
				Spec: rlarkv1alpha1.UpgradeJobSpec{
					Image:        "registry.rlark.io/rlarkadm:v0.2.0",
					ClusterNames: []string{"beijing"},
				},
			},
			expectedStatus: http.StatusCreated,
			validate: func(t *testing.T, g *Gateway, name string) {
				assert.NotEmpty(t, name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.request)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", "/api/v1/upgrade-jobs", bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusCreated {
				var response rlarkv1alpha1.UpgradeJob
				err = json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Equal(t, tt.request.Spec.Image, response.Spec.Image)

				if tt.validate != nil {
					tt.validate(t, g, response.Name)
				}
			}
		})
	}
}

func TestGetUpgradeJob(t *testing.T) {
	job := &rlarkv1alpha1.UpgradeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgrade-v0.2.0",
		},
		Spec: rlarkv1alpha1.UpgradeJobSpec{
			Image:        "registry.rlark.io/rlarkadm:v0.2.0",
			ClusterNames: []string{"beijing"},
		},
		Status: rlarkv1alpha1.UpgradeJobStatus{
			Phase:         rlarkv1alpha1.UpgradeJobUpgrading,
			TotalClusters: 1,
			Conditions: []rlarkv1alpha1.UpgradeJobCondition{
				{
					Cluster:     "beijing",
					Status:      "Succeeded",
					TargetImage: "registry.rlark.io/rlarkadm:v0.2.0",
					UpgradeTime: &metav1.Time{Time: time.Now()},
				},
			},
		},
	}

	g := setupTestGateway(job)

	router := gin.Default()
	router.GET("/api/v1/upgrade-jobs/:name", g.rlinfv1alpha1GetUpgradeJob)

	t.Run("get existing upgrade job", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/v1/upgrade-jobs/upgrade-v0.2.0", nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response rlarkv1alpha1.UpgradeJob
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "upgrade-v0.2.0", response.Name)
		assert.Equal(t, "registry.rlark.io/rlarkadm:v0.2.0", response.Spec.Image)
		assert.Equal(t, rlarkv1alpha1.UpgradeJobUpgrading, response.Status.Phase)
		assert.Equal(t, int32(1), response.Status.TotalClusters)
		assert.Len(t, response.Status.Conditions, 1)
	})

	t.Run("get non-existent upgrade job", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/v1/upgrade-jobs/nonexistent", nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestListUpgradeJobs(t *testing.T) {
	jobs := []rlarkv1alpha1.UpgradeJob{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "upgrade-v0.2.0"},
			Spec: rlarkv1alpha1.UpgradeJobSpec{
				Image:        "registry.rlark.io/rlarkadm:v0.2.0",
				ClusterNames: []string{"beijing"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "upgrade-v0.3.0"},
			Spec: rlarkv1alpha1.UpgradeJobSpec{
				Image:        "registry.rlark.io/rlarkadm:v0.3.0",
				ClusterNames: []string{"shanghai"},
			},
		},
	}

	objs := []runtime.Object{}
	for i := range jobs {
		objs = append(objs, &jobs[i])
	}

	g := setupTestGateway(objs...)

	router := gin.Default()
	router.GET("/api/v1/upgrade-jobs", g.rlinfv1alpha1ListUpgradeJobs)

	t.Run("list all upgrade jobs", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/v1/upgrade-jobs", nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response rlarkv1alpha1.UpgradeJobList
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Len(t, response.Items, 2)
	})
}

func TestDeleteUpgradeJob(t *testing.T) {
	t.Run("delete completed job", func(t *testing.T) {
		job := &rlarkv1alpha1.UpgradeJob{
			ObjectMeta: metav1.ObjectMeta{Name: "upgrade-v0.2.0"},
			Spec: rlarkv1alpha1.UpgradeJobSpec{
				Image:        "registry.rlark.io/rlarkadm:v0.2.0",
				ClusterNames: []string{"beijing"},
			},
			Status: rlarkv1alpha1.UpgradeJobStatus{
				Phase: rlarkv1alpha1.UpgradeJobSucceeded,
			},
		}

		g := setupTestGateway(job)

		router := gin.Default()
		router.DELETE("/api/v1/upgrade-jobs/:name", g.rlinfv1alpha1DeleteUpgradeJob)

		req, err := http.NewRequest("DELETE", "/api/v1/upgrade-jobs/upgrade-v0.2.0", nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify job was deleted
		_, err = g.kubeClient.RlinfV1alpha1().UpgradeJobs().Get(t.Context(), "upgrade-v0.2.0", metav1.GetOptions{})
		assert.Error(t, err)
	})

	t.Run("delete non-existent job", func(t *testing.T) {
		g := setupTestGateway()

		router := gin.Default()
		router.DELETE("/api/v1/upgrade-jobs/:name", g.rlinfv1alpha1DeleteUpgradeJob)

		req, err := http.NewRequest("DELETE", "/api/v1/upgrade-jobs/nonexistent", nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

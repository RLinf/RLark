package task

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes"
	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/binding"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

func assemblyInput(t *testing.T, allowed bool) (KubernetesTaskAssemblyInput, *atomic.Int32) {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews" {
			t.Errorf("Build started unexpected API activity: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected activity", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"apiVersion": "authorization.k8s.io/v1", "kind": "SelfSubjectAccessReview", "status": map[string]any{"allowed": allowed}}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	scheme := executionScheme(t)
	return KubernetesTaskAssemblyInput{
		ManagementClient: fake.NewClientBuilder().WithScheme(scheme).Build(), ManagementNamespace: "management",
		LocalClient: fake.NewClientBuilder().WithScheme(scheme).Build(), LocalConfig: &rest.Config{Host: server.URL},
		Mapper: observationDependencyMapper(), Execution: KubernetesExecutionConfig{Image: "rlark:test", TensorBoardImage: "tensorboard:test", RestartableInitContainers: true},
		PullWorkers: 2, PushWorkers: 3, ResyncPeriod: time.Minute,
	}, requests
}

func TestTaskAssemblyBuildReturnsOnlyCompleteWiring(t *testing.T) {
	input, requests := assemblyInput(t, true)
	a, err := BuildKubernetesTaskAssembly(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	c := a.controller
	if err := c.validateExecutionDependencies(); err != nil {
		t.Fatal(err)
	}
	if err := c.host.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.host.Bundle.Implementations != c.execution.Implementations.(workloadImplementationRegistry) || a.resync.host != c.host || a.resync.namespace != input.ManagementNamespace {
		t.Fatal("Host, Coordinator, observations and resync were not assembled together")
	}
	if c.observationScope.Namespace != tenant.DefaultNamespace || len(a.observers.controllers) != 2 || requests.Load() != 10 {
		t.Fatalf("scope=%v observers=%d preflight=%d", c.observationScope, len(a.observers.controllers), requests.Load())
	}
	if c.PullMaxConcurrentReconciles != 2 || c.PushMaxConcurrentReconciles != 3 || a.setupTried {
		t.Fatal("Build changed worker options or attempted installation")
	}
	input.LocalConfig.Host = "http://changed.invalid"
	input.Execution.Image = "changed"
	if c.LocalKubeConfig.Host == input.LocalConfig.Host || c.Image == input.Execution.Image {
		t.Fatal("assembly retained mutable configuration")
	}
}

func TestTaskAssemblyBuildFailureAndRebuild(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*KubernetesTaskAssemblyInput)
	}{
		{"management client", func(i *KubernetesTaskAssemblyInput) { i.ManagementClient = nil }},
		{"local client", func(i *KubernetesTaskAssemblyInput) { i.LocalClient = nil }},
		{"config", func(i *KubernetesTaskAssemblyInput) { i.LocalConfig = nil }},
		{"mapper", func(i *KubernetesTaskAssemblyInput) { i.Mapper = nil }},
		{"namespace", func(i *KubernetesTaskAssemblyInput) { i.ManagementNamespace = "bad/namespace" }},
		{"pull workers", func(i *KubernetesTaskAssemblyInput) { i.PullWorkers = -1 }},
		{"push workers", func(i *KubernetesTaskAssemblyInput) { i.PushWorkers = -1 }},
		{"resync", func(i *KubernetesTaskAssemblyInput) { i.ResyncPeriod = -time.Second }},
		{"RLark image empty", func(i *KubernetesTaskAssemblyInput) { i.Execution.Image = "" }},
		{"RLark image whitespace", func(i *KubernetesTaskAssemblyInput) { i.Execution.Image = " " }},
		{"TensorBoard image empty", func(i *KubernetesTaskAssemblyInput) { i.Execution.TensorBoardImage = "" }},
		{"TensorBoard image whitespace", func(i *KubernetesTaskAssemblyInput) { i.Execution.TensorBoardImage = " " }},
		{"unmapped root", func(i *KubernetesTaskAssemblyInput) { i.Mapper = meta.NewDefaultRESTMapper(nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, requests := assemblyInput(t, true)
			bad := input
			test.mutate(&bad)
			if a, err := BuildKubernetesTaskAssembly(context.Background(), bad); err == nil || a != nil {
				t.Fatalf("partial assembly returned: %v %v", a, err)
			}
			if requests.Load() != 0 {
				t.Fatal("invalid input performed preflight")
			}
			if _, err := BuildKubernetesTaskAssembly(context.Background(), input); err != nil {
				t.Fatalf("failed Build poisoned retry: %v", err)
			}
		})
	}
	input, requests := assemblyInput(t, false)
	if a, err := BuildKubernetesTaskAssembly(context.Background(), input); err == nil || a != nil || requests.Load() != 1 {
		t.Fatalf("RBAC failure: assembly=%v error=%v requests=%d", a, err, requests.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if a, err := BuildKubernetesTaskAssembly(ctx, input); !errors.Is(err, context.Canceled) || a != nil || requests.Load() != 1 {
		t.Fatalf("cancelled Build: assembly=%v error=%v requests=%d", a, err, requests.Load())
	}
}

func TestTaskAssembliesDoNotShareMutableState(t *testing.T) {
	input, _ := assemblyInput(t, true)
	one, err := BuildKubernetesTaskAssembly(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	two, err := BuildKubernetesTaskAssembly(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if one.controller == two.controller || one.controller.triggers == two.controller.triggers || one.observers.cache == two.observers.cache || one.controller.execution == two.controller.execution || one.controller.host == two.controller.host {
		t.Fatal("independent constructions share runtime state")
	}
}

func TestTaskAssemblyLatePreflightFailureIsRecoverable(t *testing.T) {
	input, _ := assemblyInput(t, true)
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		allowed := requests.Add(1) != 10
		if err := json.NewEncoder(w).Encode(map[string]any{"apiVersion": "authorization.k8s.io/v1", "kind": "SelfSubjectAccessReview", "status": map[string]any{"allowed": allowed}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	input.LocalConfig = &rest.Config{Host: server.URL}
	if a, err := BuildKubernetesTaskAssembly(context.Background(), input); err == nil || a != nil || requests.Load() != 10 {
		t.Fatalf("late preflight failure: assembly=%v err=%v requests=%d", a, err, requests.Load())
	}
	if _, err := BuildKubernetesTaskAssembly(context.Background(), input); err != nil {
		t.Fatalf("late failure poisoned reconstruction: %v", err)
	}
}

func TestObservationBuildFailureDoesNotReserveControllerNames(t *testing.T) {
	input, _ := assemblyInput(t, true)
	a, err := BuildKubernetesTaskAssembly(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	c := a.controller
	plans := []observationWatchPlan{{root: c.observations[0]}, {root: c.observations[0]}}
	if runner, err := c.newObservationRunner(a.observers.cache, *c.observationScope, plans); err == nil || runner != nil {
		t.Fatalf("duplicate source group accepted: %v %v", runner, err)
	}
	if _, err := c.newObservationRunner(a.observers.cache, *c.observationScope, plans[:1]); err != nil {
		t.Fatalf("partial source construction reserved global names: %v", err)
	}
}

func TestTaskAssemblyPreflightHonorsCancellation(t *testing.T) {
	input, _ := assemblyInput(t, true)
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { close(entered); <-release }))
	defer server.Close()
	defer close(release)
	input.LocalConfig = &rest.Config{Host: server.URL}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		a, err := BuildKubernetesTaskAssembly(ctx, input)
		if a != nil {
			result <- errors.New("cancelled Build returned an assembly")
			return
		}
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("preflight did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation chain lost: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("preflight ignored cancellation")
	}
}

type assemblyTestManager struct {
	manager.Manager
	added  []manager.Runnable
	failAt int
	err    error
}

func (m *assemblyTestManager) Add(r manager.Runnable) error {
	m.added = append(m.added, r)
	if len(m.added) == m.failAt {
		return m.err
	}
	return nil
}

func assemblyManagementManager(t *testing.T, input KubernetesTaskAssemblyInput) *assemblyTestManager {
	t.Helper()
	mapper := observationDependencyMapper().(*meta.DefaultRESTMapper)
	mapper.Add(rlarkv1alpha1.GroupVersion.WithKind("Task"), meta.RESTScopeNamespace)
	m, err := ctrl.NewManager(input.LocalConfig, ctrl.Options{
		Scheme: input.ManagementClient.Scheme(), Metrics: metricsserver.Options{BindAddress: "0"},
		Controller:     config.Controller{SkipNameValidation: new(true)},
		MapperProvider: func(*rest.Config, *http.Client) (meta.RESTMapper, error) { return mapper, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return &assemblyTestManager{Manager: m}
}

func TestTaskAssemblySetupIsSingleAttemptEvenAfterFailure(t *testing.T) {
	for _, stage := range []string{"success", "pull", "resync", "observation"} {
		t.Run(stage, func(t *testing.T) {
			input, requests := assemblyInput(t, true)
			a, err := BuildKubernetesTaskAssembly(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			management := assemblyManagementManager(t, input)
			local := &assemblyTestManager{}
			want := errors.New("install failed")
			switch stage {
			case "pull":
				management.failAt, management.err = 1, want
			case "resync":
				management.failAt, management.err = 2, want
			case "observation":
				local.failAt, local.err = 1, want
			}
			err = a.Setup(management, local)
			if stage == "success" {
				if err != nil || len(management.added) != 2 || len(local.added) != 1 {
					t.Fatalf("setup=%v management=%d local=%d", err, len(management.added), len(local.added))
				}
				if management.added[1] != a.resync || local.added[0] != a.observers || !a.resync.NeedLeaderElection() || a.observers.NeedLeaderElection() {
					t.Fatal("runnable ownership/leader election changed")
				}
			} else if !errors.Is(err, want) {
				t.Fatalf("lost setup error: %v", err)
			}
			if (stage == "pull" || stage == "resync") && len(local.added) != 0 {
				t.Fatal("installed observers after management setup failure")
			}
			count := len(management.added) + len(local.added)
			if err := a.Setup(management, local); err == nil || len(management.added)+len(local.added) != count {
				t.Fatal("Setup retried on a partially installed manager")
			}
			if requests.Load() != 10 {
				t.Fatal("Setup performed API activity or started watches")
			}
		})
	}
	var missing *KubernetesTaskAssembly
	if err := missing.Setup(nil, nil); err == nil {
		t.Fatal("nil assembly accepted")
	}
}

func TestTaskAssemblyConcurrentSetupInstallsOnce(t *testing.T) {
	input, _ := assemblyInput(t, true)
	a, err := BuildKubernetesTaskAssembly(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	management, local := assemblyManagementManager(t, input), &assemblyTestManager{}
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { errors <- a.Setup(management, local) })
	}
	wg.Wait()
	successes := 0
	for range 2 {
		if <-errors == nil {
			successes++
		}
	}
	if successes != 1 || len(management.added) != 2 || len(local.added) != 1 {
		t.Fatalf("successes=%d management=%d local=%d", successes, len(management.added), len(local.added))
	}
}

type cancelledAssemblyCache struct {
	failedObservationCache
	joined chan struct{}
}

func (c cancelledAssemblyCache) Start(ctx context.Context) error {
	<-ctx.Done()
	close(c.joined)
	return nil
}

type failingAssemblyObserver struct {
	controller.TypedController[runtimeObservationRequest]
	err error
}

func (c failingAssemblyObserver) Start(context.Context) error { return c.err }

func TestTaskObservationWorkerFailureCancelsAndJoinsCache(t *testing.T) {
	joined := make(chan struct{})
	want := errors.New("observer startup failure")
	r := &observationRunner{
		cache:       cancelledAssemblyCache{joined: joined},
		controllers: []controller.TypedController[runtimeObservationRequest]{failingAssemblyObserver{err: want}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.Start(ctx); !errors.Is(err, want) {
		t.Fatalf("startup error lost: %v", err)
	}
	select {
	case <-joined:
	default:
		t.Fatal("runner returned before cache exited")
	}
}

func TestTaskObservationCancellationJoinsWorkersAndCache(t *testing.T) {
	for _, beforeStart := range []bool{false, true} {
		cacheJoined, workerJoined := make(chan struct{}), make(chan struct{})
		r := &observationRunner{cache: cancelledAssemblyCache{joined: cacheJoined}, controllers: []controller.TypedController[runtimeObservationRequest]{cancelledObservationController{cancelled: workerJoined}}}
		ctx, cancel := context.WithCancel(context.Background())
		if beforeStart {
			cancel()
		}
		result := make(chan error, 1)
		go func() { result <- r.Start(ctx) }()
		cancel()
		cancel()
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("cancel did not join observation group")
		}
		for _, joined := range []chan struct{}{cacheJoined, workerJoined} {
			select {
			case <-joined:
			default:
				t.Fatal("component not joined")
			}
		}
	}
}

func TestTaskAssemblyUsesExplicitExecutionSettings(t *testing.T) {
	input, _ := assemblyInput(t, true)
	t.Setenv(tensorBoardImageEnv, "untrusted-new-default")
	t.Setenv("KUBERNETES_SUPPORT_SIDECAR_FEATURE", "false")
	a, err := BuildKubernetesTaskAssembly(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	binder := a.controller.execution.Binder.(binding.BindingPlanBinder)
	if !binder.Lowerer.(kubernetesruntime.UnitLowerer).RestartableInitContainers {
		t.Fatal("Build ignored explicit lowering setting and read the environment")
	}
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{{Type: taskapi.EnvironmentTypeTensorBoard, Version: taskapi.EnvironmentVersionTensorBoardV1, Parameters: apiextensionsv1.JSON{Raw: []byte(`{"logDir":"/logs"}`)}}}
	snapshot, err := compatibility.DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.controller.execution.Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range result.Plan.Objects {
		if object.Kind.Kind != "Deployment" {
			continue
		}
		var workload appsv1.Deployment
		if err := json.Unmarshal(object.Manifest.Object, &workload); err != nil {
			t.Fatal(err)
		}
		for _, container := range workload.Spec.Template.Spec.InitContainers {
			if container.Name == "tensorboard" {
				if container.Image != input.Execution.TensorBoardImage {
					t.Fatalf("Build read image from environment: %q", container.Image)
				}
				return
			}
		}
	}
	t.Fatal("explicit TensorBoard image was not bound")
}

func TestKubernetesExecutionSettingsPreserveEnvironmentDefaults(t *testing.T) {
	for _, value := range []string{"", "false", "False", "true"} {
		t.Setenv("KUBERNETES_SUPPORT_SIDECAR_FEATURE", value)
		t.Setenv(tensorBoardImageEnv, " ")
		config := KubernetesExecutionConfigFromEnvironment("rlark:test")
		if config.Image != "rlark:test" || config.TensorBoardImage != defaultTensorBoardImage || config.RestartableInitContainers != (value != "false") {
			t.Fatalf("environment %q changed configuration semantics: %#v", value, config)
		}
	}
}

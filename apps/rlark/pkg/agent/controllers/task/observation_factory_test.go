package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/observation"
	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
)

type recordingAccessReviewer struct {
	attrs []authorizationv1.ResourceAttributes
	err   error
}

// setupKubernetesPushController is a standalone observation test seam. It
// bypasses complete Task assembly and its image/capability validation.
func (c *Controller) setupKubernetesPushController(ctx context.Context, mgr ctrl.Manager, scope KubernetesObservationScope, profiles kubernetesruntime.ObservationProfiles) error {
	if mgr == nil || c.LocalKubeClient == nil || c.LocalKubeConfig == nil {
		return fmt.Errorf("kubernetes observation manager, client, and config are required")
	}
	if c.observationScope != nil {
		return fmt.Errorf("kubernetes observation sources are already configured")
	}
	runner, err := c.buildKubernetesObservationRunner(ctx, mgr.GetRESTMapper(), scope, profiles)
	if err != nil {
		return err
	}
	if err := mgr.Add(runner); err != nil {
		return fmt.Errorf("register observation sources: %w", err)
	}
	c.observationScope = &scope
	return nil
}

func (r *recordingAccessReviewer) Review(ctx context.Context, attrs authorizationv1.ResourceAttributes) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.attrs = append(r.attrs, attrs)
	return r.err
}

func observationDependencyMapper() meta.RESTMapper {
	m := meta.NewDefaultRESTMapper([]schema.GroupVersion{appsv1.SchemeGroupVersion, corev1.SchemeGroupVersion})
	for _, kind := range []string{"Deployment", "StatefulSet", "ReplicaSet"} {
		m.Add(appsv1.SchemeGroupVersion.WithKind(kind), meta.RESTScopeNamespace)
	}
	m.Add(corev1.SchemeGroupVersion.WithKind("Pod"), meta.RESTScopeNamespace)
	return m
}

func TestObservationAccessIsExplicitScopedAndDeterministic(t *testing.T) {
	scheme := executionScheme(t)
	mapper := observationDependencyMapper()
	roots, _, err := newKubernetesObservations(scheme, mapper, []observation.Kubernetes{
		{GVK: appsv1.SchemeGroupVersion.WithKind("Deployment"), CarrierType: "deployment", InstanceProfile: kubernetesruntime.ObservationReplicaSetControllerV1},
		{GVK: appsv1.SchemeGroupVersion.WithKind("StatefulSet"), CarrierType: "statefulset", InstanceProfile: kubernetesruntime.ObservationDirectControllerV1},
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := KubernetesObservationScope{RuntimeID: "kubernetes", Namespace: "runtime", ManagementNamespace: "management"}
	first, second := &recordingAccessReviewer{}, &recordingAccessReviewer{}
	plans, err := validateObservationAccess(context.Background(), mapper, first, scope, roots, kubernetesruntime.BuiltinObservationProfiles())
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || len(first.attrs) != 10 {
		t.Fatalf("plans=%d permissions=%v", len(plans), first.attrs)
	}
	for _, attr := range first.attrs {
		if attr.Namespace != "runtime" || attr.Name != "" || attr.Subresource != "" {
			t.Fatalf("unscoped/unexpected permission=%v", attr)
		}
		if attr.Resource == "replicasets" && attr.Verb != "get" {
			t.Fatalf("intermediary must be read directly: %v", attr)
		}
	}
	if _, err := validateObservationAccess(context.Background(), mapper, second, scope, roots, kubernetesruntime.BuiltinObservationProfiles()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.attrs, second.attrs) {
		t.Fatal("permissions are not deterministic")
	}
}

func TestObservationPreflightRejectsInvalidScopeProfileAndPermissions(t *testing.T) {
	scheme := executionScheme(t)
	mapper := observationDependencyMapper()
	root := observation.Kubernetes{GVK: appsv1.SchemeGroupVersion.WithKind("Deployment"), CarrierType: "deployment"}
	for _, tc := range []struct {
		name            string
		scope           KubernetesObservationScope
		profile         string
		permissionError error
	}{
		{name: "empty Runtime", scope: KubernetesObservationScope{Namespace: "runtime", ManagementNamespace: "management"}},
		{name: "all namespaces", scope: KubernetesObservationScope{RuntimeID: "kubernetes", ManagementNamespace: "management"}},
		{name: "invalid management boundary", scope: KubernetesObservationScope{RuntimeID: "kubernetes", Namespace: "runtime", ManagementNamespace: "INVALID"}},
		{name: "unknown profile", scope: KubernetesObservationScope{RuntimeID: "kubernetes", Namespace: "runtime", ManagementNamespace: "management"}, profile: "unknown"},
		{name: "denied", scope: KubernetesObservationScope{RuntimeID: "kubernetes", Namespace: "runtime", ManagementNamespace: "management"}, permissionError: errors.New("denied")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root.InstanceProfile = tc.profile
			roots, _, err := newKubernetesObservations(scheme, mapper, []observation.Kubernetes{root})
			if err != nil {
				t.Fatal(err)
			}
			reviewer := &recordingAccessReviewer{err: tc.permissionError}
			plans, err := validateObservationAccess(context.Background(), mapper, reviewer, tc.scope, roots, kubernetesruntime.BuiltinObservationProfiles())
			if err == nil || plans != nil {
				t.Fatalf("plans=%v error=%v", plans, err)
			}
			if tc.permissionError == nil && len(reviewer.attrs) != 0 {
				t.Fatal("validation performed access reviews before validating all inputs")
			}
		})
	}
}

func TestSelfAccessReviewerRejectsDeniedIndeterminateAndTransportErrors(t *testing.T) {
	for _, status := range []authorizationv1.SubjectAccessReviewStatus{{}, {Allowed: true, Denied: true}, {Allowed: true, EvaluationError: "indeterminate"}, {Allowed: true}} {
		kube := kubernetesfake.NewClientset()
		kube.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authorizationv1.SelfSubjectAccessReview{Status: status}, nil
		})
		err := (selfAccessReviewer{reviews: kube.AuthorizationV1().SelfSubjectAccessReviews()}).Review(context.Background(), authorizationv1.ResourceAttributes{Namespace: "runtime", Verb: "watch", Resource: "pods"})
		wantOK := status.Allowed && !status.Denied && status.EvaluationError == ""
		if (err == nil) != wantOK {
			t.Fatalf("status=%v error=%v", status, err)
		}
	}
	kube := kubernetesfake.NewClientset()
	want := errors.New("transport")
	kube.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) { return true, nil, want })
	if err := (selfAccessReviewer{reviews: kube.AuthorizationV1().SelfSubjectAccessReviews()}).Review(context.Background(), authorizationv1.ResourceAttributes{}); !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
}

func TestObservationCacheDoesNotShareNamespaceConfiguration(t *testing.T) {
	c := NewTaskController(baseControllerForObservationTest(executionScheme(t)))
	one := observationCacheOptions(c, observationDependencyMapper(), KubernetesObservationScope{RuntimeID: "one", Namespace: "first", ManagementNamespace: "management"})
	two := observationCacheOptions(c, observationDependencyMapper(), KubernetesObservationScope{RuntimeID: "two", Namespace: "second", ManagementNamespace: "management"})
	if !one.ReaderFailOnMissingInformer || !two.ReaderFailOnMissingInformer || len(one.DefaultNamespaces) != 1 || len(two.DefaultNamespaces) != 1 {
		t.Fatal("unbounded observation cache options")
	}
	if _, exists := two.DefaultNamespaces["first"]; exists {
		t.Fatal("cache namespace leaked across Runtime factories")
	}
	delete(one.DefaultNamespaces, "first")
	if len(two.DefaultNamespaces) != 1 {
		t.Fatal("cache options share mutable namespace maps")
	}
	if err := c.SetupPushController(nil); err == nil {
		t.Fatal("unscoped Task setup accepted")
	}
}

func TestScopedCacheMissingInformerFailsWithoutAPIFallback(t *testing.T) {
	c, r, _, root := observationFixture(t, nil)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, "unexpected API fallback", 500)
	}))
	defer server.Close()
	mapper := observationDependencyMapper()
	mapper.(*meta.DefaultRESTMapper).Add(trainingGVK, meta.RESTScopeNamespace)
	observed, err := cache.New(&rest.Config{Host: server.URL}, observationCacheOptions(c, mapper, r.push.scope))
	if err != nil {
		t.Fatal(err)
	}
	r.push.observedReader = observed
	_, err = r.Reconcile(context.Background(), runtimeObservationRequest{Key: client.ObjectKeyFromObject(root)})
	var missing *cache.ErrResourceNotCached
	if !errors.As(err, &missing) || missing.GVK != trainingGVK {
		t.Fatalf("error=%v", err)
	}
	if requests != 0 || len(c.triggers.events) != 0 {
		t.Fatalf("API calls=%d hints=%d", requests, len(c.triggers.events))
	}
}

type observationTestManager struct {
	manager.Manager
	mapper   meta.RESTMapper
	addError error
	added    int
}

func (m *observationTestManager) GetRESTMapper() meta.RESTMapper { return m.mapper }
func (m *observationTestManager) Add(manager.Runnable) error     { m.added++; return m.addError }

func TestObservationSetupNeverRegistersPartialObservers(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "RBAC denied", true: "manager registration failed"}[allowed], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path != "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews" || req.Method != http.MethodPost {
					t.Errorf("unexpected startup request: %s %s", req.Method, req.URL.Path)
					http.Error(w, "unexpected", 500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]any{"apiVersion": "authorization.k8s.io/v1", "kind": "SelfSubjectAccessReview", "status": map[string]any{"allowed": allowed}}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			c, _, _, _ := observationFixture(t, nil)
			c.observationScope = nil
			c.LocalKubeConfig = &rest.Config{Host: server.URL}
			mapper := observationDependencyMapper()
			mapper.(*meta.DefaultRESTMapper).Add(trainingGVK, meta.RESTScopeNamespace)
			want := errors.New("cannot register")
			m := &observationTestManager{mapper: mapper, addError: want}
			scope := KubernetesObservationScope{RuntimeID: "kubernetes", Namespace: "failed-setup", ManagementNamespace: "management"}
			err := c.setupKubernetesPushController(context.Background(), m, scope, kubernetesruntime.BuiltinObservationProfiles())
			if err == nil || c.observationScope != nil {
				t.Fatalf("failed setup installed observer scope: error=%v", err)
			}
			if !allowed && m.added != 0 {
				t.Fatal("RBAC failure registered a runnable")
			}
			if allowed && (!errors.Is(err, want) || m.added != 1) {
				t.Fatalf("registration not atomic: added=%d error=%v", m.added, err)
			}
		})
	}
}

type failedObservationCache struct {
	cache.Cache
	err error
}

func (c failedObservationCache) Start(context.Context) error { return c.err }

type cancelledObservationController struct {
	controller.TypedController[runtimeObservationRequest]
	cancelled chan struct{}
}

func (c cancelledObservationController) Start(ctx context.Context) error {
	<-ctx.Done()
	close(c.cancelled)
	return nil
}

func TestObservationRunnerCancelsAllWorkersOnStartupFailure(t *testing.T) {
	want := errors.New("cache startup failed")
	worker := cancelledObservationController{cancelled: make(chan struct{})}
	r := &observationRunner{cache: failedObservationCache{err: want}, controllers: []controller.TypedController[runtimeObservationRequest]{worker}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Start(ctx); !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
	select {
	case <-worker.cancelled:
	default:
		t.Fatal("worker was left running after startup failure")
	}
}

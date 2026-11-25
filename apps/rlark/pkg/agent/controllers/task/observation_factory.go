package task

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/source"

	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// KubernetesObservationScope is trusted placement configuration, not Adapter
// metadata or a tenant ID. Each factory owns one Runtime ID and namespace.
type KubernetesObservationScope struct {
	RuntimeID           agentruntime.ID
	Namespace           string
	ManagementNamespace string
}

// SetupPushController rejects the unscoped base-controller path for Tasks.
// Task observers must use BuildKubernetesTaskAssembly with explicit scope.
func (c *Controller) SetupPushController(_ any) error {
	return fmt.Errorf("task observation requires BuildKubernetesTaskAssembly with explicit Runtime and namespace scope")
}

func (s KubernetesObservationScope) validate() error {
	if strings.TrimSpace(string(s.RuntimeID)) == "" || strings.TrimSpace(string(s.RuntimeID)) != string(s.RuntimeID) {
		return fmt.Errorf("observation Runtime ID is required")
	}
	for _, ns := range []string{s.Namespace, s.ManagementNamespace} {
		if len(validation.IsDNS1123Label(ns)) != 0 {
			return fmt.Errorf("observation namespace %q is invalid or empty", ns)
		}
	}
	return nil
}

type accessReviewer interface {
	Review(context.Context, authorizationv1.ResourceAttributes) error
}

type selfAccessReviewer struct {
	reviews authorizationclient.SelfSubjectAccessReviewInterface
}

func (r selfAccessReviewer) Review(ctx context.Context, attrs authorizationv1.ResourceAttributes) error {
	response, err := r.reviews.Create(ctx, &authorizationv1.SelfSubjectAccessReview{Spec: authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &attrs}}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("review %s %s/%s in %s: %w", attrs.Verb, attrs.Group, attrs.Resource, attrs.Namespace, err)
	}
	if !response.Status.Allowed || response.Status.Denied || response.Status.EvaluationError != "" {
		return fmt.Errorf("observation permission denied or indeterminate: %s %s/%s in %s", attrs.Verb, attrs.Group, attrs.Resource, attrs.Namespace)
	}
	return nil
}

type observationWatchPlan struct {
	root    kubernetesObservation
	profile *kubernetesruntime.ObservationProfile
}

// validateObservationAccess validates every profile and mapping before performing
// any access review. No informer, controller, or goroutine exists at this point.
func validateObservationAccess(ctx context.Context, mapper meta.RESTMapper, reviewer accessReviewer, scope KubernetesObservationScope, roots []kubernetesObservation, profiles kubernetesruntime.ObservationProfiles) ([]observationWatchPlan, error) {
	if err := scope.validate(); err != nil {
		return nil, err
	}
	if reviewer == nil || mapper == nil || len(roots) == 0 {
		return nil, fmt.Errorf("observation roots, REST mapper, and access reviewer are required")
	}
	permissions := map[authorizationv1.ResourceAttributes]bool{}
	add := func(gvk schema.GroupVersionKind, verbs ...string) error {
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return fmt.Errorf("map observation dependency %s: %w", gvk, err)
		}
		if mapping.Scope == nil || mapping.Scope.Name() != meta.RESTScopeNameNamespace {
			return fmt.Errorf("observation dependency %s must be namespaced", gvk)
		}
		for _, verb := range verbs {
			permissions[authorizationv1.ResourceAttributes{Namespace: scope.Namespace, Verb: verb, Group: mapping.Resource.Group, Version: mapping.Resource.Version, Resource: mapping.Resource.Resource}] = true
		}
		return nil
	}
	plans := make([]observationWatchPlan, 0, len(roots))
	for _, root := range roots {
		if err := add(root.GVK, "get", "list", "watch"); err != nil {
			return nil, err
		}
		plan := observationWatchPlan{root: root}
		if root.InstanceProfile != "" {
			profile, err := profiles.Resolve(root.InstanceProfile, root.GVK)
			if err != nil {
				return nil, err
			}
			plan.profile = &profile
			if err := add(corev1.SchemeGroupVersion.WithKind("Pod"), "get", "list", "watch"); err != nil {
				return nil, err
			}
			for _, gvk := range profile.Intermediates {
				if err := add(gvk, "get"); err != nil {
					return nil, err
				}
			}
		}
		plans = append(plans, plan)
	}
	attrs := make([]authorizationv1.ResourceAttributes, 0, len(permissions))
	for permission := range permissions {
		attrs = append(attrs, permission)
	}
	sort.Slice(attrs, func(i, j int) bool { return fmt.Sprint(attrs[i]) < fmt.Sprint(attrs[j]) })
	for _, permission := range attrs {
		if err := reviewer.Review(ctx, permission); err != nil {
			return nil, err
		}
	}
	return plans, nil
}

func observationCacheOptions(c *Controller, mapper meta.RESTMapper, scope KubernetesObservationScope) cache.Options {
	return cache.Options{
		Scheme: c.LocalKubeClient.Scheme(), Mapper: mapper,
		DefaultNamespaces:           map[string]cache.Config{scope.Namespace: {}},
		ReaderFailOnMissingInformer: true,
	}
}

// buildKubernetesObservationRunner performs preflight and constructs sources
// without registering them on a manager or starting any background activity.
func (c *Controller) buildKubernetesObservationRunner(ctx context.Context, mapper meta.RESTMapper, scope KubernetesObservationScope, profiles kubernetesruntime.ObservationProfiles) (*observationRunner, error) {
	if c.LocalKubeClient == nil || c.LocalKubeConfig == nil || mapper == nil {
		return nil, fmt.Errorf("kubernetes observation client, config, and mapper are required")
	}
	auth, err := authorizationclient.NewForConfig(c.LocalKubeConfig)
	if err != nil {
		return nil, fmt.Errorf("create observation access reviewer: %w", err)
	}
	preflightCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	plans, err := validateObservationAccess(preflightCtx, mapper, selfAccessReviewer{reviews: auth.SelfSubjectAccessReviews()}, scope, c.observations, profiles)
	if err != nil {
		return nil, err
	}
	scopedCache, err := cache.New(c.LocalKubeConfig, observationCacheOptions(c, mapper, scope))
	if err != nil {
		return nil, fmt.Errorf("create scoped observation cache: %w", err)
	}
	return c.newObservationRunner(scopedCache, scope, plans)
}

type observationRunner struct {
	cache       cache.Cache
	controllers []controller.TypedController[runtimeObservationRequest]
}

func (r *observationRunner) NeedLeaderElection() bool { return false }

func (r *observationRunner) Start(ctx context.Context) error {
	eg, runCtx := errgroup.WithContext(ctx)
	eg.Go(func() error { return r.cache.Start(runCtx) })
	for _, observer := range r.controllers {
		eg.Go(func() error { return observer.Start(runCtx) })
	}
	return eg.Wait()
}

func (c *Controller) newObservationRunner(scopedCache cache.Cache, scope KubernetesObservationScope, plans []observationWatchPlan) (*observationRunner, error) {
	runner := &observationRunner{cache: scopedCache}
	names := make(map[string]struct{}, len(plans))
	for _, plan := range plans {
		push, err := newPushRuntimeReconciler(c, plan.root, scope, scopedCache)
		if err != nil {
			return nil, err
		}
		r := &runtimeObservationReconciler{push: *push, profile: plan.profile}
		// Unmanaged construction must not reserve names in controller-runtime's
		// irreversible process-global registry. Validate this group's names here;
		// the owning Controller/assembly permits only one installation.
		options := controller.TypedOptions[runtimeObservationRequest]{Reconciler: r, MaxConcurrentReconciles: c.PushMaxConcurrentReconciles, ReconciliationTimeout: 30 * time.Second, SkipNameValidation: new(true)}
		// Separate scoped factories may observe the same GVK. Controller names must
		// still be unique without embedding unconstrained Runtime IDs in metrics.
		scopeHash := sha256.Sum256([]byte(string(scope.RuntimeID) + "\x00" + scope.Namespace + "\x00" + scope.ManagementNamespace))
		name := fmt.Sprintf("%s-%x-push", plan.root.name, scopeHash[:6])
		if _, exists := names[name]; exists {
			return nil, fmt.Errorf("duplicate observation controller %q", name)
		}
		names[name] = struct{}{}
		observer, err := controller.NewTypedUnmanaged(name, options)
		if err != nil {
			return nil, fmt.Errorf("build observation controller: %w", err)
		}
		if err := observer.Watch(source.TypedKind(scopedCache, plan.root.typeObject, runtimeRootHandler(scope))); err != nil {
			return nil, err
		}
		if plan.profile != nil {
			if err := observer.Watch(source.TypedKind(scopedCache, client.Object(&corev1.Pod{}), runtimePodHandler(scope))); err != nil {
				return nil, err
			}
		}
		runner.controllers = append(runner.controllers, observer)
	}
	return runner, nil
}

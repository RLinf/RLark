package replication

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
)

const resyncInterval = 5 * time.Minute

type Reconciler struct {
	client.Client
	Engine *distribution.Engine
}

func New(config *rest.Config, declarationClient client.Client) (*Reconciler, error) {
	targetClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}
	mode := distribution.ReplicationMode
	return &Reconciler{
		Client: declarationClient,
		Engine: &distribution.Engine{
			DeclarationClient: declarationClient,
			TargetClient:      targetClient,
			TargetMapper:      restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient)),
			Namespaces:        namespaceResolver{client: declarationClient},
			InventoryStore:    distribution.ConfigMapInventoryStore{Client: declarationClient, Mode: mode},
			Policy:            restrictedPolicy{},
			Mode:              mode,
			MaxTargets:        1000,
		},
	}, nil
}

func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	declaration := &corev1.Secret{}
	if err := r.Get(ctx, request.NamespacedName, declaration); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if declaration.Type != distribution.ReplicationSecretType {
		return ctrl.Result{}, nil
	}
	result, err := r.Engine.Reconcile(ctx, declaration)
	if err != nil {
		return ctrl.Result{}, err
	}
	if result.Requeue {
		return ctrl.Result{RequeueAfter: time.Nanosecond}, nil
	}
	return ctrl.Result{RequeueAfter: resyncInterval}, nil
}

func (r *Reconciler) SetupWithManager(manager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(manager).
		Named("resource-replication").
		For(&corev1.Secret{}, builder.WithPredicates(secretPredicate(distribution.ReplicationSecretType))).
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.mapNamespace)).
		Complete(r)
}

func (r *Reconciler) mapNamespace(ctx context.Context, _ client.Object) []reconcile.Request {
	var secrets corev1.SecretList
	if err := r.List(ctx, &secrets); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0)
	for i := range secrets.Items {
		if secrets.Items[i].Type == distribution.ReplicationSecretType {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: secrets.Items[i].Namespace, Name: secrets.Items[i].Name}})
		}
	}
	return requests
}

type namespaceResolver struct{ client client.Client }

func (r namespaceResolver) List(ctx context.Context) ([]distribution.NamespaceIdentity, error) {
	var namespaces corev1.NamespaceList
	if err := r.client.List(ctx, &namespaces); err != nil {
		return nil, err
	}
	result := make([]distribution.NamespaceIdentity, 0, len(namespaces.Items))
	for _, namespace := range namespaces.Items {
		result = append(result, distribution.NamespaceIdentity{Name: namespace.Name, UID: namespace.UID, Labels: namespace.Labels, Phase: namespace.Status.Phase})
	}
	return result, nil
}

func secretPredicate(secretType corev1.SecretType) predicate.Predicate {
	matches := func(object client.Object) bool {
		secret, ok := object.(*corev1.Secret)
		return ok && secret.Type == secretType
	}
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return matches(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return matches(e.ObjectOld) || matches(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return matches(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return matches(e.Object) },
	}
}

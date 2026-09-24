package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
)

const resyncInterval = 5 * time.Minute

type Reconciler struct {
	managementClient    client.Client
	managementNamespace string
	engine              *distribution.Engine
	reconcileDelivery   func(context.Context, ctrl.Request) (ctrl.Result, error)
}

func New(config *rest.Config, managementClient, localClient client.Client, managementNamespace string) (*Reconciler, error) {
	targetClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}
	mode := distribution.DeliveryMode
	return &Reconciler{
		managementClient:    managementClient,
		managementNamespace: managementNamespace,
		engine: &distribution.Engine{
			DeclarationClient: managementClient,
			TargetClient:      targetClient,
			TargetMapper:      restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient)),
			Namespaces:        namespaceResolver{client: localClient},
			InventoryStore:    distribution.ConfigMapInventoryStore{Client: managementClient, Mode: mode},
			Policy:            restrictedPolicy{},
			Mode:              mode,
			MaxTargets:        1000,
		},
	}, nil
}

func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	if r.reconcileDelivery != nil {
		return r.reconcileDelivery(ctx, request)
	}
	if request.Namespace != r.managementNamespace {
		return ctrl.Result{}, nil
	}
	declaration := &corev1.Secret{}
	if err := r.managementClient.Get(ctx, request.NamespacedName, declaration); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if declaration.Type != distribution.DeliverySecretType {
		return ctrl.Result{}, nil
	}
	result, err := r.engine.Reconcile(ctx, declaration)
	if err != nil {
		return ctrl.Result{}, err
	}
	if result.Requeue {
		return ctrl.Result{RequeueAfter: time.Nanosecond}, nil
	}
	return ctrl.Result{RequeueAfter: resyncInterval}, nil
}

func (r *Reconciler) Setup(managementManager, localManager ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(managementManager).
		Named("resource-delivery").
		For(&corev1.Secret{}, builder.WithPredicates(secretPredicate())).
		Complete(r); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(localManager).
		Named("resource-delivery-namespace-watch").
		For(&corev1.Namespace{}, builder.WithPredicates(predicate.Funcs{})).
		Complete(reconcile.Func(r.reconcileNamespace))
}

func (r *Reconciler) reconcileNamespace(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	var secrets corev1.SecretList
	if err := r.managementClient.List(ctx, &secrets, client.InNamespace(r.managementNamespace)); err != nil {
		return reconcile.Result{}, err
	}
	var reconcileErrors []error
	for i := range secrets.Items {
		if secrets.Items[i].Type != distribution.DeliverySecretType {
			continue
		}
		if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: r.managementNamespace, Name: secrets.Items[i].Name}}); err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("reconcile delivery %s: %w", secrets.Items[i].Name, err))
		}
	}
	return reconcile.Result{}, errors.Join(reconcileErrors...)
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

func secretPredicate() predicate.Predicate {
	matches := func(object client.Object) bool {
		secret, ok := object.(*corev1.Secret)
		return ok && secret.Type == distribution.DeliverySecretType
	}
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return matches(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return matches(e.ObjectOld) || matches(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return matches(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return matches(e.Object) },
	}
}

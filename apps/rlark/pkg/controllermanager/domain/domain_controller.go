package domain

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const (
	podDomainField = "spec.domain"
	// domainPeerDomainField indexes DomainPeer CRs by the Domain they belong
	// to, so the controller never has to list every DomainPeer in the cluster
	// and filter in memory.
	domainPeerDomainField = "rlinf.io/domain-owner"
)

// domainPeerLabel is the canonical, stable label recording which Domain a
// DomainPeer belongs to. Before it existed, membership was only expressed by
// naming the DomainPeer after its Domain.
const domainPeerLabel = "rlinf.io/domain"

// Reason values for the Domain Validated condition. They are stable, CamelCase
// identifiers consumed by clients.
const (
	domainReasonCIDRValid   = "CIDRValid"
	domainReasonCIDRInvalid = "InvalidCIDR"
)

// Reason codes this reconciler attaches to a classified pass. Closed set: they
// are used as a metric label.
const (
	reasonDomainSynced     = "DomainSynced"
	reasonPeersDeleted     = "DomainPeersDeleted"
	reasonPeerWriteFailed  = "DomainPeerWriteFailed"
	reasonPodListFailed    = "PodListFailed"
	reasonIPPoolExhausted  = "IPPoolExhausted"
	reasonTaskLookupFailed = "TaskLookupFailed"
)

// Reconciler watches Domain and Pod CRs, and generates DomainPeer
// (one per cluster/namespace per domain) containing the pod list.
//
// DomainPeer is namespaced: each cluster (represented by a namespace) has its
// own DomainPeer per domain, named after the domain. The DomainPeer.Spec.Pods
// field records all pods belonging to that domain in that cluster.
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int

	// Kubernetes client configuration.
	KubeClientConfig configs.KubernetesClientConfig
	// Server Address
	ServerAddress string

	// Shell classifies the pass, records it, and emits events. The zero value
	// is silent, so a test may leave it unset.
	Shell controller.Shell
}

// +kubebuilder:rbac:groups=rlinf.io,resources=domains,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=domains/finalizers,verbs=update
// +kubebuilder:rbac:groups=rlinf.io,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks,verbs=get;list;watch
// +kubebuilder:rbac:groups=rlinf.io,resources=domainpeers,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles a Domain reconciliation request.
// Triggered by Domain changes or Pod CR changes (mapped to the associated domain).
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Shell.Run(ctx, "domain", func(ctx context.Context) (controller.Result, error) {
		return r.reconcile(ctx, req)
	})
}

func (r *Reconciler) reconcile(ctx context.Context, req ctrl.Request) (controller.Result, error) {
	logger := log.FromContext(ctx).WithValues("domain", req.Name)

	// 1. Get Domain
	var domain rlarkv1alpha1.Domain
	if err := r.Get(ctx, types.NamespacedName{Name: req.Name}, &domain); err != nil {
		if apierrors.IsNotFound(err) {
			// Domain deleted — delete all DomainPeers for this domain
			if err := r.deleteDomainPeers(ctx, logger, req.Name); err != nil {
				return controller.Result{Reason: reasonPeerWriteFailed}, err
			}
			return controller.Result{
				Outcome: sharedreconcile.OutcomeDropped,
				Reason:  reasonPeersDeleted,
				Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
			}, nil
		}
		return controller.Result{Reason: controller.ReasonReconcileFailed}, err
	}

	// Deletion is decided before any desired-state work: a Domain being
	// deleted has no desired state left to converge on, and reconciling one
	// would recreate the very peers cleanup is removing.
	if !domain.DeletionTimestamp.IsZero() {
		return r.reconcileDeletion(ctx, logger, &domain)
	}
	// Claim the Domain before the first DomainPeer is written, so a delete
	// request can never win a race against peer creation.
	if err := r.claimDomain(ctx, &domain); err != nil {
		return controller.Result{Reason: reasonPeerCleanupFinalizerAdded}, err
	}

	// Snapshot after the claim and before the pass edits Status in place, so
	// the status write patches exactly what changed instead of replaying the
	// finalizer update.
	before := domain.DeepCopy()

	ippool, err := NewIPPool(domain.Spec.CIDR)
	if err != nil {
		// An invalid Spec cannot be fixed by retrying, so record the conclusion
		// in the Validated condition, surface it to the user as an event, and
		// stop reconciling until the Spec changes.
		logger.Info("invalid CIDR in Domain spec, blocking until the Spec changes",
			"cidr", domain.Spec.CIDR, "error", err.Error())
		r.Shell.Warnf(&domain, domainReasonCIDRInvalid, "Invalid CIDR %q: %v", domain.Spec.CIDR, err)
		if writeErr := r.markDomainInvalid(ctx, &domain, before, err.Error()); writeErr != nil {
			return statusWriteFailure(writeErr), writeErr
		}
		return controller.Result{
			Outcome: sharedreconcile.OutcomeBlocked,
			Reason:  domainReasonCIDRInvalid,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange},
		}, nil
	}

	// 2. List Pod CRs belonging to this domain through the cache index.
	var podList rlarkv1alpha1.PodList
	if err := r.List(ctx, &podList, client.MatchingFields{podDomainField: domain.Name}); err != nil {
		logger.Error(err, "failed to list Pods")
		return controller.Result{Reason: reasonPodListFailed}, err
	}

	// 3. Generate IP Allocation Map for the domain.
	// Keep the first allocation per pod key so that a pod reclaims its original
	// IP after a restart (later duplicate records are ignored).
	oldIPAllocMap := make(map[string]rlarkv1alpha1.DomainIPAllocation)
	for _, ipAlloc := range domain.Status.IPAllocations {
		if _, exists := oldIPAllocMap[ipAlloc.Pod]; !exists {
			oldIPAllocMap[ipAlloc.Pod] = ipAlloc
		}
	}

	// 4. Group pods by namespace (each namespace = one cluster).
	// Terminal and not-yet-scheduled pods are skipped. A newly created
	// management Pod receives status in a second API write; waiting for Node and
	// local IP avoids allocating an address and rewriting every DomainPeer twice.
	// Terminal pods' UID-named CRs may
	// coexist with the recreated pod's CR, and processing both would produce
	// duplicate entries and incorrect IP allocation. Their namespaces remain
	// known, however, so a transient container restart does not delete the
	// DomainPeer and break the still-running network sidecar.
	podsByNamespace := make(map[string][]rlarkv1alpha1.DomainPodInfo)
	nonAllocPodsByNamespace := make(map[string][]rlarkv1alpha1.DomainPodInfo)
	knownNamespaces := make(map[string]struct{})
	reusedIPs := make(map[string]bool)     // IPs already reclaimed in this pass
	reusedAlloc := make(map[string]string) // podKey -> reclaimed IP
	podTaskRefs := make(map[string]podTaskRef)
	for _, pod := range podList.Items {
		if pod.Spec.Domain != domain.Name {
			continue
		}
		knownNamespaces[pod.Namespace] = struct{}{}
		// Record the Task coordinates ahead of the state filters below. They
		// are identity, not observed state, so they stay usable for an
		// allocation retained from an earlier pass whose Pod has since gone
		// terminal.
		podTaskRefs[pod.Namespace+"/"+pod.Spec.PodNamespace+"/"+pod.Spec.PodName] = podTaskRef{
			namespace: pod.Spec.TaskNamespace,
			name:      pod.Spec.TaskName,
		}
		if pod.Status.Phase == rlarkv1alpha1.PodPhaseSucceeded || pod.Status.Phase == rlarkv1alpha1.PodPhaseFailed {
			continue
		}
		if pod.Status.Node == "" || pod.Status.IP == "" {
			continue
		}
		ns := pod.Namespace
		podKey := ns + "/" + pod.Spec.PodNamespace + "/" + pod.Spec.PodName
		if alloc, ok := oldIPAllocMap[podKey]; ok && !reusedIPs[alloc.IP] {
			reusedIPs[alloc.IP] = true
			reusedAlloc[podKey] = alloc.IP
			podsByNamespace[ns] = append(podsByNamespace[ns], rlarkv1alpha1.DomainPodInfo{
				GlobalNamespace: ns,
				Namespace:       pod.Spec.PodNamespace,
				Name:            pod.Spec.PodName,
				UID:             pod.Name, // 控制面的 Name 即为数据面的 UID
				Node:            pod.Status.Node,
				IP:              alloc.IP,
				LocalIP:         pod.Status.IP,
			})
			ippool.MarkAllocated(alloc.IP) // mark IP as allocated
			delete(oldIPAllocMap, podKey)  // mark as allocated
		} else {
			nonAllocPodsByNamespace[ns] = append(nonAllocPodsByNamespace[ns], rlarkv1alpha1.DomainPodInfo{
				GlobalNamespace: ns,
				Namespace:       pod.Spec.PodNamespace,
				Name:            pod.Spec.PodName,
				UID:             pod.Name, // 控制面的 Name 即为数据面的 UID
				Node:            pod.Status.Node,
				IP:              "",
				LocalIP:         pod.Status.IP,
			})
		}
	}

	// 5. Rebuild IPAllocations: keep reused ones, drop conflicting/duplicate/expired ones
	// (keep 128 expired IPs for potential reuse).
	var newIPAllocations []rlarkv1alpha1.DomainIPAllocation
	seenPods := make(map[string]bool)
	expireIPCount := len(oldIPAllocMap) - 128
	if expireIPCount < 0 {
		expireIPCount = 0
	}
	for _, ipAlloc := range domain.Status.IPAllocations {
		if seenPods[ipAlloc.Pod] {
			continue // skip duplicate IP allocations
		}
		seenPods[ipAlloc.Pod] = true

		if reusedIP, ok := reusedAlloc[ipAlloc.Pod]; ok {
			// Pod was reused: keep only the allocation matching the reclaimed IP.
			if ipAlloc.IP == reusedIP {
				newIPAllocations = append(newIPAllocations, ipAlloc)
			}
			continue
		}

		// Pod was not reused (terminal or absent). Drop if its IP was reclaimed
		// by another pod (conflict).
		if reusedIPs[ipAlloc.IP] {
			continue
		}
		if expireIPCount > 0 {
			expireIPCount--
			continue
		}
		newIPAllocations = append(newIPAllocations, ipAlloc)
		ippool.MarkAllocated(ipAlloc.IP) // prevent reallocation of retained IPs
	}
	domain.Status.IPAllocations = newIPAllocations

	// 6. Allocate IPs for pods that don't have an allocation yet
	for ns, pods := range nonAllocPodsByNamespace {
		for _, pod := range pods {
			ip, err := ippool.Allocate()
			if err != nil {
				// The pool is sized by the Spec CIDR, so retrying the same
				// input cannot help until pods release addresses or the Spec
				// widens the pool.
				logger.Error(err, "failed to allocate IP for pod", "namespace", ns, "pod", pod.Name)
				r.Shell.Warnf(&domain, reasonIPPoolExhausted,
					"Cannot allocate an IP for pod %s/%s: %v", ns, pod.Name, err)
				return controller.Result{Reason: reasonIPPoolExhausted}, err
			}
			pod.IP = ip
			podsByNamespace[ns] = append(podsByNamespace[ns], pod)
			domain.Status.IPAllocations = append(domain.Status.IPAllocations, rlarkv1alpha1.DomainIPAllocation{
				IP:  ip,
				Pod: ns + "/" + pod.Namespace + "/" + pod.Name,
			})
		}
	}

	// 7. Resolve the Job and Task an allocation belongs to, so the list can be
	// read along those dimensions instead of by Pod alone.
	if err := resolveAllocationOwners(ctx, newTaskJobResolver(r.Client),
		domain.Status.IPAllocations, podTaskRefs); err != nil {
		// Identity resolution reads Tasks the Job controller owns, so a failure
		// here is transient. Returning before the Status write keeps the pass
		// all-or-nothing: the next pass recomputes from the same input.
		logger.Error(err, "failed to resolve the Job owning an allocated pod")
		return controller.Result{Reason: reasonTaskLookupFailed}, err
	}
	sort.Slice(domain.Status.IPAllocations, func(i, j int) bool {
		return domain.Status.IPAllocations[i].Pod < domain.Status.IPAllocations[j].Pod
	})

	// 8. Update Domain.Status with the new allocations and the observed generation
	statusChanged := !reflect.DeepEqual(before.Status.IPAllocations, domain.Status.IPAllocations)
	if domain.Status.ObservedGeneration != domain.Generation {
		domain.Status.ObservedGeneration = domain.Generation
		statusChanged = true
	}
	if apimeta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             domainReasonCIDRValid,
		ObservedGeneration: domain.Generation,
	}) {
		statusChanged = true
	}
	if statusChanged {
		if err := r.Status().Patch(ctx, &domain, client.MergeFrom(before)); err != nil {
			logger.Error(err, "failed to patch Domain status")
			return statusWriteFailure(err), err
		}
	}

	// 9. Create or update DomainPeer per namespace
	signer := newSigner(r)
	allPods := make([]rlarkv1alpha1.DomainPodInfo, 0)
	for _, pods := range podsByNamespace {
		allPods = append(allPods, pods...)
	}
	sort.Slice(allPods, func(i, j int) bool {
		if allPods[i].GlobalNamespace != allPods[j].GlobalNamespace {
			return allPods[i].GlobalNamespace < allPods[j].GlobalNamespace
		}
		if allPods[i].Namespace != allPods[j].Namespace {
			return allPods[i].Namespace < allPods[j].Namespace
		}
		return allPods[i].Name < allPods[j].Name
	})
	for ns := range podsByNamespace {
		if err := r.createOrUpdateDomainPeer(ctx, logger, &domain, ns, allPods, signer, ippool.PrefixLength()); err != nil {
			return controller.Result{Reason: reasonPeerWriteFailed}, err
		}
	}

	// 10. Delete DomainPeers in namespaces that no longer have pods for this domain
	if err := r.cleanupStaleDomainPeers(ctx, logger, &domain, knownNamespaces); err != nil {
		return controller.Result{Reason: reasonPeerWriteFailed}, err
	}

	return controller.Result{
		Outcome: sharedreconcile.OutcomeSucceeded,
		Reason:  reasonDomainSynced,
		Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange},
	}, nil
}

// statusWriteFailure classifies a failed status write. A conflict is the
// expected outcome of losing a race against another writer and is kept distinct
// from a genuine write failure.
func statusWriteFailure(err error) controller.Result {
	if apierrors.IsConflict(err) {
		return controller.Result{Reason: controller.ReasonStatusWriteConflict}
	}
	return controller.Result{Reason: controller.ReasonStatusWriteFailed}
}

// markDomainInvalid records an invalid Spec in the Validated condition. An
// invalid Spec cannot be fixed by retrying, so the caller stops reconciling and
// waits for a Spec change instead of requeueing forever.
func (r *Reconciler) markDomainInvalid(ctx context.Context, domain, before *rlarkv1alpha1.Domain, message string) error {
	changed := apimeta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeValidated,
		Status:             metav1.ConditionFalse,
		Reason:             domainReasonCIDRInvalid,
		Message:            message,
		ObservedGeneration: domain.Generation,
	})
	if domain.Status.ObservedGeneration != domain.Generation {
		domain.Status.ObservedGeneration = domain.Generation
		changed = true
	}
	if !changed {
		return nil
	}
	return r.Status().Patch(ctx, domain, client.MergeFrom(before))
}

// SetupWithManager registers the controller with the manager.
// It watches Domain as the primary resource and Pod as a secondary resource
// (pod changes trigger reconciliation of the associated domain).
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &rlarkv1alpha1.Pod{}, podDomainField, func(obj client.Object) []string {
		pod, ok := obj.(*rlarkv1alpha1.Pod)
		if !ok || pod.Spec.Domain == "" {
			return nil
		}
		return []string{pod.Spec.Domain}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &rlarkv1alpha1.DomainPeer{},
		domainPeerDomainField, indexDomainPeerByDomain); err != nil {
		return fmt.Errorf("index DomainPeers by Domain: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Domain{}, builder.WithPredicates(domainChangedPredicate())).
		// DomainPeer deletions must wake the Domain up, otherwise a pass that
		// decided lifecycleWait has no event to resume on and the finalizer is
		// only released by the next full resync. Owns() cannot do this on its
		// own: a peer written before ownership metadata existed carries no
		// OwnerReference, so its deletion would map back to no Domain at all
		// and the finalizer would hold the Domain until a resync. Mapping
		// through the same keys as the field index covers both the owned and
		// the legacy shape.
		Watches(
			&rlarkv1alpha1.DomainPeer{},
			handler.EnqueueRequestsFromMapFunc(domainsForPeer),
		).
		Named("domain").
		Watches(
			&rlarkv1alpha1.Pod{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				pod, ok := obj.(*rlarkv1alpha1.Pod)
				if !ok || pod.Spec.Domain == "" {
					return nil
				}
				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{Name: pod.Spec.Domain},
				}}
			}),
		).
		WithOptions(controllerconfig.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}

// domainChangedPredicate admits the updates this reconciler has to act on.
//
// GenerationChangedPredicate alone is not enough once the lifecycle is driven
// by a finalizer: adding or removing a finalizer and setting a deletion
// timestamp leave Generation untouched, so the controller would never observe
// its own claim nor the deletion it has to answer for.
func domainChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldDomain, oldOK := e.ObjectOld.(*rlarkv1alpha1.Domain)
			newDomain, newOK := e.ObjectNew.(*rlarkv1alpha1.Domain)
			return !oldOK || !newOK || oldDomain.Generation != newDomain.Generation ||
				!reflect.DeepEqual(oldDomain.Finalizers, newDomain.Finalizers) ||
				!reflect.DeepEqual(oldDomain.DeletionTimestamp, newDomain.DeletionTimestamp)
		},
	}
}

// domainParentRef describes the Domain a DomainPeer must be attributed to.
func domainParentRef(domain *rlarkv1alpha1.Domain) utils.ParentRef {
	return utils.ParentRef{
		UID:        domain.UID,
		APIVersion: rlarkv1alpha1.GroupVersion.String(),
		Kind:       "Domain",
		Name:       domain.Name,
		Label:      domainPeerLabel,
	}
}

// indexDomainPeerByDomain reports the Domain names a DomainPeer may belong to.
//
// Three keys are needed and all of them are only candidates, never proof. A
// peer written by the current controller carries a controller OwnerReference,
// a peer renamed away from its Domain carries only the canonical label, and a
// peer written before ownership metadata existed carries only its own name.
// The field index, the event mapping and the orphan sweeper all read this one
// helper, so none of them can arrive at a different candidate set than the
// others: a peer the sweeper considers is always a peer whose real parent it
// will look up, and a peer event always wakes the Domain that owns it.
func indexDomainPeerByDomain(obj client.Object) []string {
	peer, ok := obj.(*rlarkv1alpha1.DomainPeer)
	if !ok {
		return nil
	}
	values := []string{peer.Name}
	for _, key := range utils.ChildIndexKeys(peer, rlarkv1alpha1.GroupVersion.String(), "Domain", domainPeerLabel) {
		if !slices.Contains(values, key) {
			values = append(values, key)
		}
	}
	return values
}

// domainsForPeer maps a DomainPeer event back to the Domains that may be
// waiting on it.
//
// A Domain that decided to wait for its peers to disappear schedules no timer,
// so it only resumes on a child event. Missing a candidate here means its
// finalizer holds the object until the next full resync.
func domainsForPeer(_ context.Context, obj client.Object) []reconcile.Request {
	names := indexDomainPeerByDomain(obj)
	requests := make([]reconcile.Request, 0, len(names))
	for _, name := range names {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name},
		})
	}
	return requests
}

// setPeerOwnership writes the canonical ownership metadata onto peer and
// reports whether anything changed. It is idempotent, so it can run on every
// reconcile.
func (r *Reconciler) setPeerOwnership(domain *rlarkv1alpha1.Domain, peer *rlarkv1alpha1.DomainPeer) (bool, error) {
	changed := false
	if peer.Labels[domainPeerLabel] != domain.Name {
		if peer.Labels == nil {
			peer.Labels = map[string]string{}
		}
		peer.Labels[domainPeerLabel] = domain.Name
		changed = true
	}
	if peer.Annotations[utils.ParentUIDAnnotation] != string(domain.UID) {
		peer.Annotations = utils.MergeAnnotations(peer.Annotations, map[string]string{
			utils.ParentUIDAnnotation: string(domain.UID),
		})
		changed = true
	}
	// Domain is cluster-scoped, so it may own a namespaced DomainPeer and
	// Kubernetes garbage collection reclaims the peer once the Domain is gone.
	if !domainParentRef(domain).Controls(peer) {
		if err := ctrl.SetControllerReference(domain, peer, r.Scheme); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func (r *Reconciler) createOrUpdateDomainPeer(
	ctx context.Context, logger logr.Logger,
	domain *rlarkv1alpha1.Domain, namespace string, pods []rlarkv1alpha1.DomainPodInfo,
	signer *signer, prefixLen int,
) error {
	domainName := domain.Name
	desiredPeer := &rlarkv1alpha1.DomainPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      domainName,
			Namespace: namespace,
		},
		Spec: rlarkv1alpha1.DomainPeerSpec{
			PrefixLen: prefixLen,
			Pods:      pods,
		},
	}

	var existingPeer rlarkv1alpha1.DomainPeer
	err := r.Get(ctx, types.NamespacedName{Name: domainName, Namespace: namespace}, &existingPeer)
	if err != nil && client.IgnoreNotFound(err) != nil {
		logger.Error(err, "failed to get DomainPeer", "namespace", namespace)
		return err
	}

	if err != nil {
		logger.Info("creating DomainPeer", "namespace", namespace, "podCount", len(pods))
		if _, err := r.setPeerOwnership(domain, desiredPeer); err != nil {
			return err
		}
		cert, key, err := signer.Sign(ctx, domainName, namespace)
		if err != nil {
			logger.Error(err, "failed to sign certificate for DomainPeer", "namespace", namespace)
			return err
		}
		desiredPeer.Spec.Cert = string(cert)
		desiredPeer.Spec.Key = string(key)
		return r.Create(ctx, desiredPeer)
	}

	if !classifyDomainPeer(&existingPeer, domainParentRef(domain)).claimable() {
		return fmt.Errorf("DomainPeer %s/%s cannot be attributed to Domain %s",
			existingPeer.Namespace, existingPeer.Name, domainName)
	}

	changed, err := r.setPeerOwnership(domain, &existingPeer)
	if err != nil {
		return err
	}
	changed = changed || existingPeer.Spec.PrefixLen != prefixLen || !reflect.DeepEqual(existingPeer.Spec.Pods, pods)
	existingPeer.Spec.PrefixLen = prefixLen
	existingPeer.Spec.Pods = pods
	certData, err := cert.LoadData([]byte(existingPeer.Spec.Cert), []byte(existingPeer.Spec.Key))
	if err != nil || certData.SSHCert == nil || !certData.IsValid() {
		// 重新签发证书
		logger.Info("existing DomainPeer certificate is invalid or expired, re-signing", "namespace", namespace)
		cert, key, err := signer.Sign(ctx, domainName, namespace)
		if err != nil {
			logger.Error(err, "failed to sign certificate for DomainPeer", "namespace", namespace)
			return err
		}
		existingPeer.Spec.Cert = string(cert)
		existingPeer.Spec.Key = string(key)
		changed = true
	}
	if !changed {
		return nil
	}
	if err := r.Update(ctx, &existingPeer); err != nil {
		logger.Error(err, "failed to update DomainPeer", "namespace", namespace)
		return err
	}

	logger.V(1).Info("DomainPeer updated", "namespace", namespace, "podCount", len(pods))
	return nil
}

// deleteDomainPeers cleans up after a Domain that is already gone from the API
// server. The Domain UID is no longer available, so ownership cannot be proven
// here: peers that carry a controller reference are left to Kubernetes garbage
// collection, and only peers still relying on the naming contract are deleted
// directly. Proven cleanup while the Domain still exists belongs to a Domain
// finalizer.
func (r *Reconciler) deleteDomainPeers(ctx context.Context, logger logr.Logger, domainName string) error {
	var peerList rlarkv1alpha1.DomainPeerList
	if err := r.List(ctx, &peerList, client.MatchingFields{domainPeerDomainField: domainName}); err != nil {
		logger.Error(err, "failed to list DomainPeers")
		return err
	}
	for i := range peerList.Items {
		peer := &peerList.Items[i]
		if metav1.GetControllerOf(peer) != nil {
			continue
		}
		// The field index returns a peer for every candidate Domain name it
		// carries, so a peer labelled for a live Domain is also returned under
		// its own name. Deleting it here would destroy a peer that still has an
		// owner, so the naming contract only applies when the label does not
		// point somewhere else.
		if label := peer.Labels[domainPeerLabel]; label != "" && label != domainName {
			continue
		}
		if err := r.Delete(ctx, peer); err != nil && client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to delete DomainPeer", "namespace", peer.Namespace)
			return err
		}
	}
	logger.Info("DomainPeers deleted for domain", "domain", domainName)
	return nil
}

func (r *Reconciler) cleanupStaleDomainPeers(
	ctx context.Context, logger logr.Logger,
	domain *rlarkv1alpha1.Domain, knownNamespaces map[string]struct{},
) error {
	var peerList rlarkv1alpha1.DomainPeerList
	if err := r.List(ctx, &peerList, client.MatchingFields{domainPeerDomainField: domain.Name}); err != nil {
		return err
	}
	parent := domainParentRef(domain)
	for i := range peerList.Items {
		peer := &peerList.Items[i]
		if _, ok := knownNamespaces[peer.Namespace]; ok {
			continue // still has pods, skip
		}
		if !classifyDomainPeer(peer, parent).claimable() {
			logger.V(1).Info("skipping DomainPeer that cannot be attributed to this Domain",
				"namespace", peer.Namespace, "name", peer.Name)
			continue
		}
		// Namespace no longer has pods for this domain — delete DomainPeer
		if err := r.Delete(ctx, peer); err != nil && client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to delete stale DomainPeer", "namespace", peer.Namespace)
			return err
		}
	}
	return nil
}

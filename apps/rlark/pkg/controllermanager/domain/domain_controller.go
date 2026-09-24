package domain

import (
	"context"
	"reflect"
	"sort"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
)

const podDomainField = "spec.domain"

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
}

// +kubebuilder:rbac:groups=rlinf.io,resources=domains,verbs=get;list;watch
// +kubebuilder:rbac:groups=rlinf.io,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=rlinf.io,resources=domainpeers,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles a Domain reconciliation request.
// Triggered by Domain changes or Pod CR changes (mapped to the associated domain).
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("domain", req.Name)

	// 1. Get Domain
	var domain rlarkv1alpha1.Domain
	if err := r.Get(ctx, types.NamespacedName{Name: req.Name}, &domain); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Domain deleted — delete all DomainPeers for this domain
			return r.deleteDomainPeers(ctx, logger, req.Name)
		}
		return ctrl.Result{}, err
	}
	ippool, err := NewIPPool(domain.Spec.CIDR)
	if err != nil {
		logger.Error(err, "invalid CIDR in Domain spec, skip")
		return ctrl.Result{}, nil
	}
	originalAllocations := append([]rlarkv1alpha1.DomainIPAllocation(nil), domain.Status.IPAllocations...)

	// 2. List Pod CRs belonging to this domain through the cache index.
	var podList rlarkv1alpha1.PodList
	if err := r.List(ctx, &podList, client.MatchingFields{podDomainField: domain.Name}); err != nil {
		logger.Error(err, "failed to list Pods")
		return ctrl.Result{}, err
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
	for _, pod := range podList.Items {
		if pod.Spec.Domain != domain.Name {
			continue
		}
		knownNamespaces[pod.Namespace] = struct{}{}
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
				logger.Error(err, "failed to allocate IP for pod", "namespace", ns, "pod", pod.Name)
				return ctrl.Result{}, err
			}
			pod.IP = ip
			podsByNamespace[ns] = append(podsByNamespace[ns], pod)
			domain.Status.IPAllocations = append(domain.Status.IPAllocations, rlarkv1alpha1.DomainIPAllocation{
				IP:   ip,
				Job:  "", // TODO
				Task: "", // TODO
				Pod:  ns + "/" + pod.Namespace + "/" + pod.Name,
			})
		}
	}
	sort.Slice(domain.Status.IPAllocations, func(i, j int) bool {
		return domain.Status.IPAllocations[i].Pod < domain.Status.IPAllocations[j].Pod
	})

	// 7. Update Domain.Status.IPAllocations with the new allocations
	if !reflect.DeepEqual(originalAllocations, domain.Status.IPAllocations) {
		if err := r.Status().Update(ctx, &domain); err != nil {
			logger.Error(err, "failed to update Domain status")
			return ctrl.Result{}, err
		}
	}

	// 8. Create or update DomainPeer per namespace
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
		if err := r.createOrUpdateDomainPeer(ctx, logger, domain.Name, ns, allPods, signer, ippool.PrefixLength()); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 9. Delete DomainPeers in namespaces that no longer have pods for this domain
	if err := r.cleanupStaleDomainPeers(ctx, logger, domain.Name, knownNamespaces); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Domain{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
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

func (r *Reconciler) createOrUpdateDomainPeer(
	ctx context.Context, logger logr.Logger,
	domainName, namespace string, pods []rlarkv1alpha1.DomainPodInfo,
	signer *signer, prefixLen int,
) error {
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
		cert, key, err := signer.Sign(ctx, domainName, namespace)
		if err != nil {
			logger.Error(err, "failed to sign certificate for DomainPeer", "namespace", namespace)
			return err
		}
		desiredPeer.Spec.Cert = string(cert)
		desiredPeer.Spec.Key = string(key)
		return r.Create(ctx, desiredPeer)
	}

	changed := existingPeer.Spec.PrefixLen != prefixLen || !reflect.DeepEqual(existingPeer.Spec.Pods, pods)
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

func (r *Reconciler) deleteDomainPeers(ctx context.Context, logger logr.Logger, domainName string) (ctrl.Result, error) {
	var peerList rlarkv1alpha1.DomainPeerList
	if err := r.List(ctx, &peerList); err != nil {
		logger.Error(err, "failed to list DomainPeers")
		return ctrl.Result{}, err
	}
	for _, peer := range peerList.Items {
		if peer.Name != domainName {
			continue
		}
		if err := r.Delete(ctx, &peer); err != nil && client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to delete DomainPeer", "namespace", peer.Namespace)
			return ctrl.Result{}, err
		}
	}
	logger.Info("DomainPeers deleted for domain", "domain", domainName)
	return ctrl.Result{}, nil
}

func (r *Reconciler) cleanupStaleDomainPeers(ctx context.Context, logger logr.Logger, domainName string, knownNamespaces map[string]struct{}) error {
	var peerList rlarkv1alpha1.DomainPeerList
	if err := r.List(ctx, &peerList); err != nil {
		return err
	}
	for _, peer := range peerList.Items {
		if peer.Name != domainName {
			continue
		}
		if _, ok := knownNamespaces[peer.Namespace]; ok {
			continue // still has pods, skip
		}
		// Namespace no longer has pods for this domain — delete DomainPeer
		if err := r.Delete(ctx, &peer); err != nil && client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to delete stale DomainPeer", "namespace", peer.Namespace)
			return err
		}
	}
	return nil
}

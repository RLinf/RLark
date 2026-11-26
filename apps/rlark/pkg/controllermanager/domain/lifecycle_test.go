package domain

import (
	"context"
	"slices"
	"testing"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// blockerFinalizer stands in for a third party holding a DomainPeer open, so a
// fixture can represent a peer that is terminating but not yet gone.
const blockerFinalizer = "test.rlinf.io/blocker"

func deletingDomain(name string, finalizers ...string) *rlarkv1alpha1.Domain {
	deletedAt := metav1.Now()
	return &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			UID:               types.UID("domain-" + name),
			DeletionTimestamp: &deletedAt,
			Finalizers:        finalizers,
		},
		Spec: rlarkv1alpha1.DomainSpec{CIDR: "10.244.0.0/16"},
	}
}

func liveDomain(name string) *rlarkv1alpha1.Domain {
	return &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID("domain-" + name)},
		Spec:       rlarkv1alpha1.DomainSpec{CIDR: "10.244.0.0/16"},
	}
}

// ownedPeer is a peer carrying the canonical metadata this controller writes.
func ownedPeer(t *testing.T, domain *rlarkv1alpha1.Domain, name, namespace string) *rlarkv1alpha1.DomainPeer {
	t.Helper()
	peer := makeDomainPeer(name, namespace, "", "")
	adoptDomainPeer(t, domain, peer)
	return peer
}

// labelOnlyPeer is a peer from before the canonical metadata existed: its only
// membership evidence is the Domain label.
func labelOnlyPeer(domain *rlarkv1alpha1.Domain, name, namespace string) *rlarkv1alpha1.DomainPeer {
	peer := makeDomainPeer(name, namespace, "", "")
	peer.Labels = map[string]string{domainPeerLabel: domain.Name}
	return peer
}

// ownedPeerWithoutLabel is a peer this controller owns whose canonical label
// was stripped, leaving the OwnerReference as its only membership evidence.
func ownedPeerWithoutLabel(t *testing.T, domain *rlarkv1alpha1.Domain, name, namespace string) *rlarkv1alpha1.DomainPeer {
	t.Helper()
	peer := ownedPeer(t, domain, name, namespace)
	delete(peer.Labels, domainPeerLabel)
	return peer
}

// unprovenPeer claims this Domain's UID but carries no controller reference to
// back the claim, so the evidence contradicts itself.
func unprovenPeer(domain *rlarkv1alpha1.Domain, name, namespace string) *rlarkv1alpha1.DomainPeer {
	peer := makeDomainPeer(name, namespace, "", "")
	peer.Labels = map[string]string{domainPeerLabel: domain.Name}
	peer.Annotations = map[string]string{utils.ParentUIDAnnotation: string(domain.UID)}
	return peer
}

// foreignPeer provably belongs to a different Domain.
func foreignPeer(domain *rlarkv1alpha1.Domain, name, namespace string) *rlarkv1alpha1.DomainPeer {
	peer := makeDomainPeer(name, namespace, "", "")
	peer.Labels = map[string]string{domainPeerLabel: domain.Name}
	peer.Annotations = map[string]string{utils.ParentUIDAnnotation: "someone-else"}
	return peer
}

func terminating(peer *rlarkv1alpha1.DomainPeer) *rlarkv1alpha1.DomainPeer {
	deletedAt := metav1.Now()
	peer.DeletionTimestamp = &deletedAt
	peer.Finalizers = []string{blockerFinalizer}
	return peer
}

// The cleanup rule is a pure function so that every branch can be proven
// without an API server, and so the executing code has exactly four cases to
// implement.
func TestDecidePeerCleanup(t *testing.T) {
	domain := deletingDomain("alpha", CleanupFinalizer)

	cases := []struct {
		name       string
		peers      []rlarkv1alpha1.DomainPeer
		action     lifecycleAction
		reason     string
		wantDelete int
	}{
		{
			name:   "no peers leaves nothing to clean up",
			action: lifecycleProceed,
			reason: reasonPeerCleanupFinalizerFreed,
		},
		{
			name:   "a peer of another Domain is not this Domain's problem",
			peers:  []rlarkv1alpha1.DomainPeer{*foreignPeer(domain, "alpha", "cluster-a")},
			action: lifecycleProceed,
			reason: reasonPeerCleanupFinalizerFreed,
		},
		{
			name:       "an owned peer must be deleted",
			peers:      []rlarkv1alpha1.DomainPeer{*ownedPeer(t, domain, "alpha", "cluster-a")},
			action:     lifecycleCleanup,
			reason:     reasonPeerCleanupPending,
			wantDelete: 1,
		},
		{
			name:       "a label-only peer is still owned and must be deleted",
			peers:      []rlarkv1alpha1.DomainPeer{*labelOnlyPeer(domain, "legacy-peer", "cluster-a")},
			action:     lifecycleCleanup,
			reason:     reasonPeerCleanupPending,
			wantDelete: 1,
		},
		{
			name:   "an already terminating peer is waited for, not deleted again",
			peers:  []rlarkv1alpha1.DomainPeer{*terminating(ownedPeer(t, domain, "alpha", "cluster-a"))},
			action: lifecycleWait,
			reason: reasonPeerCleanupTerminating,
		},
		{
			name:   "a peer whose ownership cannot be proven blocks the pass",
			peers:  []rlarkv1alpha1.DomainPeer{*unprovenPeer(domain, "alpha", "cluster-a")},
			action: lifecycleBlock,
			reason: reasonPeerCleanupBlocked,
		},
		{
			// Cleaning up the understood part and then releasing the finalizer
			// would hand the unproven peer to garbage collection, so blocking
			// has to win.
			name: "blocking wins over deleting",
			peers: []rlarkv1alpha1.DomainPeer{
				*ownedPeer(t, domain, "alpha", "cluster-a"),
				*unprovenPeer(domain, "alpha", "cluster-b"),
			},
			action: lifecycleBlock,
			reason: reasonPeerCleanupBlocked,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := decidePeerCleanup(domain, tc.peers)
			if decision.Action != tc.action {
				t.Fatalf("Action = %d, want %d", decision.Action, tc.action)
			}
			if decision.Reason != tc.reason {
				t.Fatalf("Reason = %q, want %q", decision.Reason, tc.reason)
			}
			if len(decision.Delete) != tc.wantDelete {
				t.Fatalf("Delete = %d peers, want %d", len(decision.Delete), tc.wantDelete)
			}
			if (len(decision.Blocked) > 0) != (tc.action == lifecycleBlock) {
				t.Fatalf("Blocked = %d peers for action %d", len(decision.Blocked), tc.action)
			}
		})
	}
}

// The event message is rendered from an unbounded set, so it must stay bounded.
func TestBlockedPeerMessageIsBounded(t *testing.T) {
	domain := deletingDomain("alpha", CleanupFinalizer)
	var peers []*rlarkv1alpha1.DomainPeer
	for _, ns := range []string{"a", "b", "c", "d", "e"} {
		peers = append(peers, unprovenPeer(domain, "alpha", ns))
	}
	message := blockedPeerMessage(peers)
	want := "a/alpha, b/alpha, c/alpha, ..."
	if message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
}

func newLifecycleReconciler(t *testing.T, objs ...client.Object) *Reconciler {
	t.Helper()
	scheme := newTestScheme(t)
	cl := newTestClientBuilder(t).
		WithObjects(objs...).
		WithStatusSubresource(&rlarkv1alpha1.Domain{}).
		Build()
	return &Reconciler{Client: cl, Scheme: scheme}
}

func getDomain(t *testing.T, r *Reconciler, name string) (*rlarkv1alpha1.Domain, bool) {
	t.Helper()
	var domain rlarkv1alpha1.Domain
	err := r.Get(context.Background(), types.NamespacedName{Name: name}, &domain)
	if apierrors.IsNotFound(err) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("get Domain %s: %v", name, err)
	}
	return &domain, true
}

// A Domain must be claimed before the first DomainPeer is written, otherwise a
// delete request arriving in between leaves peers behind with no cleanup
// signal. The claim does not end the pass: it changes no desired state, so the
// same pass goes on to converge.
func TestReconcileClaimsDomainBeforeWritingPeers(t *testing.T) {
	domain := liveDomain("alpha")
	r := newLifecycleReconciler(t, domain)

	result, err := r.reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "alpha"}})
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if result.Reason == reasonPeerCleanupFinalizerAdded {
		t.Fatalf("claim failed: %+v", result)
	}

	claimed, found := getDomain(t, r, "alpha")
	if !found {
		t.Fatal("Domain disappeared")
	}
	if !controllerutil.ContainsFinalizer(claimed, CleanupFinalizer) {
		t.Fatalf("finalizers = %v, want the cleanup finalizer", claimed.Finalizers)
	}

	// The claim is idempotent: a second pass must not duplicate the finalizer.
	if _, err = r.reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "alpha"}}); err != nil {
		t.Fatalf("second reconcile returned error: %v", err)
	}
	claimed, found = getDomain(t, r, "alpha")
	if !found {
		t.Fatal("Domain disappeared")
	}
	if len(claimed.Finalizers) != 1 {
		t.Fatalf("finalizers = %v, want exactly one", claimed.Finalizers)
	}
}

// Deletion converges across passes: the finalizer is held until a later pass
// observes the peers gone, so an interruption between deleting a peer and
// releasing the finalizer cannot lose the obligation.
func TestReconcileDeletionHoldsFinalizerUntilPeersAreGone(t *testing.T) {
	domain := deletingDomain("alpha", CleanupFinalizer)
	peer := ownedPeer(t, domain, "alpha", "cluster-a")
	r := newLifecycleReconciler(t, domain, peer)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "alpha"}}

	result, err := r.reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("first pass returned error: %v", err)
	}
	if result.Outcome != sharedreconcile.OutcomeCleanupInProgress {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, sharedreconcile.OutcomeCleanupInProgress)
	}
	if _, found := getDomain(t, r, "alpha"); !found {
		t.Fatal("Domain was released before its peers were gone")
	}

	var peers rlarkv1alpha1.DomainPeerList
	if err := r.List(context.Background(), &peers); err != nil {
		t.Fatalf("list DomainPeers: %v", err)
	}
	if len(peers.Items) != 0 {
		t.Fatalf("DomainPeers remaining = %d, want 0", len(peers.Items))
	}

	// Re-entry on the same object must now find nothing left and release.
	result, err = r.reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("second pass returned error: %v", err)
	}
	if result.Outcome != sharedreconcile.OutcomeSucceeded {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, sharedreconcile.OutcomeSucceeded)
	}
	if result.Reason != reasonPeerCleanupFinalizerFreed {
		t.Fatalf("Reason = %q, want %q", result.Reason, reasonPeerCleanupFinalizerFreed)
	}
	if _, found := getDomain(t, r, "alpha"); found {
		t.Fatal("Domain still present after the finalizer was released")
	}
}

// A peer that is already terminating must not be deleted again, and the pass
// must stay in cleanup rather than release the finalizer early.
func TestReconcileDeletionWaitsForTerminatingPeers(t *testing.T) {
	domain := deletingDomain("alpha", CleanupFinalizer)
	peer := terminating(ownedPeer(t, domain, "alpha", "cluster-a"))
	r := newLifecycleReconciler(t, domain, peer)

	result, err := r.reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "alpha"}})
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if result.Outcome != sharedreconcile.OutcomeCleanupInProgress {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, sharedreconcile.OutcomeCleanupInProgress)
	}
	if result.Reason != reasonPeerCleanupTerminating {
		t.Fatalf("Reason = %q, want %q", result.Reason, reasonPeerCleanupTerminating)
	}
	// Waiting must not poll: the peer's disappearance arrives as a watch event
	// through the DomainPeer ownership watch.
	if result.Retry.After != 0 {
		t.Fatalf("Retry.After = %v, want 0 while waiting for an event", result.Retry.After)
	}
	if _, found := getDomain(t, r, "alpha"); !found {
		t.Fatal("Domain was released while a peer was still terminating")
	}
}

// C6: an object whose ownership cannot be proven must neither be deleted nor
// abandoned to garbage collection.
func TestReconcileDeletionBlocksOnUnprovenPeer(t *testing.T) {
	domain := deletingDomain("alpha", CleanupFinalizer)
	peer := unprovenPeer(domain, "alpha", "cluster-a")
	r := newLifecycleReconciler(t, domain, peer)

	result, err := r.reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "alpha"}})
	// Blocking is a classified outcome, not a failure: returning an error
	// would retry an object only an operator can unblock.
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if result.Outcome != sharedreconcile.OutcomeBlocked {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, sharedreconcile.OutcomeBlocked)
	}
	if result.Reason != reasonPeerCleanupBlocked {
		t.Fatalf("Reason = %q, want %q", result.Reason, reasonPeerCleanupBlocked)
	}
	if _, found := getDomain(t, r, "alpha"); !found {
		t.Fatal("Domain was released while a peer of unknown ownership remained")
	}

	var remaining rlarkv1alpha1.DomainPeer
	if err := r.Get(context.Background(),
		types.NamespacedName{Namespace: "cluster-a", Name: "alpha"}, &remaining); err != nil {
		t.Fatalf("unproven DomainPeer was destroyed on a guess: %v", err)
	}
}

// Without the finalizer this controller owes nothing, and re-adding one to a
// deleting object would strand it forever.
func TestReconcileDeletionWithoutFinalizerIsDropped(t *testing.T) {
	domain := deletingDomain("alpha", blockerFinalizer)
	r := newLifecycleReconciler(t, domain)

	result, err := r.reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "alpha"}})
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if result.Outcome != sharedreconcile.OutcomeDropped {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, sharedreconcile.OutcomeDropped)
	}
	if result.Reason != reasonPeerCleanupNotOwned {
		t.Fatalf("Reason = %q, want %q", result.Reason, reasonPeerCleanupNotOwned)
	}
	claimed, found := getDomain(t, r, "alpha")
	if !found {
		t.Fatal("Domain disappeared")
	}
	if controllerutil.ContainsFinalizer(claimed, CleanupFinalizer) {
		t.Fatalf("finalizers = %v, want the cleanup finalizer to stay absent", claimed.Finalizers)
	}
}

// A deleting Domain that waits on a peer has to be woken by that peer's
// deletion. Peers written before ownership metadata existed carry no
// OwnerReference, so the mapping must fall back to the name and the label;
// otherwise the finalizer holds the Domain until the next full resync.
func TestDomainsForPeerCoversLegacyAndOwnedShapes(t *testing.T) {
	domain := liveDomain("alpha")

	cases := []struct {
		name string
		peer *rlarkv1alpha1.DomainPeer
		want []string
	}{
		{
			name: "owned peer",
			peer: ownedPeer(t, domain, "alpha", "rlark-system"),
			want: []string{"alpha"},
		},
		{
			name: "legacy peer named after its Domain",
			peer: makeDomainPeer("alpha", "rlark-system", "", ""),
			want: []string{"alpha"},
		},
		{
			name: "legacy peer carrying only the label",
			peer: labelOnlyPeer(domain, "renamed", "rlark-system"),
			want: []string{"renamed", "alpha"},
		},
		{
			name: "owned peer whose name and label both differ",
			peer: ownedPeer(t, domain, "renamed", "rlark-system"),
			want: []string{"renamed", "alpha"},
		},
		{
			name: "owned peer carrying no label",
			peer: ownedPeerWithoutLabel(t, domain, "renamed", "rlark-system"),
			want: []string{"renamed", "alpha"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := domainsForPeer(context.Background(), tc.peer)
			got := make([]string, 0, len(requests))
			for _, request := range requests {
				if request.Namespace != "" {
					t.Fatalf("request %v is namespaced, Domain is cluster scoped", request)
				}
				got = append(got, request.Name)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("domains = %v, want %v", got, tc.want)
			}
		})
	}
}

// A peer is indexed under its own name as well as under its Domain label, so a
// request for a Domain that never existed still finds peers belonging to a live
// Domain. Cleanup after a vanished Domain must not delete those.
func TestDeleteDomainPeersKeepsPeerLabelledForAnotherDomain(t *testing.T) {
	domain := liveDomain("alpha")
	peer := labelOnlyPeer(domain, "renamed", "rlark-system")
	r := newLifecycleReconciler(t, domain, peer)

	if err := r.deleteDomainPeers(context.Background(), logr.Discard(), peer.Name); err != nil {
		t.Fatalf("deleteDomainPeers: %v", err)
	}

	var got rlarkv1alpha1.DomainPeer
	if err := r.Get(context.Background(),
		types.NamespacedName{Namespace: peer.Namespace, Name: peer.Name}, &got); err != nil {
		t.Fatalf("DomainPeer labelled for live Domain %s was deleted: %v", domain.Name, err)
	}
}

// A peer whose only membership evidence is the naming contract is still deleted
// when the Domain it is named after is gone.
func TestDeleteDomainPeersRemovesPeerNamedAfterVanishedDomain(t *testing.T) {
	peer := makeDomainPeer("alpha", "rlark-system", "", "")
	r := newLifecycleReconciler(t, peer)

	if err := r.deleteDomainPeers(context.Background(), logr.Discard(), "alpha"); err != nil {
		t.Fatalf("deleteDomainPeers: %v", err)
	}

	var got rlarkv1alpha1.DomainPeer
	err := r.Get(context.Background(),
		types.NamespacedName{Namespace: peer.Namespace, Name: peer.Name}, &got)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("legacy DomainPeer still present, get returned %v", err)
	}
}

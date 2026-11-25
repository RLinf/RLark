package domain

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// CleanupFinalizer keeps a deleted Domain present in the API server until its
// DomainPeers have been removed under proven ownership.
//
// Without it, the only cleanup signal is the Domain already being gone, and at
// that point the Domain UID is gone with it: ownership can no longer be proven,
// so peers written by older controllers can only be matched by name. The
// finalizer moves cleanup to a point where the UID is still readable.
//
// This finalizer answers for this controller's own obligation only. It is
// released as soon as that obligation is discharged, regardless of which other
// finalizers are still present.
const CleanupFinalizer = "domains.rlinf.io/peer-cleanup"

// lifecycleAction is the decision a lifecycle pass reaches about a Domain that
// is being deleted. Deciding is separated from executing so the rule can be
// tested without an API server, and so the executing code cannot quietly
// invent a fifth behaviour.
type lifecycleAction int

const (
	// lifecycleProceed means nothing blocks the Domain. While the Domain
	// exists that means running the normal reconcile; while it is being
	// deleted it means the cleanup obligation is discharged and the finalizer
	// can be released.
	lifecycleProceed lifecycleAction = iota
	// lifecycleCleanup means owned children are still present and must be
	// deleted. The finalizer stays until a later pass observes them gone.
	lifecycleCleanup
	// lifecycleWait means every owned child is already terminating. The
	// finalizer stays, and the pass waits for the deletion events instead of
	// polling.
	lifecycleWait
	// lifecycleBlock means at least one candidate child cannot be attributed
	// to this Domain with proof. Nothing is deleted and the finalizer stays:
	// releasing it would hand an object of unknown ownership to garbage
	// collection, and deleting it would destroy someone else's object.
	lifecycleBlock
)

// Reason codes attached to a lifecycle pass. Closed set: they are used as a
// metric label.
const (
	reasonPeerCleanupFinalizerAdded = "DomainPeerCleanupFinalizerAdded"
	reasonPeerCleanupPending        = "DomainPeerCleanupPending"
	reasonPeerCleanupTerminating    = "DomainPeerTerminating"
	reasonPeerCleanupBlocked        = "DomainPeerOwnershipUnproven"
	reasonPeerCleanupFinalizerFreed = "DomainPeerCleanupFinalizerFreed"
	reasonPeerCleanupNotOwned       = "DomainNotClaimed"
	reasonPeerListFailed            = "DomainPeerListFailed"
)

// peerCleanupDecision is the result of one cleanup decision. Only the slice
// that belongs to Action is populated.
type peerCleanupDecision struct {
	Action lifecycleAction
	Reason string
	// Delete lists the peers to delete, and is set only for
	// lifecycleCleanup.
	Delete []*rlarkv1alpha1.DomainPeer
	// Blocked lists the peers whose ownership could not be proven, and is set
	// only for lifecycleBlock. It exists to be reported, not acted on.
	Blocked []*rlarkv1alpha1.DomainPeer
}

// decidePeerCleanup decides what a deleting Domain still owes on its
// DomainPeers. It is a pure function: it returns decisions only and must not
// mutate the Domain or any peer.
//
// peers is the candidate set from the Domain index, which deliberately admits
// peers whose membership rests only on the naming contract, so attribution is
// re-checked here rather than trusted from the index.
//
// A peer that provably belongs to another parent is not this Domain's problem
// and is skipped. A peer that cannot be attributed either way blocks the pass,
// because C6 requires ownership to be proven before an object is destroyed and
// an unproven peer must not be deleted on a guess.
func decidePeerCleanup(domain *rlarkv1alpha1.Domain, peers []rlarkv1alpha1.DomainPeer) peerCleanupDecision {
	parent := domainParentRef(domain)

	var (
		toDelete    []*rlarkv1alpha1.DomainPeer
		blocked     []*rlarkv1alpha1.DomainPeer
		terminating int
	)
	for i := range peers {
		peer := &peers[i]
		switch classifyDomainPeer(peer, parent) {
		case peerForeign:
			// Provably someone else's object: leaving it alone is the correct
			// result, not a deferred decision.
			continue
		case peerUnproven:
			blocked = append(blocked, peer)
		case peerOwned, peerLegacy:
			if peer.DeletionTimestamp.IsZero() {
				toDelete = append(toDelete, peer)
			} else {
				terminating++
			}
		}
	}

	// Blocking wins over deleting: once any peer is unattributable, this pass
	// stops touching the set at all rather than cleaning up the part it
	// happens to understand and then releasing the finalizer.
	if len(blocked) > 0 {
		return peerCleanupDecision{
			Action:  lifecycleBlock,
			Reason:  reasonPeerCleanupBlocked,
			Blocked: blocked,
		}
	}
	if len(toDelete) > 0 {
		return peerCleanupDecision{
			Action: lifecycleCleanup,
			Reason: reasonPeerCleanupPending,
			Delete: toDelete,
		}
	}
	if terminating > 0 {
		return peerCleanupDecision{
			Action: lifecycleWait,
			Reason: reasonPeerCleanupTerminating,
		}
	}
	return peerCleanupDecision{
		Action: lifecycleProceed,
		Reason: reasonPeerCleanupFinalizerFreed,
	}
}

// blockedPeerMessage renders the unattributable peers for an event or a log.
func blockedPeerMessage(peers []*rlarkv1alpha1.DomainPeer) string {
	const maxListed = 3
	message := ""
	for i, peer := range peers {
		if i == maxListed {
			message += ", ..."
			break
		}
		if i > 0 {
			message += ", "
		}
		message += peer.Namespace + "/" + peer.Name
	}
	return message
}

// peerOwnership is the DomainPeer-specific outcome of an ownership decision.
type peerOwnership int

const (
	// peerForeign means the DomainPeer provably belongs to something else, so
	// it must neither be mutated nor deleted.
	peerForeign peerOwnership = iota
	// peerOwned means ownership is proven by the canonical metadata.
	peerOwned
	// peerLegacy means ownership rests only on the naming contract or a label,
	// and the canonical metadata still has to be written.
	peerLegacy
	// peerUnproven means the evidence is absent or self-contradictory. It is
	// kept distinct from peerForeign: "belongs to someone else" is a
	// conclusion, while "cannot be decided" is a reason to stop.
	peerUnproven
)

// claimable reports whether this controller may write to or delete the peer.
func (o peerOwnership) claimable() bool {
	return o == peerOwned || o == peerLegacy
}

// classifyDomainPeer attributes peer to parent. It delegates to
// utils.ClassifyChild so the controller does not re-implement owner matching,
// and only adds one bounded compatibility rule on top of it: a DomainPeer that
// carries no ownership metadata at all is accepted as legacy when its name
// matches the Domain, because that naming contract was the only membership
// evidence this controller used to emit. Without that rule every DomainPeer
// created before this change would block the reconcile after an upgrade.
func classifyDomainPeer(peer *rlarkv1alpha1.DomainPeer, parent utils.ParentRef) peerOwnership {
	decision, err := utils.ClassifyChild(peer, parent, "")
	switch {
	case err == nil:
		if decision.Legacy() {
			return peerLegacy
		}
		return peerOwned
	case decision.Reason == utils.ReasonUnprovenOwnership && peer.Name == parent.Name:
		return peerLegacy
	case decision.Ownership == utils.ChildUnproven:
		return peerUnproven
	default:
		return peerForeign
	}
}

// claimDomain claims a live Domain before its first DomainPeer is written, so
// a delete request can never win a race against peer creation. It is a no-op
// once the Domain is already claimed.
//
// The claim happens inside the pass that will write the peers rather than
// ending it, because adding a finalizer changes no desired state and there is
// nothing to re-read before converging.
//
// The finalizer is added with Update rather than a merge patch: a merge patch
// replaces metadata.finalizers wholesale and carries no optimistic
// concurrency, so it can silently drop a finalizer another controller added
// concurrently.
func (r *Reconciler) claimDomain(ctx context.Context, domain *rlarkv1alpha1.Domain) error {
	if !controllerutil.AddFinalizer(domain, CleanupFinalizer) {
		return nil
	}
	if err := r.Update(ctx, domain); err != nil {
		return fmt.Errorf("add Domain cleanup finalizer: %w", err)
	}
	return nil
}

// reconcileDeletion discharges the cleanup obligation of a deleting Domain.
//
// It is re-entrant by construction: every pass re-reads the candidate peers,
// re-decides from that state, and releases the finalizer only once the decision
// says nothing is left. A pass that is interrupted after deleting some peers
// and before releasing the finalizer therefore resumes correctly.
func (r *Reconciler) reconcileDeletion(ctx context.Context, logger logr.Logger, domain *rlarkv1alpha1.Domain) (controller.Result, error) {
	if !controllerutil.ContainsFinalizer(domain, CleanupFinalizer) {
		// Never claimed, or already released. Either way this controller owes
		// nothing, and re-adding the finalizer to a deleting object would only
		// strand it.
		return controller.Result{
			Outcome: sharedreconcile.OutcomeDropped,
			Reason:  reasonPeerCleanupNotOwned,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
		}, nil
	}

	var peerList rlarkv1alpha1.DomainPeerList
	if err := r.List(ctx, &peerList, client.MatchingFields{domainPeerDomainField: domain.Name}); err != nil {
		return controller.Result{Reason: reasonPeerListFailed},
			fmt.Errorf("list DomainPeers of Domain %s: %w", domain.Name, err)
	}

	decision := decidePeerCleanup(domain, peerList.Items)
	switch decision.Action {
	case lifecycleBlock:
		message := blockedPeerMessage(decision.Blocked)
		logger.Info("Domain cleanup blocked by DomainPeers of unproven ownership", "peers", message)
		r.Shell.Warnf(domain, decision.Reason,
			"Cleanup is blocked by DomainPeers whose ownership cannot be proven: %s", message)
		// The finalizer stays. Recovery is an operator repairing or removing
		// the peer, which arrives as a watch event.
		return controller.Result{
			Outcome: sharedreconcile.OutcomeBlocked,
			Reason:  decision.Reason,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange},
		}, nil
	case lifecycleCleanup:
		for _, peer := range decision.Delete {
			if err := r.Delete(ctx, peer); client.IgnoreNotFound(err) != nil {
				return controller.Result{Reason: decision.Reason},
					fmt.Errorf("delete DomainPeer %s/%s: %w", peer.Namespace, peer.Name, err)
			}
		}
		return controller.Result{
			Outcome: sharedreconcile.OutcomeCleanupInProgress,
			Reason:  decision.Reason,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange},
		}, nil
	case lifecycleWait:
		return controller.Result{
			Outcome: sharedreconcile.OutcomeCleanupInProgress,
			Reason:  decision.Reason,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange},
		}, nil
	}

	controllerutil.RemoveFinalizer(domain, CleanupFinalizer)
	if err := r.Update(ctx, domain); err != nil {
		return controller.Result{Reason: decision.Reason},
			fmt.Errorf("remove Domain cleanup finalizer: %w", err)
	}
	return controller.Result{
		Outcome: sharedreconcile.OutcomeSucceeded,
		Reason:  decision.Reason,
		Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
	}, nil
}

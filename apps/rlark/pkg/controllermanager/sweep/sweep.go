// Package sweep reclaims control-plane children whose parent is gone.
//
// Parent-driven cleanup is the primary path: the owning controller holds a
// finalizer and deletes its children, and Kubernetes garbage collection
// reclaims children that carry a controller OwnerReference. Neither path is
// complete. A watch event can be lost, a finalizer can be removed by force,
// and children written by older controllers carry no OwnerReference at all.
// The sweeper is the periodic backstop for exactly those leftovers: it
// recomputes from current state on every pass and never relies on what an
// earlier pass decided.
package sweep

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// StaleSinceAnnotation records when the sweeper first observed a child as
// orphaned. The decision to delete is deliberately two-phase, and the first
// phase has to survive a restart: keeping the timestamp in memory would reset
// the clock on every leader change and the TTL would never elapse. The
// data-plane Pod sweeper uses the same annotation for the same reason.
const StaleSinceAnnotation = "rlark.io/stale-since"

// Event reasons emitted by the sweeper. They are stable CamelCase identifiers.
const (
	reasonOrphanMarked  = "OrphanMarked"
	reasonOrphanDeleted = "OrphanDeleted"
)

// Rule describes one orphan class: a namespaced child Kind whose parent Kind
// is cluster-scoped. Because the parent is cluster-scoped, a name is enough to
// look it up and a rule needs no parent namespace.
type Rule struct {
	// Name identifies the rule in logs, events and errors.
	Name string
	// Child returns an empty child object used for a re-read.
	Child func() client.Object
	// ChildList returns an empty list used for one page of children.
	ChildList func() client.ObjectList
	// Parent returns an empty parent object used for a lookup.
	Parent func() client.Object
	// ParentNames reports the parent names a child carries evidence for. An
	// empty result means the child references no parent at all; ownership
	// cannot be proven and the sweeper must leave it alone.
	ParentNames func(child client.Object) []string
	// ParentRef builds the ownership identity a child is classified against.
	// It is also called with the empty object returned by Parent to learn the
	// APIVersion and Kind of the parent class, so it must report those two
	// fields for any parent of the right type, populated or not.
	ParentRef func(parent client.Object) utils.ParentRef
}

func (r Rule) validate() error {
	if r.Name == "" {
		return errors.New("sweep rule needs a name")
	}
	if r.Child == nil || r.ChildList == nil || r.Parent == nil || r.ParentNames == nil ||
		r.ParentRef == nil {
		return fmt.Errorf("sweep rule %s is missing an accessor", r.Name)
	}
	class := r.ParentRef(r.Parent())
	if class.APIVersion == "" || class.Kind == "" {
		return fmt.Errorf("sweep rule %s parent ref does not identify a parent class", r.Name)
	}
	return nil
}

// Options bounds how aggressively the sweeper runs.
type Options struct {
	// Period is the interval between sweeps, before jitter is applied.
	Period time.Duration
	// PageSize bounds how many children a single List returns, so a cluster
	// with many children cannot turn one sweep into one huge response.
	PageSize int64
	// StaleTTL is how long a child stays marked before it is deleted. It has
	// to be long enough for an ordinary reconcile to recover a lost event, so
	// that only genuinely orphaned children reach the delete.
	StaleTTL time.Duration
}

// DefaultOptions returns the shipped sweeper schedule.
func DefaultOptions() Options {
	return Options{
		Period:   10 * time.Minute,
		PageSize: 200,
		StaleTTL: 15 * time.Minute,
	}
}

// Validate rejects a schedule that would disable or unbound the sweep.
func (o Options) Validate() error {
	if o.Period <= 0 {
		return errors.New("orphan sweep interval must be positive")
	}
	if o.PageSize <= 0 {
		return errors.New("orphan sweep page size must be positive")
	}
	if o.StaleTTL <= 0 {
		return errors.New("orphan stale TTL must be positive")
	}
	return nil
}

// Sweeper deletes orphaned children on a jittered period, on the leader only.
type Sweeper struct {
	// reader must bypass the informer cache. controller-runtime's cache
	// reader rejects a List that carries a continue token, and listing every
	// child in the cluster unpaged is the exact failure mode this avoids.
	reader client.Reader
	writer client.Client
	shell  controller.Shell
	rules  []Rule
	opts   Options

	now    func() time.Time
	timer  waiter
	jitter func() float64
}

// New builds a sweeper for the given rules.
func New(reader client.Reader, writer client.Client, shell controller.Shell, opts Options, rules ...Rule) (*Sweeper, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, errors.New("orphan sweeper needs at least one rule")
	}
	for _, rule := range rules {
		if err := rule.validate(); err != nil {
			return nil, err
		}
	}
	return &Sweeper{
		reader: reader,
		writer: writer,
		shell:  shell,
		rules:  rules,
		opts:   opts,
		now:    time.Now,
		timer:  realWaiter{},
		jitter: rand.Float64,
	}, nil
}

// SetupWithManager registers the sweeper as a manager Runnable so it inherits
// the manager lifecycle.
func (s *Sweeper) SetupWithManager(mgr ctrl.Manager) error {
	return mgr.Add(s)
}

// NeedLeaderElection keeps the sweep on the leader. A sweep issues deletes;
// running it on every replica would multiply both the deletes and the API
// load without making any of them more correct.
func (s *Sweeper) NeedLeaderElection() bool { return true }

// Start runs sweeps until ctx is cancelled.
func (s *Sweeper) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("orphan-sweeper")
	for {
		if err := s.Sweep(ctx); err != nil {
			// Returning the error from a manager Runnable would terminate the
			// whole controller-manager. A failed sweep is recoverable by
			// definition: the next pass recomputes from current state.
			logger.Error(err, "orphan sweep failed")
		}
		if err := s.timer.Wait(ctx, jittered(s.opts.Period, s.jitter())); err != nil {
			return nil
		}
	}
}

// Sweep runs one pass over every rule. Rules are independent, so a failure in
// one must not hide the others.
func (s *Sweeper) Sweep(ctx context.Context) error {
	var errs []error
	for _, rule := range s.rules {
		if err := s.sweepRule(ctx, rule); err != nil {
			errs = append(errs, fmt.Errorf("sweep %s: %w", rule.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Sweeper) sweepRule(ctx context.Context, rule Rule) error {
	parents := newParentCache(s.reader, rule)
	var errs []error
	continueToken := ""
	for {
		list := rule.ChildList()
		opts := &client.ListOptions{Limit: s.opts.PageSize, Continue: continueToken}
		if err := s.reader.List(ctx, list, opts); err != nil {
			return fmt.Errorf("list children: %w", err)
		}
		items, err := meta.ExtractList(list)
		if err != nil {
			return fmt.Errorf("extract list items: %w", err)
		}
		for _, item := range items {
			child, ok := item.(client.Object)
			if !ok {
				errs = append(errs, fmt.Errorf("list item %T is not a client.Object", item))
				continue
			}
			if err := s.sweepChild(ctx, rule, parents, child); err != nil {
				errs = append(errs, fmt.Errorf("%s/%s: %w", child.GetNamespace(), child.GetName(), err))
			}
		}
		continueToken = list.GetContinue()
		if continueToken == "" {
			return errors.Join(errs...)
		}
	}
}

func (s *Sweeper) sweepChild(ctx context.Context, rule Rule, parents *parentCache, child client.Object) error {
	if child.GetDeletionTimestamp() != nil {
		// Already on its way out; a second delete would add nothing.
		return nil
	}
	orphaned, err := s.orphaned(ctx, rule, parents, child)
	if err != nil {
		return err
	}
	if !orphaned {
		// A child that was marked and then reclaimed must lose the mark, or
		// the next orphaning would inherit an expired clock and delete at
		// once instead of granting a fresh grace period.
		return s.clearStale(ctx, child)
	}
	return s.markOrDelete(ctx, rule, child)
}

// orphaned reports whether no live parent can still claim child.
//
// Matching a name is not proof of ownership. A child is only reclaimed when
// every parent name it references is either absent or occupied by an object
// that provably does not own it, and when nothing about the child points at
// an owner outside this rule. A child controlled by another Kind, or one
// whose ownership cannot be proven, is left alone: the sweeper deletes, and a
// delete must never rest on a guess.
func (s *Sweeper) orphaned(ctx context.Context, rule Rule, parents *parentCache, child client.Object) (bool, error) {
	names := rule.ParentNames(child)
	if len(names) == 0 {
		// No parent reference at all. Ownership cannot be proven, so deleting
		// the child would be a guess rather than a reclaim.
		return false, nil
	}
	class := rule.ParentRef(rule.Parent())
	if owner := metav1.GetControllerOf(child); owner != nil &&
		(owner.APIVersion != class.APIVersion || owner.Kind != class.Kind) {
		// Controlled by another Kind. Whatever name or label made this child
		// a candidate, it belongs to a controller this rule knows nothing
		// about and reclaiming it would destroy someone else's object.
		return false, nil
	}
	for _, name := range names {
		parent, err := parents.get(ctx, name)
		if err != nil {
			return false, err
		}
		if parent == nil {
			continue
		}
		ref := rule.ParentRef(parent)
		if ref.Controls(child) {
			// This live object is the controller owner. That is the strongest
			// evidence there is, and it outranks any annotation that says
			// otherwise: contradictory metadata is a reason to stop, not a
			// reason to delete.
			return false, nil
		}
		// Otherwise classify with the same helper the owning controller uses,
		// so the sweeper cannot reach a different verdict about who owns the
		// child. Only one verdict lets the reclaim continue: the child
		// demonstrably belongs to some other parent, for example because it
		// records the UID of an earlier object that used this name. Owned,
		// adoptable, unproven and "no reference at all" all stop here.
		decision, _ := utils.ClassifyRemovedChild(child, ref, nil)
		if decision.Reason != utils.ReasonOwnedByOtherParent {
			return false, nil
		}
	}
	return true, nil
}

func (s *Sweeper) markOrDelete(ctx context.Context, rule Rule, child client.Object) error {
	markedAt, marked := staleSince(child)
	if !marked {
		return s.markStale(ctx, rule, child)
	}
	if s.now().Sub(markedAt) < s.opts.StaleTTL {
		return nil
	}

	// The TTL has elapsed, but the decision was taken against a snapshot. Read
	// the child again and re-decide: the parent may have come back, the child
	// may have been recreated under the same name, or another actor may have
	// cleared the mark. Deleting on the strength of the earlier observation
	// alone is a time-of-check/time-of-use bug.
	fresh := rule.Child()
	if err := s.reader.Get(ctx, client.ObjectKeyFromObject(child), fresh); err != nil {
		return client.IgnoreNotFound(err)
	}
	if fresh.GetUID() != child.GetUID() || fresh.GetDeletionTimestamp() != nil {
		return nil
	}
	if freshAt, ok := staleSince(fresh); !ok || !freshAt.Equal(markedAt) {
		return nil
	}
	// Deliberately not the memoised cache: the point of this second look is
	// that it is not the one the mark was based on.
	orphaned, err := s.orphaned(ctx, rule, newParentCache(s.reader, rule), fresh)
	if err != nil {
		return err
	}
	if !orphaned {
		return s.clearStale(ctx, fresh)
	}

	uid := fresh.GetUID()
	s.shell.Warnf(fresh, reasonOrphanDeleted,
		"Deleting orphaned %s: no live parent claims it after %s", rule.Name, s.opts.StaleTTL)
	// The UID precondition is the server-side half of the same guard: if the
	// object is replaced between the Get and the Delete, the delete fails
	// instead of removing the replacement.
	if err := s.writer.Delete(ctx, fresh, client.Preconditions{UID: &uid}); err != nil {
		return fmt.Errorf("delete orphan: %w", client.IgnoreNotFound(err))
	}
	return nil
}

func (s *Sweeper) markStale(ctx context.Context, rule Rule, child client.Object) error {
	original, ok := child.DeepCopyObject().(client.Object)
	if !ok {
		return fmt.Errorf("deep copy of %T is not a client.Object", child)
	}
	child.SetAnnotations(utils.MergeAnnotations(child.GetAnnotations(), map[string]string{
		StaleSinceAnnotation: s.now().UTC().Format(time.RFC3339Nano),
	}))
	if err := s.writer.Patch(ctx, child, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("mark orphan stale: %w", err)
	}
	s.shell.Warnf(child, reasonOrphanMarked,
		"Observed as orphaned %s; it will be deleted if no parent claims it within %s", rule.Name, s.opts.StaleTTL)
	return nil
}

func (s *Sweeper) clearStale(ctx context.Context, child client.Object) error {
	if _, marked := child.GetAnnotations()[StaleSinceAnnotation]; !marked {
		return nil
	}
	original, ok := child.DeepCopyObject().(client.Object)
	if !ok {
		return fmt.Errorf("deep copy of %T is not a client.Object", child)
	}
	// MergeAnnotations removes a key whose owned value is empty.
	child.SetAnnotations(utils.MergeAnnotations(child.GetAnnotations(), map[string]string{
		StaleSinceAnnotation: "",
	}))
	if err := s.writer.Patch(ctx, child, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("clear stale mark: %w", err)
	}
	return nil
}

// staleSince reads the mark. An unparsable value is reported as absent so the
// next pass rewrites it in canonical form, rather than either deleting at once
// or never deleting at all.
func staleSince(child client.Object) (time.Time, bool) {
	raw := child.GetAnnotations()[StaleSinceAnnotation]
	if raw == "" {
		return time.Time{}, false
	}
	markedAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return markedAt, true
}

// parentCache memoises parent lookups for the duration of one rule pass.
// Children of the same parent are the common case, and a sweep must not
// degrade into one API call per child.
type parentCache struct {
	reader client.Reader
	rule   Rule
	seen   map[string]client.Object
}

func newParentCache(reader client.Reader, rule Rule) *parentCache {
	return &parentCache{reader: reader, rule: rule, seen: make(map[string]client.Object)}
}

// get returns the parent named name, or nil when it does not exist.
func (p *parentCache) get(ctx context.Context, name string) (client.Object, error) {
	if parent, ok := p.seen[name]; ok {
		return parent, nil
	}
	parent := p.rule.Parent()
	err := p.reader.Get(ctx, client.ObjectKey{Name: name}, parent)
	switch {
	case apierrors.IsNotFound(err):
		p.seen[name] = nil
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("get parent %s: %w", name, err)
	}
	p.seen[name] = parent
	return parent, nil
}

// waiter is the sleep seam the tests replace.
type waiter interface {
	Wait(ctx context.Context, d time.Duration) error
}

type realWaiter struct{}

func (realWaiter) Wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// jittered spreads sweeps over 80%-120% of period so restarted replicas do not
// line every sweep up on the same instant.
func jittered(period time.Duration, unit float64) time.Duration {
	if unit < 0 {
		unit = 0
	} else if unit > 1 {
		unit = 1
	}
	return time.Duration(float64(period) * (0.8 + 0.4*unit))
}

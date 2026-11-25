package sweep

import (
	"context"
	"errors"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// nopShell drops events. The shell is silent when it holds no recorder, so the
// tests can assert on the API objects without wiring a fake recorder.
func nopShell() controller.Shell { return controller.Shell{} }

const (
	testDomainLabel = "rlinf.io/domain"
	testJobLabel    = "rlinf.io/job"
)

var testStart = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

type fakeClock struct{ at time.Time }

func (c *fakeClock) now() time.Time          { return c.at }
func (c *fakeClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

// peerRule mirrors domain.OrphanPeerRule. It is restated here because the
// domain package imports this one, and because the engine is what these tests
// exercise; the rules have their own tests in their own packages.
func peerRule() Rule {
	return Rule{
		Name:      "DomainPeer",
		Child:     func() client.Object { return &rlarkv1alpha1.DomainPeer{} },
		ChildList: func() client.ObjectList { return &rlarkv1alpha1.DomainPeerList{} },
		Parent:    func() client.Object { return &rlarkv1alpha1.Domain{} },
		ParentNames: func(child client.Object) []string {
			peer, ok := child.(*rlarkv1alpha1.DomainPeer)
			if !ok {
				return nil
			}
			names := []string{peer.Name}
			if label := peer.Labels[testDomainLabel]; label != "" && label != peer.Name {
				names = append(names, label)
			}
			return names
		},
		ParentRef: func(parent client.Object) utils.ParentRef {
			return utils.ParentRef{
				UID:        parent.GetUID(),
				APIVersion: rlarkv1alpha1.GroupVersion.String(),
				Kind:       "Domain",
				Name:       parent.GetName(),
				Label:      testDomainLabel,
			}
		},
	}
}

// taskRule mirrors job.OrphanTaskRule.
func taskRule() Rule {
	return Rule{
		Name:      "Task",
		Child:     func() client.Object { return &rlarkv1alpha1.Task{} },
		ChildList: func() client.ObjectList { return &rlarkv1alpha1.TaskList{} },
		Parent:    func() client.Object { return &rlarkv1alpha1.Job{} },
		ParentNames: func(child client.Object) []string {
			return utils.ChildIndexKeys(child, rlarkv1alpha1.GroupVersion.String(), "Job", testJobLabel)
		},
		ParentRef: func(parent client.Object) utils.ParentRef {
			return utils.ParentRef{
				UID:        parent.GetUID(),
				APIVersion: rlarkv1alpha1.GroupVersion.String(),
				Kind:       "Job",
				Name:       parent.GetName(),
				Label:      testJobLabel,
			}
		},
	}
}

func newSweeper(t *testing.T, c client.Client, clock *fakeClock, rules ...Rule) *Sweeper {
	t.Helper()
	s, err := New(c, c, nopShell(), DefaultOptions(), rules...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.now = clock.now
	return s
}

func domainPeer(name string, mutate ...func(*rlarkv1alpha1.DomainPeer)) *rlarkv1alpha1.DomainPeer {
	peer := &rlarkv1alpha1.DomainPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "rlark-system",
			UID:       types.UID("peer-" + name),
		},
	}
	for _, m := range mutate {
		m(peer)
	}
	return peer
}

func liveDomain(name string) *rlarkv1alpha1.Domain {
	return &rlarkv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID("domain-" + name)},
	}
}

func getPeer(t *testing.T, c client.Client, name string) *rlarkv1alpha1.DomainPeer {
	t.Helper()
	peer := &rlarkv1alpha1.DomainPeer{}
	key := client.ObjectKey{Namespace: "rlark-system", Name: name}
	if err := c.Get(context.Background(), key, peer); err != nil {
		t.Fatalf("get DomainPeer %s: %v", name, err)
	}
	return peer
}

func markedAt(t *testing.T, obj client.Object) time.Time {
	t.Helper()
	at, ok := staleSince(obj)
	if !ok {
		t.Fatalf("%s carries no parsable %s annotation", obj.GetName(), StaleSinceAnnotation)
	}
	return at
}

func TestSweepMarksOrphanThenDeletesAfterTTL(t *testing.T) {
	clock := &fakeClock{at: testStart}
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(domainPeer("gone")).
		Build()
	s := newSweeper(t, c, clock, peerRule())

	// First pass only records the observation: a single sighting is not
	// enough evidence, because the parent watch may simply be lagging.
	if err := s.Sweep(context.Background()); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if got := markedAt(t, getPeer(t, c, "gone")); !got.Equal(testStart) {
		t.Fatalf("mark recorded %s, want %s", got, testStart)
	}

	// Still inside the grace period: the mark stands, the child stays.
	clock.advance(s.opts.StaleTTL - time.Second)
	if err := s.Sweep(context.Background()); err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if got := markedAt(t, getPeer(t, c, "gone")); !got.Equal(testStart) {
		t.Fatalf("mark moved to %s, want it unchanged at %s", got, testStart)
	}

	clock.advance(2 * time.Second)
	if err := s.Sweep(context.Background()); err != nil {
		t.Fatalf("third sweep: %v", err)
	}
	err := c.Get(context.Background(), client.ObjectKey{Namespace: "rlark-system", Name: "gone"}, &rlarkv1alpha1.DomainPeer{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected orphan to be deleted, got %v", err)
	}
}

func TestSweepLeavesChildWithLiveParent(t *testing.T) {
	clock := &fakeClock{at: testStart}
	peer := domainPeer("kept", func(p *rlarkv1alpha1.DomainPeer) {
		p.Annotations = map[string]string{utils.ParentUIDAnnotation: "domain-kept"}
	})
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(peer, liveDomain("kept")).
		Build()
	s := newSweeper(t, c, clock, peerRule())

	if err := s.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, marked := getPeer(t, c, "kept").Annotations[StaleSinceAnnotation]; marked {
		t.Fatal("child of a live parent must not be marked")
	}
}

func TestSweepResolvesParentThroughLegacyLabel(t *testing.T) {
	clock := &fakeClock{at: testStart}
	// The peer name and the Domain name differ; only the legacy label ties
	// them together, and that is still proof of a live parent.
	peer := domainPeer("peer-renamed", func(p *rlarkv1alpha1.DomainPeer) {
		p.Labels = map[string]string{testDomainLabel: "owner"}
	})
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(peer, liveDomain("owner")).
		Build()
	s := newSweeper(t, c, clock, peerRule())

	if err := s.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, marked := getPeer(t, c, "peer-renamed").Annotations[StaleSinceAnnotation]; marked {
		t.Fatal("peer claimed by the legacy label must not be marked")
	}
}

func TestSweepTreatsParentUIDMismatchAsOrphan(t *testing.T) {
	clock := &fakeClock{at: testStart}
	// A Domain of the same name exists, but it is a different object than the
	// one this peer was created for, so the peer belongs to nobody.
	peer := domainPeer("recreated", func(p *rlarkv1alpha1.DomainPeer) {
		p.Annotations = map[string]string{utils.ParentUIDAnnotation: "domain-previous-instance"}
	})
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(peer, liveDomain("recreated")).
		Build()
	s := newSweeper(t, c, clock, peerRule())

	if err := s.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, marked := getPeer(t, c, "recreated").Annotations[StaleSinceAnnotation]; !marked {
		t.Fatal("peer recorded against a different parent instance must be marked")
	}
}

// A Task that another controller controls must survive even when the name it
// shares with this rule's parent class no longer resolves. Deleting on a name
// match alone would destroy an object this sweeper knows nothing about.
func TestSweepIgnoresChildControlledByAnotherKind(t *testing.T) {
	clock := &fakeClock{at: testStart}
	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name: "borrowed", Namespace: "rlark-system", UID: "task-borrowed",
			Labels: map[string]string{testJobLabel: "gone"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
				Name:       "gone",
				UID:        "sts-gone",
				Controller: ptr.To(true),
			}},
		},
	}
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(task).Build()
	s := newSweeper(t, c, clock, taskRule())

	clock.advance(time.Hour)
	for range 2 {
		if err := s.Sweep(context.Background()); err != nil {
			t.Fatalf("sweep: %v", err)
		}
	}
	got := &rlarkv1alpha1.Task{}
	key := client.ObjectKey{Namespace: "rlark-system", Name: "borrowed"}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Task owned by another controller must survive: %v", err)
	}
	if _, marked := got.Annotations[StaleSinceAnnotation]; marked {
		t.Fatal("Task owned by another controller must not be marked")
	}
}

// A live parent that controls the child wins over a stale recorded UID. The
// two disagree, and a delete must never be taken on contradictory evidence.
func TestSweepKeepsChildControlledByLiveParentDespiteStaleUID(t *testing.T) {
	clock := &fakeClock{at: testStart}
	domain := liveDomain("flapping")
	peer := domainPeer("flapping", func(p *rlarkv1alpha1.DomainPeer) {
		p.Annotations = map[string]string{utils.ParentUIDAnnotation: "domain-previous-instance"}
		p.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: rlarkv1alpha1.GroupVersion.String(),
			Kind:       "Domain",
			Name:       domain.Name,
			UID:        domain.UID,
			Controller: ptr.To(true),
		}}
	})
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(peer, domain).Build()
	s := newSweeper(t, c, clock, peerRule())

	clock.advance(time.Hour)
	for range 2 {
		if err := s.Sweep(context.Background()); err != nil {
			t.Fatalf("sweep: %v", err)
		}
	}
	got := getPeer(t, c, "flapping")
	if _, marked := got.Annotations[StaleSinceAnnotation]; marked {
		t.Fatal("peer controlled by a live parent must not be marked")
	}
}

func TestSweepClearsStaleMarkWhenParentReturns(t *testing.T) {
	clock := &fakeClock{at: testStart}
	peer := domainPeer("recovered", func(p *rlarkv1alpha1.DomainPeer) {
		p.Annotations = map[string]string{StaleSinceAnnotation: testStart.Format(time.RFC3339Nano)}
	})
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(peer, liveDomain("recovered")).
		Build()
	s := newSweeper(t, c, clock, peerRule())

	// The mark has to go, otherwise the next time this child is orphaned it
	// would inherit an already-expired clock and be deleted without grace.
	clock.advance(time.Hour)
	if err := s.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, marked := getPeer(t, c, "recovered").Annotations[StaleSinceAnnotation]; marked {
		t.Fatal("stale mark must be cleared once a parent claims the child again")
	}
}

func TestSweepIgnoresChildWithoutParentReference(t *testing.T) {
	clock := &fakeClock{at: testStart}
	// No OwnerReference, no label, no recorded parent UID: nothing proves who
	// this Task belongs to, so the sweeper has no business deleting it.
	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "handmade", Namespace: "rlark-system", UID: "task-handmade"},
	}
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(task).Build()
	s := newSweeper(t, c, clock, taskRule())

	clock.advance(time.Hour)
	for range 2 {
		if err := s.Sweep(context.Background()); err != nil {
			t.Fatalf("sweep: %v", err)
		}
	}
	got := &rlarkv1alpha1.Task{}
	key := client.ObjectKey{Namespace: "rlark-system", Name: "handmade"}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("unattributable Task must survive: %v", err)
	}
	if _, marked := got.Annotations[StaleSinceAnnotation]; marked {
		t.Fatal("unattributable Task must not be marked")
	}
}

func TestSweepIgnoresTerminatingChild(t *testing.T) {
	clock := &fakeClock{at: testStart}
	deletedAt := metav1.Now()
	peer := domainPeer("leaving", func(p *rlarkv1alpha1.DomainPeer) {
		p.DeletionTimestamp = &deletedAt
		p.Finalizers = []string{"test.rlinf.io/blocker"}
	})
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(peer).Build()
	s := newSweeper(t, c, clock, peerRule())

	if err := s.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, marked := getPeer(t, c, "leaving").Annotations[StaleSinceAnnotation]; marked {
		t.Fatal("a child already being deleted must not be marked")
	}
}

func TestSweepSkipsDeleteWhenMarkChangedSinceList(t *testing.T) {
	clock := &fakeClock{at: testStart}
	peer := domainPeer("contended", func(p *rlarkv1alpha1.DomainPeer) {
		p.Annotations = map[string]string{StaleSinceAnnotation: testStart.Format(time.RFC3339Nano)}
	})
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(peer).
		WithInterceptorFuncs(interceptor.Funcs{
			// Stand in for another actor rewriting the mark between the List
			// the decision was taken on and the delete.
			Get: func(ctx context.Context, inner client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if err := inner.Get(ctx, key, obj, opts...); err != nil {
					return err
				}
				if fetched, ok := obj.(*rlarkv1alpha1.DomainPeer); ok {
					fetched.Annotations[StaleSinceAnnotation] = clock.now().Format(time.RFC3339Nano)
				}
				return nil
			},
		}).
		Build()
	s := newSweeper(t, c, clock, peerRule())

	clock.advance(2 * s.opts.StaleTTL)
	if err := s.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "rlark-system", Name: "contended"}, &rlarkv1alpha1.DomainPeer{}); err != nil {
		t.Fatalf("child whose mark changed must survive the pass: %v", err)
	}
}

func TestSweepSkipsDeleteWhenChildWasReplaced(t *testing.T) {
	clock := &fakeClock{at: testStart}
	peer := domainPeer("reborn", func(p *rlarkv1alpha1.DomainPeer) {
		p.Annotations = map[string]string{StaleSinceAnnotation: testStart.Format(time.RFC3339Nano)}
	})
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(peer).
		WithInterceptorFuncs(interceptor.Funcs{
			// Same name, different object: deleting it would destroy the
			// replacement rather than reclaim the orphan.
			Get: func(ctx context.Context, inner client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if err := inner.Get(ctx, key, obj, opts...); err != nil {
					return err
				}
				if fetched, ok := obj.(*rlarkv1alpha1.DomainPeer); ok {
					fetched.UID = "peer-reborn-second-instance"
				}
				return nil
			},
		}).
		Build()
	s := newSweeper(t, c, clock, peerRule())

	clock.advance(2 * s.opts.StaleTTL)
	if err := s.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "rlark-system", Name: "reborn"}, &rlarkv1alpha1.DomainPeer{}); err != nil {
		t.Fatalf("replaced child must survive the pass: %v", err)
	}
}

func TestSweepVisitsEveryPage(t *testing.T) {
	clock := &fakeClock{at: testStart}
	objects := []client.Object{domainPeer("a"), domainPeer("b"), domainPeer("c")}
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(objects...).Build()
	s, err := New(c, c, nopShell(), Options{Period: time.Minute, PageSize: 1, StaleTTL: time.Minute}, peerRule())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.now = clock.now

	if err := s.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if _, marked := getPeer(t, c, name).Annotations[StaleSinceAnnotation]; !marked {
			t.Fatalf("orphan %s on a later page was not visited", name)
		}
	}
}

func TestSweepReportsFailingRuleWithoutSkippingOthers(t *testing.T) {
	clock := &fakeClock{at: testStart}
	listFailure := errors.New("boom")
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(domainPeer("gone")).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, inner client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*rlarkv1alpha1.TaskList); ok {
					return listFailure
				}
				return inner.List(ctx, list, opts...)
			},
		}).
		Build()
	// The failing rule is first, so a sequential implementation that bailed on
	// the first error would leave the second rule unswept.
	s := newSweeper(t, c, clock, taskRule(), peerRule())

	err := s.Sweep(context.Background())
	if !errors.Is(err, listFailure) {
		t.Fatalf("sweep error = %v, want it to wrap %v", err, listFailure)
	}
	if _, marked := getPeer(t, c, "gone").Annotations[StaleSinceAnnotation]; !marked {
		t.Fatal("a failing rule must not stop the remaining rules")
	}
}

func TestNewRejectsUnusableConfiguration(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()
	cases := map[string]struct {
		opts  Options
		rules []Rule
	}{
		"no rules":           {opts: DefaultOptions()},
		"zero period":        {opts: Options{PageSize: 1, StaleTTL: time.Minute}, rules: []Rule{peerRule()}},
		"zero page size":     {opts: Options{Period: time.Minute, StaleTTL: time.Minute}, rules: []Rule{peerRule()}},
		"zero stale ttl":     {opts: Options{Period: time.Minute, PageSize: 1}, rules: []Rule{peerRule()}},
		"unnamed rule":       {opts: DefaultOptions(), rules: []Rule{{Child: peerRule().Child}}},
		"rule without lists": {opts: DefaultOptions(), rules: []Rule{{Name: "DomainPeer"}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(c, c, nopShell(), tc.opts, tc.rules...); err == nil {
				t.Fatal("expected New to reject the configuration")
			}
		})
	}
}

func TestSweeperRunsOnLeaderOnly(t *testing.T) {
	clock := &fakeClock{at: testStart}
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()
	if !newSweeper(t, c, clock, peerRule()).NeedLeaderElection() {
		t.Fatal("a sweeper that issues deletes must run on the leader only")
	}
}

func TestJitteredStaysWithinBand(t *testing.T) {
	period := 10 * time.Minute
	for _, unit := range []float64{-1, 0, 0.5, 1, 2} {
		got := jittered(period, unit)
		if got < 8*time.Minute || got > 12*time.Minute {
			t.Fatalf("jittered(%s, %v) = %s, want within 80%%-120%%", period, unit, got)
		}
	}
	if got := jittered(period, 0.5); got != period {
		t.Fatalf("jittered(%s, 0.5) = %s, want the unjittered period", period, got)
	}
}

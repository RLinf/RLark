package task

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	hostpkg "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/host"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
)

func TestTaskResyncScansPagesOnceAndFiltersScope(t *testing.T) {
	var emitted []trigger.Trigger
	resync := &TaskResync{
		client: &stubClient{}, namespace: "cluster", host: resyncTestHost(), period: time.Minute,
		emit: func(value trigger.Trigger) error { emitted = append(emitted, value); return nil }, pageSize: 2, budget: 10,
	}
	page := 0
	resync.list = func(_ context.Context, tasks *rlarkv1alpha1.TaskList, _ ...client.ListOption) error {
		page++
		switch page {
		case 1:
			tasks.Items = []rlarkv1alpha1.Task{
				claimedTaskForResync("claim", "uid-1", "cluster/Kubernetes", "Docker"),
				taskForResync("unclaimed", "uid-2", "Kubernetes", nil),
			}
			tasks.Continue = "next"
		case 2:
			tasks.Items = []rlarkv1alpha1.Task{
				claimedTaskForResync("other", "uid-3", "other/Kubernetes", "Kubernetes"),
				taskForResync("wrong-type", "uid-4", "Docker", nil),
			}
		default:
			t.Fatal("unexpected extra page")
		}
		return nil
	}
	if err := resync.scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if page != 2 || len(emitted) != 2 || emitted[0].Key.Name != "claim" || emitted[1].Key.Name != "unclaimed" {
		t.Fatalf("pages=%d emitted=%#v", page, emitted)
	}
	for _, value := range emitted {
		if value.Kind != trigger.Periodic || value.Generation != 3 || value.ObjectUID == "" {
			t.Fatalf("trigger=%#v", value)
		}
	}
}

func TestTaskResyncStartRunsImmediatelyAndStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	waiter := &cancelingTimer{cancel: cancel}
	scans := 0
	resync := &TaskResync{
		client: &stubClient{}, namespace: "cluster", host: resyncTestHost(), period: time.Minute,
		emit: func(trigger.Trigger) error { return nil }, timer: waiter, jitter: func() float64 { return 0.5 }, pageSize: 1, budget: 1,
		list: func(_ context.Context, _ *rlarkv1alpha1.TaskList, _ ...client.ListOption) error { scans++; return nil },
	}
	if err := resync.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if scans != 1 || waiter.duration != time.Minute {
		t.Fatalf("scans=%d duration=%s", scans, waiter.duration)
	}
}

func TestTaskResyncDisabledWaitsForCancellationWithoutScanning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resync := &TaskResync{client: &stubClient{}, namespace: "cluster", host: resyncTestHost(), emit: func(trigger.Trigger) error { return nil }}
	if err := resync.Start(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTaskResyncBridgePressureContinuesAcrossPages(t *testing.T) {
	var emitted []string
	page := 0
	resync := &TaskResync{
		client: &stubClient{}, namespace: "cluster", host: resyncTestHost(), pageSize: 2, budget: 10,
		emit: func(value trigger.Trigger) error {
			emitted = append(emitted, value.Key.Name)
			if value.Key.Name == "two" {
				return ErrTriggerBridgeSaturated
			}
			return nil
		},
		list: func(_ context.Context, tasks *rlarkv1alpha1.TaskList, _ ...client.ListOption) error {
			page++
			if page == 1 {
				tasks.Items = []rlarkv1alpha1.Task{taskForResync("one", "uid-1", "Kubernetes", nil), taskForResync("two", "uid-2", "Kubernetes", nil)}
				tasks.Continue = "next"
			} else {
				tasks.Items = []rlarkv1alpha1.Task{taskForResync("three", "uid-3", "Kubernetes", nil)}
			}
			return nil
		},
	}
	if err := resync.scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(emitted, []string{"one", "two", "three"}) || page != 2 {
		t.Fatalf("pages=%d emitted=%v", page, emitted)
	}
}

func TestTaskResyncStartRetriesListFailureWithoutStoppingWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	waiter := &cancelAfterWaitsTimer{cancel: cancel, cancelAfter: 2}
	lists := 0
	resync := &TaskResync{
		client: &stubClient{}, namespace: "cluster", host: resyncTestHost(), period: time.Minute,
		emit: func(trigger.Trigger) error { return nil }, timer: waiter, jitter: func() float64 { return 0.5 }, pageSize: 1, budget: 1,
		list: func(_ context.Context, _ *rlarkv1alpha1.TaskList, _ ...client.ListOption) error {
			lists++
			if lists == 1 {
				return errors.New("temporary API outage")
			}
			return nil
		},
	}
	if err := resync.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if lists != 2 || waiter.waits != 2 {
		t.Fatalf("lists=%d waits=%d", lists, waiter.waits)
	}
}

func TestTaskResyncUsesHostSchedulingDecision(t *testing.T) {
	host := resyncTestHost()
	tests := []struct {
		name string
		task rlarkv1alpha1.Task
		want bool
	}{
		{name: "unclaimed eligible", task: taskForResync("task", "uid", "Kubernetes", nil), want: true},
		{name: "annotation without finalizer is not claim", task: taskForResync("task", "uid", "Docker", map[string]string{hostpkg.TaskClaimantAnnotation: "cluster/Kubernetes"}), want: false},
		{name: "own claim survives selector drift", task: claimedTaskForResync("task", "uid", "cluster/Kubernetes", "Docker"), want: true},
		{name: "foreign claim", task: claimedTaskForResync("task", "uid", "other/Kubernetes", "Kubernetes"), want: false},
		{name: "legacy claim realizable", task: claimedTaskForResync("task", "uid", "", "Kubernetes"), want: true},
		{name: "legacy claim unsupported", task: claimedTaskForResync("task", "uid", "", "Docker"), want: false},
		{name: "legacy three-part claimant", task: claimedTaskForResync("task", "uid", "cluster/Kubernetes/domain", "Kubernetes"), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := (&TaskResync{host: host}).shouldEnqueue(&test.task); got != test.want {
				t.Fatalf("shouldEnqueue()=%v, want %v for metadata=%#v", got, test.want, test.task.ObjectMeta)
			}
		})
	}
}

func TestJitteredResyncPeriodBounds(t *testing.T) {
	period := 10 * time.Minute
	if got := jitteredResyncPeriod(period, -1); got != 8*time.Minute {
		t.Fatalf("lower=%s", got)
	}
	if got := jitteredResyncPeriod(period, 1.5); got != 12*time.Minute {
		t.Fatalf("upper=%s", got)
	}
}

func taskForResync(name, uid, agentType string, annotations map[string]string) rlarkv1alpha1.Task {
	return rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "cluster",
			Name:        name,
			UID:         types.UID(uid),
			Generation:  3,
			Annotations: annotations,
		},
		Spec: rlarkv1alpha1.TaskSpec{
			Placement: rlarkv1alpha1.TaskPlacementSpec{},
			Runtime: rlarkv1alpha1.TaskRuntimeSpec{
				Kind: rlarkv1alpha1.RuntimeKind(agentType),
			},
			Workload: rlarkv1alpha1.TaskWorkloadSpec{
				Kind:    "test-workload",
				Version: "v1",
				Carrier: rlarkv1alpha1.TaskWorkloadCarrier{
					Type:    "test-carrier",
					Version: "v1",
				},
			},
		},
	}
}

func claimedTaskForResync(name, uid, claimant, agentType string) rlarkv1alpha1.Task {
	task := taskForResync(name, uid, agentType, map[string]string{})
	task.Finalizers = []string{hostpkg.TaskFinalizer}
	if claimant != "" {
		task.Annotations[hostpkg.TaskClaimantAnnotation] = claimant
	}
	return task
}

func resyncTestHost() *hostpkg.Host {
	return &hostpkg.Host{Claimant: "cluster/Kubernetes", Eligibility: resyncEligibility{}}
}

type resyncEligibility struct{}

func (resyncEligibility) Matches(task *rlarkv1alpha1.Task) bool {
	return task.Spec.Runtime.Kind == rlarkv1alpha1.RuntimeKindKubernetes
}

func (resyncEligibility) ValidationMessage(task *rlarkv1alpha1.Task) string {
	if task.Spec.Workload.Kind == "" {
		return "missing workload"
	}
	return ""
}

type cancelingTimer struct {
	cancel   context.CancelFunc
	duration time.Duration
}

func (t *cancelingTimer) Wait(ctx context.Context, duration time.Duration) error {
	t.duration = duration
	t.cancel()
	<-ctx.Done()
	return ctx.Err()
}

type cancelAfterWaitsTimer struct {
	cancel      context.CancelFunc
	cancelAfter int
	waits       int
}

func (t *cancelAfterWaitsTimer) Wait(ctx context.Context, _ time.Duration) error {
	t.waits++
	if t.waits >= t.cancelAfter {
		t.cancel()
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

type stubClient struct{ client.Client }

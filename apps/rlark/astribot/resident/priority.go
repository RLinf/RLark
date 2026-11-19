// Package resident is what the cluster Agent keeps for workloads that live on
// every node of a class (per-node Tasks): the PriorityClass they run at, and
// their updates on nodes that are in use.
package resident

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/astribot/nodeagent"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

const (
	// PriorityClassName is the class a per-node resident workload names in its
	// Pod template. It is above every Pod without a class, so a node short of
	// resources evicts those first, and below the system classes RLark's own
	// node components run at. It never preempts: a resident instance waits for
	// room on its node rather than remove what runs there.
	PriorityClassName = "rlark-node-resident"
	// PriorityValue is the class's value.
	PriorityValue int32 = 1000000

	priorityClassInterval = 5 * time.Minute
)

// PriorityClass keeps PriorityClassName in the cluster. Applications refer to
// it by name and make none of their own.
type PriorityClass struct {
	Local client.Client
}

// Start creates the class and puts it back if it is deleted. A class's value
// cannot be changed, so one of the same name that someone else made is left as
// it is and reported.
func (p *PriorityClass) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("resident-priority-class")
	for {
		if err := p.ensure(ctx); err != nil {
			logger.Error(err, "keep PriorityClass; retrying", "name", PriorityClassName)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(priorityClassInterval):
		}
	}
}

func (p *PriorityClass) ensure(ctx context.Context) error {
	var existing schedulingv1.PriorityClass
	err := p.Local.Get(ctx, client.ObjectKey{Name: PriorityClassName}, &existing)
	if apierrors.IsNotFound(err) {
		return client.IgnoreAlreadyExists(p.Local.Create(ctx, Desired()))
	}
	if err != nil {
		return err
	}
	desired := Desired()
	if existing.Value != desired.Value || existing.PreemptionPolicy == nil || *existing.PreemptionPolicy != *desired.PreemptionPolicy {
		return fmt.Errorf("PriorityClass %s exists with value %d, not %d and never preempting; delete it to have RLark's", PriorityClassName, existing.Value, desired.Value)
	}
	return nil
}

// Desired is the class as RLark makes it.
func Desired() *schedulingv1.PriorityClass {
	never := corev1.PreemptNever
	return &schedulingv1.PriorityClass{
		ObjectMeta:       metav1.ObjectMeta{Name: PriorityClassName, Labels: map[string]string{nodeagent.ManagedByLabel: nodeagent.ManagedBy}},
		Value:            PriorityValue,
		PreemptionPolicy: &never,
		Description:      "RLark per-node resident workloads: kept on a node under pressure before Pods without a class, never preempting.",
	}
}

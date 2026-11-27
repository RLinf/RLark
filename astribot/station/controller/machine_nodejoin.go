package controller

import (
	"context"
	"fmt"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const machineNodeJoinGV = "rlinf.io/v1alpha1"

var machineNodeJoinGVK = schema.GroupVersionKind{Group: "rlinf.io", Version: "v1alpha1", Kind: "NodeJoin"}

type machineNodeJoinResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              struct {
		ClusterID string `json:"clusterID"`
		NodeName  string `json:"nodeName"`
	} `json:"spec,omitempty"`
	Status struct {
		Phase      string             `json:"phase,omitempty"`
		Stage      string             `json:"stage,omitempty"`
		Conditions []metav1.Condition `json:"conditions,omitempty"`
	} `json:"status,omitempty"`
}

func (o *machineNodeJoinResource) DeepCopyObject() runtime.Object {
	copy := *o
	copy.ObjectMeta = *o.ObjectMeta.DeepCopy()
	copy.Status.Conditions = append([]metav1.Condition(nil), o.Status.Conditions...)
	return &copy
}

type machineNodeJoinList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []machineNodeJoinResource `json:"items"`
}

func (l *machineNodeJoinList) DeepCopyObject() runtime.Object {
	copy := *l
	copy.ListMeta = *l.ListMeta.DeepCopy()
	copy.Items = make([]machineNodeJoinResource, len(l.Items))
	for i := range l.Items {
		copy.Items[i] = *l.Items[i].DeepCopyObject().(*machineNodeJoinResource)
	}
	return &copy
}

func registerMachineNodeJoinKinds(scheme *runtime.Scheme) error {
	if !scheme.Recognizes(machineNodeJoinGVK) {
		scheme.AddKnownTypeWithName(machineNodeJoinGVK, &machineNodeJoinResource{})
	}
	listGVK := machineNodeJoinGVK.GroupVersion().WithKind("NodeJoinList")
	if !scheme.Recognizes(listGVK) {
		scheme.AddKnownTypeWithName(listGVK, &machineNodeJoinList{})
	}
	metav1.AddToGroupVersion(scheme, machineNodeJoinGVK.GroupVersion())
	return nil
}

func (r *MachineReconciler) projectNodeJoin(ctx context.Context, machine *stationv1alpha1.Machine) (bool, error) {
	serverAdmission := machine.Spec.Admit != nil && machine.Spec.Admit.Type == stationv1alpha1.MachineTypeServer
	if !serverAdmission && machine.Status.NodeJoin == nil {
		return false, nil
	}
	joins := &machineNodeJoinList{}
	joins.SetGroupVersionKind(machineNodeJoinGVK.GroupVersion().WithKind("NodeJoinList"))
	if err := r.Reader.List(ctx, joins, client.MatchingLabels{MachineLabel: machine.Name}); err != nil {
		return true, fmt.Errorf("list NodeJoins for Machine %s: %w", machine.Name, err)
	}
	clusterID := ""
	if machine.Spec.Admit != nil {
		clusterID = machine.Spec.Admit.ClusterID
	}
	var selected *machineNodeJoinResource
	for i := range joins.Items {
		join := &joins.Items[i]
		if join.Annotations["station.rlark.io/machine"] != machine.Name || clusterID != "" && join.Spec.ClusterID != clusterID ||
			machine.Status.NodeName != "" && join.Spec.NodeName != machine.Status.NodeName {
			continue
		}
		if selected != nil {
			machine.Status.Problem, machine.Status.Message = stationv1alpha1.MachineJoinBlocked, "more than one NodeJoin claims this Machine; resolve the conflicting joins"
			machine.Status.NodeJoin = &stationv1alpha1.MachineNodeJoinStatus{Phase: "Conflict", Reason: "MultipleNodeJoins"}
			return true, nil
		}
		selected = join
	}
	if selected == nil {
		if !serverAdmission {
			machine.Status.Phase = stationv1alpha1.MachineDiscovered
			machine.Status.ClusterID, machine.Status.NodeName = "", ""
			machine.Status.Operation, machine.Status.NodeJoin = "", nil
			machine.Status.Problem, machine.Status.Message = "", ""
			return true, nil
		}
		machine.Status.Phase = stationv1alpha1.MachineAdmitted
		machine.Status.Problem, machine.Status.Message = stationv1alpha1.MachineJoinNotStarted, "Server admission exists but its NodeJoin has not been created"
		machine.Status.NodeJoin = &stationv1alpha1.MachineNodeJoinStatus{Phase: "Pending", Reason: "JoinNotStarted"}
		return true, nil
	}
	machine.Status.Operation = selected.Name
	machine.Status.ClusterID = selected.Spec.ClusterID
	machine.Status.NodeName = selected.Spec.NodeName
	projection := &stationv1alpha1.MachineNodeJoinStatus{Phase: selected.Status.Phase, Stage: selected.Status.Stage}
	for _, condition := range selected.Status.Conditions {
		if condition.Type == "Progressing" && condition.Status == metav1.ConditionFalse {
			projection.Reason = condition.Reason
			if condition.Message != "" {
				machine.Status.Message = condition.Message
			}
		}
	}
	machine.Status.NodeJoin = projection
	switch projection.Phase {
	case "Succeeded":
		machine.Status.Phase = stationv1alpha1.MachineJoined
		machine.Status.Problem, machine.Status.Message = "", ""
	case "Failed", "Blocked":
		machine.Status.Phase = stationv1alpha1.MachineAdmitted
		machine.Status.Problem = stationv1alpha1.MachineJoinBlocked
		if projection.Reason == "" {
			projection.Reason = "NodeJoinFailed"
		}
	default:
		machine.Status.Phase = stationv1alpha1.MachineAdmitted
		machine.Status.Problem = ""
	}
	return true, nil
}

func (r *MachineReconciler) collectCandidate(ctx context.Context, machine *stationv1alpha1.Machine) (reconcile.Result, error) {
	joins := &machineNodeJoinList{}
	joins.SetGroupVersionKind(machineNodeJoinGVK.GroupVersion().WithKind("NodeJoinList"))
	if err := r.Reader.List(ctx, joins, client.MatchingLabels{MachineNodeJoinLabel: machine.Name}); err != nil {
		return reconcile.Result{}, fmt.Errorf("check NodeJoins before collecting Machine %s: %w", machine.Name, err)
	}
	if len(joins.Items) != 0 {
		return reconcile.Result{RequeueAfter: time.Hour}, nil
	}
	// A concurrent admission or a fresh sighting must win over collection.
	if err := r.Client.Delete(ctx, machine, client.Preconditions{UID: &machine.UID, ResourceVersion: &machine.ResourceVersion}); err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, fmt.Errorf("collect expired Machine %s: %w", machine.Name, err)
	}
	return reconcile.Result{}, nil
}

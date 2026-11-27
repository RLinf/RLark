package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var errRobotModelNotFound = errors.New("NodeClass refers to an unknown RobotModel")

// effectiveNodeClassIdentity keeps existing classes readable while new
// classes use the RobotModel catalog as their authority.
func effectiveNodeClassIdentity(class *stationv1alpha1.NodeClass) (kind, model string) {
	kind, model = string(class.Spec.Kind), class.Spec.Model
	if model == "" {
		model = class.Spec.Labels["rlark.io/robot-model"]
	}
	if kind == "" && (model != "" || class.Spec.Labels["rlark.io/node-category"] == "robot") {
		kind = string(stationv1alpha1.NodeClassKindRobot)
	}
	return kind, model
}

func effectiveModel(class *stationv1alpha1.NodeClass) string {
	_, model := effectiveNodeClassIdentity(class)
	return model
}

// validateNodeClassModel validates an explicit catalog reference. Legacy
// classes inferred from labels remain compatible even before RobotModel is
// populated during rollout.
func validateNodeClassModel(ctx context.Context, reader client.Reader, class *stationv1alpha1.NodeClass) error {
	if class.Spec.Model == "" {
		return nil
	}
	model := &stationv1alpha1.RobotModel{}
	if err := reader.Get(ctx, types.NamespacedName{Name: class.Spec.Model}, model); apierrors.IsNotFound(err) {
		return errRobotModelNotFound
	} else if err != nil {
		return err
	}
	return nil
}

func machineIdentityCondition(ctx context.Context, reader client.Reader, admission *stationv1alpha1.NodeAdmission, observation *stationv1alpha1.NodeObservation, now time.Time, fresh bool) (metav1.Condition, error) {
	unknown := func(reason, message string) metav1.Condition {
		return metav1.Condition{Type: "IdentityVerified", Status: metav1.ConditionUnknown, Reason: reason, Message: message}
	}
	if !fresh {
		return unknown("ObservationStale", "identity report is outside the authenticated observation freshness window"), nil
	}
	if observation == nil || observation.Identity == nil || observation.Identity.Category == "" {
		return unknown("NotReported", "the authenticated node has not reported its machine type"), nil
	}
	wantKind, wantModel, desiredReason, err := desiredMachineIdentity(ctx, reader, admission)
	if err != nil {
		return metav1.Condition{}, err
	}
	if desiredReason != "" {
		return metav1.Condition{Type: "IdentityVerified", Status: metav1.ConditionFalse, Reason: desiredReason, Message: "the selected machine identity is incomplete or references an unknown RobotModel"}, nil
	}
	if wantKind == "" {
		return unknown("DesiredUnknown", "this legacy admission has no unambiguous desired machine identity"), nil
	}
	got := observation.Identity
	if got.Category != wantKind || wantModel != "" && got.Model != wantModel {
		return metav1.Condition{Type: "IdentityVerified", Status: metav1.ConditionFalse, Reason: "Mismatch",
			Message: fmt.Sprintf("selected identity is %s/%s; authenticated Node reports %s/%s", wantKind, wantModel, got.Category, got.Model)}, nil
	}
	return metav1.Condition{Type: "IdentityVerified", Status: metav1.ConditionTrue, Reason: "Matched", Message: "authenticated Node identity matches the selected machine identity"}, nil
}

func desiredMachineIdentity(ctx context.Context, reader client.Reader, admission *stationv1alpha1.NodeAdmission) (kind, model, reason string, err error) {
	if admission == nil {
		return "", "", "", nil
	}
	if selected := admission.Spec.Machine; selected != nil {
		kind, model = string(selected.Category), selected.Model
		if selected.Category == stationv1alpha1.MachineCategoryRobot && model != "" {
			catalog := &stationv1alpha1.RobotModel{}
			if err := reader.Get(ctx, types.NamespacedName{Name: model}, catalog); apierrors.IsNotFound(err) {
				return kind, model, "UnknownModel", nil
			} else if err != nil {
				return "", "", "", err
			}
		}
		return kind, model, "", nil
	}
	if admission.Spec.NodeClass != "" {
		class := &stationv1alpha1.NodeClass{}
		if err := reader.Get(ctx, types.NamespacedName{Name: admission.Spec.NodeClass}, class); apierrors.IsNotFound(err) {
			return "", "", "DesiredUnknown", nil
		} else if err != nil {
			return "", "", "", err
		}
		kind, model = effectiveNodeClassIdentity(class)
		if class.Spec.Model != "" {
			if err := validateNodeClassModel(ctx, reader, class); err != nil {
				if errors.Is(err, errRobotModelNotFound) {
					return kind, model, "UnknownModel", nil
				}
				return "", "", "", err
			}
		}
		if model != "" {
			return kind, model, "", nil
		}
	}
	labels := admission.Spec.Labels
	if model = labels["rlark.io/robot-model"]; model != "" || labels["rlark.io/node-category"] == "robot" {
		return "Robot", model, "", nil
	}
	if labels["station.rlark.io/machine-type"] == "server" || labels["rlark.io/node-category"] != "" {
		return "Server", labels["station.rlark.io/machine-model"], "", nil
	}
	return "", "", "", nil
}

func (r *MachineReconciler) observeMachineIdentity(ctx context.Context, machine *stationv1alpha1.Machine) error {
	status := &machine.Status
	if status.ClusterID == "" || status.NodeName == "" {
		return nil
	}
	namespace, err := r.Clusters.Namespace(ctx, status.ClusterID)
	if err != nil {
		return fmt.Errorf("resolve cluster %s for identity observation: %w", status.ClusterID, err)
	}
	enrollment := &stationv1alpha1.NetworkEnrollment{}
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: status.NodeName}, enrollment); apierrors.IsNotFound(err) {
		status.ObservedIdentity = nil
		status.IdentityVerified, status.IdentityReason = "Unknown", "NotReported"
		return nil
	} else if err != nil {
		return fmt.Errorf("read NetworkEnrollment %s/%s: %w", namespace, status.NodeName, err)
	}
	observation := enrollment.Status.Observation
	if observation == nil || observation.NodeUID != enrollment.Spec.NodeUID || observation.Identity == nil {
		status.ObservedIdentity = nil
		status.IdentityVerified, status.IdentityReason = "Unknown", "NotReported"
	} else {
		identity := observation.Identity
		status.ObservedIdentity = &stationv1alpha1.MachineIdentity{Category: identity.Category, Model: identity.Model, Source: identity.Source,
			ReportedAt: identity.ReportedAt.DeepCopy()}
		status.IdentityVerified, status.IdentityReason = "Unknown", "NotReported"
	}
	if enrollment.Status.Node != nil {
		for _, condition := range enrollment.Status.Node.Conditions {
			if condition.Type == "IdentityVerified" {
				status.IdentityVerified, status.IdentityReason = string(condition.Status), condition.Reason
				break
			}
		}
	}
	return nil
}

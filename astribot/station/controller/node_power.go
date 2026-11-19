package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// robotPowerStateLabel is, on a robot's Node, what its power is now: the
	// robot's model Addon (s1u-robot) writes it; a node without it is no robot
	// whose power RLark keeps.
	robotPowerStateLabel = "rlark.io/power-state"
	// robotPowerLabel and robotPowerSetAnnotation are, on the management
	// plane's Node, the power wanted of the robot and who set it last: what
	// the Gateway's robot-power API writes.
	robotPowerLabel         = "rlark.io/robot-power"
	robotPowerSetAnnotation = "rlark.io/robot-power-set"
	// nodeInUseAnnotation is, on a member Node, the marks of what uses the
	// machine now (a Run's Station, a session on the device layer).
	nodeInUseAnnotation = "rlark.io/in-use"
	// PowerOffAnnotation is, on a removal, what its PowerOff stage found.
	PowerOffAnnotation = "station.rlark.io/power-off"
)

// ErrNodeInUse stops the removal of a robot a Run or a session uses.
var ErrNodeInUse = errors.New("the robot is in use")

// RobotPowerOff powers a robot off before it is removed, the robot's own way:
// its wanted power is set to off, as the Gateway's robot-power API sets it,
// and its model Addon stops the driver, waits for the arms to sink and turns
// the motors off. The stage is done when the robot reads off. A robot whose
// node no longer reports is not powered off: the removal says so and goes on.
type RobotPowerOff struct {
	Members NodeMemberClientSource
	// Management reads and writes the management plane's Node and the operation.
	Management client.Client
	Now        func() time.Time
}

func (p RobotPowerOff) PowerOff(ctx context.Context, op *stationv1alpha1.NodeOperation) (bool, error) {
	member, err := p.Members.Client(ctx, op.Spec.ClusterID)
	if err != nil {
		return false, fmt.Errorf("connect member cluster: %w", err)
	}
	node, err := member.CoreV1().Nodes().Get(ctx, op.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("read node before power off: %w", err)
	}
	if string(node.UID) != op.Spec.NodeUID || node.DeletionTimestamp != nil {
		return false, ErrNodeIdentityChanged
	}
	state, robot := node.Labels[robotPowerStateLabel]
	switch {
	case !robot:
		return true, nil
	case !nodeReady(node):
		return true, p.note(ctx, op, "not powered off: the node no longer reports (power "+state+")")
	case node.Annotations[nodeInUseAnnotation] != "":
		return false, fmt.Errorf("%w: %s", ErrNodeInUse, node.Annotations[nodeInUseAnnotation])
	case state == "off":
		return true, p.note(ctx, op, "powered off")
	}
	wanted := &unstructured.Unstructured{}
	wanted.SetGroupVersionKind(rlarkNodeGVK)
	if err := p.Management.Get(ctx, types.NamespacedName{Namespace: op.Namespace, Name: op.Spec.NodeName}, wanted); err != nil && !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("read the robot's wanted power: %w", err)
	} else if err != nil {
		return false, fmt.Errorf("the management plane has no Node %s to set the robot's power on", op.Spec.NodeName)
	}
	if strings.HasPrefix(wanted.GetLabels()[robotPowerLabel], "off") {
		return false, nil
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	now = now.UTC()
	record, _ := json.Marshal(map[string]string{"desired": "off", "by": "removal " + op.Name + " (" + op.Spec.RequestedBy + ")", "at": now.Format(time.RFC3339)})
	patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{
		"labels":      map[string]string{robotPowerLabel: fmt.Sprintf("off.%d", now.Unix())},
		"annotations": map[string]string{robotPowerSetAnnotation: string(record)},
	}})
	if err := p.Management.Patch(ctx, wanted, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return false, fmt.Errorf("set the robot's wanted power to off: %w", err)
	}
	return false, p.note(ctx, op, "power off asked of the robot (was "+state+")")
}

// note writes on the operation what its PowerOff stage found.
func (p RobotPowerOff) note(ctx context.Context, op *stationv1alpha1.NodeOperation, text string) error {
	if op.Annotations[PowerOffAnnotation] == text {
		return nil
	}
	patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]string{PowerOffAnnotation: text}}})
	if err := p.Management.Patch(ctx, op.DeepCopy(), client.RawPatch(types.MergePatchType, patch)); err != nil {
		return fmt.Errorf("record the power off on the operation: %w", err)
	}
	return nil
}

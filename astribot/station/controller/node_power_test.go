package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func powerFixture(t *testing.T, node *corev1.Node) (RobotPowerOff, client.Client, *kubefake.Clientset, *stationv1alpha1.NodeOperation) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Name: "remove-s1u-03", Namespace: testNamespace},
		Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: node.Name, NodeUID: string(node.UID), RequestedBy: "alice"}}
	wanted := &unstructured.Unstructured{}
	wanted.SetGroupVersionKind(rlarkNodeGVK)
	wanted.SetNamespace(testNamespace)
	wanted.SetName(node.Name)
	wanted.SetLabels(map[string]string{robotPowerLabel: "ready.1794729000"})
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(op, wanted).Build()
	member := kubefake.NewSimpleClientset(node)
	at := time.Unix(1794830000, 0)
	return RobotPowerOff{Members: nodeMemberClients{"cluster-a": member}, Management: management, Now: func() time.Time { return at }}, management, member, op
}

func robotNode(state string, ready corev1.ConditionStatus, inUse string) *corev1.Node {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "s1u-03-x86", UID: "node-uid", Labels: map[string]string{robotPowerStateLabel: state}},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}}}}
	if inUse != "" {
		node.Annotations = map[string]string{nodeInUseAnnotation: inUse}
	}
	return node
}

func TestARobotIsPoweredOffItsOwnWayBeforeItIsRemoved(t *testing.T) {
	ctx := context.Background()
	node := robotNode("ready", corev1.ConditionTrue, "")
	power, management, member, op := powerFixture(t, node)

	// Its wanted power is set to off, as the Gateway's robot-power API sets it; the robot is not off yet.
	if done, err := power.PowerOff(ctx, op); done || err != nil {
		t.Fatalf("first pass: %v %v", done, err)
	}
	wanted := &unstructured.Unstructured{}
	wanted.SetGroupVersionKind(rlarkNodeGVK)
	_ = management.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: "s1u-03-x86"}, wanted)
	if wanted.GetLabels()[robotPowerLabel] != "off.1794830000" || !strings.Contains(wanted.GetAnnotations()[robotPowerSetAnnotation], `"by":"removal remove-s1u-03 (alice)"`) {
		t.Fatalf("wanted power: %v %v", wanted.GetLabels(), wanted.GetAnnotations())
	}
	// While the robot lowers itself nothing more is written; once it reads off the stage is done.
	if done, err := power.PowerOff(ctx, op); done || err != nil {
		t.Fatalf("while it lowers: %v %v", done, err)
	}
	node.Labels[robotPowerStateLabel] = "off"
	_, _ = member.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if done, err := power.PowerOff(ctx, op); !done || err != nil {
		t.Fatalf("off: %v %v", done, err)
	}
	stored := &stationv1alpha1.NodeOperation{}
	_ = management.Get(ctx, client.ObjectKeyFromObject(op), stored)
	if stored.Annotations[PowerOffAnnotation] != "powered off" {
		t.Fatalf("the operation's record: %v", stored.Annotations)
	}
}

func TestARobotInUseIsNotRemoved(t *testing.T) {
	power, _, _, op := powerFixture(t, robotNode("ready", corev1.ConditionTrue, "fleet-station"))
	if _, err := power.PowerOff(context.Background(), op); !errors.Is(err, ErrNodeInUse) || !strings.Contains(err.Error(), "fleet-station") {
		t.Fatalf("a robot a Run uses: %v", err)
	}
}

func TestAnUnreachableRobotIsRemovedWithANote(t *testing.T) {
	ctx := context.Background()
	power, management, _, op := powerFixture(t, robotNode("ready", corev1.ConditionUnknown, ""))
	if done, err := power.PowerOff(ctx, op); !done || err != nil {
		t.Fatalf("%v %v", done, err)
	}
	stored := &stationv1alpha1.NodeOperation{}
	_ = management.Get(ctx, client.ObjectKeyFromObject(op), stored)
	if !strings.Contains(stored.Annotations[PowerOffAnnotation], "no longer reports") {
		t.Fatalf("the operation's record: %v", stored.Annotations)
	}
}

func TestANodeThatIsNoRobotHasNoPowerToTurnOff(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "nb260706069", UID: "node-uid"}}
	power, _, _, op := powerFixture(t, node)
	if done, err := power.PowerOff(context.Background(), op); !done || err != nil {
		t.Fatalf("%v %v", done, err)
	}
}

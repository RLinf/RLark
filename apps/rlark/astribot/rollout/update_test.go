package rollout

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers"
)

var now = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

// A DaemonSet RLark rolls, at template generation 2 as the DaemonSet
// controller has seen it, for desired nodes, with policy as its annotation.
func rolledDaemonSet(policy string, desired int32) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "station", Namespace: "runtime", UID: "daemonset-uid", Generation: 2, Annotations: map[string]string{
			rlarkv1alpha1.RolloutAnnotation: policy, appsv1.DeprecatedTemplateGeneration: "2",
		}},
		Spec: appsv1.DaemonSetSpec{
			Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": "station"}},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType},
		},
		Status: appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: desired},
	}
}

// robot is a node: Ready or not, and what its node Agent last reported of its
// in-use directory (nil: nothing yet).
func robot(name string, ready bool, inUse *string) *corev1.Node {
	status := corev1.ConditionUnknown
	if ready {
		status = corev1.ConditionTrue
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: status}}}}
	if inUse != nil {
		node.Annotations = map[string]string{rlarkv1alpha1.NodeInUseAnnotation: *inUse}
	}
	return node
}

// instance is the DaemonSet's Pod on node, made from template generation, ready for an hour or not ready.
func instance(node, generation string, ready bool) *corev1.Pod {
	controller := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "station-" + node, Namespace: "runtime", UID: types.UID("pod-" + node),
			Labels:          map[string]string{"app": "station", templateGenerationLabel: generation},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: "station", UID: "daemonset-uid", Controller: &controller}},
		},
		Spec: corev1.PodSpec{NodeName: node},
	}
	if ready {
		pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(now.Add(-time.Hour))}}
	}
	return pod
}

// fleet is n ready, idle nodes named 00, 01, ... with a Pod each: the first updated of them run
// the new template, the rest the old one.
func fleet(n, updated int) []client.Object {
	idle := ""
	var objects []client.Object
	for i := range n {
		name := fmt.Sprintf("%02d", i)
		generation := "1"
		if i < updated {
			generation = "2"
		}
		objects = append(objects, robot(name, true, &idle), instance(name, generation, true))
	}
	return objects
}

func TestUpdaterReplacesOutdatedPods(t *testing.T) {
	idle, working := "", "fleet-station"
	const defaults, whenIdle = `{}`, `{"whenIdle":true}`
	terminating := func(pod *corev1.Pod) *corev1.Pod {
		deleted := metav1.NewTime(now)
		pod.DeletionTimestamp, pod.Finalizers = &deleted, []string{"test/held"}
		return pod
	}
	justReady := func(pod *corev1.Pod) *corev1.Pod {
		pod.Status.Conditions[0].LastTransitionTime = metav1.NewTime(now.Add(-10 * time.Second))
		return pod
	}
	tests := []struct {
		name            string
		policy          string
		minReadySeconds int32
		objects         []client.Object
		replaced        []string
		pending         bool
	}{
		{
			name:   "a node that is off neither holds the others back nor is touched: the canary goes first",
			policy: defaults, pending: true,
			objects:  []client.Object{robot("a", true, &idle), instance("a", "1", true), robot("b", false, &idle), instance("b", "1", true), robot("c", true, &idle), instance("c", "1", true)},
			replaced: []string{"a"},
		},
		{
			name:   "a Pod left terminating on a node that is off uses up nothing",
			policy: defaults, pending: true,
			objects:  []client.Object{robot("a", true, &idle), instance("a", "2", true), robot("b", false, &idle), terminating(instance("b", "1", true)), robot("c", true, &idle), instance("c", "1", true)},
			replaced: []string{"c"},
		},
		{
			name:   "the node that was off is back: it is one more node to replace",
			policy: defaults, pending: true,
			objects:  []client.Object{robot("a", true, &idle), instance("a", "2", true), robot("b", true, &idle), instance("b", "1", true), robot("c", true, &idle), instance("c", "2", true)},
			replaced: []string{"b"},
		},
		{
			name:   "one node first",
			policy: defaults, pending: true,
			objects:  fleet(100, 0),
			replaced: []string{"00"},
		},
		{
			name:   "nothing more while the canary's new Pod is not available",
			policy: defaults, pending: true,
			objects: append(fleet(100, 0)[2:], robot("00", true, &idle), instance("00", "2", false)),
		},
		{
			name:   "nor before it has been ready for the DaemonSet's minReadySeconds",
			policy: defaults, minReadySeconds: 150, pending: true,
			objects: append(fleet(100, 0)[2:], robot("00", true, &idle), justReady(instance("00", "2", true))),
		},
		{
			name:   "after the canary a tenth of the ready nodes at a time",
			policy: defaults, pending: true,
			objects:  fleet(100, 1),
			replaced: []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "10"},
		},
		{
			name:   "new Pods that do not become available use up the bound, so a rollout that breaks stops",
			policy: defaults, pending: true,
			objects: func() []client.Object {
				objects := fleet(100, 11)
				for i := 1; i <= 10; i++ {
					objects[2*i+1] = instance(fmt.Sprintf("%02d", i), "2", false)
				}
				return objects
			}(),
		},
		{
			name:   "the policy's own canary and bound",
			policy: `{"canary":2,"maxUnavailable":3}`, pending: true,
			objects:  fleet(10, 0),
			replaced: []string{"00", "01"},
		},
		{
			name:   "a bound of everything, after the canary",
			policy: `{"maxUnavailable":"100%"}`, pending: true,
			objects:  fleet(10, 1),
			replaced: []string{"01", "02", "03", "04", "05", "06", "07", "08", "09"},
		},
		{
			name:   "an outdated Pod that does not run anyway is replaced beside the bound, after the canary",
			policy: defaults, pending: true,
			objects:  []client.Object{robot("a", true, &idle), instance("a", "2", true), robot("b", true, &idle), instance("b", "1", false), robot("c", true, &idle), instance("c", "1", true)},
			replaced: []string{"b"},
		},
		{
			name:   "the canary is a Pod that ran, not one that did not run before either",
			policy: defaults, pending: true,
			objects:  []client.Object{robot("a", true, &idle), instance("a", "1", false), robot("b", true, &idle), instance("b", "1", true)},
			replaced: []string{"b"},
		},
		{
			name:   "whenIdle: a node in use and one not reported yet keep their Pods",
			policy: whenIdle, pending: true,
			objects:  []client.Object{robot("a", true, &working), instance("a", "1", true), robot("b", true, nil), instance("b", "1", true), robot("c", true, &idle), instance("c", "1", true)},
			replaced: []string{"c"},
		},
		{
			name:   "without whenIdle a node in use is replaced like any other",
			policy: defaults, pending: true,
			objects:  []client.Object{robot("a", true, &working), instance("a", "1", true)},
			replaced: []string{"a"},
		},
		{
			name:    "every Pod runs the current template",
			policy:  defaults,
			objects: fleet(3, 3),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			daemonSet := rolledDaemonSet(test.policy, int32(len(test.objects)/2))
			daemonSet.Spec.MinReadySeconds = test.minReadySeconds
			local := fake.NewClientBuilder().WithScheme(controllers.MgmtScheme).WithObjects(append(test.objects, daemonSet)...).Build()
			result, err := (&Updater{Local: local, Now: func() time.Time { return now }}).Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(daemonSet)})
			if err != nil {
				t.Fatal(err)
			}
			if (result.RequeueAfter > 0) != test.pending {
				t.Fatalf("result=%#v, pending=%v", result, test.pending)
			}
			var pods corev1.PodList
			if err := local.List(context.Background(), &pods); err != nil {
				t.Fatal(err)
			}
			left := map[string]bool{}
			for _, pod := range pods.Items {
				left[pod.Spec.NodeName] = true
			}
			var replaced []string
			for _, object := range test.objects {
				if pod, isPod := object.(*corev1.Pod); isPod && !left[pod.Spec.NodeName] {
					replaced = append(replaced, pod.Spec.NodeName)
				}
			}
			sort.Strings(replaced)
			if !reflect.DeepEqual(replaced, test.replaced) {
				t.Fatalf("replaced=%v, want %v", replaced, test.replaced)
			}
		})
	}
}

// A DaemonSet that rolls by itself, and one that says OnDelete without the annotation (its Pods are replaced by
// hand), are not the Updater's.
func TestUpdaterLeavesOtherDaemonSets(t *testing.T) {
	for _, change := range []func(*appsv1.DaemonSet){
		func(d *appsv1.DaemonSet) { d.Spec.UpdateStrategy.Type = appsv1.RollingUpdateDaemonSetStrategyType },
		func(d *appsv1.DaemonSet) { delete(d.Annotations, rlarkv1alpha1.RolloutAnnotation) },
	} {
		daemonSet := rolledDaemonSet(`{}`, 1)
		change(daemonSet)
		idle := ""
		pod := instance("a", "1", true)
		local := fake.NewClientBuilder().WithScheme(controllers.MgmtScheme).WithObjects(daemonSet, robot("a", true, &idle), pod).Build()
		if _, err := (&Updater{Local: local}).Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(daemonSet)}); err != nil {
			t.Fatal(err)
		}
		if err := local.Get(context.Background(), client.ObjectKeyFromObject(pod), &corev1.Pod{}); err != nil {
			t.Fatalf("a Pod of a DaemonSet RLark does not roll was deleted: %v", err)
		}
	}
}

func TestUpdaterWaitsForTheDaemonSetControllerToSeeANewTemplate(t *testing.T) {
	idle := ""
	daemonSet := rolledDaemonSet(`{}`, 1)
	// The template changed again; the Pods' generation labels still compare with the one before.
	daemonSet.Generation = 3
	pod := instance("a", "1", true)
	local := fake.NewClientBuilder().WithScheme(controllers.MgmtScheme).WithObjects(daemonSet, robot("a", true, &idle), pod).Build()
	result, err := (&Updater{Local: local}).Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(daemonSet)})
	if err != nil || result.RequeueAfter == 0 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(pod), &corev1.Pod{}); err != nil {
		t.Fatalf("the Pod was deleted before the DaemonSet controller saw the template: %v", err)
	}
}

// A node that does not hold the new template's images keeps its Pod until a pull Pod has pulled them there.
func TestUpdaterPullsTheNewImagesBeforeItReplaces(t *testing.T) {
	const station, wireguard = "registry.rlark.internal:5443/rlark/station-agent@sha256:8480ceda", "registry.rlark.internal:5443/rlark/wireguard@sha256:1e91f510"
	idle := ""
	withImages := func(daemonSet *appsv1.DaemonSet) *appsv1.DaemonSet {
		daemonSet.Spec.Template.Spec = corev1.PodSpec{
			HostNetwork: true, ImagePullSecrets: []corev1.LocalObjectReference{{Name: "ir-site"}},
			InitContainers: []corev1.Container{{Name: "setup", Image: station, ImagePullPolicy: corev1.PullNever}},
			Containers:     []corev1.Container{{Name: "station-agent", Image: station, ImagePullPolicy: corev1.PullNever}, {Name: "wireguard", Image: wireguard, ImagePullPolicy: corev1.PullNever}},
		}
		return daemonSet
	}
	holding := func(node *corev1.Node, images ...string) *corev1.Node {
		for _, image := range images {
			node.Status.Images = append(node.Status.Images, corev1.ContainerImage{Names: []string{"10.254.240.1:5443/rlark/x@" + strings.SplitN(image, "@", 2)[1]}})
		}
		return node
	}
	pull := func(node, generation string, states ...corev1.ContainerState) *corev1.Pod {
		pod := pullPod(withImages(rolledDaemonSet(`{}`, 1)), node, []string{station, wireguard})
		pod.Labels[pullTemplateLabel] = generation
		for i, state := range states {
			pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, corev1.ContainerStatus{Name: fmt.Sprintf("image-%d", i), State: state})
		}
		return pod
	}
	ended := corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "StartError", ExitCode: 128}}
	pulling := corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}}
	failing := corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "unauthorized"}}
	tests := []struct {
		name     string
		objects  []client.Object
		replaced bool
		pulls    []string // the pull Pods left, by node and template generation
	}{
		{name: "the node lacks them: a pull Pod is made, the old Pod stays", objects: []client.Object{robot("a", true, &idle), instance("a", "1", true)}, pulls: []string{"a/2"}},
		{name: "still pulling", objects: []client.Object{robot("a", true, &idle), instance("a", "1", true), pull("a", "2", ended, pulling)}, pulls: []string{"a/2"}},
		{name: "a pull that fails keeps the old Pod", objects: []client.Object{robot("a", true, &idle), instance("a", "1", true), pull("a", "2", ended, failing)}, pulls: []string{"a/2"}},
		{name: "pulled: the old Pod and the pull Pod go", objects: []client.Object{robot("a", true, &idle), instance("a", "1", true), pull("a", "2", ended, ended)}, replaced: true},
		{name: "a node that lists them needs no pull", objects: []client.Object{holding(robot("a", true, &idle), station, wireguard), instance("a", "1", true)}, replaced: true},
		{name: "a pull Pod of an earlier template goes, one of this template is made", objects: []client.Object{robot("a", true, &idle), instance("a", "1", true), pull("a", "1", ended, ended)}, pulls: []string{"a/2"}},
		{name: "nothing outdated: no pull Pod is left", objects: []client.Object{robot("a", true, &idle), instance("a", "2", true), pull("a", "2", ended, ended)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			daemonSet := withImages(rolledDaemonSet(`{}`, 1))
			local := fake.NewClientBuilder().WithScheme(controllers.MgmtScheme).WithObjects(append(test.objects, daemonSet)...).Build()
			if _, err := (&Updater{Local: local, Now: func() time.Time { return now }}).Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(daemonSet)}); err != nil {
				t.Fatal(err)
			}
			var pods corev1.PodList
			if err := local.List(context.Background(), &pods); err != nil {
				t.Fatal(err)
			}
			replaced, pulls := true, []string{}
			for _, pod := range pods.Items {
				switch {
				case pod.Name == "station-a":
					replaced = false
				case pod.Labels[PullForLabel] == "station":
					pulls = append(pulls, pod.Spec.NodeName+"/"+pod.Labels[pullTemplateLabel])
					if want := []string{station, wireguard}; len(pod.Spec.Containers) != 2 || pod.Spec.Containers[0].Image != want[0] || pod.Spec.Containers[1].Image != want[1] ||
						pod.Spec.Containers[0].ImagePullPolicy != corev1.PullIfNotPresent || !pod.Spec.HostNetwork || pod.Spec.ImagePullSecrets[0].Name != "ir-site" || pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
						t.Fatalf("pull Pod %#v", pod.Spec)
					}
				}
			}
			if replaced != test.replaced || !reflect.DeepEqual(pulls, append([]string{}, test.pulls...)) {
				t.Fatalf("replaced=%v pulls=%v, want %v %v", replaced, pulls, test.replaced, test.pulls)
			}
		})
	}
}

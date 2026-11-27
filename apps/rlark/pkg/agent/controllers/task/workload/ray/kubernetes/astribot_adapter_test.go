package kubernetes_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	compatibility "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes"
	kubernetesworkload "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

func TestRayKubernetesAdapterRendersPerNodeTaskAsDaemonSet(t *testing.T) {
	for _, test := range []struct {
		name       string
		parameters string
		want       string
	}{
		// The DaemonSet replaces nothing by itself; the policy is kept on it for the cluster Agent's rollout.
		{name: "default", want: `{}`},
		{name: "percentage", parameters: `{"maxUnavailable":"25%"}`, want: `{"maxUnavailable":"25%"}`},
		{name: "count and canary", parameters: `{"canary":2,"maxUnavailable":3}`, want: `{"canary":2,"maxUnavailable":3}`},
		{name: "when idle", parameters: `{"whenIdle":true,"maxUnavailable":"10%"}`, want: `{"maxUnavailable":"10%","whenIdle":true}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			task := compileTask(kubernetesworkload.CarrierDaemonSet)
			task.Spec.Workload.Carrier.Parameters = apiextensionsv1.JSON{Raw: []byte(test.parameters)}
			plan, err := kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace).Compile(context.Background(), workload.CompileRequest{
				Task:    mustSnapshot(t, task),
				Runtime: agentruntime.Info{Kind: compatibility.RuntimeKindKubernetes},
			})
			if err != nil {
				t.Fatal(err)
			}
			primary := findObject(t, plan.Objects, "workload")
			if primary.Kind != (agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}) || plan.Units[0].Locator.Type != primary.Kind {
				t.Fatalf("primary kind=%#v unit=%#v", primary.Kind, plan.Units[0])
			}
			var daemonSet appsv1.DaemonSet
			if err := json.Unmarshal(primary.Manifest.Object, &daemonSet); err != nil {
				t.Fatal(err)
			}
			if strategy := daemonSet.Spec.UpdateStrategy; strategy.Type != appsv1.OnDeleteDaemonSetStrategyType || strategy.RollingUpdate != nil || daemonSet.Annotations[rlarkv1alpha1.RolloutAnnotation] != test.want {
				t.Fatalf("update strategy=%#v rollout=%s", strategy, daemonSet.Annotations[rlarkv1alpha1.RolloutAnnotation])
			}
			// The class of nodes is the Task's node selector; the Pods carry the
			// Task incarnation the selector and the instance projection rely on.
			pod := daemonSet.Spec.Template
			if pod.Spec.NodeSelector["zone"] != "edge" || pod.Spec.NodeSelector["rlark.io/enabled"] != "true" {
				t.Fatalf("node selector=%#v", pod.Spec.NodeSelector)
			}
			if daemonSet.Spec.Selector.MatchLabels[kubernetesworkload.ManagementTaskUIDLabel] != "task-uid" || pod.Labels[kubernetesworkload.ManagementTaskUIDLabel] != "task-uid" {
				t.Fatalf("selector=%#v labels=%#v", daemonSet.Spec.Selector, pod.Labels)
			}
			if daemonSet.Annotations[kubernetesworkload.ManagementTaskUIDAnnotation] != "task-uid" || daemonSet.Name != "task" || daemonSet.Namespace != "runtime" {
				t.Fatalf("metadata=%#v", daemonSet.ObjectMeta)
			}
			if len(pod.Spec.Tolerations) != 0 {
				t.Fatalf("tolerations were added: %#v", pod.Spec.Tolerations)
			}
		})
	}
}

// daemonSetInstance is a per-node instance as the Kubernetes provider reports
// it: its node, whether that node is Ready, the template generation its Pod was
// made from, whether it is ready, and a container waiting reason.
func daemonSetInstance(node string, nodeReady bool, generation string, ready bool, waiting string) agentruntime.Instance {
	name := "task-" + node
	conditions := []agentruntime.InstanceCondition{{Scope: "node", Subject: node}, {Scope: "revision", Subject: name, Message: generation}}
	if !nodeReady {
		conditions = append(conditions, agentruntime.InstanceCondition{Scope: "node", Subject: node, Reason: "NotReady"})
	}
	if ready {
		conditions = append(conditions, agentruntime.InstanceCondition{Scope: "instance", Subject: name, Reason: "Ready"})
	}
	if waiting != "" {
		conditions = append(conditions, agentruntime.InstanceCondition{Scope: "container", Subject: "main", Reason: waiting})
	}
	return agentruntime.Instance{Name: name, Phase: "Running", Conditions: conditions}
}

func TestObservePerNodeTaskCountsOnlyInstancesOnReadyNodes(t *testing.T) {
	current := func(node string) agentruntime.Instance { return daemonSetInstance(node, true, "2", true, "") }
	inUse := func(instance agentruntime.Instance) agentruntime.Instance {
		instance.Conditions = append(instance.Conditions, agentruntime.InstanceCondition{Scope: "node", Reason: "InUse", Message: "fleet-station"})
		return instance
	}
	tests := []struct {
		name       string
		generation int64
		whenIdle   bool
		instances  []agentruntime.Instance
		phase      rlarkv1alpha1.TaskPhase
		message    string
	}{
		{name: "every ready node runs the current version", instances: []agentruntime.Instance{current("a"), current("b")}, phase: rlarkv1alpha1.TaskPhaseRunning},
		{name: "spec change not yet seen by the DaemonSet", generation: 3, instances: []agentruntime.Instance{current("a")}, phase: rlarkv1alpha1.TaskPhasePending},
		{name: "node switched off", instances: []agentruntime.Instance{current("a"), daemonSetInstance("b", false, "2", false, "")},
			phase: rlarkv1alpha1.TaskPhaseRunning, message: "1 of 2 instances on nodes that are not ready"},
		{name: "ready node crash-looping", instances: []agentruntime.Instance{current("a"), daemonSetInstance("b", true, "2", false, "CrashLoopBackOff")},
			phase: rlarkv1alpha1.TaskPhasePending, message: "pod task-b container main: CrashLoopBackOff"},
		{name: "joining node not yet ready", instances: []agentruntime.Instance{current("a"), daemonSetInstance("c", false, "2", false, "ContainerCreating")},
			phase: rlarkv1alpha1.TaskPhaseRunning, message: "1 of 2 instances on nodes that are not ready"},
		{name: "joining ready node pulling its image", instances: []agentruntime.Instance{current("a"), current("b"), daemonSetInstance("c", true, "2", false, "ContainerCreating")},
			phase: rlarkv1alpha1.TaskPhasePending, message: "2 of 3 instances on ready nodes updated and available"},
		{name: "update rolling out", instances: []agentruntime.Instance{current("a"), daemonSetInstance("b", true, "1", true, "")},
			phase: rlarkv1alpha1.TaskPhasePending, message: "1 of 2 instances on ready nodes updated and available"},
		{name: "crash-looping instance on a node switched off", instances: []agentruntime.Instance{current("a"), daemonSetInstance("b", false, "2", false, "CrashLoopBackOff")},
			phase: rlarkv1alpha1.TaskPhaseRunning, message: "1 of 2 instances on nodes that are not ready"},
		{name: "no node in the class", phase: rlarkv1alpha1.TaskPhaseRunning, message: "no node matches the Task's node selector"},
		// Updated only on idle nodes: the old version at work on a node in use is where it should be.
		{name: "old version on a node in use", whenIdle: true, instances: []agentruntime.Instance{current("a"), inUse(daemonSetInstance("b", true, "1", true, ""))},
			phase: rlarkv1alpha1.TaskPhaseRunning, message: "1 of 2 instances on ready nodes wait for their nodes to be idle before they are updated"},
		{name: "old version on an idle node", whenIdle: true, instances: []agentruntime.Instance{current("a"), daemonSetInstance("b", true, "1", true, "")},
			phase: rlarkv1alpha1.TaskPhasePending, message: "1 of 2 instances on ready nodes updated and available"},
		{name: "old version failing on a node in use", whenIdle: true, instances: []agentruntime.Instance{current("a"), inUse(daemonSetInstance("b", true, "1", false, "ContainerCreating"))},
			phase: rlarkv1alpha1.TaskPhasePending, message: "1 of 2 instances on ready nodes updated and available"},
		{name: "rolling update does not wait for a node in use", instances: []agentruntime.Instance{current("a"), inUse(daemonSetInstance("b", true, "1", true, ""))},
			phase: rlarkv1alpha1.TaskPhasePending, message: "1 of 2 instances on ready nodes updated and available"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generation := test.generation
			if generation == 0 {
				generation = 2
			}
			object := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Generation: generation, Annotations: map[string]string{appsv1.DeprecatedTemplateGeneration: "2", rlarkv1alpha1.RolloutAnnotation: rlarkv1alpha1.RolloutPolicy{WhenIdle: test.whenIdle}.Annotation()}},
				Spec:   appsv1.DaemonSetSpec{UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType}},
				Status: appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: int32(len(test.instances))}}
			manifest, _ := json.Marshal(object)
			phase, message, err := kubernetesworkload.ObserveObject(kubernetesworkload.CarrierDaemonSet, agentruntime.Object{Manifest: agentruntime.TypedManifest{Object: manifest}}, agentruntime.InstanceSnapshot{Instances: test.instances})
			if err != nil {
				t.Fatal(err)
			}
			if phase != test.phase || message != test.message {
				t.Fatalf("got (%q,%q), want (%q,%q)", phase, message, test.phase, test.message)
			}
		})
	}
}

func TestRayKubernetesAdapterRunsTaskWithoutRayRoleThroughLauncherOnly(t *testing.T) {
	task := compileTask(kubernetesworkload.CarrierDaemonSet)
	parameters, err := json.Marshal(taskapi.RayExecutionParametersV1{Prepare: &taskapi.InlineScript{Inline: "station"}})
	if err != nil {
		t.Fatal(err)
	}
	task.Spec.Workload.Parameters.Raw = parameters
	plan, err := kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace).Compile(context.Background(), workload.CompileRequest{
		Task:    mustSnapshot(t, task),
		Runtime: agentruntime.Info{Kind: compatibility.RuntimeKindKubernetes},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Processes) != 1 || !reflect.DeepEqual(plan.Processes[0].Args, []string{"ray-launcher"}) {
		t.Fatalf("processes=%#v, want ray-launcher without a role", plan.Processes)
	}
	for _, object := range plan.Objects {
		if object.LogicalName == "ray-service" {
			t.Fatal("a Task without a Ray role got a Ray head Service")
		}
	}
	var daemonSet appsv1.DaemonSet
	if err := json.Unmarshal(findObject(t, plan.Objects, "workload").Manifest.Object, &daemonSet); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{}
	for _, variable := range daemonSet.Spec.Template.Spec.Containers[0].Env {
		environment[variable.Name] = variable.Value
	}
	if _, found := environment["RLARK_HEAD_ADDRESS"]; found || environment["RLARK_PREPARE_SCRIPT"] != "station" {
		t.Fatalf("environment=%#v", environment)
	}
}

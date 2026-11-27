package job

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
)

// perNodeTemplate is a per-node Task as a tenant submits it: one instance on
// every node of the robot class.
func perNodeTemplate(name string) rlarkv1alpha1.JobTaskTemplate {
	return rlarkv1alpha1.JobTaskTemplate{Name: name, TaskSpec: rlarkv1alpha1.TaskSpec{
		Placement: rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"rlark.io/node-category": "robot"}, PerNode: true},
		Runtime:   rlarkv1alpha1.TaskRuntimeSpec{Kind: rlarkv1alpha1.RuntimeKindKubernetes},
		Workload: rlarkv1alpha1.TaskWorkloadSpec{
			Kind:     taskapi.WorkloadTypeRay,
			Version:  taskapi.WorkloadVersionRayV1,
			Carrier:  rlarkv1alpha1.TaskWorkloadCarrier{Type: taskapi.CarrierTypeKubernetesDaemonSet, Version: taskapi.CarrierVersionKubernetesV1},
			Template: &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "station:v1"}}}},
		},
	}}
}

func TestValidatePerNodeTaskTemplates(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*rlarkv1alpha1.JobTaskTemplate)
		wantError string
	}{
		{name: "per-node Task"},
		{name: "rollout bound", mutate: func(t *rlarkv1alpha1.JobTaskTemplate) {
			t.Workload.Carrier.Parameters = apiextensionsv1.JSON{Raw: []byte(`{"maxUnavailable":"20%"}`)}
		}},
		{name: "updated only on idle nodes", mutate: func(t *rlarkv1alpha1.JobTaskTemplate) {
			t.Workload.Carrier.Parameters = apiextensionsv1.JSON{Raw: []byte(`{"whenIdle":true}`)}
		}},
		{name: "no node class", mutate: func(t *rlarkv1alpha1.JobTaskTemplate) { t.Placement.NodeSelector = nil },
			wantError: "spec.tasks[0].placement.perNode requires placement.nodeSelector to name the class of nodes"},
		{name: "head", mutate: func(t *rlarkv1alpha1.JobTaskTemplate) { t.Head = true },
			wantError: "spec.tasks[0].head must be false for a per-node Task"},
		{name: "replicated carrier", mutate: func(t *rlarkv1alpha1.JobTaskTemplate) {
			t.Workload.Carrier.Type = taskapi.CarrierTypeKubernetesStatefulSet
		}, wantError: "spec.tasks[0].workload.carrier must be rlinf.io/kubernetes-daemonset v1 for a per-node Task"},
		{name: "DaemonSet carrier without perNode", mutate: func(t *rlarkv1alpha1.JobTaskTemplate) { t.Placement.PerNode = false },
			wantError: "spec.tasks[0].workload.carrier rlinf.io/kubernetes-daemonset requires placement.perNode"},
		{name: "replicas", mutate: func(t *rlarkv1alpha1.JobTaskTemplate) {
			t.Workload.Carrier.Parameters = apiextensionsv1.JSON{Raw: []byte(`{"replicas":2}`)}
		}, wantError: `spec.tasks[0].workload.carrier.parameters: decode task extension parameters: json: unknown field "replicas"`},
		{name: "zero maxUnavailable", mutate: func(t *rlarkv1alpha1.JobTaskTemplate) {
			t.Workload.Carrier.Parameters = apiextensionsv1.JSON{Raw: []byte(`{"maxUnavailable":"0%"}`)}
		}, wantError: "spec.tasks[0].workload.carrier.parameters: maxUnavailable must be a positive integer or a percentage from 1% to 100%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := perNodeTemplate("station")
			if tt.mutate != nil {
				tt.mutate(&template)
			}
			err := validateTaskTemplates(&rlarkv1alpha1.Job{Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}})
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("validateTaskTemplates() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantError {
				t.Fatalf("validateTaskTemplates() error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func robotNode(name, namespace, category string, phase rlarkv1alpha1.NodePhase) *rlarkv1alpha1.Node {
	return &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"rlark.io/node-category": category}}, Status: rlarkv1alpha1.NodeStatus{Phase: phase}}
}

// A per-node Task goes to the cluster of its class of nodes even when every
// node of the class is offline and the cluster is cordoned: the class, not the
// nodes that are up right now, decides where it lives.
func TestPerNodeTaskGoesToTheClusterOfItsNodes(t *testing.T) {
	template := perNodeTemplate("station")
	template.Placement.NodeSelector["rlark.io/node-category"] = "robot,arm"
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "stations", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}, Status: rlarkv1alpha1.JobStatus{Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "station"}}}}
	cluster := &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "robots"}, Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "robots", Cordoned: true}, Status: rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterCordoned}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(cluster,
		robotNode("s1u-01", "robots", "robot", rlarkv1alpha1.NodeOffline),
		robotNode("arm-01", "robots", "arm", rlarkv1alpha1.NodeOffline),
		robotNode("gpu-01", "cloud", "gpu", rlarkv1alpha1.NodeOnline),
	).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	if _, err := r.dispatchTasks(context.Background(), job, logr.Discard()); err != nil {
		t.Fatal(err)
	}
	var task rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKey{Name: "stations-station", Namespace: "robots"}, &task); err != nil {
		t.Fatal(err)
	}
	if !task.Spec.Placement.PerNode || task.Spec.Workload.Carrier.Type != taskapi.CarrierTypeKubernetesDaemonSet {
		t.Fatalf("Task spec=%#v", task.Spec)
	}
}

func TestPerNodeTaskRejectsAClassSpanningClusters(t *testing.T) {
	template := perNodeTemplate("station")
	scheme := jobTestScheme(t)
	for _, tt := range []struct {
		name  string
		nodes []client.Object
		want  string
	}{
		{name: "two clusters", nodes: []client.Object{robotNode("s1u-01", "site-a", "robot", rlarkv1alpha1.NodeOnline), robotNode("s1u-02", "site-b", "robot", rlarkv1alpha1.NodeOnline)}, want: "matches Nodes of 2 clusters"},
		{name: "no node", nodes: []client.Object{robotNode("gpu-01", "cloud", "gpu", rlarkv1alpha1.NodeOnline)}, want: "no Nodes match selector"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{Client: jobTestClientBuilder(scheme).WithObjects(tt.nodes...).Build(), Scheme: scheme}
			if _, err := r.resolveTaskNamespace(context.Background(), &template); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("resolveTaskNamespace() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// The Ray head waits for a fixed number of nodes and gives ranks by position;
// a per-node Task's instance count changes with its class, so it counts for
// neither.
func TestPerNodeTaskIsNotCountedInRayTopology(t *testing.T) {
	two := int32(2)
	head := rayTaskTemplate(t, "head", true, nil, "")
	station := perNodeTemplate("station")
	workers := rayTaskTemplate(t, "workers", false, &two, "")
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "run"}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{head, station, workers}}}
	if total := totalNodeCount(job.Spec.Tasks); total != 3 {
		t.Fatalf("totalNodeCount() = %d, want 3", total)
	}
	if rank := rankStartForTask(job, "workers"); rank != 1 {
		t.Fatalf("rankStartForTask(workers) = %d, want 1", rank)
	}
	built := buildTask(job, station, "run-station", "robots")
	parameters, err := taskapi.DecodeParametersStrict[taskapi.RayExecutionParametersV1](built.Spec.Workload.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if parameters.Role != taskapi.RayRoleWorker || parameters.HeadTaskName != "run-head" || parameters.TotalNodes != 3 {
		t.Fatalf("per-node Ray parameters = %#v", parameters)
	}
}

// A Job without a head forms no Ray cluster, so its Tasks get no Ray role and
// the launcher only runs their scripts instead of waiting for a head.
func TestTaskOfJobWithoutHeadHasNoRayRole(t *testing.T) {
	prepare, err := json.Marshal(taskapi.RayExecutionParametersV1{Prepare: &taskapi.InlineScript{Inline: "station"}})
	if err != nil {
		t.Fatal(err)
	}
	station := perNodeTemplate("station")
	station.Workload.Parameters = apiextensionsv1.JSON{Raw: prepare}
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "stations"}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{station}}}
	built := buildTask(job, station, "stations-station", "robots")
	parameters, err := taskapi.DecodeParametersStrict[taskapi.RayExecutionParametersV1](built.Spec.Workload.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if parameters.Role != "" || parameters.HeadTaskName != "" || parameters.Prepare == nil || parameters.Prepare.Inline != "station" {
		t.Fatalf("Ray parameters = %#v", parameters)
	}
	if strings.Contains(string(built.Spec.Workload.Parameters.Raw), `"role"`) {
		t.Fatalf("parameters carry a role: %s", built.Spec.Workload.Parameters.Raw)
	}
	if _, found := built.Annotations[rlarkv1alpha1.RayRoleAnnotation]; found {
		t.Fatalf("annotations = %#v, want no Ray role", built.Annotations)
	}
}

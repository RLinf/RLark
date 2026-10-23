package compatibility

import (
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
)

func TestDecodeTaskSeparatesWorkloadAndCarrier(t *testing.T) {
	carriers := []string{
		taskapi.CarrierTypeKubernetesDeployment,
		taskapi.CarrierTypeKubernetesDaemonSet,
		taskapi.CarrierTypeKubernetesStatefulSet,
	}
	for _, carrier := range carriers {
		t.Run(carrier, func(t *testing.T) {
			task := canonicalTask(carrier)
			snapshot, err := DecodeTask(task)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.WorkloadSelector.Type != taskapi.WorkloadTypeRay || snapshot.WorkloadSelector.Version != taskapi.WorkloadVersionRayV1 {
				t.Fatalf("workload selector = %#v", snapshot.WorkloadSelector)
			}
			if snapshot.CarrierSelector.Type != carrier || snapshot.CarrierSelector.Version != taskapi.CarrierVersionKubernetesV1 {
				t.Fatalf("carrier selector = %#v", snapshot.CarrierSelector)
			}
			if string(snapshot.Configuration.Parameters) != `{"role":"head"}` {
				t.Fatalf("workload parameters = %s", snapshot.Configuration.Parameters)
			}
			if snapshot.Identity.Logical.String() != "cluster/task" || snapshot.Identity.ObjectUID != "uid" {
				t.Fatalf("identity = %#v", snapshot.Identity)
			}
		})
	}
}

func TestDecodeTaskRejectsMissingWorkload(t *testing.T) {
	_, err := DecodeTask(&rlarkv1alpha1.Task{Spec: rlarkv1alpha1.TaskSpec{
		Runtime: rlarkv1alpha1.TaskRuntimeSpec{Kind: rlarkv1alpha1.RuntimeKindKubernetes},
	}})
	if !errors.Is(err, ErrMissingKubernetesWorkload) {
		t.Fatalf("DecodeTask() error = %v", err)
	}
}

func TestDecodeTaskCopiesMutableWorkloadInput(t *testing.T) {
	task := canonicalTask(taskapi.CarrierTypeKubernetesDeployment)
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	task.Spec.Workload.Carrier.Parameters.Raw = []byte(`{"replicas":2}`)
	snapshot, err := DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	task.Spec.Workload.Template.Spec.Containers[0].Image = "changed:v2"
	task.Spec.Workload.Parameters.Raw[0] = '['
	task.Spec.Workload.Carrier.Parameters.Raw[0] = '['
	if got := snapshot.Configuration.Template.Spec.Containers[0].Image; got != "example:v1" {
		t.Fatalf("snapshot template = %q", got)
	}
	if got := string(snapshot.Configuration.Parameters); got != `{"role":"head"}` {
		t.Fatalf("snapshot workload parameters = %q", got)
	}
	if got := string(snapshot.Configuration.Carrier.Parameters); got != `{"replicas":2}` {
		t.Fatalf("snapshot carrier parameters = %q", got)
	}
}

func TestDecodeTaskBuildsNetworkRequirementFromEnvironment(t *testing.T) {
	task := canonicalTask(taskapi.CarrierTypeKubernetesDeployment)
	task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{{
		Type:       taskapi.EnvironmentTypeNetwork,
		Version:    taskapi.EnvironmentVersionNetworkV1,
		Parameters: apiextensionsv1.JSON{Raw: []byte(`{"domain":"domain-a"}`)},
	}}

	snapshot, err := DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Environment.Environments) != 1 {
		t.Fatalf("network requirements = %#v", snapshot.Environment)
	}
	requirement := snapshot.Environment.Environments[0]
	if string(requirement.Parameters) != `{"domain":"domain-a"}` {
		t.Fatalf("network parameters = %s", requirement.Parameters)
	}
}

func TestDecodeTaskDefersNetworkValidationToResolver(t *testing.T) {
	task := canonicalTask(taskapi.CarrierTypeKubernetesDeployment)
	task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{{
		Type:       taskapi.EnvironmentTypeNetwork,
		Version:    taskapi.EnvironmentVersionNetworkV1,
		Parameters: apiextensionsv1.JSON{Raw: []byte(`{"domain":"","unknown":true}`)},
	}}
	snapshot, err := DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(snapshot.Environment.Environments[0].Parameters); got != `{"domain":"","unknown":true}` {
		t.Fatalf("network parameters = %s", got)
	}
}

func TestDecodeTaskBuildsSSHRequirementAndCopiesLocalDeclaration(t *testing.T) {
	task := canonicalTask(taskapi.CarrierTypeKubernetesDeployment)
	task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{{
		Type:       taskapi.EnvironmentTypeSSH,
		Version:    taskapi.EnvironmentVersionSSHV1,
		Parameters: apiextensionsv1.JSON{Raw: []byte(`{"authorizedKeys":["ssh-ed25519 public-key"]}`)},
	}}

	snapshot, err := DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Environment.Environments) != 1 {
		t.Fatalf("SSH requirements = %#v", snapshot.Environment)
	}
	requirement := snapshot.Environment.Environments[0]
	if requirement.Type != taskapi.EnvironmentTypeSSH || requirement.Version != taskapi.EnvironmentVersionSSHV1 || len(requirement.Parameters) != 0 || len(requirement.Consumers) != 1 {
		t.Fatalf("SSH requirement = %#v", requirement)
	}
	if len(snapshot.LocalEnvironments) != 1 || string(snapshot.LocalEnvironments[0].Parameters.Raw) != `{"authorizedKeys":["ssh-ed25519 public-key"]}` {
		t.Fatalf("local environments = %#v", snapshot.LocalEnvironments)
	}
	task.Spec.Environments[0].Parameters.Raw[0] = '['
	if string(snapshot.LocalEnvironments[0].Parameters.Raw) != `{"authorizedKeys":["ssh-ed25519 public-key"]}` {
		t.Fatal("local Environment declaration was not copied")
	}
}

func TestDecodeTaskPreservesDuplicateEnvironmentDeclarations(t *testing.T) {
	task := canonicalTask(taskapi.CarrierTypeKubernetesDeployment)
	environment := rlarkv1alpha1.TaskEnvironment{Type: taskapi.EnvironmentTypeSSH, Version: taskapi.EnvironmentVersionSSHV1, Parameters: apiextensionsv1.JSON{Raw: []byte(`{"authorizedKeys":[]}`)}}
	task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{environment, environment}
	snapshot, err := DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Environment.Environments) != 2 || len(snapshot.LocalEnvironments) != 2 {
		t.Fatalf("duplicate environments were not preserved: %#v", snapshot.Environment.Environments)
	}
}

func TestDecodeTaskPreservesOpenEnvironmentDeclarations(t *testing.T) {
	tests := []rlarkv1alpha1.TaskEnvironment{
		{
			Type:       taskapi.EnvironmentTypeTensorBoard,
			Version:    "v2",
			Parameters: apiextensionsv1.JSON{Raw: []byte(`{"logDir":"/logs"}`)},
		},
		{
			Type:       "example.io/unknown",
			Version:    "v1",
			Parameters: apiextensionsv1.JSON{Raw: []byte(`{}`)},
		},
	}
	for _, environment := range tests {
		task := canonicalTask(taskapi.CarrierTypeKubernetesDeployment)
		task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{environment}
		snapshot, err := DecodeTask(task)
		if err != nil {
			t.Fatalf("open environment was rejected: %v", err)
		}
		if got := snapshot.Environment.Environments[0]; got.Type != environment.Type || got.Version != environment.Version || string(got.Parameters) != string(environment.Parameters.Raw) {
			t.Fatalf("environment = %#v", got)
		}
	}
}

func TestDecodeTaskRejectsUnsupportedRuntimeAndWorkload(t *testing.T) {
	tests := map[string]rlarkv1alpha1.TaskSpec{
		"runtime": {
			Runtime:  rlarkv1alpha1.TaskRuntimeSpec{Kind: "Docker"},
			Workload: canonicalTask(taskapi.CarrierTypeKubernetesDeployment).Spec.Workload,
		},
		"workload type": {
			Runtime: rlarkv1alpha1.TaskRuntimeSpec{Kind: rlarkv1alpha1.RuntimeKindKubernetes},
			Workload: rlarkv1alpha1.TaskWorkloadSpec{
				Kind:    "example.io/unknown",
				Version: taskapi.WorkloadVersionRayV1,
			},
		},
		"workload version": {
			Runtime: rlarkv1alpha1.TaskRuntimeSpec{Kind: rlarkv1alpha1.RuntimeKindKubernetes},
			Workload: rlarkv1alpha1.TaskWorkloadSpec{
				Kind:    taskapi.WorkloadTypeRay,
				Version: "v2",
			},
		},
	}
	for name, spec := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeTask(&rlarkv1alpha1.Task{Spec: spec}); err == nil {
				t.Fatal("DecodeTask() error = nil")
			}
		})
	}
}

func canonicalTask(carrier string) *rlarkv1alpha1.Task {
	return &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "task",
			Namespace:  "cluster",
			UID:        "uid",
			Generation: 3,
		},
		Spec: rlarkv1alpha1.TaskSpec{
			Placement: rlarkv1alpha1.TaskPlacementSpec{},
			Runtime: rlarkv1alpha1.TaskRuntimeSpec{
				Kind: rlarkv1alpha1.RuntimeKindKubernetes,
				Name: "runtime-a",
			},
			Workload: rlarkv1alpha1.TaskWorkloadSpec{
				Kind:       taskapi.WorkloadTypeRay,
				Version:    taskapi.WorkloadVersionRayV1,
				Parameters: apiextensionsv1.JSON{Raw: []byte(`{"role":"head"}`)},
				Template:   &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "example:v1"}}}},
				Carrier: rlarkv1alpha1.TaskWorkloadCarrier{
					Type:    carrier,
					Version: taskapi.CarrierVersionKubernetesV1,
				},
			},
		},
	}
}

package gateway

import (
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
)

func TestInjectTensorBoardProxyUsesVersionedEnvironment(t *testing.T) {
	t.Parallel()

	task := &rlarkiov1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "task"},
		Spec: rlarkiov1alpha1.TaskSpec{Environments: []rlarkiov1alpha1.TaskEnvironment{{
			Type: taskapi.EnvironmentTypeTensorBoard, Version: taskapi.EnvironmentVersionTensorBoardV1,
			Parameters: apiextensionsv1.JSON{Raw: []byte(`{"logDir":"/logs"}`)},
		}}},
	}
	injectTensorBoardProxy(task)
	if task.Status.TensorBoardProxy != taskTensorBoardProxyPath(task.Name) {
		t.Fatalf("TensorBoard proxy = %q", task.Status.TensorBoardProxy)
	}
}

func TestInjectTensorBoardProxyRejectsInvalidEnvironment(t *testing.T) {
	t.Parallel()

	for _, environment := range []rlarkiov1alpha1.TaskEnvironment{
		{Type: taskapi.EnvironmentTypeTensorBoard, Version: taskapi.EnvironmentVersionTensorBoardV1, Parameters: apiextensionsv1.JSON{Raw: []byte(`{"logDir":""}`)}},
		{Type: taskapi.EnvironmentTypeTensorBoard, Version: "v2", Parameters: apiextensionsv1.JSON{Raw: []byte(`{"logDir":"/logs"}`)}},
		{Type: taskapi.EnvironmentTypeTensorBoard, Version: taskapi.EnvironmentVersionTensorBoardV1, Parameters: apiextensionsv1.JSON{Raw: []byte(`{"logDir":"/logs","unknown":true}`)}},
	} {
		task := &rlarkiov1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task"}, Spec: rlarkiov1alpha1.TaskSpec{Environments: []rlarkiov1alpha1.TaskEnvironment{environment}}}
		injectTensorBoardProxy(task)
		if task.Status.TensorBoardProxy != "" {
			t.Fatalf("invalid environment exposed proxy: %#v", environment)
		}
	}
}

func TestInjectTensorBoardProxyIntoMapUsesEnvironment(t *testing.T) {
	t.Parallel()

	item := map[string]any{
		"metadata": map[string]any{"name": "task"},
		"spec": map[string]any{"environments": []any{map[string]any{
			"type": taskapi.EnvironmentTypeTensorBoard, "version": taskapi.EnvironmentVersionTensorBoardV1,
			"parameters": map[string]any{"logDir": "/logs"},
		}}},
	}
	injectTensorBoardProxyIntoMap(item)
	status, _ := item["status"].(map[string]any)
	if status["tensorBoardProxy"] != taskTensorBoardProxyPath("task") {
		t.Fatalf("map TensorBoard status = %#v", status)
	}
}

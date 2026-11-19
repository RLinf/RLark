package restore

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func init() {
	Registry["jobs"] = &JobRestorable{}
}

// JobRestorable 实现 Job 的恢复接口。
type JobRestorable struct{}

// Kind 返回资源类型名。
func (r *JobRestorable) Kind() string {
	return "Job"
}

// BuildRestoredObject 从归档 raw 构造新的 Job CR。
func (r *JobRestorable) BuildRestoredObject(archived map[string]any, name string) (client.Object, error) {
	// 提取 spec
	specRaw, _ := json.Marshal(archived["spec"])
	var spec rlarkiov1alpha1.JobSpec
	if err := json.Unmarshal(specRaw, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal job spec: %w", err)
	}

	return &rlarkiov1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: extractAnnotations(archived, r.IsSystemAnnotation),
			Labels:      extractLabels(archived),
		},
		Spec: spec,
		// Status 不设置，让 Controller 重新填
	}, nil
}

// IsSystemAnnotation 判断是否为系统内部 annotation。
func (r *JobRestorable) IsSystemAnnotation(key string) bool {
	switch key {
	case "rlark.io/resource-id",
		"rlark.io/deletion-policy",
		"kubectl.kubernetes.io/last-applied-configuration":
		return true
	}
	return false
}

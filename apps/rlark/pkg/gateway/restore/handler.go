package restore

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	versioned "github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// CreateResource 根据资源类型调用对应的 KCP Create。
// 新增资源时在此添加 type switch 分支。
// 作为 Gateway 的方法，通过 Gateway.kubeClient 调用。
type Creator struct {
	kubeClient versioned.Interface
}

// NewCreator 创建一个 Creator。
func NewCreator(kubeClient versioned.Interface) *Creator {
	return &Creator{kubeClient: kubeClient}
}

// Create 根据资源类型调用对应的 KCP Create。
func (c *Creator) Create(ctx context.Context, resource string, obj client.Object) (client.Object, error) {
	switch resource {
	case "jobs":
		return c.kubeClient.RlinfV1alpha1().Jobs().Create(ctx, obj.(*rlarkiov1alpha1.Job), metav1.CreateOptions{})
	default:
		return nil, fmt.Errorf("resource %s create not implemented", resource)
	}
}

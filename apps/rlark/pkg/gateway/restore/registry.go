// Package restore 提供通用的资源恢复能力。
//
// 通过注册表（Registry）模式，新增资源恢复只需：
//  1. 实现 RestorableResource 接口（提供类型、恢复逻辑）
//  2. 在 init() 里注册到 Registry
package restore

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RestorableResource 定义可恢复资源的通用接口。
// 新增资源恢复只需实现此接口并注册到 restorableResources。
type RestorableResource interface {
	// Kind 返回资源类型名（用于日志和错误信息）。
	Kind() string

	// BuildRestoredObject 从归档 raw 构造新的 CR 对象。
	// 返回 client.Object 以便 Gateway 统一调用 Create。
	BuildRestoredObject(archived map[string]any, name string) (client.Object, error)

	// IsSystemAnnotation 判断是否为系统内部 annotation，恢复时剔除。
	IsSystemAnnotation(key string) bool
}

// Registry 注册各资源的恢复逻辑。
// 新增资源时只需在对应文件的 init() 里注册，无需修改此文件。
var Registry = map[string]RestorableResource{}

// extractAnnotations 从归档 raw 提取用户 annotation（剔除系统 annotation）。
func extractAnnotations(archived map[string]any, isSystem func(string) bool) map[string]string {
	annotations := map[string]string{}
	if meta, ok := archived["metadata"].(map[string]any); ok {
		if ann, ok := meta["annotations"].(map[string]any); ok {
			for k, v := range ann {
				if str, ok := v.(string); ok && !isSystem(k) {
					annotations[k] = str
				}
			}
		}
	}
	return annotations
}

// extractLabels 从归档 raw 提取 labels。
func extractLabels(archived map[string]any) map[string]string {
	labels := map[string]string{}
	if meta, ok := archived["metadata"].(map[string]any); ok {
		if l, ok := meta["labels"].(map[string]any); ok {
			for k, v := range l {
				if str, ok := v.(string); ok {
					labels[k] = str
				}
			}
		}
	}
	return labels
}

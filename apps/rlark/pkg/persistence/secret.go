package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

const (
	// SecretPrefix 是资源 Secret 名称前缀。
	SecretPrefix = "resource-"
	// NameLabel 标记资源名称。
	NameLabel = "rlark.io/persistence-name"
	// KindLabel 标记资源类型（job/task/workflow/node）。
	KindLabel = "rlark.io/persistence-kind"
)

// SecretBackend 使用 Kubernetes Secret 作为持久化存储（测试/CI 模式）。
// 支持 controller-runtime client 和 client-go 两种 client。
// Secret 存在 management namespace（和 SSH key / UI auth 等系统资源一致）。
type SecretBackend struct {
	client client.Client
	// clientGo 是 client-go client，Gateway 用（如果设置了，优先用）。
	clientGo kubernetes.Interface
	// namespace 是 Secret 所在 namespace（management namespace）。
	namespace string
}

// NewSecretBackend 创建一个 Secret Backend（controller-runtime client，Syncer 用）。
func NewSecretBackend(c client.Client, namespace string) *SecretBackend {
	return &SecretBackend{client: c, namespace: namespace}
}

// NewSecretBackendFromClientGo 创建一个 Secret Backend（client-go client，Gateway 用）。
func NewSecretBackendFromClientGo(client kubernetes.Interface, namespace string) *SecretBackend {
	return &SecretBackend{clientGo: client, namespace: namespace}
}

// Save 保存资源状态到 Secret（测试模式）。
// 支持所有资源（Job/Task/Workflow/Node），每种资源存一个 Secret。
func (b *SecretBackend) Save(ctx context.Context, m db.ResourceModel) error {
	base := m.GetBase()
	if base == nil {
		return apierrors.NewBadRequest("model has no base")
	}

	// 通过 model 类型判断资源类型
	kind := getResourceKind(m)
	if kind == "" {
		return nil // 未知类型不保存
	}

	name := base.Name
	namespace := base.Namespace
	sName := secretName(kind, namespace, name)
	raw := base.Raw

	secret := &corev1.Secret{}
	err := b.client.Get(ctx, client.ObjectKey{Name: sName, Namespace: b.namespace}, secret)
	if apierrors.IsNotFound(err) {
		// 创建新 Secret
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sName,
				Namespace: b.namespace,
				Labels: map[string]string{
					NameLabel: name,
					KindLabel: kind,
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"raw": raw,
			},
		}
		return b.client.Create(ctx, secret)
	}
	if err != nil {
		return err
	}

	// 更新现有 Secret
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data["raw"] = raw
	return b.client.Update(ctx, secret)
}

// Load 读取资源数据。
func (b *SecretBackend) Load(ctx context.Context, kind, namespace, name string) (*Resource, error) {
	sName := secretName(kind, namespace, name)

	if b.clientGo != nil {
		// client-go client（Gateway 用）
		secret, err := b.clientGo.CoreV1().Secrets(b.namespace).Get(ctx, sName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return b.extractFromSecret(secret)
	}

	// controller-runtime client（Syncer 用）
	secret := &corev1.Secret{}
	err := b.client.Get(ctx, client.ObjectKey{Name: sName, Namespace: b.namespace}, secret)
	if err != nil {
		return nil, err
	}
	return b.extractFromSecret(secret)
}

// extractFromSecret 从 Secret 提取资源数据（复用逻辑）。
func (b *SecretBackend) extractFromSecret(secret *corev1.Secret) (*Resource, error) {
	raw, ok := secret.Data["raw"]
	if !ok {
		return nil, apierrors.NewNotFound(corev1.Resource("secrets"), secret.Name)
	}

	return &Resource{
		Raw: raw,
	}, nil
}

// List 读取资源列表。
// 测试模式简化实现：只支持 namespace、label selector 和分页，不支持复杂排序和
// field selector。
//
// The label selectors are evaluated in memory because Secret storage has no
// query language of its own. They cannot be dropped: a caller restricted to its
// own tenants expresses that restriction as a label selector, so ignoring the
// selectors would widen every scoped list to the whole store.
func (b *SecretBackend) List(ctx context.Context, kind string, opts db.ListOptions) (*ListResult, error) {
	var secretList *corev1.SecretList
	var err error

	if b.clientGo != nil {
		secretList, err = b.clientGo.CoreV1().Secrets(b.namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", KindLabel, kind),
		})
	} else {
		secretList = &corev1.SecretList{}
		err = b.client.List(ctx, secretList,
			client.InNamespace(b.namespace),
			client.MatchingLabels{KindLabel: kind},
		)
	}
	if err != nil {
		return nil, err
	}

	var items []map[string]any
	for i := range secretList.Items {
		raw := secretList.Items[i].Data["raw"]
		if raw == nil {
			continue
		}

		var item map[string]any
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}

		if opts.Namespace != "" {
			if ns, _ := item["metadata"].(map[string]any)["namespace"].(string); ns != opts.Namespace {
				continue
			}
		}

		if !matchesLabelSelectors(item, opts.LabelSelector) {
			continue
		}

		items = append(items, item)
	}

	total := len(items)
	if opts.Offset > 0 && opts.Offset < len(items) {
		items = items[opts.Offset:]
	}
	if opts.Limit > 0 && opts.Limit < len(items) {
		items = items[:opts.Limit]
	}

	return &ListResult{Items: items, Total: total}, nil
}

// matchesLabelSelectors reports whether an object satisfies every selector.
//
// The operators mirror the database backed path: "in" takes a comma separated
// candidate list, "!=" also matches an object that carries no such label, and an
// absent or empty tenant label reads as the default tenant, so an object stored
// before the tenant label existed stays attributed to the default tenant instead
// of disappearing from every scoped list.
func matchesLabelSelectors(item map[string]any, selectors []db.LabelSelector) bool {
	for _, selector := range selectors {
		value, present := labelOf(item, selector.Key)
		if selector.Key == tenant.IDLabel && value == "" {
			value, present = tenant.DefaultID, true
		}
		switch selector.Op {
		case "!=":
			if present && value == selector.Value {
				return false
			}
		case "in":
			if !present || !slices.Contains(strings.Split(selector.Value, ","), value) {
				return false
			}
		default:
			if !present || value != selector.Value {
				return false
			}
		}
	}
	return true
}

// labelOf reads one label off a decoded object.
func labelOf(item map[string]any, key string) (string, bool) {
	metadata, ok := item["metadata"].(map[string]any)
	if !ok {
		return "", false
	}
	labels, ok := metadata["labels"].(map[string]any)
	if !ok {
		return "", false
	}
	value, ok := labels[key].(string)
	return value, ok
}

// Delete 删除资源记录（删除 Secret）。
func (b *SecretBackend) Delete(ctx context.Context, kind, namespace, name string) error {
	sName := secretName(kind, namespace, name)

	if b.clientGo != nil {
		// client-go client（Gateway 用）
		err := b.clientGo.CoreV1().Secrets(b.namespace).Delete(ctx, sName, metav1.DeleteOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// controller-runtime client（Syncer 用）
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sName,
			Namespace: b.namespace,
		},
	}
	err := b.client.Delete(ctx, secret)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// secretName 构造资源 Secret 名称。
// namespaced 资源（tasks/nodes）的 Secret 名称包含 namespace 避免冲突。
// cluster-scoped 资源（jobs/workflows）的 Secret 名称不包含 namespace。
func secretName(kind, namespace, name string) string {
	if namespace != "" {
		return SecretPrefix + kind + "-" + namespace + "-" + name
	}
	return SecretPrefix + kind + "-" + name
}

// Package archive 提供通用的资源归档能力。
//
// 通过注册表（Registry）模式，新增资源归档只需：
//  1. 实现 ArchivableResource 接口（提供类型、终态判断、清理逻辑）
//  2. 在 init() 里注册到 DefaultRegistry
//  3. 在 controller_manager.go 里把对应 controller 加入 reconcilers
package archive

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ArchivableResource 定义可归档资源的通用接口。
type ArchivableResource interface {
	// Kind 返回资源类型名（用于日志和 controller 命名）。
	Kind() string

	// NewObject 返回一个新的空对象（用于 client.Get）。
	NewObject() client.Object

	// IsTerminal 判断对象是否处于终态（需要归档）。
	IsTerminal(obj client.Object) bool

	// GetEndTime 获取对象的终态结束时间（用于计算归档 TTL）。
	GetEndTime(obj client.Object) *time.Time

	// SetupWithManager 注册 controller 到 manager。
	SetupWithManager(mgr ctrl.Manager, reconciler *Reconciler) error
}

// registry 是全局资源注册表。
type registry struct {
	resources []ArchivableResource
}

// DefaultRegistry 是默认的全局注册表。
var DefaultRegistry = &registry{}

// Register 注册一个可归档资源。
func (r *registry) Register(res ArchivableResource) {
	r.resources = append(r.resources, res)
}

// Resources 返回所有已注册的资源。
func (r *registry) Resources() []ArchivableResource {
	return r.resources
}

// Reconciler 是通用的归档 reconciler，通过 ArchivableResource 处理不同类型。
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
	ArchiveTTL              time.Duration
	resource                ArchivableResource
}

// NewReconciler 为指定资源创建一个归档 reconciler。
func NewReconciler(c client.Client, scheme *runtime.Scheme, res ArchivableResource, maxWorkers int, ttl time.Duration) *Reconciler {
	return &Reconciler{
		Client:                  c,
		Scheme:                  scheme,
		MaxConcurrentReconciles: maxWorkers,
		ArchiveTTL:              ttl,
		resource:                res,
	}
}

// Reconcile 处理单个资源。
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := r.resource.NewObject()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 已在删除中，无需处理
	if !obj.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// 非终态，不处理
	if !r.resource.IsTerminal(obj) {
		return ctrl.Result{}, nil
	}

	// 未到归档时间，延迟重试
	endTime := r.resource.GetEndTime(obj)
	if endTime != nil {
		ttl := r.ArchiveTTL
		if ttl <= 0 {
			ttl = DefaultArchiveTTL
		}
		remaining := ttl - time.Since(*endTime)
		if remaining > 0 {
			return ctrl.Result{RequeueAfter: remaining}, nil
		}
	}

	// DELETE KCP CR。持久化由 Syncer finalizer 保证。
	if err := r.Delete(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// terminalOrDeletingPredicate 只处理终态或正在删除的资源。
// CreateFunc 也判断 isTerminal，避免活跃资源触发不必要的 Reconcile。
// GenericFunc 返回 true 用于 informer resync 兜底。
func terminalOrDeletingPredicate(isTerminal func(client.Object) bool) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isTerminal(e.Object) },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			obj := e.ObjectNew
			return isTerminal(obj) || !obj.GetDeletionTimestamp().IsZero()
		},
	}
}

// SetupWithManager 注册 controller。
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.ArchiveTTL <= 0 {
		r.ArchiveTTL = DefaultArchiveTTL
	}
	return r.resource.SetupWithManager(mgr, r)
}

// DefaultArchiveTTL 是终态资源在 kcp 保留的时长，超过后触发归档删除。
const DefaultArchiveTTL = 5 * time.Minute

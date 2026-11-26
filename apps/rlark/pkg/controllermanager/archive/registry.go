// Package archive 提供通用的资源归档能力。
//
// 通过注册表（Registry）模式，新增资源归档只需：
//  1. 实现 ArchivableResource 接口（提供类型、终态判断、清理逻辑）
//  2. 在 init() 里注册到 DefaultRegistry
//  3. 在 controller_manager.go 里把对应 controller 加入 reconcilers
//
// Task is deliberately not registered here. A Task only exists to serve its
// Job: the Job reconciler recreates a Task that is missing while the Job is
// alive, so deleting a terminal Task on a TTL would produce a delete/recreate
// loop rather than an archive. A Task whose Job is gone is removed by the
// owner reference, and the orphan sweeper deletes the remainder whose owner
// can no longer be proven.
package archive

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// ArchiveObservedSinceAnnotation records when this controller first saw a
// terminal object that carries no end time. It is the archive TTL origin for
// exactly those objects, and it has to live on the object so a requeue or a
// process restart measures from the same instant rather than starting over.
const ArchiveObservedSinceAnnotation = "rlark.io/archive-observed-since"

// Reason codes this reconciler attaches to a classified pass. Closed set: they
// are used as a metric label.
const (
	reasonArchiveDeleting = "ArchiveDeleting"
	reasonArchiveTTLOpen  = "ArchiveTTLNotElapsed"
	reasonArchiveDeleted  = "ArchiveDeleted"
	reasonNotTerminal     = "NotTerminal"
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
	// Shell classifies the pass and records it. The zero value is silent, so a
	// test may leave it unset.
	Shell    controller.Shell
	resource ArchivableResource
}

// NewReconciler 为指定资源创建一个归档 reconciler。
func NewReconciler(c client.Client, scheme *runtime.Scheme, res ArchivableResource, maxWorkers int, ttl time.Duration, shell controller.Shell) *Reconciler {
	return &Reconciler{
		Client:                  c,
		Scheme:                  scheme,
		MaxConcurrentReconciles: maxWorkers,
		ArchiveTTL:              ttl,
		Shell:                   shell,
		resource:                res,
	}
}

// Reconcile 处理单个资源。
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Shell.Run(ctx, r.resource.Kind(), func(ctx context.Context) (controller.Result, error) {
		return r.reconcile(ctx, req)
	})
}

func (r *Reconciler) reconcile(ctx context.Context, req ctrl.Request) (controller.Result, error) {
	obj := r.resource.NewObject()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return controller.Result{
				Outcome: sharedreconcile.OutcomeDropped,
				Reason:  controller.ReasonObjectGone,
				Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
			}, nil
		}
		return controller.Result{Reason: controller.ReasonReconcileFailed}, err
	}

	// 已在删除中，无需处理
	if !obj.GetDeletionTimestamp().IsZero() {
		return controller.Result{
			Outcome: sharedreconcile.OutcomeDropped,
			Reason:  reasonArchiveDeleting,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
		}, nil
	}

	// 非终态，不处理
	if !r.resource.IsTerminal(obj) {
		return controller.Result{
			Outcome: sharedreconcile.OutcomeWaitingForChange,
			Reason:  reasonNotTerminal,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange},
		}, nil
	}

	// 未到归档时间，延迟重试。TTL 到期不产生任何事件，所以这一趟必须自驱；
	// 外壳给间隔加抖动，避免同批终态资源在同一刻集中删除。
	//
	// 结束时间缺失时不能立刻删除：归档删除不可逆，status 少写一个字段不应
	// 被放大成即时的数据丢失。但也不能每趟都按完整 TTL 重新排队——那样计时
	// 起点会被无限推后，对象永远不会被归档。这种情况改用第一次观察到终态的
	// 时间作为起点，并把它写回对象，让重排队和进程重启都从同一时刻计时。
	ttl := r.ArchiveTTL
	if ttl <= 0 {
		ttl = DefaultArchiveTTL
	}
	origin := r.resource.GetEndTime(obj)
	if origin == nil {
		observed, err := r.observeArchiveOrigin(ctx, obj)
		if err != nil {
			return controller.Result{Reason: controller.ReasonReconcileFailed}, err
		}
		origin = &observed
	}
	if remaining := ttl - time.Since(*origin); remaining > 0 {
		return controller.Result{
			Outcome: sharedreconcile.OutcomeWaitingForChange,
			Reason:  reasonArchiveTTLOpen,
			Retry: sharedreconcile.RetryDirective{
				Class: sharedreconcile.RetryOnChange,
				After: remaining,
			},
		}, nil
	}

	// DELETE KCP CR。持久化由 Syncer finalizer 保证。
	if err := r.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			// 另一趟（或另一个写者）已经删掉了，目标状态已达成。
			return controller.Result{
				Outcome: sharedreconcile.OutcomeSucceeded,
				Reason:  reasonArchiveDeleted,
				Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
			}, nil
		}
		return controller.Result{Reason: controller.ReasonReconcileFailed}, err
	}

	return controller.Result{
		Outcome: sharedreconcile.OutcomeSucceeded,
		Reason:  reasonArchiveDeleted,
		Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
	}, nil
}

// observeArchiveOrigin 返回缺少结束时间的对象的归档计时起点。
//
// 第一次看到这样的对象时，把当前时间写进注解；之后每一趟都读回同一个值。
// 计时起点必须留在对象上，否则重排队和进程重启都会把它重置，TTL 永远走不
// 完。写入失败直接返回错误：拿不到稳定的起点就不能继续往删除走。
func (r *Reconciler) observeArchiveOrigin(ctx context.Context, obj client.Object) (time.Time, error) {
	if recorded, ok := obj.GetAnnotations()[ArchiveObservedSinceAnnotation]; ok {
		if parsed, err := time.Parse(time.RFC3339, recorded); err == nil {
			return parsed, nil
		}
		// 注解被写坏了。这里不能沿用它，否则一个任意的时间戳就能决定对象
		// 什么时候被删除；用当前时间重写，计时从这一刻重新开始。
	}
	now := time.Now().UTC()
	before := obj.DeepCopyObject().(client.Object)
	obj.SetAnnotations(utils.MergeAnnotations(obj.GetAnnotations(),
		map[string]string{ArchiveObservedSinceAnnotation: now.Format(time.RFC3339)}))
	if err := r.Patch(ctx, obj, client.MergeFrom(before)); err != nil {
		return time.Time{}, fmt.Errorf("record archive origin for %s %s/%s: %w",
			r.resource.Kind(), obj.GetNamespace(), obj.GetName(), err)
	}
	return now, nil
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

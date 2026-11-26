package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	"github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// tasksObservedOnNode returns the sorted names of Tasks in the namespace whose
// observed state (status.observedNodes, reported by the data-plane Agent)
// currently places workload on the given node. Unmanaging a node with observed
// Tasks would orphan those workloads, so callers must reject unmanage while
// this list is non-empty.
func tasksObservedOnNode(ctx context.Context, client versioned.Interface, namespace, nodeName string) ([]string, error) {
	tasks, err := client.RlinfV1alpha1().Tasks(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, task := range tasks.Items {
		if slices.Contains(task.Status.ObservedNodes, nodeName) {
			names = append(names, task.Name)
		}
	}
	slices.Sort(names)
	return names, nil
}

// rejectNodeUnmanage writes a 409 response explaining that the node still
// hosts Tasks and must be drained before it can be unmanaged.
func rejectNodeUnmanage(c *gin.Context, namespace, nodeName string, tasks []string) {
	shown := tasks
	if len(shown) > 5 {
		shown = shown[:5]
	}
	message := fmt.Sprintf(
		"node %s/%s still hosts %d task(s) (%s); drain the node and wait until all tasks leave it before unmanaging",
		namespace, nodeName, len(tasks), strings.Join(shown, ", "))
	if len(tasks) > len(shown) {
		message += fmt.Sprintf(" and %d more", len(tasks)-len(shown))
	}
	respondError(c, http.StatusConflict, CodeResourceConflict, message)
}

// rejectNodeUnmanageNotCordoned writes a 409 response explaining that the node
// must be cordoned (spec.unschedulable=true) before it can be unmanaged, so
// that no new workloads are placed on it during the unmanage transition.
func rejectNodeUnmanageNotCordoned(c *gin.Context, namespace, nodeName string) {
	message := fmt.Sprintf(
		"node %s/%s is not cordoned; cordon the node first to stop new task scheduling before unmanaging",
		namespace, nodeName)
	respondError(c, http.StatusConflict, CodeResourceConflict, message)
}

// ensureNodeCordoned fetches the RLark Node CR and returns nil when the node
// cannot receive new Tasks, or a sentinel error that callers should convert
// into a 409 reject via rejectNodeUnmanageNotCordoned.
//
// A cordoned node (spec.unschedulable=true) is the expected case. A node whose
// Agent is not connected is accepted too: the controller reports it Offline
// instead of Cordoned and marks it unschedulable (placement fails closed when
// AgentConnected is not True), so no new Tasks can be placed on it and demanding
// an explicit cordon would leave an unreachable node permanently un-unmanageable.
func ensureNodeCordoned(ctx context.Context, client versioned.Interface, namespace, nodeName string) error {
	node, err := client.RlinfV1alpha1().Nodes(namespace).Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// 管理面 Node CR 不存在时无法检查 cordon 状态,
			// 这种情况极少见,让上层继续走 tasks 检查。
			return nil
		}
		return fmt.Errorf("get node %s/%s: %w", namespace, nodeName, err)
	}
	if node.Spec.Unschedulable {
		return nil
	}
	if connected := apimeta.FindStatusCondition(node.Status.Conditions, rlarkv1alpha1.ConditionTypeAgentConnected); connected == nil || connected.Status != metav1.ConditionTrue {
		return nil
	}
	return errNodeNotCordoned
}

var errNodeNotCordoned = errors.New("node is not cordoned")

// releaseNodeDrainRequests removes Task DrainRequests that target nodeName.
// Node Agents normally remove these entries when the Drain operation leaves the
// Node Spec, but a deleted Node is never reconciled again. This is the
// control-plane fallback for deleting a Node while its Agent is offline: without
// it, affected Tasks stay stopped forever with no observer left to release them.
func releaseNodeDrainRequests(ctx context.Context, client versioned.Interface, namespace, nodeName string) error {
	tasks, err := client.RlinfV1alpha1().Tasks(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range tasks.Items {
		task := &tasks.Items[i]
		kept := make([]rlarkv1alpha1.TaskDrainRequest, 0, len(task.Spec.DrainRequests))
		for _, request := range task.Spec.DrainRequests {
			if request.NodeName != nodeName {
				kept = append(kept, request)
			}
		}
		if len(kept) == len(task.Spec.DrainRequests) {
			continue
		}
		updated := task.DeepCopy()
		if len(kept) == 0 {
			updated.Spec.DrainRequests = nil
		} else {
			updated.Spec.DrainRequests = kept
		}
		if _, err := client.RlinfV1alpha1().Tasks(namespace).Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("release drain requests from task %s: %w", task.Name, err)
		}
	}
	return nil
}

// nodeDrainStopped reports whether a completed Node Drain stopped this Task.
// The Node Controller writes NodeDrainStoppedAnnotation and, when the drain
// itself suspended the Task, NodeDrainSuspendedAnnotation naming the drained
// node.
func nodeDrainStopped(task *rlarkv1alpha1.Task) bool {
	return task.Annotations[rlarkv1alpha1.NodeDrainStoppedAnnotation] != "" ||
		task.Annotations[rlarkv1alpha1.NodeDrainSuspendedAnnotation] != ""
}

// nodeDrainStoppedTasksForJob returns the Tasks owned by job that a node Drain
// stopped. They stay suspended until the Job is started explicitly.
func nodeDrainStoppedTasksForJob(ctx context.Context, client versioned.Interface, job *rlarkv1alpha1.Job) ([]rlarkv1alpha1.Task, error) {
	tasks, err := client.RlinfV1alpha1().Tasks(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{taskJobLabel: job.Name}).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("list Tasks for Job %q: %w", job.Name, err)
	}
	stopped := make([]rlarkv1alpha1.Task, 0)
	for i := range tasks.Items {
		task := &tasks.Items[i]
		if !metav1.IsControlledBy(task, job) || !nodeDrainStopped(task) {
			continue
		}
		stopped = append(stopped, *task)
	}
	return stopped, nil
}

// clearNodeDrainStop removes the node Drain stop markers from task and releases
// the suspension the drain itself imposed. A Task the user suspended keeps its
// own desired state.
func clearNodeDrainStop(ctx context.Context, client versioned.Interface, task *rlarkv1alpha1.Task) error {
	if !task.DeletionTimestamp.IsZero() || !nodeDrainStopped(task) {
		return nil
	}
	drainSuspended := task.Annotations[rlarkv1alpha1.NodeDrainSuspendedAnnotation] != ""
	annotations := map[string]any{rlarkv1alpha1.NodeDrainStoppedAnnotation: nil}
	if drainSuspended {
		annotations[rlarkv1alpha1.NodeDrainSuspendedAnnotation] = nil
	}
	patch := map[string]any{"metadata": map[string]any{"annotations": annotations}}
	if drainSuspended {
		patch["spec"] = map[string]any{"suspended": false}
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("build node drain marker patch for Task %s/%s: %w", task.Namespace, task.Name, err)
	}
	if _, err := client.RlinfV1alpha1().Tasks(task.Namespace).Patch(ctx, task.Name, types.MergePatchType, body, metav1.PatchOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("clear node drain markers from Task %s/%s: %w", task.Namespace, task.Name, err)
	}
	return nil
}

// clearNodeDrainStops removes the Node drain stop markers that this Node wrote
// onto Tasks when its Drain completed. It is only used when the Node object is
// deleted, where no Job start is available to release the markers. Cancelling a
// drain deliberately leaves them behind so the stopped Tasks stay suspended.
func clearNodeDrainStops(ctx context.Context, client versioned.Interface, namespace, nodeName string) error {
	tasks, err := client.RlinfV1alpha1().Tasks(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range tasks.Items {
		task := &tasks.Items[i]
		if task.Annotations[rlarkv1alpha1.NodeDrainStoppedAnnotation] != nodeName {
			continue
		}
		updated := task.DeepCopy()
		delete(updated.Annotations, rlarkv1alpha1.NodeDrainStoppedAnnotation)
		if updated.Annotations[rlarkv1alpha1.NodeDrainSuspendedAnnotation] == nodeName {
			delete(updated.Annotations, rlarkv1alpha1.NodeDrainSuspendedAnnotation)
			updated.Spec.Suspended = false
		}
		if _, err := client.RlinfV1alpha1().Tasks(namespace).Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("clear node drain markers from task %s: %w", task.Name, err)
		}
	}
	return nil
}

// jobHasNodeDrainStops reports whether any Task owned by job still carries a
// node Drain stop marker.
func (g *Gateway) jobHasNodeDrainStops(ctx context.Context, job *rlarkv1alpha1.Job) (bool, error) {
	if g.kubeClient == nil {
		return false, nil
	}
	tasks, err := nodeDrainStoppedTasksForJob(ctx, g.kubeClient, job)
	if err != nil {
		return false, err
	}
	return len(tasks) > 0, nil
}

// clearNodeDrainStopsForJob releases the Tasks owned by job that a node Drain
// stopped. Starting the Job is the explicit resume action for a drain, so the
// markers must be gone before the Job controller can dispatch Tasks again.
func (g *Gateway) clearNodeDrainStopsForJob(ctx context.Context, job *rlarkv1alpha1.Job) error {
	if g.kubeClient == nil {
		return nil
	}
	tasks, err := nodeDrainStoppedTasksForJob(ctx, g.kubeClient, job)
	if err != nil {
		return err
	}
	for i := range tasks {
		if err := clearNodeDrainStop(ctx, g.kubeClient, &tasks[i]); err != nil {
			return err
		}
	}
	return nil
}

// rlinfv1alpha1DeleteNode 取消纳管:删除管理面的 Node CR。
// 为避免正在运行的工作负载失去状态上报而被遗弃,当节点上仍观测到任务
// (Task.status.observedNodes 包含该节点名)时拒绝删除,返回 409;
// 运维需先排空节点并等待任务全部迁离后再取消纳管。
// 删除前会清理所有指向该节点的 Task DrainRequests，覆盖 Node Agent 离线、
// 无法自行释放请求的情况。
func (g *Gateway) rlinfv1alpha1DeleteNode(c *gin.Context) {
	if g.kubeClient == nil {
		respondError(c, http.StatusNotFound, CodeInvalidRequest, "kube client not configured")
		return
	}

	name := c.Param("name")
	namespace := c.Query("namespace")
	if name == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "name is required")
		return
	}
	if namespace == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "namespace query parameter is required for namespaced resource: nodes")
		return
	}
	if !g.authorizeTenantObjectByName(c, "nodes", namespace, name) {
		return
	}

	// 前置校验 1: 节点必须已 cordon,避免取消纳管过程中有新任务被调度上来。
	if err := ensureNodeCordoned(c.Request.Context(), g.kubeClient, namespace, name); err != nil {
		if errors.Is(err, errNodeNotCordoned) {
			rejectNodeUnmanageNotCordoned(c, namespace, name)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("check node cordon state: %v", err))
		return
	}

	// 前置校验 2: 节点上不能有仍被观测到的任务。
	tasks, err := tasksObservedOnNode(c.Request.Context(), g.kubeClient, namespace, name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("list tasks on node: %v", err))
		return
	}
	if len(tasks) > 0 {
		rejectNodeUnmanage(c, namespace, name, tasks)
		return
	}

	a, ok := g.accessors["nodes"]
	if !ok {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "unknown resource: nodes")
		return
	}

	ctx := c.Request.Context()
	// Release before deletion so a crash between the two steps cannot leave
	// stopped Tasks behind without a Node Agent to observe the missing Node.
	if err := releaseNodeDrainRequests(ctx, g.kubeClient, namespace, name); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("release node drain requests: %v", err))
		return
	}
	// A deleted Node never cancels its drain, so the Tasks it kept stopped must be
	// released explicitly or they could never be placed again.
	if err := clearNodeDrainStops(ctx, g.kubeClient, namespace, name); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("clear node drain markers: %v", err))
		return
	}

	// 删除资源时，先从持久化 Backend 删除（PG 或 Secret）。
	if g.persistenceBackend != nil {
		_ = g.persistenceBackend.Delete(ctx, "nodes", namespace, name)
	}
	if err := a.delete(ctx, namespace, name); err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}

	// A Node Agent may have recorded a request just before the Node disappeared.
	// Sweep again now that the Node can no longer be reconciled.
	if err := releaseNodeDrainRequests(ctx, g.kubeClient, namespace, name); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("release node drain requests: %v", err))
		return
	}

	c.Status(http.StatusOK)
}

const maxNodeOperations = 32

var errNodeOperationsLimit = errors.New("node operations limit reached")

// drainNode 请求一个 Drain 操作，并将 spec.unschedulable 置为 true。
// 若节点已有未完成的 Drain，则复用其 operation ID，避免重复请求不断叠加操作。
// 已完成的 Drain 保留在 Spec 中，后续独立 Drain 可以创建新操作，但操作总数有上限。
func (g *Gateway) drainNode(c *gin.Context) {
	if g.kubeClient == nil {
		respondError(c, http.StatusNotFound, CodeInvalidRequest, "kube client not configured")
		return
	}

	name := c.Param("name")
	namespace := c.Query("namespace")
	if name == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "name is required")
		return
	}
	if namespace == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "namespace query parameter is required for namespaced resource: nodes")
		return
	}
	if !g.authorizeTenantObjectByName(c, "nodes", namespace, name) {
		return
	}

	var updated *rlarkv1alpha1.Node
	var opID string
	operationCount := 0
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := g.kubeClient.RlinfV1alpha1().Nodes(namespace).Get(c.Request.Context(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		opID = ""
		completed := make(map[string]struct{}, len(node.Status.DrainOperations))
		for _, status := range node.Status.DrainOperations {
			if status.Completed {
				completed[status.OperationID] = struct{}{}
			}
		}
		for _, operation := range node.Spec.Operations {
			if operation.Type != rlarkv1alpha1.NodeOperationDrain || operation.ID == "" {
				continue
			}
			if _, done := completed[operation.ID]; !done {
				opID = operation.ID
				break
			}
		}

		before := node.DeepCopy()
		needsPatch := !node.Spec.Unschedulable
		if opID == "" {
			operationCount = len(node.Spec.Operations)
			if operationCount >= maxNodeOperations {
				return errNodeOperationsLimit
			}
			opID = "drain-" + uuid.NewString()
			node.Spec.Operations = append(node.Spec.Operations, rlarkv1alpha1.NodeOperationRequest{
				ID:   opID,
				Type: rlarkv1alpha1.NodeOperationDrain,
			})
			needsPatch = true
		}
		if !needsPatch {
			updated = node
			return nil
		}

		node.Spec.Unschedulable = true

		patch, err := mergeNodePatch(before, node)
		if err != nil {
			return err
		}
		updated, err = g.kubeClient.RlinfV1alpha1().Nodes(namespace).Patch(
			c.Request.Context(), name, types.MergePatchType, patch, metav1.PatchOptions{},
		)
		return err
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "node not found"})
			return
		}
		if errors.Is(err, errNodeOperationsLimit) {
			respondErrorWithDetails(c, http.StatusConflict, CodeResourceConflict,
				fmt.Sprintf("node spec.operations cannot exceed %d entries; remove completed operations with undrain or recover before requesting another drain", maxNodeOperations),
				map[string]any{"field": "spec.operations", "limit": maxNodeOperations, "actual": operationCount})
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"data":        updated,
		"operationId": opID,
	})
}

// undrainNode 移除 Node.spec.operations 中所有 Drain 类型的操作请求。
// 它不会自动将 spec.unschedulable 恢复为 false —— 运维需要显式执行 uncordon 来恢复调度。
// 这样做避免了在 drain 仍在进行（但 controller 还没完成清理）或者用户希望保留 cordon 状态
// 的场景下出现隐式的状态反转。
func (g *Gateway) undrainNode(c *gin.Context) {
	if g.kubeClient == nil {
		respondError(c, http.StatusNotFound, CodeInvalidRequest, "kube client not configured")
		return
	}

	name := c.Param("name")
	namespace := c.Query("namespace")
	if name == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "name is required")
		return
	}
	if namespace == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "namespace query parameter is required for namespaced resource: nodes")
		return
	}
	if !g.authorizeTenantObjectByName(c, "nodes", namespace, name) {
		return
	}

	var updated *rlarkv1alpha1.Node
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := g.kubeClient.RlinfV1alpha1().Nodes(namespace).Get(c.Request.Context(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// 移除所有 Drain 类型的 operation；其他 operation（EnterMaintenance）保持不变。
		filtered := make([]rlarkv1alpha1.NodeOperationRequest, 0, len(node.Spec.Operations))
		for _, op := range node.Spec.Operations {
			if op.Type != rlarkv1alpha1.NodeOperationDrain {
				filtered = append(filtered, op)
			}
		}

		// 如果根本没有 Drain operation，直接返回当前对象，避免无谓的 patch。
		if len(filtered) == len(node.Spec.Operations) {
			updated = node
			return nil
		}

		before := node.DeepCopy()
		node.Spec.Operations = filtered

		patch, err := mergeNodePatch(before, node)
		if err != nil {
			return err
		}
		updated, err = g.kubeClient.RlinfV1alpha1().Nodes(namespace).Patch(
			c.Request.Context(), name, types.MergePatchType, patch, metav1.PatchOptions{},
		)
		return err
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "node not found"})
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	// Cancelling the drain releases the drain requests it wrote. The stop markers
	// stay behind on purpose: only starting the Job (or unsuspending the Task)
	// resumes a Task the drain stopped.
	ctx := c.Request.Context()
	if err := releaseNodeDrainRequests(ctx, g.kubeClient, namespace, name); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("release node drain requests: %v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updated,
	})
}

// uncordonNode 将 Node.spec.unschedulable 置为 false，恢复新任务调度到该节点，
// 同时移除 spec.operations 中所有 Drain 类型的操作请求。
//
// uncordon 表达“恢复该节点调度”的意图，与仍在生效的 Drain 相互矛盾：只要 Drain
// 操作还留在 spec.operations，Task Host 就会持续停止该节点上被排空的 Task，使
// uncordon 变成静默无效。因此 uncordon 一并取消 Drain。
//
// 被排空的 Task 不会随 uncordon 自动恢复：Node Controller 写入的
// NodeDrainStoppedAnnotation / NodeDrainSuspendedAnnotation 会保留，Task 保持
// suspended，直到运维显式启动其 Job。EnterMaintenance 等其他操作保持不变，
// 需要一并清理时用 recoverNode。
//
// 若节点已经可调度且没有 Drain 操作，直接返回当前对象，避免无谓 patch。
func (g *Gateway) uncordonNode(c *gin.Context) {
	if g.kubeClient == nil {
		respondError(c, http.StatusNotFound, CodeInvalidRequest, "kube client not configured")
		return
	}

	name := c.Param("name")
	namespace := c.Query("namespace")
	if name == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "name is required")
		return
	}
	if namespace == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "namespace query parameter is required for namespaced resource: nodes")
		return
	}
	if !g.authorizeTenantObjectByName(c, "nodes", namespace, name) {
		return
	}

	var updated *rlarkv1alpha1.Node
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := g.kubeClient.RlinfV1alpha1().Nodes(namespace).Get(c.Request.Context(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		filtered := make([]rlarkv1alpha1.NodeOperationRequest, 0, len(node.Spec.Operations))
		for _, op := range node.Spec.Operations {
			if op.Type == rlarkv1alpha1.NodeOperationDrain {
				continue
			}
			filtered = append(filtered, op)
		}
		opsChanged := len(filtered) != len(node.Spec.Operations)

		if !node.Spec.Unschedulable && !opsChanged {
			updated = node
			return nil
		}

		before := node.DeepCopy()
		node.Spec.Operations = filtered
		node.Spec.Unschedulable = false

		patch, err := mergeNodePatch(before, node)
		if err != nil {
			return err
		}
		updated, err = g.kubeClient.RlinfV1alpha1().Nodes(namespace).Patch(
			c.Request.Context(), name, types.MergePatchType, patch, metav1.PatchOptions{},
		)
		return err
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "node not found"})
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	ctx := c.Request.Context()
	// The Node is schedulable again, so release the lingering DrainRequests it
	// wrote. The stop markers are kept: uncordon only restores scheduling, and the
	// drained Tasks stay suspended until their Job is started explicitly.
	if err := releaseNodeDrainRequests(ctx, g.kubeClient, namespace, name); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("release node drain requests: %v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updated,
	})
}

// recoverNode 是 drain/故障维修 之后的一键恢复:
//   - 移除 spec.operations 中所有 Drain 和 EnterMaintenance 类型的操作;
//   - 将 spec.unschedulable 置为 false。
//
// 与 uncordon 一致，recover 只恢复节点自身的调度能力，不会自动恢复被排空停止的
// Task；它们保持 suspended 直到运维显式启动其 Job。
//
// 若节点已经处于正常状态（没有 Drain/EnterMaintenance 且 unschedulable=false），
// 直接返回当前对象，避免无谓 patch。
func (g *Gateway) recoverNode(c *gin.Context) {
	if g.kubeClient == nil {
		respondError(c, http.StatusNotFound, CodeInvalidRequest, "kube client not configured")
		return
	}

	name := c.Param("name")
	namespace := c.Query("namespace")
	if name == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "name is required")
		return
	}
	if namespace == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "namespace query parameter is required for namespaced resource: nodes")
		return
	}
	if !g.authorizeTenantObjectByName(c, "nodes", namespace, name) {
		return
	}

	var updated *rlarkv1alpha1.Node
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := g.kubeClient.RlinfV1alpha1().Nodes(namespace).Get(c.Request.Context(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		filtered := make([]rlarkv1alpha1.NodeOperationRequest, 0, len(node.Spec.Operations))
		for _, op := range node.Spec.Operations {
			if op.Type == rlarkv1alpha1.NodeOperationDrain || op.Type == rlarkv1alpha1.NodeOperationEnterMaintenance {
				continue
			}
			filtered = append(filtered, op)
		}

		opsChanged := len(filtered) != len(node.Spec.Operations)
		unschedChanged := node.Spec.Unschedulable

		if !opsChanged && !unschedChanged {
			updated = node
			return nil
		}

		before := node.DeepCopy()
		node.Spec.Operations = filtered
		node.Spec.Unschedulable = false

		patch, err := mergeNodePatch(before, node)
		if err != nil {
			return err
		}
		updated, err = g.kubeClient.RlinfV1alpha1().Nodes(namespace).Patch(
			c.Request.Context(), name, types.MergePatchType, patch, metav1.PatchOptions{},
		)
		return err
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "node not found"})
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	ctx := c.Request.Context()
	// Release the lingering DrainRequests, but keep the drain stop markers: the
	// drained Tasks stay suspended until their Job is started explicitly.
	if err := releaseNodeDrainRequests(ctx, g.kubeClient, namespace, name); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("release node drain requests: %v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updated,
	})
}

// mergeNodePatch builds a JSON merge patch (RFC 7396) from before to after for Node resources.
//
// JSON merge patch replaces arrays wholesale, so spec.operations is not merged
// entry by entry. The patch therefore carries the resourceVersion read in
// before as an optimistic-concurrency precondition: the API server applies the
// patch to the current object and rejects the resulting update with 409 when
// that resourceVersion no longer matches. Callers wrap the call in
// retry.RetryOnConflict, which re-reads the Node and recomputes the change
// instead of dropping an operation written by a concurrent request.
func mergeNodePatch(before, after *rlarkv1alpha1.Node) ([]byte, error) {
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return nil, fmt.Errorf("marshal before Node: %w", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return nil, fmt.Errorf("marshal after Node: %w", err)
	}
	patch, err := jsonpatch.CreateMergePatch(beforeJSON, afterJSON)
	if err != nil {
		return nil, fmt.Errorf("create Node merge patch: %w", err)
	}
	if before.ResourceVersion == "" {
		return patch, nil
	}

	var document map[string]any
	if decodeErr := json.Unmarshal(patch, &document); decodeErr != nil {
		return nil, fmt.Errorf("decode Node merge patch: %w", decodeErr)
	}
	metadata, _ := document["metadata"].(map[string]any)
	if metadata == nil {
		metadata = make(map[string]any, 1)
	}
	metadata["resourceVersion"] = before.ResourceVersion
	document["metadata"] = metadata

	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode Node merge patch: %w", err)
	}
	return encoded, nil
}

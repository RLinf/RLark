package pod

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const staleSinceAnnotation = "rlark.io/stale-since"

// OrphanSweeper repairs missed local Pod delete events. It only considers
// management Pods explicitly owned by this agent and carrying a local UID.
type OrphanSweeper struct {
	c        *Controller
	interval time.Duration
	pageSize int64
	staleTTL time.Duration
	now      func() time.Time
}

func NewOrphanSweeper(c *Controller, interval time.Duration, pageSize int64, staleTTL ...time.Duration) *OrphanSweeper {
	ttl := 15 * time.Minute
	if len(staleTTL) > 0 {
		ttl = staleTTL[0]
	}
	return &OrphanSweeper{c: c, interval: interval, pageSize: pageSize, staleTTL: ttl, now: time.Now}
}

func (s *OrphanSweeper) Start(ctx context.Context) error {
	if err := s.Sweep(ctx); err != nil {
		log.FromContext(ctx).Error(err, "initial management Pod orphan sweep failed")
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.Sweep(ctx); err != nil {
				log.FromContext(ctx).Error(err, "management Pod orphan sweep failed")
			}
		}
	}
}

func (s *OrphanSweeper) NeedLeaderElection() bool { return true }

func (s *OrphanSweeper) Sweep(ctx context.Context) error {
	var errs []error
	if err := s.observeLegacyPods(ctx); err != nil {
		errs = append(errs, err)
	}
	pods, err := s.listManagedPods(ctx)
	if err != nil {
		errs = append(errs, err)
		return errors.Join(errs...)
	}
	for i := range pods {
		pod := &pods[i]
		uid := pod.Labels[rlarkv1alpha1.PodLabelLocalPodUID]
		if uid == "" || pod.Spec.PodName == "" || pod.Spec.PodNamespace == "" {
			continue
		}
		if orphan, err := s.taskIdentityGone(ctx, pod); err != nil {
			errs = append(errs, fmt.Errorf("inspect management Pod %s: %w", pod.Name, err))
			continue
		} else if orphan {
			if err := s.markOrDeleteStale(ctx, pod); err != nil {
				errs = append(errs, fmt.Errorf("mark or delete management Pod %s: %w", pod.Name, err))
			}
			continue
		}
		var local corev1.Pod
		err := s.c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: pod.Spec.PodName, Namespace: pod.Spec.PodNamespace}, &local)
		if err == nil && string(local.UID) == uid {
			action, err := s.observeLocalPod(ctx, &local)
			if err != nil {
				errs = append(errs, fmt.Errorf("observe local Pod %s/%s: %w", local.Namespace, local.Name, err))
				continue
			}
			if action != nil {
				if err := action(ctx); err != nil {
					errs = append(errs, fmt.Errorf("refresh management Pod %s: %w", pod.Name, err))
				}
			}
			continue
		}
		if err != nil && client.IgnoreNotFound(err) != nil {
			errs = append(errs, fmt.Errorf("get local Pod %s/%s: %w", pod.Spec.PodNamespace, pod.Spec.PodName, err))
			continue
		}
		if err := s.markOrDeleteStale(ctx, pod); err != nil {
			errs = append(errs, fmt.Errorf("mark or delete management Pod %s: %w", pod.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (s *OrphanSweeper) listManagedPods(ctx context.Context) ([]rlarkv1alpha1.Pod, error) {
	var result []rlarkv1alpha1.Pod
	continueToken := ""
	for {
		var pods rlarkv1alpha1.PodList
		opts := &client.ListOptions{Namespace: s.c.ManagementNamespace, Limit: s.pageSize, Continue: continueToken}
		client.MatchingLabels{rlarkv1alpha1.PodLabelAgentScope: s.c.ManagementNamespace}.ApplyToList(opts)
		if err := s.c.ManagementClient.List(ctx, &pods, opts); err != nil {
			return nil, err
		}
		result = append(result, pods.Items...)
		continueToken = pods.Continue
		if continueToken == "" {
			return result, nil
		}
	}
}

func (s *OrphanSweeper) markOrDeleteStale(ctx context.Context, pod *rlarkv1alpha1.Pod) error {
	if value := pod.Annotations[staleSinceAnnotation]; value != "" {
		staleSince, err := time.Parse(time.RFC3339Nano, value)
		if err == nil && s.now().Sub(staleSince) >= s.staleTTL {
			current, stale, err := s.currentlyStale(ctx, pod)
			if err != nil || !stale {
				return err
			}
			if err := s.c.ManagementClient.Delete(ctx, current); err != nil && client.IgnoreNotFound(err) != nil {
				return err
			}
			return nil
		}
		if err == nil {
			return nil
		}
	}
	original := pod.DeepCopy()
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[staleSinceAnnotation] = s.now().UTC().Format(time.RFC3339Nano)
	if err := s.c.ManagementClient.Patch(ctx, pod, client.MergeFrom(original)); err != nil {
		return err
	}
	var current rlarkv1alpha1.Pod
	if err := s.c.ManagementClient.Get(ctx, client.ObjectKeyFromObject(pod), &current); err != nil {
		return err
	}
	statusOriginal := current.DeepCopy()
	current.Status.Phase = rlarkv1alpha1.PodPhaseUnknown
	current.Status.Message = "Stale: local Pod is missing or its identity changed"
	if err := s.c.ManagementClient.Status().Patch(ctx, &current, client.MergeFrom(statusOriginal)); err != nil &&
		client.IgnoreNotFound(err) != nil {
		return err
	}
	return nil
}

func (s *OrphanSweeper) currentlyStale(ctx context.Context, observed *rlarkv1alpha1.Pod) (*rlarkv1alpha1.Pod, bool, error) {
	var current rlarkv1alpha1.Pod
	if err := s.c.ManagementClient.Get(ctx, client.ObjectKeyFromObject(observed), &current); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, false, nil
		}
		return nil, false, err
	}
	if current.UID != observed.UID || current.Annotations[staleSinceAnnotation] != observed.Annotations[staleSinceAnnotation] {
		return &current, false, nil
	}
	uid := current.Labels[rlarkv1alpha1.PodLabelLocalPodUID]
	if uid == "" || current.Spec.PodName == "" || current.Spec.PodNamespace == "" {
		return &current, false, nil
	}
	orphan, err := s.taskIdentityGone(ctx, &current)
	if err != nil || orphan {
		return &current, orphan, err
	}
	var local corev1.Pod
	err = s.c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: current.Spec.PodName, Namespace: current.Spec.PodNamespace}, &local)
	if err == nil {
		return &current, string(local.UID) != uid, nil
	}
	if client.IgnoreNotFound(err) == nil {
		return &current, true, nil
	}
	return &current, false, err
}

func (s *OrphanSweeper) observeLocalPod(ctx context.Context, local *corev1.Pod) (func(context.Context) error, error) {
	annotations := local.Annotations
	taskName := annotations["rlark.io/management-task-name"]
	taskNamespace := annotations["rlark.io/management-task-namespace"]
	taskUID := annotations["rlark.io/management-task-uid"]
	if taskName == "" || taskNamespace == "" {
		return nil, nil
	}
	if taskUID == "" {
		resolvedUID, err := s.c.resolveLegacyTaskUID(ctx, local, taskName, taskNamespace)
		if err != nil {
			return nil, err
		}
		if resolvedUID == "" {
			return nil, nil
		}
		taskUID = resolvedUID
	}
	reconciler := &pushPodReconciler{c: s.c}
	desired, err := reconciler.buildRLarkPodFromK8sPod(ctx, local, taskName, taskNamespace, taskUID)
	if err != nil {
		return nil, err
	}
	desired.Annotations = map[string]string{staleSinceAnnotation: ""}
	return func(ctx context.Context) error {
		_, err := reconciler.updateManagementPod(ctx, log.FromContext(ctx), desired)
		return err
	}, nil
}

func (s *OrphanSweeper) observeLegacyPods(ctx context.Context) error {
	var errs []error
	continueToken := ""
	for {
		var pods rlarkv1alpha1.PodList
		if err := s.c.ManagementClient.List(ctx, &pods, &client.ListOptions{Namespace: s.c.ManagementNamespace, Limit: s.pageSize, Continue: continueToken}); err != nil {
			errs = append(errs, err)
			return errors.Join(errs...)
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			backfill, err := s.observeLegacyIdentity(ctx, pod)
			if err != nil {
				errs = append(errs, fmt.Errorf("inspect legacy management Pod %s: %w", pod.Name, err))
				continue
			}
			if backfill != nil {
				if err := backfill(ctx); err != nil {
					errs = append(errs, fmt.Errorf("backfill legacy management Pod %s: %w", pod.Name, err))
				}
			}
		}
		continueToken = pods.Continue
		if continueToken == "" {
			return errors.Join(errs...)
		}
	}
}

func (s *OrphanSweeper) taskIdentityGone(ctx context.Context, pod *rlarkv1alpha1.Pod) (bool, error) {
	taskName := pod.Labels[rlarkv1alpha1.PodLabelTaskName]
	taskUID := pod.Labels[rlarkv1alpha1.PodLabelTaskUID]
	if taskName == "" || taskUID == "" || pod.Spec.TaskNamespace == "" {
		return false, nil
	}
	var task rlarkv1alpha1.Task
	err := s.c.ManagementClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: pod.Spec.TaskNamespace}, &task)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return true, nil
		}
		return false, fmt.Errorf("verify Task for management Pod %s: %w", pod.Name, err)
	}
	if string(task.UID) != taskUID {
		return true, nil
	}
	domain := pod.Labels[rlarkv1alpha1.PodLabelDomain]
	return domain != "" && task.Spec.Domain != domain, nil
}

// backfillLegacyIdentity adopts only the old UID-named form when the local Pod
// and current management Task prove a unique identity. Ambiguous legacy
// objects are deliberately left untouched for operator review.
func (s *OrphanSweeper) observeLegacyIdentity(ctx context.Context, pod *rlarkv1alpha1.Pod) (func(context.Context) error, error) {
	if pod.Labels[rlarkv1alpha1.PodLabelAgentScope] != "" || pod.Labels[rlarkv1alpha1.PodLabelLocalPodUID] != "" ||
		pod.Spec.PodName == "" || pod.Spec.PodNamespace == "" || pod.Spec.TaskName == "" || pod.Spec.TaskNamespace != s.c.ManagementNamespace {
		return nil, nil
	}
	var local corev1.Pod
	if err := s.c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: pod.Spec.PodName, Namespace: pod.Spec.PodNamespace}, &local); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, nil
		}
		return nil, err
	}
	if pod.Name != string(local.UID) {
		return nil, nil
	}
	annotations := local.Annotations
	if annotations["rlark.io/management-task-name"] != pod.Spec.TaskName || annotations["rlark.io/management-task-namespace"] != pod.Spec.TaskNamespace || annotations["rlark.io/management-task-uid"] == "" {
		return nil, nil
	}
	var task rlarkv1alpha1.Task
	if err := s.c.ManagementClient.Get(ctx, types.NamespacedName{Name: pod.Spec.TaskName, Namespace: pod.Spec.TaskNamespace}, &task); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, nil
		}
		return nil, err
	}
	if string(task.UID) != annotations["rlark.io/management-task-uid"] || (pod.Spec.Domain != "" && task.Spec.Domain != pod.Spec.Domain) {
		return nil, nil
	}
	pod = pod.DeepCopy()
	return func(ctx context.Context) error {
		original := pod.DeepCopy()
		if pod.Labels == nil {
			pod.Labels = map[string]string{}
		}
		pod.Labels[rlarkv1alpha1.PodLabelAgentScope] = s.c.ManagementNamespace
		pod.Labels[rlarkv1alpha1.PodLabelLocalPodUID] = string(local.UID)
		pod.Labels[rlarkv1alpha1.PodLabelLocalPodName] = local.Name
		pod.Labels[rlarkv1alpha1.PodLabelLocalPodNamespace] = local.Namespace
		pod.Labels[rlarkv1alpha1.PodLabelTaskName] = task.Name
		pod.Labels[rlarkv1alpha1.PodLabelTaskUID] = string(task.UID)
		if pod.Spec.Domain != "" {
			pod.Labels[rlarkv1alpha1.PodLabelDomain] = pod.Spec.Domain
		}
		return s.c.ManagementClient.Patch(ctx, pod, client.MergeFrom(original))
	}, nil
}

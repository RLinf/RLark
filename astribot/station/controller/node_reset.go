package controller

import (
	"context"
	"fmt"
	"slices"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// nodeResetNamespace is where the RLark node agent runs: its image and the
// accounts it pulls with are the reset pod's.
const nodeResetNamespace = "rlark-system"

// nodeResetScript runs on the host as a service of its own, so the reset
// outlives the pod that started it: kubeadm reset stops the kubelet and
// removes every container. It waits until the kubelet has reported the pod
// done, and resets with the kubeadm and the container runtime the node was
// joined with (an installer's in /opt/rlark/bin, or the machine's own).
const nodeResetScript = `sleep 10
PATH=/opt/rlark/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
[ ! -e /etc/kubernetes/manifests/kube-apiserver.yaml ] || exit 1
socket=$(sed -n 's|.*--container-runtime-endpoint=\(unix://[^ "]*\).*|\1|p' /var/lib/kubelet/kubeadm-flags.env 2>/dev/null)
kubeadm reset --force ${socket:+--cri-socket "$socket"}
rm -rf /etc/cni/net.d/*
`

// InClusterNodeReset resets a removed node from inside its cluster, with no
// login to its machine: a pod Kubernetes places on that very Node starts
// kubeadm reset on the host. A node whose kubelet no longer reports is not
// reset now; its machine clears what it kept when it joins again (the
// installer, and the join of a NodeClass machine).
type InClusterNodeReset struct{ Members NodeMemberClientSource }

func nodeResetPodName(op *stationv1alpha1.NodeOperation) string {
	return "rlark-node-reset-" + string(op.UID)
}

func nodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// Reset is done once the kubelet has stopped reporting: after the reset pod
// has run, or at once on a node whose kubelet was gone before.
func (r InClusterNodeReset) Reset(ctx context.Context, op *stationv1alpha1.NodeOperation) (bool, error) {
	member, err := r.Members.Client(ctx, op.Spec.ClusterID)
	if err != nil {
		return false, fmt.Errorf("connect member cluster: %w", err)
	}
	node, err := member.CoreV1().Nodes().Get(ctx, op.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("read node before reset: %w", err)
	}
	if string(node.UID) != op.Spec.NodeUID || node.DeletionTimestamp != nil {
		return false, ErrNodeIdentityChanged
	}
	pods := member.CoreV1().Pods(nodeResetNamespace)
	pod, err := pods.Get(ctx, nodeResetPodName(op), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if !nodeReady(node) {
			return true, nil
		}
		want, err := nodeResetPod(ctx, member, op)
		if err != nil {
			return false, err
		}
		if _, err := pods.Create(ctx, want, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return false, fmt.Errorf("start the reset on the node: %w", err)
		}
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("read the reset pod: %w", err)
	}
	if pod.Status.Phase == corev1.PodFailed {
		return false, fmt.Errorf("the reset could not be started on the node: %s %s", pod.Status.Reason, pod.Status.Message)
	}
	return pod.Status.Phase == corev1.PodSucceeded && !nodeReady(node), nil
}

// nodeResetPod is a pod on the node, in the host's namespaces, with the
// image and pull accounts of the RLark node agent: the image every node has.
func nodeResetPod(ctx context.Context, member kubernetes.Interface, op *stationv1alpha1.NodeOperation) (*corev1.Pod, error) {
	sets, err := member.AppsV1().DaemonSets(nodeResetNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("find the RLark node agent: %w", err)
	}
	var agent *corev1.PodSpec
	for i := range sets.Items {
		spec := &sets.Items[i].Spec.Template.Spec
		if len(spec.Containers) > 0 && slices.Contains(spec.Containers[0].Args, "--mode=node") {
			agent = spec
		}
	}
	if agent == nil {
		return nil, fmt.Errorf("%w: cluster %s runs no RLark node agent in %s", ErrNodeStageUnavailable, op.Spec.ClusterID, nodeResetNamespace)
	}
	privileged, never := true, false
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: nodeResetPodName(op), Namespace: nodeResetNamespace,
			Labels: map[string]string{"app.kubernetes.io/name": "rlark-node-reset", "station.rlark.io/operation-uid": string(op.UID)}},
		Spec: corev1.PodSpec{
			NodeName: op.Spec.NodeName, RestartPolicy: corev1.RestartPolicyNever, HostPID: true, HostNetwork: true,
			AutomountServiceAccountToken: &never, ImagePullSecrets: agent.ImagePullSecrets,
			Tolerations: []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
			Containers: []corev1.Container{{
				Name: "reset", Image: agent.Containers[0].Image,
				Command: []string{"nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--",
					"systemd-run", "--unit", "rlark-node-reset-" + string(op.UID)[:8], "--collect", "--quiet", "/bin/sh", "-c", nodeResetScript},
				SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
			}},
		},
	}, nil
}

type NodeResetDriver interface {
	Reset(context.Context, *stationv1alpha1.NodeOperation) (bool, error)
}

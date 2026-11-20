package stationidentity

import (
	"context"
	"errors"
	"fmt"
	"slices"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Evidence is what a verified Pod-bound token says of its caller: the Node its Pod runs on.
type Evidence struct {
	ClusterID, NodeName, NodeUID string
	// SystemUUID is the Node's (status.nodeInfo.systemUUID, the kubelet's
	// reading of the machine's firmware): it tells a reinstalled machine from
	// another one, and is evidence only.
	SystemUUID string
	// NodePressures are the pressure conditions that are True on that Node.
	NodePressures []string
}

// Registration is supplied by a trusted server-side registry, never by the
// caller. The client must target that registered cluster with verified TLS.
type Registration struct {
	ClusterID, Namespace, ServiceAccount, ServiceAccountUID, Audience string
}

type registeredReader struct {
	registration Registration
	client       kubernetes.Interface
}

func (r registeredReader) Authenticate(ctx context.Context, token string) (Evidence, error) {
	return Authenticate(ctx, r.client, r.registration, token)
}

// CurrentNodeUID rechecks the member Node on each query. A historical
// heartbeat is not proof that a Node still exists.
func (r registeredReader) CurrentNodeUID(ctx context.Context, nodeName string) (string, error) {
	node, err := r.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("read current member Node: %w", err)
	}
	if node.DeletionTimestamp != nil || node.UID == "" {
		return "", ErrIdentityDenied
	}
	return string(node.UID), nil
}

// Authenticate resolves a Pod-bound token with a registered cluster API to
// the Node the Pod runs on.
func Authenticate(ctx context.Context, client kubernetes.Interface, reg Registration, token string) (Evidence, error) {
	review, err := client.AuthenticationV1().TokenReviews().Create(ctx, &authv1.TokenReview{Spec: authv1.TokenReviewSpec{Token: token, Audiences: []string{reg.Audience}}}, metav1.CreateOptions{})
	if err != nil {
		return Evidence{}, fmt.Errorf("%w: TokenReview request failed: %w", ErrIdentityUnavailable, err)
	}
	if review.Status.Error != "" || !review.Status.Authenticated || review.Status.User.Username != "system:serviceaccount:"+reg.Namespace+":"+reg.ServiceAccount || string(review.Status.User.UID) != reg.ServiceAccountUID {
		return Evidence{}, errors.New("token review rejected identity")
	}
	names, uids := review.Status.User.Extra["authentication.kubernetes.io/pod-name"], review.Status.User.Extra["authentication.kubernetes.io/pod-uid"]
	if len(names) != 1 || len(uids) != 1 {
		return Evidence{}, errors.New("token missing bound pod identity")
	}
	pod, err := client.CoreV1().Pods(reg.Namespace).Get(ctx, names[0], metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return Evidence{}, fmt.Errorf("%w: read bound Pod: %w", ErrIdentityUnavailable, err)
		}
		return Evidence{}, fmt.Errorf("read bound pod: %w", err)
	}
	if pod.Spec.NodeName == "" {
		return Evidence{}, errors.New("pod has no assigned node")
	}
	sa, err := client.CoreV1().ServiceAccounts(reg.Namespace).Get(ctx, reg.ServiceAccount, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return Evidence{}, fmt.Errorf("%w: read registered ServiceAccount: %w", ErrIdentityUnavailable, err)
		}
		return Evidence{}, fmt.Errorf("read service account: %w", err)
	}
	node, err := client.CoreV1().Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return Evidence{}, fmt.Errorf("%w: read bound Node: %w", ErrIdentityUnavailable, err)
		}
		return Evidence{}, fmt.Errorf("read node: %w", err)
	}
	if string(sa.UID) != reg.ServiceAccountUID || pod.Spec.ServiceAccountName != reg.ServiceAccount || string(pod.UID) != uids[0] || node.UID == "" ||
		!slices.Contains(review.Status.Audiences, reg.Audience) ||
		pod.DeletionTimestamp != nil || sa.DeletionTimestamp != nil || node.DeletionTimestamp != nil {
		return Evidence{}, errors.New("the token is not of a current Pod of the registered ServiceAccount on a current Node")
	}
	// The Node is in hand for its UID; its system UUID and pressure are stored
	// with the report it vouches for, so no agent is given a way to read or to state them.
	e := Evidence{ClusterID: reg.ClusterID, NodeName: node.Name, NodeUID: string(node.UID), SystemUUID: node.Status.NodeInfo.SystemUUID, NodePressures: []string{}}
	for _, condition := range node.Status.Conditions {
		if condition.Status == corev1.ConditionTrue && slices.Contains([]corev1.NodeConditionType{corev1.NodeDiskPressure, corev1.NodeMemoryPressure, corev1.NodePIDPressure}, condition.Type) {
			e.NodePressures = append(e.NodePressures, string(condition.Type))
		}
	}
	return e, nil
}

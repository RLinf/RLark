package stationidentity

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Registration is supplied by a trusted server-side registry, never by the
// caller. The client must target that registered cluster with verified TLS.
type Registration struct {
	ClusterID, Namespace, ServiceAccount, ServiceAccountUID, Audience string
}

type RegisteredCluster struct {
	Registration Registration
	Client       kubernetes.Interface
}

type registeredReader struct{ cluster RegisteredCluster }

func (r registeredReader) Authenticate(ctx context.Context, token string) (Evidence, error) {
	return Authenticate(ctx, r.cluster.Client, r.cluster.Registration, token)
}

// CurrentNodeUID rechecks the member Node on each query. A historical
// heartbeat is not proof that a Node still exists.
func (r registeredReader) CurrentNodeUID(ctx context.Context, nodeName string) (string, error) {
	node, err := r.cluster.Client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
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
	name, okName := singleExtra(review.Status.User.Extra, "authentication.kubernetes.io/pod-name")
	uid, okUID := singleExtra(review.Status.User.Extra, "authentication.kubernetes.io/pod-uid")
	if !okName || !okUID {
		return Evidence{}, errors.New("token missing bound pod identity")
	}
	pod, err := client.CoreV1().Pods(reg.Namespace).Get(ctx, name, metav1.GetOptions{})
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
	e := Evidence{ClusterID: reg.ClusterID, RegisteredClusterID: reg.ClusterID, ServiceAccountNamespace: reg.Namespace, RegisteredNamespace: reg.Namespace, ServiceAccountName: reg.ServiceAccount, RegisteredServiceAccount: reg.ServiceAccount, TokenServiceAccountUID: string(review.Status.User.UID), RegisteredServiceAccountUID: string(sa.UID), PodServiceAccountName: pod.Spec.ServiceAccountName, TokenPodName: name, PodName: pod.Name, TokenPodUID: uid, PodUID: string(pod.UID), PodNodeName: pod.Spec.NodeName, NodeName: node.Name, NodeUID: string(node.UID), TokenAuthenticated: review.Status.Authenticated, TokenAudiences: review.Status.Audiences, RequiredAudiences: []string{reg.Audience}, PodDeleted: pod.DeletionTimestamp != nil || sa.DeletionTimestamp != nil, NodeDeleted: node.DeletionTimestamp != nil}
	// The Node is in hand for its UID; its pressure is stored with the report
	// it vouches for, so no agent is given a way to read or to state it.
	e.NodePressures = []string{}
	for _, condition := range node.Status.Conditions {
		if condition.Status == corev1.ConditionTrue && slices.Contains([]corev1.NodeConditionType{corev1.NodeDiskPressure, corev1.NodeMemoryPressure, corev1.NodePIDPressure}, condition.Type) {
			e.NodePressures = append(e.NodePressures, string(condition.Type))
		}
	}
	if string(sa.UID) != reg.ServiceAccountUID {
		return Evidence{}, errors.New("service account registration changed")
	}
	if err := Validate(e); err != nil {
		return Evidence{}, err
	}
	return e, nil
}

func singleExtra(extras map[string]authv1.ExtraValue, key string) (string, bool) {
	values := extras[key]
	returnValue := ""
	if len(values) == 1 {
		returnValue = values[0]
	}
	return returnValue, len(values) == 1 && returnValue != "" && strings.TrimSpace(returnValue) == returnValue
}

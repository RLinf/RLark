// Package identity gives every component that talks to the management plane (kcp) an identity of its
// own: a ServiceAccount in kcp's root workspace, the permissions it needs and a token kcp signs with
// its service-account key. kcp writes its administrator's credential anew at every start; these
// tokens outlive that, because the key they are signed with is kept in kcp's data.
//
// A component is given a kubeconfig that reads its token from the file next to it, so a rotated token
// is taken up within a minute (or at the first 401) without a restart.
package identity

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// LabelIdentity marks the token Secrets of an identity with the component's name.
const LabelIdentity = "rlark.io/identity"

// KubeconfigKey and TokenKey are the names of a component's kubeconfig and token, as files in its
// directory or keys of its Secret.
const (
	KubeconfigKey = "kubeconfig"
	TokenKey      = "token"
)

// Component is one client of the management plane.
type Component struct {
	// Name names the identity: its directory, its Secret and the label of its tokens.
	Name           string
	Namespace      string
	ServiceAccount string
	// Rules, when set, are granted through a ClusterRole of the component's name.
	Rules []rbacv1.PolicyRule
	// ClusterRole, when set, is an existing ClusterRole bound to the component.
	ClusterRole string
}

// Credential is what a component is given: its kubeconfig (which reads the token from the file
// TokenKey next to it) and its token.
type Credential struct {
	Kubeconfig []byte
	Token      []byte
	// Secret is the token Secret in kcp the token comes from.
	Secret string
}

// Ensure makes the component's ServiceAccount and permissions what they should be.
func Ensure(ctx context.Context, kcp kubernetes.Interface, c Component) error {
	if err := create(ctx, func() error {
		_, err := kcp.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: c.Namespace}}, metav1.CreateOptions{})
		return err
	}); err != nil {
		return fmt.Errorf("namespace %s: %w", c.Namespace, err)
	}
	if err := create(ctx, func() error {
		_, err := kcp.CoreV1().ServiceAccounts(c.Namespace).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: c.ServiceAccount, Namespace: c.Namespace}}, metav1.CreateOptions{})
		return err
	}); err != nil {
		return fmt.Errorf("service account %s/%s: %w", c.Namespace, c.ServiceAccount, err)
	}
	role := c.ClusterRole
	if c.Rules != nil {
		role = c.Name
		desired := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: role}, Rules: c.Rules}
		current, err := kcp.RbacV1().ClusterRoles().Get(ctx, role, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			_, err = kcp.RbacV1().ClusterRoles().Create(ctx, desired, metav1.CreateOptions{})
		case err == nil:
			current.Rules = c.Rules
			_, err = kcp.RbacV1().ClusterRoles().Update(ctx, current, metav1.UpdateOptions{})
		}
		if err != nil {
			return fmt.Errorf("cluster role %s: %w", role, err)
		}
	}
	if role == "" {
		return nil
	}
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: c.Name},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: c.ServiceAccount, Namespace: c.Namespace}},
	}
	current, err := kcp.RbacV1().ClusterRoleBindings().Get(ctx, c.Name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		_, err = kcp.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
	case err == nil && current.RoleRef != binding.RoleRef:
		// A binding's role cannot be changed in place.
		if err = kcp.RbacV1().ClusterRoleBindings().Delete(ctx, c.Name, metav1.DeleteOptions{}); err == nil {
			_, err = kcp.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
		}
	case err == nil:
		current.Subjects = binding.Subjects
		_, err = kcp.RbacV1().ClusterRoleBindings().Update(ctx, current, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("cluster role binding %s: %w", c.Name, err)
	}
	return nil
}

func create(ctx context.Context, fn func() error) error {
	if err := fn(); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// Tokens lists the component's token Secrets, the newest last.
func Tokens(ctx context.Context, kcp kubernetes.Interface, c Component) ([]corev1.Secret, error) {
	list, err := kcp.CoreV1().Secrets(c.Namespace).List(ctx, metav1.ListOptions{LabelSelector: LabelIdentity + "=" + c.Name})
	if err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		a, b := list.Items[i].CreationTimestamp, list.Items[j].CreationTimestamp
		if a.Equal(&b) {
			return list.Items[i].Name < list.Items[j].Name
		}
		return a.Before(&b)
	})
	return list.Items, nil
}

// NewToken asks kcp for a new token of the component and waits until kcp has signed it.
func NewToken(ctx context.Context, kcp kubernetes.Interface, c Component) (*corev1.Secret, error) {
	secret, err := kcp.CoreV1().Secrets(c.Namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: c.ServiceAccount + "-token-",
			Namespace:    c.Namespace,
			Labels:       map[string]string{LabelIdentity: c.Name},
			Annotations:  map[string]string{corev1.ServiceAccountNameKey: c.ServiceAccount},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("token secret: %w", err)
	}
	return signed(ctx, kcp, c.Namespace, secret.Name)
}

// signed waits until kcp's token controller has put the token into the Secret.
func signed(ctx context.Context, kcp kubernetes.Interface, namespace, name string) (*corev1.Secret, error) {
	var secret *corev1.Secret
	err := wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
		s, err := kcp.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		secret = s
		return len(s.Data[corev1.ServiceAccountTokenKey]) > 0, nil
	})
	if err != nil {
		return nil, fmt.Errorf("kcp signed no token into %s/%s: %w", namespace, name, err)
	}
	return secret, nil
}

// Issue returns the component's credential: its newest token, or a new one when it has none.
func Issue(ctx context.Context, kcp kubernetes.Interface, c Component, server string, ca []byte) (*Credential, error) {
	if err := Ensure(ctx, kcp, c); err != nil {
		return nil, err
	}
	tokens, err := Tokens(ctx, kcp, c)
	if err != nil {
		return nil, err
	}
	var secret *corev1.Secret
	if len(tokens) > 0 {
		secret, err = signed(ctx, kcp, c.Namespace, tokens[len(tokens)-1].Name)
	} else {
		secret, err = NewToken(ctx, kcp, c)
	}
	if err != nil {
		return nil, err
	}
	return credential(c, server, ca, secret)
}

// Rotate gives the component a new token. The older ones stay valid until Retire.
func Rotate(ctx context.Context, kcp kubernetes.Interface, c Component, server string, ca []byte) (*Credential, error) {
	if err := Ensure(ctx, kcp, c); err != nil {
		return nil, err
	}
	secret, err := NewToken(ctx, kcp, c)
	if err != nil {
		return nil, err
	}
	return credential(c, server, ca, secret)
}

// Retire deletes every token of the component but the one named keep; kcp stops accepting them at once.
func Retire(ctx context.Context, kcp kubernetes.Interface, c Component, keep string) ([]string, error) {
	tokens, err := Tokens(ctx, kcp, c)
	if err != nil {
		return nil, err
	}
	var retired []string
	for _, s := range tokens {
		if s.Name == keep {
			continue
		}
		if err := kcp.CoreV1().Secrets(c.Namespace).Delete(ctx, s.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return retired, fmt.Errorf("delete %s/%s: %w", c.Namespace, s.Name, err)
		}
		retired = append(retired, s.Name)
	}
	return retired, nil
}

func credential(c Component, server string, ca []byte, secret *corev1.Secret) (*Credential, error) {
	config := clientcmdapi.NewConfig()
	config.Clusters["kcp"] = &clientcmdapi.Cluster{Server: server, CertificateAuthorityData: ca}
	// The token is read from the file next to the kubeconfig and read again when it changes.
	config.AuthInfos[c.Name] = &clientcmdapi.AuthInfo{TokenFile: TokenKey}
	config.Contexts[c.Name] = &clientcmdapi.Context{Cluster: "kcp", AuthInfo: c.Name, Namespace: c.Namespace}
	config.CurrentContext = c.Name
	kubeconfig, err := clientcmd.Write(*config)
	if err != nil {
		return nil, err
	}
	return &Credential{Kubeconfig: kubeconfig, Token: secret.Data[corev1.ServiceAccountTokenKey], Secret: secret.Name}, nil
}

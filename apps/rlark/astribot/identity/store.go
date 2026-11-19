package identity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Store is where a component finds its credential.
type Store interface {
	Put(ctx context.Context, c Component, cred *Credential) error
	// Where says where the credential of c is kept.
	Where(c Component) string
}

// Dir keeps each credential in <dir>/<component>/{kubeconfig,token}, for components started by
// docker compose with that directory mounted.
type Dir string

// Put writes the token first and replaces each file whole, so a component never reads half of one.
func (d Dir) Put(_ context.Context, c Component, cred *Credential) error {
	dir := filepath.Join(string(d), c.Name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, f := range []struct {
		name string
		data []byte
	}{{TokenKey, cred.Token}, {KubeconfigKey, cred.Kubeconfig}} {
		path := filepath.Join(dir, f.name)
		tmp := filepath.Join(dir, "."+f.name)
		if err := os.WriteFile(tmp, f.data, 0o600); err != nil {
			return err
		}
		// A file written again keeps who may read it: a component that runs as a user of its own was
		// let read the first one.
		if old, err := os.Stat(path); err == nil {
			if err := os.Chmod(tmp, old.Mode().Perm()); err != nil {
				return err
			}
			if st, ok := old.Sys().(*syscall.Stat_t); ok {
				if err := os.Lchown(tmp, int(st.Uid), int(st.Gid)); err != nil {
					return err
				}
			}
		}
		if err := os.Rename(tmp, path); err != nil {
			return err
		}
	}
	return nil
}

func (d Dir) Where(c Component) string { return filepath.Join(string(d), c.Name) }

// Secrets keeps each credential in a Secret of the given name in a cluster that runs the
// component, mounted whole (not by subPath) so a rotated token reaches the running Pod.
type Secrets struct {
	Client    kubernetes.Interface
	Namespace string
	// Name gives the Secret's name for a component.
	Name func(Component) string
}

func (s Secrets) Put(ctx context.Context, c Component, cred *Credential) error {
	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: s.Name(c), Namespace: s.Namespace, Labels: map[string]string{LabelIdentity: c.Name}},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{KubeconfigKey: cred.Kubeconfig, TokenKey: cred.Token},
	}
	secrets := s.Client.CoreV1().Secrets(s.Namespace)
	current, err := secrets.Get(ctx, desired.Name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		_, err = secrets.Create(ctx, desired, metav1.CreateOptions{})
	case err == nil:
		current.Data = desired.Data
		if current.Labels == nil {
			current.Labels = map[string]string{}
		}
		current.Labels[LabelIdentity] = c.Name
		_, err = secrets.Update(ctx, current, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("secret %s/%s: %w", s.Namespace, desired.Name, err)
	}
	return nil
}

func (s Secrets) Where(c Component) string { return "Secret " + s.Namespace + "/" + s.Name(c) }

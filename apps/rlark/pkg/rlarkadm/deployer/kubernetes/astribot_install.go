package kubernetes

import (
	"context"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/component"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/health"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// InstallTo installs the components into the cluster restConfig reaches: the
// Gateway installs a new cluster's agent this way, with the cluster's
// credential held in memory only. The control plane's kcp needs Install, whose
// kubeconfig file kubectl reads.
func (d *Installer) InstallTo(ctx context.Context, restConfig *rest.Config, cfg *types.DeployConfig, certBundle *cert.Bundle) error {
	logger := log.FromContext(ctx)
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create clientset: %w", err)
	}

	if err := ensureNamespace(ctx, clientset); err != nil {
		return err
	}
	if cfg.UsesKubernetesManagementAPI() {
		apiExtensions, err := apiextensionsclient.NewForConfig(restConfig)
		if err != nil {
			return fmt.Errorf("create apiextensions clientset: %w", err)
		}
		if err := installCRDsToKubernetes(ctx, apiExtensions); err != nil {
			return err
		}
		if err := ensureUIAuthSecretInKubernetes(ctx, clientset, constants.Namespace); err != nil {
			return err
		}
	}

	if certBundle != nil {
		if err := createCertSecret(ctx, clientset, cfg, certBundle); err != nil {
			return err
		}
	}
	if err := putImagePullCredentials(ctx, clientset, cfg.Kubernetes.ImagePullCredentials); err != nil {
		return err
	}

	if cfg.DB != nil {
		if err := createDBConfigMap(ctx, clientset, cfg); err != nil {
			return err
		}
		if err := createPostgresInitConfigMap(ctx, clientset); err != nil {
			return err
		}
	}

	// todo 可以并发部署
	for _, c := range component.ComponentsForPlane(cfg) {
		c.HealthCheckFn = health.K8sWorkloadHealthCheck(clientset, c)
		if c.Name == constants.ComponentKCP {
			c.PostDeployFn = initializeKCPManagementAPIFn(ctx, clientset, d.kubeconfig)
		}
		if err := ensureRBAC(ctx, clientset, cfg, &c); err != nil {
			return err
		}

		if svc := component.Service(cfg, &c); svc != nil {
			if err := createService(ctx, clientset, svc); err != nil {
				return err
			}
			logger.Info("service created", "name", svc.Name)
		}

		if err := createWorkload(ctx, clientset, &c, cfg); err != nil {
			return err
		}

		logger.Info("component deployed", "name", c.Name, "port", c.Port)

		if err := health.WaitForHealthy(cfg, c); err != nil {
			return err
		}

		if c.PostDeployFn != nil {
			if err := c.PostDeployFn(cfg); err != nil {
				return err
			}
		}
	}

	logger.Info("plane deployed", "plane", cfg.Plane, "namespace", constants.Namespace)

	d.summary = d.buildSummary(ctx, clientset, cfg)
	return nil
}

// putImagePullCredentials puts in the image registry credentials RLark gave the cluster, before the agent that
// delivers them runs: its own image is pulled with them. The agent delivers each under the credential's own name and
// adopts nothing (its delivery policy), so the installer's copy has a name of its own, InstallCredentialName.
func putImagePullCredentials(ctx context.Context, clientset kubernetes.Interface, credentials []types.ImagePullCredential) error {
	for _, credential := range credentials {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: types.InstallCredentialName(credential.Name), Namespace: constants.Namespace},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(credential.DockerConfigJSON)},
		}
		existing, err := clientset.CoreV1().Secrets(constants.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			_, err = clientset.CoreV1().Secrets(constants.Namespace).Create(ctx, secret, metav1.CreateOptions{})
		case err == nil:
			existing.Data, existing.Type = secret.Data, secret.Type
			_, err = clientset.CoreV1().Secrets(constants.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
		}
		if err != nil {
			return fmt.Errorf("put image pull credential %s: %w", credential.Name, err)
		}
	}
	return nil
}

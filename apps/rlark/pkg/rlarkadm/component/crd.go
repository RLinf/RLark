package component

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// CRDInstaller defines the interface for installing CRDs.
type CRDInstaller interface {
	Install(ctx context.Context) error
}

// DirectCRDInstaller installs CRDs directly via the API.
type DirectCRDInstaller struct {
	Client apiextensionsclient.Interface
}

// Install installs CRDs directly via the API.
func (i *DirectCRDInstaller) Install(ctx context.Context) error {
	files, err := readCRDFiles()
	if err != nil {
		return err
	}

	for name, content := range files {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := yaml.Unmarshal(content, crd); err != nil {
			return fmt.Errorf("decode CRD %s: %w", name, err)
		}

		crds := i.Client.ApiextensionsV1().CustomResourceDefinitions()
		existing, err := crds.Get(ctx, crd.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			if _, err := crds.Create(ctx, crd, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create CRD %s: %w", crd.Name, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("get CRD %s: %w", crd.Name, err)
		}

		crd.ResourceVersion = existing.ResourceVersion
		if _, err := crds.Update(ctx, crd, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update CRD %s: %w", crd.Name, err)
		}
	}
	return nil
}

// ExecCRDInstaller installs CRDs via kubectl exec into a pod.
type ExecCRDInstaller struct {
	PodName    string
	Kubeconfig string
}

// Install installs CRDs via kubectl exec.
func (i *ExecCRDInstaller) Install(ctx context.Context) error {
	logger := log.GetLogger()

	files, err := readCRDFiles()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		logger.Error(nil, "no CRD files embedded, skipping CRD install")
		return nil
	}

	for crdName, content := range files {
		kc := constants.KCPDataDir + "/admin.kubeconfig"
		var lastErr error
		for attempt := 0; attempt < 5; attempt++ {
			if err := i.applyCRD(crdName, content, kc); err == nil {
				lastErr = nil
				break
			} else {
				lastErr = err
			}
			if attempt < 2 {
				time.Sleep(5 * time.Second)
			}
		}
		if lastErr != nil {
			return lastErr
		}
		logger.Info("applied CRD", "file", crdName)
	}

	return nil
}

func (i *ExecCRDInstaller) applyCRD(crdName string, content []byte, kc string) error {
	// Try create first
	createCmd := kubectlCommand(i.Kubeconfig, "-n", constants.Namespace, "exec", "-i", i.PodName, "--",
		"kubectl", "--kubeconfig", kc, "create", "--validate=false", "-f", "-")
	createCmd.Stdin = strings.NewReader(string(content))
	var createErr bytes.Buffer
	createCmd.Stderr = &createErr
	if err := createCmd.Run(); err == nil {
		return nil
	}

	// Try replace
	replaceCmd := kubectlCommand(i.Kubeconfig, "-n", constants.Namespace, "exec", "-i", i.PodName, "--",
		"kubectl", "--kubeconfig", kc, "replace", "--validate=false", "-f", "-")
	replaceCmd.Stdin = strings.NewReader(string(content))
	var replaceErr bytes.Buffer
	replaceCmd.Stderr = &replaceErr
	if err := replaceCmd.Run(); err == nil {
		return nil
	}

	// If immutable field error, delete and recreate
	if strings.Contains(replaceErr.String(), "field is immutable") {
		deleteCmd := kubectlCommand(i.Kubeconfig, "-n", constants.Namespace, "exec", i.PodName, "--",
			"kubectl", "--kubeconfig", kc, "delete", "--ignore-not-found", "-f", "-")
		deleteCmd.Stdin = strings.NewReader(string(content))
		var deleteErr bytes.Buffer
		deleteCmd.Stderr = &deleteErr
		if err := deleteCmd.Run(); err != nil {
			return fmt.Errorf("delete CRD %s: %s", crdName, deleteErr.String())
		}

		recreateCmd := kubectlCommand(i.Kubeconfig, "-n", constants.Namespace, "exec", "-i", i.PodName, "--",
			"kubectl", "--kubeconfig", kc, "create", "--validate=false", "-f", "-")
		recreateCmd.Stdin = strings.NewReader(string(content))
		var recreateErr bytes.Buffer
		recreateCmd.Stderr = &recreateErr
		if err := recreateCmd.Run(); err != nil {
			return fmt.Errorf("recreate CRD %s: %s", crdName, recreateErr.String())
		}
		return nil
	}

	return fmt.Errorf("apply CRD %s: create: %s | replace: %s", crdName, createErr.String(), replaceErr.String())
}

// kubectlCommand creates a kubectl command with optional kubeconfig.
func kubectlCommand(kubeconfig string, args ...string) *exec.Cmd {
	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}
	return exec.Command("kubectl", args...)
}

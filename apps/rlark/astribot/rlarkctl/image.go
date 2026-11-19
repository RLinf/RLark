package rlarkctl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	rlarkflags "github.com/rlinf/rlark/apps/rlark/pkg/rlarkctl"
)

// SiteRegistry is the name every site resolves to its own image registry or cache.
const SiteRegistry = "registry.rlark.internal:5443"

// registryCommand gives the tools that talk to the site registries (skopeo, BuildKit, docker) what they need
// from the management plane: the login, through the docker credential helper protocol so that it is never
// written to a file, and the CA the registries' certificates are signed by.
func registryCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "registry", Short: "Login and trust for the site image registries."}
	cmd.AddCommand(&cobra.Command{
		Use:   "credential-helper get",
		Short: "Docker credential helper: the server URL on stdin, its login from the management plane on stdout.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "get" {
				return fmt.Errorf("only get is supported")
			}
			server, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
			if err != nil && err != io.EOF {
				return err
			}
			clientset, err := kubeClientset()
			if err != nil {
				return err
			}
			username, password, err := registryLogin(cmd.Context(), clientset, server)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"ServerURL": strings.TrimSpace(server), "Username": username, "Secret": password})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "ca",
		Short: "Print the CA the site registries' certificates are signed by (a public certificate).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ca, err := registryCA(cmd.Context())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), ca)
			return err
		},
	})
	return cmd
}

// imageCommand pushes a built image into the site registry.
func imageCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "image", Short: "Images in the site registries."}
	var retries int
	var registry string
	push := &cobra.Command{
		Use:   "push SOURCE REPOSITORY[:TAG]",
		Short: "Copy an image (any skopeo source, e.g. docker-daemon:fleet/robot:abc) into the site registry; prints the reference by digest.",
		Long: "The login comes from the management plane through the credential helper and is never written to a file;\n" +
			"the registry's certificate is checked against the site CA. A failed copy is retried, and the image is read\n" +
			"back by digest before the reference is printed.",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ca, err := registryCA(ctx)
			if err != nil {
				return err
			}
			self, err := os.Executable()
			if err != nil {
				return err
			}
			work, err := os.MkdirTemp("", "rlarkctl-push-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(work)
			if err := writePushTools(work, self, registry, ca); err != nil {
				return err
			}
			destination := "docker://" + registry + "/" + args[1]
			env := append(os.Environ(), "PATH="+work+string(os.PathListSeparator)+os.Getenv("PATH"))
			// --insecure-policy: the images are ours and unsigned; what is checked is the registry's certificate.
			copyArgs := []string{"--insecure-policy", "copy", "--retry-times", strconv.Itoa(retries), "--dest-authfile", filepath.Join(work, "auth.json"), "--dest-cert-dir", filepath.Join(work, "certs"), args[0], destination}
			if err := run(ctx, env, cmd.ErrOrStderr(), nil, "skopeo", copyArgs...); err != nil {
				return err
			}
			var digest strings.Builder
			inspectArgs := []string{"--insecure-policy", "inspect", "--retry-times", strconv.Itoa(retries), "--authfile", filepath.Join(work, "auth.json"), "--cert-dir", filepath.Join(work, "certs"), "--format", "{{.Digest}}", destination}
			if err := run(ctx, env, cmd.ErrOrStderr(), &digest, "skopeo", inspectArgs...); err != nil {
				return err
			}
			repository := args[1]
			if i := strings.LastIndex(repository, ":"); i > strings.LastIndex(repository, "/") {
				repository = repository[:i]
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s/%s@%s\n", registry, repository, strings.TrimSpace(digest.String()))
			return err
		},
	}
	push.Flags().IntVar(&retries, "retries", 5, "How many times a failed copy is tried again")
	push.Flags().StringVar(&registry, "registry", SiteRegistry, "The registry to push to, host:port")
	cmd.AddCommand(push)
	return cmd
}

// writePushTools lays down, in a private directory, what skopeo needs: an auth file that only names the
// credential helper (no secret in it), the helper itself as a small script calling this binary, and the CA.
func writePushTools(dir, self, registry, ca string) error {
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"credHelpers":{"`+registry+`":"rlark"}}`), 0o600); err != nil {
		return err
	}
	helper := "#!/bin/sh\nexec " + strconv.Quote(self) + " registry credential-helper \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "docker-credential-rlark"), []byte(helper), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "certs"), 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "certs", "ca.crt"), []byte(ca+"\n"), 0o600)
}

func run(ctx context.Context, env []string, stderr io.Writer, stdout io.Writer, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Env, c.Stderr, c.Stdout = env, stderr, stdout
	if stdout == nil {
		c.Stdout = stderr
	}
	return c.Run()
}

func kubeClientset() (kubernetes.Interface, error) {
	config, err := rlarkflags.KubeClientConfig.BuildRestConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

// registryLogin finds the image registry credential for the server among the management plane's image
// registry objects.
func registryLogin(ctx context.Context, clientset kubernetes.Interface, server string) (string, string, error) {
	host := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(server), "https://"), "http://"), "/")
	secrets := &corev1.SecretList{}
	if id := os.Getenv("RLARK_REGISTRY_CREDENTIAL"); id != "" {
		// A pusher's kubeconfig may be allowed to read only this one object (RBAC resourceNames), not to list secrets.
		secret, err := clientset.CoreV1().Secrets(metav1.NamespaceDefault).Get(ctx, id+"-replication", metav1.GetOptions{})
		if err != nil {
			return "", "", err
		}
		secrets.Items = append(secrets.Items, *secret)
	} else {
		list, err := clientset.CoreV1().Secrets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: common.ImageRegistryReplicationLabel + "=true"})
		if err != nil {
			return "", "", err
		}
		secrets = list
	}
	for _, secret := range secrets.Items {
		var credential struct{ Registry, Username, Password string }
		if err := json.Unmarshal(secret.Data[common.ImageRegistryCredentialDataKey], &credential); err != nil {
			continue
		}
		if credential.Registry == host {
			return credential.Username, credential.Password, nil
		}
	}
	return "", "", fmt.Errorf("credentials not found in native keychain") // the helper protocol's own wording
}

// registryCA is the caCert of the registry-trust Addon: the CA every node already trusts the registries with.
func registryCA(ctx context.Context) (string, error) {
	config, err := rlarkflags.KubeClientConfig.BuildRestConfig()
	if err != nil {
		return "", err
	}
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return "", err
	}
	addons, err := client.Resource(schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: "addons"}).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	for _, addon := range addons.Items {
		name, _, _ := unstructured.NestedString(addon.Object, "spec", "addonName")
		ca, _, _ := unstructured.NestedString(addon.Object, "spec", "values", "caCert")
		if name == "registry-trust" && ca != "" {
			return strings.TrimSpace(ca), nil
		}
	}
	return "", fmt.Errorf("no registry-trust Addon with a caCert in the management plane")
}

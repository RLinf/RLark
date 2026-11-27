// Package stationmember registers a member cluster with the site's Station: what the cluster runs for it (the
// member RBAC, Station API's CA, the station agents) and how the management plane reaches its API (the credentials
// station-api and station-controller read from kcp). The Gateway does it for a cluster it makes; rlarkctl station
// register for one made otherwise (the cluster Station itself runs in). The manifests are the release's tree's.
package stationmember

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/retry"
)

const (
	// Namespace is Station's in a member cluster.
	Namespace = "station-system"

	credentialNamespace          = "station-member-credentials"
	operationCredentialNamespace = "station-operation-member-credentials"
)

// Manifests are the release's tree's files a member cluster is given (astribot/station/deploy).
type Manifests struct {
	MemberRBAC, OperationRBAC, StationAgent string
}

// Registration is what a member cluster is registered with.
type Registration struct {
	// Cluster is the cluster's name; APIName the name the API's certificate holds (the control plane's name in the
	// SiteNetwork's names), which the credentials verify it by.
	Cluster, APIName string
	// StationAPIURL is Station API as the cluster's agents reach it (https://host:port); StationAPICA its CA.
	StationAPIURL string
	StationAPICA  []byte
	// AgentImage and WireguardImage are the station agents'; PullSecret the image registry credential RLark
	// delivers into station-system, by its id (ir-…).
	AgentImage, WireguardImage, PullSecret string
	Manifests                              Manifests
}

// Register installs Station's part into the member and stores, in kcp, how station-api and station-controller reach
// the member's API. Done again, it changes nothing that is as it should be.
func Register(ctx context.Context, m *Member, management kubernetes.Interface, r Registration) error {
	if err := m.Apply(ctx, r.Manifests.MemberRBAC); err != nil {
		return fmt.Errorf("Station's member RBAC: %w", err)
	}
	caSecret, _ := json.Marshal(&corev1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: "station-api-ca", Namespace: Namespace},
		Data:       map[string][]byte{"ca.crt": r.StationAPICA},
	})
	if err := m.Apply(ctx, string(caSecret)); err != nil {
		return err
	}
	manifest := strings.NewReplacer(
		"https://REPLACE_STATION_API_ENDPOINT", r.StationAPIURL,
		"REPLACE_REGISTERED_CLUSTER_ID", r.Cluster,
		"REPLACE_RLARK_IMAGE", r.AgentImage,
		"REPLACE_PRIVATE_WIREGUARD_IMAGE", r.WireguardImage,
		"REPLACE_IMAGE_PULL_SECRET", r.PullSecret,
	).Replace(r.Manifests.StationAgent)
	if err := m.Apply(ctx, manifest); err != nil {
		return fmt.Errorf("Station's agents: %w", err)
	}

	reader, err := m.Token(ctx, Namespace, "station-api-reader", "station-api-reader-token")
	if err != nil {
		return err
	}
	agent, err := m.Client.CoreV1().ServiceAccounts(Namespace).Get(ctx, "station-agent", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("the ServiceAccount station-agent: %w", err)
	}
	readerConfig, err := m.Kubeconfig(r.APIName, reader)
	if err != nil {
		return err
	}
	name := SecretName(r.Cluster)
	annotations := map[string]string{"station.rlark.io/cluster-id": r.Cluster}
	if err := PutSecret(ctx, management, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: credentialNamespace, Annotations: annotations},
		Data: map[string][]byte{
			"kubeconfig": readerConfig, "service-account-namespace": []byte(Namespace), "service-account-name": []byte("station-agent"),
			"service-account-uid": []byte(agent.UID), "audience": []byte("station.rlark.io"),
		},
	}); err != nil {
		return fmt.Errorf("store Station's member credential: %w", err)
	}
	if err := m.Apply(ctx, r.Manifests.OperationRBAC); err != nil {
		return fmt.Errorf("Station's node operation RBAC: %w", err)
	}
	operator, err := m.Token(ctx, Namespace, "station-node-operator", "station-node-operator-token")
	if err != nil {
		return err
	}
	operatorConfig, err := m.Kubeconfig(r.APIName, operator)
	if err != nil {
		return err
	}
	if err := PutSecret(ctx, management, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: operationCredentialNamespace, Annotations: annotations},
		Data:       map[string][]byte{"kubeconfig": operatorConfig},
	}); err != nil {
		return fmt.Errorf("store Station's node operation credential: %w", err)
	}
	return nil
}

// SecretName is the name of a cluster's Station credentials in kcp, as station-controller looks them up
// (member_credential_names.go).
func SecretName(cluster string) string {
	digest := sha256.Sum256([]byte(cluster))
	return "station-member-" + strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(digest[:10]), "="))
}

// PutSecret writes a credential of a member cluster into kcp, in a namespace of such credentials.
func PutSecret(ctx context.Context, management kubernetes.Interface, secret *corev1.Secret) error {
	if _, err := management.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: secret.Namespace}}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	secrets := management.CoreV1().Secrets(secret.Namespace)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := secrets.Get(ctx, secret.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			_, err = secrets.Create(ctx, secret, metav1.CreateOptions{})
			return err
		}
		if err != nil {
			return err
		}
		current.Data, current.Annotations = secret.Data, secret.Annotations
		_, err = secrets.Update(ctx, current, metav1.UpdateOptions{})
		return err
	})
}

// Member is a member cluster, reached as its administrator.
type Member struct {
	Config  *rest.Config
	Client  kubernetes.Interface
	Dynamic dynamic.Interface
	mapper  *restmapper.DeferredDiscoveryRESTMapper
}

func New(config *rest.Config) (*Member, error) {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	disco, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Member{Config: config, Client: client, Dynamic: dyn, mapper: restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disco))}, nil
}

// Apply applies every object of a manifest, as the Gateway's field manager.
func (m *Member) Apply(ctx context.Context, manifest string) error {
	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		object := &unstructured.Unstructured{}
		if err := decoder.Decode(&object.Object); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(object.Object) == 0 {
			continue
		}
		gvk := object.GroupVersionKind()
		mapping, err := m.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return fmt.Errorf("%s %s: %w", gvk.Kind, object.GetName(), err)
		}
		resource := m.Dynamic.Resource(mapping.Resource)
		var client dynamic.ResourceInterface = resource
		if mapping.Scope.Name() == "namespace" {
			client = resource.Namespace(object.GetNamespace())
		}
		data, err := json.Marshal(object.Object)
		if err != nil {
			return err
		}
		force := true
		if _, err := client.Patch(ctx, object.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{FieldManager: "rlark-gateway", Force: &force}); err != nil {
			return fmt.Errorf("%s %s: %w", gvk.Kind, object.GetName(), err)
		}
	}
}

// Token is the long-lived token of a ServiceAccount, by a Secret of its own.
func (m *Member) Token(ctx context.Context, namespace, serviceAccount, secret string) (string, error) {
	_, err := m.Client.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secret, Namespace: namespace, Annotations: map[string]string{corev1.ServiceAccountNameKey: serviceAccount}},
		Type:       corev1.SecretTypeServiceAccountToken,
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return "", fmt.Errorf("the token Secret %s/%s: %w", namespace, secret, err)
	}
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(time.Second) {
		got, err := m.Client.CoreV1().Secrets(namespace).Get(ctx, secret, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if token := got.Data[corev1.ServiceAccountTokenKey]; len(token) > 0 {
			return string(token), nil
		}
	}
	return "", fmt.Errorf("the token controller has not filled %s/%s in 1 min", namespace, secret)
}

// Kubeconfig is how the management plane reaches the member's API with a token: at the address the member is
// reached at here (m.Config.Host: the address the SiteNetwork's names give the control plane's name), verified as
// name, the name the API's certificate holds. station-controller keeps the address at what the names say.
func (m *Member) Kubeconfig(name, token string) ([]byte, error) {
	config := clientcmdapi.NewConfig()
	config.Clusters["member"] = &clientcmdapi.Cluster{Server: m.Config.Host, CertificateAuthorityData: m.Config.CAData, TLSServerName: name}
	config.AuthInfos["member"] = &clientcmdapi.AuthInfo{Token: token}
	config.Contexts["member"] = &clientcmdapi.Context{Cluster: "member", AuthInfo: "member"}
	config.CurrentContext = "member"
	return clientcmd.Write(*config)
}

// DaemonSetReady waits until a DaemonSet runs its current Pods on every node it wants.
func (m *Member) DaemonSetReady(ctx context.Context, namespace, name string, within time.Duration) (string, error) {
	var ds *appsv1.DaemonSet
	var err error
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(5 * time.Second) {
		if ds, err = m.Client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return "", err
		}
		s := ds.Status
		if s.ObservedGeneration >= ds.Generation && s.DesiredNumberScheduled > 0 && s.NumberReady == s.DesiredNumberScheduled && s.UpdatedNumberScheduled == s.DesiredNumberScheduled {
			return fmt.Sprintf("%s %d/%d", name, s.NumberReady, s.DesiredNumberScheduled), nil
		}
	}
	if ds == nil {
		return "", fmt.Errorf("DaemonSet %s/%s is not there after %s", namespace, name, within)
	}
	return "", fmt.Errorf("DaemonSet %s/%s is %d/%d ready after %s", namespace, name, ds.Status.NumberReady, ds.Status.DesiredNumberScheduled, within)
}

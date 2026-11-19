package stationidentity

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rlinf/rlark/apps/rlark/astribot/stationmember"
)

const (
	memberKubeconfigKey     = "kubeconfig"
	memberServiceAccountNS  = "service-account-namespace"
	memberServiceAccount    = "service-account-name"
	memberServiceAccountUID = "service-account-uid"
	memberAudience          = "audience"
	memberClusterIDKey      = "station.rlark.io/cluster-id"
)

var stationListGVR = schema.GroupVersionResource{Group: "station.rlark.io", Version: "v1alpha1", Resource: "stations"}
var ErrStationBindingNotFound = errors.New("Station binding not found")

type MemberClientFactory func([]byte) (kubernetes.Interface, error)

type memberClientEntry struct {
	digest [32]byte
	client kubernetes.Interface
}

// KubernetesClusterRegistry resolves a requested cluster ID only through the
// management API's Cluster object and its dedicated credential Secret.
type KubernetesClusterRegistry struct {
	Management      dynamic.Interface
	Core            kubernetes.Interface
	CredentialsNS   string
	NewMemberClient MemberClientFactory

	mu    sync.Mutex
	cache map[string]memberClientEntry
}

func NewKubernetesClusterRegistry(management dynamic.Interface, core kubernetes.Interface, credentialsNamespace string, factory MemberClientFactory) *KubernetesClusterRegistry {
	if factory == nil {
		factory = newMemberKubernetesClient
	}
	return &KubernetesClusterRegistry{
		Management: management, Core: core, CredentialsNS: credentialsNamespace,
		NewMemberClient: factory, cache: make(map[string]memberClientEntry),
	}
}

func (r *KubernetesClusterRegistry) Reader(ctx context.Context, clusterID string) (IdentityReader, error) {
	_, err := (KubernetesClusterNamespaceResolver{Client: r.Management}).ResolveObservationNamespace(ctx, clusterID)
	if errors.Is(err, ErrClusterNotRegistered) {
		return nil, ErrClusterNotRegistered
	}
	if err != nil {
		return nil, fmt.Errorf("resolve registered cluster: %w", err)
	}
	cluster, err := r.Management.Resource(rlarkClusterGVR).Get(ctx, clusterID, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, ErrClusterNotRegistered
	}
	if err != nil {
		return nil, fmt.Errorf("read registered Cluster: %w", err)
	}
	delisted, found, err := unstructured.NestedBool(cluster.Object, "spec", "delisted")
	if err != nil {
		return nil, fmt.Errorf("read Cluster delisted state: %w", err)
	}
	if found && delisted {
		return nil, ErrClusterNotRegistered
	}

	secretName := stationmember.SecretName(clusterID)
	secret, err := r.Core.CoreV1().Secrets(r.CredentialsNS).Get(ctx, secretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		r.evict(clusterID)
		return nil, ErrClusterNotRegistered
	}
	if err != nil {
		return nil, fmt.Errorf("read member credential Secret: %w", err)
	}
	if secret.Annotations[memberClusterIDKey] != clusterID {
		r.evict(clusterID)
		return nil, errors.New("member credential Secret ownership mismatch")
	}
	data := secret.Data
	kubeconfig := data[memberKubeconfigKey]
	reg := Registration{
		ClusterID: clusterID, Namespace: string(data[memberServiceAccountNS]),
		ServiceAccount: string(data[memberServiceAccount]), ServiceAccountUID: string(data[memberServiceAccountUID]),
		Audience: string(data[memberAudience]),
	}
	if len(kubeconfig) == 0 || reg.Namespace == "" || reg.ServiceAccount == "" || reg.ServiceAccountUID == "" || reg.Audience == "" {
		r.evict(clusterID)
		return nil, errors.New("member credential Secret is incomplete")
	}
	digest := sha256.Sum256(kubeconfig)
	r.mu.Lock()
	entry, ok := r.cache[clusterID]
	if !ok || entry.digest != digest {
		memberClient, createErr := r.NewMemberClient(append([]byte(nil), kubeconfig...))
		if createErr != nil {
			r.mu.Unlock()
			return nil, fmt.Errorf("create registered member client: %w", createErr)
		}
		entry = memberClientEntry{digest: digest, client: memberClient}
		r.cache[clusterID] = entry
	}
	r.mu.Unlock()
	return registeredReader{cluster: RegisteredCluster{
		Registration: reg, Client: entry.client,
		Bindings: KubernetesStationBindingResolver{Management: r.Management},
	}}, nil
}

// MemberClient returns the client of a registered cluster.
func (r *KubernetesClusterRegistry) MemberClient(ctx context.Context, clusterID string) (kubernetes.Interface, error) {
	reader, err := r.Reader(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	return reader.(registeredReader).cluster.Client, nil
}

func (r *KubernetesClusterRegistry) evict(clusterID string) {
	r.mu.Lock()
	delete(r.cache, clusterID)
	r.mu.Unlock()
}

// KubernetesStationBindingResolver reads current Station objects from the
// authoritative management namespace and rejects ambiguous Node bindings.
type KubernetesStationBindingResolver struct{ Management dynamic.Interface }

func (r KubernetesStationBindingResolver) Resolve(ctx context.Context, clusterID, nodeName, nodeUID string) (Binding, error) {
	namespace, err := (KubernetesClusterNamespaceResolver{Client: r.Management}).ResolveObservationNamespace(ctx, clusterID)
	if err != nil {
		if !errors.Is(err, ErrClusterNotRegistered) {
			return Binding{}, fmt.Errorf("%w: resolve Station namespace", ErrIdentityUnavailable)
		}
		return Binding{}, fmt.Errorf("resolve Station namespace: %w", err)
	}
	stations, err := r.Management.Resource(stationListGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Binding{}, fmt.Errorf("%w: list current Stations: %w", ErrIdentityUnavailable, err)
	}
	var binding Binding
	for i := range stations.Items {
		station := &stations.Items[i]
		name, _, nameErr := unstructured.NestedString(station.Object, "spec", "binding", "nodeName")
		uid, _, uidErr := unstructured.NestedString(station.Object, "spec", "binding", "nodeUID")
		if nameErr != nil || uidErr != nil {
			return Binding{}, fmt.Errorf("read Station %q binding", station.GetName())
		}
		retired, _, retiredErr := unstructured.NestedBool(station.Object, "spec", "retired")
		// A retired Station's node was removed: its name is free for the next machine.
		if name != nodeName || retired && uid != nodeUID {
			continue
		}
		if binding.StationUID != "" {
			return Binding{}, errors.New("multiple Stations claim the same Node binding")
		}
		if uid != nodeUID {
			// A reinstalled machine under the same name is a verified Node with no
			// binding of its own: it may report, so that the Station shows the
			// conflict and an administrator can rebind it, and nothing more.
			return Binding{}, fmt.Errorf("%w: Station %q is bound to another Node UID under this name", ErrStationBindingNotFound, station.GetName())
		}
		version, _, versionErr := unstructured.NestedInt64(station.Object, "spec", "binding", "version")
		if retiredErr != nil || versionErr != nil || version <= 0 || station.GetUID() == "" {
			return Binding{}, fmt.Errorf("Station %q has invalid binding identity", station.GetName())
		}
		phase, _, phaseErr := unstructured.NestedString(station.Object, "status", "phase")
		if phaseErr != nil {
			return Binding{}, fmt.Errorf("read Station %q phase", station.GetName())
		}
		// A Conflict phase may reflect a stale pre-rebind observation. The
		// current Station binding and member Node UID were verified above, so
		// that exact agent must be allowed to publish the fresh observation that
		// lets the Station controller resolve the conflict.
		inactive := retired || phase == "Retired"
		binding = Binding{NodeUID: uid, StationUID: string(station.GetUID()), Generation: version, Inactive: inactive}
	}
	if binding.StationUID == "" {
		return Binding{}, ErrStationBindingNotFound
	}
	return binding, nil
}

func newMemberKubernetesClient(kubeconfig []byte) (kubernetes.Interface, error) {
	config, err := secureMemberRESTConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func secureMemberRESTConfig(kubeconfig []byte) (*rest.Config, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, errors.New("invalid member kubeconfig")
	}
	serverURL, err := url.Parse(config.Host)
	if err != nil || serverURL.Scheme != "https" || serverURL.Host == "" {
		return nil, errors.New("member API server must use HTTPS")
	}
	if config.TLSClientConfig.Insecure || config.ExecProvider != nil || config.AuthProvider != nil {
		return nil, errors.New("insecure TLS and external credential plugins are unsupported")
	}
	if config.TLSClientConfig.CAFile != "" || config.TLSClientConfig.CertFile != "" || config.TLSClientConfig.KeyFile != "" {
		return nil, errors.New("file-path TLS credentials are unsupported; embed data in the management Secret")
	}
	config.Timeout = 10 * time.Second
	return config, nil
}

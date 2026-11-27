package controller

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MemberKubeconfigSecretKey = "kubeconfig"
	ClusterIDAnnotation       = "station.rlark.io/cluster-id"
	// memberAPITimeout bounds one request to a member cluster API. A node
	// operation waits on that API (the reset pod, the Node delete, the
	// verification); without a bound, an API endpoint that is itself the
	// node being removed leaves the operation Running forever instead of
	// recording a Blocked condition.
	memberAPITimeout = 30 * time.Second
)

// SecretKubeconfigSource reads verified member credentials from the management API.
// It never reads a host path or writes credentials to disk.
type SecretKubeconfigSource struct {
	Reader    client.Reader
	Namespace string
}

func (s SecretKubeconfigSource) Kubeconfig(ctx context.Context, clusterID string) ([]byte, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: s.Namespace, Name: MemberKubeconfigSecretName(clusterID)}
	if err := s.Reader.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("read member kubeconfig Secret: %w", err)
	}
	if secret.Annotations[ClusterIDAnnotation] != clusterID {
		return nil, errors.New("member kubeconfig Secret ownership does not match the registered cluster")
	}
	data := secret.Data[MemberKubeconfigSecretKey]
	if len(data) == 0 {
		return nil, errors.New("member kubeconfig Secret has no kubeconfig data")
	}
	return append([]byte(nil), data...), nil
}

// MemberKubeconfigSecretName is deterministic but does not expose arbitrary cluster IDs in names.
func MemberKubeconfigSecretName(clusterID string) string {
	digest := sha256.Sum256([]byte(clusterID))
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest[:10])
	return "station-member-" + strings.ToLower(encoded)
}

type MemberKubeconfigSource interface {
	Kubeconfig(context.Context, string) ([]byte, error)
}

type cachedMemberClient struct {
	digest [32]byte
	client kubernetes.Interface
}

// KubeconfigNodeReader validates target Node identity against the member API.
// Kubeconfigs are sourced from management Secrets, TLS-verified by client-go,
// and retained only in the in-memory client cache.
type KubeconfigNodeReader struct {
	source MemberKubeconfigSource
	mu     sync.Mutex
	cache  map[string]cachedMemberClient
}

func NewKubeconfigNodeReader(source MemberKubeconfigSource) *KubeconfigNodeReader {
	return &KubeconfigNodeReader{source: source, cache: make(map[string]cachedMemberClient)}
}

// Client resolves a verified member API client and refreshes it on credential rotation.
func (r *KubeconfigNodeReader) Client(ctx context.Context, clusterID string) (kubernetes.Interface, error) {
	if clusterID == "" {
		return nil, errors.New("cluster ID is required")
	}
	kubeconfig, err := r.source.Kubeconfig(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("resolve member Kubernetes credentials: %w", err)
	}
	digest := sha256.Sum256(kubeconfig)
	r.mu.Lock()
	entry, ok := r.cache[clusterID]
	if !ok || entry.digest != digest {
		config, configErr := secureRESTConfigFromKubeconfig(kubeconfig)
		if configErr != nil {
			r.mu.Unlock()
			return nil, fmt.Errorf("member kubeconfig is not supported: %w", configErr)
		}
		memberClient, clientErr := kubernetes.NewForConfig(config)
		if clientErr != nil {
			r.mu.Unlock()
			return nil, fmt.Errorf("create member Kubernetes client: %w", clientErr)
		}
		entry = cachedMemberClient{digest: digest, client: memberClient}
		r.cache[clusterID] = entry
	}
	r.mu.Unlock()
	return entry.client, nil
}

func secureRESTConfigFromKubeconfig(kubeconfig []byte) (*rest.Config, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, errors.New("invalid kubeconfig")
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
	config.Timeout = memberAPITimeout
	return config, nil
}

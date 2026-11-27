package release

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// The release reaches each member cluster as the ServiceAccount rlark-system/rlark-release there (releaseMemberRBAC
// states what it may do), by a kubeconfig the management plane keeps for the cluster: the Secret
// CredentialNamespace/release-<cluster>, key kubeconfig, labelled with the cluster. Its server is the address the
// SiteNetwork's names give the cluster's API and its tls-server-name that name, as Station's member credentials:
// station-controller keeps the address at what the names say, so a control plane that moves is followed (ReachedAs).
const (
	CredentialNamespace = "rlark-release-member-credentials"
	credentialKey       = "kubeconfig"
	ClusterLabel        = "rlark.io/cluster-id"
	releaseAccount      = "rlark-release"
	releaseToken        = "rlark-release-token"
	readerAccount       = "rlark-reader"
	// readerHours is how long a token of the reader lasts.
	readerHours = 1
)

// CredentialName is the Secret of a cluster's credential in CredentialNamespace.
func CredentialName(cluster string) string { return "release-" + cluster }

// IssueMemberCredential gives the release its way into a member cluster: what it may do there (rbac, the tree's
// astribot/release/member-rbac.yaml, which holds its ServiceAccount and the Secret of its token) written by member,
// whoever administers the cluster; then the token as a kubeconfig whose server is server, verified as serverName
// (ReachedAs: the address the site's names give the API's name, and that name), kept in the management plane. The
// Gateway does it for a cluster it makes; rlarkctl release credential for one it did not.
func IssueMemberCredential(ctx context.Context, member kubernetes.Interface, memberDefinitions dynamic.Interface, management kubernetes.Interface, cluster, server, serverName string, rbac []byte) error {
	definitions, err := ParseDefinitions(releaseMemberRBAC, rbac)
	if err != nil {
		return err
	}
	if err := applyDefinitions(ctx, memberDefinitions, definitions, cluster); err != nil {
		return err
	}
	var token *corev1.Secret
	if err := wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
		token, err = member.CoreV1().Secrets(agentNamespace).Get(ctx, releaseToken, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return len(token.Data["token"]) > 0 && len(token.Data["ca.crt"]) > 0, nil
	}); err != nil {
		return fmt.Errorf("cluster %s: the token of %s/%s: %w", cluster, agentNamespace, releaseAccount, err)
	}
	config := clientcmdapi.NewConfig()
	config.Clusters[cluster] = &clientcmdapi.Cluster{Server: server, TLSServerName: serverName, CertificateAuthorityData: token.Data["ca.crt"]}
	config.AuthInfos[releaseAccount] = &clientcmdapi.AuthInfo{Token: string(token.Data["token"])}
	config.Contexts[cluster] = &clientcmdapi.Context{Cluster: cluster, AuthInfo: releaseAccount}
	config.CurrentContext = cluster
	kubeconfig, err := clientcmd.Write(*config)
	if err != nil {
		return err
	}

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: CredentialNamespace}}
	if _, err := management.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: CredentialNamespace, Name: CredentialName(cluster), Labels: map[string]string{ClusterLabel: cluster}},
		Data:       map[string][]byte{credentialKey: kubeconfig},
	}
	secrets := management.CoreV1().Secrets(CredentialNamespace)
	_, err = secrets.Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
	}
	return err
}

// ReaderKubeconfig is a kubeconfig of the cluster's reader (astribot/release/member-rbac.yaml, which every release
// writes): the release's credential's way to the cluster, with a token of the reader for an hour, which the release
// asks the cluster for (TokenRequest); no token of the reader is kept anywhere.
func ReaderKubeconfig(ctx context.Context, management kubernetes.Interface, cluster string) ([]byte, error) {
	secret, err := management.CoreV1().Secrets(CredentialNamespace).Get(ctx, CredentialName(cluster), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("cluster %s: the release's credential: %w", cluster, err)
	}
	config, err := clientcmd.Load(secret.Data[credentialKey])
	if err != nil {
		return nil, fmt.Errorf("cluster %s: %w", cluster, err)
	}
	rest, err := clientcmd.NewDefaultClientConfig(*config, nil).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("cluster %s: %w", cluster, err)
	}
	member, err := kubernetes.NewForConfig(rest)
	if err != nil {
		return nil, err
	}
	seconds := int64(readerHours * 3600)
	token, err := member.CoreV1().ServiceAccounts(agentNamespace).CreateToken(ctx, readerAccount,
		&authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &seconds}}, metav1.CreateOptions{})
	if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
		return nil, fmt.Errorf("cluster %s has no reader yet: a release writes it (astribot/release/member-rbac.yaml): %w", cluster, err)
	}
	if err != nil {
		return nil, fmt.Errorf("cluster %s: a token of the reader: %w", cluster, err)
	}
	config.AuthInfos = map[string]*clientcmdapi.AuthInfo{readerAccount: {Token: token.Status.Token}}
	for _, context := range config.Contexts {
		context.AuthInfo = readerAccount
	}
	return clientcmd.Write(*config)
}

// ReachedAs is how the release's credential reaches a cluster's API, given how its administrator does from here
// (administrator.Host, verified as its tls-server-name or the host's name): where the SiteNetwork's names declare that
// name (cp.<…>.rlark.internal), at the address they give it, verified as the name, which station-controller keeps
// current; as the administrator does otherwise (a cluster whose API the site does not name).
func ReachedAs(ctx context.Context, management dynamic.Interface, administrator *rest.Config) (server, serverName string, err error) {
	api, err := url.Parse(administrator.Host)
	if err != nil {
		return "", "", err
	}
	name := administrator.TLSClientConfig.ServerName
	if name == "" {
		name = api.Hostname()
	}
	network, err := management.Resource(siteNetworkGVR).Namespace("station-system").Get(ctx, "default", metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("SiteNetwork station-system/default: %w", err)
	}
	names, _, _ := unstructured.NestedSlice(network.Object, "spec", "names")
	for _, entry := range names {
		entry, _ := entry.(map[string]any)
		if address, _ := entry["address"].(string); entry["name"] == name {
			port := api.Port()
			if port == "" {
				port = "443"
			}
			return "https://" + net.JoinHostPort(address, port), name, nil
		}
	}
	return administrator.Host, administrator.TLSClientConfig.ServerName, nil
}

// MemberConfigs are the clusters of the management plane, each as its credential reaches it.
func MemberConfigs(ctx context.Context, management dynamic.Interface, core kubernetes.Interface) (map[string]*rest.Config, error) {
	clusters, err := management.Resource(clusterGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	configs := map[string]*rest.Config{}
	for _, cluster := range clusters.Items {
		name := cluster.GetName()
		secret, err := core.CoreV1().Secrets(CredentialNamespace).Get(ctx, CredentialName(name), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("cluster %s has no credential of the release (%s/%s): rlarkctl release credential --cluster %s", name, CredentialNamespace, CredentialName(name), name)
		}
		if err != nil {
			return nil, err
		}
		if configs[name], err = clientcmd.RESTConfigFromKubeConfig(secret.Data[credentialKey]); err != nil {
			return nil, fmt.Errorf("cluster %s: %w", name, err)
		}
	}
	return configs, nil
}

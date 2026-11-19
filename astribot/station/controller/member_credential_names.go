package controller

import (
	"context"
	"fmt"
	"net"
	"net/url"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// keepMemberCredentials keeps the address in every member credential at what
// the SiteNetwork declares for the name the credential verifies the API by
// (tls-server-name): each kubeconfig Secret of the credentials' namespaces
// (Station's, its node operations', the release's). A control plane that
// moves to another machine is one changed line of spec.names; their readers
// read the Secret for every client they make, so they follow without being
// told.
//
// The address and not the name is what the credential holds because its
// readers run in the management machine's network, whose /etc/hosts a
// container copies once when it starts.
func (r *NetworkReconciler) keepMemberCredentials(ctx context.Context, spec stationv1alpha1.SiteNetworkSpec) error {
	address := map[string]string{}
	for _, name := range spec.Names {
		address[name.Name] = name.Address
	}
	for _, namespace := range r.MemberCredentialNamespaces {
		var secrets corev1.SecretList
		if err := r.reader.List(ctx, &secrets, client.InNamespace(namespace)); err != nil {
			return fmt.Errorf("list the member credentials of %s: %w", namespace, err)
		}
		for i := range secrets.Items {
			secret := &secrets.Items[i]
			if _, isCredential := secret.Data[MemberKubeconfigSecretKey]; !isCredential {
				continue
			}
			config, err := clientcmd.Load(secret.Data[MemberKubeconfigSecretKey])
			if err != nil {
				return fmt.Errorf("the credential %s/%s: %w", namespace, secret.Name, err)
			}
			moved := false
			for _, api := range config.Clusters {
				to, named := address[api.TLSServerName]
				server, err := url.Parse(api.Server)
				if !named || err != nil || server.Hostname() == to {
					continue
				}
				server.Host = net.JoinHostPort(to, server.Port())
				api.Server, moved = server.String(), true
			}
			if !moved {
				continue
			}
			if secret.Data[MemberKubeconfigSecretKey], err = clientcmd.Write(*config); err != nil {
				return fmt.Errorf("the credential %s/%s: %w", namespace, secret.Name, err)
			}
			if err := r.client.Update(ctx, secret); err != nil {
				return fmt.Errorf("move the credential %s/%s: %w", namespace, secret.Name, err)
			}
		}
	}
	return nil
}

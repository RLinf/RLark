package kubernetes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"github.com/rlinf/rlark/apps/rlark/astribot/identity"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
)

// issueKCPIdentities gives the control plane's components their identities in kcp and writes each
// credential into the Secret its Pods mount. kcp's administrator credential (admin) is used for this
// only: kcp writes it anew at every start, so a component given it would be turned away after kcp's
// first restart.
func issueKCPIdentities(ctx context.Context, restConfig *rest.Config, clientset *kubernetes.Clientset, podName string, admin []byte) error {
	adminConfig, err := clientcmd.Load(admin)
	if err != nil {
		return fmt.Errorf("kcp admin kubeconfig: %w", err)
	}
	kcpConfig, err := clientcmd.NewNonInteractiveClientConfig(*adminConfig, identity.RootContext, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return fmt.Errorf("kcp admin kubeconfig, context %q: %w", identity.RootContext, err)
	}
	// The components reach kcp where its administrator kubeconfig says; this machine reaches it
	// through a port forward to the kcp Pod, checking kcp's certificate for that same name.
	server := kcpConfig.Host
	local, stop, err := forwardKCP(restConfig, clientset, podName)
	if err != nil {
		return err
	}
	defer close(stop)
	u, err := url.Parse(server)
	if err != nil {
		return err
	}
	kcpConfig.TLSClientConfig.ServerName = u.Hostname()
	u.Host = "127.0.0.1:" + strconv.Itoa(local)
	kcpConfig.Host = u.String()
	kcp, err := kubernetes.NewForConfig(kcpConfig)
	if err != nil {
		return err
	}

	store := identity.Secrets{Client: clientset, Namespace: constants.Namespace, Name: identity.RLarkSecretName}
	for _, c := range identity.RLark() {
		cred, err := identity.Issue(ctx, kcp, c, server, kcpConfig.CAData)
		if err != nil {
			return fmt.Errorf("identity of %s: %w", c.Name, err)
		}
		if err := store.Put(ctx, c, cred); err != nil {
			return err
		}
		log.GetLogger().Info("kcp identity issued", "component", c.Name, "token", cred.Secret, "in", store.Where(c))
	}
	return nil
}

// forwardKCP forwards a local port to kcp's port in its Pod until stop is closed.
func forwardKCP(restConfig *rest.Config, clientset *kubernetes.Clientset, podName string) (int, chan struct{}, error) {
	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return 0, nil, err
	}
	target := clientset.CoreV1().RESTClient().Post().Resource("pods").Namespace(constants.Namespace).Name(podName).SubResource("portforward").URL()
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, target)
	stop, ready := make(chan struct{}), make(chan struct{})
	forwarder, err := portforward.NewOnAddresses(dialer, []string{"127.0.0.1"}, []string{"0:6443"}, stop, ready, io.Discard, os.Stderr)
	if err != nil {
		return 0, nil, err
	}
	failed := make(chan error, 1)
	go func() { failed <- forwarder.ForwardPorts() }()
	select {
	case <-ready:
	case err := <-failed:
		return 0, nil, fmt.Errorf("port forward to %s: %w", podName, err)
	}
	ports, err := forwarder.GetPorts()
	if err != nil {
		close(stop)
		return 0, nil, err
	}
	return int(ports[0].Local), stop, nil
}

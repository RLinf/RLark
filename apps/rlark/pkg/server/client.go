package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rancher/remotedialer"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Client is a client.
type Client struct {
	baseURL         string
	tlsConfig       *tls.Config
	netDialer       remotedialer.Dialer
	transport       http.RoundTripper
	httpClient      *http.Client
	websocketDialer *websocket.Dialer
}

// NewClient creates a new Client with the given base URL, TLS configuration, and network dialer.
func NewClient(baseURL string, tlsConfig *tls.Config, netDialer remotedialer.Dialer) *Client {
	if netDialer == nil {
		netDialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		}
	}
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext:     netDialer,
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	websocketDialer := &websocket.Dialer{
		TLSClientConfig:  tlsConfig,
		NetDialContext:   netDialer,
		HandshakeTimeout: remotedialer.HandshakeTimeOut,
	}
	return &Client{
		baseURL:         baseURL,
		tlsConfig:       tlsConfig,
		netDialer:       netDialer,
		transport:       transport,
		httpClient:      httpClient,
		websocketDialer: websocketDialer,
	}
}

// NewClientFromConfig creates a new Client from the given ClientConfig.
func NewClientFromConfig(config ClientConfig) (*Client, error) {
	clientCert, err := tls.LoadX509KeyPair(config.ClientCertPath, config.ClientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}
	var caCertPool *x509.CertPool
	if config.CAPath != "" {
		caCertPool = x509.NewCertPool()
		caCertData, err := os.ReadFile(config.CAPath)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate: %w", err)
		}
		if ok := caCertPool.AppendCertsFromPEM(caCertData); !ok {
			return nil, fmt.Errorf("failed to append CA certificate")
		}
	}
	u, err := url.Parse(config.ServerAddress)
	if err != nil {
		return nil, fmt.Errorf("parse server address: %w", err)
	}
	dialerTarget := u.Host
	if config.ServerHostname != "" {
		if port := u.Port(); port != "" {
			u.Host = config.ServerHostname + ":" + port
		} else {
			u.Host = config.ServerHostname
		}
	}

	netDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, dialerTarget)
	}
	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{clientCert},
		RootCAs:            caCertPool,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: config.InsecureSkipTLSVerify,
	}

	return NewClient(u.String(), tlsConfig, netDialer), nil
}

// NewClientFromKubernetes creates a new Client by discovering the server's IP
// and retrieving the client certificate from Kubernetes secrets. It can only be
// used when the client is running inside the same network as the server and has
// access to the Kubernetes API.
func NewClientFromKubernetes(ctx context.Context, serverAddr string, kubeConfig configs.KubernetesClientConfig) (*Client, error) {
	restConfig, err := kubeConfig.BuildRestConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create kube client: %w", err)
	}
	namespace := kubeConfig.DefaultNamespace()

	if !strings.HasPrefix(serverAddr, "http") {
		serverAddr = "https://" + serverAddr
	}
	serverUrl, err := url.Parse(serverAddr)
	if err != nil {
		return nil, fmt.Errorf("parse server address: %w", err)
	}
	if serverUrl.Hostname() == "" {
		leases, err := kubeClient.CoordinationV1().Leases(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list leases in namespace %s: %w", namespace, err)
		}
		serverIPs := make([]string, 0)
		for _, item := range leases.Items {
			if strings.HasPrefix(item.Name, ServerPeerPrefix) {
				if item.Spec.HolderIdentity == nil || item.Spec.LeaseDurationSeconds == nil {
					continue
				}
				if time.Since(item.Spec.RenewTime.Time) > time.Duration(*item.Spec.LeaseDurationSeconds)*time.Second {
					continue
				}
				ip, _, _ := strings.Cut(*item.Spec.HolderIdentity, "/")
				if net.ParseIP(ip) != nil {
					serverIPs = append(serverIPs, ip)
				}
			}
		}
		if len(serverIPs) == 0 {
			return nil, fmt.Errorf("no server peers found in namespace %s", namespace)
		}
		serverHost := serverIPs[rand.Intn(len(serverIPs))]
		serverUrl.Host = net.JoinHostPort(serverHost, serverUrl.Port())
	}

	clientSecret, err := kubeClient.CoreV1().Secrets(namespace).Get(ctx, common.AdminCertSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", namespace, common.AdminCertSecretName, err)
	}
	certData, err := cert.LoadData(clientSecret.Data["client.crt"], clientSecret.Data["client.key"])
	if err != nil {
		return nil, fmt.Errorf("load cert data from secret: %w", err)
	}
	clientCert, err := tls.X509KeyPair(certData.CertPEM, certData.KeyPEM)
	if err != nil {
		return nil, fmt.Errorf("create client certificate: %w", err)
	}

	var caCertPool *x509.CertPool
	caSecret, err := kubeClient.CoreV1().Secrets(namespace).Get(ctx, common.AdminCertSecretName, metav1.GetOptions{})
	if err == nil {
		caCertPool = x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caSecret.Data["ca.crt"])
	}

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{clientCert},
		RootCAs:            caCertPool,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}
	return NewClient(serverUrl.String(), tlsConfig, nil), nil
}

// BuildURL 构建完整的请求 URL.
func (c *Client) BuildURL(parts ...string) string {
	u, _ := url.Parse(c.baseURL)
	u.Path = path.Join(u.Path, path.Join(parts...))
	return u.String()
}

// BuildURLWithQuery 构建带查询参数的请求 URL.
func (c *Client) BuildURLWithQuery(query url.Values, parts ...string) string {
	u, _ := url.Parse(c.baseURL)
	u.Path = path.Join(u.Path, path.Join(parts...))
	if query != nil {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

// DoRequest 执行 HTTP 请求.
func (c *Client) DoRequest(ctx context.Context, method, rawURL string, body io.Reader, requestOptions ...func(*http.Request)) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, option := range requestOptions {
		option(req)
	}
	return c.httpClient.Do(req)
}

// DoRequestWithObject performs a request and decodes the response.
func (c *Client) DoRequestWithObject(ctx context.Context, method, rawURL string, obj any, requestOptions ...func(*http.Request)) (*http.Response, error) {
	body, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object: %w", err)
	}
	return c.DoRequest(ctx, method, rawURL, bytes.NewReader(body), requestOptions...)
}

// DialWebsocket dials the websocket.
func (c *Client) DialWebsocket(ctx context.Context, header http.Header) (*websocket.Conn, *http.Response, error) {
	urlStr := c.BuildURL("api", "connect")
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, nil, fmt.Errorf("parse websocket URL: %w", err)
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	return c.websocketDialer.DialContext(ctx, u.String(), header)
}

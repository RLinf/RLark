package server

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
)

// Config holds the server configuration parameters.
type Config struct {
	// HTTPS Port to listen on.
	HTTPSPort int
	// SSH Port to listen on.
	SSHPort int

	// UnsafeHTTPPort is the port to listen on for unsafe HTTP connections.
	UnsafeHTTPPort int

	// AutoSignTLSCACert indicates whether to automatically sign a TLS CA certificate if it does not exist in Kubernetes.
	AutoSignTLSCACert bool

	// TLSDomains is the list of domain names to include in the TLS certificate (e.g., "rlark.example.com", "localhost").
	TLSDomains []string

	// Kubernetes client configuration.
	KubeClientConfig configs.KubernetesClientConfig

	// DBConfigPath is the file path to the database configuration (e.g., YAML or JSON).
	DBConfigPath string

	// PeerServiceName is the DNS name of the peer service for clustering (optional).
	PeerServiceName string
	// Peers is a list of peer server addresses for clustering (optional).
	Peers []string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		HTTPSPort:         8443,
		SSHPort:           2222,
		UnsafeHTTPPort:    8888,
		AutoSignTLSCACert: false,
		TLSDomains:        []string{"localhost"},
		KubeClientConfig:  configs.DefaultKubernetesClientConfig(),
		DBConfigPath:      "",
		PeerServiceName:   "",
		Peers:             []string{},
	}
}

// SetupFlags defines command-line flags for the server configuration.
func (c *Config) SetupFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.HTTPSPort, "https-port", c.HTTPSPort, "HTTPS port to listen on")
	fs.IntVar(&c.SSHPort, "ssh-port", c.SSHPort, "SSH port to listen on")
	fs.IntVar(&c.UnsafeHTTPPort, "unsafe-http-port", c.UnsafeHTTPPort, "Unsafe HTTP port to listen on")
	fs.BoolVar(&c.AutoSignTLSCACert, "auto-sign-tls-ca-cert", c.AutoSignTLSCACert, "Automatically sign a TLS CA certificate if it does not exist in Kubernetes")
	fs.StringSliceVar(&c.TLSDomains, "tls-domains", c.TLSDomains, "Comma-separated list of domain names to include in the TLS certificate (e.g., \"localhost\")")
	fs.StringVar(&c.DBConfigPath, "db-config", c.DBConfigPath, "Path to database configuration file")
	fs.StringVar(&c.PeerServiceName, "peer-service", c.PeerServiceName, "DNS name of the peer service for clustering")
	fs.StringSliceVar(&c.Peers, "peers", c.Peers, "Comma-separated list of peer server addresses for clustering")

	c.KubeClientConfig.SetupFlags(fs)
}

// ClientConfig holds configuration options.
type ClientConfig struct {
	ServerAddress         string
	ServerHostname        string
	ClientCertPath        string
	ClientKeyPath         string
	CAPath                string
	InsecureSkipTLSVerify bool

	ServerNamespace string // auto load from client cert
}

// DefaultClientConfig returns the default clientConfig.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		ServerAddress:         "https://localhost:8443",
		ServerHostname:        "",
		ClientCertPath:        "",
		ClientKeyPath:         "",
		CAPath:                "",
		InsecureSkipTLSVerify: false,
	}
}

// SetupFlags sets the upFlags.
func (c *ClientConfig) SetupFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.ServerAddress, "server-address", c.ServerAddress, "Address of the server to connect to")
	fs.StringVar(&c.ServerHostname, "server-hostname", c.ServerHostname, "Expected hostname of the server for TLS verification (optional)")
	fs.StringVar(&c.ClientCertPath, "client-cert", c.ClientCertPath, "Path to the client TLS certificate")
	fs.StringVar(&c.ClientKeyPath, "client-key", c.ClientKeyPath, "Path to the client TLS private key")
	fs.StringVar(&c.CAPath, "ca-cert", c.CAPath, "Path to the CA certificate for verifying the server")
	fs.BoolVar(&c.InsecureSkipTLSVerify, "insecure-skip-tls-verify", c.InsecureSkipTLSVerify, "Skip TLS certificate verification (not recommended)")
}

func (c *ClientConfig) loadNamespace() error {
	if c.ServerNamespace != "" {
		return nil
	}

	certPEM, err := os.ReadFile(c.ClientCertPath)
	if err != nil {
		return fmt.Errorf("read client certificate: %w", err)
	}
	keyPEM, err := os.ReadFile(c.ClientKeyPath)
	if err != nil {
		return fmt.Errorf("read client key: %w", err)
	}
	clientCert, err := cert.LoadData(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("load client certificate: %w", err)
	}
	certMeta, _ := cert.GetX509CertMeta(clientCert.Cert)
	if certMeta == nil || certMeta[apis.MetaNamespace] == "" {
		return fmt.Errorf("invalid client certificate: missing namespace")
	}
	c.ServerNamespace = certMeta[apis.MetaNamespace]
	return nil
}

// BuildKubeAPIConfig builds the kubeAPIConfig.
func (c *ClientConfig) BuildKubeAPIConfig() (*api.Config, error) {
	if err := c.loadNamespace(); err != nil {
		return nil, err
	}

	config := api.NewConfig()

	cluster := api.NewCluster()
	cluster.Server = fmt.Sprintf("%s/api/kubernetes", c.ServerAddress)
	if c.InsecureSkipTLSVerify {
		cluster.InsecureSkipTLSVerify = true
	} else if c.CAPath != "" {
		cluster.CertificateAuthority = c.CAPath
	}
	config.Clusters["management"] = cluster

	user := api.NewAuthInfo()
	user.ClientCertificate = c.ClientCertPath
	user.ClientKey = c.ClientKeyPath
	config.AuthInfos["agent"] = user

	context := api.NewContext()
	context.Cluster = "management"
	context.AuthInfo = "agent"
	context.Namespace = c.ServerNamespace
	config.Contexts["default"] = context
	config.CurrentContext = "default"
	return config, nil
}

// BuildRestConfig builds the restConfig.
func (c *ClientConfig) BuildRestConfig() (*rest.Config, error) {
	return clientcmd.BuildConfigFromKubeconfigGetter("", c.BuildKubeAPIConfig)
}

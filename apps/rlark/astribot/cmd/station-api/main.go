package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rlinf/rlark/apps/rlark/astribot/stationidentity"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func main() {
	config := configs.DefaultKubernetesClientConfig()
	var address, credentialsNamespace, certFile, keyFile, gatewayCredentialFile string
	var bootstrap stationidentity.Bootstrap
	var bootstrapCAFile string
	cmd := &cobra.Command{
		Use:   "station-api",
		Short: "Run the Station identity and observation API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			return run(ctx, config, address, credentialsNamespace, certFile, keyFile, gatewayCredentialFile, &bootstrap, bootstrapCAFile)
		},
	}
	config.SetupFlags(cmd.Flags())
	cmd.Flags().StringVar(&address, "addr", ":8443", "HTTPS listener address")
	cmd.Flags().StringVar(&credentialsNamespace, "member-credentials-namespace", "station-member-credentials", "namespace containing registered member kubeconfig Secrets")
	cmd.Flags().StringVar(&certFile, "tls-cert-file", os.Getenv("STATION_API_TLS_CERT_FILE"), "TLS certificate path mounted into the container")
	cmd.Flags().StringVar(&keyFile, "tls-key-file", os.Getenv("STATION_API_TLS_KEY_FILE"), "TLS private key path mounted into the container")
	cmd.Flags().StringVar(&gatewayCredentialFile, "gateway-credential-file", "", "Optional dedicated Gateway network-summary credential file")
	cmd.Flags().StringVar(&bootstrapCAFile, "bootstrap-ca-file", "", "CA certificate machines verify this API with; enables node admission by one-time code")
	cmd.Flags().StringVar(&bootstrap.BundleFile, "bootstrap-bundle-file", "", "Installer served to admitted machines, as the tar it is unpacked from on the machine")
	cmd.Flags().StringVar(&bootstrap.AddonsDir, "bootstrap-addons-dir", "/var/lib/rlark/addons", "Directory of what the addons publish; a NodeClass's uplink.setup names a file in it")
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "station-api failed")
		os.Exit(1)
	}
}

func run(ctx context.Context, kubeConfig configs.KubernetesClientConfig, address, credentialsNamespace, certFile, keyFile, gatewayCredentialFile string, bootstrap *stationidentity.Bootstrap, bootstrapCAFile string) error {
	if address == "" || credentialsNamespace == "" || certFile == "" || keyFile == "" {
		return errors.New("listener, credential namespace and TLS certificate paths are required")
	}
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		return errors.New("load Station API TLS certificate")
	}

	config, err := kubeConfig.BuildRestConfig()
	if err != nil {
		return errors.New("load management API credentials")
	}
	config.Timeout = 10 * time.Second
	management, err := dynamic.NewForConfig(config)
	if err != nil {
		return errors.New("create management API client")
	}
	core, err := kubernetes.NewForConfig(config)
	if err != nil {
		return errors.New("create management core API client")
	}
	registry := stationidentity.NewKubernetesClusterRegistry(management, core, credentialsNamespace, nil)
	enrolled := stationidentity.EnrolledKeys(management)
	documents, err := stationidentity.NewNetworkDocuments(management, enrolled)
	if err != nil {
		return errors.New("make the secret of the network document challenges")
	}
	api := &stationidentity.API{
		Registry:           registry,
		Documents:          documents,
		ObservationLimiter: stationidentity.NewTokenBucketObservationLimiter(1, 5, 10000, 10*time.Minute),
		Observations: &stationidentity.KubernetesObservationSink{
			Client: management, Namespaces: stationidentity.KubernetesClusterNamespaceResolver{Client: management}, Enrolled: enrolled,
			Admitted: documents.Admitted, Declared: &stationidentity.DeclaredKeys{Client: management},
		},
		Enrollments: &stationidentity.KubernetesNetworkEnroller{
			Client: management, Namespaces: stationidentity.KubernetesClusterNamespaceResolver{Client: management},
		},
		EnrollmentLimiter: stationidentity.NewTokenBucketObservationLimiter(1, 5, 10000, 10*time.Minute),
		Ready: func(readyCtx context.Context) error {
			_, err := management.Resource(rlarkClusterGVR).List(readyCtx, metav1.ListOptions{Limit: 1})
			return err
		},
	}
	if bootstrapCAFile != "" {
		ca, err := os.ReadFile(bootstrapCAFile)
		if err != nil || bootstrap.BundleFile == "" {
			return errors.New("node admission needs a readable CA file and the installer bundle")
		}
		bootstrap.CA, bootstrap.Management, bootstrap.Member = ca, management, registry.MemberClient
		api.Bootstrap = bootstrap
	}
	if gatewayCredentialFile != "" {
		api.NetworkSummary = &stationidentity.NodeNetworkSummaryReader{Registry: registry, Client: management, Namespaces: stationidentity.KubernetesClusterNamespaceResolver{Client: management}}
		api.GatewayCredential = func() ([]byte, error) { return readGatewayCredential(gatewayCredentialFile) }
	}
	server := &http.Server{
		Addr: address, Handler: api,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return errors.New("bind Station API listener")
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ServeTLS(listener, certFile, keyFile) }()
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("Station API listener stopped")
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return errors.New("shut down Station API listener")
		}
		return nil
	}
}

func readGatewayCredential(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || len(token) > 4096 || strings.ContainsAny(token, " \t\r\n") {
		return nil, errors.New("invalid gateway credential")
	}
	return []byte(token), nil
}

var rlarkClusterGVR = schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: "clusters"}

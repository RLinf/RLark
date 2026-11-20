package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	"github.com/rlinf/rlark/astribot/station/controller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// builtInAddonsDirectory is where this program's image holds the built-in addons (apps/rlark/pkg/addons/catalog).
const builtInAddonsDirectory = "/usr/share/rlark/addons"

func main() {
	var kubeconfig, credentialsNamespace, nodeOperationCredentialsNamespace, releaseCredentialsNamespace, metricsAddress, probeAddress, networkNamespace, networkName string
	var nodeOperations bool
	var installerBundle, addonsDirectory, stationAPIBootstrapURL, stationAPICAFile, secretsNamespace string
	flag.StringVar(&kubeconfig, "management-kubeconfig", os.Getenv("KUBECONFIG"), "management API kubeconfig; omit to use in-cluster credentials")
	flag.StringVar(&credentialsNamespace, "member-credentials-namespace", "station-system", "namespace containing per-member kubeconfig Secrets")
	flag.StringVar(&nodeOperationCredentialsNamespace, "node-operation-member-credentials-namespace", "station-operation-member-credentials", "namespace containing least-privilege per-member kubeconfigs for node operations")
	flag.StringVar(&releaseCredentialsNamespace, "release-member-credentials-namespace", "rlark-release-member-credentials", "namespace of the release's per-member kubeconfigs (rlarkctl release), whose addresses follow the SiteNetwork's names as Station's do")
	flag.StringVar(&metricsAddress, "metrics-bind-address", ":8080", "Prometheus metrics bind address")
	flag.StringVar(&probeAddress, "health-probe-bind-address", ":8081", "health and readiness probe bind address")
	flag.BoolVar(&nodeOperations, "enable-node-operations", false, "Enable experimental node operations; requires NodeOperation CRD and dedicated permissions")
	flag.StringVar(&networkNamespace, "site-network-namespace", "station-system", "namespace of the SiteNetwork that answers NetworkEnrollments")
	flag.StringVar(&networkName, "site-network-name", "default", "name of the SiteNetwork that answers NetworkEnrollments")
	flag.StringVar(&installerBundle, "installer-bundle", "", "the node installer as a tar (bootstrap/build.sh): with it, machines of a NodeClass are found on their sites' networks and joined from the list of Machines; needs --enable-node-operations")
	flag.StringVar(&addonsDirectory, "addons-dir", "/var/lib/rlark/addons", "directory of what the site's addons publish; a NodeClass's uplink.setup names a file of a built-in addon (this image carries them) or one in it")
	flag.StringVar(&secretsNamespace, "rlark-secrets-namespace", "default", "namespace of the management plane where RLark keeps its secrets; a NodeClass names the passwords of its machines there")
	flag.StringVar(&stationAPIBootstrapURL, "station-api-bootstrap-url", "", "the URL a machine reaches Station API at before it has a tunnel")
	flag.StringVar(&stationAPICAFile, "station-api-ca-file", "", "the CA certificate a machine verifies Station API with")
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))
	must := func(err error, what string) {
		if err != nil {
			ctrl.Log.Error(err, what)
			os.Exit(1)
		}
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	must(err, "load management Kubernetes credentials")
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(stationv1alpha1.AddToScheme(scheme))
	manager, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                  scheme,
		Metrics:                 metricsserver.Options{BindAddress: metricsAddress},
		HealthProbeBindAddress:  probeAddress,
		LeaderElection:          true,
		LeaderElectionID:        "station-controller.station.rlark.io",
		LeaderElectionNamespace: "station-system",
	})
	must(err, "create Station controller manager")
	reader, writer := manager.GetAPIReader(), manager.GetClient()
	network := types.NamespacedName{Namespace: networkNamespace, Name: networkName}
	registration := controller.ManagementClusterResolver{Reader: reader}
	readiness := controller.ManagementNodeReadiness{Reader: reader}
	targetReader := controller.NewKubeconfigNodeReader(controller.SecretKubeconfigSource{Reader: reader, Namespace: credentialsNamespace})
	must(controller.NewNodeReconciler(writer, reader, readiness, controller.DefaultOfflineAfter, time.Now).SetupWithManager(manager), "register node reconciler")
	// The Kubernetes version of each member and of its nodes' kubelets, on its RLark Nodes (the console's 集群管理).
	must(manager.Add(&controller.KubernetesVersionReporter{Reader: reader, Writer: writer, Members: targetReader, Every: time.Minute}), "register Kubernetes version reporter")
	memberCredentialNamespaces := []string{credentialsNamespace, releaseCredentialsNamespace}
	if nodeOperations {
		memberCredentialNamespaces = append(memberCredentialNamespaces, nodeOperationCredentialsNamespace)
		members := controller.NewKubeconfigNodeReader(controller.SecretKubeconfigSource{Reader: reader, Namespace: nodeOperationCredentialsNamespace})
		access := controller.NodeClassAccess{Reader: reader, Namespace: secretsNamespace}
		peerCleaner := controller.StationTunnelRelease{Reader: reader, Writer: writer, Members: members, Network: network}
		executor := &controller.NodeLifecycleExecutor{
			Members:         members,
			ResetDriver:     controller.InClusterNodeReset{Members: members},
			Power:           controller.RobotPowerOff{Members: members, Management: writer},
			Management:      writer,
			PeerCleaner:     peerCleaner,
			JoinedReadiness: controller.StationJoinedReadiness{Reader: reader, Nodes: readiness},
			RemovalReady:    peerCleaner.Check,
		}
		operations := controller.NewNodeOperationReconciler(writer, reader, registration, executor)
		if installerBundle != "" {
			executor.Installer = &controller.InstallerJoinDriver{Reader: reader, Writer: writer, Access: access, Members: members,
				BundleFile: installerBundle, AddonsDirs: []string{builtInAddonsDirectory, addonsDirectory}, StationAPI: stationAPIBootstrapURL, CAFile: stationAPICAFile}
			operations.Concurrency = 10
			must((&controller.MachineReconciler{Client: writer, Reader: reader, Inspector: controller.SSHMachineInspector{Access: access},
				Clusters: registration, Namespace: networkNamespace}).SetupWithManager(manager), "register Machine reconciler")
		}
		must(operations.SetupWithManager(manager), "register node operation reconciler")
	}
	must((&controller.NodeClassReconciler{Client: writer, Reader: reader}).SetupWithManager(manager), "register NodeClass reconciler")
	networkReconciler := controller.NewNetworkReconciler(writer, reader, network, time.Now)
	networkReconciler.MemberCredentialNamespaces = memberCredentialNamespaces
	must(networkReconciler.SetupWithManager(manager), "register SiteNetwork reconciler")
	must(manager.AddHealthzCheck("healthz", healthz.Ping), "register health check")
	must(manager.AddReadyzCheck("readyz", healthz.Ping), "register readiness check")
	must(manager.AddReadyzCheck("cache-sync", func(request *http.Request) error {
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		if !manager.GetCache().WaitForCacheSync(ctx) {
			return errors.New("controller caches are not synchronized")
		}
		return nil
	}), "register cache readiness check")
	must(manager.Start(ctrl.SetupSignalHandler()), "run Station controller manager")
}

package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spf13/pflag"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/utils/lru"
)

// astribotState is what our part of the Gateway holds; Gateway embeds it.
type astribotState struct {
	stationClient    dynamic.Interface
	networkSummary   nodeNetworkSummaryClient
	stationBootstrap stationBootstrap

	// registryHTTPClient overrides the registry browsing client (tests).
	registryHTTPClient *http.Client

	// registryImages caches image metadata (registryImageInfo) by registry, repository and manifest digest: a
	// digest is a content address, so an entry never goes stale; tags are resolved every time.
	registryImages *lru.Cache

	// provisions are the clusters being made, nodeJoins the machines being joined over SSH; provisionDial logs in to
	// a machine of either.
	provisions    stagedRuns
	nodeJoins     stagedRuns
	provisionDial provisionDialFunc
	cameraStreams cameraStreamLimiter
}

// AstribotConfig is our part of the Gateway's configuration; Config embeds it. Every field is required.
type AstribotConfig struct {
	// RegistryBrowseAllowedHosts limits the registry origins the Gateway may query.
	RegistryBrowseAllowedHosts   []string
	StationAPIAddress            string
	StationAPICAFile             string
	StationGatewayCredentialFile string
	// StationAPIBootstrapURL is the HTTPS origin of Station API as a machine
	// that joins reaches it (the api-url station-agent has), for join codes.
	// StationAPIAddress is where the Gateway itself reaches Station API, which
	// may be another address.
	StationAPIBootstrapURL string
}

func (c *AstribotConfig) setupFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(&c.RegistryBrowseAllowedHosts, "registry-browse-allowed-hosts", nil, "Exact host[:port] values or IP CIDRs permitted for read-only HTTPS registry browsing")
	fs.StringVar(&c.StationAPIAddress, "station-api-address", "", "Station API HTTPS origin, as the Gateway reaches it")
	fs.StringVar(&c.StationAPICAFile, "station-api-ca-file", "", "Station API CA bundle")
	fs.StringVar(&c.StationGatewayCredentialFile, "station-gateway-credential-file", "", "Dedicated Station API network summary credential file")
	fs.StringVar(&c.StationAPIBootstrapURL, "station-api-bootstrap-url", "", "Station API HTTPS origin as machines that join reach it")
}

// setupAstribot is NewGateway's part for our state.
func (g *Gateway) setupAstribot() {
	g.registryImages = lru.New(registryImageCacheEntries)
	g.provisionDial = dialProvisionMachine
	g.cameraStreams = cameraStreamLimiter{perUser: make(map[string]int)}
}

// initAstribot is init's part for our clients, right after the Kubernetes clients.
func (g *Gateway) initAstribot(restConfig *rest.Config) error {
	var err error
	g.stationClient, err = dynamic.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create dynamic Kubernetes client: %w", err)
	}
	if len(g.config.RegistryBrowseAllowedHosts) == 0 {
		return errors.New("--registry-browse-allowed-hosts is required")
	}
	g.networkSummary, err = newStationNetworkClient(g.config.StationAPIAddress, g.config.StationAPICAFile, g.config.StationGatewayCredentialFile)
	if err != nil {
		return fmt.Errorf("configure Station network summaries: %w", err)
	}
	g.stationBootstrap, err = newStationBootstrapClient(g.config.StationAPIBootstrapURL, g.config.StationAPIAddress, g.config.StationAPICAFile)
	if err != nil {
		return fmt.Errorf("configure join codes: %w", err)
	}
	return nil
}

// startAstribot is Run's part once init is done, before the HTTP server starts.
func (g *Gateway) startAstribot(ctx context.Context) error {
	g.rerenderStorageClasses(ctx)
	if err := g.interruptProvisions(ctx); err != nil {
		return err
	}
	if err := g.interruptNodeJoins(ctx); err != nil {
		return err
	}
	go g.sweepRobotClaims(ctx)
	return nil
}

// registerAstribotRoutes registers our routes under api (/api/v1, behind requireJWT), in groups with the
// middleware RegisterRoutes gives the official groups of the same paths.
func (g *Gateway) registerAstribotRoutes(api *gin.RouterGroup) {
	adminAPI := api.Group("", requireAdmin())
	rlinfv1alpha1 := api.Group("/rlinf.io/v1alpha1")
	adminRlinfv1alpha1 := rlinfv1alpha1.Group("", requireAdmin())
	jobSecrets := adminAPI.Group("/secrets")
	{
		jobSecrets.GET("", g.listJobSecrets)
		jobSecrets.POST("", g.createJobSecret)
		jobSecrets.PUT("/:name", g.updateJobSecret)
		jobSecrets.DELETE("/:name", g.deleteJobSecret)
	}
	serviceAccounts := adminAPI.Group("/service-accounts")
	{
		serviceAccounts.GET("", g.listServiceAccounts)
		serviceAccounts.POST("", g.createServiceAccount)
		serviceAccounts.POST("/:name/token", g.rotateServiceAccountToken)
		serviceAccounts.DELETE("/:name", g.deleteServiceAccount)
	}
	station := adminAPI.Group("/station/stations")
	{
		station.GET("", g.handleListStations)
		station.GET("/:name", g.handleGetStation)
		station.GET("/:name/running-images", g.handleStationRunningImages)
		station.GET("/:name/body-state", g.handleStationBodyState)
		station.DELETE("/:name", g.handleDeleteStation)
	}
	adminAPI.GET("/station/summary", g.handleStationSummary)
	stationNodeOperations := adminAPI.Group("/station/node-operations")
	{
		stationNodeOperations.POST("", g.handleCreateNodeOperation)
		stationNodeOperations.GET("", g.handleListNodeOperations)
		stationNodeOperations.GET("/:name", g.handleGetNodeOperation)
	}
	adminAPI.GET("/station/site-network", g.handleGetSiteNetwork)
	adminAPI.PUT("/station/site-network/sites/:site/discovery", g.handleUpdateSiteDiscovery)
	adminAPI.GET("/station/site-network/sites/:site/scan", g.handleDiscoveryScan)
	adminAPI.POST("/station/site-network/sites/:site/scan", g.handleDiscoveryScan)
	adminAPI.GET("/station/robot-models", g.handleListRobotModels)
	adminAPI.DELETE("/station/robot-models/:name", g.handleDeleteRobotModel)
	nodeAdmissions := adminAPI.Group("/station/node-admissions")
	{
		nodeAdmissions.GET("", g.handleListNodeAdmissions)
		nodeAdmissions.POST("", g.handleIssueNodeAdmission)
		nodeAdmissions.POST("/:name/reissue", g.handleReissueNodeAdmission)
		nodeAdmissions.POST("/:name/revoke", g.handleRevokeNodeAdmission)
	}
	nodeJoins := adminAPI.Group("/station/node-joins")
	{
		nodeJoins.GET("", g.handleListNodeJoins)
		nodeJoins.POST("", g.handleCreateNodeJoin)
		nodeJoins.GET("/:name", g.handleGetNodeJoin)
		nodeJoins.POST("/:name/retry", g.handleRetryNodeJoin)
		nodeJoins.POST("/:name/clear", g.handleClearNodeJoin)
	}
	clusterProvisions := adminAPI.Group("/cluster-provisions")
	{
		clusterProvisions.GET("", g.handleListClusterProvisions)
		clusterProvisions.POST("", g.handleCreateClusterProvision)
		clusterProvisions.GET("/:name", g.handleGetClusterProvision)
		clusterProvisions.POST("/:name/retry", g.handleRetryClusterProvision)
		clusterProvisions.POST("/:name/clear", g.handleClearClusterProvision)
	}
	adminAPI.GET("/station/machines", g.handleListMachines)
	adminAPI.POST("/station/machines/admit", g.handleAdmitMachines)
	adminAPI.GET("/station/node-classes/:name/access", g.handleGetNodeClassAccess)
	adminAPI.PUT("/station/node-classes/:name/access", g.handleSetNodeClassAccess)
	api.GET("/node-networks/:name", g.handleNodeNetwork)
	api.GET("/site-nodes", g.handleListSiteNodes)
	api.GET("/site-nodes/:name", g.handleGetSiteNode)

	adminClusters := adminAPI.Group("/clusters")
	{
		adminClusters.PUT("/:cluster_id/node-agents", g.setClusterNodeAgents)
		adminClusters.PUT("/:cluster_id/display-name", g.setClusterDisplayName)
		adminClusters.PUT("/:cluster_id/cluster-type", g.setClusterType)
	}

	g.registerRobotRoutes(rlinfv1alpha1.Group("/nodes"))
	adminNodes := adminRlinfv1alpha1.Group("/nodes")
	{
		adminNodes.PUT("/:name/display-name", g.handleSetNodeDisplayName)
	}

	imageRegistries := adminAPI.Group("/image-registries")
	{
		imageRegistries.GET("/:id/repositories", g.handleRegistryRepositories)
		imageRegistries.GET("/:id/tags", g.handleRegistryTags)
		imageRegistries.GET("/:id/tag-details", g.handleRegistryTagDetails)
		imageRegistries.GET("/:id/manifest", g.handleRegistryManifest)
	}
	api.GET("/image-registries/:id/password", g.auditRegistryPasswordAccess(), requireAdmin(), g.handleGetImageRegistryPassword)
}

package gateway

import "github.com/gin-gonic/gin"

// RegisterRoutes registers the routes.
func (g *Gateway) RegisterRoutes(r gin.IRouter) {
	auth := r.Group("/api/v1/auth")
	{
		auth.POST("/login", g.handleLogin)
	}

	api := r.Group("/api/v1", g.requireJWT())
	rlinfv1alpha1 := api.Group("/rlinf.io/v1alpha1")
	adminAPI := api.Group("", requireAdmin())
	adminRlinfv1alpha1 := rlinfv1alpha1.Group("", requireAdmin())

	api.GET("/api-reference", g.handleAPIReference)

	// Clusters API
	clusters := api.Group("/clusters")
	{
		clusters.GET("", g.listClusters)
		clusters.GET("/:cluster_id", g.getCluster)
	}

	nodes := rlinfv1alpha1.Group("/nodes")
	{
		nodes.GET("", g.rlinfv1alpha1ListNodes)
		nodes.GET("/:name", g.rlinfv1alpha1GetNode)
	}
	adminNodes := adminRlinfv1alpha1.Group("/nodes")
	{
		adminNodes.POST("", g.rlinfv1alpha1CreateNode)
		adminNodes.PUT("/:name", g.rlinfv1alpha1UpdateNode)
		adminNodes.PATCH("/:name", g.rlinfv1alpha1PatchNode)
		adminNodes.DELETE("/:name", g.rlinfv1alpha1DeleteNode)
	}

	workflows := rlinfv1alpha1.Group("/workflows")
	{
		workflows.GET("", g.rlinfv1alpha1ListWorkflows)
		workflows.POST("", g.rlinfv1alpha1CreateWorkflow)
		workflows.GET("/:name", g.rlinfv1alpha1GetWorkflow)
		workflows.PUT("/:name", g.rlinfv1alpha1UpdateWorkflow)
		workflows.PATCH("/:name", g.rlinfv1alpha1PatchWorkflow)
		workflows.DELETE("/:name", g.rlinfv1alpha1DeleteWorkflow)
	}

	jobs := rlinfv1alpha1.Group("/jobs")
	{
		jobs.GET("", g.rlinfv1alpha1ListJobs)
		jobs.GET("/tags", g.rlinfv1alpha1ListJobTags)
		jobs.POST("", g.rlinfv1alpha1CreateJob)
		jobs.GET("/:name", g.rlinfv1alpha1GetJob)
		jobs.PUT("/:name", g.rlinfv1alpha1UpdateJob)
		jobs.PATCH("/:name", g.rlinfv1alpha1PatchJob)
		jobs.DELETE("/:name", g.rlinfv1alpha1DeleteJob)
		jobs.GET("/:name/logs", g.rlinfv1alpha1JobLogs)
		jobs.GET("/:name/logs/label-values", g.rlinfv1alpha1JobLogLabelValues)
		jobs.GET("/:name/metrics", g.rlinfv1alpha1JobMetrics)
	}

	tasks := rlinfv1alpha1.Group("/tasks")
	{
		tasks.GET("", g.rlinfv1alpha1ListTasksWithProxy)
		tasks.POST("", g.rlinfv1alpha1CreateTask)
		tasks.GET("/:name", g.rlinfv1alpha1GetTaskWithProxy)
		tasks.PUT("/:name", g.rlinfv1alpha1UpdateTask)
		tasks.PATCH("/:name", g.rlinfv1alpha1PatchTask)
		tasks.DELETE("/:name", g.rlinfv1alpha1DeleteTask)
		tasks.Any("/:name/tensorboard/*path", g.handleTaskTensorBoardProxy)
	}

	pods := rlinfv1alpha1.Group("/pods")
	{
		pods.GET("", g.rlinfv1alpha1ListPods)
		pods.GET("/:name/events", g.handlePodEvents)
		pods.GET("/:name", g.rlinfv1alpha1GetPod)
		pods.PATCH("/:name", g.rlinfv1alpha1PatchPod)
		pods.GET("/:name/terminal", g.handlePodTerminal)
	}

	domains := rlinfv1alpha1.Group("/domains")
	{
		domains.GET("", g.rlinfv1alpha1ListDomains)
		domains.GET("/:name", g.rlinfv1alpha1GetDomain)
	}
	adminDomains := adminRlinfv1alpha1.Group("/domains")
	{
		adminDomains.POST("", g.rlinfv1alpha1CreateDomain)
		adminDomains.PUT("/:name", g.rlinfv1alpha1UpdateDomain)
		adminDomains.PATCH("/:name", g.rlinfv1alpha1PatchDomain)
		adminDomains.DELETE("/:name", g.rlinfv1alpha1DeleteDomain)
	}

	certificates := adminAPI.Group("/certificates")
	{
		certificates.GET("/agent", g.handleListAgentCerts)
		certificates.GET("/agent/:cluster_id", g.handleGetAgentCert)
		certificates.POST("/agent", g.handleSignAgentCert)
		certificates.POST("/revoke", g.handleRevokeCertificate)
	}

	sshUserKeys := api.Group("/ssh-user-keys")
	{
		sshUserKeys.GET("", g.handleListSSHUserKeys)
		sshUserKeys.POST("", g.handleCreateSSHUserKey)
		sshUserKeys.DELETE("/:id", g.handleDeleteSSHUserKey)
	}

	images := api.Group("/images")
	{
		images.GET("", g.listImages)
	}

	// Image Registry APIs
	imageRegistries := adminAPI.Group("/image-registries")
	{
		imageRegistries.GET("", g.handleListImageRegistries)
		imageRegistries.POST("", g.handleCreateImageRegistry)
		imageRegistries.GET("/:id", g.handleGetImageRegistry)
		imageRegistries.PUT("/:id", g.handleUpdateImageRegistry)
		imageRegistries.DELETE("/:id", g.handleDeleteImageRegistry)
	}

	// System Config APIs
	systemConfig := api.Group("/system-config")
	{
		systemConfig.GET("", g.handleGetSystemConfig)
	}
	adminSystemConfig := adminAPI.Group("/system-config")
	{
		adminSystemConfig.PUT("", g.handleUpdateSystemConfig)
	}

	// Storage APIs
	storage := api.Group("/storage")
	{
		storage.GET("/storageclass", g.listStorageClass)

		scFiles := storage.Group("/storageclass/:name/:cluster")
		{
			scFiles.GET("/list", g.listStorageClassFiles)
			scFiles.POST("/upload", g.uploadStorageClassFile)
			scFiles.GET("/object/*key", g.getStorageClassObject)
			scFiles.DELETE("/object/*key", g.deleteStorageClassObject)
		}
	}
	adminStorage := adminAPI.Group("/storage")
	{
		adminStorage.POST("/storageclass", g.createStorageClass)
		adminStorage.PUT("/storageclass/:name", g.updateStorageClass)
		adminStorage.DELETE("/storageclass/:name", g.deleteStorageClass)
		adminStorage.GET("/storageclass/provider", g.listProvider)
	}

	// Addon Catalog APIs
	addons := adminAPI.Group("/addons")
	{
		addons.GET("", g.listAddonCatalog)
		addons.GET("/:name", g.getAddonCatalog)
	}

	// Installed Addons API (all clusters or filtered by ?cluster=)
	installedAddons := adminAPI.Group("/installed-addons")
	{
		installedAddons.GET("", g.listInstalledAddons)
	}

	// Cluster Addon APIs
	clusterAddons := adminAPI.Group("/clusters/:cluster_id/addons")
	{
		clusterAddons.GET("", g.listClusterAddons)
		clusterAddons.POST("", g.installClusterAddon)
		clusterAddons.GET("/:name", g.getClusterAddon)
		clusterAddons.PUT("/:name", g.updateClusterAddon)
		clusterAddons.DELETE("/:name", g.deleteClusterAddon)
	}
}

// --- Node handlers ---

func (g *Gateway) rlinfv1alpha1ListNodes(c *gin.Context)  { g.handleList("nodes")(c) }
func (g *Gateway) rlinfv1alpha1CreateNode(c *gin.Context) { g.handleKubeCreate("nodes")(c) }
func (g *Gateway) rlinfv1alpha1GetNode(c *gin.Context)    { g.handleGet("nodes")(c) }
func (g *Gateway) rlinfv1alpha1UpdateNode(c *gin.Context) { g.handleKubeUpdate("nodes")(c) }
func (g *Gateway) rlinfv1alpha1PatchNode(c *gin.Context)  { g.handleKubePatch("nodes")(c) }
func (g *Gateway) rlinfv1alpha1DeleteNode(c *gin.Context) { g.handleKubeDelete("nodes")(c) }

// --- Workflow handlers ---

func (g *Gateway) rlinfv1alpha1ListWorkflows(c *gin.Context)  { g.handleList("workflows")(c) }
func (g *Gateway) rlinfv1alpha1CreateWorkflow(c *gin.Context) { g.handleKubeCreate("workflows")(c) }
func (g *Gateway) rlinfv1alpha1GetWorkflow(c *gin.Context)    { g.handleGet("workflows")(c) }
func (g *Gateway) rlinfv1alpha1UpdateWorkflow(c *gin.Context) { g.handleKubeUpdate("workflows")(c) }
func (g *Gateway) rlinfv1alpha1PatchWorkflow(c *gin.Context)  { g.handleKubePatch("workflows")(c) }
func (g *Gateway) rlinfv1alpha1DeleteWorkflow(c *gin.Context) { g.handleKubeDelete("workflows")(c) }

// --- Job handlers ---

func (g *Gateway) rlinfv1alpha1ListJobs(c *gin.Context)    { g.handleListJobs(c) }
func (g *Gateway) rlinfv1alpha1ListJobTags(c *gin.Context) { g.handleListJobTags(c) }
func (g *Gateway) rlinfv1alpha1CreateJob(c *gin.Context)   { g.handleKubeCreate("jobs")(c) }
func (g *Gateway) rlinfv1alpha1GetJob(c *gin.Context)      { g.handleGet("jobs")(c) }
func (g *Gateway) rlinfv1alpha1UpdateJob(c *gin.Context)   { g.handleKubeUpdate("jobs")(c) }
func (g *Gateway) rlinfv1alpha1PatchJob(c *gin.Context)    { g.handleKubePatch("jobs")(c) }
func (g *Gateway) rlinfv1alpha1DeleteJob(c *gin.Context)   { g.handleKubeDelete("jobs")(c) }

// --- Job sub-resource handlers ---

func (g *Gateway) rlinfv1alpha1JobMetrics(c *gin.Context) {
	c.JSON(501, gin.H{"message": "not implemented"})
}

// --- Task handlers ---

func (g *Gateway) rlinfv1alpha1CreateTask(c *gin.Context) { g.handleKubeCreate("tasks")(c) }
func (g *Gateway) rlinfv1alpha1UpdateTask(c *gin.Context) { g.handleKubeUpdate("tasks")(c) }
func (g *Gateway) rlinfv1alpha1PatchTask(c *gin.Context)  { g.handleKubePatch("tasks")(c) }
func (g *Gateway) rlinfv1alpha1DeleteTask(c *gin.Context) { g.handleKubeDelete("tasks")(c) }

// --- Pod handlers ---

func (g *Gateway) rlinfv1alpha1ListPods(c *gin.Context) { g.handleList("pods")(c) }
func (g *Gateway) rlinfv1alpha1GetPod(c *gin.Context)   { g.handleGet("pods")(c) }
func (g *Gateway) rlinfv1alpha1PatchPod(c *gin.Context) { g.handleKubePatch("pods")(c) }

// --- Domain handlers ---

func (g *Gateway) rlinfv1alpha1ListDomains(c *gin.Context)  { g.handleList("domains")(c) }
func (g *Gateway) rlinfv1alpha1CreateDomain(c *gin.Context) { g.handleKubeCreate("domains")(c) }
func (g *Gateway) rlinfv1alpha1GetDomain(c *gin.Context)    { g.handleGet("domains")(c) }
func (g *Gateway) rlinfv1alpha1UpdateDomain(c *gin.Context) { g.handleKubeUpdate("domains")(c) }
func (g *Gateway) rlinfv1alpha1PatchDomain(c *gin.Context)  { g.handleKubePatch("domains")(c) }
func (g *Gateway) rlinfv1alpha1DeleteDomain(c *gin.Context) { g.handleKubeDelete("domains")(c) }

package gateway

import "github.com/gin-gonic/gin"

// RegisterRoutes registers the routes.
func (g *Gateway) RegisterRoutes(r gin.IRouter) {
	rlinfv1alpha1 := r.Group("/api/v1/rlinf.io/v1alpha1")

	// Clusters API
	clusters := r.Group("/api/v1/clusters")
	{
		clusters.GET("", g.listClusters)
		clusters.GET("/:cluster_id", g.getCluster)
	}

	nodes := rlinfv1alpha1.Group("/nodes")
	{
		nodes.GET("", g.rlinfv1alpha1ListNodes)
		nodes.POST("", g.rlinfv1alpha1CreateNode)
		nodes.GET("/:name", g.rlinfv1alpha1GetNode)
		nodes.PUT("/:name", g.rlinfv1alpha1UpdateNode)
		nodes.PATCH("/:name", g.rlinfv1alpha1PatchNode)
		nodes.DELETE("/:name", g.rlinfv1alpha1DeleteNode)
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
		jobs.POST("", g.rlinfv1alpha1CreateJob)
		jobs.GET("/:name", g.rlinfv1alpha1GetJob)
		jobs.PUT("/:name", g.rlinfv1alpha1UpdateJob)
		jobs.PATCH("/:name", g.rlinfv1alpha1PatchJob)
		jobs.DELETE("/:name", g.rlinfv1alpha1DeleteJob)
		jobs.GET("/:name/logs", g.rlinfv1alpha1JobLogs)
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
		pods.GET("/:name", g.rlinfv1alpha1GetPod)
		pods.PATCH("/:name", g.rlinfv1alpha1PatchPod)
		pods.GET("/:name/terminal", g.handlePodTerminal)
	}

	domains := rlinfv1alpha1.Group("/domains")
	{
		domains.GET("", g.rlinfv1alpha1ListDomains)
		domains.POST("", g.rlinfv1alpha1CreateDomain)
		domains.GET("/:name", g.rlinfv1alpha1GetDomain)
		domains.PUT("/:name", g.rlinfv1alpha1UpdateDomain)
		domains.PATCH("/:name", g.rlinfv1alpha1PatchDomain)
		domains.DELETE("/:name", g.rlinfv1alpha1DeleteDomain)
	}

	certificates := r.Group("/api/v1/certificates")
	{
		certificates.GET("/agent", g.handleListAgentCerts)
		certificates.GET("/agent/:cluster_id", g.handleGetAgentCert)
		certificates.POST("/agent", g.handleSignAgentCert)
		certificates.POST("/revoke", g.handleRevokeCertificate)
	}

	sshUserKeys := r.Group("/api/v1/ssh-user-keys")
	{
		sshUserKeys.GET("", g.handleListSSHUserKeys)
		sshUserKeys.POST("", g.handleCreateSSHUserKey)
		sshUserKeys.DELETE("/:id", g.handleDeleteSSHUserKey)
	}

	auth := r.Group("/api/v1/auth")
	{
		auth.POST("/login", g.handleLogin)
	}

	// Image Registry APIs
	imageRegistries := r.Group("/api/v1/image-registries")
	{
		imageRegistries.GET("", g.handleListImageRegistries)
		imageRegistries.POST("", g.handleCreateImageRegistry)
		imageRegistries.GET("/:name", g.handleGetImageRegistry)
		imageRegistries.PUT("/:name", g.handleUpdateImageRegistry)
		imageRegistries.DELETE("/:name", g.handleDeleteImageRegistry)
	}

	// System Config APIs
	systemConfig := r.Group("/api/v1/system-config")
	{
		systemConfig.GET("", g.handleGetSystemConfig)
		systemConfig.PUT("", g.handleUpdateSystemConfig)
	}

	// Storage APIs
	storage := r.Group("/api/v1/storage")
	{
		storage.GET("/storageclass", g.listStorageClass)
		storage.POST("/storageclass", g.createStorageClass)
		storage.PUT("/storageclass/:name", g.updateStorageClass)
		storage.DELETE("/storageclass/:name", g.deleteStorageClass)
		storage.GET("/storageclass/provider", g.listProvider)

		scFiles := storage.Group("/storageclass/:name/:cluster")
		{
			scFiles.GET("/list", g.listStorageClassFiles)
			scFiles.POST("/upload", g.uploadStorageClassFile)
			scFiles.GET("/object/*key", g.getStorageClassObject)
			scFiles.DELETE("/object/*key", g.deleteStorageClassObject)
		}
	}

	// Addon Catalog APIs
	addons := r.Group("/api/v1/addons")
	{
		addons.GET("", g.listAddonCatalog)
		addons.GET("/:name", g.getAddonCatalog)
	}

	// Installed Addons API (all clusters or filtered by ?cluster=)
	installedAddons := r.Group("/api/v1/installed-addons")
	{
		installedAddons.GET("", g.listInstalledAddons)
	}

	// Cluster Addon APIs
	clusterAddons := r.Group("/api/v1/clusters/:cluster_id/addons")
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

func (g *Gateway) rlinfv1alpha1ListJobs(c *gin.Context)  { g.handleList("jobs")(c) }
func (g *Gateway) rlinfv1alpha1CreateJob(c *gin.Context) { g.handleKubeCreate("jobs")(c) }
func (g *Gateway) rlinfv1alpha1GetJob(c *gin.Context)    { g.handleGet("jobs")(c) }
func (g *Gateway) rlinfv1alpha1UpdateJob(c *gin.Context) { g.handleKubeUpdate("jobs")(c) }
func (g *Gateway) rlinfv1alpha1PatchJob(c *gin.Context)  { g.handleKubePatch("jobs")(c) }
func (g *Gateway) rlinfv1alpha1DeleteJob(c *gin.Context) { g.handleKubeDelete("jobs")(c) }

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

package gateway

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var stationClusterGVR = schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: "clusters"}

// pageOptions is a list request's page: limit (1 to 500, else the default) and continue. On a bad limit it has
// answered.
func pageOptions(c *gin.Context, limit int64) (metav1.ListOptions, bool) {
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 || parsed > 500 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 500"})
			return metav1.ListOptions{}, false
		}
		limit = parsed
	}
	return metav1.ListOptions{Limit: limit, Continue: c.Query("continue")}, true
}

// clusterNamespace reads the required cluster_id of the request and its Cluster's namespace. On failure it has
// answered.
func (g *Gateway) clusterNamespace(c *gin.Context) (clusterID, namespace string, ok bool) {
	clusterID = c.Query("cluster_id")
	if clusterID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_id is required"})
		return "", "", false
	}
	namespace, err := g.stationNamespace(c, clusterID)
	return clusterID, namespace, err == nil
}

// handleListStations answers GET /api/v1/station/stations: the site nodes (NetworkEnrollments) of one cluster, or of
// all without cluster_id, each without its network document.
func (g *Gateway) handleListStations(c *gin.Context) {
	if hasStationQuery(c) {
		g.handleFilteredStations(c)
		return
	}
	options, ok := pageOptions(c, 200)
	if !ok {
		return
	}
	resource := g.stationClient.Resource(networkEnrollmentGVR)
	var scope dynamic.ResourceInterface = resource
	if clusterID := c.Query("cluster_id"); clusterID != "" {
		clusters, ok := g.stationQueryClusters(c)
		if !ok {
			return
		}
		namespace, ok := clusters.namespace(c, clusterID)
		if !ok {
			return
		}
		scope = resource.Namespace(namespace)
	}
	records, err := scope.List(c.Request.Context(), options)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
		return
	}
	for i := range records.Items {
		unstructured.RemoveNestedField(records.Items[i].Object, "status", "config")
	}
	c.JSON(http.StatusOK, gin.H{"items": records.Items, "continue": records.GetContinue()})
}

func hasStationQuery(c *gin.Context) bool {
	for _, key := range []string{"q", "phase", "model", "zone", "power_state", "occupancy", "alert_only", "sort"} {
		if _, ok := c.GetQuery(key); ok {
			return true
		}
	}
	return false
}

func listAllStations(c *gin.Context, resource dynamic.ResourceInterface) ([]unstructured.Unstructured, error) {
	var all []unstructured.Unstructured
	continueToken := ""
	for {
		page, err := resource.List(c.Request.Context(), metav1.ListOptions{Limit: 500, Continue: continueToken})
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		continueToken = page.GetContinue()
		if continueToken == "" {
			return all, nil
		}
	}
}

func stationText(record *unstructured.Unstructured, paths ...[]string) string {
	parts := []string{record.GetName(), record.GetNamespace()}
	for _, path := range paths {
		value, _, _ := unstructured.NestedString(record.Object, path...)
		parts = append(parts, value)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func stationAlert(record *unstructured.Unstructured) bool {
	return stationAlertSeverity(record) > 0
}

func stationAlertSeverity(record *unstructured.Unstructured) int {
	labels, annotations := stationNodeLabels(record), stationNodeAnnotations(record)
	state, _, _ := unstructured.NestedString(record.Object, "status", "node", "state")
	selfCheckFailed := labels["rlark.io/self-check-state"] == "failed" || stationConditionFalse(record, "SelfCheck")
	power := labels["rlark.io/power-state"]
	if power == "fault" || power == "estop" || selfCheckFailed {
		return 3
	}
	if state == "Conflict" || state == "Offline" {
		return 2
	}
	if stationCameraFault(labels, annotations) || labels["rlark.io/maintenance-state"] != "" && labels["rlark.io/maintenance-state"] != "none" {
		return 1
	}
	return 0
}

func stationPhase(record *unstructured.Unstructured) string {
	retired, _, _ := unstructured.NestedBool(record.Object, "spec", "retired")
	if retired {
		return "Retired"
	}
	phase, _, _ := unstructured.NestedString(record.Object, "status", "node", "state")
	if phase == "Bound" || phase == "Offline" || phase == "Conflict" || phase == "Retired" {
		return phase
	}
	return "Unknown"
}

func stationNodeLabels(record *unstructured.Unstructured) map[string]string {
	labels, _, _ := unstructured.NestedStringMap(record.Object, "metadata", "labels")
	return labels
}

func stationNodeAnnotations(record *unstructured.Unstructured) map[string]string {
	annotations, _, _ := unstructured.NestedStringMap(record.Object, "metadata", "annotations")
	return annotations
}

func stationConditionFalse(record *unstructured.Unstructured, conditionType string) bool {
	conditions, _, _ := unstructured.NestedSlice(record.Object, "status", "node", "conditions")
	for _, value := range conditions {
		condition, ok := value.(map[string]interface{})
		if ok && condition["type"] == conditionType && condition["status"] == "False" {
			return true
		}
	}
	return false
}

func stationCameraFault(labels, annotations map[string]string) bool {
	if labels["rlark.io/camera-fault"] != "" || annotations["rlark.io/camera-fault"] != "" {
		return true
	}
	var report struct {
		Cameras map[string]struct {
			State string `json:"state"`
			Step  int    `json:"step"`
		} `json:"cameras"`
	}
	if json.Unmarshal([]byte(annotations[siteNodeCameras]), &report) != nil {
		return false
	}
	for _, camera := range report.Cameras {
		if (camera.State == "stale" || camera.State == "absent") && camera.Step != 1 && camera.Step != 2 {
			return true
		}
	}
	return false
}

func stationMetaModel(record *unstructured.Unstructured) string {
	labels := stationNodeLabels(record)
	if labels["rlark.io/robot-model"] != "" {
		return labels["rlark.io/robot-model"]
	}
	if labels["rlark.io/node-category"] != "" {
		return labels["rlark.io/node-category"]
	}
	for _, path := range [][]string{{"status", "site", "declared_model"}, {"status", "site", "observed_model"}, {"status", "site", "declared_type"}, {"status", "site", "observed_type"}, {"status", "observation", "identity", "model"}} {
		value, _, _ := unstructured.NestedString(record.Object, path...)
		if value != "" {
			return value
		}
	}
	return ""
}

func stationZone(record *unstructured.Unstructured) string {
	labels := stationNodeLabels(record)
	for _, value := range []string{stationNestedString(record, "status", "site", "zone"), stationNestedString(record, "status", "site", "name"), stationNestedString(record, "status", "site"), labels["site.rlark.io/zone"], labels["rlark.io/zone"]} {
		if value != "" {
			return value
		}
	}
	return ""
}

func stationNestedString(record *unstructured.Unstructured, fields ...string) string {
	value, _, _ := unstructured.NestedString(record.Object, fields...)
	return value
}

type stationQueryClusters struct {
	namespaces map[string]string
	owners     map[string]string // An empty owner marks a namespace claimed by multiple clusters.
	names      map[string]string
}

func (g *Gateway) stationQueryClusters(c *gin.Context) (stationQueryClusters, bool) {
	clusters, err := listAllStations(c, g.stationClient.Resource(stationClusterGVR))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Cluster API request failed"})
		return stationQueryClusters{}, false
	}
	index := stationQueryClusters{
		namespaces: make(map[string]string, len(clusters)),
		owners:     make(map[string]string, len(clusters)),
		names:      make(map[string]string, len(clusters)),
	}
	for i := range clusters {
		cluster := &clusters[i]
		id := cluster.GetName()
		namespace := stationNestedString(cluster, "spec", "agentNamespace")
		index.namespaces[id] = namespace
		if namespace == "" {
			continue
		}
		if _, exists := index.owners[namespace]; exists {
			index.owners[namespace] = ""
			delete(index.names, namespace)
			continue
		}
		index.owners[namespace] = id
		name := stationNestedString(cluster, "spec", "displayName")
		if name == "" {
			name = id
		}
		index.names[namespace] = name
	}
	return index, true
}

func (index stationQueryClusters) namespace(c *gin.Context, clusterID string) (string, bool) {
	namespace, exists := index.namespaces[clusterID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cluster not found"})
		return "", false
	}
	if namespace == "" || index.owners[namespace] != clusterID {
		c.JSON(http.StatusConflict, gin.H{"error": "Cluster namespace mapping is invalid or ambiguous"})
		return "", false
	}
	return namespace, true
}

func enrichStationNodes(c *gin.Context, client dynamic.Interface, items []unstructured.Unstructured) error {
	namespaces := map[string]bool{}
	for i := range items {
		namespaces[items[i].GetNamespace()] = true
	}
	for namespace := range namespaces {
		list, err := listAllStations(c, client.Resource(siteNodeGVR).Namespace(namespace))
		if err != nil {
			return err
		}
		byName := make(map[string]unstructured.Unstructured, len(list))
		for i := range list {
			byName[list[i].GetName()] = list[i]
		}
		for i := range items {
			if items[i].GetNamespace() != namespace {
				continue
			}
			name, _, _ := unstructured.NestedString(items[i].Object, "status", "nodeName")
			if name == "" {
				name, _, _ = unstructured.NestedString(items[i].Object, "spec", "nodeName")
			}
			if name == "" {
				name = items[i].GetName()
			}
			node, ok := byName[name]
			if !ok {
				continue
			}
			labels := map[string]string{}
			for key, value := range items[i].GetLabels() {
				labels[key] = value
			}
			for key, value := range node.GetLabels() {
				labels[key] = value
			}
			if observed, _, _ := unstructured.NestedStringMap(node.Object, "status", "observedNodeLabels"); len(observed) > 0 {
				for key, value := range observed {
					if _, exists := labels[key]; !exists {
						labels[key] = value
					}
				}
			}
			annotations := map[string]string{}
			for key, value := range items[i].GetAnnotations() {
				annotations[key] = value
			}
			for key, value := range node.GetAnnotations() {
				annotations[key] = value
			}
			if len(labels) > 0 {
				_ = unstructured.SetNestedStringMap(items[i].Object, labels, "metadata", "labels")
			}
			if len(annotations) > 0 {
				_ = unstructured.SetNestedStringMap(items[i].Object, annotations, "metadata", "annotations")
			}
			if conditions, found, _ := unstructured.NestedSlice(node.Object, "status", "conditions"); found {
				stationConditions, _, _ := unstructured.NestedSlice(items[i].Object, "status", "node", "conditions")
				byType := make(map[string]interface{}, len(stationConditions)+len(conditions))
				for _, condition := range stationConditions {
					if object, ok := condition.(map[string]interface{}); ok {
						if kind, ok := object["type"].(string); ok {
							byType[kind] = condition
						}
					}
				}
				for _, condition := range conditions {
					if object, ok := condition.(map[string]interface{}); ok {
						if kind, ok := object["type"].(string); ok {
							byType[kind] = condition
						}
					}
				}
				merged := make([]interface{}, 0, len(byType))
				for _, condition := range byType {
					merged = append(merged, condition)
				}
				_ = unstructured.SetNestedSlice(items[i].Object, merged, "status", "node", "conditions")
			}
		}
	}
	return nil
}

func (g *Gateway) stationRecords(c *gin.Context) ([]unstructured.Unstructured, map[string]string, bool) {
	clusters, ok := g.stationQueryClusters(c)
	if !ok {
		return nil, nil, false
	}
	resource := g.stationClient.Resource(networkEnrollmentGVR)
	var scope dynamic.ResourceInterface = resource
	if clusterID := c.Query("cluster_id"); clusterID != "" {
		namespace, ok := clusters.namespace(c, clusterID)
		if !ok {
			return nil, nil, false
		}
		scope = resource.Namespace(namespace)
	}
	items, err := listAllStations(c, scope)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
		return nil, nil, false
	}
	for i := range items {
		if owner, known := clusters.owners[items[i].GetNamespace()]; known && owner == "" {
			c.JSON(http.StatusConflict, gin.H{"error": "Station records belong to an ambiguous cluster namespace"})
			return nil, nil, false
		}
	}
	if err := enrichStationNodes(c, g.stationClient, items); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
		return nil, nil, false
	}
	return items, clusters.names, true
}

func (g *Gateway) handleFilteredStations(c *gin.Context) {
	phaseFilter := strings.ToLower(strings.TrimSpace(c.Query("phase")))
	for _, phase := range strings.Split(phaseFilter, ",") {
		if phase != "" && phase != "bound" && phase != "offline" && phase != "conflict" && phase != "retired" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid phase"})
			return
		}
	}
	powerFilter := strings.ToLower(strings.TrimSpace(c.Query("power_state")))
	if powerFilter != "" && powerFilter != "running" && powerFilter != "powered" && powerFilter != "off" && powerFilter != "alert" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid power_state"})
		return
	}
	occupancyFilter := strings.ToLower(strings.TrimSpace(c.Query("occupancy")))
	if occupancyFilter != "" && occupancyFilter != "occupied" && occupancyFilter != "idle" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid occupancy"})
		return
	}
	sortFilter := strings.ToLower(strings.TrimSpace(c.Query("sort")))
	if sortFilter != "" && sortFilter != "alert" && sortFilter != "name" && sortFilter != "heartbeat" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sort"})
		return
	}
	limit := int64(200)
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 1 || n > 500 {
			c.JSON(400, gin.H{"error": "limit must be between 1 and 500"})
			return
		}
		limit = n
	}
	items, clusterNames, ok := g.stationRecords(c)
	if !ok {
		return
	}
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	model := strings.ToLower(c.Query("model"))
	zone := strings.ToLower(c.Query("zone"))
	phases := map[string]bool{}
	for _, phase := range strings.Split(phaseFilter, ",") {
		if phase != "" {
			phases[phase] = true
		}
	}
	matched := make([]unstructured.Unstructured, 0, len(items))
	for _, item := range items {
		phase := stationPhase(&item)
		modelValue := stationMetaModel(&item)
		zoneValue := stationZone(&item)
		if query != "" && !strings.Contains(stationText(&item, []string{"spec", "nodeUID"}, []string{"spec", "systemUUID"}, []string{"metadata", "annotations", "rlark.io/display-name"}, []string{"metadata", "labels", "rlark.io/station-mark"}, []string{"status", "site", "declared_model"}, []string{"status", "site", "observed_model"}, []string{"status", "site", "declared_type"}, []string{"status", "site", "observed_type"}), query) && !strings.Contains(strings.ToLower(modelValue+" "+zoneValue+" "+clusterNames[item.GetNamespace()]), query) {
			continue
		}
		if model != "" && !strings.Contains(strings.ToLower(modelValue), model) {
			continue
		}
		if zone != "" && !strings.Contains(strings.ToLower(zoneValue), zone) {
			continue
		}
		if len(phases) > 0 && !phases[strings.ToLower(phase)] {
			continue
		}
		if strings.EqualFold(c.Query("alert_only"), "true") || c.Query("alert_only") == "1" {
			if !stationAlert(&item) {
				continue
			}
		}
		if powerFilter != "" {
			power := stationNodeLabels(&item)["rlark.io/power-state"]
			switch powerFilter {
			case "alert":
				if stationAlertSeverity(&item) < 3 {
					continue
				}
			case "off":
				if power != "off" {
					continue
				}
			case "running":
				if power != "ready" {
					continue
				}
			case "powered":
				if power != "powered" && power != "ready" {
					continue
				}
			}
		}
		if occupancyFilter != "" {
			labels, annotations := stationNodeLabels(&item), stationNodeAnnotations(&item)
			occupied := labels["rlark.io/occupied"] == "true" || labels["rlark.io/occupancy"] == "occupied" || annotations["rlark.io/occupancy"] == "occupied"
			if occupancyFilter == "occupied" && !occupied || occupancyFilter == "idle" && occupied {
				continue
			}
		}
		matched = append(matched, item)
	}
	switch sortFilter {
	case "name":
		sort.SliceStable(matched, func(i, j int) bool {
			return matched[i].GetName() < matched[j].GetName()
		})
	case "", "alert":
		sort.SliceStable(matched, func(i, j int) bool {
			a, b := stationAlertSeverity(&matched[i]), stationAlertSeverity(&matched[j])
			if a != b {
				return a > b
			}
			clusterA, clusterB := clusterNames[matched[i].GetNamespace()], clusterNames[matched[j].GetNamespace()]
			if clusterA != clusterB {
				return clusterA < clusterB
			}
			return matched[i].GetName() < matched[j].GetName()
		})
	case "heartbeat":
		sort.SliceStable(matched, func(i, j int) bool {
			a, _, _ := unstructured.NestedString(matched[i].Object, "status", "node", "lastSeenTime")
			b, _, _ := unstructured.NestedString(matched[j].Object, "status", "node", "lastSeenTime")
			if a != b {
				return a > b
			}
			clusterA, clusterB := clusterNames[matched[i].GetNamespace()], clusterNames[matched[j].GetNamespace()]
			if clusterA != clusterB {
				return clusterA < clusterB
			}
			return matched[i].GetName() < matched[j].GetName()
		})
	}
	offset := int64(0)
	if raw := c.Query("continue"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			c.JSON(400, gin.H{"error": "invalid continue"})
			return
		}
		offset = n
	}
	start := min(int(offset), len(matched))
	end := min(start+int(limit), len(matched))
	out := matched[start:end]
	for i := range out {
		unstructured.RemoveNestedField(out[i].Object, "status", "config")
	}
	next := ""
	if end < len(matched) {
		next = strconv.Itoa(end)
	}
	c.JSON(200, gin.H{"items": out, "continue": next, "totalMatched": len(matched)})
}

func (g *Gateway) handleStationSummary(c *gin.Context) {
	items, _, ok := g.stationRecords(c)
	if !ok {
		return
	}
	type clusterSummary struct {
		ClusterID   string `json:"clusterId"`
		Namespace   string `json:"namespace"`
		Total       int    `json:"total"`
		Robots      int    `json:"robots"`
		Bound       int    `json:"bound"`
		Offline     int    `json:"offline"`
		Conflict    int    `json:"conflict"`
		Alerts      int    `json:"alerts"`
		DriverReady int    `json:"driverReady"`
		CameraFault int    `json:"cameraFault"`
	}
	byCluster := map[string]*clusterSummary{}
	summary := gin.H{"total": len(items), "robots": 0, "otherNodes": 0, "byPhase": gin.H{"Bound": 0, "Offline": 0, "Conflict": 0, "Retired": 0, "Unknown": 0}, "alerts": 0, "driverReady": 0, "cameraFault": 0, "occupied": 0}
	robots, alerts, drivers, camera, occupied := 0, 0, 0, 0, 0
	for i := range items {
		it := &items[i]
		cid, _, _ := unstructured.NestedString(it.Object, "spec", "clusterID")
		cs := byCluster[cid]
		if cs == nil {
			cs = &clusterSummary{ClusterID: cid, Namespace: it.GetNamespace()}
			byCluster[cid] = cs
		}
		cs.Total++
		labels, annotations := stationNodeLabels(it), stationNodeAnnotations(it)
		robot := labels["rlark.io/robot-model"] != "" || labels["rlark.io/node-category"] == "robot"
		if robot {
			robots++
			cs.Robots++
		}
		phase := stationPhase(it)
		phaseMap := summary["byPhase"].(gin.H)
		phaseMap[phase] = phaseMap[phase].(int) + 1
		switch phase {
		case "Bound":
			cs.Bound++
		case "Offline":
			cs.Offline++
		case "Conflict":
			cs.Conflict++
		}
		if stationAlert(it) {
			alerts++
			cs.Alerts++
		}
		if labels["rlark.io/power-state"] == "ready" {
			drivers++
			cs.DriverReady++
		}
		if stationCameraFault(labels, annotations) {
			camera++
			cs.CameraFault++
		}
		if labels["rlark.io/occupied"] == "true" || labels["rlark.io/occupancy"] == "occupied" || annotations["rlark.io/occupancy"] == "occupied" {
			occupied++
		}
	}
	clusters := make([]clusterSummary, 0, len(byCluster))
	for _, v := range byCluster {
		clusters = append(clusters, *v)
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].ClusterID < clusters[j].ClusterID })
	for _, it := range items {
		labels := stationNodeLabels(&it)
		if labels["rlark.io/robot-model"] == "" && labels["rlark.io/node-category"] != "robot" {
			summary["otherNodes"] = summary["otherNodes"].(int) + 1
		}
	}
	summary["robots"] = robots
	summary["alerts"] = alerts
	summary["driverReady"] = drivers
	summary["cameraFault"] = camera
	summary["occupied"] = occupied
	summary["clusters"] = clusters
	summary["generatedAt"] = time.Now().UTC().Format(time.RFC3339)
	c.JSON(200, summary)
}

// handleGetStation answers GET /api/v1/station/stations/:name?cluster_id=: one site node, without its network
// document.
func (g *Gateway) handleGetStation(c *gin.Context) {
	_, namespace, ok := g.clusterNamespace(c)
	if !ok {
		return
	}
	record, err := g.stationClient.Resource(networkEnrollmentGVR).Namespace(namespace).Get(c.Request.Context(), c.Param("name"), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "site node not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
		return
	}
	unstructured.RemoveNestedField(record.Object, "status", "config")
	c.JSON(http.StatusOK, record.Object)
}

func (g *Gateway) stationNamespace(c *gin.Context, clusterID string) (string, error) {
	cluster, err := g.stationClient.Resource(stationClusterGVR).Get(c.Request.Context(), clusterID, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cluster not found"})
		return "", err
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Cluster API request failed"})
		return "", err
	}
	namespace, _, _ := unstructured.NestedString(cluster.Object, "spec", "agentNamespace")
	if namespace == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "Cluster namespace mapping is invalid"})
		return "", apierrors.NewBadRequest("invalid Cluster namespace mapping")
	}
	return namespace, nil
}

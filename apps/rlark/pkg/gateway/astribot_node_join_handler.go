package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/nodessh"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/util/retry"
)

// nodeJoinClusterLabel holds a NodeJoin's cluster, by which a cluster's joins are listed.
const nodeJoinClusterLabel = "rlark.io/cluster"

type createNodeJoinRequest struct {
	ClusterID        string            `json:"cluster_id"`
	MachineName      string            `json:"machine_name,omitempty"`
	HostKey          string            `json:"host_key,omitempty"`
	HostKeyAlgorithm string            `json:"host_key_algorithm,omitempty"`
	MachineType      string            `json:"machine_type,omitempty"`
	MachineModel     string            `json:"machine_model,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Address          string            `json:"address"`
	// Port is the machine's SSH port, 22 when it is not given.
	Port         int32  `json:"port,omitempty"`
	User         string `json:"user"`
	Password     string `json:"password"`
	SudoPassword string `json:"sudo_password,omitempty"`
	// NodeName is the node's name; empty takes the machine's host name.
	NodeName string `json:"node_name,omitempty"`
	// TunnelEndpoint is the ip:port other sites dial for the machine's tunnel, when it joins through one; empty, the
	// address at port 51820, which must then be an IPv4 address.
	TunnelEndpoint string `json:"tunnel_endpoint,omitempty"`
}

// nodeJoinRequest retries a join from the stage it stopped at, or clears it. User, when given, is the account to log
// in with from now on; MachineGone (clear) says the machine is not there to be put back.
type nodeJoinRequest struct {
	Password     string `json:"password"`
	SudoPassword string `json:"sudo_password,omitempty"`
	User         string `json:"user"`
	MachineGone  bool   `json:"machine_gone"`
}

// handleCreateNodeJoin answers POST /api/v1/station/node-joins: it logs in to the machine once, at once, so that a
// password the machine refuses is said on the form and the node's name is known; then it records the join and runs
// its stages in the background, from the login it made.
func (g *Gateway) handleCreateNodeJoin(c *gin.Context) {
	var request createNodeJoinRequest
	if !decodeOneJSON(c, maxNodeAdmissionRequestBytes, &request, false) {
		return
	}
	portProvided := request.Port != 0
	password := []byte(request.Password)
	sudoPassword := []byte(request.SudoPassword)
	request.Password = ""
	request.SudoPassword = ""
	if request.Port == 0 {
		request.Port = 22
	}
	request.TunnelEndpoint = strings.TrimSpace(request.TunnelEndpoint)
	if request.ClusterID == "" || request.Address == "" || net.ParseIP(request.Address) == nil && len(validation.IsDNS1123Subdomain(request.Address)) > 0 ||
		request.Port < 1 || request.Port > 65535 || request.User == "" || len(request.User) > 64 || strings.ContainsAny(request.User, " \t\r\n\x00'\"\\$`") ||
		len(password) == 0 || len(password) > 4096 || len(sudoPassword) > 4096 || request.TunnelEndpoint != "" && !validTunnelEndpoint(request.TunnelEndpoint) {
		clear(password)
		clear(sudoPassword)
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_id, address (an IP address or host name), port, user and password are required; tunnel_endpoint is an IPv4 ip:port"})
		return
	}
	namespace, err := g.stationNamespace(c, request.ClusterID)
	if err != nil {
		clear(password)
		clear(sudoPassword)
		return
	}
	var candidate *unstructured.Unstructured
	if request.MachineName != "" {
		candidate, err = g.stationClient.Resource(machineGVR).Namespace(machineNamespace).Get(c.Request.Context(), request.MachineName, metav1.GetOptions{})
		if err != nil {
			clear(password)
			clear(sudoPassword)
			c.JSON(http.StatusBadRequest, gin.H{"error": "scanned Machine is unavailable"})
			return
		}
		candidateAddress, _, _ := unstructured.NestedString(candidate.Object, "status", "entry", "address")
		candidatePort, _, _ := unstructured.NestedInt64(candidate.Object, "status", "entry", "port")
		candidateKey, _, _ := unstructured.NestedString(candidate.Object, "status", "entry", "hostKeyFingerprint")
		candidateAlgorithm, _, _ := unstructured.NestedString(candidate.Object, "status", "entry", "hostKeyAlgorithm")
		candidatePhase, _, _ := unstructured.NestedString(candidate.Object, "status", "phase")
		candidateCluster, _, _ := unstructured.NestedString(candidate.Object, "status", "clusterID")
		candidateNode, _, _ := unstructured.NestedString(candidate.Object, "status", "nodeName")
		if candidatePort == 0 {
			candidatePort = 22
		}
		if candidatePhase == "Joined" || candidateCluster != "" || candidateNode != "" {
			clear(password)
			clear(sudoPassword)
			c.JSON(http.StatusConflict, gin.H{"error": "scanned Machine is already a joined node"})
			return
		}
		if (request.MachineType != "" && request.MachineType != "Server") || candidateAddress == "" || candidateKey == "" || candidateAlgorithm == "" ||
			(request.Address != "" && request.Address != candidateAddress) || (portProvided && int64(request.Port) != candidatePort) ||
			(request.HostKey != "" && request.HostKey != candidateKey) || (request.HostKeyAlgorithm != "" && request.HostKeyAlgorithm != candidateAlgorithm) {
			clear(password)
			clear(sudoPassword)
			c.JSON(http.StatusConflict, gin.H{"error": "scanned candidate identity changed; refresh discovery and review the host key"})
			return
		}
		request.Address, request.Port, request.HostKey, request.HostKeyAlgorithm, request.MachineType = candidateAddress, int32(candidatePort), candidateKey, candidateAlgorithm, "Server"
		if _, present, _ := unstructured.NestedMap(candidate.Object, "spec", "admit"); present {
			clear(password)
			clear(sudoPassword)
			c.JSON(http.StatusConflict, gin.H{"error": "scanned Machine already has an admission"})
			return
		}
	} else if request.MachineType != "" && request.MachineType != "Server" {
		clear(password)
		clear(sudoPassword)
		c.JSON(http.StatusBadRequest, gin.H{"error": "NodeJoin only accepts Server machines"})
		return
	}
	if request.MachineType == "" {
		request.MachineType = "Server"
	}
	for key, value := range request.Labels {
		if len(validation.IsQualifiedName(key)) > 0 || len(validation.IsValidLabelValue(value)) > 0 {
			clear(password)
			clear(sudoPassword)
			c.JSON(http.StatusBadRequest, gin.H{"error": "labels must be valid Kubernetes labels"})
			return
		}
	}
	ctx := c.Request.Context()
	login, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var machine provisionMachine
	var hostKey string
	if candidate != nil {
		annotations, annotationErr := nodeJoinAnnotations(request, request.HostKey)
		if annotationErr != nil {
			clear(password)
			clear(sudoPassword)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid NodeJoin metadata"})
			return
		}
		probe := &rlarkv1alpha1.NodeJoin{ObjectMeta: metav1.ObjectMeta{Annotations: annotations}, Spec: rlarkv1alpha1.NodeJoinSpec{Machine: rlarkv1alpha1.NodeJoinMachine{Address: request.Address, Port: request.Port, User: request.User}}}
		machine, hostKey, err = g.dialNodeJoinMachine(login, probe, password, sudoPassword)
	} else {
		machine, hostKey, err = g.provisionDial(login, rlarkv1alpha1.ClusterProvisionMachine{Address: request.Address, Port: request.Port, User: request.User}, password, "")
	}
	if err != nil {
		clear(password)
		clear(sudoPassword)
		var failure *nodessh.Error
		if errors.As(err, &failure) {
			loginRefused(failure).respond(c)
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	started := false
	defer func() {
		if !started {
			_ = machine.Close()
			clear(password)
			clear(sudoPassword)
		}
	}()
	if request.NodeName == "" {
		name, err := machine.Run(login, "hostname", nil, nil)
		if request.NodeName = nodeNameOfHost(name); err != nil || request.NodeName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("the machine's host name %q is no node name; give one", strings.TrimSpace(name)), "reason": "NodeName"})
			return
		}
	}
	name := nodeJoinName(request.ClusterID, request.NodeName)
	if len(validation.IsDNS1123Subdomain(request.NodeName)) > 0 || len(validation.IsDNS1123Subdomain(name)) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_name must be a DNS subdomain", "reason": "NodeName"})
		return
	}
	client := g.kubeClient.RlinfV1alpha1().NodeJoins()
	joins, err := client.List(ctx, metav1.ListOptions{})
	if err != nil {
		respondKubeError(c, "list node joins: "+err.Error(), err)
		return
	}
	for _, other := range joins.Items {
		switch {
		case other.Name == name && other.Status.Phase == rlarkv1alpha1.ClusterProvisionSucceeded:
			if err := client.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				respondKubeError(c, "delete the earlier join: "+err.Error(), err)
				return
			}
		case other.Name == name:
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("an earlier join of %s is left (%s, at stage %s): retry it, or clear it and start over", request.NodeName, other.Status.Phase, other.Status.Stage)})
			return
		case other.Status.Phase == rlarkv1alpha1.ClusterProvisionRunning && other.Spec.Machine.Address == request.Address && other.Spec.Machine.Port == request.Port:
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("the join %s runs on %s:%d now", other.Name, request.Address, request.Port)})
			return
		}
	}
	if candidate != nil {
		resource := g.stationClient.Resource(machineGVR).Namespace(machineNamespace)
		latest, err := resource.Get(ctx, candidate.GetName(), metav1.GetOptions{})
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "scanned Machine changed before its admission was recorded; refresh and retry"})
			return
		}
		latestAddress, _, _ := unstructured.NestedString(latest.Object, "status", "entry", "address")
		latestPort, _, _ := unstructured.NestedInt64(latest.Object, "status", "entry", "port")
		latestKey, _, _ := unstructured.NestedString(latest.Object, "status", "entry", "hostKeyFingerprint")
		latestAlgorithm, _, _ := unstructured.NestedString(latest.Object, "status", "entry", "hostKeyAlgorithm")
		latestSite, _, _ := unstructured.NestedString(latest.Object, "spec", "site")
		initialSite, _, _ := unstructured.NestedString(candidate.Object, "spec", "site")
		if latestPort == 0 {
			latestPort = 22
		}
		_, alreadyAdmitted, _ := unstructured.NestedMap(latest.Object, "spec", "admit")
		if latestAddress != request.Address || latestPort != int64(request.Port) || latestKey != request.HostKey || latestAlgorithm != request.HostKeyAlgorithm || latestSite != initialSite || alreadyAdmitted {
			c.JSON(http.StatusConflict, gin.H{"error": "scanned candidate identity changed; refresh discovery and review before joining"})
			return
		}
		patch, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]string{"resourceVersion": latest.GetResourceVersion()},
			"spec":     map[string]interface{}{"admit": machineAdmissionForServer(request, c.GetString(authSubjectKey), time.Now().UTC().Format(time.RFC3339))},
		})
		if _, err := resource.Patch(ctx, latest.GetName(), types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "scanned Machine changed before its admission was recorded; refresh and retry"})
			return
		}
	}
	annotations, annotationErr := nodeJoinAnnotations(request, hostKey)
	if annotationErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid NodeJoin metadata"})
		return
	}
	labels := map[string]string{nodeJoinClusterLabel: request.ClusterID}
	if candidate != nil {
		labels[nodeJoinMachineAnnotation] = candidate.GetName()
	}
	j, err := client.Create(ctx, &rlarkv1alpha1.NodeJoin{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels, Annotations: annotations},
		Spec: rlarkv1alpha1.NodeJoinSpec{
			ClusterID: request.ClusterID, NodeName: request.NodeName, RequestedBy: c.GetString(authSubjectKey),
			Machine: rlarkv1alpha1.NodeJoinMachine{Address: request.Address, Port: request.Port, User: request.User, TunnelEndpoint: request.TunnelEndpoint},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		if candidate != nil {
			_ = g.clearMachineAdmission(ctx, candidate.GetName(), request.ClusterID, c.GetString(authSubjectKey))
		}
		respondKubeError(c, "create node join: "+err.Error(), err)
		return
	}
	j.Status = newJoinStatus(hostKey)
	if j, err = client.UpdateStatus(ctx, j, metav1.UpdateOptions{}); err != nil {
		if candidate != nil {
			_ = g.clearMachineAdmission(ctx, candidate.GetName(), request.ClusterID, c.GetString(authSubjectKey))
		}
		_ = client.Delete(ctx, name, metav1.DeleteOptions{})
		respondKubeError(c, "write the node join's status: "+err.Error(), err)
		return
	}
	started = g.startNodeJoin(ctx, &nodeJoinRun{g: g, name: name, namespace: namespace, password: password, sudoPassword: sudoPassword, machine: machine}, joinStages[0])
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, gin.H{"data": j})
}

// startNodeJoin runs the join in the background, from stage on; false when it already runs.
func (g *Gateway) startNodeJoin(ctx context.Context, run *nodeJoinRun, stage string) bool {
	if !g.nodeJoins.start(run.name) {
		return false
	}
	go run.run(context.WithoutCancel(ctx), stage)
	return true
}

// handleListNodeJoins answers GET /api/v1/station/node-joins?cluster_id=: the cluster's joins, running ones first,
// then the newest.
func (g *Gateway) handleListNodeJoins(c *gin.Context) {
	clusterID := c.Query("cluster_id")
	if clusterID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_id is required"})
		return
	}
	items, err := g.nodeJoinsOf(c.Request.Context(), clusterID)
	if err != nil {
		respondKubeError(c, "list node joins: "+err.Error(), err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// nodeJoinsOf lists a cluster's joins, running ones first and then the newest, and deletes those that succeeded more
// than joinKeptFor ago.
func (g *Gateway) nodeJoinsOf(ctx context.Context, clusterID string) ([]rlarkv1alpha1.NodeJoin, error) {
	client := g.kubeClient.RlinfV1alpha1().NodeJoins()
	list, err := client.List(ctx, metav1.ListOptions{LabelSelector: nodeJoinClusterLabel + "=" + clusterID})
	if err != nil {
		return nil, err
	}
	items := make([]rlarkv1alpha1.NodeJoin, 0, len(list.Items))
	for _, j := range list.Items {
		// A join that succeeded is listed for joinKeptFor after its last stage.
		if j.Status.Phase == rlarkv1alpha1.ClusterProvisionSucceeded && time.Since(j.Status.Stages[len(j.Status.Stages)-1].FinishedAt.Time) > joinKeptFor {
			if err := client.Delete(ctx, j.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return nil, err
			}
			continue
		}
		items = append(items, j)
	}
	slices.SortStableFunc(items, func(a, b rlarkv1alpha1.NodeJoin) int {
		if running := (a.Status.Phase == rlarkv1alpha1.ClusterProvisionRunning); running != (b.Status.Phase == rlarkv1alpha1.ClusterProvisionRunning) {
			if running {
				return -1
			}
			return 1
		}
		return b.CreationTimestamp.Compare(a.CreationTimestamp.Time)
	})
	return items, nil
}

func (g *Gateway) handleGetNodeJoin(c *gin.Context) {
	j, err := g.kubeClient.RlinfV1alpha1().NodeJoins().Get(c.Request.Context(), c.Param("name"), metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, "get node join: "+err.Error(), err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": j})
}

// handleRetryNodeJoin answers POST /api/v1/station/node-joins/:name/retry: a join that stopped goes on from its stage.
func (g *Gateway) handleRetryNodeJoin(c *gin.Context) {
	var request nodeJoinRequest
	if !decodeOneJSON(c, maxNodeAdmissionRequestBytes, &request, false) {
		return
	}
	password := []byte(request.Password)
	sudoPassword := []byte(request.SudoPassword)
	transferred := false
	defer func() {
		if !transferred {
			clear(password)
			clear(sudoPassword)
		}
	}()
	request.Password = ""
	request.SudoPassword = ""
	ctx := c.Request.Context()
	client := g.kubeClient.RlinfV1alpha1().NodeJoins()
	j, err := client.Get(ctx, c.Param("name"), metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, "get node join: "+err.Error(), err)
		return
	}
	if j.Status.Phase != rlarkv1alpha1.ClusterProvisionFailed {
		c.JSON(http.StatusConflict, gin.H{"error": "the join of " + j.Spec.NodeName + " is " + string(j.Status.Phase) + ": only a stopped one is retried"})
		return
	}
	if joinNeedsMachine(j.Status.Stage) && len(password) == 0 {
		clear(password)
		clear(sudoPassword)
		c.JSON(http.StatusBadRequest, gin.H{"error": "stage " + j.Status.Stage + " logs in to the machine: its SSH password is required"})
		return
	}
	namespace, err := g.stationNamespace(c, j.Spec.ClusterID)
	if err != nil {
		clear(password)
		clear(sudoPassword)
		return
	}
	if user := strings.TrimSpace(request.User); user != "" && user != j.Spec.Machine.User {
		j.Spec.Machine.User = user
		if j, err = client.Update(ctx, j, metav1.UpdateOptions{}); err != nil {
			clear(password)
			clear(sudoPassword)
			respondKubeError(c, "change the machine's account: "+err.Error(), err)
			return
		}
	}
	run := &nodeJoinRun{g: g, name: j.Name, namespace: namespace}
	run.password, run.sudoPassword = password, sudoPassword
	if !g.startNodeJoin(ctx, run, j.Status.Stage) {
		c.JSON(http.StatusConflict, gin.H{"error": "the join of " + j.Spec.NodeName + " is running"})
		return
	}
	transferred = true
	c.JSON(http.StatusAccepted, gin.H{"data": j})
}

// clearTunnelScript takes away the tunnel a join started on the machine; its key and the installer stay, so that a
// join that starts over finds the same machine.
const clearTunnelScript = `set -eu
systemctl disable --now rlark-station-gateway >/dev/null 2>&1 || true
rm -f /etc/systemd/system/rlark-station-gateway.service /etc/rlark/station/bootstrap.json /var/lib/station/wg/last-applied.json
ip link del st-wg0 2>/dev/null || true
systemctl daemon-reload
echo cleared=yes
`

// handleClearNodeJoin answers POST /api/v1/station/node-joins/:name/clear: a join that is not running is taken away,
// with what it left. Before its code's line ran: its declaration as a hub, and with the password the tunnel it
// started on the machine. After: the node's name is removed as a removal by name does (a NodeOperation Remove), which
// retires its site node record and deletes its admission or its Node. It answers what it did, a line a step.
func (g *Gateway) handleClearNodeJoin(c *gin.Context) {
	var request nodeJoinRequest
	if !decodeOneJSON(c, maxNodeAdmissionRequestBytes, &request, true) {
		return
	}
	password := []byte(request.Password)
	sudoPassword := []byte(request.SudoPassword)
	request.Password = ""
	request.SudoPassword = ""
	defer clear(password)
	defer clear(sudoPassword)
	ctx := c.Request.Context()
	client := g.kubeClient.RlinfV1alpha1().NodeJoins()
	j, err := client.Get(ctx, c.Param("name"), metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, "get node join: "+err.Error(), err)
		return
	}
	namespace, err := g.stationNamespace(c, j.Spec.ClusterID)
	if err != nil {
		return
	}
	reached := func(stage string) bool {
		return slices.ContainsFunc(j.Status.Stages, func(s rlarkv1alpha1.ClusterProvisionStage) bool {
			return s.Name == stage && s.State != rlarkv1alpha1.ClusterProvisionStagePending && s.State != rlarkv1alpha1.ClusterProvisionStageSkipped
		})
	}
	if j.Status.Phase == rlarkv1alpha1.ClusterProvisionRunning || !g.nodeJoins.start(j.Name) {
		c.JSON(http.StatusConflict, gin.H{"error": "the join of " + j.Spec.NodeName + " is running"})
		return
	}
	defer g.nodeJoins.end(j.Name)
	if user := strings.TrimSpace(request.User); user != "" {
		j.Spec.Machine.User = user
	}
	// A join that succeeded leaves its node in the cluster: only its record goes (a node is removed on its own page).
	var steps []string
	switch {
	case j.Status.Phase == rlarkv1alpha1.ClusterProvisionSucceeded:
	case reached(stageInstall):
		steps, err = g.removeJoinedName(ctx, namespace, j, c.GetString(authSubjectKey))
	default:
		steps, err = g.clearUnredeemedJoin(ctx, j, password, sudoPassword)
	}
	if err == nil {
		if err = g.clearNodeJoinMachineAdmission(ctx, j); err == nil && j.Annotations[nodeJoinMachineAnnotation] != "" {
			steps = append(steps, "cleared the scanned Machine admission")
		}
	}
	if err == nil {
		if err = client.Delete(ctx, j.Name, metav1.DeleteOptions{}); err == nil || apierrors.IsNotFound(err) {
			err = nil
			steps = append(steps, "删掉这条 SSH 入网记录")
		}
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "steps": steps})
		return
	}
	c.JSON(http.StatusOK, gin.H{"steps": steps})
}

// clearUnredeemedJoin takes away what a join left before its code's line ran: the tunnel on the machine, when the
// password is given, and its declaration of the machine as a hub.
func (g *Gateway) clearUnredeemedJoin(ctx context.Context, j *rlarkv1alpha1.NodeJoin, password, sudoPassword []byte) ([]string, error) {
	var steps []string
	machineTouched := slices.ContainsFunc(j.Status.Stages, func(s rlarkv1alpha1.ClusterProvisionStage) bool {
		return s.Name == stageTunnel && (s.State == rlarkv1alpha1.ClusterProvisionStageDone || s.State == rlarkv1alpha1.ClusterProvisionStageFailed || s.State == rlarkv1alpha1.ClusterProvisionStageRunning)
	})
	switch {
	case machineTouched && len(password) > 0:
		machine, _, err := g.dialNodeJoinMachine(ctx, j, password, sudoPassword)
		if err != nil {
			return steps, fmt.Errorf("登录 %s@%s：%w", j.Spec.Machine.User, j.Spec.Machine.Address, err)
		}
		_, err = machine.Run(ctx, clearTunnelScript, nil, nil)
		_ = machine.Close()
		if err != nil {
			return steps, fmt.Errorf("撤掉机器上的隧道：%w", err)
		}
		steps = append(steps, "机器 "+j.Spec.Machine.Address+"：撤掉这次建的隧道；隧道密钥和安装包留在机器上")
	case machineTouched:
		steps = append(steps, "机器 "+j.Spec.Machine.Address+" 没动（没给口令或已不在）：它上面的隧道服务还在")
	}
	if !j.Status.Facts.DeclaredHub {
		return steps, nil
	}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		network, err := g.provisionNetwork(ctx)
		if err != nil {
			return err
		}
		nodes, _, _ := unstructured.NestedSlice(network.object.Object, "spec", "nodes")
		nodes = slices.DeleteFunc(nodes, func(raw interface{}) bool {
			entry, _ := raw.(map[string]interface{})
			return entry["clusterID"] == j.Spec.ClusterID && entry["nodeName"] == j.Spec.NodeName
		})
		if err := unstructured.SetNestedSlice(network.object.Object, nodes, "spec", "nodes"); err != nil {
			return err
		}
		_, err = g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Update(ctx, network.object, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return steps, fmt.Errorf("从 SiteNetwork 里撤掉这台机器的 Hub 声明：%w", err)
	}
	return append(steps, fmt.Sprintf("SiteNetwork：撤掉 %s 的 Hub 声明（隧道地址 %s 放出）", j.Spec.NodeName, j.Status.Facts.TunnelAddress)), nil
}

// removeJoinedName removes the node's name as the console's "start over" of an admission does: a NodeOperation
// Remove without a UID, which station-controller carries out (the Node its cluster has, or what RLark keeps of the
// name: the site node record, the admission).
func (g *Gateway) removeJoinedName(ctx context.Context, namespace string, j *rlarkv1alpha1.NodeJoin, actor string) ([]string, error) {
	requestID := "clear-" + j.Spec.NodeName + "-" + strconv.FormatInt(time.Now().Unix(), 36)
	name := nodeOperationName(j.Spec.ClusterID, requestID)
	spec := map[string]any{"requestID": requestID, "action": "Remove", "clusterID": j.Spec.ClusterID, "nodeName": j.Spec.NodeName, "requestedBy": actor}
	operation := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeOperation",
		"metadata": map[string]any{"name": name, "namespace": namespace}, "spec": spec}}
	if _, err := g.stationClient.Resource(nodeOperationGVR).Namespace(namespace).Create(ctx, operation, metav1.CreateOptions{}); err != nil {
		return nil, fmt.Errorf("提交删除节点 %s：%w", j.Spec.NodeName, err)
	}
	return []string{fmt.Sprintf("提交删除节点 %s（%s）：集群里有这个节点就删掉它；没有就退役它的场站节点记录、删掉入网许可，放出公钥和隧道地址", j.Spec.NodeName, name)}, nil
}

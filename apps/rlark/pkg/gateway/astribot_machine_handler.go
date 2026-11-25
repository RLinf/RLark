package gateway

import (
	"context"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
)

var machineGVR = schema.GroupVersionResource{Group: "station.rlark.io", Version: "v1alpha1", Resource: "machines"}

// machineNamespace is where the Machines are: with the SiteNetwork.
const machineNamespace = "station-system"

// machine is a machine RLark knows of as an administrator reads it: what was
// seen of it and how its join goes. Nothing of any account is in it.
type machine struct {
	Name         string `json:"name"`
	Site         string `json:"site"`
	NodeClass    string `json:"node_class"`
	MachineType  string `json:"machine_type"`
	MachineModel string `json:"machine_model,omitempty"`
	Source       string `json:"source"`
	// Phase is Discovered (seen, not a node), Admitted (its join is under
	// way) or Joined; Problem names what is wrong with it, if anything.
	Phase   string `json:"phase"`
	Problem string `json:"problem,omitempty"`
	Message string `json:"message,omitempty"`
	Serial  string `json:"serial,omitempty"`
	// InspectedAt is when RLark last logged in to the machine's entry device
	// and reached the computer behind it, whose host key is NodeHostKeyFingerprint.
	InspectedAt            string `json:"inspected_at,omitempty"`
	NodeHostKeyFingerprint string `json:"node_host_key_fingerprint,omitempty"`
	HostKeyFingerprint     string `json:"host_key,omitempty"`
	Port                   int64  `json:"port,omitempty"`
	HostKeyAlgorithm       string `json:"host_key_algorithm,omitempty"`
	Banner                 string `json:"banner,omitempty"`
	Address                string `json:"address,omitempty"`
	MAC                    string `json:"mac,omitempty"`
	// LastSeen is when a hub of its site last saw the machine.
	LastSeen  string `json:"last_seen,omitempty"`
	ClusterID string `json:"cluster_id,omitempty"`
	NodeName  string `json:"node_name,omitempty"`
	// Admissible says whether the machine can be admitted now: one that is
	// not a node, was read and has no problem, or one whose join stopped.
	Admissible bool `json:"admissible"`
	// Joining says that an administrator's request for the machine is being
	// acted on: it is not admitted again until its join succeeds or stops.
	Joining     bool   `json:"joining"`
	RequestedBy string `json:"requested_by,omitempty"`
	RequestedAt string `json:"requested_at,omitempty"`
	// AdmitClusterID is the cluster an administrator admitted the machine to.
	AdmitClusterID string `json:"admit_cluster_id,omitempty"`
	// Join is where the NodeOperation of the administrator's present request
	// stands: why it stopped, when it did, is the machine's Message; one that
	// succeeded while the machine is Discovered again made a node removed since,
	// and the request is spent.
	Join             *machineJoin           `json:"join,omitempty"`
	NodeJoin         *machineNodeJoin       `json:"node_join,omitempty"`
	ObservedIdentity map[string]interface{} `json:"observed_identity,omitempty"`
	IdentityVerified string                 `json:"identity_verified,omitempty"`
	IdentityReason   string                 `json:"identity_reason,omitempty"`
}

type machineJoin struct {
	Phase string `json:"phase,omitempty"`
	// Stage is the last stage it completed.
	Stage        string `json:"stage,omitempty"`
	Reason       string `json:"reason,omitempty"`
	MachineType  string `json:"machine_type,omitempty"`
	MachineModel string `json:"machine_model,omitempty"`
}

type machineNodeJoin struct {
	Phase  string `json:"phase,omitempty"`
	Stage  string `json:"stage,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// machineOperationLabel is on the NodeOperation that joins a Machine and names
// it, machineRequestAnnotation the Machine's generation it was started for
// (station-controller, MachineLabel and machineRequestAnnotation).
const (
	machineOperationLabel    = "station.rlark.io/machine"
	machineRequestAnnotation = "station.rlark.io/machine-generation"
)

// machineClass is what the list shows of a NodeClass: the labels a machine of
// it gets, whose values an administrator may replace for one machine, and
// which of its passwords have been entered.
type machineClass struct {
	Kind                string                 `json:"kind,omitempty"`
	Model               string                 `json:"model,omitempty"`
	Labels              map[string]string      `json:"labels,omitempty"`
	Taints              []string               `json:"taints,omitempty"`
	Naming              map[string]interface{} `json:"naming,omitempty"`
	HasDiscovery        bool                   `json:"hasDiscovery"`
	HostKeyFingerprints []string               `json:"hostKeyFingerprints,omitempty"`
	Access              *machineClassAccess    `json:"access,omitempty"`
}

type machineClassAccess struct {
	Entry machineCredentialStatus  `json:"entry"`
	Node  machineCredentialStatus  `json:"node"`
	Sudo  *machineCredentialStatus `json:"sudo,omitempty"`
}

type machineCredentialStatus struct {
	Configured bool `json:"configured"`
}

type admitMachinesRequest struct {
	ClusterID    string   `json:"cluster_id"`
	Machines     []string `json:"machines"`
	NodeClass    string   `json:"node_class,omitempty"`
	MachineType  string   `json:"machine_type,omitempty"`
	MachineModel string   `json:"machine_model,omitempty"`
	Model        string   `json:"model,omitempty"`
	// Labels replace, for these machines, the values of labels their class declares.
	Labels map[string]string `json:"labels,omitempty"`
}

type admittedMachine struct {
	Name     string `json:"name"`
	Admitted bool   `json:"admitted"`
	Error    string `json:"error,omitempty"`
}

// machineView is a Machine as the list shows it, with the join of its
// present request among joins (machineJoins).
func machineView(object *unstructured.Unstructured, joins map[string]*machineJoin, classViews ...map[string]machineClass) machine {
	classes := map[string]machineClass{}
	if len(classViews) > 0 && classViews[0] != nil {
		classes = classViews[0]
	}
	text := func(fields ...string) string {
		value, _, _ := unstructured.NestedString(object.Object, fields...)
		return value
	}
	nodeClass, _, _ := unstructured.NestedString(object.Object, "spec", "nodeClass")
	classKind, classModel := machineClassIdentity(classes, nodeClass)
	view := machine{
		Name: object.GetName(), Site: text("spec", "site"), NodeClass: nodeClass, Source: text("spec", "source"),
		Phase: text("status", "phase"), Problem: text("status", "problem"), Message: text("status", "message"), Serial: text("status", "serial"),
		InspectedAt: text("status", "inspectedAt"), NodeHostKeyFingerprint: text("status", "nodeHostKeyFingerprint"), HostKeyFingerprint: text("status", "entry", "hostKeyFingerprint"),
		Address: text("status", "entry", "address"), MAC: text("status", "entry", "mac"), LastSeen: text("status", "entry", "lastSeen"),
		Port: nestedInt64(object.Object, "status", "entry", "port"), HostKeyAlgorithm: text("status", "entry", "hostKeyAlgorithm"), Banner: text("status", "entry", "banner"),
		ClusterID: text("status", "clusterID"), NodeName: text("status", "nodeName"),
		RequestedBy: text("spec", "admit", "requestedBy"), RequestedAt: text("spec", "admit", "requestedAt"), AdmitClusterID: text("spec", "admit", "clusterID"),
		Join: joins[object.GetName()],
	}
	view.MachineModel = text("spec", "admit", "model")
	if view.MachineModel == "" {
		view.MachineModel, _, _ = unstructured.NestedString(object.Object, "status", "observedIdentity", "model")
	}
	view.ObservedIdentity, _, _ = unstructured.NestedMap(object.Object, "status", "observedIdentity")
	view.IdentityVerified = text("status", "identityVerified")
	view.IdentityReason = text("status", "identityReason")
	if nodeJoin, found, _ := unstructured.NestedMap(object.Object, "status", "nodeJoin"); found {
		view.NodeJoin = &machineNodeJoin{Phase: stringValue(nodeJoin["phase"]), Stage: stringValue(nodeJoin["stage"]), Reason: stringValue(nodeJoin["reason"])}
	}
	if view.NodeJoin == nil {
		if join := joins[object.GetName()]; join != nil {
			view.NodeJoin = &machineNodeJoin{Phase: join.Phase, Stage: join.Stage, Reason: join.Reason}
		}
	}
	if view.MachineModel == "" {
		view.MachineModel = classModel
	}
	view.MachineType = text("spec", "admit", "type")
	if machine, found, _ := unstructured.NestedMap(object.Object, "spec", "admit", "machine"); found && view.MachineType == "" {
		view.MachineType = stringValue(machine["category"])
	}
	if view.MachineType == "" {
		view.MachineType = text("status", "observedIdentity", "category")
	}
	if view.NodeJoin != nil && view.MachineType == "" {
		if join := joins[object.GetName()]; join != nil {
			view.MachineType, view.MachineModel = join.MachineType, join.MachineModel
		}
	}
	if view.MachineType == "" {
		view.MachineType = classKind
	}
	if view.MachineType == "" {
		if view.NodeClass != "" {
			view.MachineType = "Robot"
		} else {
			view.MachineType = "Unknown"
		}
	}
	// A request is acted on from when it is written until the join it starts succeeds or stops; station-controller
	// may first read the machine, and retries a fault that is not the machine's.
	view.Joining = view.Phase != "Joined" && view.AdmitClusterID != "" && (view.Join == nil || view.Join.Phase != "Succeeded" && view.Join.Phase != "Blocked" && view.Join.Phase != "Failed")
	// Read means its entry device let RLark in and the computer behind it answered.
	entryCandidate := view.Phase == "Discovered" && view.Problem == "" && view.NodeClass == "" && view.Source == "Discovery" && view.Address != "" && text("status", "entry", "hostKeyFingerprint") != "" && view.HostKeyAlgorithm != ""
	view.Admissible = !view.Joining && (view.Serial != "" && (view.Phase == "Discovered" && view.Problem == "" && view.InspectedAt != "" && view.NodeHostKeyFingerprint != "" ||
		view.Phase == "Admitted" && view.Problem != "") || entryCandidate)
	return view
}

func stringValue(value interface{}) string { text, _ := value.(string); return text }

func nestedInt64(object map[string]interface{}, fields ...string) int64 {
	value, _, _ := unstructured.NestedInt64(object, fields...)
	return value
}

func machineClassIdentity(classes map[string]machineClass, name string) (string, string) {
	class, ok := classes[name]
	if !ok {
		return "", ""
	}
	kind, model := class.Kind, class.Model
	if model == "" {
		model = class.Labels["rlark.io/robot-model"]
	}
	if kind == "" && (model != "" || class.Labels["rlark.io/node-category"] == "robot") {
		kind = "Robot"
	}
	if kind == "" && name != "" {
		kind = "Robot"
	}
	return kind, model
}

// machineJoins are the NodeOperations that join Machines, by the Machine and
// the generation of the request each was started for.
func (g *Gateway) machineJoins(ctx context.Context) (map[string]*machineJoin, error) {
	joins, err := g.stationClient.Resource(nodeOperationGVR).Namespace("").List(ctx, metav1.ListOptions{LabelSelector: machineOperationLabel})
	if err != nil {
		return nil, err
	}
	joinOf := map[string]*machineJoin{}
	for i := range joins.Items {
		phase, _, _ := unstructured.NestedString(joins.Items[i].Object, "status", "phase")
		stage, _, _ := unstructured.NestedString(joins.Items[i].Object, "status", "stage")
		machineName := joins.Items[i].GetLabels()[machineOperationLabel]
		joinOf[machineName] = &machineJoin{Phase: phase, Stage: stage}
	}
	if g.kubeClient == nil {
		return joinOf, nil
	}
	nodeJoins, err := g.kubeClient.RlinfV1alpha1().NodeJoins().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range nodeJoins.Items {
		join := &nodeJoins.Items[i]
		machineName := join.Labels["station.rlark.io/machine"]
		if machineName == "" {
			continue
		}
		phase, stage := string(join.Status.Phase), join.Status.Stage
		annotations := join.Annotations
		if machineName != "" {
			joinOf[machineName] = &machineJoin{Phase: phase, Stage: stage, Reason: annotations["station.rlark.io/join-reason"], MachineType: annotations["station.rlark.io/machine-type"], MachineModel: annotations["station.rlark.io/machine-model"]}
		}
	}
	return joinOf, nil
}

// handleListMachines answers GET /api/v1/station/machines?site=&phase=: the
// machines RLark knows of, newest sighting first, and their classes.
func (g *Gateway) handleListMachines(c *gin.Context) {
	ctx := c.Request.Context()
	list, err := g.stationClient.Resource(machineGVR).Namespace(machineNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Machine request failed"})
		return
	}
	classList, err := g.stationClient.Resource(nodeClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "NodeClass request failed"})
		return
	}
	classes := map[string]machineClass{}
	for i := range classList.Items {
		classObject := &classList.Items[i]
		labels, _, _ := unstructured.NestedStringMap(classObject.Object, "spec", "labels")
		taints, _, _ := unstructured.NestedStringSlice(classObject.Object, "spec", "taints")
		kind, _, _ := unstructured.NestedString(classObject.Object, "spec", "kind")
		model, _, _ := unstructured.NestedString(classObject.Object, "spec", "model")
		if kind == "" && (labels["rlark.io/robot-model"] != "" || labels["rlark.io/node-category"] == "robot") {
			kind = "Robot"
		}
		discovery, hasDiscovery, _ := unstructured.NestedMap(classObject.Object, "spec", "discovery")
		fingerprints, _, _ := unstructured.NestedStringSlice(discovery, "hostKeyFingerprints")
		naming, _, _ := unstructured.NestedMap(classObject.Object, "spec", "naming")
		class := machineClass{Kind: kind, Model: model, Labels: labels, Taints: taints, Naming: naming, HasDiscovery: hasDiscovery, HostKeyFingerprints: fingerprints}
		if access, hasAccess, accessErr := g.nodeClassAccessOf(ctx, classObject); accessErr != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "NodeClass access status request failed"})
			return
		} else if hasAccess {
			class.Access = &machineClassAccess{Entry: machineCredentialStatus{Configured: access.Entry.Configured}, Node: machineCredentialStatus{Configured: access.Node.Configured}}
			if access.Sudo != nil {
				class.Access.Sudo = &machineCredentialStatus{Configured: access.Sudo.Configured}
			}
		}
		classes[classObject.GetName()] = class
	}
	joins, err := g.machineJoins(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "NodeOperation request failed"})
		return
	}
	site, phase := c.Query("site"), c.Query("phase")
	items := make([]machine, 0, len(list.Items))
	for i := range list.Items {
		if view := machineView(&list.Items[i], joins, classes); (site == "" || view.Site == site) && (phase == "" || view.Phase == phase) {
			items = append(items, view)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].LastSeen > items[j].LastSeen })
	c.JSON(http.StatusOK, gin.H{"items": items, "classes": classes})
}

// handleAdmitMachines answers POST /api/v1/station/machines/admit: the
// administrator's decision that the named machines are to be nodes of a
// cluster. It is written on each Machine (spec.admit) with who and when;
// station-controller names, admits and joins them. Each machine is answered
// for by itself: one that cannot be admitted does not hold the others back.
func (g *Gateway) handleAdmitMachines(c *gin.Context) {
	var request admitMachinesRequest
	if !decodeOneJSON(c, maxNodeAdmissionRequestBytes, &request, false) {
		return
	}
	if request.ClusterID == "" || len(request.Machines) == 0 || len(request.Machines) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_id and 1 to 500 machines are required"})
		return
	}
	if request.MachineType == "Server" || request.MachineType == "server" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server machines must use the SSH NodeJoin path"})
		return
	}
	if request.MachineType != "" && request.MachineType != "Robot" && request.MachineType != "robot" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "machine_type must be Robot; Server uses NodeJoin"})
		return
	}
	if request.MachineModel == "" {
		request.MachineModel = request.Model
	}
	for key, value := range request.Labels {
		if len(validation.IsQualifiedName(key)) > 0 || len(validation.IsValidLabelValue(value)) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "labels must be valid Kubernetes labels"})
			return
		}
	}
	ctx := c.Request.Context()
	// The cluster is a registered one; where its machines reach its API, Station API answers them.
	if _, err := g.stationNamespace(c, request.ClusterID); err != nil {
		return
	}
	joins, err := g.machineJoins(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "NodeOperation request failed"})
		return
	}
	classCache := map[string]*unstructured.Unstructured{}
	resource := g.stationClient.Resource(machineGVR).Namespace(machineNamespace)
	actor, now := c.GetString(authSubjectKey), time.Now().UTC().Format(time.RFC3339)
	answers := make([]admittedMachine, 0, len(request.Machines))
	for _, name := range request.Machines {
		answer := admittedMachine{Name: name}
		object, err := resource.Get(ctx, name, metav1.GetOptions{})
		var view machine
		if err == nil {
			view = machineView(object, joins)
		}
		switch {
		case apierrors.IsNotFound(err):
			answer.Error = "no such machine"
		case err != nil:
			answer.Error = "Machine request failed"
		case view.Joining:
			answer.Error = "the machine is being joined to cluster " + view.AdmitClusterID + " already, as " + view.RequestedBy + " asked at " + view.RequestedAt
		case !view.Admissible:
			answer.Error = "the machine cannot be admitted as it is"
		}
		if answer.Error == "" {
			declaredClass, _, _ := unstructured.NestedString(object.Object, "spec", "nodeClass")
			className := request.NodeClass
			if className == "" {
				className = declaredClass
			}
			if className == "" {
				answer.Error = "a Robot admission requires node_class; Server machines use NodeJoin"
			}
			class := classCache[className]
			if answer.Error == "" && class == nil {
				class, err = g.stationClient.Resource(nodeClassGVR).Get(ctx, className, metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						answer.Error = "no such NodeClass " + className
					} else {
						answer.Error = "NodeClass request failed"
					}
				} else {
					classCache[className] = class
				}
			}
			classLabels := map[string]string{}
			classModel, classKind := "", ""
			if class != nil {
				classLabels, _, _ = unstructured.NestedStringMap(class.Object, "spec", "labels")
				classModel, _, _ = unstructured.NestedString(class.Object, "spec", "model")
				classKind, _, _ = unstructured.NestedString(class.Object, "spec", "kind")
				if classModel == "" {
					classModel = classLabels["rlark.io/robot-model"]
				}
				if classKind == "" && (classModel != "" || classLabels["rlark.io/node-category"] == "robot") {
					classKind = "Robot"
				}
			}
			if answer.Error == "" && classKind != "" && classKind != "Robot" {
				answer.Error = "NodeClass " + className + " is not a Robot profile"
			}
			if answer.Error == "" && declaredClass != "" && declaredClass != className {
				answer.Error = "the selected NodeClass does not match this candidate"
			}
			fingerprints := []string{}
			if class != nil {
				fingerprints, _, _ = unstructured.NestedStringSlice(class.Object, "spec", "discovery", "hostKeyFingerprints")
			}
			candidateKey, _, _ := unstructured.NestedString(object.Object, "status", "entry", "hostKeyFingerprint")
			if answer.Error == "" && (candidateKey == "" || !slices.Contains(fingerprints, candidateKey)) {
				answer.Error = "the candidate host key is not declared by the selected Robot class"
			}
			model := request.MachineModel
			if model == "" {
				model = request.Model
			}
			if model == "" {
				model = classModel
			}
			if answer.Error == "" && classModel != "" && model != classModel {
				answer.Error = "the selected model does not match NodeClass " + className
			}
			if answer.Error == "" && model == "" {
				answer.Error = "machine_model is required for a Robot admission"
			}
			if answer.Error == "" && classModel == "" {
				if _, modelErr := g.stationClient.Resource(robotModelGVR).Get(ctx, model, metav1.GetOptions{}); modelErr != nil {
					if apierrors.IsNotFound(modelErr) {
						answer.Error = "unknown RobotModel " + model
					} else {
						answer.Error = "RobotModel request failed"
					}
				}
			}
			admit := map[string]interface{}{"clusterID": request.ClusterID, "type": "Robot", "nodeClass": className, "model": model, "requestedBy": actor, "requestedAt": now}
			if len(request.Labels) > 0 {
				labels := map[string]interface{}{}
				for key, value := range request.Labels {
					if _, declared := classLabels[key]; !declared {
						answer.Error = "label " + key + " is not one NodeClass " + className + " declares"
					}
					labels[key] = value
				}
				admit["labels"] = labels
			}
			if answer.Error == "" {
				if err := unstructured.SetNestedMap(object.Object, admit, "spec", "admit"); err != nil {
					answer.Error = "Machine request failed"
				} else if _, err := resource.Update(ctx, object, metav1.UpdateOptions{}); apierrors.IsConflict(err) {
					answer.Error = "the machine changed meanwhile; try again"
				} else if err != nil {
					log.FromContext(ctx).Error(err, "admit Machine", "machine", name)
					answer.Error = "Machine request failed"
				}
			}
		}
		answer.Admitted = answer.Error == ""
		log.FromContext(ctx).WithName("audit").Info("machine", "action", "admit", "actor", actor, "cluster", request.ClusterID, "machine", name, "admitted", answer.Admitted)
		answers = append(answers, answer)
	}
	c.JSON(http.StatusOK, gin.H{"machines": answers})
}

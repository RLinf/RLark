package gateway

import (
	"net/http"
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
	Name      string `json:"name"`
	Site      string `json:"site"`
	NodeClass string `json:"node_class"`
	Source    string `json:"source"`
	// Phase is Discovered (seen, not a node), Admitted (its join is under
	// way) or Joined; Problem names what is wrong with it, if anything.
	Phase   string `json:"phase"`
	Problem string `json:"problem,omitempty"`
	Message string `json:"message,omitempty"`
	Serial  string `json:"serial,omitempty"`
	Address string `json:"address,omitempty"`
	MAC     string `json:"mac,omitempty"`
	// LastSeen is when a hub of its site last saw the machine.
	LastSeen  string `json:"last_seen,omitempty"`
	ClusterID string `json:"cluster_id,omitempty"`
	NodeName  string `json:"node_name,omitempty"`
	// Admissible says whether the machine can be admitted now: one that is
	// not a node and has no problem, or one whose join stopped.
	Admissible  bool   `json:"admissible"`
	RequestedBy string `json:"requested_by,omitempty"`
	RequestedAt string `json:"requested_at,omitempty"`
}

// machineClass is what the list shows of a NodeClass: the labels a machine of
// it gets, whose values an administrator may replace for one machine, and
// which of its passwords have been entered.
type machineClass struct {
	Labels map[string]string `json:"labels,omitempty"`
	Access *nodeClassAccess  `json:"access,omitempty"`
}

type admitMachinesRequest struct {
	ClusterID string   `json:"cluster_id"`
	Machines  []string `json:"machines"`
	// Labels replace, for these machines, the values of labels their class declares.
	Labels map[string]string `json:"labels,omitempty"`
}

type admittedMachine struct {
	Name     string `json:"name"`
	Admitted bool   `json:"admitted"`
	Error    string `json:"error,omitempty"`
}

func machineView(object *unstructured.Unstructured) machine {
	text := func(fields ...string) string {
		value, _, _ := unstructured.NestedString(object.Object, fields...)
		return value
	}
	view := machine{
		Name: object.GetName(), Site: text("spec", "site"), NodeClass: text("spec", "nodeClass"), Source: text("spec", "source"),
		Phase: text("status", "phase"), Problem: text("status", "problem"), Message: text("status", "message"), Serial: text("status", "serial"),
		Address: text("status", "entry", "address"), MAC: text("status", "entry", "mac"), LastSeen: text("status", "entry", "lastSeen"),
		ClusterID: text("status", "clusterID"), NodeName: text("status", "nodeName"),
		RequestedBy: text("spec", "admit", "requestedBy"), RequestedAt: text("spec", "admit", "requestedAt"),
	}
	view.Admissible = view.Serial != "" && (view.Phase == "Discovered" && view.Problem == "" || view.Phase == "Admitted" && view.Problem != "")
	return view
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
	classes, err := g.stationClient.Resource(nodeClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "NodeClass request failed"})
		return
	}
	site, phase := c.Query("site"), c.Query("phase")
	items := make([]machine, 0, len(list.Items))
	for i := range list.Items {
		if view := machineView(&list.Items[i]); (site == "" || view.Site == site) && (phase == "" || view.Phase == phase) {
			items = append(items, view)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].LastSeen > items[j].LastSeen })
	declared := map[string]machineClass{}
	for i := range classes.Items {
		labels, _, _ := unstructured.NestedStringMap(classes.Items[i].Object, "spec", "labels")
		class := machineClass{Labels: labels}
		access, ok, err := g.nodeClassAccessOf(ctx, &classes.Items[i])
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "secret request failed"})
			return
		}
		if ok {
			class.Access = &access
		}
		declared[classes.Items[i].GetName()] = class
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "classes": declared})
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
	classes := map[string]map[string]string{}
	resource := g.stationClient.Resource(machineGVR).Namespace(machineNamespace)
	actor, now := c.GetString(authSubjectKey), time.Now().UTC().Format(time.RFC3339)
	answers := make([]admittedMachine, 0, len(request.Machines))
	for _, name := range request.Machines {
		answer := admittedMachine{Name: name}
		object, err := resource.Get(ctx, name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			answer.Error = "no such machine"
		case err != nil:
			answer.Error = "Machine request failed"
		case !machineView(object).Admissible:
			answer.Error = "the machine cannot be admitted as it is"
		}
		if answer.Error == "" {
			class, _, _ := unstructured.NestedString(object.Object, "spec", "nodeClass")
			if _, read := classes[class]; !read {
				if declared, err := g.stationClient.Resource(nodeClassGVR).Get(ctx, class, metav1.GetOptions{}); err == nil {
					classes[class], _, _ = unstructured.NestedStringMap(declared.Object, "spec", "labels")
				}
			}
			admit := map[string]interface{}{"clusterID": request.ClusterID, "requestedBy": actor, "requestedAt": now}
			if len(request.Labels) > 0 {
				labels := map[string]interface{}{}
				for key, value := range request.Labels {
					if _, declared := classes[class][key]; !declared {
						answer.Error = "label " + key + " is not one NodeClass " + class + " declares"
					}
					labels[key] = value
				}
				admit["labels"] = labels
			}
			if answer.Error == "" {
				_ = unstructured.SetNestedMap(object.Object, admit, "spec", "admit")
				if _, err := resource.Update(ctx, object, metav1.UpdateOptions{}); apierrors.IsConflict(err) {
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

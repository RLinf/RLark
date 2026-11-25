package gateway

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var robotModelGVR = schema.GroupVersionResource{Group: "station.rlark.io", Version: "v1alpha1", Resource: "robotmodels"}

type robotModelReferenceCounts struct {
	NodeClasses    int `json:"nodeClasses"`
	Nodes          int `json:"nodes"`
	Machines       int `json:"machines"`
	NodeAdmissions int `json:"nodeAdmissions"`
	NodeJoins      int `json:"nodeJoins"`
}

type robotModelView struct {
	Name        string                    `json:"name"`
	DisplayName string                    `json:"displayName,omitempty"`
	Vendor      string                    `json:"vendor,omitempty"`
	Description string                    `json:"description,omitempty"`
	References  robotModelReferenceCounts `json:"references"`
}

func (g *Gateway) robotModelReferences(c *gin.Context, name string) (robotModelReferenceCounts, error) {
	ctx := c.Request.Context()
	var counts robotModelReferenceCounts
	classes, err := g.stationClient.Resource(nodeClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return counts, err
	}
	for i := range classes.Items {
		labels, _, _ := unstructured.NestedStringMap(classes.Items[i].Object, "spec", "labels")
		value, _, _ := unstructured.NestedString(classes.Items[i].Object, "spec", "model")
		if value == name || labels["rlark.io/robot-model"] == name {
			counts.NodeClasses++
		}
	}
	nodeReferences := map[string]struct{}{}
	nodes, err := g.stationClient.Resource(siteNodeGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return counts, err
	}
	for i := range nodes.Items {
		node := &nodes.Items[i]
		declared, _, _ := unstructured.NestedString(node.Object, "spec", "machine", "model")
		observed, _, _ := unstructured.NestedString(node.Object, "status", "observation", "identity", "model")
		if declared == name || observed == name || node.GetLabels()["station.rlark.io/machine-model"] == name || node.GetLabels()["rlark.io/robot-model"] == name {
			nodeReferences[node.GetNamespace()+"/"+node.GetName()] = struct{}{}
		}
	}
	enrollments, err := g.stationClient.Resource(networkEnrollmentGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return counts, err
	}
	for i := range enrollments.Items {
		enrollment := &enrollments.Items[i]
		declared, _, _ := unstructured.NestedString(enrollment.Object, "spec", "machine", "model")
		observed, _, _ := unstructured.NestedString(enrollment.Object, "status", "observation", "identity", "model")
		if declared == name || observed == name || enrollment.GetLabels()["rlark.io/robot-model"] == name {
			nodeReferences[enrollment.GetNamespace()+"/"+enrollment.GetName()] = struct{}{}
		}
	}
	counts.Nodes = len(nodeReferences)
	machines, err := g.stationClient.Resource(machineGVR).Namespace(machineNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return counts, err
	}
	for i := range machines.Items {
		machine := &machines.Items[i]
		admit, _, _ := unstructured.NestedString(machine.Object, "spec", "admit", "model")
		observed, _, _ := unstructured.NestedString(machine.Object, "status", "observedIdentity", "model")
		if admit == name || observed == name {
			counts.Machines++
		}
	}
	admissions, err := g.stationClient.Resource(nodeAdmissionGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return counts, err
	}
	for i := range admissions.Items {
		admission := &admissions.Items[i]
		model, _, _ := unstructured.NestedString(admission.Object, "spec", "machine", "model")
		if model == name {
			counts.NodeAdmissions++
		}
	}
	if g.kubeClient != nil {
		joins, err := g.kubeClient.RlinfV1alpha1().NodeJoins().List(ctx, metav1.ListOptions{})
		if err != nil {
			return counts, err
		}
		for i := range joins.Items {
			if joins.Items[i].Annotations[nodeJoinModelAnnotation] == name {
				counts.NodeJoins++
			}
		}
	}
	return counts, nil
}

func (g *Gateway) handleListRobotModels(c *gin.Context) {
	list, err := g.stationClient.Resource(robotModelGVR).List(c.Request.Context(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "RobotModel request failed"})
		return
	}
	items := make([]robotModelView, 0, len(list.Items))
	for i := range list.Items {
		object := &list.Items[i]
		counts, err := g.robotModelReferences(c, object.GetName())
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "RobotModel reference request failed"})
			return
		}
		displayName, _, _ := unstructured.NestedString(object.Object, "spec", "displayName")
		vendor, _, _ := unstructured.NestedString(object.Object, "spec", "vendor")
		description, _, _ := unstructured.NestedString(object.Object, "spec", "description")
		items = append(items, robotModelView{Name: object.GetName(), DisplayName: displayName, Vendor: vendor, Description: description, References: counts})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (g *Gateway) handleDeleteRobotModel(c *gin.Context) {
	name := c.Param("name")
	resource := g.stationClient.Resource(robotModelGVR)
	model, err := resource.Get(c.Request.Context(), name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "RobotModel not found"})
		} else {
			c.JSON(http.StatusBadGateway, gin.H{"error": "RobotModel request failed"})
		}
		return
	}
	counts, err := g.robotModelReferences(c, name)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "RobotModel reference request failed"})
		return
	}
	if counts.NodeClasses+counts.Nodes+counts.Machines+counts.NodeAdmissions+counts.NodeJoins > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "RobotModel is referenced and cannot be deleted", "references": counts})
		return
	}
	if err := resource.Delete(c.Request.Context(), model.GetName(), metav1.DeleteOptions{}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "RobotModel could not be deleted"})
		return
	}
	c.Status(http.StatusNoContent)
	c.Writer.WriteHeaderNow()
}

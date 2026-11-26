package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// A Node's robot power, its robot claim and its display name are set through their own endpoints
// (astribot_robot_power_handler.go, astribot_robot_claim_handler.go, astribot_node_display_name_handler.go), which
// check and audit them. The generic Node update and patch may not change them; each answers the request when it
// refuses.

// nodeUpdateAllowed: a Node's new object keeps the stored power, claim and display name.
func nodeUpdateAllowed(c *gin.Context, a *resourceAccessor, namespace, name string, body []byte) bool {
	existing, err := a.get(c.Request.Context(), namespace, name, metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return false
	}
	labels := any(existing).(metav1.Object).GetLabels()
	if value, _ := robotPowerIn(body); value != labels[robotPowerLabel] {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, robotPowerRefused)
		return false
	}
	if value, _ := robotLabelIn(body, robotClaimLabel); value != labels[robotClaimLabel] {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, robotClaimRefused)
		return false
	}
	if value, _ := robotLabelIn(body, stationMarkLabel); value != labels[stationMarkLabel] ||
		nodeAnnotations(body)[stationMarkAnnotation] != existing.(metav1.Object).GetAnnotations()[stationMarkAnnotation] {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, stationMarkRefused)
		return false
	}
	if value, _ := robotLabelIn(body, stationLabel); value != labels[stationLabel] {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, stationLabelRefused)
		return false
	}
	if value, _ := displayNameIn(body); value != existing.(*rlarkiov1alpha1.Node).Spec.DisplayName {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, nodeDisplayNameRefused)
		return false
	}
	return true
}

// nodePatchAllowed: a Node's patch names neither power nor claim, and names the display name only as it is.
func nodePatchAllowed(c *gin.Context, a *resourceAccessor, namespace, name string, body []byte) bool {
	if _, named := robotPowerIn(body); named {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, robotPowerRefused)
		return false
	}
	if _, named := robotLabelIn(body, robotClaimLabel); named {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, robotClaimRefused)
		return false
	}
	_, marked := robotLabelIn(body, stationMarkLabel)
	if _, annotated := nodeAnnotations(body)[stationMarkAnnotation]; marked || annotated {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, stationMarkRefused)
		return false
	}
	if _, named := robotLabelIn(body, stationLabel); named {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, stationLabelRefused)
		return false
	}
	value, named := displayNameIn(body)
	if !named {
		return true
	}
	existing, err := a.get(c.Request.Context(), namespace, name, metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return false
	}
	if value != existing.(*rlarkiov1alpha1.Node).Spec.DisplayName {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, nodeDisplayNameRefused)
		return false
	}
	return true
}

func nodeAnnotations(body []byte) map[string]string {
	var object struct {
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(body, &object)
	return object.Metadata.Annotations
}

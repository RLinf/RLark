package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	stationMarkLabel      = "rlark.io/station-mark"
	stationMarkAnnotation = "rlark.io/station-mark-set"
	stationMarkRefused    = "a station mark is managed through PUT /api/v1/rlinf.io/v1alpha1/nodes/{name}/station-mark"
)

type stationMark struct {
	Mark   string    `json:"mark"`
	Reason string    `json:"reason"`
	By     string    `json:"by"`
	At     time.Time `json:"at"`
}

func (g *Gateway) handleSetStationMark(c *gin.Context) {
	var request struct {
		Mark   *string `json:"mark"`
		Reason string  `json:"reason"`
		Holder string  `json:"holder"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Mark == nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "mark must be repair, disabled or an empty string")
		return
	}
	mark, reason := *request.Mark, strings.TrimSpace(request.Reason)
	if (mark != "" && mark != "repair" && mark != "disabled") || (mark != "" && reason == "") || len(reason) > 1000 {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "repair and disabled require a reason of up to 1000 bytes")
		return
	}
	ctx, name, namespace := c.Request.Context(), c.Param("name"), c.Query("namespace")
	if !g.authorizeTenantObjectByName(c, "nodes", namespace, name) {
		return
	}
	nodes := g.accessors["nodes"]
	actor := c.GetString(authSubjectKey)
	for attempt := 0; ; attempt++ {
		existing, err := nodes.get(ctx, namespace, name, metav1.GetOptions{})
		if err != nil {
			respondKubeError(c, err.Error(), err)
			return
		}
		node := existing.(metav1.Object)
		labels := node.GetLabels()
		if labels["rlark.io/robot-id"] == "" {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, "the node is not a robot")
			return
		}
		if claim, held := robotClaimOf(node); mark != "" && held && request.Holder != "" && claim.Holder != request.Holder {
			respondErrorWithDetails(c, http.StatusConflict, CodeRobotClaimBusy, "the robot is held by "+claim.Holder, map[string]any{"current": claim})
			return
		}
		var previous stationMark
		_ = json.Unmarshal([]byte(node.GetAnnotations()[stationMarkAnnotation]), &previous)
		if labels[stationMarkLabel] == mark && (mark == "" || previous.Reason == reason) {
			c.JSON(http.StatusOK, existing)
			return
		}
		writeLabels := map[string]any{stationMarkLabel: nil}
		annotations := map[string]any{stationMarkAnnotation: nil}
		if mark != "" {
			record, _ := json.Marshal(stationMark{Mark: mark, Reason: reason, By: actor, At: time.Now().UTC()})
			writeLabels[stationMarkLabel], annotations[stationMarkAnnotation] = mark, string(record)
			// Mark and release are one update, so a racing acquisition cannot leave a marked robot claimed.
			writeLabels[robotClaimLabel], annotations[robotClaimSetAnnotation] = nil, nil
		}
		patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{
			"resourceVersion": node.GetResourceVersion(), "labels": writeLabels, "annotations": annotations,
		}})
		result, err := nodes.patch(ctx, namespace, name, patch)
		if apierrors.IsConflict(err) && attempt == 0 {
			continue
		}
		log.FromContext(ctx).WithName("audit").Info("station mark", "actor", actor,
			"namespace", namespace, "node", name, "mark", mark, "reason", reason, "set", err == nil)
		if err != nil {
			respondCreateError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
		return
	}
}

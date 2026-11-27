package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// robotRestartAnnotation is, on a robot's management Node, the last restart
// its claim holder asked for: {id, level, holder, by, at}. The agents carry it
// to the cluster's Node (pullManagementAnnotations), where the robot's power
// service runs each new id once and reports it in rlark.io/power.restart.
const (
	robotRestartAnnotation = "rlark.io/robot-restart"
	robotRestartRefused    = "a robot's restart (annotation " + robotRestartAnnotation + ") is asked for with POST /api/v1/rlinf.io/v1alpha1/nodes/{name}/robot-restart"
)

type robotRestart struct {
	ID     string    `json:"id"`
	Level  string    `json:"level"`
	Holder string    `json:"holder"`
	By     string    `json:"by"`
	At     time.Time `json:"at"`
}

// handleRobotRestart answers POST /api/v1/rlinf.io/v1alpha1/nodes/{name}/robot-restart:
// the Run that holds the robot asks it to restart its driver (level driver:
// the driver stopped, the arms let sink, the driver started) or also its
// motors (level power: powered off between, as long as the vendor asks).
// The desired power stays as it is. 409 unless the robot's claim names the
// holder, was made by the caller (or the caller is an operator) and its
// station is not marked. The same request_id asked again is
// answered with the request it already is.
func (g *Gateway) handleRobotRestart(c *gin.Context) {
	var request struct {
		Level     string `json:"level"`
		Holder    string `json:"holder"`
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Level != "driver" && request.Level != "power" || request.Holder == "" ||
		request.RequestID == "" || len(validation.IsValidLabelValue(request.RequestID)) != 0 {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "level must be driver or power, holder the claim's holder and request_id a label value of 1 to 63 characters")
		return
	}
	ctx, name, namespace, actor := c.Request.Context(), c.Param("name"), c.Query("namespace"), c.GetString(authSubjectKey)
	nodes := g.accessors["nodes"]
	for attempt := 0; ; attempt++ {
		existing, err := nodes.get(ctx, namespace, name, metav1.GetOptions{})
		if err != nil {
			respondKubeError(c, err.Error(), err)
			return
		}
		node := existing.(metav1.Object)
		if mark := node.GetLabels()[stationMarkLabel]; mark != "" {
			respondErrorWithDetails(c, http.StatusConflict, CodeRobotClaimBusy, "the station is marked "+mark, map[string]any{"mark": mark})
			return
		}
		if claim, held := robotClaimOf(node); !held || claim.Holder != request.Holder || claim.By != actor && !isOperator(c) {
			var current any
			if held {
				current = claim
			}
			respondErrorWithDetails(c, http.StatusConflict, CodeRobotClaimBusy, "the robot is not held by "+request.Holder+" for "+actor, map[string]any{"current": current})
			return
		}
		var previous robotRestart
		_ = json.Unmarshal([]byte(node.GetAnnotations()[robotRestartAnnotation]), &previous)
		if previous.ID == request.RequestID && previous.Holder == request.Holder {
			c.JSON(http.StatusOK, previous)
			return
		}
		restart := robotRestart{ID: request.RequestID, Level: request.Level, Holder: request.Holder, By: actor, At: time.Now().UTC().Truncate(time.Second)}
		record, _ := json.Marshal(restart)
		patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{
			"resourceVersion": node.GetResourceVersion(), "annotations": map[string]string{robotRestartAnnotation: string(record)},
		}})
		_, err = nodes.patch(ctx, namespace, name, patch)
		if apierrors.IsConflict(err) && attempt == 0 {
			continue
		}
		log.FromContext(ctx).WithName("audit").Info("robot restart", "actor", actor, "namespace", namespace, "node", name,
			"holder", restart.Holder, "level", restart.Level, "id", restart.ID, "set", err == nil)
		if err != nil {
			respondCreateError(c, err)
			return
		}
		c.JSON(http.StatusOK, restart)
		return
	}
}

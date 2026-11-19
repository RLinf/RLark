package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// robotPowerLabel is, on a robot's management Node, the power an
	// administrator wants it to have: off, powered or ready, and when that
	// was set. The agents carry it to the cluster's Node, where the robot's
	// model Addon converges on it.
	robotPowerLabel = "rlark.io/robot-power"
	// robotPowerSetAnnotation says beside it who set it last, and when.
	robotPowerSetAnnotation = "rlark.io/robot-power-set"
)

var robotPowerStates = []string{"off", "powered", "ready"}

type setRobotPowerRequest struct {
	Desired string `json:"desired"`
}

// handleSetRobotPower answers PUT /api/v1/rlinf.io/v1alpha1/nodes/{name}/robot-power:
// the one way a robot's desired power is set. It writes the label, with the
// time so that the same state set again is a new value, and who set it; every
// call is in the audit log with its actor. The Node's own PATCH and PUT
// refuse the label, so that no change of it goes unrecorded.
func (g *Gateway) handleSetRobotPower(c *gin.Context) {
	var request setRobotPowerRequest
	if err := c.ShouldBindJSON(&request); err != nil || !slices.Contains(robotPowerStates, request.Desired) {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "desired must be off, powered or ready")
		return
	}
	ctx, name, namespace := c.Request.Context(), c.Param("name"), c.Query("namespace")
	if !g.authorizeTenantObjectByName(c, "nodes", namespace, name) {
		return
	}
	nodes := g.accessors["nodes"]
	existing, err := nodes.get(ctx, namespace, name, metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}
	was := any(existing).(metav1.Object).GetLabels()[robotPowerLabel]
	actor, now := c.GetString(authSubjectKey), time.Now().UTC()
	record, _ := json.Marshal(map[string]string{"desired": request.Desired, "by": actor, "at": now.Format(time.RFC3339)})
	patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{
		"labels":      map[string]string{robotPowerLabel: fmt.Sprintf("%s.%d", request.Desired, now.Unix())},
		"annotations": map[string]string{robotPowerSetAnnotation: string(record)},
	}})
	result, err := nodes.patch(ctx, namespace, name, patch)
	log.FromContext(ctx).WithName("audit").Info("robot power", "action", "set", "actor", actor, "namespace", namespace, "node", name,
		"desired", request.Desired, "was", was, "set", err == nil)
	if err != nil {
		respondCreateError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func robotPowerIn(body []byte) (value string, named bool) { return robotLabelIn(body, robotPowerLabel) }

// robotLabelIn reads one of a robot's labels (its desired power, its claim)
// out of a Node's patch or new object: its value (empty where the body takes
// the label away) and whether the body names the label at all.
func robotLabelIn(body []byte, label string) (value string, named bool) {
	var object struct {
		Metadata struct {
			Labels map[string]*string `json:"labels"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(body, &object)
	set, named := object.Metadata.Labels[label]
	if set != nil {
		value = *set
	}
	return value, named
}

const robotPowerRefused = "a robot's desired power (label " + robotPowerLabel + ") is set with PUT /api/v1/rlinf.io/v1alpha1/nodes/{name}/robot-power"

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// The passwords every machine of a NodeClass has are values of job secrets:
// the class names each by secret and key (access.entry.password,
// access.node.password, access.node.sudoPassword) and station-controller
// reads them where it logs in. The node's passwords are entered here, by an
// administrator, once for the class. What is entered is never answered: a
// read says whether a value is there, and who entered it when.

// nodeClassPasswordsSetAnnotation says, on the NodeClass, who entered the
// node's passwords and when. Writing it is also what has station-controller
// look at the class's machines anew.
const nodeClassPasswordsSetAnnotation = "station.rlark.io/node-passwords-set"

// nodeClassPassword is one password of a class as an administrator reads it.
type nodeClassPassword struct {
	Secret     string `json:"secret"`
	Key        string `json:"key"`
	Configured bool   `json:"configured"`
	SetBy      string `json:"set_by,omitempty"`
	SetAt      string `json:"set_at,omitempty"`
}

type nodeClassAccess struct {
	// Entry is the entry device's password, entered with its secret.
	Entry nodeClassPassword `json:"entry"`
	// Node is the password of the computer that becomes the node.
	Node nodeClassPassword `json:"node"`
	// Sudo is there where the class names another password for sudo.
	Sudo *nodeClassPassword `json:"sudo,omitempty"`
}

type setNodeClassAccessRequest struct {
	NodePassword string `json:"node_password"`
	// SudoPassword is given where the node's sudo asks for another password.
	SudoPassword string `json:"sudo_password,omitempty"`
}

// nodeClassAccessOf reads which passwords of a class are there. ok is false
// for a class that declares no access.
func (g *Gateway) nodeClassAccessOf(ctx context.Context, class *unstructured.Unstructured) (access nodeClassAccess, ok bool, err error) {
	secrets := map[string]map[string]string{}
	password := func(fields ...string) (nodeClassPassword, error) {
		value, _, _ := unstructured.NestedStringMap(class.Object, fields...)
		read := nodeClassPassword{Secret: value["secret"], Key: value["key"]}
		if read.Secret == "" {
			return read, nil
		}
		if _, known := secrets[read.Secret]; !known {
			declaration, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Get(ctx, jobSecretReplicationName(read.Secret), metav1.GetOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				return read, err
			}
			secrets[read.Secret] = nil
			if err == nil {
				request, err := readJobSecret(declaration)
				if err != nil {
					return read, err
				}
				secrets[read.Secret] = request.Data
			}
		}
		read.Configured = secrets[read.Secret][read.Key] != ""
		return read, nil
	}
	if access.Entry, err = password("spec", "access", "entry", "password"); err != nil || access.Entry.Secret == "" {
		return access, false, err
	}
	if access.Node, err = password("spec", "access", "node", "password"); err != nil {
		return access, false, err
	}
	sudo, err := password("spec", "access", "node", "sudoPassword")
	if err != nil {
		return access, false, err
	}
	var set struct{ By, At string }
	_ = json.Unmarshal([]byte(class.GetAnnotations()[nodeClassPasswordsSetAnnotation]), &set)
	if access.Node.Configured {
		access.Node.SetBy, access.Node.SetAt = set.By, set.At
	}
	if sudo.Secret != "" {
		if sudo.Configured {
			sudo.SetBy, sudo.SetAt = set.By, set.At
		}
		access.Sudo = &sudo
	}
	return access, true, nil
}

// handleGetNodeClassAccess answers GET /api/v1/station/node-classes/{name}/access.
func (g *Gateway) handleGetNodeClassAccess(c *gin.Context) {
	class, err := g.stationClient.Resource(nodeClassGVR).Get(c.Request.Context(), c.Param("name"), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such NodeClass"})
		return
	} else if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "NodeClass request failed"})
		return
	}
	access, declared, err := g.nodeClassAccessOf(c.Request.Context(), class)
	if err != nil {
		log.FromContext(c.Request.Context()).Error(err, "read the passwords of a NodeClass", "class", class.GetName())
		c.JSON(http.StatusBadGateway, gin.H{"error": "secret request failed"})
		return
	}
	if !declared {
		c.JSON(http.StatusConflict, gin.H{"error": "the NodeClass declares no access"})
		return
	}
	c.JSON(http.StatusOK, access)
}

// handleSetNodeClassAccess answers PUT /api/v1/station/node-classes/{name}/access:
// the password of the computers of the class and, where sudo asks for
// another, that one. Each becomes the value the class names, in a job secret
// that exists; the secret's other values stay as they are. The sudo password
// of a class that names none is kept beside the node's password, and the
// class then names it.
func (g *Gateway) handleSetNodeClassAccess(c *gin.Context) {
	var request setNodeClassAccessRequest
	if !decodeOneJSON(c, maxNodeAdmissionRequestBytes, &request, false) {
		return
	}
	for _, value := range []string{request.NodePassword, request.SudoPassword} {
		if len(value) > 256 || strings.ContainsAny(value, "\r\n\x00") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a password is one line of at most 256 characters"})
			return
		}
	}
	if request.NodePassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_password is required"})
		return
	}
	ctx, name, actor := c.Request.Context(), c.Param("name"), c.GetString(authSubjectKey)
	classes := g.stationClient.Resource(nodeClassGVR)
	class, err := classes.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such NodeClass"})
		return
	} else if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "NodeClass request failed"})
		return
	}
	node, _, _ := unstructured.NestedStringMap(class.Object, "spec", "access", "node", "password")
	if node["secret"] == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "the NodeClass declares no access"})
		return
	}
	writes := map[string]map[string]string{node["secret"]: {node["key"]: request.NodePassword}}
	patch := map[string]interface{}{}
	if request.SudoPassword != "" {
		sudo, _, _ := unstructured.NestedStringMap(class.Object, "spec", "access", "node", "sudoPassword")
		if sudo["secret"] == "" {
			sudo = map[string]string{"secret": node["secret"], "key": node["key"] + "_sudo"}
			patch["spec"] = map[string]interface{}{"access": map[string]interface{}{"node": map[string]interface{}{"sudoPassword": sudo}}}
		}
		if writes[sudo["secret"]] == nil {
			writes[sudo["secret"]] = map[string]string{}
		}
		writes[sudo["secret"]][sudo["key"]] = request.SudoPassword
	}
	secrets := g.rawClient.CoreV1().Secrets(g.managementNamespace())
	for secret, values := range writes {
		declaration, err := secrets.Get(ctx, jobSecretReplicationName(secret), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "secret " + secret + ", which the NodeClass names, does not exist; create it with the entry device's password first"})
			return
		}
		var stored jobSecretRequest
		if err == nil {
			stored, err = readJobSecret(declaration)
		}
		if err == nil {
			for key, value := range values {
				stored.Data[key] = value
			}
			err = putJobSecret(ctx, g.rawClient, g.managementNamespace(), actor, stored, true)
		}
		if err != nil {
			log.FromContext(ctx).Error(err, "write the passwords of a NodeClass", "class", name, "secret", secret)
			c.JSON(http.StatusBadGateway, gin.H{"error": "secret request failed"})
			return
		}
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		log.FromContext(ctx).WithName("audit").Info("node class access", "action", "set", "actor", actor, "class", name, "secret", secret, "keys", strings.Join(keys, ","))
	}
	set, _ := json.Marshal(map[string]string{"by": actor, "at": time.Now().UTC().Format(time.RFC3339)})
	patch["metadata"] = map[string]interface{}{"annotations": map[string]interface{}{nodeClassPasswordsSetAnnotation: string(set)}}
	body, _ := json.Marshal(patch)
	class, err = classes.Patch(ctx, name, types.MergePatchType, body, metav1.PatchOptions{})
	if err != nil {
		log.FromContext(ctx).Error(err, "record who entered the passwords of a NodeClass", "class", name)
		c.JSON(http.StatusBadGateway, gin.H{"error": "NodeClass request failed"})
		return
	}
	access, _, err := g.nodeClassAccessOf(ctx, class)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "secret request failed"})
		return
	}
	c.JSON(http.StatusOK, access)
}

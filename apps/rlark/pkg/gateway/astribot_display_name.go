package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// displayNamePattern is the character class of the CRDs' spec.displayName
// and of the console's Job names: letters, digits, CJK, '.', '_' and '-'.
var displayNamePattern = regexp.MustCompile(`^[A-Za-z0-9一-龥._-]+$`)

const displayNameMaxLength = 64

// displayNameMu holds a rename from its uniqueness check to its write, so two
// renames at the same time cannot both claim one name. Renames are rare and
// the gateway runs as one replica.
var displayNameMu sync.Mutex

// errDisplayNameUsed is a display name another object already shows, as its
// display name or as its metadata.name.
var errDisplayNameUsed = errors.New("display name already used")

// validateDisplayName binds {displayName} and answers 400 for a name the CRD
// would refuse. The name is trimmed; an empty one clears the display name.
func validateDisplayName(c *gin.Context) (string, bool) {
	var request struct {
		DisplayName string `json:"displayName"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return "", false
	}
	name := strings.TrimSpace(request.DisplayName)
	if name != "" && (utf8.RuneCountInString(name) > displayNameMaxLength || !displayNamePattern.MatchString(name)) {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "displayName must be at most 64 letters, digits, Chinese characters, '.', '_' or '-'")
		return "", false
	}
	return name, true
}

// displayNameIn reads spec.displayName out of a Node's patch or new object:
// its value (empty where the body takes it away) and whether the body names it.
func displayNameIn(body []byte) (value string, named bool) {
	var object struct {
		Spec map[string]json.RawMessage `json:"spec"`
	}
	_ = json.Unmarshal(body, &object)
	raw, named := object.Spec["displayName"]
	_ = json.Unmarshal(raw, &value)
	return value, named
}

const nodeDisplayNameRefused = "a Node's spec.displayName is set with PUT /api/v1/rlinf.io/v1alpha1/nodes/{name}/display-name"

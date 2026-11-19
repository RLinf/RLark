package auth

import (
	"slices"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
)

type permissionChecker struct{}

// IsAdmin reports whether admin.
func (permissionChecker) IsAdmin(certMeta map[string]string) bool {
	if certMeta == nil {
		return false
	}
	return certMeta[apis.MetaPermissionAdmin] == "true"
}

// HasAgentProxyPermission reports whether agentProxyPermission exists.
func (p permissionChecker) HasAgentProxyPermission(certMeta map[string]string, agentID string) bool {
	if certMeta == nil {
		return false
	}
	if p.IsAdmin(certMeta) {
		return true
	}
	proxyAgents := certMeta[apis.MetaPermissionAgentProxy]
	splitAgents := strings.Split(proxyAgents, ",")
	return slices.Contains(splitAgents, "*") || slices.Contains(splitAgents, agentID)
}

// HasDomainProxyPermission reports whether domainProxyPermission exists.
func (p permissionChecker) HasDomainProxyPermission(certMeta map[string]string) (string, bool) {
	if certMeta == nil {
		return "", false
	}
	domainID := certMeta[apis.MetaDomainID]
	if domainID == "" {
		return "", false
	}
	return domainID, true
}

// PermissionChecker is an exported variable.
var PermissionChecker = &permissionChecker{}

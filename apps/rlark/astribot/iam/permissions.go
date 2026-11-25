package iam

import (
	"net/url"
	"path"
	"strings"
)

func safeAPIPath(raw string) (string, bool) {
	if raw == "" || raw[0] != '/' || strings.Contains(raw, "//") || strings.Contains(raw, "\\") {
		return "", false
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "%2e") || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.Contains(lower, "%25") {
		return "", false
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || strings.Contains(decoded, "//") || strings.Contains(decoded, "\\") {
		return "", false
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	if path.Clean(decoded) != decoded || !strings.HasPrefix(decoded, "/api/v1/") {
		return "", false
	}
	return decoded, true
}

func allowed(role Role, method, rawPath string) bool {
	p, ok := safeAPIPath(rawPath)
	if !ok {
		return false
	}
	if role == RoleAdmin {
		return true
	}
	if isResourcePath(p, "/api/v1/auth") || isResourcePath(p, "/api/v1/users") || isResourcePath(p, "/api/v1/tenants") {
		return false
	}
	if method == "OPTIONS" || method == "CONNECT" || method == "TRACE" {
		return false
	}
	if strings.Contains(p, "/tensorboard") {
		parts := strings.Split(strings.TrimPrefix(p, "/api/v1/rlinf.io/v1alpha1/tasks/"), "/")
		if strings.HasPrefix(p, "/api/v1/rlinf.io/v1alpha1/tasks/") && len(parts) >= 3 && parts[1] == "tensorboard" && parts[2] != "" {
			return method == "GET" || method == "HEAD"
		}
		return false
	}
	if isResourcePath(p, "/api/v1/secrets") || isResourcePath(p, "/api/v1/service-accounts") || isResourcePath(p, "/api/v1/certificates") || isResourcePath(p, "/api/v1/station") || p == "/api/v1/image-registries" || strings.HasPrefix(p, "/api/v1/image-registries/") || p == "/api/v1/system-config" && method != "GET" {
		return false
	}
	if strings.HasPrefix(p, "/api/v1/rlinf.io/v1alpha1/") {
		resource := strings.TrimPrefix(p, "/api/v1/rlinf.io/v1alpha1/")
		parts := strings.Split(resource, "/")
		return allowedWorkload(role, method, parts)
	}
	if p == "/api/v1/clusters" || strings.HasPrefix(p, "/api/v1/clusters/") {
		return (method == "GET" || method == "HEAD") && len(strings.Split(strings.TrimPrefix(p, "/api/v1/clusters"), "/")) <= 2
	}
	if p == "/api/v1/node-networks" || strings.HasPrefix(p, "/api/v1/node-networks/") || p == "/api/v1/site-nodes" || strings.HasPrefix(p, "/api/v1/site-nodes/") {
		parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(p, "/api/v1/node-networks"), "/api/v1/site-nodes"), "/")
		return (method == "GET" || method == "HEAD") && len(parts) <= 2
	}
	if p == "/api/v1/images" {
		return method == "GET" || method == "HEAD"
	}
	if isResourcePath(p, "/api/v1/storage") {
		if role == RoleViewer {
			return (method == "GET" || method == "HEAD") && allowedStorage(method, strings.TrimPrefix(p, "/api/v1/storage"))
		}
		return allowedStorage(method, strings.TrimPrefix(p, "/api/v1/storage"))
	}
	if isResourcePath(p, "/api/v1/ssh-user-keys") {
		if role == RoleViewer {
			return method == "GET" || method == "HEAD"
		}
		return contains([]string{"GET", "HEAD", "POST", "DELETE"}, method)
	}
	return false
}

func allowedWorkload(role Role, method string, parts []string) bool {
	if len(parts) == 0 || parts[0] == "" {
		return false
	}
	read := method == "GET" || method == "HEAD"
	write := role == RoleOperator && contains([]string{"POST", "PUT", "PATCH", "DELETE"}, method)
	switch parts[0] {
	case "workflows":
		if len(parts) == 1 {
			return read || write && method == "POST"
		}
		return len(parts) == 2 && (read || write)
	case "jobs":
		if len(parts) == 1 {
			return read || write && method == "POST"
		}
		if len(parts) == 2 && parts[1] == "tags" {
			return read
		}
		if len(parts) == 2 {
			return read || write
		}
		if len(parts) == 3 && parts[2] == "start" {
			return role == RoleOperator && method == "POST"
		}
		if len(parts) == 3 && parts[2] == "logs" || len(parts) == 4 && parts[2] == "logs" && parts[3] == "label-values" {
			return read
		}
	case "tasks":
		if len(parts) == 1 {
			return read || write && method == "POST"
		}
		return len(parts) == 2 && (read || write)
	case "pods":
		if len(parts) == 1 {
			return read
		}
		if len(parts) == 2 {
			return read || role == RoleOperator && method == "PATCH"
		}
		if len(parts) == 3 && parts[2] == "events" {
			return read
		}
		if len(parts) == 3 && parts[2] == "terminal" {
			return role == RoleOperator && method == "GET"
		}
	case "nodes":
		if len(parts) == 1 || len(parts) == 2 {
			return read
		}
		if len(parts) == 3 && (parts[2] == "cameras" || parts[2] == "robot-claim") {
			if parts[2] == "robot-claim" {
				return role == RoleOperator && (method == "PUT" || method == "DELETE")
			}
			return read
		}
		if len(parts) == 4 && parts[2] == "cameras" || len(parts) == 5 && parts[2] == "cameras" && parts[4] == "stream" {
			return read
		}
		if len(parts) == 3 && role == RoleOperator {
			return parts[2] == "robot-power" && method == "PUT" || parts[2] == "robot-pose" && method == "POST"
		}
	case "domains":
		return len(parts) <= 2 && read
	}
	return false
}

func allowedStorage(method, pathSuffix string) bool {
	parts := strings.Split(strings.TrimPrefix(pathSuffix, "/"), "/")
	if len(parts) == 1 && parts[0] == "storageclass" {
		return method == "GET" || method == "POST"
	}
	if len(parts) == 2 && parts[0] == "storageclass" && parts[1] == "provider" {
		return method == "GET"
	}
	if len(parts) == 2 && parts[0] == "storageclass" {
		return method == "PUT" || method == "DELETE"
	}
	if len(parts) == 3 && parts[0] == "storageclass" && parts[2] == "list" {
		return method == "GET"
	}
	if len(parts) == 3 && parts[0] == "storageclass" && parts[2] == "upload" {
		return method == "POST"
	}
	if len(parts) >= 4 && parts[0] == "storageclass" && parts[2] == "object" && parts[3] != "" {
		return method == "GET" || method == "DELETE"
	}
	return false
}

func isResourcePath(value, root string) bool {
	return value == root || strings.HasPrefix(value, root+"/")
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// Write-path quotas on the number and the length of spec entries.
//
// These limits deliberately live here instead of in the CRD schema. A schema
// bound applies to every writer, including controllers and agents that merely
// relay an already stored object, and raising or lowering it would have to roll
// out to every cluster and would retroactively invalidate stored objects. The
// API server also rejects with Kubernetes validation output rather than the
// REST error envelope, so only the write path can name the offending field in a
// stable response. See the API conventions in the developer guide.
const (
	maxJobTags           = 10
	maxJobTagKeyLength   = 10
	maxJobTagValues      = 10
	maxJobTagValueLength = 10
)

// specQuotaError reports a single spec entry that exceeds a write-path quota.
// It carries the field path so the response can point a client at the entry it
// has to shrink.
type specQuotaError struct {
	message string
	field   string
	limit   int
	actual  int
}

func (e *specQuotaError) Error() string {
	return e.message
}

func countQuotaError(field string, limit, actual int, unit string) *specQuotaError {
	return &specQuotaError{
		message: fmt.Sprintf("%s holds %d %s, which exceeds the limit of %d", field, actual, unit, limit),
		field:   field,
		limit:   limit,
		actual:  actual,
	}
}

func lengthQuotaError(field string, limit, actual int) *specQuotaError {
	return &specQuotaError{
		message: fmt.Sprintf("%s is %d characters long, which exceeds the limit of %d", field, actual, limit),
		field:   field,
		limit:   limit,
		actual:  actual,
	}
}

// hasSpecQuotas reports whether a resource carries write-path spec quotas.
func hasSpecQuotas(resource string) bool {
	switch resource {
	case "jobs", "tasks", "workflows":
		return true
	default:
		return false
	}
}

// validateSpecQuotas enforces the write-path quotas of resource on a submitted
// body. The body may be a whole object or a merge patch, because a merge patch
// replaces an array as a whole: a quota-bearing array is either absent from the
// patch, and then the stored value was already checked when it was written, or
// present in full and checked here.
//
// Only quotas are enforced. An entry whose shape does not match the schema is a
// structural problem that the CRD schema closes and the API server reports, so
// it is passed through untouched rather than reported as a quota violation.
func validateSpecQuotas(resource string, body []byte) *specQuotaError {
	if !hasSpecQuotas(resource) {
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		return nil
	}
	spec, ok := object["spec"].(map[string]any)
	if !ok {
		return nil
	}
	if resource != "workflows" {
		return validateJobTagQuotas(spec, "spec")
	}
	templates, ok := spec["jobTemplates"].([]any)
	if !ok {
		return nil
	}
	for i, entry := range templates {
		template, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		jobSpec, ok := template["spec"].(map[string]any)
		if !ok {
			continue
		}
		if err := validateJobTagQuotas(jobSpec, fmt.Sprintf("spec.jobTemplates[%d].spec", i)); err != nil {
			return err
		}
	}
	return nil
}

// validateJobTagQuotas checks the tag quotas of a job spec reached at path.
func validateJobTagQuotas(spec map[string]any, path string) *specQuotaError {
	tags, ok := spec["tags"].([]any)
	if !ok {
		return nil
	}
	field := path + ".tags"
	if len(tags) > maxJobTags {
		return countQuotaError(field, maxJobTags, len(tags), "tags")
	}
	for i, entry := range tags {
		tag, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		tagField := fmt.Sprintf("%s[%d]", field, i)
		if key, ok := tag["key"].(string); ok {
			if count := utf8.RuneCountInString(key); count > maxJobTagKeyLength {
				return lengthQuotaError(tagField+".key", maxJobTagKeyLength, count)
			}
		}
		values, ok := tag["values"].([]any)
		if !ok {
			continue
		}
		if len(values) > maxJobTagValues {
			return countQuotaError(tagField+".values", maxJobTagValues, len(values), "values")
		}
		for j, rawValue := range values {
			value, ok := rawValue.(string)
			if !ok {
				continue
			}
			if count := utf8.RuneCountInString(value); count > maxJobTagValueLength {
				return lengthQuotaError(fmt.Sprintf("%s.values[%d]", tagField, j), maxJobTagValueLength, count)
			}
		}
	}
	return nil
}

// enforceSpecQuotas answers the request itself when body breaks a quota and
// reports whether the handler may continue.
func enforceSpecQuotas(c *gin.Context, resource string, body []byte) bool {
	quotaErr := validateSpecQuotas(resource, body)
	if quotaErr == nil {
		return true
	}
	respondErrorWithDetails(c, http.StatusBadRequest, CodeInvalidRequestBody, quotaErr.Error(), map[string]any{
		"field":  quotaErr.field,
		"limit":  quotaErr.limit,
		"actual": quotaErr.actual,
	})
	return false
}

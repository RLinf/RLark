package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// jobBodyWithTags builds a submitted body carrying the given tags.
func jobBodyWithTags(t *testing.T, tags []map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"name": "jo-1"},
		"spec":     map[string]any{"tags": tags},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return body
}

func TestValidateSpecQuotasAcceptsBoundaryValues(t *testing.T) {
	tags := make([]map[string]any, 0, maxJobTags)
	for range maxJobTags {
		values := make([]any, 0, maxJobTagValues)
		for j := range maxJobTagValues {
			values = append(values, fmt.Sprintf("v%09d", j))
		}
		tags = append(tags, map[string]any{
			"key":    strings.Repeat("k", maxJobTagKeyLength),
			"values": values,
		})
	}

	if err := validateSpecQuotas("jobs", jobBodyWithTags(t, tags)); err != nil {
		t.Fatalf("validateSpecQuotas() rejected a spec that is exactly at the limits: %v", err)
	}
}

func TestValidateSpecQuotasRejectsTooManyTags(t *testing.T) {
	tags := make([]map[string]any, 0, maxJobTags+1)
	for range maxJobTags + 1 {
		tags = append(tags, map[string]any{"key": "k", "values": []any{"v"}})
	}

	err := validateSpecQuotas("jobs", jobBodyWithTags(t, tags))
	if err == nil {
		t.Fatal("validateSpecQuotas() accepted more tags than the limit")
	}
	if err.field != "spec.tags" {
		t.Fatalf("field = %q, want %q", err.field, "spec.tags")
	}
	if err.limit != maxJobTags || err.actual != maxJobTags+1 {
		t.Fatalf("limit/actual = %d/%d, want %d/%d", err.limit, err.actual, maxJobTags, maxJobTags+1)
	}
}

func TestValidateSpecQuotasRejectsLongKey(t *testing.T) {
	tags := []map[string]any{
		{"key": "ok", "values": []any{"v"}},
		{"key": strings.Repeat("k", maxJobTagKeyLength+1), "values": []any{"v"}},
	}

	err := validateSpecQuotas("tasks", jobBodyWithTags(t, tags))
	if err == nil {
		t.Fatal("validateSpecQuotas() accepted a key longer than the limit")
	}
	if err.field != "spec.tags[1].key" {
		t.Fatalf("field = %q, want %q", err.field, "spec.tags[1].key")
	}
}

func TestValidateSpecQuotasCountsKeyLengthInCharacters(t *testing.T) {
	// A multi-byte key at the limit must pass: the removed schema marker
	// bounded characters, not bytes.
	tags := []map[string]any{{"key": strings.Repeat("标", maxJobTagKeyLength)}}
	if err := validateSpecQuotas("jobs", jobBodyWithTags(t, tags)); err != nil {
		t.Fatalf("validateSpecQuotas() counted bytes instead of characters: %v", err)
	}

	tags = []map[string]any{{"key": strings.Repeat("标", maxJobTagKeyLength+1)}}
	err := validateSpecQuotas("jobs", jobBodyWithTags(t, tags))
	if err == nil {
		t.Fatal("validateSpecQuotas() accepted a key longer than the limit")
	}
	if err.actual != maxJobTagKeyLength+1 {
		t.Fatalf("actual = %d, want %d", err.actual, maxJobTagKeyLength+1)
	}
}

func TestValidateSpecQuotasRejectsTooManyValues(t *testing.T) {
	values := make([]any, 0, maxJobTagValues+1)
	for range maxJobTagValues + 1 {
		values = append(values, "v")
	}
	tags := []map[string]any{{"key": "k", "values": values}}

	err := validateSpecQuotas("jobs", jobBodyWithTags(t, tags))
	if err == nil {
		t.Fatal("validateSpecQuotas() accepted more values than the limit")
	}
	if err.field != "spec.tags[0].values" {
		t.Fatalf("field = %q, want %q", err.field, "spec.tags[0].values")
	}
}

func TestValidateSpecQuotasRejectsLongValue(t *testing.T) {
	tags := []map[string]any{{
		"key":    "k",
		"values": []any{"ok", strings.Repeat("v", maxJobTagValueLength+1)},
	}}

	err := validateSpecQuotas("jobs", jobBodyWithTags(t, tags))
	if err == nil {
		t.Fatal("validateSpecQuotas() accepted a value longer than the limit")
	}
	if err.field != "spec.tags[0].values[1]" {
		t.Fatalf("field = %q, want %q", err.field, "spec.tags[0].values[1]")
	}
}

func TestValidateSpecQuotasReachesWorkflowJobTemplates(t *testing.T) {
	tags := make([]map[string]any, 0, maxJobTags+1)
	for range maxJobTags + 1 {
		tags = append(tags, map[string]any{"key": "k"})
	}
	body, err := json.Marshal(map[string]any{
		"spec": map[string]any{
			"jobTemplates": []any{
				map[string]any{"name": "first", "spec": map[string]any{"tags": []any{}}},
				map[string]any{"name": "second", "spec": map[string]any{"tags": tags}},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	quotaErr := validateSpecQuotas("workflows", body)
	if quotaErr == nil {
		t.Fatal("validateSpecQuotas() ignored the tags nested in a job template")
	}
	if quotaErr.field != "spec.jobTemplates[1].spec.tags" {
		t.Fatalf("field = %q, want %q", quotaErr.field, "spec.jobTemplates[1].spec.tags")
	}
}

func TestValidateSpecQuotasPassesPatchWithoutQuotaFields(t *testing.T) {
	// A merge patch replaces an array as a whole, so a patch that does not
	// mention tags leaves the already checked stored value in place.
	patch := []byte(`{"spec":{"suspend":true}}`)
	if err := validateSpecQuotas("jobs", patch); err != nil {
		t.Fatalf("validateSpecQuotas() rejected a patch that carries no tags: %v", err)
	}
	if err := validateSpecQuotas("jobs", []byte(`{"spec":{"tags":null}}`)); err != nil {
		t.Fatalf("validateSpecQuotas() rejected a patch clearing the tags: %v", err)
	}
}

func TestValidateSpecQuotasLeavesStructuralProblemsToTheSchema(t *testing.T) {
	for _, body := range []string{
		`not json`,
		`{"spec":"oops"}`,
		`{"spec":{"tags":"oops"}}`,
		`{"spec":{"tags":[{"key":1}]}}`,
		`{"spec":{"tags":[{"values":"oops"}]}}`,
	} {
		if err := validateSpecQuotas("jobs", []byte(body)); err != nil {
			t.Fatalf("validateSpecQuotas(%s) reported a structural problem as a quota violation: %v", body, err)
		}
	}
}

func TestValidateSpecQuotasSkipsResourcesWithoutQuotas(t *testing.T) {
	tags := make([]map[string]any, 0, maxJobTags+1)
	for range maxJobTags + 1 {
		tags = append(tags, map[string]any{"key": "k"})
	}
	if err := validateSpecQuotas("nodes", jobBodyWithTags(t, tags)); err != nil {
		t.Fatalf("validateSpecQuotas() enforced job tag quotas on an unrelated resource: %v", err)
	}
}

func TestEnforceSpecQuotasReportsOffendingField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	tags := []map[string]any{{"key": strings.Repeat("k", maxJobTagKeyLength+3)}}
	if enforceSpecQuotas(c, "jobs", jobBodyWithTags(t, tags)) {
		t.Fatal("enforceSpecQuotas() accepted a key longer than the limit")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	var body ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != CodeInvalidRequestBody {
		t.Fatalf("code = %q, want %q", body.Code, CodeInvalidRequestBody)
	}
	if body.Details["field"] != "spec.tags[0].key" {
		t.Fatalf("details.field = %v, want %q", body.Details["field"], "spec.tags[0].key")
	}
	if body.Details["limit"] != float64(maxJobTagKeyLength) {
		t.Fatalf("details.limit = %v, want %d", body.Details["limit"], maxJobTagKeyLength)
	}
	if body.Details["actual"] != float64(maxJobTagKeyLength+3) {
		t.Fatalf("details.actual = %v, want %d", body.Details["actual"], maxJobTagKeyLength+3)
	}
	if body.Message == "" {
		t.Fatal("the response carries no human readable message")
	}
}

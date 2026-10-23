package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

func TestFillBaseFromRawKeepsMissingUIDEmpty(t *testing.T) {
	base := &BaseResourceModel{}
	fillBaseFromRaw(base, map[string]any{
		"metadata": map[string]any{"name": "jo-1", "namespace": "default"},
	})

	// A missing UID must stay empty. Synthesizing one would claim an object
	// instance identity that the management API never issued.
	if base.UID != "" {
		t.Fatalf("uid = %q, want empty when metadata.uid is absent", base.UID)
	}
	if base.Name != "jo-1" || base.Namespace != "default" {
		t.Fatalf("name/namespace = %q/%q, want jo-1/default", base.Name, base.Namespace)
	}
	if base.ID != "default/jo-1" {
		t.Fatalf("id = %q, want default/jo-1", base.ID)
	}
}

func TestFillBaseFromRawKeepsProvidedUID(t *testing.T) {
	base := &BaseResourceModel{}
	fillBaseFromRaw(base, map[string]any{
		"metadata": map[string]any{"name": "jo-1", "uid": "real-uid"},
	})

	if base.UID != "real-uid" {
		t.Fatalf("uid = %q, want real-uid", base.UID)
	}
}

// TestErrNotFoundIsDetectableThroughWrapping pins the contract the gateway
// relies on: a missing row is reported as ErrNotFound and stays detectable with
// errors.Is after the store adds its own context, while a different driver
// error is not misreported as missing.
func TestErrNotFoundIsDetectableThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf("get default/jo-1 from jobs: %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Fatal("wrapped ErrNotFound is not detectable with errors.Is")
	}
	if errors.Is(fmt.Errorf("get: %w", sql.ErrConnDone), ErrNotFound) {
		t.Fatal("an unrelated driver error must not be reported as ErrNotFound")
	}
}

// TestLabelColumnExprNormalizesTenantLabel pins the asymmetry between the
// tenant label and every other label: a row without the tenant label belongs to
// the default tenant, so the selector has to compare against the normalized
// value instead of a NULL column.
func TestLabelColumnExprNormalizesTenantLabel(t *testing.T) {
	plain, plainArgs := labelColumnExpr("lj", "team")
	if plain != "lj.raw #>> ?" {
		t.Fatalf("unexpected expression for a regular label: %s", plain)
	}
	if len(plainArgs) != 1 {
		t.Fatalf("a regular label must bind exactly the path, got %d arguments", len(plainArgs))
	}

	normalized, normalizedArgs := labelColumnExpr("lj", tenant.IDLabel)
	if normalized != "COALESCE(NULLIF(lj.raw #>> ?, ''), ?)" {
		t.Fatalf("unexpected expression for the tenant label: %s", normalized)
	}
	if len(normalizedArgs) != 2 || normalizedArgs[1] != tenant.DefaultID {
		t.Fatalf("the tenant default is not bound as an argument: %#v", normalizedArgs)
	}
}

// TestLabelSelectorSQLKeepsAbsentTenantVisible renders the generated SQL for
// each supported operator. Without the normalization a row that never carried
// the tenant label would be filtered out by "=", "!=" and "in" alike, because
// comparing NULL yields NULL.
func TestLabelSelectorSQLKeepsAbsentTenantVisible(t *testing.T) {
	bunDB := bun.NewDB(nil, pgdialect.New())

	for _, op := range []string{"=", "!=", "in"} {
		colExpr, colArgs := labelColumnExpr("lj", tenant.IDLabel)
		query := applySelector(
			bunDB.NewSelect().Model(&LatestJobModel{}),
			colExpr,
			colArgs,
			op,
			tenant.DefaultID,
		)
		sql := query.String()
		if !strings.Contains(sql, "COALESCE(NULLIF(") {
			t.Fatalf("operator %q lost the tenant normalization: %s", op, sql)
		}
		if !strings.Contains(sql, `'`+tenant.DefaultID+`'`) {
			t.Fatalf("operator %q lost the normalized default value: %s", op, sql)
		}
	}

	colExpr, colArgs := labelColumnExpr("lj", "team")
	query := applySelector(
		bunDB.NewSelect().Model(&LatestJobModel{}),
		colExpr,
		colArgs,
		"=",
		"infra",
	)
	if sql := query.String(); strings.Contains(sql, "COALESCE(NULLIF(") {
		t.Fatalf("a regular label must not be normalized: %s", sql)
	}
}

// TestLabelSelectorInRendersPostgresArray pins the bind form of the "in"
// operator. A plain []string is rendered as a JSON array, which PostgreSQL
// rejects with "malformed array literal" at execution time, so the candidate
// list must be wrapped as a text[] parameter.
func TestLabelSelectorInRendersPostgresArray(t *testing.T) {
	bunDB := bun.NewDB(nil, pgdialect.New())

	colExpr, colArgs := labelColumnExpr("lj", tenant.IDLabel)
	query := applySelector(
		bunDB.NewSelect().Model(&LatestJobModel{}),
		colExpr,
		colArgs,
		"in",
		"default,acme",
	)

	sql := query.String()
	if !strings.Contains(sql, `'{"default","acme"}'`) {
		t.Fatalf("the candidate list is not bound as a PostgreSQL array: %s", sql)
	}
	if strings.Contains(sql, `["default","acme"]`) {
		t.Fatalf("the candidate list is rendered as a JSON array: %s", sql)
	}
}

// TestSelectorPathsAreBoundAsArguments pins the injection fix. A label key and
// a field selector path arrive from the query string, so neither may be
// formatted into the quoted JSONB path literal: a key such as
// "x}' IS NULL OR 'a" would otherwise close the literal and append its own
// predicate, which turns the filter into an always true expression and lets the
// caller reach rows and tables the request never selected.
//
// The rendered statement still contains the payload, but only as the escaped
// body of a single array literal, so the assertions compare the escaped form
// against the raw one instead of searching for the payload text.
func TestSelectorPathsAreBoundAsArguments(t *testing.T) {
	bunDB := bun.NewDB(nil, pgdialect.New())

	for _, key := range []string{
		`x}' IS NULL OR 'a`,
		`a'b`,
		`x}' IS NULL OR (SELECT count(*) FROM latest_jobs) > 0 OR 'a`,
	} {
		colExpr, colArgs := labelColumnExpr("lj", key)
		sql := applySelector(
			bunDB.NewSelect().Model(&LatestJobModel{}),
			colExpr,
			colArgs,
			"=",
			"a",
		).String()

		if strings.Contains(sql, key) {
			t.Fatalf("label key %q was not escaped: %s", key, sql)
		}
		if !strings.Contains(sql, strings.ReplaceAll(key, "'", "''")) {
			t.Fatalf("label key %q did not render as an escaped literal: %s", key, sql)
		}
		// The payload must stay inside the path literal, so the statement keeps
		// exactly one predicate: the selector comparison.
		if got := strings.Count(sql, " WHERE "); got != 1 {
			t.Fatalf("label key %q produced %d WHERE clauses: %s", key, got, sql)
		}
	}

	path := `x}' IS NULL OR 'a`
	colExpr, colArgs := jsonPathExpr("lj", strings.Split(path, "."))
	sql := applySelector(
		bunDB.NewSelect().Model(&LatestJobModel{}),
		colExpr,
		colArgs,
		"=",
		"a",
	).String()
	if strings.Contains(sql, path) {
		t.Fatalf("field selector path was not escaped: %s", sql)
	}
	if !strings.Contains(sql, strings.ReplaceAll(path, "'", "''")) {
		t.Fatalf("field selector path did not render as an escaped literal: %s", sql)
	}
}

// TestNotEqualSelectorMatchesAbsentKeys pins the selector semantics Kubernetes
// defines and the Kubernetes backed list path already implements: an object
// that does not carry the key matches "!=". A plain "!=" comparison evaluates
// to NULL for a missing key, so those objects would silently disappear and the
// two list paths would answer the same selector differently.
func TestNotEqualSelectorMatchesAbsentKeys(t *testing.T) {
	bunDB := bun.NewDB(nil, pgdialect.New())

	colExpr, colArgs := labelColumnExpr("lj", "team")
	sql := applySelector(
		bunDB.NewSelect().Model(&LatestJobModel{}),
		colExpr,
		colArgs,
		"!=",
		"infra",
	).String()

	if !strings.Contains(sql, "IS DISTINCT FROM 'infra'") {
		t.Fatalf("!= does not use IS DISTINCT FROM: %s", sql)
	}
}

// TestListResultEnvelope checks the serialized pagination envelope. The archive
// path has to report the same field names as the Kubernetes path, otherwise a
// client cannot tell a truncated page from a complete one.
func TestListResultEnvelope(t *testing.T) {
	encoded, err := json.Marshal(&ListResult{
		Items:   []map[string]any{{"kind": "Job"}},
		Total:   7,
		Limit:   2,
		Offset:  4,
		HasMore: true,
	})
	if err != nil {
		t.Fatalf("marshal list result: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal list result: %v", err)
	}
	for _, field := range []string{"items", "total", "limit", "offset", "hasMore"} {
		if _, ok := decoded[field]; !ok {
			t.Fatalf("field %q missing from the list envelope: %s", field, encoded)
		}
	}
	if decoded["hasMore"] != true {
		t.Fatalf("hasMore not preserved: %s", encoded)
	}
	if decoded["offset"] != float64(4) || decoded["limit"] != float64(2) || decoded["total"] != float64(7) {
		t.Fatalf("pagination window not preserved: %s", encoded)
	}
}

// TestTagSelectorSQLComparesAgainstTheRequestedKey pins the argument order of
// the tag filter. "?TableAlias" is a named placeholder and consumes no
// positional argument; feeding the alias in as one shifts every argument by a
// position, the key comparison then renders as a reference to the table row
// type and PostgreSQL rejects the statement with
// "operator does not exist: text = latest_jobs".
func TestTagSelectorSQLComparesAgainstTheRequestedKey(t *testing.T) {
	bunDB := bun.NewDB(nil, pgdialect.New())

	query := applyTagSelectors(
		bunDB.NewSelect().Model(&LatestJobModel{}),
		[]TagSelector{
			{Key: "suite", Value: "mr251"},
			{Key: "batch", Value: "two"},
			{Key: "suite", Value: "mr252"},
		},
	)
	sql := query.String()

	if !strings.Contains(sql, `tag->>'key' = 'suite'`) || !strings.Contains(sql, `tag->>'key' = 'batch'`) {
		t.Fatalf("tag keys are not compared as literal values: %s", sql)
	}
	if strings.Contains(sql, `tag->>'key' = "lj"`) {
		t.Fatalf("the table alias leaked into the key comparison: %s", sql)
	}
	// Values of one key are ORed through the IN list, different keys are ANDed
	// by separate EXISTS clauses.
	if !strings.Contains(sql, `value IN ('mr251', 'mr252')`) {
		t.Fatalf("values of the same key are not ORed: %s", sql)
	}
	if !strings.Contains(sql, `value IN ('two')`) {
		t.Fatalf("the second key lost its value list: %s", sql)
	}
	if got := strings.Count(sql, "jsonb_array_elements(COALESCE("); got != 2 {
		t.Fatalf("expected one EXISTS clause per key, got %d: %s", got, sql)
	}
	if !strings.Contains(sql, `"lj".raw->'spec'->'tags'`) {
		t.Fatalf("the table alias was not substituted: %s", sql)
	}
}

// TestTagSelectorSQLIsStableAcrossCalls keeps the rendered statement
// independent of Go map iteration order, so an identical request always
// produces an identical query.
func TestTagSelectorSQLIsStableAcrossCalls(t *testing.T) {
	bunDB := bun.NewDB(nil, pgdialect.New())
	selectors := []TagSelector{
		{Key: "suite", Value: "mr251"},
		{Key: "batch", Value: "two"},
		{Key: "stage", Value: "regression"},
	}

	first := applyTagSelectors(bunDB.NewSelect().Model(&LatestJobModel{}), selectors).String()
	for i := 0; i < 10; i++ {
		if got := applyTagSelectors(bunDB.NewSelect().Model(&LatestJobModel{}), selectors).String(); got != first {
			t.Fatalf("tag selector SQL is not stable\n first: %s\nlater: %s", first, got)
		}
	}
}

// TestTagSelectorsWithoutSelectorsLeaveTheQueryUntouched guards the common case
// where no tag filter was requested.
func TestTagSelectorsWithoutSelectorsLeaveTheQueryUntouched(t *testing.T) {
	bunDB := bun.NewDB(nil, pgdialect.New())
	base := bunDB.NewSelect().Model(&LatestJobModel{})
	before := base.String()

	if after := applyTagSelectors(base, nil).String(); after != before {
		t.Fatalf("an empty tag selector changed the query\nbefore: %s\n after: %s", before, after)
	}
}

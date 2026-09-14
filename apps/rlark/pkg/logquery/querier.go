package logquery

import (
	"context"
	"fmt"
	"time"
)

// Config describes which log backend to use and its backend-specific
// configuration. `Config` is intentionally opaque to the caller — each
// backend interprets the map as it sees fit.
type Config struct {
	// Backend selects the implementation: "sls", "loki", ...
	Backend string `json:"backend"`
	// Config holds backend-specific fields. For "sls": endpoint, project,
	// logstore, accessKeyId, accessKeySecret. See sls.go for details.
	Config map[string]interface{} `json:"config,omitempty"`
}

// Query is a single log query request.
type Query struct {
	// Raw is the user-supplied query string in the backend's native syntax.
	// The platform does not parse or translate it; users are expected to know
	// the syntax of whichever backend is configured.
	Raw string

	// Time range, both inclusive.
	From time.Time
	To   time.Time

	// Maximum number of entries to return.
	Limit int

	// Labels the platform injects into the query to enforce tenant/cluster
	// isolation (e.g. {"cluster_id": "xxx"}). Each backend combines these
	// with Raw using its own syntax.
	Labels map[string]string

	// Cursor is a pagination token returned by a previous query (Result.NextCursor).
	// Empty means "start from the beginning of the time range". Each backend
	// interprets the token in its own way; callers must treat it as opaque.
	Cursor string
}

// Entry is a single log line.
type Entry struct {
	Timestamp time.Time              `json:"timestamp"`
	Line      string                 `json:"line"`
	Labels    map[string]string      `json:"labels,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// Result is the outcome of a Query.
type Result struct {
	Entries []Entry `json:"entries"`
	HasMore bool    `json:"hasMore,omitempty"`
	// NextCursor is the token the caller should pass back as Query.Cursor to
	// fetch the next page. Empty when there are no more entries.
	NextCursor string `json:"nextCursor,omitempty"`
}

// Querier is implemented by each log backend.
type Querier interface {
	// Query runs a one-shot query against the backend.
	Query(ctx context.Context, q Query) (*Result, error)
}

// NewQuerier constructs a Querier for the given config. Returns an error if
// the backend is unknown or the config is invalid.
func NewQuerier(cfg *Config) (Querier, error) {
	if cfg == nil {
		return nil, fmt.Errorf("log config is nil")
	}
	switch cfg.Backend {
	case "sls":
		return newSLSQuerier(cfg.Config)
	default:
		return nil, fmt.Errorf("unsupported log backend: %q", cfg.Backend)
	}
}

// ValidateConfig checks that the config is well-formed for the chosen
// backend, without actually constructing a client.
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("log config is nil")
	}
	switch cfg.Backend {
	case "sls":
		return validateSLSConfig(cfg.Config)
	default:
		return fmt.Errorf("unsupported log backend: %q", cfg.Backend)
	}
}

// Package observability provides best-effort, bounded execution diagnostics.
// Observations never participate in reconciliation decisions or persistence.
package observability

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// Payload identifies one optional detailed event projection.
type Payload string

const (
	// PayloadTaskSnapshot enables a sanitized projection of the decoded Task.
	PayloadTaskSnapshot Payload = "task_snapshot"
	// PayloadPlan enables a sanitized projection of a compiled workload plan.
	PayloadPlan Payload = "plan"
	// PayloadOperations enables a sanitized projection of calculated Runtime operations.
	PayloadOperations Payload = "operations"
	// PayloadManifests enables sanitized Runtime manifest projections.
	PayloadManifests Payload = "manifests"
)

// DefaultMaxEventBytes is the default upper bound for one encoded event payload.
const DefaultMaxEventBytes = 32 * 1024

const configDiagnosticInterval = time.Minute

// Config is one immutable observer configuration snapshot.
type Config struct {
	// Payloads selects optional detailed projections. Unselected events retain
	// only their bounded, low-cardinality attributes.
	Payloads map[Payload]bool
	// MaxEventBytes bounds the encoded payload of each event. Non-positive
	// values select DefaultMaxEventBytes.
	MaxEventBytes int
}

// Enabled reports whether one optional payload is configured.
func (c Config) Enabled(payload Payload) bool { return c.Payloads[payload] }

// ConfigSource provides one configuration snapshot per reconciliation.
type ConfigSource interface {
	Snapshot() (Config, error)
}

// Clock supplies deterministic timestamps to observer scopes.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Reconcile identifies one execution observation root without making it authoritative state.
type Reconcile struct {
	Name       string
	TaskKey    string
	TaskUID    string
	Generation int64
	RuntimeID  string
}

// Event is one typed, bounded diagnostic point within a reconcile scope.
type Event struct {
	Name       string
	At         time.Time
	Attributes map[string]string
	Payload    json.RawMessage
	Truncated  bool
}

// Result closes a scope with stable low-cardinality execution fields.
type Result struct {
	Outcome   string
	Reason    string
	FaultCode string
}

// Scope records events for one reconciliation. Methods must never panic into callers.
type Scope interface {
	Event(Event)
	End(Result)
}

// Observer starts one best-effort execution scope.
type Observer interface {
	Start(context.Context, Reconcile) (context.Context, Scope)
}

type noopObserver struct{}
type noopScope struct{}

// Noop returns an observer with no allocation or external effects beyond interface calls.
func Noop() Observer { return noopObserver{} }

func (noopObserver) Start(ctx context.Context, _ Reconcile) (context.Context, Scope) {
	return ctx, noopScope{}
}
func (noopScope) Event(Event) {}
func (noopScope) End(Result)  {}

// Sanitizer projects detailed payloads before they reach a recorder or exporter.
type Sanitizer interface {
	Sanitize(Payload, any) (any, error)
}

// Safe wraps an observer with panic isolation, immutable config snapshots,
// production sanitization, and payload bounds.
func Safe(observer Observer, source ConfigSource, clock Clock, sanitizer Sanitizer) Observer {
	if observer == nil {
		observer = Noop()
	}
	if source == nil {
		source = StaticConfigSource{Config: Config{MaxEventBytes: DefaultMaxEventBytes}}
	}
	if clock == nil {
		clock = realClock{}
	}
	if sanitizer == nil {
		sanitizer = ProductionSanitizer()
	}
	return &safeObserver{next: observer, source: source, clock: clock, sanitizer: sanitizer}
}

type safeObserver struct {
	next                 Observer
	source               ConfigSource
	clock                Clock
	sanitizer            Sanitizer
	mu                   sync.Mutex
	last                 Config
	lastConfigDiagnostic time.Time
}

func (o *safeObserver) Start(ctx context.Context, root Reconcile) (resultCtx context.Context, resultScope Scope) {
	config, configErr := o.snapshot()
	resultCtx, resultScope = ctx, noopScope{}
	func() {
		defer func() { _ = recover() }()
		resultCtx, resultScope = o.next.Start(ctx, root)
	}()
	if resultScope == nil {
		resultScope = noopScope{}
	}
	scope := &safeScope{next: resultScope, config: config, clock: o.clock, sanitizer: o.sanitizer}
	if configErr != nil && o.allowConfigDiagnostic() {
		scope.Event(Event{Name: "ConfigRejected", Attributes: map[string]string{"reason": "InvalidDynamicConfig"}})
	}
	return resultCtx, scope
}

func (o *safeObserver) allowConfigDiagnostic() bool {
	now := o.clock.Now()
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.lastConfigDiagnostic.IsZero() && now.Sub(o.lastConfigDiagnostic) < configDiagnosticInterval {
		return false
	}
	o.lastConfigDiagnostic = now
	return true
}

func (o *safeObserver) snapshot() (Config, error) {
	config, err := o.source.Snapshot()
	o.mu.Lock()
	defer o.mu.Unlock()
	if err != nil {
		if o.last.MaxEventBytes == 0 {
			o.last = normalizedConfig(Config{})
		}
		return cloneConfig(o.last), err
	}
	o.last = normalizedConfig(config)
	return cloneConfig(o.last), nil
}

type safeScope struct {
	next      Scope
	config    Config
	clock     Clock
	sanitizer Sanitizer
}

func (s *safeScope) Event(event Event) {
	if event.At.IsZero() {
		event.At = s.clock.Now()
	}
	event.Attributes = cloneStrings(event.Attributes)
	if len(event.Payload) > s.config.MaxEventBytes {
		event.Payload = append(json.RawMessage(nil), event.Payload[:s.config.MaxEventBytes]...)
		event.Truncated = true
	} else {
		event.Payload = append(json.RawMessage(nil), event.Payload...)
	}
	defer func() { _ = recover() }()
	s.next.Event(event)
}

// Detailed sanitizes and records one configured detailed payload.
func (s *safeScope) Detailed(name string, payload Payload, value any, attributes map[string]string) {
	if !s.config.Enabled(payload) {
		return
	}
	projected, err := s.sanitizer.Sanitize(payload, value)
	if err != nil {
		s.Event(Event{Name: name, Attributes: mergeStrings(attributes, map[string]string{"payloadStatus": "Rejected"})})
		return
	}
	data, err := json.Marshal(projected)
	if err != nil {
		s.Event(Event{Name: name, Attributes: mergeStrings(attributes, map[string]string{"payloadStatus": "EncodeFailed"})})
		return
	}
	s.Event(Event{Name: name, Attributes: attributes, Payload: data})
}

func (s *safeScope) End(result Result) {
	defer func() { _ = recover() }()
	s.next.End(result)
}

// DetailedScope is implemented by safe scopes that accept configured payloads.
type DetailedScope interface {
	Scope
	Detailed(string, Payload, any, map[string]string)
}

// RecordDetailed records an optional payload when scope supports safe detailed events.
func RecordDetailed(scope Scope, name string, payload Payload, value any, attributes map[string]string) {
	if detailed, ok := scope.(DetailedScope); ok {
		detailed.Detailed(name, payload, value, attributes)
	}
}

// StaticConfigSource returns a fixed configuration.
type StaticConfigSource struct{ Config Config }

func (s StaticConfigSource) Snapshot() (Config, error) { return cloneConfig(s.Config), nil }

func normalizedConfig(config Config) Config {
	if config.MaxEventBytes <= 0 {
		config.MaxEventBytes = DefaultMaxEventBytes
	}
	if config.Payloads == nil {
		config.Payloads = map[Payload]bool{}
	}
	return config
}

func cloneConfig(config Config) Config {
	result := config
	result.Payloads = make(map[Payload]bool, len(config.Payloads))
	for key, value := range config.Payloads {
		result.Payloads[key] = value
	}
	return result
}

func cloneStrings(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func mergeStrings(left, right map[string]string) map[string]string {
	result := cloneStrings(left)
	for key, value := range right {
		result[key] = value
	}
	return result
}

func sensitiveKey(key string) bool {
	value := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", ""), "_", ""))
	for _, fragment := range []string{"password", "passwd", "token", "secret", "privatekey", "clientkey", "credential", "authorization", "apikey"} {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

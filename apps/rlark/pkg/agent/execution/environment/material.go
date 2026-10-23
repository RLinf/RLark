package environment

import (
	"context"
	"reflect"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// MaterialName identifies one opaque item within a resolved binding's local
// sensitive material. Names are trusted Handler contracts, not remote paths or
// secret values.
type MaterialName string

// MaterialRequest identifies sensitive material by durable execution identity
// and resolved binding identity. It contains no value-bearing fields.
type MaterialRequest struct {
	Task            agentruntime.TaskIdentity
	Kind            RequirementKind
	RequirementName string
	SourceRevision  string
	Name            MaterialName
}

// MaterialLease provides temporary in-process access to opaque sensitive bytes.
// Bytes are valid only until Close returns. Callers must not retain, stringify,
// serialize, log, hash, or include them in an error. Close must be idempotent.
type MaterialLease interface {
	Bytes() []byte
	Close()
}

// ByteMaterial owns a mutable copy of sensitive bytes and clears it on Close.
// Providers may return it when they do not need a stronger store-backed lease.
// The input slice is copied and remains the caller's responsibility.
type ByteMaterial struct {
	value  []byte
	closed bool
}

// NewByteMaterial copies value into a new locally owned lease.
func NewByteMaterial(value []byte) *ByteMaterial {
	return &ByteMaterial{value: append([]byte(nil), value...)}
}

// Bytes returns the leased bytes until Close; afterward it returns nil.
func (m *ByteMaterial) Bytes() []byte {
	if m == nil || m.closed {
		return nil
	}
	return m.value
}

// Close clears the locally owned buffer and is safe to call repeatedly.
func (m *ByteMaterial) Close() {
	if m == nil || m.closed {
		return
	}
	for i := range m.value {
		m.value[i] = 0
	}
	m.value = nil
	m.closed = true
}

// MaterialProvider acquires sensitive material from a trusted local source.
// Implementations must honor cancellation and return sanitized errors that
// contain neither material nor secret-store payloads.
type MaterialProvider interface {
	Acquire(context.Context, MaterialRequest) (MaterialLease, error)
}

// MaterialAccess is the restricted view exposed to a Handler during Apply.
// Acquire is unavailable while write intents are computed.
type MaterialAccess interface {
	Acquire(context.Context, MaterialName) (MaterialLease, error)
}

type bindingMaterialAccess struct {
	provider MaterialProvider
	request  MaterialRequest
	leases   []MaterialLease
}

func (a *bindingMaterialAccess) Acquire(ctx context.Context, name MaterialName) (MaterialLease, error) {
	if a == nil || isNilInterface(a.provider) {
		return nil, fault.New(fault.FailedPrecondition, "environment material provider is not configured")
	}
	request := a.request
	request.Name = name
	lease, err := a.provider.Acquire(ctx, request)
	if err != nil {
		return nil, err
	}
	if isNilInterface(lease) {
		return nil, fault.New(fault.Internal, "environment material provider returned no lease")
	}
	a.leases = append(a.leases, lease)
	return lease, nil
}

// isNilInterface recognizes both a nil interface and a typed nil stored in an
// interface, preventing provider and lease method calls on nil implementations.
func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (a *bindingMaterialAccess) close() {
	if a == nil {
		return
	}
	for i := len(a.leases) - 1; i >= 0; i-- {
		a.leases[i].Close()
	}
	a.leases = nil
}

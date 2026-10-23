package environment

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// HandlerID is the stable identity of one trusted Environment handler.
type HandlerID string

// HandlerKey selects a handler for one Runtime, requirement, and trusted
// profile type. Selection never uses Task-provided handler names.
type HandlerKey struct {
	RuntimeKind agentruntime.Kind
	BindingKind RequirementKind
	ProfileType string
}

// HandlerInfo describes trusted selection, ordering, and algorithm metadata.
// Requires names mandatory handlers. Before and After are optional ordering
// constraints and apply only when their referenced handlers participate.
type HandlerInfo struct {
	ID        HandlerID
	Key       HandlerKey
	Requires  []HandlerID
	Before    []HandlerID
	After     []HandlerID
	Algorithm string
}

// SemanticSlot identifies one logical field or aggregate input written by a
// handler. Slots are Runtime Binder contracts rather than arbitrary field paths.
type SemanticSlot string

// WriteIntent reserves one semantic slot with a canonical non-secret value.
// Value is used only for conflict comparison and must not contain raw material.
type WriteIntent struct {
	Slot  SemanticSlot
	Value []byte
}

// HandlerRequest supplies one resolved binding and the Binder-owned working
// Plan. Handlers must not retain the Plan after Bind returns.
type HandlerRequest struct {
	Task    agentruntime.TaskIdentity
	Runtime agentruntime.Info
	Binding ResolvedBinding
	Targets map[string]executionplan.MutationTarget
	Plan    *executionplan.Plan
	// Materials is always nil during Intents. During Apply it is non-nil only
	// when a MaterialProvider is configured. Material bytes must never influence
	// a WriteIntent.
	Materials MaterialAccess
}

// Handler declares semantic writes and applies one resolved binding to a
// Binder-owned working copy. Intents must be deterministic and side-effect free;
// Apply must perform no external I/O.
type Handler interface {
	Info() HandlerInfo
	Intents(context.Context, HandlerRequest) ([]WriteIntent, error)
	Apply(context.Context, HandlerRequest) error
}

// HandlerRegistration installs one trusted Handler in an immutable Registry.
type HandlerRegistration struct {
	Handler Handler
}

// Registry validates, selects, orders, and runs trusted Environment handlers.
// It is immutable after construction and safe for concurrent reads when its
// registered Handlers are themselves safe for concurrent use.
type Registry struct {
	byID  map[HandlerID]registeredHandler
	byKey map[HandlerKey]HandlerID
}

type registeredHandler struct {
	handler Handler
	info    HandlerInfo
}

// NewRegistry validates registrations and returns an immutable Handler registry.
func NewRegistry(registrations []HandlerRegistration) (*Registry, error) {
	registry := &Registry{byID: make(map[HandlerID]registeredHandler, len(registrations)), byKey: make(map[HandlerKey]HandlerID, len(registrations))}
	for _, registration := range registrations {
		if registration.Handler == nil {
			return nil, fault.New(fault.FailedPrecondition, "environment handler registration has no handler")
		}
		info := registration.Handler.Info()
		if strings.TrimSpace(string(info.ID)) == "" {
			return nil, fault.New(fault.FailedPrecondition, "environment handler ID is required")
		}
		if info.Key.RuntimeKind == "" || info.Key.BindingKind == "" || strings.TrimSpace(info.Key.ProfileType) == "" {
			return nil, fault.New(fault.FailedPrecondition, fmt.Sprintf("environment handler %q has an incomplete selection key", info.ID))
		}
		if strings.TrimSpace(info.Algorithm) == "" {
			return nil, fault.New(fault.FailedPrecondition, fmt.Sprintf("environment handler %q algorithm is required", info.ID))
		}
		for _, relation := range []struct {
			name       string
			references []HandlerID
		}{
			{name: "Requires", references: info.Requires},
			{name: "Before", references: info.Before},
			{name: "After", references: info.After},
		} {
			if slices.Contains(relation.references, info.ID) {
				return nil, fault.New(fault.FailedPrecondition, fmt.Sprintf("environment handler %q cannot reference itself in %s", info.ID, relation.name))
			}
		}
		if _, found := registry.byID[info.ID]; found {
			return nil, fault.New(fault.AlreadyExists, fmt.Sprintf("duplicate environment handler ID %q", info.ID))
		}
		if previous, found := registry.byKey[info.Key]; found {
			return nil, fault.New(fault.AlreadyExists, fmt.Sprintf("environment handler key is registered by both %q and %q", previous, info.ID))
		}
		info.Requires = append([]HandlerID(nil), info.Requires...)
		info.Before = append([]HandlerID(nil), info.Before...)
		info.After = append([]HandlerID(nil), info.After...)
		registry.byID[info.ID] = registeredHandler{handler: registration.Handler, info: info}
		registry.byKey[info.Key] = info.ID
	}
	return registry, nil
}

// HandlerJob binds one selected Handler to one resolved binding request.
type HandlerJob struct {
	Handler Handler
	Request HandlerRequest
}

// Jobs selects exactly one handler for every binding and verifies that every
// required handler is also selected by a binding in this request. Before and
// After references are optional when their handler is not selected. Returned
// jobs are in stable dependency order independent of registration and binding
// order.
func (r *Registry) Jobs(runtime agentruntime.Info, bindings []ResolvedBinding, targets map[string]executionplan.MutationTarget, working *executionplan.Plan) ([]HandlerJob, error) {
	if r == nil {
		return nil, fault.New(fault.FailedPrecondition, "environment handler registry is required")
	}
	jobs := make(map[HandlerID][]HandlerRequest)
	for _, binding := range bindings {
		key := HandlerKey{RuntimeKind: runtime.Kind, BindingKind: binding.Kind, ProfileType: binding.Profile.Type}
		id, found := r.byKey[key]
		if !found {
			return nil, fault.New(fault.FailedPrecondition, fmt.Sprintf("no environment handler for Runtime %q binding %q profile %q", runtime.Kind, binding.Kind, binding.Profile.Type))
		}
		jobs[id] = append(jobs[id], HandlerRequest{Runtime: runtime, Binding: binding, Targets: targets, Plan: working})
	}

	selected := make(map[HandlerID]struct{}, len(jobs))
	for id := range jobs {
		selected[id] = struct{}{}
	}
	for id := range selected {
		registered := r.byID[id]
		for _, required := range registered.info.Requires {
			if _, found := selected[required]; !found {
				return nil, fault.New(fault.FailedPrecondition, fmt.Sprintf("environment handler %q requires selected handler %q", id, required))
			}
		}
	}

	order, err := r.order(selected)
	if err != nil {
		return nil, err
	}
	result := make([]HandlerJob, 0, len(bindings)+len(selected))
	for _, id := range order {
		requests := jobs[id]
		sort.SliceStable(requests, func(i, j int) bool {
			left, right := requests[i].Binding, requests[j].Binding
			return string(left.Kind)+"\x00"+left.RequirementName < string(right.Kind)+"\x00"+right.RequirementName
		})
		for _, request := range requests {
			result = append(result, HandlerJob{Handler: r.byID[id].handler, Request: request})
		}
	}
	return result, nil
}

func (r *Registry) order(selected map[HandlerID]struct{}) ([]HandlerID, error) {
	indegree := make(map[HandlerID]int, len(selected))
	children := make(map[HandlerID]map[HandlerID]struct{}, len(selected))
	for id := range selected {
		indegree[id] = 0
	}
	addEdge := func(from, to HandlerID) {
		if children[from] == nil {
			children[from] = make(map[HandlerID]struct{})
		}
		if _, found := children[from][to]; found {
			return
		}
		children[from][to] = struct{}{}
		indegree[to]++
	}
	for id := range selected {
		info := r.byID[id].info
		for _, required := range info.Requires {
			if _, found := selected[required]; found {
				addEdge(required, id)
			}
		}
		for _, before := range info.Before {
			if _, found := selected[before]; found {
				addEdge(id, before)
			}
		}
		for _, after := range info.After {
			if _, found := selected[after]; found {
				addEdge(after, id)
			}
		}
	}

	ready := make(map[HandlerID]struct{})
	for id, degree := range indegree {
		if degree == 0 {
			ready[id] = struct{}{}
		}
	}
	result := make([]HandlerID, 0, len(selected))
	for len(ready) > 0 {
		ids := sortedHandlerIDs(ready)
		id := ids[0]
		delete(ready, id)
		result = append(result, id)
		for child := range children[id] {
			indegree[child]--
			if indegree[child] == 0 {
				ready[child] = struct{}{}
			}
		}
	}
	if len(result) != len(selected) {
		return nil, fault.New(fault.FailedPrecondition, "environment handler ordering contains a dependency cycle")
	}
	return result, nil
}

func sortedHandlerIDs(values map[HandlerID]struct{}) []HandlerID {
	result := make([]HandlerID, 0, len(values))
	for id := range values {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

// ValidateWriteIntents rejects empty or duplicate non-equivalent semantic
// writes. Equivalent writes are idempotent and reserve one canonical slot.
func ValidateWriteIntents(jobs []HandlerJob, intents [][]WriteIntent) error {
	if len(jobs) != len(intents) {
		return fault.New(fault.Internal, "environment handler intent results do not match jobs")
	}
	type reservation struct {
		writer HandlerID
		value  []byte
	}
	reserved := make(map[SemanticSlot]reservation)
	for index, jobIntents := range intents {
		writer := jobs[index].Handler.Info().ID
		for _, intent := range jobIntents {
			if strings.TrimSpace(string(intent.Slot)) == "" {
				return fault.New(fault.Internal, fmt.Sprintf("environment handler %q declared an empty semantic slot", writer))
			}
			previous, found := reserved[intent.Slot]
			if !found {
				reserved[intent.Slot] = reservation{writer: writer, value: append([]byte(nil), intent.Value...)}
				continue
			}
			if bytes.Equal(previous.value, intent.Value) {
				continue
			}
			return fault.New(fault.Conflict, fmt.Sprintf("semantic slot %q is written by both %q and %q", intent.Slot, previous.writer, writer))
		}
	}
	return nil
}

// Run computes every handler's declared intents before mutation, validates
// global slot conflicts, and then applies jobs in stable dependency order. The
// declarations are a trusted-handler contract; a concrete Binder must still
// perform final Runtime-schema validation on the resulting working copy.
func Run(ctx context.Context, task agentruntime.TaskIdentity, materials MaterialProvider, jobs []HandlerJob) error {
	intents := make([][]WriteIntent, len(jobs))
	for index, job := range jobs {
		if err := ctx.Err(); err != nil {
			return fault.Wrap(fault.DeadlineExceeded, "environment binding canceled before intent collection", err)
		}
		declared, err := job.Handler.Intents(ctx, job.Request)
		if err != nil {
			return err
		}
		intents[index] = declared
	}
	if err := ValidateWriteIntents(jobs, intents); err != nil {
		return err
	}
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return fault.Wrap(fault.DeadlineExceeded, "environment binding canceled before handler apply", err)
		}
		request := job.Request
		request.Materials = nil
		var access *bindingMaterialAccess
		if !isNilInterface(materials) {
			access = &bindingMaterialAccess{provider: materials, request: MaterialRequest{Task: task, Kind: job.Request.Binding.Kind, RequirementName: job.Request.Binding.RequirementName, SourceRevision: job.Request.Binding.SourceRevision}}
			request.Materials = access
		}
		err := func() error {
			if access != nil {
				defer access.close()
			}
			return job.Handler.Apply(ctx, request)
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

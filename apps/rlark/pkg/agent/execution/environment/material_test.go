package environment

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

const testMaterialName MaterialName = "test-material"

type testMaterialLease struct {
	value  []byte
	closed int
}

func (l *testMaterialLease) Bytes() []byte { return l.value }
func (l *testMaterialLease) Close()        { l.closed++ }

type testMaterialProvider struct {
	requests []MaterialRequest
	leases   []*testMaterialLease
	lease    MaterialLease
	noLease  bool
	err      error
}

func (p *testMaterialProvider) Acquire(_ context.Context, request MaterialRequest) (MaterialLease, error) {
	p.requests = append(p.requests, request)
	if p.err != nil {
		return nil, p.err
	}
	if p.noLease || p.lease != nil {
		return p.lease, nil
	}
	lease := &testMaterialLease{value: []byte("restricted-key")}
	p.leases = append(p.leases, lease)
	return lease, nil
}

type materialHandler struct {
	testHandler
	intentSawAccess bool
	applySawAccess  bool
	applyErr        error
	panicAfterRead  bool
}

func (h *materialHandler) Intents(_ context.Context, request HandlerRequest) ([]WriteIntent, error) {
	h.intentSawAccess = request.Materials != nil
	return []WriteIntent{{Slot: "container/main/credential", Value: []byte("env-fallback")}}, nil
}

func (h *materialHandler) Apply(ctx context.Context, request HandlerRequest) error {
	h.applySawAccess = request.Materials != nil
	if request.Materials == nil {
		return fault.New(fault.FailedPrecondition, "material access is unavailable")
	}
	lease, err := request.Materials.Acquire(ctx, testMaterialName)
	if err != nil {
		return err
	}
	if string(lease.Bytes()) != "restricted-key" {
		return errors.New("unexpected material")
	}
	if h.panicAfterRead {
		panic("handler panic")
	}
	return h.applyErr
}

func TestRunRestrictsMaterialToApplyAndClosesLeases(t *testing.T) {
	provider := &testMaterialProvider{}
	handler := &materialHandler{testHandler: testHandler{info: HandlerInfo{ID: "credential"}}}
	binding := ResolvedBinding{Kind: RequirementSecret, RequirementName: "credential", SourceRevision: "secret-rv"}
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "tenant", Name: "task"}, ObjectUID: "task-uid"}
	jobs := []HandlerJob{{Handler: handler, Request: HandlerRequest{Binding: binding}}}
	if err := Run(context.Background(), task, provider, jobs); err != nil {
		t.Fatal(err)
	}
	if handler.intentSawAccess {
		t.Fatal("material access was exposed while collecting intents")
	}
	if !handler.applySawAccess {
		t.Fatal("material access was not exposed while applying with a configured provider")
	}
	wantRequest := MaterialRequest{Task: task, Kind: RequirementSecret, RequirementName: "credential", SourceRevision: "secret-rv", Name: testMaterialName}
	if !reflect.DeepEqual(provider.requests, []MaterialRequest{wantRequest}) {
		t.Fatalf("requests=%#v, want %#v", provider.requests, []MaterialRequest{wantRequest})
	}
	if len(provider.leases) != 1 || provider.leases[0].closed != 1 {
		t.Fatalf("leases=%#v", provider.leases)
	}
}

func TestRunLeavesMaterialsNilWithoutProvider(t *testing.T) {
	var typedNil *testMaterialProvider
	for name, provider := range map[string]MaterialProvider{"nil": nil, "typed nil": typedNil} {
		t.Run(name, func(t *testing.T) {
			handler := &materialHandler{testHandler: testHandler{info: HandlerInfo{ID: "credential"}}}
			err := Run(context.Background(), agentruntime.TaskIdentity{}, provider, []HandlerJob{{Handler: handler, Request: HandlerRequest{Binding: ResolvedBinding{Kind: RequirementSecret, RequirementName: "credential"}}}})
			if !fault.Is(err, fault.FailedPrecondition) {
				t.Fatalf("error=%v", err)
			}
			if handler.intentSawAccess || handler.applySawAccess {
				t.Fatalf("material access exposed without provider: intents=%t apply=%t", handler.intentSawAccess, handler.applySawAccess)
			}
		})
	}
}

func TestRunClosesMaterialLeaseWhenApplyFails(t *testing.T) {
	provider := &testMaterialProvider{}
	sentinel := errors.New("apply failed")
	handler := &materialHandler{testHandler: testHandler{info: HandlerInfo{ID: "credential"}}, applyErr: sentinel}
	err := Run(context.Background(), agentruntime.TaskIdentity{}, provider, []HandlerJob{{Handler: handler, Request: HandlerRequest{Binding: ResolvedBinding{Kind: RequirementSecret, RequirementName: "credential"}}}})
	if !errors.Is(err, sentinel) || len(provider.leases) != 1 || provider.leases[0].closed != 1 {
		t.Fatalf("error=%v leases=%#v", err, provider.leases)
	}
}

func TestRunClosesMaterialLeaseWhenApplyPanics(t *testing.T) {
	provider := &testMaterialProvider{}
	handler := &materialHandler{testHandler: testHandler{info: HandlerInfo{ID: "credential"}}, panicAfterRead: true}
	defer func() {
		if recover() == nil {
			t.Fatal("Run did not propagate handler panic")
		}
		if len(provider.leases) != 1 || provider.leases[0].closed != 1 {
			t.Fatalf("leases=%#v", provider.leases)
		}
	}()
	_ = Run(context.Background(), agentruntime.TaskIdentity{}, provider, []HandlerJob{{Handler: handler, Request: HandlerRequest{Binding: ResolvedBinding{Kind: RequirementSecret, RequirementName: "credential"}}}})
}

func TestMaterialAccessRequiresConfiguredProvider(t *testing.T) {
	var typedNil *testMaterialProvider
	for name, provider := range map[string]MaterialProvider{"nil": nil, "typed nil": typedNil} {
		t.Run(name, func(t *testing.T) {
			access := &bindingMaterialAccess{provider: provider}
			if _, err := access.Acquire(context.Background(), testMaterialName); !fault.Is(err, fault.FailedPrecondition) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestMaterialAccessRejectsNilProviderLease(t *testing.T) {
	var typedNil *testMaterialLease
	for name, provider := range map[string]*testMaterialProvider{
		"nil":       {noLease: true},
		"typed nil": {lease: typedNil},
	} {
		t.Run(name, func(t *testing.T) {
			access := &bindingMaterialAccess{provider: provider}
			lease, err := access.Acquire(context.Background(), testMaterialName)
			if lease != nil || !fault.Is(err, fault.Internal) {
				t.Fatalf("lease=%#v error=%v", lease, err)
			}
			access.close()
		})
	}
}

func TestByteMaterialCopiesAndClearsValue(t *testing.T) {
	source := []byte("private-key")
	lease := NewByteMaterial(source)
	source[0] = 'X'
	borrowed := lease.Bytes()
	if string(borrowed) != "private-key" {
		t.Fatalf("bytes=%q", borrowed)
	}
	lease.Close()
	lease.Close()
	if lease.Bytes() != nil {
		t.Fatalf("closed bytes=%q", lease.Bytes())
	}
	for index, value := range borrowed {
		if value != 0 {
			t.Fatalf("borrowed[%d]=%d, want zero", index, value)
		}
	}
}

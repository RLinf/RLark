package grpc_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	runtimegrpc "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime/transport/grpc"
	executiongrpc "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/transport/grpc"
	commonv1 "github.com/rlinf/rlark/sdks/execution-go/gen/common/v1"
	runtimev1 "github.com/rlinf/rlark/sdks/execution-go/gen/runtime/v1"
)

type provider struct {
	applied    agentruntime.ApplyRequest
	deleted    agentruntime.DeleteRequest
	query      agentruntime.InstanceQuery
	candidates agentruntime.ManagedObjectQuery
	err        error
}

func (*provider) Info(context.Context) (agentruntime.Info, error) {
	return agentruntime.Info{ID: "runtime", Kind: "Kubernetes", Capabilities: []agentruntime.Capability{agentruntime.CapabilityCandidateInventory, agentruntime.CapabilityOwnedFieldProjection, agentruntime.CapabilityClaimedWorkloadRecovery}}, nil
}
func (p *provider) Get(_ context.Context, ref agentruntime.ObjectRef) (agentruntime.Object, error) {
	if p.err != nil {
		return agentruntime.Object{}, p.err
	}
	return runtimeObject(ref), nil
}
func (p *provider) ListManaged(_ context.Context, task agentruntime.TaskIdentity) (agentruntime.ObjectSnapshot, error) {
	if p.err != nil {
		return agentruntime.ObjectSnapshot{}, p.err
	}
	ref := agentruntime.ObjectRef{RuntimeID: "runtime", ID: "managed", Namespace: "default", Name: "task", UID: "uid", LogicalTask: task.Logical}
	return agentruntime.ObjectSnapshot{Revision: "objects-1", Objects: []agentruntime.Object{runtimeObject(ref)}}, nil
}
func (p *provider) ListCandidates(_ context.Context, query agentruntime.ManagedObjectQuery) (agentruntime.ObjectSnapshot, error) {
	if p.err != nil {
		return agentruntime.ObjectSnapshot{}, p.err
	}
	p.candidates = query
	ref := agentruntime.ObjectRef{RuntimeID: "runtime", ID: "v1/Service", Namespace: "default", Name: "task-head", UID: "service-uid", LogicalTask: query.Task.Logical}
	return agentruntime.ObjectSnapshot{Revision: "candidates-1", Objects: []agentruntime.Object{runtimeObject(ref)}}, nil
}
func (p *provider) ClaimedWorkload(context.Context, agentruntime.TaskIdentity) (string, string, error) {
	if p.err != nil {
		return "", "", p.err
	}
	return "Deployment", "v1alpha1", nil
}
func (p *provider) Apply(_ context.Context, request agentruntime.ApplyRequest) (agentruntime.ApplyResult, error) {
	if p.err != nil {
		return agentruntime.ApplyResult{}, p.err
	}
	p.applied = request
	object := agentruntime.Object{Ref: agentruntime.ObjectRef{RuntimeID: "runtime", ID: "object", Namespace: "default", Name: "task", UID: "uid", LogicalTask: request.Task.Logical}, Manifest: request.Desired.Manifest}
	return agentruntime.ApplyResult{Ref: object.Ref, Object: object}, nil
}
func (p *provider) Delete(_ context.Context, request agentruntime.DeleteRequest) error {
	p.deleted = request
	return p.err
}
func (p *provider) ListInstances(_ context.Context, query agentruntime.InstanceQuery) (agentruntime.InstanceSnapshot, error) {
	if p.err != nil {
		return agentruntime.InstanceSnapshot{}, p.err
	}
	p.query = query
	index, rank := int32(2), int32(3)
	return agentruntime.InstanceSnapshot{Revision: "instances-1", Instances: []agentruntime.Instance{{Ref: agentruntime.InstanceRef{RuntimeID: "runtime", ID: "pod", UID: "pod-uid", Parent: query.Workloads[0]}, Task: query.Task, Name: "worker-2", Role: "worker", Index: &index, Rank: &rank, Phase: "Running", Reason: "Ready", Message: "ready", Conditions: []agentruntime.InstanceCondition{{Source: "kubernetes", Scope: "container", Subject: "trainer", Reason: "OOMKilled", Message: "limit"}}}}}, nil
}

func TestClientServerRoundTrip(t *testing.T) {
	implementation := &provider{}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.UnaryInterceptor(executiongrpc.UnaryServerErrorInterceptor))
	runtimev1.RegisterRuntimeProviderServer(server, runtimegrpc.NewServer(implementation))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithUnaryInterceptor(executiongrpc.UnaryClientErrorInterceptor))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := runtimegrpc.NewClient(runtimev1.NewRuntimeProviderClient(connection))
	ctx := context.Background()
	info, err := client.Info(ctx)
	if err != nil || info.ID != "runtime" {
		t.Fatalf("Info()=%#v, %v", info, err)
	}
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "cluster", Name: "task"}, ObjectUID: "task-uid"}
	desired := agentruntime.DesiredObject{LogicalName: "workload", Kind: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Manifest: agentruntime.TypedManifest{Type: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Object: []byte(`{"kind":"Deployment"}`)}, Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid", IncarnationPolicy: agentruntime.IncarnationOwnershipAllowAdoption}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace, OwnedFields: []string{"data", "binaryData"}, ReconcileProfile: agentruntime.ObjectKind{Group: "kubernetes.execution.rlark.io", Version: "v1", Kind: "WorkloadProjection"}}
	owner := agentruntime.ObjectRef{RuntimeID: "runtime", ID: "apps/v1, Kind=Deployment", Namespace: "default", Name: "task", UID: "root-uid", LogicalTask: task.Logical}
	result, err := client.Apply(ctx, agentruntime.ApplyRequest{IdempotencyKey: "key", Task: task, Desired: desired, ProvenOwners: []agentruntime.ObjectRef{owner}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Ref.ID != "object" || implementation.applied.IdempotencyKey != "key" || implementation.applied.Desired.LogicalName != "workload" || implementation.applied.Desired.Ownership.IncarnationPolicy != agentruntime.IncarnationOwnershipAllowAdoption || implementation.applied.Desired.ReconcileProfile != desired.ReconcileProfile || len(implementation.applied.ProvenOwners) != 1 || implementation.applied.ProvenOwners[0] != owner || len(implementation.applied.Desired.OwnedFields) != 2 {
		t.Fatalf("unexpected apply round trip: %#v %#v", result, implementation.applied)
	}
	desired.Units = []agentruntime.RuntimeUnit{{Name: "pod", Kind: agentruntime.UnitPodTemplate, Role: "worker", Locator: agentruntime.ObjectLocator{Type: desired.Kind, Path: ".spec.template"}}}
	if _, err := client.Apply(ctx, agentruntime.ApplyRequest{Task: task, Desired: desired}); err != nil {
		t.Fatal(err)
	}
	if len(implementation.applied.Desired.Units) != 1 || implementation.applied.Desired.Units[0] != desired.Units[0] {
		t.Fatalf("units were not preserved: %#v", implementation.applied.Desired.Units)
	}
	desired.Units = nil
	candidate := agentruntime.ObjectCandidate{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "default", Name: "task-head", Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid", IncarnationPolicy: agentruntime.IncarnationOwnershipAllowAdoption}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup}
	candidates, err := client.ListCandidates(ctx, agentruntime.ManagedObjectQuery{Task: task, Candidates: []agentruntime.ObjectCandidate{candidate}})
	if err != nil || candidates.Revision != "candidates-1" || len(candidates.Objects) != 1 || implementation.candidates.Candidates[0].Name != "task-head" || implementation.candidates.Candidates[0].Ownership.IncarnationPolicy != agentruntime.IncarnationOwnershipAllowAdoption {
		t.Fatalf("ListCandidates() = %#v, %v; query=%#v", candidates, err, implementation.candidates)
	}
	workloadType, workloadVersion, err := client.ClaimedWorkload(ctx, task)
	if err != nil || workloadType != "Deployment" || workloadVersion != "v1alpha1" {
		t.Fatalf("ClaimedWorkload() = %q, %q, %v", workloadType, workloadVersion, err)
	}
	ref := result.Ref
	object, err := client.Get(ctx, ref)
	if err != nil || object.Ref != ref || string(object.Manifest.Object) != `{"kind":"Deployment"}` {
		t.Fatalf("Get() = %#v, %v", object, err)
	}
	objects, err := client.ListManaged(ctx, task)
	if err != nil || objects.Revision != "objects-1" || len(objects.Objects) != 1 {
		t.Fatalf("ListManaged() = %#v, %v", objects, err)
	}
	current := objects.Objects[0].Ref
	if _, err := client.Apply(ctx, agentruntime.ApplyRequest{IdempotencyKey: "update", Task: task, Desired: desired, Current: &current}); err != nil {
		t.Fatal(err)
	}
	if implementation.applied.Current == nil || *implementation.applied.Current != current {
		t.Fatalf("current reference was not preserved: %#v", implementation.applied)
	}
	options := agentruntime.TypedOptions{Type: agentruntime.ObjectKind{Group: "rlark.io", Version: "v1", Kind: "DiscoveryOptions"}, Value: []byte(`{"pods":true}`)}
	instances, err := client.ListInstances(ctx, agentruntime.InstanceQuery{Task: task, Workloads: []agentruntime.ObjectRef{ref}, Roots: []string{"workload"}, Options: &options})
	if err != nil || instances.Revision != "instances-1" || len(instances.Instances) != 1 {
		t.Fatalf("ListInstances() = %#v, %v", instances, err)
	}
	instance := instances.Instances[0]
	if instance.Name != "worker-2" || instance.Index == nil || *instance.Index != 2 || implementation.query.Options == nil || string(implementation.query.Options.Value) != `{"pods":true}` {
		t.Fatalf("unexpected instance round trip: %#v query=%#v", instance, implementation.query)
	}
	if len(instance.Conditions) != 1 || instance.Conditions[0].Subject != "trainer" || instance.Conditions[0].Reason != "OOMKilled" {
		t.Fatalf("instance conditions were not preserved: %#v", instance.Conditions)
	}
	if err := client.Delete(ctx, agentruntime.DeleteRequest{IdempotencyKey: "delete-key", Task: task, Ref: ref}); err != nil {
		t.Fatal(err)
	}
	if implementation.deleted.IdempotencyKey != "delete-key" || implementation.deleted.Ref != ref || implementation.deleted.Task != task {
		t.Fatalf("unexpected delete request: %#v", implementation.deleted)
	}
}

func TestClientServerPropagatesProviderErrors(t *testing.T) {
	want := fault.Wrap(fault.Unavailable, "provider unavailable", errors.New("offline"))
	implementation := &provider{err: want}
	client, cleanup := newTestClient(t, implementation)
	defer cleanup()
	ctx := context.Background()
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "cluster", Name: "task"}}
	ref := agentruntime.ObjectRef{RuntimeID: "runtime", ID: "object", LogicalTask: task.Logical}
	if _, err := client.Get(ctx, ref); !fault.Is(err, fault.Unavailable) {
		t.Fatalf("Get() error = %v", err)
	}
	if _, err := client.ListManaged(ctx, task); err == nil {
		t.Fatal("ListManaged() error = nil")
	}
	if err := client.Delete(ctx, agentruntime.DeleteRequest{Task: task, Ref: ref}); err == nil {
		t.Fatal("Delete() error = nil")
	}
}

func TestClientServerMapsFaultsWithoutInterceptors(t *testing.T) {
	implementation := &provider{err: fault.New(fault.NotFound, "missing")}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	runtimev1.RegisterRuntimeProviderServer(server, runtimegrpc.NewServer(implementation))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := runtimegrpc.NewClient(runtimev1.NewRuntimeProviderClient(connection))
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "cluster", Name: "task"}}
	ref := agentruntime.ObjectRef{RuntimeID: "runtime", ID: "object", LogicalTask: task.Logical}
	if _, err := client.Get(context.Background(), ref); !fault.Is(err, fault.NotFound) {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestServerRejectsUnsupportedProtocol(t *testing.T) {
	server := runtimegrpc.NewServer(&provider{})
	if _, err := server.GetInfo(context.Background(), &runtimev1.GetInfoRequest{}); err == nil {
		t.Fatal("GetInfo() error = nil")
	}
}

func TestServerRejectsNewRuntimeRPCsWithoutCapabilities(t *testing.T) {
	server := runtimegrpc.NewServer(&legacyProvider{})
	task := &commonv1.TaskIdentity{LogicalId: &commonv1.LogicalTaskId{Namespace: "cluster", Name: "task"}}
	if _, err := server.ListCandidateObjects(context.Background(), &runtimev1.ListCandidateObjectsRequest{Task: task}); !fault.Is(fault.FromGRPC(err), fault.FailedPrecondition) {
		t.Fatalf("ListCandidateObjects() error=%v, want failed precondition", err)
	}
	if _, err := server.GetClaimedWorkload(context.Background(), &runtimev1.GetClaimedWorkloadRequest{Task: task}); !fault.Is(fault.FromGRPC(err), fault.FailedPrecondition) {
		t.Fatalf("GetClaimedWorkload() error=%v, want failed precondition", err)
	}
}

type legacyProvider struct{ implementation provider }

func (p *legacyProvider) Info(ctx context.Context) (agentruntime.Info, error) {
	return p.implementation.Info(ctx)
}
func (p *legacyProvider) Get(ctx context.Context, ref agentruntime.ObjectRef) (agentruntime.Object, error) {
	return p.implementation.Get(ctx, ref)
}
func (p *legacyProvider) ListManaged(ctx context.Context, task agentruntime.TaskIdentity) (agentruntime.ObjectSnapshot, error) {
	return p.implementation.ListManaged(ctx, task)
}
func (p *legacyProvider) Apply(ctx context.Context, request agentruntime.ApplyRequest) (agentruntime.ApplyResult, error) {
	return p.implementation.Apply(ctx, request)
}
func (p *legacyProvider) Delete(ctx context.Context, request agentruntime.DeleteRequest) error {
	return p.implementation.Delete(ctx, request)
}
func (p *legacyProvider) ListInstances(ctx context.Context, query agentruntime.InstanceQuery) (agentruntime.InstanceSnapshot, error) {
	return p.implementation.ListInstances(ctx, query)
}

func TestServerRejectsMalformedArchitectureRequests(t *testing.T) {
	server := runtimegrpc.NewServer(&provider{})
	task := &commonv1.TaskIdentity{LogicalId: &commonv1.LogicalTaskId{Namespace: "cluster", Name: "task"}}
	ref := &commonv1.ObjectReference{RuntimeId: "runtime", Id: "object", LogicalTaskId: task.LogicalId}
	tests := []struct {
		name string
		call func() error
	}{
		{name: "missing get reference", call: func() error {
			_, err := server.GetObject(context.Background(), &runtimev1.GetObjectRequest{})
			return err
		}},
		{name: "missing task", call: func() error {
			_, err := server.ListManagedObjects(context.Background(), &runtimev1.ListManagedObjectsRequest{})
			return err
		}},
		{name: "missing desired", call: func() error {
			_, err := server.ApplyObject(context.Background(), &runtimev1.ApplyObjectRequest{Task: task})
			return err
		}},
		{name: "missing delete reference", call: func() error {
			_, err := server.DeleteObject(context.Background(), &runtimev1.DeleteObjectRequest{Task: task})
			return err
		}},
		{name: "missing instance task", call: func() error {
			_, err := server.ListInstances(context.Background(), &runtimev1.ListInstancesRequest{Workloads: []*commonv1.ObjectReference{ref}})
			return err
		}},
		{name: "malformed instance reference", call: func() error {
			_, err := server.ListInstances(context.Background(), &runtimev1.ListInstancesRequest{Task: task, Workloads: []*commonv1.ObjectReference{nil}})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !fault.Is(fault.FromGRPC(err), fault.InvalidArgument) {
				t.Fatalf("error = %v, want invalid argument", err)
			}
		})
	}
}

func TestClientRejectsMalformedProviderResponses(t *testing.T) {
	client := runtimegrpc.NewClient(&malformedRuntimeClient{})
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "cluster", Name: "task"}}
	ref := agentruntime.ObjectRef{RuntimeID: "runtime", ID: "object", LogicalTask: task.Logical}
	if _, err := client.Get(context.Background(), ref); err == nil {
		t.Fatal("Get() accepted a nil Runtime object")
	}
	if _, err := client.ListManaged(context.Background(), task); err == nil {
		t.Fatal("ListManaged() accepted a nil object in a complete snapshot")
	}
	if _, err := client.ListInstances(context.Background(), agentruntime.InstanceQuery{Task: task, Workloads: []agentruntime.ObjectRef{ref}}); err == nil {
		t.Fatal("ListInstances() accepted a nil instance in a complete snapshot")
	}
}

type malformedRuntimeClient struct {
	runtimev1.RuntimeProviderClient
}

func (*malformedRuntimeClient) GetObject(context.Context, *runtimev1.GetObjectRequest, ...grpc.CallOption) (*runtimev1.GetObjectResponse, error) {
	return &runtimev1.GetObjectResponse{}, nil
}

func (*malformedRuntimeClient) ListManagedObjects(context.Context, *runtimev1.ListManagedObjectsRequest, ...grpc.CallOption) (*runtimev1.ListManagedObjectsResponse, error) {
	return &runtimev1.ListManagedObjectsResponse{Objects: []*commonv1.RuntimeObject{nil}}, nil
}

func (*malformedRuntimeClient) ListInstances(context.Context, *runtimev1.ListInstancesRequest, ...grpc.CallOption) (*runtimev1.ListInstancesResponse, error) {
	return &runtimev1.ListInstancesResponse{Instances: []*commonv1.RuntimeInstance{nil}}, nil
}

func newTestClient(t *testing.T, implementation agentruntime.Provider) (*runtimegrpc.Client, func()) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.UnaryInterceptor(executiongrpc.UnaryServerErrorInterceptor))
	runtimev1.RegisterRuntimeProviderServer(server, runtimegrpc.NewServer(implementation))
	go func() { _ = server.Serve(listener) }()
	connection, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithUnaryInterceptor(executiongrpc.UnaryClientErrorInterceptor))
	if err != nil {
		server.Stop()
		t.Fatal(err)
	}
	return runtimegrpc.NewClient(runtimev1.NewRuntimeProviderClient(connection)), func() {
		_ = connection.Close()
		server.Stop()
	}
}

func runtimeObject(ref agentruntime.ObjectRef) agentruntime.Object {
	return agentruntime.Object{Ref: ref, Manifest: agentruntime.TypedManifest{Type: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Object: []byte(`{"kind":"Deployment"}`)}}
}

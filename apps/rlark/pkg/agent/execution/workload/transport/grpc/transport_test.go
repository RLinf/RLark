package grpc_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	corev1 "k8s.io/api/core/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	executiongrpc "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/transport/grpc"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
	workloadgrpc "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload/transport/grpc"
	commonv1 "github.com/rlinf/rlark/sdks/execution-go/gen/common/v1"
	workloadv1 "github.com/rlinf/rlark/sdks/execution-go/gen/workload/v1"
)

type adapter struct {
	compile workload.CompileRequest
	observe workload.ObserveRequest
	plan    *executionplan.Plan
	err     error
}

func (*adapter) Info(context.Context) (workload.TypeInfo, error) {
	return workload.TypeInfo{ID: "PyTorchJob", Version: "v1", SupportedRuntimeKinds: []agentruntime.Kind{"Kubernetes"}, Configuration: workload.ConfigurationContract{Version: "v1", Schema: []byte(`{"type":"object"}`)}, Capabilities: []agentruntime.Capability{agentruntime.CapabilityWorkloadInventory, agentruntime.CapabilityOwnedFieldProjection}}, nil
}
func (a *adapter) Compile(_ context.Context, request workload.CompileRequest) (executionplan.Plan, error) {
	if a.err != nil {
		return executionplan.Plan{}, a.err
	}
	a.compile = request
	if a.plan != nil {
		return a.plan.DeepCopy(), nil
	}
	return executionplan.Plan{AdapterID: "PyTorchJob", AdapterVersion: "v1", CompiledHash: "compiled", Objects: []agentruntime.DesiredObject{desiredObject()}, Discovery: executionplan.InstanceDiscovery{Roots: []string{"workload"}}}, nil
}
func (a *adapter) Observe(_ context.Context, request workload.ObserveRequest) (workload.Observation, error) {
	if a.err != nil {
		return workload.Observation{}, a.err
	}
	a.observe = request
	return workload.Observation{Phase: "Running", Reason: "Ready", Message: "running"}, nil
}
func (a *adapter) Inventory(_ context.Context, request workload.InventoryRequest) (workload.Inventory, error) {
	if a.err != nil {
		return workload.Inventory{}, a.err
	}
	return workload.Inventory{Candidates: []agentruntime.ObjectCandidate{{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Service"}, Namespace: "default", Name: request.Task.Logical.Name + "-head", Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: request.Task.ObjectUID}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup}}}, nil
}

func TestClientServerRoundTrip(t *testing.T) {
	implementation := &adapter{}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.UnaryInterceptor(executiongrpc.UnaryServerErrorInterceptor))
	workloadv1.RegisterWorkloadAdapterServer(server, workloadgrpc.NewServer(implementation))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithUnaryInterceptor(executiongrpc.UnaryClientErrorInterceptor))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := workloadgrpc.NewClient(workloadv1.NewWorkloadAdapterClient(connection))
	ctx := context.Background()
	info, err := client.Info(ctx)
	if err != nil || info.ID != "PyTorchJob" {
		t.Fatalf("Info()=%#v, %v", info, err)
	}
	if len(info.Capabilities) != 2 {
		t.Fatalf("Info capabilities=%v", info.Capabilities)
	}
	task := coordination.TaskSnapshot{
		Identity: agentruntime.TaskIdentity{
			Logical: agentruntime.LogicalTaskID{
				Namespace: "cluster",
				Name:      "task",
			},
			ObjectUID: "uid",
		},
		RuntimeSelector: coordination.RuntimeSelector{Kind: "Kubernetes"},
		WorkloadSelector: coordination.WorkloadSelector{
			Type:    "PyTorchJob",
			Version: "v1",
		},
		CarrierSelector: coordination.CarrierSelector{
			Type:    "rlinf.io/kubernetes-job",
			Version: "v1",
		},
		Configuration: coordination.WorkloadConfiguration{
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:  "main",
				Image: "example:v1",
			}}}},
			Parameters: []byte(`{"workers":2}`),
			Carrier: coordination.VersionedConfiguration{
				Type:       "rlinf.io/kubernetes-job",
				Version:    "v1",
				Parameters: []byte(`{"completions":2}`),
			},
		},
	}
	plan, err := client.Compile(ctx, workload.CompileRequest{Task: task, Runtime: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}})
	if err != nil {
		t.Fatal(err)
	}
	configuration := implementation.compile.Task.Configuration
	if plan.CompiledHash != "compiled" {
		t.Fatalf("compiled hash = %q", plan.CompiledHash)
	}
	if implementation.compile.Task.WorkloadSelector.Type != "PyTorchJob" {
		t.Fatalf("workload selector = %#v", implementation.compile.Task.WorkloadSelector)
	}
	if implementation.compile.Task.CarrierSelector.Type != "rlinf.io/kubernetes-job" {
		t.Fatalf("carrier selector = %#v", implementation.compile.Task.CarrierSelector)
	}
	if len(configuration.Template.Spec.Containers) != 1 || configuration.Template.Spec.Containers[0].Image != "example:v1" {
		t.Fatalf("template = %#v", configuration.Template)
	}
	if string(configuration.Parameters) != `{"workers":2}` {
		t.Fatalf("workload parameters = %s", configuration.Parameters)
	}
	if configuration.Carrier.Type != "rlinf.io/kubernetes-job" || configuration.Carrier.Version != "v1" {
		t.Fatalf("carrier = %#v", configuration.Carrier)
	}
	if string(configuration.Carrier.Parameters) != `{"completions":2}` {
		t.Fatalf("carrier parameters = %s", configuration.Carrier.Parameters)
	}
	ref := agentruntime.ObjectRef{RuntimeID: "runtime", ID: "job", Namespace: "default", Name: "task", UID: "job-uid", LogicalTask: task.Identity.Logical}
	index, rank := int32(1), int32(2)
	objects := agentruntime.ObjectSnapshot{Revision: "objects-1", Objects: []agentruntime.Object{{Ref: ref, Manifest: agentruntime.TypedManifest{Type: desiredObject().Kind, Object: []byte(`{"kind":"Job"}`)}}}}
	instances := agentruntime.InstanceSnapshot{Revision: "instances-1", Instances: []agentruntime.Instance{{Ref: agentruntime.InstanceRef{RuntimeID: "runtime", ID: "pod", UID: "pod-uid", Parent: ref}, Task: task.Identity, Name: "worker-1", Role: "worker", Index: &index, Rank: &rank, Phase: "Running", Reason: "Ready", Message: "ready"}}}
	observation, err := client.Observe(ctx, workload.ObserveRequest{Task: task, Plan: executionplan.ExecutablePlan{Digests: executionplan.Digests{CompiledHash: "compiled", ExecutableHash: "executable"}, Objects: []agentruntime.DesiredObject{desiredObject()}}, Objects: objects, Instances: instances})
	if err != nil {
		t.Fatal(err)
	}
	if observation.Phase != "Running" || implementation.observe.Objects.Revision != "objects-1" || len(implementation.observe.Objects.Objects) != 1 || implementation.observe.Objects.Objects[0].Ref != ref || implementation.observe.Instances.Revision != "instances-1" || len(implementation.observe.Instances.Instances) != 1 || implementation.observe.Instances.Instances[0].Index == nil || *implementation.observe.Instances.Instances[0].Index != 1 {
		t.Fatalf("unexpected observation round trip: %#v %#v", observation, implementation.observe)
	}
	inventory, err := client.Inventory(ctx, workload.InventoryRequest{Task: task.Identity, Runtime: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}})
	if err != nil || len(inventory.Candidates) != 1 || inventory.Candidates[0].Name != "task-head" || inventory.Candidates[0].Ownership.OwnerKey != "uid" {
		t.Fatalf("Inventory()=%#v, %v", inventory, err)
	}
}

func TestClientServerPropagatesAdapterErrors(t *testing.T) {
	implementation := &adapter{err: fault.Wrap(fault.Unavailable, "adapter unavailable", errors.New("offline"))}
	client, cleanup := newTestClient(t, implementation)
	defer cleanup()
	task := coordination.TaskSnapshot{Identity: agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "cluster", Name: "task"}}, RuntimeSelector: coordination.RuntimeSelector{Kind: "Kubernetes"}, WorkloadSelector: coordination.WorkloadSelector{Type: "PyTorchJob", Version: "v1"}}
	if _, err := client.Compile(context.Background(), workload.CompileRequest{Task: task, Runtime: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}); !fault.Is(err, fault.Unavailable) {
		t.Fatalf("Compile() error = %v", err)
	}
	if _, err := client.Observe(context.Background(), workload.ObserveRequest{Task: task}); !fault.Is(err, fault.Unavailable) {
		t.Fatalf("Observe() error = %v", err)
	}
}

func TestRemoteCompileTransportsEnvironmentPlanWithWireTargets(t *testing.T) {
	locator := agentruntime.ObjectLocator{Type: agentruntime.ObjectKind{Version: "v1", Kind: "PodTemplate"}, Path: ".spec.template"}
	plan := executionplan.Plan{
		Environment: executionplan.Requirements{
			Environments: []executionplan.EnvironmentRequirement{{
				Name:      "credential",
				Type:      "example.io/credential",
				Version:   "v1",
				Consumers: []executionplan.ConsumerSelector{{Targets: []string{"pod"}}},
			}},
		},
		Targets:   []executionplan.MutationTarget{{Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Locator: locator}},
		Units:     []executionplan.WorkloadUnit{{Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Locator: locator}},
		Processes: []executionprocess.Declaration{{Name: "primary", Target: "pod", Locator: locator, ContainerName: "main", Role: executionprocess.RolePrimary, Executable: "/rlark-tools/rlark-tools", Args: []string{"idle"}, RestartPolicy: executionprocess.RestartNever, Required: true}},
	}
	client, cleanup := newTestClient(t, &adapter{plan: &plan})
	defer cleanup()
	task := coordination.TaskSnapshot{Identity: agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "cluster", Name: "task"}}, RuntimeSelector: coordination.RuntimeSelector{Kind: "Kubernetes"}, WorkloadSelector: coordination.WorkloadSelector{Type: "PyTorchJob", Version: "v1"}}
	got, err := client.Compile(context.Background(), workload.CompileRequest{Task: task, Runtime: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Environment.Environments) != 1 || len(got.Targets) != 1 || got.Targets[0].Name != "pod" || len(got.Units) != 1 || len(got.Processes) != 1 || got.Processes[0].Executable != "/rlark-tools/rlark-tools" {
		t.Fatalf("Compile() plan=%#v", got)
	}
}

func TestClientRestoresErrorsWithoutInterceptor(t *testing.T) {
	implementation := &adapter{err: fault.New(fault.Unavailable, "adapter unavailable")}
	client, cleanup := newTestClientWithInterceptor(t, implementation, false)
	defer cleanup()
	task := coordination.TaskSnapshot{Identity: agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "cluster", Name: "task"}}, RuntimeSelector: coordination.RuntimeSelector{Kind: "Kubernetes"}, WorkloadSelector: coordination.WorkloadSelector{Type: "PyTorchJob", Version: "v1"}}
	if _, err := client.Compile(context.Background(), workload.CompileRequest{Task: task, Runtime: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}); !fault.Is(err, fault.Unavailable) {
		t.Fatalf("Compile() error = %v", err)
	}
	if _, err := client.Observe(context.Background(), workload.ObserveRequest{Task: task}); !fault.Is(err, fault.Unavailable) {
		t.Fatalf("Observe() error = %v", err)
	}
}

func TestServerRejectsUnsupportedProtocol(t *testing.T) {
	server := workloadgrpc.NewServer(&adapter{})
	if _, err := server.GetInfo(context.Background(), &workloadv1.GetInfoRequest{}); err == nil {
		t.Fatal("GetInfo() error = nil")
	}
}

func TestServerRejectsInventoryWithoutCapability(t *testing.T) {
	server := workloadgrpc.NewServer(&legacyAdapter{})
	task := &commonv1.TaskIdentity{LogicalId: &commonv1.LogicalTaskId{Namespace: "cluster", Name: "task"}}
	if _, err := server.Inventory(context.Background(), &workloadv1.InventoryRequest{Task: task, Runtime: &workloadv1.RuntimeInfo{Id: "runtime", Kind: "Kubernetes"}}); !fault.Is(fault.FromGRPC(err), fault.FailedPrecondition) {
		t.Fatalf("Inventory() error=%v, want failed precondition", err)
	}
}

type legacyAdapter struct{ implementation adapter }

func (a *legacyAdapter) Info(ctx context.Context) (workload.TypeInfo, error) {
	return a.implementation.Info(ctx)
}
func (a *legacyAdapter) Compile(ctx context.Context, request workload.CompileRequest) (executionplan.Plan, error) {
	return a.implementation.Compile(ctx, request)
}
func (a *legacyAdapter) Observe(ctx context.Context, request workload.ObserveRequest) (workload.Observation, error) {
	return a.implementation.Observe(ctx, request)
}

func TestServerRejectsMalformedObservationWithoutPanic(t *testing.T) {
	server := workloadgrpc.NewServer(&adapter{})
	task := &workloadv1.TaskSnapshot{
		Identity:                &commonv1.TaskIdentity{LogicalId: &commonv1.LogicalTaskId{Namespace: "cluster", Name: "task"}},
		Configuration:           &commonv1.TypedValue{Type: &commonv1.TypeReference{Group: "execution.rlark.io", Version: "v1", Kind: "WorkloadConfiguration"}, Value: []byte(`{}`), ContentType: "application/json"},
		EnvironmentRequirements: &commonv1.TypedValue{Type: &commonv1.TypeReference{Group: "execution.rlark.io", Version: "v1", Kind: "EnvironmentRequirements"}, Value: []byte(`{}`), ContentType: "application/json"},
	}
	plan := &workloadv1.ExecutablePlan{Digests: &workloadv1.PlanDigests{}, UpdatePolicy: &commonv1.TypedValue{Type: &commonv1.TypeReference{Group: "execution.rlark.io", Version: "v1", Kind: "UpdatePolicy"}, Value: []byte(`{}`), ContentType: "application/json"}}
	request := &workloadv1.ObserveRequest{Task: task, Plan: plan, Objects: &commonv1.ObjectSnapshot{Objects: []*commonv1.RuntimeObject{nil}}, Instances: &commonv1.InstanceSnapshot{}}
	if _, err := server.Observe(context.Background(), request); !fault.Is(fault.FromGRPC(err), fault.InvalidArgument) {
		t.Fatalf("Observe() error = %v", err)
	}
}

func TestServerRejectsNonJSONTypedValues(t *testing.T) {
	server := workloadgrpc.NewServer(&adapter{})
	task := &workloadv1.TaskSnapshot{
		Identity:                &commonv1.TaskIdentity{LogicalId: &commonv1.LogicalTaskId{Namespace: "cluster", Name: "task"}},
		Configuration:           &commonv1.TypedValue{Type: &commonv1.TypeReference{Group: "execution.rlark.io", Version: "v1", Kind: "WorkloadConfiguration"}, Value: []byte(`{}`), ContentType: "application/yaml"},
		EnvironmentRequirements: &commonv1.TypedValue{Type: &commonv1.TypeReference{Group: "execution.rlark.io", Version: "v1", Kind: "EnvironmentRequirements"}, Value: []byte(`{}`), ContentType: "application/json"},
	}
	if _, err := server.Compile(context.Background(), &workloadv1.CompileRequest{Task: task, Runtime: &workloadv1.RuntimeInfo{Id: "runtime", Kind: "Kubernetes"}}); !fault.Is(fault.FromGRPC(err), fault.InvalidArgument) {
		t.Fatalf("Compile() error = %v", err)
	}
}

func newTestClient(t *testing.T, implementation workload.Adapter) (*workloadgrpc.Client, func()) {
	return newTestClientWithInterceptor(t, implementation, true)
}

func newTestClientWithInterceptor(t *testing.T, implementation workload.Adapter, clientInterceptor bool) (*workloadgrpc.Client, func()) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.UnaryInterceptor(executiongrpc.UnaryServerErrorInterceptor))
	workloadv1.RegisterWorkloadAdapterServer(server, workloadgrpc.NewServer(implementation))
	go func() { _ = server.Serve(listener) }()
	options := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() })}
	if clientInterceptor {
		options = append(options, grpc.WithUnaryInterceptor(executiongrpc.UnaryClientErrorInterceptor))
	}
	connection, err := grpc.NewClient("passthrough:///bufnet", options...)
	if err != nil {
		server.Stop()
		t.Fatal(err)
	}
	return workloadgrpc.NewClient(workloadv1.NewWorkloadAdapterClient(connection)), func() {
		_ = connection.Close()
		server.Stop()
	}
}

func desiredObject() agentruntime.DesiredObject {
	return agentruntime.DesiredObject{LogicalName: "workload", Kind: agentruntime.ObjectKind{Group: "batch", Version: "v1", Kind: "Job"}, Manifest: agentruntime.TypedManifest{Type: agentruntime.ObjectKind{Group: "batch", Version: "v1", Kind: "Job"}, Object: []byte(`{"kind":"Job"}`)}, Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityLogicalTask, OwnerKey: "cluster/task"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace, OwnedFields: []string{"data"}}
}

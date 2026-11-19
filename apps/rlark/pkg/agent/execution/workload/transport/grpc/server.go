package grpc

import (
	"context"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
	commonv1 "github.com/rlinf/rlark/sdks/execution-go/gen/common/v1"
	workloadv1 "github.com/rlinf/rlark/sdks/execution-go/gen/workload/v1"
)

// Server exposes a workload.Adapter through the generated gRPC service.
type Server struct {
	workloadv1.UnimplementedWorkloadAdapterServer
	adapter workload.Adapter
}

// NewServer constructs a gRPC adapter for a Workload Adapter.
func NewServer(adapter workload.Adapter) *Server { return &Server{adapter: adapter} }

// GetInfo negotiates protocol compatibility and reports the Workload contract.
func (s *Server) GetInfo(ctx context.Context, request *workloadv1.GetInfoRequest) (*workloadv1.GetInfoResponse, error) {
	if !supportsProtocol(request.GetSupportedProtocolVersions()) {
		return nil, fault.ToGRPC(fault.New(fault.InvalidArgument, fmt.Sprintf("client does not support protocol major version %d", protocolMajor)))
	}
	info, err := s.adapter.Info(ctx)
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	wire := &workloadv1.WorkloadTypeInfo{Id: string(info.ID), Version: info.Version, Configuration: &workloadv1.ConfigurationContract{Version: info.Configuration.Version, Schema: info.Configuration.Schema, SchemaContentType: "application/schema+json"}, ProtocolVersion: &commonv1.ProtocolVersion{Major: protocolMajor, Minor: protocolMinor}}
	for _, kind := range info.SupportedRuntimeKinds {
		wire.SupportedRuntimeKinds = append(wire.SupportedRuntimeKinds, string(kind))
	}
	for _, capability := range info.Capabilities {
		wire.Capabilities = append(wire.Capabilities, toWireCapability(capability))
	}
	return &workloadv1.GetInfoResponse{Info: wire}, nil
}

// Inventory returns identity-derived cleanup candidates without compiling Spec.
func (s *Server) Inventory(ctx context.Context, request *workloadv1.InventoryRequest) (*workloadv1.InventoryResponse, error) {
	adapter, ok := s.adapter.(workload.InventoryAdapter)
	if !ok {
		return nil, fault.ToGRPC(fault.New(fault.FailedPrecondition, "Workload Adapter does not support identity inventory"))
	}
	task, err := fromWireTaskIdentity(request.GetTask())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	if request.GetRuntime() == nil {
		return nil, fault.ToGRPC(fault.New(fault.InvalidArgument, "runtime info is required"))
	}
	inventory, err := adapter.Inventory(ctx, workload.InventoryRequest{Task: task, Runtime: agentruntime.Info{ID: agentruntime.ID(request.GetRuntime().GetId()), Kind: agentruntime.Kind(request.GetRuntime().GetKind())}})
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	response := &workloadv1.InventoryResponse{}
	for _, candidate := range inventory.Candidates {
		response.Candidates = append(response.Candidates, toWireCandidate(candidate))
	}
	return response, nil
}

// Compile validates wire input and compiles a declarative Workload plan.
func (s *Server) Compile(ctx context.Context, request *workloadv1.CompileRequest) (*workloadv1.CompileResponse, error) {
	task, err := fromWireTask(request.GetTask())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	if request.GetRuntime() == nil {
		return nil, fault.ToGRPC(fault.New(fault.InvalidArgument, "runtime info is required"))
	}
	plan, err := s.adapter.Compile(ctx, workload.CompileRequest{Task: task, Runtime: agentruntime.Info{ID: agentruntime.ID(request.GetRuntime().GetId()), Kind: agentruntime.Kind(request.GetRuntime().GetKind())}})
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	wire, err := toWirePlan(plan)
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	return &workloadv1.CompileResponse{Plan: wire}, nil
}

// Observe validates complete snapshots and projects normalized Workload state.
func (s *Server) Observe(ctx context.Context, request *workloadv1.ObserveRequest) (*workloadv1.ObserveResponse, error) {
	task, err := fromWireTask(request.GetTask())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	plan, err := fromWireExecutable(request.GetPlan())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	objects, err := fromWireObjectSnapshot(request.GetObjects())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	instances, err := fromWireInstanceSnapshot(request.GetInstances())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	observation, err := s.adapter.Observe(ctx, workload.ObserveRequest{Task: task, Plan: plan, Objects: objects, Instances: instances})
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	return &workloadv1.ObserveResponse{Observation: &workloadv1.Observation{Phase: string(observation.Phase), Reason: observation.Reason, Message: observation.Message}}, nil
}

func supportsProtocol(versions []*commonv1.ProtocolVersion) bool {
	for _, version := range versions {
		if version.GetMajor() == protocolMajor {
			return true
		}
	}
	return false
}

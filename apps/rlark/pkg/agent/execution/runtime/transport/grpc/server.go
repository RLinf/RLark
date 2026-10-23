package grpc

import (
	"context"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	commonv1 "github.com/rlinf/rlark/sdks/execution-go/gen/common/v1"
	runtimev1 "github.com/rlinf/rlark/sdks/execution-go/gen/runtime/v1"
)

// Server exposes a runtime.Provider through the generated gRPC service.
type Server struct {
	runtimev1.UnimplementedRuntimeProviderServer
	provider agentruntime.Provider
}

// NewServer constructs a gRPC adapter for provider.
func NewServer(provider agentruntime.Provider) *Server { return &Server{provider: provider} }

// GetInfo negotiates protocol compatibility and reports Runtime identity.
func (s *Server) GetInfo(ctx context.Context, request *runtimev1.GetInfoRequest) (*runtimev1.GetInfoResponse, error) {
	if !supportsProtocol(request.GetSupportedProtocolVersions()) {
		return nil, fault.ToGRPC(fault.New(fault.InvalidArgument, fmt.Sprintf("client does not support protocol major version %d", protocolMajor)))
	}
	info, err := s.provider.Info(ctx)
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	capabilities := make([]commonv1.ProtocolCapability, 0, len(info.Capabilities))
	for _, capability := range info.Capabilities {
		capabilities = append(capabilities, toWireCapability(capability))
	}
	return &runtimev1.GetInfoResponse{Info: &runtimev1.RuntimeInfo{Id: string(info.ID), Kind: string(info.Kind), ProtocolVersion: &commonv1.ProtocolVersion{Major: protocolMajor, Minor: protocolMinor}, Capabilities: capabilities}}, nil
}

// GetObject retrieves one fenced Runtime object.
func (s *Server) GetObject(ctx context.Context, request *runtimev1.GetObjectRequest) (*runtimev1.GetObjectResponse, error) {
	ref, err := fromWireObjectRef(request.GetReference())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	object, err := s.provider.Get(ctx, ref)
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	return &runtimev1.GetObjectResponse{Object: toWireObject(object)}, nil
}

// ListManagedObjects returns a complete managed-object snapshot for a Task.
func (s *Server) ListManagedObjects(ctx context.Context, request *runtimev1.ListManagedObjectsRequest) (*runtimev1.ListManagedObjectsResponse, error) {
	task, err := fromWireTask(request.GetTask())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	snapshot, err := s.provider.ListManaged(ctx, task)
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	response := &runtimev1.ListManagedObjectsResponse{Revision: snapshot.Revision}
	for _, object := range snapshot.Objects {
		response.Objects = append(response.Objects, toWireObject(object))
	}
	return response, nil
}

// ApplyObject validates and applies one desired Runtime object.
func (s *Server) ApplyObject(ctx context.Context, request *runtimev1.ApplyObjectRequest) (*runtimev1.ApplyObjectResponse, error) {
	task, err := fromWireTask(request.GetTask())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	desired, err := fromWireDesired(request.GetDesired())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	apply := agentruntime.ApplyRequest{IdempotencyKey: request.GetIdempotencyKey(), Task: task, Desired: desired}
	if request.GetCurrent() != nil {
		current, err := fromWireObjectRef(request.GetCurrent())
		if err != nil {
			return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
		}
		apply.Current = &current
	}
	for _, owner := range request.GetProvenOwners() {
		converted, err := fromWireObjectRef(owner)
		if err != nil {
			return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
		}
		apply.ProvenOwners = append(apply.ProvenOwners, converted)
	}
	result, err := s.provider.Apply(ctx, apply)
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	return &runtimev1.ApplyObjectResponse{Object: toWireObject(result.Object)}, nil
}

// DeleteObject validates and deletes one ownership-fenced Runtime object.
func (s *Server) DeleteObject(ctx context.Context, request *runtimev1.DeleteObjectRequest) (*runtimev1.DeleteObjectResponse, error) {
	task, err := fromWireTask(request.GetTask())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	ref, err := fromWireObjectRef(request.GetReference())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	if err := s.provider.Delete(ctx, agentruntime.DeleteRequest{IdempotencyKey: request.GetIdempotencyKey(), Task: task, Ref: ref}); err != nil {
		return nil, fault.ToGRPC(err)
	}
	return &runtimev1.DeleteObjectResponse{}, nil
}

// ListInstances returns a complete instance snapshot for the requested roots.
func (s *Server) ListInstances(ctx context.Context, request *runtimev1.ListInstancesRequest) (*runtimev1.ListInstancesResponse, error) {
	task, err := fromWireTask(request.GetTask())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	query := agentruntime.InstanceQuery{Task: task, Roots: append([]string(nil), request.GetRoots()...)}
	for _, workload := range request.GetWorkloads() {
		ref, err := fromWireObjectRef(workload)
		if err != nil {
			return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
		}
		query.Workloads = append(query.Workloads, ref)
	}
	if request.GetOptions() != nil {
		query.Options = &agentruntime.TypedOptions{Type: fromWireType(request.GetOptions().GetType()), Value: append([]byte(nil), request.GetOptions().GetValue()...)}
	}
	snapshot, err := s.provider.ListInstances(ctx, query)
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	response := &runtimev1.ListInstancesResponse{Revision: snapshot.Revision}
	for _, instance := range snapshot.Instances {
		response.Instances = append(response.Instances, toWireInstance(instance))
	}
	return response, nil
}

// ListCandidateObjects returns a bounded Adapter-directed object snapshot.
func (s *Server) ListCandidateObjects(ctx context.Context, request *runtimev1.ListCandidateObjectsRequest) (*runtimev1.ListCandidateObjectsResponse, error) {
	provider, ok := s.provider.(agentruntime.InventoryProvider)
	if !ok {
		return nil, fault.ToGRPC(fault.New(fault.FailedPrecondition, "Runtime Provider does not support candidate inventory"))
	}
	task, err := fromWireTask(request.GetTask())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	query := agentruntime.ManagedObjectQuery{Task: task}
	for _, candidate := range request.GetCandidates() {
		converted, err := fromWireCandidate(candidate)
		if err != nil {
			return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
		}
		query.Candidates = append(query.Candidates, converted)
	}
	snapshot, err := provider.ListCandidates(ctx, query)
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	response := &runtimev1.ListCandidateObjectsResponse{Revision: snapshot.Revision}
	for _, object := range snapshot.Objects {
		response.Objects = append(response.Objects, toWireObject(object))
	}
	return response, nil
}

// GetClaimedWorkload recovers a legacy claim's Workload identity.
func (s *Server) GetClaimedWorkload(ctx context.Context, request *runtimev1.GetClaimedWorkloadRequest) (*runtimev1.GetClaimedWorkloadResponse, error) {
	provider, ok := s.provider.(agentruntime.ClaimedWorkloadProvider)
	if !ok {
		return nil, fault.ToGRPC(fault.New(fault.FailedPrecondition, "Runtime Provider does not support claimed Workload recovery"))
	}
	task, err := fromWireTask(request.GetTask())
	if err != nil {
		return nil, fault.ToGRPC(fault.Wrap(fault.InvalidArgument, err.Error(), err))
	}
	workloadType, workloadVersion, err := provider.ClaimedWorkload(ctx, task)
	if err != nil {
		return nil, fault.ToGRPC(err)
	}
	return &runtimev1.GetClaimedWorkloadResponse{WorkloadType: workloadType, WorkloadVersion: workloadVersion}, nil
}

func supportsProtocol(versions []*commonv1.ProtocolVersion) bool {
	for _, version := range versions {
		if version.GetMajor() == protocolMajor {
			return true
		}
	}
	return false
}

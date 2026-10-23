package grpc

import (
	"context"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
	commonv1 "github.com/rlinf/rlark/sdks/execution-go/gen/common/v1"
	workloadv1 "github.com/rlinf/rlark/sdks/execution-go/gen/workload/v1"
)

const (
	protocolMajor = 2
	protocolMinor = 0
)

// Client adapts a generated gRPC WorkloadAdapter client to workload.Adapter.
type Client struct {
	client workloadv1.WorkloadAdapterClient
}

var _ workload.Adapter = (*Client)(nil)
var _ workload.InventoryAdapter = (*Client)(nil)

// NewClient constructs a Workload Adapter backed by a gRPC client.
func NewClient(client workloadv1.WorkloadAdapterClient) *Client { return &Client{client: client} }

// Info negotiates the protocol and returns the remote Workload Type contract.
func (c *Client) Info(ctx context.Context) (workload.TypeInfo, error) {
	response, err := c.client.GetInfo(ctx, &workloadv1.GetInfoRequest{SupportedProtocolVersions: []*commonv1.ProtocolVersion{{Major: protocolMajor, Minor: protocolMinor}}})
	if err != nil {
		return workload.TypeInfo{}, fault.FromGRPC(err)
	}
	info := response.GetInfo()
	if info == nil || info.GetProtocolVersion().GetMajor() != protocolMajor || info.GetConfiguration() == nil {
		return workload.TypeInfo{}, fmt.Errorf("workload adapter returned invalid info")
	}
	result := workload.TypeInfo{ID: workload.TypeID(info.GetId()), Version: info.GetVersion(), Configuration: workload.ConfigurationContract{Version: info.GetConfiguration().GetVersion(), Schema: append([]byte(nil), info.GetConfiguration().GetSchema()...)}}
	for _, kind := range info.GetSupportedRuntimeKinds() {
		result.SupportedRuntimeKinds = append(result.SupportedRuntimeKinds, agentruntime.Kind(kind))
	}
	for _, capability := range info.GetCapabilities() {
		converted, err := fromWireCapability(capability)
		if err == nil {
			result.Capabilities = append(result.Capabilities, converted)
		}
	}
	return result, nil
}

// Inventory asks the remote Adapter for identity-derived cleanup candidates.
func (c *Client) Inventory(ctx context.Context, request workload.InventoryRequest) (workload.Inventory, error) {
	response, err := c.client.Inventory(ctx, &workloadv1.InventoryRequest{Task: toWireTaskIdentity(request.Task), Runtime: &workloadv1.RuntimeInfo{Id: string(request.Runtime.ID), Kind: string(request.Runtime.Kind)}})
	if err != nil {
		return workload.Inventory{}, fault.FromGRPC(err)
	}
	result := workload.Inventory{}
	for _, candidate := range response.GetCandidates() {
		converted, err := fromWireCandidate(candidate)
		if err != nil {
			return workload.Inventory{}, err
		}
		result.Candidates = append(result.Candidates, converted)
	}
	return result, nil
}

// Compile asks the remote Adapter to compile a Task into a declarative plan.
func (c *Client) Compile(ctx context.Context, request workload.CompileRequest) (executionplan.Plan, error) {
	task, err := toWireTask(request.Task)
	if err != nil {
		return executionplan.Plan{}, err
	}
	response, err := c.client.Compile(ctx, &workloadv1.CompileRequest{Task: task, Runtime: &workloadv1.RuntimeInfo{Id: string(request.Runtime.ID), Kind: string(request.Runtime.Kind)}})
	if err != nil {
		return executionplan.Plan{}, fault.FromGRPC(err)
	}
	return fromWirePlan(response.GetPlan())
}

// Observe asks the remote Adapter to project current Runtime snapshots.
func (c *Client) Observe(ctx context.Context, request workload.ObserveRequest) (workload.Observation, error) {
	task, err := toWireTask(request.Task)
	if err != nil {
		return workload.Observation{}, err
	}
	plan, err := toWireExecutable(request.Plan)
	if err != nil {
		return workload.Observation{}, err
	}
	response, err := c.client.Observe(ctx, &workloadv1.ObserveRequest{Task: task, Plan: plan, Objects: toWireObjectSnapshot(request.Objects), Instances: toWireInstanceSnapshot(request.Instances)})
	if err != nil {
		return workload.Observation{}, fault.FromGRPC(err)
	}
	observation := response.GetObservation()
	if observation == nil {
		return workload.Observation{}, fmt.Errorf("workload adapter returned no observation")
	}
	return workload.Observation{Phase: workload.Phase(observation.GetPhase()), Reason: observation.GetReason(), Message: observation.GetMessage()}, nil
}

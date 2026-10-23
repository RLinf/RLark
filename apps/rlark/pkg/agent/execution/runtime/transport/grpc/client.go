package grpc

import (
	"context"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	commonv1 "github.com/rlinf/rlark/sdks/execution-go/gen/common/v1"
	runtimev1 "github.com/rlinf/rlark/sdks/execution-go/gen/runtime/v1"
)

const (
	protocolMajor = 2
	protocolMinor = 0
)

// Client adapts a generated gRPC RuntimeProvider client to runtime.Provider.
type Client struct {
	client runtimev1.RuntimeProviderClient
}

var _ agentruntime.Provider = (*Client)(nil)

// NewClient constructs a Runtime Provider backed by a gRPC client.
func NewClient(client runtimev1.RuntimeProviderClient) *Client { return &Client{client: client} }

// Info negotiates the protocol and returns the remote Runtime identity.
func (c *Client) Info(ctx context.Context) (agentruntime.Info, error) {
	response, err := c.client.GetInfo(ctx, &runtimev1.GetInfoRequest{SupportedProtocolVersions: []*commonv1.ProtocolVersion{{Major: protocolMajor, Minor: protocolMinor}}})
	if err != nil {
		return agentruntime.Info{}, fault.FromGRPC(err)
	}
	if response.GetInfo() == nil || response.GetInfo().GetProtocolVersion().GetMajor() != protocolMajor {
		return agentruntime.Info{}, fmt.Errorf("runtime provider returned unsupported protocol version")
	}
	capabilities := make([]agentruntime.Capability, 0, len(response.GetInfo().GetCapabilities()))
	for _, capability := range response.GetInfo().GetCapabilities() {
		converted, err := fromWireCapability(capability)
		if err != nil {
			continue
		}
		capabilities = append(capabilities, converted)
	}
	return agentruntime.Info{ID: agentruntime.ID(response.GetInfo().GetId()), Kind: agentruntime.Kind(response.GetInfo().GetKind()), Capabilities: capabilities}, nil
}

// Get retrieves one Runtime object by its fenced reference.
func (c *Client) Get(ctx context.Context, ref agentruntime.ObjectRef) (agentruntime.Object, error) {
	response, err := c.client.GetObject(ctx, &runtimev1.GetObjectRequest{Reference: toWireObjectRef(ref)})
	if err != nil {
		return agentruntime.Object{}, fault.FromGRPC(err)
	}
	return fromWireObject(response.GetObject())
}

// ListManaged returns the remote Provider's complete managed-object snapshot.
func (c *Client) ListManaged(ctx context.Context, task agentruntime.TaskIdentity) (agentruntime.ObjectSnapshot, error) {
	response, err := c.client.ListManagedObjects(ctx, &runtimev1.ListManagedObjectsRequest{Task: toWireTask(task)})
	if err != nil {
		return agentruntime.ObjectSnapshot{}, fault.FromGRPC(err)
	}
	objects := make([]agentruntime.Object, 0, len(response.GetObjects()))
	for _, object := range response.GetObjects() {
		converted, err := fromWireObject(object)
		if err != nil {
			return agentruntime.ObjectSnapshot{}, err
		}
		objects = append(objects, converted)
	}
	return agentruntime.ObjectSnapshot{Revision: response.GetRevision(), Objects: objects}, nil
}

// Apply forwards an idempotent object apply to the remote Provider.
func (c *Client) Apply(ctx context.Context, request agentruntime.ApplyRequest) (agentruntime.ApplyResult, error) {
	wire := &runtimev1.ApplyObjectRequest{IdempotencyKey: request.IdempotencyKey, Task: toWireTask(request.Task), Desired: toWireDesired(request.Desired)}
	if request.Current != nil {
		wire.Current = toWireObjectRef(*request.Current)
	}
	for _, owner := range request.ProvenOwners {
		wire.ProvenOwners = append(wire.ProvenOwners, toWireObjectRef(owner))
	}
	response, err := c.client.ApplyObject(ctx, wire)
	if err != nil {
		return agentruntime.ApplyResult{}, fault.FromGRPC(err)
	}
	object, err := fromWireObject(response.GetObject())
	if err != nil {
		return agentruntime.ApplyResult{}, err
	}
	return agentruntime.ApplyResult{Ref: object.Ref, Object: object}, nil
}

// ListCandidates returns a complete snapshot for Adapter-declared candidates.
func (c *Client) ListCandidates(ctx context.Context, query agentruntime.ManagedObjectQuery) (agentruntime.ObjectSnapshot, error) {
	wire := &runtimev1.ListCandidateObjectsRequest{Task: toWireTask(query.Task)}
	for _, candidate := range query.Candidates {
		wire.Candidates = append(wire.Candidates, toWireCandidate(candidate))
	}
	response, err := c.client.ListCandidateObjects(ctx, wire)
	if err != nil {
		return agentruntime.ObjectSnapshot{}, fault.FromGRPC(err)
	}
	objects := make([]agentruntime.Object, 0, len(response.GetObjects()))
	for _, object := range response.GetObjects() {
		converted, err := fromWireObject(object)
		if err != nil {
			return agentruntime.ObjectSnapshot{}, err
		}
		objects = append(objects, converted)
	}
	return agentruntime.ObjectSnapshot{Revision: response.GetRevision(), Objects: objects}, nil
}

// ClaimedWorkload recovers one legacy claim's Workload implementation identity.
func (c *Client) ClaimedWorkload(ctx context.Context, task agentruntime.TaskIdentity) (string, string, error) {
	response, err := c.client.GetClaimedWorkload(ctx, &runtimev1.GetClaimedWorkloadRequest{Task: toWireTask(task)})
	if err != nil {
		return "", "", fault.FromGRPC(err)
	}
	return response.GetWorkloadType(), response.GetWorkloadVersion(), nil
}

var _ agentruntime.InventoryProvider = (*Client)(nil)
var _ agentruntime.ClaimedWorkloadProvider = (*Client)(nil)

// Delete forwards an idempotent, ownership-fenced deletion to the remote Provider.
func (c *Client) Delete(ctx context.Context, request agentruntime.DeleteRequest) error {
	_, err := c.client.DeleteObject(ctx, &runtimev1.DeleteObjectRequest{IdempotencyKey: request.IdempotencyKey, Task: toWireTask(request.Task), Reference: toWireObjectRef(request.Ref)})
	return fault.FromGRPC(err)
}

// ListInstances returns the complete remote instance snapshot for a query.
func (c *Client) ListInstances(ctx context.Context, query agentruntime.InstanceQuery) (agentruntime.InstanceSnapshot, error) {
	wire := &runtimev1.ListInstancesRequest{Task: toWireTask(query.Task), Roots: append([]string(nil), query.Roots...)}
	for _, workload := range query.Workloads {
		wire.Workloads = append(wire.Workloads, toWireObjectRef(workload))
	}
	if query.Options != nil {
		wire.Options = toWireValue(query.Options.Type, query.Options.Value)
	}
	response, err := c.client.ListInstances(ctx, wire)
	if err != nil {
		return agentruntime.InstanceSnapshot{}, fault.FromGRPC(err)
	}
	instances := make([]agentruntime.Instance, 0, len(response.GetInstances()))
	for _, instance := range response.GetInstances() {
		converted, err := fromWireInstance(instance)
		if err != nil {
			return agentruntime.InstanceSnapshot{}, err
		}
		instances = append(instances, converted)
	}
	return agentruntime.InstanceSnapshot{Revision: response.GetRevision(), Instances: instances}, nil
}

package coordinator

import (
	"context"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

// Embedded base interfaces deliberately hide optional inventory methods.
type noInventoryAdapter struct{ workload.Adapter }
type noInventoryProvider struct{ agentruntime.Provider }

func TestMandatoryInventoryDeclarationsAndInterfacesBeforeUse(t *testing.T) {
	for _, missing := range []string{"adapter declaration", "provider declaration", "adapter interface", "provider interface"} {
		t.Run(missing, func(t *testing.T) {
			provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}
			adapter := &fakeAdapter{}
			c := testCoordinator(provider, adapter)
			switch missing {
			case "adapter declaration":
				adapter.capabilities = nil
			case "provider declaration":
				c.Runtimes[0].Info.Capabilities = nil
			case "adapter interface":
				c.Workloads[0].Adapter = noInventoryAdapter{adapter}
			case "provider interface":
				c.Runtimes[0].Provider = noInventoryProvider{provider}
			}
			task := coordinatorTask()
			if err := c.ValidateCapabilities(context.Background(), task.RuntimeSelector, task.WorkloadSelector); !fault.Is(err, fault.FailedPrecondition) {
				t.Fatalf("assembly validation=%v", err)
			}
			if _, err := c.Reconcile(context.Background(), task); !fault.Is(err, fault.FailedPrecondition) {
				t.Fatalf("reconcile=%v", err)
			}
			if _, err := c.CleanupTask(context.Background(), task.Identity, task.RuntimeSelector, task.WorkloadSelector, task.CarrierSelector); !fault.Is(err, fault.FailedPrecondition) {
				t.Fatalf("cleanup=%v", err)
			}
			if provider.listManaged+provider.listInstances+provider.applies+provider.deletes+adapter.compiles != 0 {
				t.Fatal("invalid inventory composition was used")
			}
		})
	}
}

func TestOwnedProjectionNeverFallsBackToWholeObject(t *testing.T) {
	for _, missing := range []string{"adapter", "runtime", "shared fields"} {
		t.Run(missing, func(t *testing.T) {
			o := desired("config", "v1", "ConfigMap", "config", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"config"},"data":{"v":"new"}}`)
			o.Ownership = agentruntime.ResourceOwnership{Scope: agentruntime.IdentitySharedResource, OwnerKey: "shared-script"}
			o.OwnedFields = []string{"data.v"}
			p := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes", Capabilities: []agentruntime.Capability{agentruntime.CapabilityReconcileProfiles, agentruntime.CapabilityOwnedFieldProjection}}}
			a := &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{o}}, capabilities: []agentruntime.Capability{agentruntime.CapabilityReconcileProfiles, agentruntime.CapabilityOwnedFieldProjection}}
			c := testCoordinator(p, a)
			switch missing {
			case "adapter":
				a.capabilities = []agentruntime.Capability{agentruntime.CapabilityWorkloadInventory, agentruntime.CapabilityReconcileProfiles}
			case "runtime":
				c.Runtimes[0].Info.Capabilities = []agentruntime.Capability{agentruntime.CapabilityCandidateInventory, agentruntime.CapabilityReconcileProfiles}
			case "shared fields":
				a.plan.Objects[0].OwnedFields = nil
			}
			if _, err := c.Reconcile(context.Background(), coordinatorTask()); !fault.Is(err, fault.FailedPrecondition) {
				t.Fatalf("projection rejection=%v", err)
			}
			if p.listManaged+p.applies+p.deletes != 0 {
				t.Fatal("unsupported projection reached Provider")
			}
		})
	}
}

func TestAdmissionDoesNotObserveStaleWorkloads(t *testing.T) {
	p := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}
	a := &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{desired("root", "v1", "ConfigMap", "root", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"root"}}`)}}}
	c := testCoordinator(p, a)
	c.Runtimes[0].Info.Capabilities = append(c.Runtimes[0].Info.Capabilities, "UnknownOptional")
	if err := c.AdmitTask(context.Background(), coordinatorTask()); err != nil {
		t.Fatal(err)
	}
	if p.listManaged+p.listInstances+p.applies+p.deletes != 0 || a.observes != 0 {
		t.Fatal("admission observed or mutated workload state")
	}
	// Ordinary execution must not require the optional legacy recovery method.
	if err := c.ValidateCapabilities(context.Background(), coordination.RuntimeSelector{ID: "runtime", Kind: "Kubernetes"}, coordinatorTask().WorkloadSelector); err != nil {
		t.Fatal(err)
	}
}

func TestDesiredAdoptionRequiresCapabilitiesEvenWithoutInventoryAdoption(t *testing.T) {
	for _, missing := range []string{"adapter", "runtime"} {
		t.Run(missing, func(t *testing.T) {
			o := desired("root", "v1", "ConfigMap", "root", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"root"}}`)
			o.Ownership.IncarnationPolicy = agentruntime.IncarnationOwnershipAllowAdoption
			p := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes", Capabilities: []agentruntime.Capability{agentruntime.CapabilityIncarnationAdoption, agentruntime.CapabilityReconcileProfiles}}}
			a := &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{o}}, capabilities: []agentruntime.Capability{agentruntime.CapabilityIncarnationAdoption, agentruntime.CapabilityReconcileProfiles}}
			c := testCoordinator(p, a)
			if missing == "adapter" {
				a.capabilities = []agentruntime.Capability{agentruntime.CapabilityWorkloadInventory, agentruntime.CapabilityReconcileProfiles}
			} else {
				c.Runtimes[0].Info.Capabilities = []agentruntime.Capability{agentruntime.CapabilityCandidateInventory, agentruntime.CapabilityReconcileProfiles}
			}
			if err := c.AdmitTask(context.Background(), coordinatorTask()); !fault.Is(err, fault.FailedPrecondition) {
				t.Fatalf("adoption admission=%v", err)
			}
			if _, err := c.Reconcile(context.Background(), coordinatorTask()); !fault.Is(err, fault.FailedPrecondition) {
				t.Fatalf("adoption reconcile=%v", err)
			}
			if p.applies+p.deletes+p.listManaged != 0 {
				t.Fatal("desired adoption without capability reached Provider")
			}
		})
	}
}

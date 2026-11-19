package coordination

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// RuntimeSelector selects one registered Provider by ID and implementation kind.
type RuntimeSelector struct {
	ID   agentruntime.ID
	Kind agentruntime.Kind
}

// WorkloadSelector selects one versioned Workload Adapter.
type WorkloadSelector struct {
	Type    string
	Version string
}

// CarrierSelector selects one Runtime-specific workload carrier contract.
type CarrierSelector struct {
	Type    string
	Version string
}

// WorkloadConfiguration is the canonical common workload input supplied to the
// selected Adapter. Workload semantics are carried by WorkloadSelector. The
// Adapter strictly validates and interprets Carrier and emits one complete Plan.
type WorkloadConfiguration struct {
	Template   corev1.PodTemplateSpec
	Parameters json.RawMessage
	Carrier    VersionedConfiguration
}

// VersionedConfiguration is an Adapter-owned extension envelope. Parameters
// remain opaque until the selected Adapter strictly decodes its type and version.
type VersionedConfiguration struct {
	Type       string
	Version    string
	Parameters json.RawMessage
}

// LocalEnvironmentDeclaration is the deep-copied management declaration used
// by trusted in-process assembly. It is intentionally absent from workload
// transport and portable Plans.
type LocalEnvironmentDeclaration struct {
	Type       string
	Version    string
	Parameters apiextensionsv1.JSON
}

// TaskSnapshot is the immutable execution input decoded from a management Task.
type TaskSnapshot struct {
	Identity          agentruntime.TaskIdentity
	RuntimeSelector   RuntimeSelector
	WorkloadSelector  WorkloadSelector
	CarrierSelector   CarrierSelector
	Configuration     WorkloadConfiguration
	Environment       executionplan.Requirements
	LocalEnvironments []LocalEnvironmentDeclaration
	Deleting          bool
}

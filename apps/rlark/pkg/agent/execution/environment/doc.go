// Package environment defines the in-process boundary between portable
// Workload requirements and Runtime-specific plan binding.
//
// A Workload Adapter declares requirements and explicit MutationTargets. A
// Resolver validates those declarations and returns concrete, non-secret
// binding metadata. A PlanBinder then applies Runtime-specific mutations to a
// deep copy of the compiled plan before the Coordinator reads or writes Runtime
// state. Targets authorize locations; they do not authorize arbitrary JSONPath
// mutation or Provider-side inference.
//
// Secret material must not be placed in portable requirements, ResolvedBinding
// Value fields, binding revisions, status, or diagnostics. A trusted local
// Handler that genuinely needs material may acquire a short-lived lease through
// the Binder's restricted, non-serializable in-process MaterialAccess only
// during Apply. Resolvers never receive raw material.
//
// Handler order is not a fixed list of business phases. Trusted local handlers
// declare stable IDs plus mandatory Requires and optional Before/After edges.
// The Registry produces a stable topological order and rejects cycles. Task and
// Adapter payloads cannot select handler IDs or control this ordering graph.
//
// Production Kubernetes assembly composes separate Network, RLark tools, and
// TensorBoard resolvers and handlers. Unsupported requirement categories fail
// closed before Runtime observation or effects. PassthroughResolver remains an
// explicit bridge for bundles that accept only an empty Environment contract;
// it is not used by the production Kubernetes bundle. Kubernetes SSH mutation
// remains on its legacy compatibility path until a concrete binding is wired.
package environment

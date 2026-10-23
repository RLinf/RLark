// Package workload defines portable Workload compilation, identity-derived
// inventory, environment binding, and semantic observation contracts.
//
// Adapter.Compile returns a Plan containing desired object declarations,
// portable environment requirements, and explicit mutation targets. A
// PlanBinder must work on Plan.DeepCopy output and return an ExecutablePlan that
// contains only Runtime-ready state. Providers consume that final executable
// plan and must not hide Workload- or Environment-specific mutations.
package workload

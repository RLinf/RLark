// Package kubernetes dispatches portable Environment declarations to registered
// Kubernetes Environment resolvers.
package kubernetes

import (
	"context"
	"fmt"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
)

// ResolverRegistration binds one open Environment type string to its resolver.
// Version and parameter validation remain owned by the registered resolver.
type ResolverRegistration struct {
	Type     string
	Resolver environment.Resolver
}

// Resolver is an immutable type-dispatch registry.
type Resolver struct {
	resolvers map[string]environment.Resolver
}

// NewResolver validates and copies registrations. Duplicate Environment types
// are rejected instead of depending on registration order.
func NewResolver(registrations []ResolverRegistration) (Resolver, error) {
	resolvers := make(map[string]environment.Resolver, len(registrations))
	for _, registration := range registrations {
		typeName := strings.TrimSpace(registration.Type)
		if typeName == "" {
			return Resolver{}, fmt.Errorf("environment resolver type is required")
		}
		if registration.Resolver == nil {
			return Resolver{}, fmt.Errorf("environment resolver for type %q is required", typeName)
		}
		if _, found := resolvers[typeName]; found {
			return Resolver{}, fmt.Errorf("environment resolver type %q is registered more than once", typeName)
		}
		resolvers[typeName] = registration.Resolver
	}
	return Resolver{resolvers: resolvers}, nil
}

func (r Resolver) Resolve(ctx context.Context, request environment.ResolveRequest) (environment.ResolvedEnvironment, error) {
	result := environment.ResolvedEnvironment{Task: request.Task}
	grouped := make(map[string][]executionplan.EnvironmentRequirement, len(request.Requirements.Environments))
	orderedTypes := make([]string, 0, len(request.Requirements.Environments))
	for _, requirement := range request.Requirements.Environments {
		if _, found := grouped[requirement.Type]; !found {
			orderedTypes = append(orderedTypes, requirement.Type)
		}
		grouped[requirement.Type] = append(grouped[requirement.Type], requirement)
	}
	for _, requirementType := range orderedTypes {
		resolver, found := r.resolvers[requirementType]
		if !found {
			return environment.ResolvedEnvironment{}, fault.New(fault.FailedPrecondition, "Kubernetes environment resolver does not support type "+requirementType)
		}
		resolved, err := resolver.Resolve(ctx, environment.ResolveRequest{
			Task:         request.Task,
			Runtime:      request.Runtime,
			Requirements: executionplan.Requirements{Environments: grouped[requirementType]},
			Targets:      request.Targets,
		})
		if err != nil {
			return environment.ResolvedEnvironment{}, err
		}
		result.Bindings = append(result.Bindings, resolved.Bindings...)
	}
	return result, nil
}

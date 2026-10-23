package runtime

import "context"

// UnitLoweringRequest supplies one Binder-owned desired object and selected
// Runtime metadata to an effect-free Runtime-specific lowering implementation.
type UnitLoweringRequest struct {
	Runtime Info
	Object  DesiredObject
}

// UnitLowerer adapts Adapter-declared units for one Runtime's trusted version
// and capability contract. Implementations must perform no external I/O and
// must return an object with the same logical identity, ownership, lifecycle,
// dependencies, update policy, and unit declarations.
type UnitLowerer interface {
	Lower(context.Context, UnitLoweringRequest) (DesiredObject, error)
}

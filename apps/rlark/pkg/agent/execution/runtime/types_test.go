package runtime

import "testing"

func TestResourceOwnershipAllowsAdoptionOnlyForTaskIncarnation(t *testing.T) {
	for _, scope := range []IdentityScope{IdentityLogicalTask, IdentityRuntimeInstance, IdentitySharedResource, IdentityExternal} {
		ownership := ResourceOwnership{Scope: scope, OwnerKey: "owner", IncarnationPolicy: IncarnationOwnershipAllowAdoption}
		if err := ownership.Validate(); err == nil {
			t.Fatalf("scope %q accepted incarnation adoption", scope)
		}
	}
	if err := (ResourceOwnership{Scope: IdentityTaskIncarnation, OwnerKey: "owner", IncarnationPolicy: IncarnationOwnershipAllowAdoption}).Validate(); err != nil {
		t.Fatalf("Task-incarnation adoption rejected: %v", err)
	}
}

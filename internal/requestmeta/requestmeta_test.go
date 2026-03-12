package requestmeta

import (
	"context"
	"testing"
)

func TestWithActor_Bead_l3d_17_1(t *testing.T) {
	ctx := context.Background()
	actor := "test-actor"

	ctx = WithActor(ctx, actor)

	if got := Actor(ctx); got != actor {
		t.Errorf("Actor() = %v, want %v", got, actor)
	}
}

func TestWithRole_Bead_l3d_17_1(t *testing.T) {
	ctx := context.Background()
	role := "test-role"

	ctx = WithRole(ctx, role)

	if got := Role(ctx); got != role {
		t.Errorf("Role() = %v, want %v", got, role)
	}
}

func TestActor_EmptyContext_Bead_l3d_17_1(t *testing.T) {
	ctx := context.Background()

	if got := Actor(ctx); got != "" {
		t.Errorf("Actor() = %v, want empty string", got)
	}
}

func TestRole_EmptyContext_Bead_l3d_17_1(t *testing.T) {
	ctx := context.Background()

	if got := Role(ctx); got != "" {
		t.Errorf("Role() = %v, want empty string", got)
	}
}

func TestActorAndRoleTogether_Bead_l3d_17_1(t *testing.T) {
	ctx := context.Background()
	actor := "test-actor"
	role := "test-role"

	ctx = WithActor(ctx, actor)
	ctx = WithRole(ctx, role)

	if got := Actor(ctx); got != actor {
		t.Errorf("Actor() = %v, want %v", got, actor)
	}
	if got := Role(ctx); got != role {
		t.Errorf("Role() = %v, want %v", got, role)
	}
}

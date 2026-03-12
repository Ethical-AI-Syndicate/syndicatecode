package requestmeta

import (
	"context"
	"testing"
)

func TestActorRoundtrip_Bead_l3d_17_1(t *testing.T) {
	ctx := WithActor(context.Background(), "system")
	if got := Actor(ctx); got != "system" {
		t.Errorf("Actor() = %q, want %q", got, "system")
	}
}

func TestRoleRoundtrip_Bead_l3d_17_1(t *testing.T) {
	ctx := WithRole(context.Background(), "operator")
	if got := Role(ctx); got != "operator" {
		t.Errorf("Role() = %q, want %q", got, "operator")
	}
}

func TestActorMissing_Bead_l3d_17_1(t *testing.T) {
	if got := Actor(context.Background()); got != "" {
		t.Errorf("Actor() = %q, want empty", got)
	}
}

func TestRoleMissing_Bead_l3d_17_1(t *testing.T) {
	if got := Role(context.Background()); got != "" {
		t.Errorf("Role() = %q, want empty", got)
	}
}

func TestActorAndRoleCombined_Bead_l3d_17_1(t *testing.T) {
	ctx := WithActor(context.Background(), "admin")
	ctx = WithRole(ctx, "superuser")

	if got := Actor(ctx); got != "admin" {
		t.Errorf("Actor() = %q, want %q", got, "admin")
	}
	if got := Role(ctx); got != "superuser" {
		t.Errorf("Role() = %q, want %q", got, "superuser")
	}
}

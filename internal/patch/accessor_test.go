package patch

import "testing"

func TestEngineRepoRoot_Bead_l3d_17_1(t *testing.T) {
	t.Run("nil engine", func(t *testing.T) {
		result := EngineRepoRoot(nil)
		if result != "" {
			t.Errorf("EngineRepoRoot(nil) = %v, want empty string", result)
		}
	})

	t.Run("non-nil engine", func(t *testing.T) {
		engine := &Engine{repoRoot: "/test/path"}
		result := EngineRepoRoot(engine)
		if result != "/test/path" {
			t.Errorf("EngineRepoRoot() = %v, want '/test/path'", result)
		}
	})
}

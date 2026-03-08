package context

import (
	"reflect"
	"testing"
)

func TestClassifyTaskClassFromTurnMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		want    TaskClass
	}{
		{name: "edit task", message: "fix auth bug in internal/controlplane/server.go", want: TaskClassEdit},
		{name: "testing task", message: "run tests for context package", want: TaskClassTest},
		{name: "debug task", message: "debug panic in session manager", want: TaskClassDebug},
		{name: "analysis task", message: "explain architecture decisions", want: TaskClassAnalysis},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyTaskClass(tc.message)
			if got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

func TestRetrievalProfileForTaskClassIsDeterministic(t *testing.T) {
	t.Parallel()

	first := RetrievalProfileForTaskClass(TaskClassEdit)
	second := RetrievalProfileForTaskClass(TaskClassEdit)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic profile output")
	}
}

func TestRetrievalProfileForUnknownClassUsesConservativeDefaults(t *testing.T) {
	t.Parallel()

	profile := RetrievalProfileForTaskClass(TaskClass("unknown"))
	if profile.RetrievalRadius != RetrievalRadiusTargeted {
		t.Fatalf("expected targeted retrieval radius for unknown class, got %s", profile.RetrievalRadius)
	}
	if len(profile.SourcePriorities) == 0 {
		t.Fatalf("expected source priorities to be defined")
	}
	if profile.TokenBudget["control"] == 0 || profile.TokenBudget["completion_reserve"] == 0 {
		t.Fatalf("expected policy-compliant base budget allocations to be present")
	}
}

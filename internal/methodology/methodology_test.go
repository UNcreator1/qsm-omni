package methodology

import (
	"strings"
	"testing"
)

func TestPromptSelectsBuildContracts(t *testing.T) {
	prompt := Prompt(PhaseBuild)
	if !strings.Contains(prompt, "test-driven-development") {
		t.Fatalf("expected TDD contract in build prompt:\n%s", prompt)
	}
	if !strings.Contains(prompt, "verification-before-completion") {
		t.Fatalf("expected verification contract in build prompt:\n%s", prompt)
	}
	if strings.Contains(prompt, "collapse-readiness") {
		t.Fatalf("did not expect collapse contract in build prompt:\n%s", prompt)
	}
}

func TestAllReturnsCopy(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("expected methodology contracts")
	}
	all[0].Name = "mutated"
	if All()[0].Name == "mutated" {
		t.Fatal("All returned shared backing data")
	}
}

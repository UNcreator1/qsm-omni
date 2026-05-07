package swarm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	qruntime "github.com/nemoclaws/quantum-swarm-v3/internal/runtime"
)

func TestSynthesisAndChopCreateRooms(t *testing.T) {
	root := t.TempDir()
	q, err := lake.Open(filepath.Join(root, ".lake"))
	if err != nil {
		t.Fatal(err)
	}
	obj := Objective{ID: "obj-1", Request: "build a product"}
	agents := []Agent{
		{ID: "alpha", Role: "architecture", Model: "m1", Provider: "p"},
		{ID: "beta", Role: "implementation", Model: "m2", Provider: "p"},
	}
	hyps, err := Synthesizer{Lake: q}.BrainDump(obj, agents)
	if err != nil {
		t.Fatal(err)
	}
	if len(hyps) != 2 {
		t.Fatalf("expected 2 hypotheses, got %d", len(hyps))
	}
	positions, err := Chopper{Lake: q, RoomsDir: filepath.Join(root, ".rooms")}.Chop(obj, hyps, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 3 {
		t.Fatalf("expected 3 positions, got %d", len(positions))
	}
	for _, p := range positions {
		if _, err := os.Stat(filepath.Join(p.Room, "PLAN.md")); err != nil {
			t.Fatalf("missing plan for %s: %v", p.ID, err)
		}
		if _, err := os.Stat(filepath.Join(p.Room, "FORCE_REQUIREMENTS_CHECKLIST.md")); err != nil {
			t.Fatalf("missing force requirements checklist for %s: %v", p.ID, err)
		}
		if _, err := os.Stat(filepath.Join(p.Room, "QSM_FORCE_CHECKLIST.json")); err != nil {
			t.Fatalf("missing force requirements json for %s: %v", p.ID, err)
		}
	}
}

func TestSimulatedHarnessWritesEvidence(t *testing.T) {
	room := t.TempDir()
	result, err := SimulatedHarness{}.Run(Position{ID: "pos-01", Room: room, Strategy: "test heavy", SourceAgent: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.BuildPassed || !result.TestPassed || result.Score <= 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if _, err := os.Stat(result.EvidencePath); err != nil {
		t.Fatalf("missing evidence: %v", err)
	}
}

func TestRealAgentPromptIncludesMethodology(t *testing.T) {
	prompt := realAgentPrompt(
		Position{ID: "pos-01", Room: ".rooms/pos-01"},
		Agent{ID: "alpha", Role: "builder", Provider: "p", Model: "m"},
		Objective{ID: "obj-1", Request: "build a product"},
		qruntime.Config{LakePath: ".lake", WikiPath: "internal/wiki/wiki.md"},
	)
	for _, want := range []string{"MANDATORY FORCE REQUIREMENTS", "QSM_FORCE_CHECKLIST.json", "QSM methodology contracts", "writing-plans", "test-driven-development", "verification-before-completion", "OpenHarness-inspired node kit", "PERMISSIONS.md", "hooks.json"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q:\n%s", want, prompt)
		}
	}
}

func TestSimulatedHarnessBuildsSnakeGame(t *testing.T) {
	room := t.TempDir()
	result, err := SimulatedHarness{}.Run(Position{ID: "pos-01", Name: "Snake", Room: room, Strategy: "build a snake game", SourceAgent: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "style.css", "game.js", "README.md"} {
		if _, err := os.Stat(filepath.Join(result.ProductPath, name)); err != nil {
			t.Fatalf("missing generated %s: %v", name, err)
		}
	}
	if result.Score < 0.89 {
		t.Fatalf("expected snake game branch to receive product score, got %.2f", result.Score)
	}
}

func TestNormalizeProductPathAcceptsFileEvidence(t *testing.T) {
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(product, "README.md")
	if err := os.WriteFile(readme, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := normalizeProductPath(readme); got != product {
		t.Fatalf("expected product directory %s, got %s", product, got)
	}
}

func TestResolveAgentProductPathDoesNotDoubleRoom(t *testing.T) {
	room := filepath.Join(".rooms", "pos-02")
	cases := map[string]string{
		"./product":             filepath.Join(room, "product"),
		".rooms/pos-02/product": filepath.Join(room, "product"),
		"product/README.md":     filepath.Join(room, "product", "README.md"),
	}
	for input, want := range cases {
		if got := resolveAgentProductPath(room, input); got != want {
			t.Fatalf("resolveAgentProductPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestVerifyProductStaticWebFindsMissingAsset(t *testing.T) {
	product := t.TempDir()
	if err := os.WriteFile(filepath.Join(product, "index.html"), []byte(`<!doctype html><html><head><script src="game.js"></script></head><body><canvas></canvas></body></html>`), 0644); err != nil {
		t.Fatal(err)
	}
	verification := VerifyProduct(product)
	if verification.Passed {
		t.Fatalf("expected missing asset failure: %#v", verification)
	}
}

func TestVerifyProductStaticWebPassesWithAssets(t *testing.T) {
	product := t.TempDir()
	if err := os.WriteFile(filepath.Join(product, "index.html"), []byte(`<!doctype html><html><head><script src="game.js"></script></head><body><canvas></canvas></body></html>`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "game.js"), []byte(`const ok = true;`), 0644); err != nil {
		t.Fatal(err)
	}
	verification := VerifyProduct(product)
	if !verification.Passed {
		t.Fatalf("expected verifier pass: %#v", verification)
	}
}

func TestQSMTestsOverrideAgentClaim(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available")
	}
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "index.html"), []byte(`<!doctype html><html><body><script src="app.js"></script></body></html>`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "app.js"), []byte(`const broken = ;`), 0644); err != nil {
		t.Fatal(err)
	}
	result := BranchResult{
		PositionID:   "pos-01",
		Room:         room,
		ProductPath:  product,
		EvidencePath: filepath.Join(room, "evidence.json"),
		BuildPassed:  true,
		TestPassed:   true,
		LintPassed:   true,
		Score:        1,
	}
	if err := verifyProductAndTests(&result); err == nil {
		t.Fatalf("expected QSM tests to fail")
	}
	if result.TestPassed || result.LintPassed {
		t.Fatalf("expected QSM report to override agent claim: %#v", result)
	}
	if result.TestReport == nil || result.TestReport.Passed {
		t.Fatalf("expected failing test report: %#v", result.TestReport)
	}
}

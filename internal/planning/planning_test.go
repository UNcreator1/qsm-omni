package planning

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	qruntime "github.com/nemoclaws/quantum-swarm-v3/internal/runtime"
	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
)

func TestGenerateSimulatedApprovesAndWritesLakeArtifacts(t *testing.T) {
	root := t.TempDir()
	q, err := lake.Open(filepath.Join(root, ".lake"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Generate(root, q, swarm.Objective{ID: "obj-test", Request: "Build a snake game"}, qruntime.Config{HarnessMode: qruntime.HarnessSimulated})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Approved {
		t.Fatalf("expected approved plan, blockers=%v", report.Blockers)
	}
	if len(report.Materials) == 0 {
		t.Fatal("expected material checks")
	}
	if len(report.Artifacts) != 10 {
		t.Fatalf("expected 10 planning artifacts, got %d", len(report.Artifacts))
	}
	artifacts, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, artifact := range artifacts {
		kinds[artifact.Kind] = true
	}
	for _, kind := range []string{"objective_contract", "materials_inventory", "resource_freshness_report", "chop_readiness_verdict"} {
		if !kinds[kind] {
			t.Fatalf("missing lake artifact kind %s", kind)
		}
	}
}

func TestGenerateRealHarnessRequiresFreshHealthyRouteHealth(t *testing.T) {
	root := t.TempDir()
	wikiPath := filepath.Join(root, "wiki.md")
	runnerPath := filepath.Join(root, "runner.py")
	if err := os.WriteFile(wikiPath, []byte("# wiki\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runnerPath, []byte("print('ok')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	rt := qruntime.Config{
		HarnessMode:     qruntime.HarnessLangChain,
		LangChainRunner: runnerPath,
		WikiPath:        wikiPath,
	}
	report, err := Generate(root, nil, swarm.Objective{ID: "obj-real", Request: "Build with real nodes"}, rt)
	if err != nil {
		t.Fatal(err)
	}
	if report.Approved {
		t.Fatal("expected missing route health to block real harness")
	}
	writeRouteHealthFixture(t, root, string(qruntime.HarnessLangChain), time.Now().UTC(), true)
	report, err = Generate(root, nil, swarm.Objective{ID: "obj-real", Request: "Build with real nodes"}, rt)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Approved {
		t.Fatalf("expected fresh healthy route health to approve, blockers=%v", report.Blockers)
	}
}

func TestGenerateRealHarnessRejectsStaleRouteHealth(t *testing.T) {
	root := t.TempDir()
	wikiPath := filepath.Join(root, "wiki.md")
	runnerPath := filepath.Join(root, "runner.py")
	if err := os.WriteFile(wikiPath, []byte("# wiki\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runnerPath, []byte("print('ok')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeRouteHealthFixture(t, root, string(qruntime.HarnessLangChain), time.Now().Add(-31*time.Minute).UTC(), true)
	report, err := Generate(root, nil, swarm.Objective{ID: "obj-real", Request: "Build with real nodes"}, qruntime.Config{
		HarnessMode:     qruntime.HarnessLangChain,
		LangChainRunner: runnerPath,
		WikiPath:        wikiPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Approved {
		t.Fatal("expected stale route health to block real harness")
	}
}

func writeRouteHealthFixture(t *testing.T, root, mode string, checkedAt time.Time, ok bool) {
	t.Helper()
	stateDir := filepath.Join(root, ".state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{
		"harness_mode": mode,
		"checked_at":   checkedAt,
		"results": []map[string]any{
			{"ok": ok},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "route_health.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

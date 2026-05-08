package failure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
	"github.com/nemoclaws/quantum-swarm-v3/internal/tester"
)

func TestAnalyzeClassifiesCommandFailureAndWritesLesson(t *testing.T) {
	root := t.TempDir()
	room := filepath.Join(root, ".rooms", "pos-01")
	logDir := filepath.Join(room, ".qsm_test", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	stdout := filepath.Join(logDir, "cmd-01.out")
	stderr := filepath.Join(logDir, "cmd-01.err")
	trace := filepath.Join(room, ".qsm_test", "trace.jsonl")
	for _, path := range []string{stdout, stderr, trace} {
		if err := os.WriteFile(path, []byte("evidence\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	testReport := &tester.TestReport{
		Schema:    tester.SchemaReport,
		Passed:    false,
		Path:      filepath.Join(room, ".qsm_test", "qsm_test_report.json"),
		TracePath: trace,
		Summary:   tester.TestSummary{Commands: 1, FailedCommands: 1},
		Commands: []tester.CommandResult{{
			Name:       "go tests",
			Kind:       "test",
			Cmd:        []string{"go", "test", "./..."},
			CWD:        filepath.Join(room, "product"),
			ExitCode:   1,
			StderrPath: stderr,
			StdoutPath: stdout,
		}},
		Errors: []string{"go tests exited with code 1"},
	}
	mustWriteJSON(t, testReport.Path, testReport)
	runReport := swarm.RunReport{
		ObjectiveID:    "obj-fail",
		RequestedNodes: 1,
		StartedNodes:   1,
		FailedNodes:    1,
		Results: []swarm.BranchResult{{
			PositionID:  "pos-01",
			AgentID:     "alpha",
			AgentModel:  "test-model",
			Room:        room,
			BuildPassed: true,
			TestPassed:  false,
			LintPassed:  true,
			TestReport:  testReport,
		}},
	}
	q, err := lake.Open(filepath.Join(root, ".lake"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(Options{
		Root:        root,
		Lake:        q,
		WriteLake:   true,
		WriteRooms:  true,
		Now:         time.Unix(100, 0).UTC(),
		RunReport:   runReport,
		ReportGiven: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed {
		t.Fatalf("expected failure analysis to pass as evidence, got errors %#v", report.Errors)
	}
	if report.AnalyzedFailures != 1 || report.LakeWrites != 1 || report.RegrowthCandidates != 1 {
		t.Fatalf("unexpected summary: %#v", report)
	}
	got := report.Failures[0]
	if got.Class != ClassCompileTestRuntime {
		t.Fatalf("expected compile/test/runtime classification, got %s", got.Class)
	}
	if got.LessonCacheID == "" {
		t.Fatalf("expected lesson cache id")
	}
	if len(got.Commands) != 1 || !strings.Contains(got.Lesson, "go test ./...") {
		t.Fatalf("expected failing command in lesson, got %#v", got)
	}
	if _, err := os.Stat(filepath.Join(room, "failure-evidence", "failure.json")); err != nil {
		t.Fatalf("expected room failure evidence: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".lake", "failures", got.ID+".json")); err != nil {
		t.Fatalf("expected lake failure record: %v", err)
	}
	items, err := q.ListCache(lake.CacheFilter{ObjectiveID: "obj-fail", Kinds: []string{"failure_lesson"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != got.LessonCacheID {
		t.Fatalf("expected one failure lesson cache item, got %#v", items)
	}
}

func TestAnalyzeRecommendsResearchOnlyForKnowledgeFailures(t *testing.T) {
	root := t.TempDir()
	room := filepath.Join(root, ".rooms", "pos-02")
	if err := os.MkdirAll(room, 0755); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(Options{
		Root: root,
		Now:  time.Unix(200, 0).UTC(),
		RunReport: swarm.RunReport{
			ObjectiveID:    "obj-knowledge",
			RequestedNodes: 1,
			StartedNodes:   1,
			FailedNodes:    1,
			Results: []swarm.BranchResult{{
				PositionID: "pos-02",
				Room:       room,
				Error:      "missing knowledge: unknown requirement for legal citation format",
			}},
		},
		ReportGiven: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("expected one failure, got %#v", report)
	}
	got := report.Failures[0]
	if got.Class != ClassMissingKnowledgeMaterials || !got.ResearchRecommended {
		t.Fatalf("expected missing knowledge with research recommendation, got %#v", got)
	}
}

func TestAnalyzeCleanRunPassesWithNoFailures(t *testing.T) {
	report, err := Analyze(Options{
		Root: t.TempDir(),
		Now:  time.Unix(300, 0).UTC(),
		RunReport: swarm.RunReport{
			ObjectiveID:    "obj-clean",
			RequestedNodes: 1,
			StartedNodes:   1,
			SucceededNodes: 1,
			Results: []swarm.BranchResult{{
				PositionID:  "pos-01",
				BuildPassed: true,
				TestPassed:  true,
				LintPassed:  true,
			}},
		},
		ReportGiven: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.AnalyzedFailures != 0 || len(report.Failures) != 0 {
		t.Fatalf("expected clean pass, got %#v", report)
	}
}

func mustWriteJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

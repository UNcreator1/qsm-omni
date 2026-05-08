package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/collapse"
	"github.com/nemoclaws/quantum-swarm-v3/internal/costing"
	"github.com/nemoclaws/quantum-swarm-v3/internal/lakebrain"
	"github.com/nemoclaws/quantum-swarm-v3/internal/planning"
	"github.com/nemoclaws/quantum-swarm-v3/internal/requirements"
	qruntime "github.com/nemoclaws/quantum-swarm-v3/internal/runtime"
	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
	"github.com/nemoclaws/quantum-swarm-v3/internal/tester"
)

func TestLoadSecretKeyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Deepseek_keys")
	if err := os.WriteFile(path, []byte("# local only\nexport DEEPSEEK_API_KEY='local-test-secret'\n"), 0600); err != nil {
		t.Fatal(err)
	}

	key, err := loadSecretKeyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if key != "local-test-secret" {
		t.Fatalf("unexpected key %q", key)
	}
}

func TestLoadSecretKeyFilePrefersDeepSeekKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("NVIDIA_API_KEY='wrong-provider-test-secret'\nDEEPSEEK_API_KEY='deepseek-right-secret'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	key, err := loadSecretKeyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if key != "deepseek-right-secret" {
		t.Fatalf("expected DeepSeek-specific key, got %q", key)
	}
}

func TestAgentsWithRoutePreservesRoles(t *testing.T) {
	agents := defaultAgents()
	routed := agentsWithRoute(agents, "deepseek-chat")
	if len(routed) != len(agents) {
		t.Fatalf("unexpected routed agent count %d", len(routed))
	}
	for i, agent := range routed {
		if agent.ID != agents[i].ID || agent.Role != agents[i].Role {
			t.Fatalf("agent identity changed: %#v -> %#v", agents[i], agent)
		}
		if agent.Provider != "" || agent.Model != "deepseek-chat" {
			t.Fatalf("unexpected route on agent %#v", agent)
		}
	}
}

func TestRouteHealthAutoDiscoversLiveFreeRouterModels(t *testing.T) {
	t.Setenv("QSM_LANGCHAIN_MODEL", "")
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"oc/nemotron-3-super-free","owned_by":"oc"},
			{"id":"openrouter/qwen/qwen3-coder:free","owned_by":"openrouter"},
			{"id":"openrouter/openai/text-embedding-3-small","owned_by":"openrouter"},
			{"id":"wombo","owned_by":"combo"}
		]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	rt := qruntime.Config{
		HarnessMode:   qruntime.HarnessLangChain,
		NineRouterURL: server.URL + "/v1",
		NineRouterKey: "test-key",
	}
	models := routeHealthModelsFromFlag("auto", defaultAgents(), rt, 10)
	joined := strings.Join(models, ",")
	for _, want := range []string{"wombo", "oc/nemotron-3-super-free", "openrouter/qwen/qwen3-coder:free"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("auto route discovery missing %s from %v", want, models)
		}
	}
	if strings.Contains(joined, "text-embedding") {
		t.Fatalf("auto route discovery should exclude embedding routes: %v", models)
	}
}

func TestHealthyAgentsPreferStableBuildHealthRoutes(t *testing.T) {
	results := []qruntime.RouteHealthResult{
		{Model: "wombo", OK: true, ContentOK: true, LatencyMS: 1000},
		{Model: "oc/nemotron-3-super-free", OK: true, ContentOK: true, LatencyMS: 5000},
	}
	health := map[string]BuildHealthModel{
		"wombo": {
			Model:       "wombo",
			Attempts:    4,
			Succeeded:   2,
			Failed:      2,
			SuccessRate: 0.5,
			LastState:   "failed",
			LastError:   "step budget reached",
		},
		"oc/nemotron-3-super-free": {
			Model:       "oc/nemotron-3-super-free",
			Attempts:    2,
			Succeeded:   2,
			SuccessRate: 1,
			LastState:   "succeeded",
		},
	}
	agents := healthyAgents(defaultAgents(), qruntime.Config{HarnessMode: qruntime.HarnessLangChain}, results, health)
	if len(agents) == 0 {
		t.Fatal("expected healthy agents")
	}
	if got := agentRoute(agents[0]); got != "oc/nemotron-3-super-free" {
		t.Fatalf("expected stable route first, got %s from %#v", got, agents)
	}
}

func TestUpdateBuildHealthStateFromRunReport(t *testing.T) {
	root := t.TempDir()
	room := filepath.Join(root, ".rooms", "pos-01")
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "README.md"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(room, "evidence.json")
	if err := os.WriteFile(evidence, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := swarm.WriteRoomStatus(room, swarm.RoomStatus{
		SchemaVersion: 1,
		ObjectiveID:   "obj-1",
		PositionID:    "pos-01",
		AgentID:       "alpha",
		AgentModel:    "oc/test-model",
		Harness:       "langchain",
		State:         swarm.RoomStateSucceeded,
		Phase:         "complete",
		ProductReady:  true,
		EvidenceReady: true,
		BuildPassed:   true,
		TestPassed:    true,
		LintPassed:    true,
		UpdatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	report := swarm.RunReport{
		ObjectiveID:    "obj-1",
		HarnessMode:    "langchain",
		RequestedNodes: 1,
		Results: []swarm.BranchResult{{
			PositionID:   "pos-01",
			AgentModel:   "oc/test-model",
			Room:         room,
			BuildPassed:  true,
			TestPassed:   true,
			LintPassed:   true,
			Score:        0.9,
			EvidencePath: evidence,
			ProductPath:  product,
		}},
	}
	state, err := updateBuildHealthState(root, report)
	if err != nil {
		t.Fatal(err)
	}
	item := state.Models["oc/test-model"]
	if item.Attempts != 1 || item.Succeeded != 1 || item.Failed != 0 || item.SuccessRate != 1 {
		t.Fatalf("unexpected build health item: %#v", item)
	}
	state, err = updateBuildHealthState(root, report)
	if err != nil {
		t.Fatal(err)
	}
	if item := state.Models["oc/test-model"]; item.Attempts != 1 {
		t.Fatalf("expected duplicate objective/position to be ignored, got %#v", item)
	}
}

func TestBuildHealthBlocksRouteAfterRepeatedFailures(t *testing.T) {
	health := map[string]BuildHealthModel{
		"bad":  {Model: "bad", Attempts: 2, Succeeded: 0, Failed: 2},
		"new":  {Model: "new", Attempts: 1, Succeeded: 0, Failed: 1},
		"good": {Model: "good", Attempts: 3, Succeeded: 1, Failed: 2},
	}
	if !buildHealthBlocksRoute("bad", health) {
		t.Fatal("expected repeated failure model to be blocked")
	}
	if buildHealthBlocksRoute("new", health) {
		t.Fatal("expected single failure model to remain available")
	}
	if buildHealthBlocksRoute("good", health) {
		t.Fatal("expected model with success history to remain available")
	}
}

func TestBuildAutorunRunArgsCarriesQualityFlags(t *testing.T) {
	args := buildAutorunRunArgs("/tmp/qsm", "Build product", "langchain", "7", "2", 3, 15*time.Second, true, true, true, nodeRuntimeTuning{})
	joined := strings.Join(args, "\x00")
	for _, want := range []string{
		"run",
		"-root\x00/tmp/qsm",
		"-request\x00Build product",
		"-harness\x00langchain",
		"-positions\x007",
		"-parallel\x002",
		"-retries\x003",
		"-retry-backoff\x0015s",
		"-shared-cache=true",
		"-route-health=true",
		"-deepseek-fallback=true",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing autorun arg %q in %#v", want, args)
		}
	}
}

func TestBuildAutorunOneCycleArgsUsesAutorunStateLoop(t *testing.T) {
	args := buildAutorunOneCycleArgs("/tmp/qsm", "Build product", "simulated", "1", "1", true, true, false, nodeRuntimeTuning{})
	joined := strings.Join(args, "\x00")
	for _, want := range []string{"autorun", "-max-cycles\x001", "-interval\x001s", "-shared-cache=true", "-route-health=true"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing autorun one-cycle arg %q in %#v", want, args)
		}
	}
	if strings.Contains(joined, "\x00run\x00") {
		t.Fatalf("plist one-cycle args should call autorun, not direct run: %#v", args)
	}
}

func TestResolveNodeRuntimeTuningLongRunDefaults(t *testing.T) {
	tuning := resolveNodeRuntimeTuning(true, 0, 0, 0, 0)
	if tuning.NodeMaxSteps < 80 {
		t.Fatalf("expected long-run node max steps, got %#v", tuning)
	}
	if tuning.DeepAgentsRecursionLimit <= tuning.NodeMaxSteps {
		t.Fatalf("expected recursion limit above node budget, got %#v", tuning)
	}
	if tuning.NodeShellTimeoutSeconds < 120 || tuning.ModelMaxRetries < 3 {
		t.Fatalf("expected production-ish shell/model retry defaults, got %#v", tuning)
	}
}

func TestBuildAutorunRunArgsCarriesLongRunTuning(t *testing.T) {
	tuning := resolveNodeRuntimeTuning(true, 101, 102, 203, 5)
	args := buildAutorunRunArgs("/tmp/qsm", "Build product", "langchain", "7", "2", 3, 15*time.Second, true, true, true, tuning)
	joined := strings.Join(args, "\x00")
	for _, want := range []string{
		"-long-run=true",
		"-node-max-steps\x00101",
		"-deepagents-recursion-limit\x00102",
		"-node-shell-timeout\x00203",
		"-model-max-retries\x005",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing long-run arg %q in %#v", want, args)
		}
	}
}

func TestForceLocalCollapseGateBlocksSecurityFail(t *testing.T) {
	score := requirements.Score(requirements.Evidence{
		ObjectiveID:      "obj-1",
		HarnessMode:      "simulated",
		RunPresent:       true,
		SecurityHigh:     1,
		TestCommands:     1,
		PlanApproved:     true,
		CollapseApproved: true,
	})
	if forceLocalCollapseGate(score) {
		t.Fatal("expected force gate to block high security finding")
	}
}

func TestForceLocalCollapseGateAllowsPartialLocalEvidence(t *testing.T) {
	score := requirements.Score(requirements.Evidence{
		ObjectiveID:          "obj-1",
		HarnessMode:          "simulated",
		PlanApproved:         true,
		RunPresent:           true,
		RequestedNodes:       1,
		StartedNodes:         1,
		SucceededNodes:       1,
		Concurrency:          1,
		MaxRetries:           1,
		AllNodesAccounted:    true,
		CollapseApproved:     true,
		TestCommands:         2,
		PassedTestCommands:   2,
		LocalPackageAuditOK:  true,
		DataLakeAtomicWrites: true,
	})
	if !forceLocalCollapseGate(score) {
		t.Fatal("expected local partial evidence to pass mandatory force gate")
	}
}

func TestLaunchdPlistEscapesAndIncludesAutorunArgs(t *testing.T) {
	plist := launchdPlist(`local.qsm.&test`, `/tmp/qsm`, `/tmp/qsm/qsm`, []string{"autorun", "-request", `A&B "build"`}, 1200)
	for _, want := range []string{
		"local.qsm.&amp;test",
		"<string>/tmp/qsm/qsm</string>",
		"<string>autorun</string>",
		"A&amp;B &quot;build&quot;",
		"<integer>1200</integer>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("missing %q in plist:\n%s", want, plist)
		}
	}
}

func TestRunQAAlphaPassesWithQSMTestEvidence(t *testing.T) {
	root := t.TempDir()
	mustWriteQAState(t, root)
	report, err := runQA(root, "alpha", false, "room", "")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed {
		t.Fatalf("expected alpha QA to pass with warnings allowed: %#v", report)
	}
	if !hasQAGate(report, "qsm-test-report", "PASS") {
		t.Fatalf("expected qsm-test-report pass: %#v", report.Gates)
	}
}

func TestRunQAProductionFailsWithoutProductionEvidence(t *testing.T) {
	root := t.TempDir()
	mustWriteQAState(t, root)
	report, err := runQA(root, "production", false, "docker", "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatalf("expected production QA to fail missing hard gates: %#v", report)
	}
	for _, want := range []string{"hard-sandbox", "coverage", "mutation", "flake", "trace-replay"} {
		if !hasQAGate(report, want, "FAIL") {
			t.Fatalf("expected %s fail gate: %#v", want, report.Gates)
		}
	}
}

func TestRunQAProductionFailsWhenDockerDaemonMissing(t *testing.T) {
	root := t.TempDir()
	mustWriteQAState(t, root)
	fakeBin := t.TempDir()
	fakeDocker := filepath.Join(fakeBin, "docker")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "Docker version fake"
  exit 0
fi
if [ "$1" = "info" ]; then
  echo "daemon unavailable" >&2
  exit 1
fi
exit 1
`
	if err := os.WriteFile(fakeDocker, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)
	report, err := runQA(root, "production", true, "docker", "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatalf("expected production QA to fail without Docker daemon: %#v", report)
	}
	if !hasQAGate(report, "hard-sandbox", "FAIL") {
		t.Fatalf("expected hard-sandbox fail: %#v", report.Gates)
	}
}

func TestTraceReportDetectsMatchedRoomCommands(t *testing.T) {
	root := t.TempDir()
	room := filepath.Join(root, ".rooms", "pos-01")
	traceDir := filepath.Join(room, ".qsm_test")
	if err := os.MkdirAll(traceDir, 0755); err != nil {
		t.Fatal(err)
	}
	trace := `{"type":"command_start","name":"test","cwd":"product","cmd":["true"],"sandbox":"room"}` + "\n" +
		`{"type":"command_end","name":"test","exit_code":0,"duration_ms":1,"sandbox":"room"}` + "\n"
	if err := os.WriteFile(filepath.Join(traceDir, "trace.jsonl"), []byte(trace), 0644); err != nil {
		t.Fatal(err)
	}
	report := swarm.RunReport{
		ObjectiveID:       "obj-trace",
		RequestedNodes:    1,
		StartedNodes:      1,
		SucceededNodes:    1,
		AllNodesAccounted: true,
		Results: []swarm.BranchResult{{
			PositionID:  "pos-01",
			Room:        room,
			BuildPassed: true,
			TestPassed:  true,
			LintPassed:  true,
			TestReport:  &tester.TestReport{TracePath: filepath.Join(traceDir, "trace.jsonl")},
		}},
	}
	if err := writeJSON(filepath.Join(root, ".state", "run_report.json"), report); err != nil {
		t.Fatal(err)
	}
	out, err := buildTraceReport(root)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Passed || out.Starts != 1 || out.Ends != 1 || out.Unmatched != 0 {
		t.Fatalf("expected passing trace report: %#v", out)
	}
}

func TestCostBudgetFailsWhenTokensExceedBudget(t *testing.T) {
	root := t.TempDir()
	t.Setenv("QSM_BUDGET_MAX_TOKENS", "10")
	if err := writeJSON(filepath.Join(root, ".state", "run_report.json"), swarm.RunReport{ObjectiveID: "obj-budget", DurationMS: 100}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, ".state", "cost_report.json"), costing.Report{Schema: costing.Schema, ObjectiveID: "obj-budget", TotalTokens: 11}); err != nil {
		t.Fatal(err)
	}
	report := buildCostBudgetReport(root)
	if report.Passed {
		t.Fatalf("expected budget failure: %#v", report)
	}
}

func TestQAReportFileGateRequiresPassedTrue(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".state", "trace_report.json")
	if err := writeJSON(path, TraceReport{Schema: "qsm.trace_report.v1", Passed: false, Errors: []string{"missing end"}}); err != nil {
		t.Fatal(err)
	}
	passed, evidence := qaReportFilePassed(path)
	if passed || !strings.Contains(evidence, "passed=false") {
		t.Fatalf("expected failed evidence gate, got passed=%v evidence=%s", passed, evidence)
	}
}

func TestForceEvidenceReadsProductionEvidenceReports(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, ".state")
	if err := os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".github", "workflows", "qsm-production-qa.yml"), []byte("name: qsm\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runReport := swarm.RunReport{
		ObjectiveID:       "obj-evidence",
		HarnessMode:       "simulated",
		RequestedNodes:    2,
		StartedNodes:      2,
		SucceededNodes:    2,
		Concurrency:       2,
		MaxRetries:        1,
		AllNodesAccounted: true,
		Results: []swarm.BranchResult{{
			PositionID:  "pos-01",
			BuildPassed: true,
			TestPassed:  true,
			LintPassed:  true,
			TestReport: &tester.TestReport{Summary: tester.TestSummary{
				Commands:       1,
				PassedCommands: 1,
			}},
		}},
	}
	mustWrite := func(name string, value any) {
		t.Helper()
		if err := writeJSON(filepath.Join(state, name), value); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("cost_report.json", costing.Report{Schema: costing.Schema, ObjectiveID: "obj-evidence", TotalTokens: 42})
	mustWrite("benchmark_report.json", BenchmarkReport{Schema: "qsm.benchmark_report.v1", Suite: "terminal-contract", Style: "SWE-bench/Terminal-Bench inspired local executable tasks", Tasks: []BenchmarkTask{{Name: "task", Passed: true}}, PassedTasks: 1})
	mustWrite("trace_report.json", TraceReport{Schema: "qsm.trace_report.v1", Passed: true})
	mustWrite("coverage_report.json", QualityReport{Schema: "qsm.coverage_report.v1", Passed: true})
	mustWrite("mutation_report.json", QualityReport{Schema: "qsm.mutation_report.v1", Passed: true})
	mustWrite("flake_report.json", QualityReport{Schema: "qsm.flake_report.v1", Passed: true})
	mustWrite("cost_budget_report.json", CostBudgetReport{Schema: "qsm.cost_budget_report.v1", Passed: true})
	mustWrite("ci_release_report.json", CIReleaseReport{Schema: "qsm.ci_release_report.v1", Passed: true, CI: true})
	mustWrite("lake_interaction_score.json", lakebrain.Report{Schema: lakebrain.SchemaInteractionReport, EnterpriseReady: true, CacheCitationCoverage: 1, WikiCitationCoverage: 0.5, DecisionCitationCoverage: 1})

	evidence := forceEvidence(root, planning.Report{Schema: "qsm.plan_report.v1", Approved: true}, runReport, collapse.Verdict{Approved: true})
	if !evidence.TracePassed || !evidence.CoveragePassed || !evidence.MutationPassed || !evidence.FlakePassed || !evidence.CostBudgetPassed {
		t.Fatalf("expected quality evidence to be populated: %#v", evidence)
	}
	if !evidence.CIReleasePassed || !evidence.CIReleaseCI || !evidence.CIWorkflowPresent {
		t.Fatalf("expected CI evidence to be populated: %#v", evidence)
	}
	if !evidence.OfficialBenchmark || !evidence.LakeEnterpriseReady || evidence.LakeCacheCitation != 1 {
		t.Fatalf("expected benchmark/lake evidence to be populated: %#v", evidence)
	}
}

func TestProductionGapReportsFailedProductionGates(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, ".state")
	qa := QAReport{
		Schema:  "qsm.qa_report.v1",
		Profile: "production",
		Passed:  false,
		Gates: []QAGate{
			{ID: "hard-sandbox", Name: "Hard sandbox ready", Status: "FAIL", Required: true, Evidence: "daemon=false", Recommendation: "Start Docker."},
			{ID: "trace-replay", Name: "Trace", Status: "PASS", Required: true, Evidence: "passed=true"},
		},
	}
	if err := writeJSON(filepath.Join(state, "qa_report.json"), qa); err != nil {
		t.Fatal(err)
	}
	score := requirements.Score(requirements.Evidence{ObjectiveID: "obj-gap", RunPresent: true})
	if err := writeJSON(filepath.Join(state, "force_score.json"), score); err != nil {
		t.Fatal(err)
	}
	report := buildProductionGapReport(root)
	if report.ProductionReady || len(report.FailedGates) != 1 || report.FailedGates[0].ID != "hard-sandbox" {
		t.Fatalf("unexpected gap report: %#v", report)
	}
	if !strings.Contains(strings.Join(report.NextActions, "\n"), "Start Docker.") {
		t.Fatalf("expected next action from failed gate: %#v", report.NextActions)
	}
}

func mustWriteQAState(t *testing.T, root string) {
	t.Helper()
	state := filepath.Join(root, ".state")
	if err := os.MkdirAll(state, 0755); err != nil {
		t.Fatal(err)
	}
	testReport := &tester.TestReport{
		Schema:      tester.SchemaReport,
		Passed:      true,
		ProductType: "node",
		Summary: tester.TestSummary{
			Commands:       1,
			PassedCommands: 1,
			Tests:          2,
			PassedTests:    2,
		},
		Security: tester.SecurityReport{Schema: "qsm.security_report.v1", Passed: true},
	}
	runReport := swarm.RunReport{
		ObjectiveID:       "obj-qa",
		HarnessMode:       "simulated",
		RequestedNodes:    1,
		StartedNodes:      1,
		SucceededNodes:    1,
		Concurrency:       1,
		MaxRetries:        1,
		AllNodesAccounted: true,
		Results: []swarm.BranchResult{{
			PositionID:  "pos-01",
			BuildPassed: true,
			TestPassed:  true,
			LintPassed:  true,
			TestReport:  testReport,
		}},
	}
	if err := writeJSON(filepath.Join(state, "run_report.json"), runReport); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(state, "verdict.json"), collapse.Verdict{Approved: true, Winner: swarm.BranchResult{PositionID: "pos-01"}}); err != nil {
		t.Fatal(err)
	}
	force := requirements.Score(requirements.Evidence{
		ObjectiveID:          "obj-qa",
		HarnessMode:          "simulated",
		PlanApproved:         true,
		RunPresent:           true,
		RequestedNodes:       1,
		StartedNodes:         1,
		SucceededNodes:       1,
		Concurrency:          1,
		MaxRetries:           1,
		AllNodesAccounted:    true,
		CollapseApproved:     true,
		TestCommands:         1,
		PassedTestCommands:   1,
		LocalPackageAuditOK:  true,
		CostReportPresent:    true,
		BenchmarkPresent:     true,
		BenchmarkPassedTasks: 1,
		BenchmarkTotalTasks:  1,
		DataLakeAtomicWrites: true,
	})
	if err := writeJSON(filepath.Join(state, "force_score.json"), force); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(state, "cost_report.json"), costing.Report{Schema: costing.Schema, ObjectiveID: "obj-qa", TotalTokens: 100}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(state, "lake_interaction_score.json"), lakebrain.Report{Schema: lakebrain.SchemaInteractionReport, ObjectiveID: "obj-qa", AverageNodeScore: 80, RefreshCoverage: 1, CacheWriteCoverage: 1, CacheCitationCoverage: 1}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(state, "benchmark_report.json"), BenchmarkReport{Schema: "qsm.benchmark_report.v1", Suite: "local-smoke", Style: "SWE-bench/Terminal-Bench inspired local executable tasks", Tasks: []BenchmarkTask{{Name: "smoke", Passed: true}}, PassedTasks: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestApplySimpleMutationSkipsQAFilesAndMutatesImplementation(t *testing.T) {
	product := t.TempDir()
	if err := os.WriteFile(filepath.Join(product, "qa_frontend_smoke.cjs"), []byte("const smoke = true;\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "server.js"), []byte("function ok(value) { return value === true; }\nmodule.exports = { ok };\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mutated, mutation, err := applySimpleMutation(product)
	if err != nil {
		t.Fatal(err)
	}
	if !mutated {
		t.Fatal("expected implementation mutation")
	}
	if !strings.Contains(mutation, "server.js") {
		t.Fatalf("expected server.js mutation, got %q", mutation)
	}
	smoke, err := os.ReadFile(filepath.Join(product, "qa_frontend_smoke.cjs"))
	if err != nil {
		t.Fatal(err)
	}
	if string(smoke) != "const smoke = true;\n" {
		t.Fatalf("QA smoke file was mutated: %q", smoke)
	}
}

func hasQAGate(report QAReport, id, status string) bool {
	for _, gate := range report.Gates {
		if gate.ID == id && gate.Status == status {
			return true
		}
	}
	return false
}

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/capacity"
	"github.com/nemoclaws/quantum-swarm-v3/internal/collapse"
	"github.com/nemoclaws/quantum-swarm-v3/internal/costing"
	"github.com/nemoclaws/quantum-swarm-v3/internal/council"
	"github.com/nemoclaws/quantum-swarm-v3/internal/delivery"
	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	"github.com/nemoclaws/quantum-swarm-v3/internal/lakebrain"
	"github.com/nemoclaws/quantum-swarm-v3/internal/planning"
	"github.com/nemoclaws/quantum-swarm-v3/internal/productmanifest"
	"github.com/nemoclaws/quantum-swarm-v3/internal/requirements"
	"github.com/nemoclaws/quantum-swarm-v3/internal/research"
	qruntime "github.com/nemoclaws/quantum-swarm-v3/internal/runtime"
	"github.com/nemoclaws/quantum-swarm-v3/internal/sandbox"
	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
	"github.com/nemoclaws/quantum-swarm-v3/internal/tester"
	"github.com/nemoclaws/quantum-swarm-v3/internal/wiki"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "capacity":
		showCapacity(os.Args[2:])
	case "doctor":
		doctor(os.Args[2:])
	case "route-health":
		routeHealth(os.Args[2:])
	case "deploy":
		deploy(os.Args[2:])
	case "autorun":
		autorun(os.Args[2:])
	case "autorun-plist":
		autorunPlist(os.Args[2:])
	case "plan":
		plan(os.Args[2:])
	case "council":
		councilCmd(os.Args[2:])
	case "stop":
		stop(os.Args[2:])
	case "run":
		run(os.Args[2:])
	case "status":
		status(os.Args[2:])
	case "lake-score":
		lakeScore(os.Args[2:])
	case "lake-maintain":
		lakeMaintain(os.Args[2:])
	case "lake-promote":
		lakePromote(os.Args[2:])
	case "force-score":
		forceScoreCmd(os.Args[2:])
	case "cost":
		costCmd(os.Args[2:])
	case "sandbox":
		sandboxCmd(os.Args[2:])
	case "trace":
		traceCmd(os.Args[2:])
	case "benchmark":
		benchmarkCmd(os.Args[2:])
	case "self-improve":
		selfImproveCmd(os.Args[2:])
	case "cost-budget":
		costBudgetCmd(os.Args[2:])
	case "coverage":
		coverageCmd(os.Args[2:])
	case "flake":
		flakeCmd(os.Args[2:])
	case "mutation":
		mutationCmd(os.Args[2:])
	case "ci-release":
		ciReleaseCmd(os.Args[2:])
	case "ci-bootstrap":
		ciBootstrapCmd(os.Args[2:])
	case "ops-readiness":
		opsReadinessCmd(os.Args[2:])
	case "compliance":
		complianceCmd(os.Args[2:])
	case "stress":
		stressCmd(os.Args[2:])
	case "recovery":
		recoveryCmd(os.Args[2:])
	case "contributor-smoke":
		contributorSmokeCmd(os.Args[2:])
	case "qa":
		qaCmd(os.Args[2:])
	case "production-gap":
		productionGapCmd(os.Args[2:])
	case "synthesize":
		synthesize(os.Args[2:])
	case "hydrate":
		hydrate(os.Args[2:])
	case "wiki":
		compileWiki(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("qsm <capacity|doctor|route-health|deploy|autorun|autorun-plist|plan|council|stop|run|status|lake-score|lake-maintain|lake-promote|force-score|cost|sandbox|trace|benchmark|self-improve|cost-budget|coverage|flake|mutation|ci-release|ci-bootstrap|ops-readiness|compliance|stress|recovery|contributor-smoke|qa|production-gap|synthesize|hydrate|wiki> [flags]")
}

func run(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	request := fs.String("request", "Build a tested auditable product", "high-level request")
	root := fs.String("root", ".", "workspace root")
	positionsFlag := fs.String("positions", "auto", "number of divergent positions, or auto")
	parallelFlag := fs.String("parallel", "auto", "max nodes to execute at once, or auto")
	harnessMode := fs.String("harness", "simulated", "harness mode: simulated, opencode, or langchain")
	sandboxBackend := fs.String("sandbox", getenvDefault("QSM_TEST_SANDBOX", sandbox.BackendRoom), "sandbox backend for QSM-owned node verification commands: room, docker, or auto")
	harnessTimeout := fs.Duration("timeout", 20*time.Minute, "real harness timeout")
	retries := fs.Int("retries", 1, "retry count for retryable harness failures")
	retryBackoff := fs.Duration("retry-backoff", 10*time.Second, "base backoff for retryable harness failures")
	sharedCache := fs.Bool("shared-cache", envBool("QSM_SHARED_CACHE"), "enable verified cross-node cache under .lake/cache")
	routeHealthGate := fs.Bool("route-health", envBool("QSM_ROUTE_HEALTH"), "probe model routes and filter unhealthy agents before real harness execution")
	routeHealthModelsFlag := fs.String("route-health-models", getenvDefault("QSM_ROUTE_HEALTH_MODELS", "auto"), "route-health model pool: auto, free, all, or comma-separated models")
	routeHealthLimit := fs.Int("route-health-limit", envInt("QSM_ROUTE_HEALTH_LIMIT", 16), "max discovered route-health models to probe")
	skipPlan := fs.Bool("skip-plan", envBool("QSM_SKIP_PLAN"), "skip Phase A planning/materials gate")
	deepSeekFallback := fs.Bool("deepseek-fallback", envBool("QSM_DEEPSEEK_FALLBACK"), "use direct DeepSeek only if route-health finds no healthy 9Router routes")
	deepSeekKeyFile := fs.String("deepseek-key-file", getenvDefault("QSM_DEEPSEEK_KEY_FILE", defaultDeepSeekKeyFile()), "DeepSeek key file for direct fallback")
	deepSeekModel := fs.String("deepseek-model", getenvDefault("QSM_DEEPSEEK_MODEL", "deepseek-chat"), "DeepSeek model for direct fallback")
	longRun := fs.Bool("long-run", envBool("QSM_LONG_RUN"), "enable production long-run node budget profile for real LangChain/OpenCode cycles")
	nodeMaxSteps := fs.Int("node-max-steps", envInt("QSM_NODE_MAX_STEPS", 0), "LangChain node graph step budget; 0 keeps runner default unless -long-run is set")
	deepAgentsRecursionLimit := fs.Int("deepagents-recursion-limit", envInt("QSM_DEEPAGENTS_RECURSION_LIMIT", 0), "DeepAgents recursion limit; 0 keeps runner default unless -long-run is set")
	nodeShellTimeout := fs.Int("node-shell-timeout", envInt("QSM_NODE_SHELL_TIMEOUT", 0), "per-tool shell timeout in seconds; 0 keeps runner default unless -long-run is set")
	modelMaxRetries := fs.Int("model-max-retries", envInt("QSM_MODEL_MAX_RETRIES", 0), "model client retry count; 0 keeps runner default unless -long-run is set")
	useCouncil := fs.Bool("council", false, "ask Council for advisory audit after node execution")
	councilMode := fs.String("council-mode", "dual", "Council mode: status, grok, gpt, or dual")
	councilTimeout := fs.Duration("council-timeout", 5*time.Minute, "Council advisory timeout")
	_ = fs.Parse(args)
	*sandboxBackend = sandbox.NormalizeBackend(*sandboxBackend)
	_ = os.Setenv("QSM_TEST_SANDBOX", *sandboxBackend)
	if *sharedCache {
		_ = os.Setenv("QSM_SHARED_CACHE", "1")
	}
	tuning := resolveNodeRuntimeTuning(*longRun, *nodeMaxSteps, *deepAgentsRecursionLimit, *nodeShellTimeout, *modelMaxRetries)
	applyNodeRuntimeTuning(tuning)
	if tuning.Any() {
		fmt.Printf("Node runtime: long_run=%v max_steps=%d recursion_limit=%d shell_timeout=%ds model_retries=%d\n",
			tuning.LongRun, tuning.NodeMaxSteps, tuning.DeepAgentsRecursionLimit, tuning.NodeShellTimeoutSeconds, tuning.ModelMaxRetries)
	}

	q := mustOpen(*root)
	obj := objective(*request)
	agents := defaultAgents()
	rt := qruntime.Load(*root, qruntime.HarnessMode(*harnessMode))
	configureOpenHarnessReference(rt)
	must(rt.ValidateForRealHarness())
	var planReport planning.Report
	if *routeHealthGate && rt.HarnessMode != qruntime.HarnessSimulated {
		primaryRouteURL := rt.NineRouterURL
		buildHealth := loadBuildHealthState(*root)
		models := routeHealthModelsFromFlag(*routeHealthModelsFlag, agents, rt, *routeHealthLimit)
		results := qruntime.ProbeRouteHealth(context.Background(), rt, models, 30*time.Second)
		fallbackUsed := false
		fallbackModel := ""
		if countHealthyRoutes(results) == 0 && *deepSeekFallback {
			fallbackRT, model, err := deepSeekFallbackRuntime(rt, *deepSeekKeyFile, *deepSeekModel)
			if err != nil {
				fmt.Printf("DeepSeek fallback unavailable: %v\n", err)
			} else if rt.HarnessMode != qruntime.HarnessLangChain {
				fmt.Printf("DeepSeek fallback skipped: harness=%s is not supported for direct fallback\n", rt.HarnessMode)
			} else {
				fallbackResults := qruntime.ProbeRouteHealth(context.Background(), fallbackRT, []string{model}, 30*time.Second)
				results = append(results, fallbackResults...)
				if countHealthyRoutes(fallbackResults) > 0 {
					rt = fallbackRT
					fallbackUsed = true
					fallbackModel = model
					_ = os.Setenv("QSM_LANGCHAIN_MODEL", model)
					fmt.Printf("DeepSeek fallback engaged: model=%s\n", model)
				} else {
					fmt.Printf("DeepSeek fallback failed route-health for model=%s\n", model)
				}
			}
		}
		must(writeRouteHealthState(*root, rt, primaryRouteURL, results, fallbackUsed, fallbackModel, buildHealthForModels(resultModels(results), buildHealth.Models)))
		writeRouteHealthCache(q, obj.ID, results)
		agents = healthyAgents(agents, rt, results, buildHealth.Models)
		fmt.Printf("Route health: %d/%d healthy, agents=%d\n", countHealthyRoutes(results), len(results), len(agents))
	}
	if !*skipPlan {
		var err error
		planReport, err = planning.Generate(*root, q, obj, rt)
		must(err)
		must(writeJSON(filepath.Join(*root, ".state", "plan_report.json"), planReport))
		fmt.Printf("Planning: approved=%v materials=%d artifacts=%d blockers=%d warnings=%d\n",
			planReport.Approved, len(planReport.Materials), len(planReport.Artifacts), len(planReport.Blockers), len(planReport.Warnings))
		if !planReport.Approved {
			log.Fatalf("planning gate blocked Chop: %s", strings.Join(planReport.Blockers, "; "))
		}
	}
	capPlan := capacity.Estimate(capacity.LocalHardware(), capacity.DefaultProfile())
	positionsN := resolvePositions(*positionsFlag, capPlan)
	parallelN := resolveParallel(*parallelFlag, positionsN, rt.HarnessMode)

	hypotheses, err := swarm.Synthesizer{Lake: q}.BrainDump(obj, agents)
	must(err)
	count, err := research.Hydrator{Lake: q}.DigestLocal(*root)
	must(err)
	positions, err := swarm.Chopper{Lake: q, RoomsDir: filepath.Join(*root, ".rooms")}.Chop(obj, hypotheses, positionsN)
	must(err)

	harness := selectHarness(rt, *harnessTimeout)
	report := swarm.Executor{
		Harness:        harness,
		Agents:         agents,
		Concurrency:    parallelN,
		HarnessMode:    string(rt.HarnessMode),
		SandboxBackend: *sandboxBackend,
		MaxRetries:     *retries,
		RetryBackoff:   *retryBackoff,
		Lake:           q,
		SharedCache:    *sharedCache,
	}.Run(context.Background(), obj, positions)
	must(writeJSON(filepath.Join(*root, ".state", "run_report.json"), report))
	buildHealth, err := updateBuildHealthState(*root, report)
	must(err)
	must(writeGroundingArtifacts(q, obj.ID, report))
	lakeReport, err := lakebrain.Analyze(q, report)
	must(err)
	must(lakebrain.Write(*root, lakeReport))
	fmt.Printf("Lake interaction: avg_node_score=%.1f refresh_coverage=%.0f%% write_coverage=%.0f%% enterprise_ready=%v\n",
		lakeReport.AverageNodeScore, lakeReport.RefreshCoverage*100, lakeReport.CacheWriteCoverage*100, lakeReport.EnterpriseReady)
	costReport := costing.Analyze(report)
	must(costing.Write(*root, costReport))
	fmt.Printf("Cost: total_tokens=%d estimated_usd=%.6f cost_per_success=%.6f\n",
		costReport.TotalTokens, costReport.EstimatedUSD, costReport.CostPerSuccess)
	if traceReport, err := buildTraceReport(*root); err == nil {
		_ = writeJSON(filepath.Join(*root, ".state", "trace_report.json"), traceReport)
		_ = os.WriteFile(filepath.Join(*root, ".state", "trace_report.md"), []byte(traceMarkdown(traceReport)), 0644)
	}
	budgetReport := buildCostBudgetReport(*root)
	_ = writeJSON(filepath.Join(*root, ".state", "cost_budget_report.json"), budgetReport)
	_ = os.WriteFile(filepath.Join(*root, ".state", "cost_budget_report.md"), []byte(costBudgetMarkdown(budgetReport)), 0644)

	if *useCouncil {
		advicePrompt := fmt.Sprintf("Advisory audit only. Do not decide deterministic collapse. Review this QSM run for architectural risks and next fixes. Objective=%s requested_nodes=%d succeeded=%d failed=%d harness=%s. Attached file is run_report.json.",
			obj.Request, report.RequestedNodes, report.SucceededNodes, report.FailedNodes, report.HarnessMode)
		advice := council.Client{Timeout: *councilTimeout}.Ask(context.Background(), *councilMode, advicePrompt, filepath.Join(*root, ".state", "run_report.json"))
		must(writeJSON(filepath.Join(*root, ".state", "council_advice.json"), advice))
		_, err := q.Put(lake.Artifact{
			Phase:      lake.PhaseAudit,
			Kind:       "council_advice",
			Source:     "council/" + advice.Mode,
			Claim:      "advisory reasoning captured; not used as deterministic collapse gate",
			Content:    advice.Summary(),
			Confidence: 0.6,
			Verified:   advice.Error == "",
			Metadata: map[string]string{
				"objective_id": obj.ID,
				"attachment":   filepath.Join(*root, ".state", "run_report.json"),
			},
		})
		must(err)
	}

	findings := collapse.Audit(report.Results)
	verdict, err := collapse.ConsensusEngine{Lake: q}.Collapse(report.Results, findings)
	must(err)
	must(writeJSON(filepath.Join(*root, ".state", "verdict.json"), verdict))
	forceScore, err := writeForceScoreArtifacts(*root, q, planReport, report, verdict)
	must(err)
	if !forceLocalCollapseGate(forceScore) {
		verdict.Approved = false
		verdict.Reason = "force requirements local collapse gate failed"
		must(writeJSON(filepath.Join(*root, ".state", "verdict.json"), verdict))
	}
	deliveryPath := ""
	if verdict.Approved && verdict.Winner.ProductPath != "" {
		deliveryPath = filepath.Join(*root, "deliveries", obj.ID)
		must(delivery.CopyProduct(verdict.Winner.ProductPath, deliveryPath))
	}

	artifacts, err := q.List()
	must(err)
	must(wiki.Compiler{OutDir: filepath.Join(*root, "internal", "wiki")}.Compile(artifacts))

	fmt.Printf("QSM run complete: %d hypotheses, %d hydrated files, %d positions, winner=%s approved=%v\n",
		len(hypotheses), count, len(positions), verdict.Winner.PositionID, verdict.Approved)
	fmt.Printf("Nodes: requested=%d started=%d succeeded=%d failed=%d parallel=%d accounted=%v\n",
		report.RequestedNodes, report.StartedNodes, report.SucceededNodes, report.FailedNodes, report.Concurrency, report.AllNodesAccounted)
	for _, result := range report.Results {
		if result.BuildPassed && result.TestPassed && result.LintPassed && result.Error == "" {
			continue
		}
		fmt.Printf("Node failure: %s error=%s\n", result.PositionID, truncateStatusError(resultFailureSummary(result), 480))
	}
	fmt.Printf("Capacity: %s\n", capPlan.Summary())
	fmt.Printf("Harness: %s\n", rt.HarnessMode)
	fmt.Printf("Build health: tracked_models=%d\n", len(buildHealth.Models))
	fmt.Printf("Force score: average=%.1f top_tier=%v\n", forceScore.AverageScore, forceScore.TopTier)
	if *sharedCache {
		fmt.Printf("Shared cache: enabled items=%v\n", report.CacheSummary)
	}
	if deliveryPath != "" {
		fmt.Printf("Delivered product: %s\n", deliveryPath)
	}
}

func writeGroundingArtifacts(q *lake.Lake, objectiveID string, report swarm.RunReport) error {
	if q == nil {
		return nil
	}
	for _, result := range report.Results {
		for i, citation := range result.Citations {
			_, err := q.Put(lake.Artifact{
				Phase:      lake.PhaseAudit,
				Kind:       "grounded_citation",
				Source:     result.PositionID,
				Claim:      fmt.Sprintf("grounded citation %d for %s", i+1, result.PositionID),
				Content:    citation.Quote,
				Confidence: citation.Score,
				Verified:   citation.Quote != "" && citation.Source != "",
				Metadata: map[string]string{
					"objective_id": objectiveID,
					"citation_id":  citation.ID,
					"source":       citation.Source,
					"source_type":  citation.SourceType,
					"sentence_id":  strconv.Itoa(citation.SentenceID),
				},
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func writeForceScoreArtifacts(root string, q *lake.Lake, planReport planning.Report, report swarm.RunReport, verdict collapse.Verdict) (requirements.ScoreReport, error) {
	score := requirements.Score(forceEvidence(root, planReport, report, verdict))
	stateDir := filepath.Join(root, ".state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return score, err
	}
	if err := writeJSON(filepath.Join(stateDir, "force_score.json"), score); err != nil {
		return score, err
	}
	if err := os.WriteFile(filepath.Join(stateDir, "force_score.md"), []byte(score.Markdown()), 0644); err != nil {
		return score, err
	}
	if q != nil {
		_, err := q.Put(lake.Artifact{
			Phase:      lake.PhaseAudit,
			Kind:       "force_requirements_score",
			Source:     "qsm-requirements-scorer",
			Claim:      fmt.Sprintf("force requirements evidence score average %.1f top_tier=%v", score.AverageScore, score.TopTier),
			Content:    score.Markdown(),
			Confidence: 0.85,
			Verified:   true,
			Metadata: map[string]string{
				"objective_id": report.ObjectiveID,
				"harness":      report.HarnessMode,
			},
		})
		if err != nil {
			return score, err
		}
	}
	return score, nil
}

func forceLocalCollapseGate(score requirements.ScoreReport) bool {
	if score.Schema == "" {
		return true
	}
	required := map[int]string{
		2: "Reliability, Availability & 24/7 Operation",
		4: "Observability, Measurability & Countability",
		5: "Security & Compliance",
	}
	for _, category := range score.Checklist.Categories {
		if _, ok := required[category.ID]; !ok {
			continue
		}
		switch category.Status {
		case "FAIL", "GAP":
			return false
		}
	}
	return true
}

func forceEvidence(root string, planReport planning.Report, report swarm.RunReport, verdict collapse.Verdict) requirements.Evidence {
	e := requirements.Evidence{
		ObjectiveID:          report.ObjectiveID,
		HarnessMode:          report.HarnessMode,
		PlanApproved:         planReport.Schema != "" && planReport.Approved,
		PlanBlockers:         len(planReport.Blockers),
		RunPresent:           report.ObjectiveID != "",
		RequestedNodes:       report.RequestedNodes,
		StartedNodes:         report.StartedNodes,
		SucceededNodes:       report.SucceededNodes,
		FailedNodes:          report.FailedNodes,
		Concurrency:          report.Concurrency,
		MaxRetries:           report.MaxRetries,
		AllNodesAccounted:    report.AllNodesAccounted,
		CollapseApproved:     verdict.Approved,
		CacheItems:           sumCacheItems(report.CacheSummary),
		AutorunStatePresent:  fileExists(filepath.Join(root, ".state", "autorun.json")),
		LocalPackageAuditOK:  localPackageAuditOK(root),
		DataLakeAtomicWrites: true,
		CreatedAt:            time.Now().UTC(),
	}
	for _, result := range report.Results {
		if result.TestReport == nil {
			continue
		}
		e.TestCommands += result.TestReport.Summary.Commands
		e.PassedTestCommands += result.TestReport.Summary.PassedCommands
		e.FailedTestCommands += result.TestReport.Summary.FailedCommands
		e.SecurityCritical += result.TestReport.Security.CriticalCount
		e.SecurityHigh += result.TestReport.Security.HighCount
		e.SecurityMedium += result.TestReport.Security.MediumCount
		for _, command := range result.TestReport.Commands {
			if command.Kind == "browser" {
				e.BrowserCommands++
			}
		}
	}
	if routeState, ok := readRouteHealthForEvidence(root); ok {
		e.RouteHealthFresh = time.Since(routeState.CheckedAt) <= 30*time.Minute
		e.HealthyRoutes = countHealthyRoutes(routeState.Results)
		e.TotalRoutes = len(routeState.Results)
	}
	var costReport costing.Report
	if readJSON(filepath.Join(root, ".state", "cost_report.json"), &costReport) == nil && costReport.Schema != "" {
		e.CostReportPresent = true
		e.EstimatedUSD = costReport.EstimatedUSD
		e.EstimatedTokens = costReport.TotalTokens
	}
	var sandboxReport sandbox.Report
	if readJSON(filepath.Join(root, ".state", "sandbox_report.json"), &sandboxReport) == nil && sandboxReport.Schema != "" {
		e.SandboxReportPresent = true
		e.HardSandboxReady = sandboxReport.HardSandboxReady
	}
	var complianceReport ComplianceReport
	if readJSON(filepath.Join(root, ".state", "compliance_report.json"), &complianceReport) == nil && complianceReport.Schema != "" {
		e.CompliancePresent = true
		e.CompliancePassed = complianceReport.Passed
		e.SBOMGenerated = complianceReport.SBOMGenerated
	}
	var benchReport BenchmarkReport
	if readJSON(filepath.Join(root, ".state", "benchmark_report.json"), &benchReport) == nil && benchReport.Schema != "" {
		e.BenchmarkPresent = true
		e.BenchmarkPassedTasks = benchReport.PassedTasks
		e.BenchmarkTotalTasks = len(benchReport.Tasks)
		e.OfficialBenchmark = officialLikeBenchmark(benchReport)
	}
	var traceReport TraceReport
	if readJSON(filepath.Join(root, ".state", "trace_report.json"), &traceReport) == nil && traceReport.Schema != "" {
		e.TraceReportPresent = true
		e.TracePassed = traceReport.Passed
	}
	var coverageReport QualityReport
	if readJSON(filepath.Join(root, ".state", "coverage_report.json"), &coverageReport) == nil && coverageReport.Schema != "" {
		e.CoveragePresent = true
		e.CoveragePassed = coverageReport.Passed
	}
	var mutationReport QualityReport
	if readJSON(filepath.Join(root, ".state", "mutation_report.json"), &mutationReport) == nil && mutationReport.Schema != "" {
		e.MutationPresent = true
		e.MutationPassed = mutationReport.Passed
	}
	var flakeReport QualityReport
	if readJSON(filepath.Join(root, ".state", "flake_report.json"), &flakeReport) == nil && flakeReport.Schema != "" {
		e.FlakePresent = true
		e.FlakePassed = flakeReport.Passed
	}
	var costBudgetReport CostBudgetReport
	if readJSON(filepath.Join(root, ".state", "cost_budget_report.json"), &costBudgetReport) == nil && costBudgetReport.Schema != "" {
		e.CostBudgetPresent = true
		e.CostBudgetPassed = costBudgetReport.Passed
	}
	var ciReport CIReleaseReport
	if readJSON(filepath.Join(root, ".state", "ci_release_report.json"), &ciReport) == nil && ciReport.Schema != "" {
		e.CIReleasePresent = true
		e.CIReleasePassed = ciReport.Passed
		e.CIReleaseCI = ciReport.CI
		e.CIReleaseLocalAllowed = ciReport.LocalAllowed
	}
	e.CIWorkflowPresent = fileExists(filepath.Join(root, ".github", "workflows", "qsm-production-qa.yml"))
	e.LaunchdPlistPresent = fileExists(filepath.Join(root, ".state", "qsm.autorun.plist"))
	var stressReport StressReport
	if readJSON(filepath.Join(root, ".state", "stress_report.json"), &stressReport) == nil && stressReport.Schema != "" {
		e.StressPresent = true
		e.StressPassed = stressReport.Passed
		e.StressNodes = stressReport.Nodes
		e.StressParallel = stressReport.Parallel
		e.LargeRepoFiles = stressReport.LargeRepoFiles
		e.LargeRepoPassed = stressReport.LargeRepoPassed
	}
	var recoveryReport RecoveryReport
	if readJSON(filepath.Join(root, ".state", "recovery_report.json"), &recoveryReport) == nil && recoveryReport.Schema != "" {
		e.RecoveryPresent = true
		e.RecoveryPassed = recoveryReport.Passed
		e.RecoveryRate = recoveryReport.RecoveryRate
	}
	var contributorReport ContributorSmokeReport
	if readJSON(filepath.Join(root, ".state", "contributor_smoke_report.json"), &contributorReport) == nil && contributorReport.Schema != "" {
		e.ContributorPresent = true
		e.ContributorPassed = contributorReport.Passed
	}
	var opsReport OpsReadinessReport
	if readJSON(filepath.Join(root, ".state", "ops_readiness_report.json"), &opsReport) == nil && opsReport.Schema != "" {
		e.OpsReadinessPresent = true
		e.OpsReadinessPassed = opsReport.Passed
		e.ApprovalGateReady = opsReport.ApprovalGateDocumented
		e.CIArtifactRetention = opsReport.CIArtifactRetention
		e.RunbookPresent = opsReport.RunbookPresent
	}
	var selfReport SelfImproveReport
	if readJSON(filepath.Join(root, ".state", "self_improvement_report.json"), &selfReport) == nil && selfReport.Schema != "" {
		e.SelfImprovePresent = true
		e.SelfImprovePassed = selfReport.Passed
		e.SelfImproveCycles = selfReport.Cycles
		e.SelfImproveForceDelta = selfReport.ForceDelta
		e.SelfImproveFailureRate = selfReport.RepeatedFailureRate
		e.SelfImproveLessons = len(selfReport.LessonsPromoted)
	}
	var lakeReport lakebrain.Report
	if readJSON(filepath.Join(root, ".state", "lake_interaction_score.json"), &lakeReport) == nil && lakeReport.Schema != "" {
		e.LakeReportPresent = true
		e.LakeEnterpriseReady = lakeReport.EnterpriseReady
		e.LakeCacheCitation = lakeReport.CacheCitationCoverage
		e.LakeWikiCitation = lakeReport.WikiCitationCoverage
		e.LakeDecisionCitation = lakeReport.DecisionCitationCoverage
	}
	return e
}

func sumCacheItems(summary map[string]int) int {
	total := 0
	for _, count := range summary {
		total += count
	}
	return total
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func localPackageAuditOK(root string) bool {
	if !fileExists(filepath.Join(root, "package-lock.json")) {
		return true
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "npm", "audit", "--prefix", root, "--audit-level=high")
	return cmd.Run() == nil
}

func readRouteHealthForEvidence(root string) (RouteHealthState, bool) {
	var state RouteHealthState
	if err := readJSON(filepath.Join(root, ".state", "route_health.json"), &state); err != nil {
		return RouteHealthState{}, false
	}
	return state, true
}

func doctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	harnessMode := fs.String("harness", "opencode", "harness mode to check: simulated, opencode, or langchain")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	rt := qruntime.Load(*root, qruntime.HarnessMode(*harnessMode))
	checks := rt.Doctor()
	if *jsonOut {
		data, err := json.MarshalIndent(checks, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	allOK := true
	for _, check := range checks {
		status := "OK"
		if !check.OK {
			status = "MISSING"
			allOK = false
		}
		fmt.Printf("[%s] %s: %s\n", status, check.Name, check.Detail)
	}
	if allOK {
		fmt.Println("real harness prerequisites look ready")
	} else {
		fmt.Println("real harness is not ready; use -harness simulated or set the missing runtime pieces")
	}
}

func plan(args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	request := fs.String("request", "Build a tested auditable product", "high-level request")
	root := fs.String("root", ".", "workspace root")
	harnessMode := fs.String("harness", "simulated", "harness mode: simulated, opencode, or langchain")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	q := mustOpen(*root)
	obj := objective(*request)
	rt := qruntime.Load(*root, qruntime.HarnessMode(*harnessMode))
	report, err := planning.Generate(*root, q, obj, rt)
	must(err)
	must(writeJSON(filepath.Join(*root, ".state", "plan_report.json"), report))

	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Printf("Planning: approved=%v materials=%d artifacts=%d blockers=%d warnings=%d\n",
		report.Approved, len(report.Materials), len(report.Artifacts), len(report.Blockers), len(report.Warnings))
	if len(report.Blockers) > 0 {
		fmt.Println("Blockers:")
		for _, blocker := range report.Blockers {
			fmt.Println("- " + blocker)
		}
	}
	if len(report.Warnings) > 0 {
		fmt.Println("Warnings:")
		for _, warning := range report.Warnings {
			fmt.Println("- " + warning)
		}
	}
	fmt.Printf("Saved plan: %s\n", filepath.Join(*root, ".state", "plan_report.json"))
}

type RouteHealthState struct {
	HarnessMode   qruntime.HarnessMode         `json:"harness_mode"`
	NineRouterURL string                       `json:"nine_router_url"`
	PrimaryURL    string                       `json:"primary_url,omitempty"`
	EffectiveURL  string                       `json:"effective_url,omitempty"`
	FallbackUsed  bool                         `json:"fallback_used,omitempty"`
	FallbackModel string                       `json:"fallback_model,omitempty"`
	Models        []string                     `json:"models"`
	Results       []qruntime.RouteHealthResult `json:"results"`
	BuildHealth   map[string]BuildHealthModel  `json:"build_health,omitempty"`
	CheckedAt     time.Time                    `json:"checked_at"`
}

func routeHealth(args []string) {
	fs := flag.NewFlagSet("route-health", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	harnessMode := fs.String("harness", "langchain", "harness mode whose routes should be checked: opencode or langchain")
	modelsFlag := fs.String("models", getenvDefault("QSM_ROUTE_HEALTH_MODELS", "auto"), "model pool: auto, free, all, or comma-separated routes")
	limit := fs.Int("limit", envInt("QSM_ROUTE_HEALTH_LIMIT", 16), "max discovered models to probe for free/all pools")
	timeout := fs.Duration("timeout", 30*time.Second, "per-route probe timeout")
	jsonOut := fs.Bool("json", false, "print JSON")
	writeCache := fs.Bool("write-cache", true, "write route_health items to .lake/cache")
	_ = fs.Parse(args)

	rt := qruntime.Load(*root, qruntime.HarnessMode(*harnessMode))
	must(rt.ValidateForRealHarness())
	models := routeHealthModelsFromFlag(*modelsFlag, defaultAgents(), rt, *limit)
	results := qruntime.ProbeRouteHealth(context.Background(), rt, models, *timeout)
	buildHealth := loadBuildHealthState(*root)
	state := RouteHealthState{
		HarnessMode:   rt.HarnessMode,
		NineRouterURL: rt.NineRouterURL,
		Models:        models,
		Results:       results,
		BuildHealth:   buildHealthForModels(models, buildHealth.Models),
		CheckedAt:     time.Now().UTC(),
	}
	must(writeJSON(filepath.Join(*root, ".state", "route_health.json"), state))
	if *writeCache {
		writeRouteHealthCache(mustOpen(*root), "route-health", results)
	}
	if *jsonOut {
		data, err := json.MarshalIndent(state, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Printf("Route health: %d/%d healthy\n", countHealthyRoutes(results), len(results))
	for _, result := range results {
		status := "FAIL"
		if result.OK {
			status = "OK"
		}
		detail := result.ResponseShape
		if result.Error != "" {
			detail = result.Error
		}
		build := ""
		if item, ok := buildHealth.Models[result.Model]; ok {
			build = fmt.Sprintf(" build=%d/%d %.0f%%", item.Succeeded, item.Attempts, item.SuccessRate*100)
			if buildHealthBlocksRoute(result.Model, buildHealth.Models) {
				build += " blocked"
			}
		}
		fmt.Printf("[%s] %s latency=%dms shape=%s%s detail=%s\n", status, result.Model, result.LatencyMS, result.ResponseShape, build, detail)
	}
	fmt.Printf("Saved route health: %s\n", filepath.Join(*root, ".state", "route_health.json"))
}

func councilCmd(args []string) {
	fs := flag.NewFlagSet("council", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	mode := fs.String("mode", "status", "Council mode: status, awake, grok, gpt, or dual")
	prompt := fs.String("prompt", "", "prompt to send to Council")
	attach := fs.String("attach", "", "optional local file to attach")
	timeout := fs.Duration("timeout", 5*time.Minute, "Council timeout")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	advice := council.Client{Timeout: *timeout}.Ask(context.Background(), *mode, *prompt, *attach)
	must(os.MkdirAll(filepath.Join(*root, ".state"), 0755))
	must(writeJSON(filepath.Join(*root, ".state", "council_advice.json"), advice))
	if *jsonOut {
		data, err := json.MarshalIndent(advice, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Println(advice.CommandOutput)
	if advice.Error != "" {
		fmt.Printf("Council error: %s\n", advice.Error)
	}
	fmt.Printf("Saved advice: %s\n", filepath.Join(*root, ".state", "council_advice.json"))
}

type DeployState struct {
	Root            string    `json:"root"`
	BinaryPath      string    `json:"binary_path"`
	NineRouterURL   string    `json:"nine_router_url"`
	NineRouterApp   string    `json:"nine_router_app"`
	NineRouterPID   int       `json:"nine_router_pid,omitempty"`
	NineRouterLog   string    `json:"nine_router_log"`
	NineRouterLive  bool      `json:"nine_router_live"`
	OpenCodePath    string    `json:"opencode_path"`
	OpenCodeConfig  string    `json:"opencode_config"`
	OpenHarnessRoot string    `json:"openharness_root,omitempty"`
	LangChainRunner string    `json:"langchain_runner"`
	DeployedAt      time.Time `json:"deployed_at"`
}

type AutorunState struct {
	Schema          string    `json:"schema"`
	Root            string    `json:"root"`
	Request         string    `json:"request"`
	HarnessMode     string    `json:"harness_mode"`
	IntervalSeconds int       `json:"interval_seconds"`
	MaxCycles       int       `json:"max_cycles"`
	Cycle           int       `json:"cycle"`
	Running         bool      `json:"running"`
	LastExitCode    int       `json:"last_exit_code"`
	LastLog         string    `json:"last_log,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func autorun(args []string) {
	fs := flag.NewFlagSet("autorun", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	request := fs.String("request", "Build a tested auditable product", "high-level request")
	harnessMode := fs.String("harness", "simulated", "harness mode: simulated, opencode, or langchain")
	positionsFlag := fs.String("positions", "auto", "number of divergent positions, or auto")
	parallelFlag := fs.String("parallel", "auto", "max nodes to execute at once, or auto")
	interval := fs.Duration("interval", 20*time.Minute, "delay between cycles; ignored after final cycle")
	maxCycles := fs.Int("max-cycles", 0, "maximum cycles; 0 runs forever")
	cycleTimeout := fs.Duration("cycle-timeout", 30*time.Minute, "timeout for each qsm run cycle")
	failureBackoff := fs.Duration("failure-backoff", 30*time.Second, "extra delay after a failed cycle")
	retries := fs.Int("retries", 1, "retry count passed to qsm run")
	retryBackoff := fs.Duration("retry-backoff", 10*time.Second, "retry backoff passed to qsm run")
	sharedCache := fs.Bool("shared-cache", true, "enable verified cross-node cache")
	routeHealthGate := fs.Bool("route-health", true, "probe route health before real harness cycles")
	deployRouter := fs.Bool("deploy-router", true, "start managed 9Router before real harness cycles when needed")
	deepSeekFallback := fs.Bool("deepseek-fallback", envBool("QSM_DEEPSEEK_FALLBACK"), "allow direct DeepSeek fallback when 9Router has no healthy routes")
	longRun := fs.Bool("long-run", envBool("QSM_LONG_RUN"), "enable production long-run node budget profile for qsm run cycles")
	nodeMaxSteps := fs.Int("node-max-steps", envInt("QSM_NODE_MAX_STEPS", 0), "LangChain node graph step budget passed to qsm run")
	deepAgentsRecursionLimit := fs.Int("deepagents-recursion-limit", envInt("QSM_DEEPAGENTS_RECURSION_LIMIT", 0), "DeepAgents recursion limit passed to qsm run")
	nodeShellTimeout := fs.Int("node-shell-timeout", envInt("QSM_NODE_SHELL_TIMEOUT", 0), "per-tool shell timeout in seconds passed to qsm run")
	modelMaxRetries := fs.Int("model-max-retries", envInt("QSM_MODEL_MAX_RETRIES", 0), "model client retry count passed to qsm run")
	_ = fs.Parse(args)
	tuning := resolveNodeRuntimeTuning(*longRun, *nodeMaxSteps, *deepAgentsRecursionLimit, *nodeShellTimeout, *modelMaxRetries)

	must(os.MkdirAll(filepath.Join(*root, ".state", "autorun"), 0755))
	statePath := filepath.Join(*root, ".state", "autorun.json")
	state := AutorunState{
		Schema:          "qsm.autorun_state.v1",
		Root:            *root,
		Request:         *request,
		HarnessMode:     *harnessMode,
		IntervalSeconds: int(interval.Seconds()),
		MaxCycles:       *maxCycles,
		LastExitCode:    0,
		StartedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	must(writeJSON(statePath, state))
	exe, err := os.Executable()
	must(err)
	for cycle := 1; *maxCycles == 0 || cycle <= *maxCycles; cycle++ {
		if *deployRouter && qruntime.HarnessMode(*harnessMode) != qruntime.HarnessSimulated {
			rt := qruntime.Load(*root, qruntime.HarnessMode(*harnessMode))
			if !routerLive(rt) {
				if _, err := startRouterProcess(*root, rt); err != nil {
					fmt.Printf("autorun router start failed: %v\n", err)
				}
			}
			_ = waitRouter(rt, 60*time.Second)
		}
		logPath := filepath.Join(*root, ".state", "autorun", fmt.Sprintf("cycle-%04d.log", cycle))
		state.Cycle = cycle
		state.Running = true
		state.LastLog = logPath
		state.LastError = ""
		state.UpdatedAt = time.Now().UTC()
		must(writeJSON(statePath, state))

		exitCode, err := runAutorunCycle(exe, logPath, *cycleTimeout, buildAutorunRunArgs(*root, *request, *harnessMode, *positionsFlag, *parallelFlag, *retries, *retryBackoff, *sharedCache, *routeHealthGate, *deepSeekFallback, tuning))
		state.Running = false
		state.LastExitCode = exitCode
		if err != nil {
			state.LastError = err.Error()
		}
		state.UpdatedAt = time.Now().UTC()
		must(writeJSON(statePath, state))
		fmt.Printf("autorun cycle=%d exit=%d log=%s\n", cycle, exitCode, logPath)

		if *maxCycles != 0 && cycle >= *maxCycles {
			break
		}
		delay := *interval
		if exitCode != 0 {
			delay += *failureBackoff
		}
		time.Sleep(delay)
	}
}

func autorunPlist(args []string) {
	fs := flag.NewFlagSet("autorun-plist", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	request := fs.String("request", "Build a tested auditable product", "high-level request")
	harnessMode := fs.String("harness", "simulated", "harness mode: simulated, opencode, or langchain")
	positionsFlag := fs.String("positions", "auto", "number of divergent positions, or auto")
	parallelFlag := fs.String("parallel", "auto", "max nodes to execute at once, or auto")
	interval := fs.Duration("interval", 20*time.Minute, "launchd StartInterval")
	label := fs.String("label", "local.qsm.autorun", "launchd label")
	out := fs.String("out", "", "output plist path")
	sharedCache := fs.Bool("shared-cache", true, "enable verified cross-node cache")
	routeHealthGate := fs.Bool("route-health", true, "probe route health before real harness cycles")
	deepSeekFallback := fs.Bool("deepseek-fallback", envBool("QSM_DEEPSEEK_FALLBACK"), "allow direct DeepSeek fallback when 9Router has no healthy routes")
	longRun := fs.Bool("long-run", envBool("QSM_LONG_RUN"), "enable production long-run node budget profile for launchd cycles")
	nodeMaxSteps := fs.Int("node-max-steps", envInt("QSM_NODE_MAX_STEPS", 0), "LangChain node graph step budget passed to autorun")
	deepAgentsRecursionLimit := fs.Int("deepagents-recursion-limit", envInt("QSM_DEEPAGENTS_RECURSION_LIMIT", 0), "DeepAgents recursion limit passed to autorun")
	nodeShellTimeout := fs.Int("node-shell-timeout", envInt("QSM_NODE_SHELL_TIMEOUT", 0), "per-tool shell timeout in seconds passed to autorun")
	modelMaxRetries := fs.Int("model-max-retries", envInt("QSM_MODEL_MAX_RETRIES", 0), "model client retry count passed to autorun")
	_ = fs.Parse(args)
	tuning := resolveNodeRuntimeTuning(*longRun, *nodeMaxSteps, *deepAgentsRecursionLimit, *nodeShellTimeout, *modelMaxRetries)

	rootAbs, err := filepath.Abs(*root)
	must(err)
	exe, err := os.Executable()
	must(err)
	if *out == "" {
		*out = filepath.Join(rootAbs, ".state", "qsm.autorun.plist")
	}
	plist := launchdPlist(*label, rootAbs, exe, buildAutorunOneCycleArgs(rootAbs, *request, *harnessMode, *positionsFlag, *parallelFlag, *sharedCache, *routeHealthGate, *deepSeekFallback, tuning), int(interval.Seconds()))
	must(os.MkdirAll(filepath.Dir(*out), 0755))
	must(os.WriteFile(*out, []byte(plist), 0644))
	fmt.Printf("Wrote launchd plist: %s\n", *out)
	fmt.Printf("Load manually when approved: launchctl load %s\n", *out)
}

func launchdPlist(label, root, exe string, args []string, intervalSeconds int) string {
	if intervalSeconds <= 0 {
		intervalSeconds = 1200
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>`)
	b.WriteString(xmlEscape(label))
	b.WriteString(`</string>
  <key>WorkingDirectory</key>
  <string>`)
	b.WriteString(xmlEscape(root))
	b.WriteString(`</string>
  <key>ProgramArguments</key>
  <array>
    <string>`)
	b.WriteString(xmlEscape(exe))
	b.WriteString("</string>\n")
	for _, arg := range args {
		b.WriteString("    <string>")
		b.WriteString(xmlEscape(arg))
		b.WriteString("</string>\n")
	}
	b.WriteString(`  </array>
  <key>StartInterval</key>
  <integer>`)
	b.WriteString(strconv.Itoa(intervalSeconds))
	b.WriteString(`</integer>
  <key>StandardOutPath</key>
  <string>`)
	b.WriteString(xmlEscape(filepath.Join(root, ".state", "autorun.launchd.out.log")))
	b.WriteString(`</string>
  <key>StandardErrorPath</key>
  <string>`)
	b.WriteString(xmlEscape(filepath.Join(root, ".state", "autorun.launchd.err.log")))
	b.WriteString(`</string>
</dict>
</plist>
`)
	return b.String()
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return replacer.Replace(value)
}

type nodeRuntimeTuning struct {
	LongRun                  bool `json:"long_run"`
	NodeMaxSteps             int  `json:"node_max_steps"`
	DeepAgentsRecursionLimit int  `json:"deepagents_recursion_limit"`
	NodeShellTimeoutSeconds  int  `json:"node_shell_timeout_seconds"`
	ModelMaxRetries          int  `json:"model_max_retries"`
}

func (t nodeRuntimeTuning) Any() bool {
	return t.LongRun || t.NodeMaxSteps > 0 || t.DeepAgentsRecursionLimit > 0 || t.NodeShellTimeoutSeconds > 0 || t.ModelMaxRetries > 0
}

func resolveNodeRuntimeTuning(longRun bool, nodeMaxSteps, recursionLimit, shellTimeoutSeconds, modelMaxRetries int) nodeRuntimeTuning {
	tuning := nodeRuntimeTuning{
		LongRun:                  longRun,
		NodeMaxSteps:             positiveInt(nodeMaxSteps),
		DeepAgentsRecursionLimit: positiveInt(recursionLimit),
		NodeShellTimeoutSeconds:  positiveInt(shellTimeoutSeconds),
		ModelMaxRetries:          positiveInt(modelMaxRetries),
	}
	if longRun {
		if tuning.NodeMaxSteps == 0 {
			tuning.NodeMaxSteps = 90
		}
		if tuning.DeepAgentsRecursionLimit == 0 {
			tuning.DeepAgentsRecursionLimit = max(tuning.NodeMaxSteps+30, 120)
		}
		if tuning.NodeShellTimeoutSeconds == 0 {
			tuning.NodeShellTimeoutSeconds = 180
		}
		if tuning.ModelMaxRetries == 0 {
			tuning.ModelMaxRetries = 4
		}
	}
	if tuning.DeepAgentsRecursionLimit > 0 && tuning.NodeMaxSteps > 0 && tuning.DeepAgentsRecursionLimit <= tuning.NodeMaxSteps {
		tuning.DeepAgentsRecursionLimit = tuning.NodeMaxSteps + 10
	}
	return tuning
}

func positiveInt(value int) int {
	if value <= 0 {
		return 0
	}
	return value
}

func applyNodeRuntimeTuning(tuning nodeRuntimeTuning) {
	if tuning.LongRun {
		_ = os.Setenv("QSM_LONG_RUN", "1")
	}
	if tuning.NodeMaxSteps > 0 {
		_ = os.Setenv("QSM_NODE_MAX_STEPS", strconv.Itoa(tuning.NodeMaxSteps))
	}
	if tuning.DeepAgentsRecursionLimit > 0 {
		_ = os.Setenv("QSM_DEEPAGENTS_RECURSION_LIMIT", strconv.Itoa(tuning.DeepAgentsRecursionLimit))
	}
	if tuning.NodeShellTimeoutSeconds > 0 {
		_ = os.Setenv("QSM_NODE_SHELL_TIMEOUT", strconv.Itoa(tuning.NodeShellTimeoutSeconds))
	}
	if tuning.ModelMaxRetries > 0 {
		_ = os.Setenv("QSM_MODEL_MAX_RETRIES", strconv.Itoa(tuning.ModelMaxRetries))
	}
}

func appendNodeRuntimeTuningArgs(args []string, tuning nodeRuntimeTuning) []string {
	if tuning.LongRun {
		args = append(args, "-long-run=true")
	}
	if tuning.NodeMaxSteps > 0 {
		args = append(args, "-node-max-steps", strconv.Itoa(tuning.NodeMaxSteps))
	}
	if tuning.DeepAgentsRecursionLimit > 0 {
		args = append(args, "-deepagents-recursion-limit", strconv.Itoa(tuning.DeepAgentsRecursionLimit))
	}
	if tuning.NodeShellTimeoutSeconds > 0 {
		args = append(args, "-node-shell-timeout", strconv.Itoa(tuning.NodeShellTimeoutSeconds))
	}
	if tuning.ModelMaxRetries > 0 {
		args = append(args, "-model-max-retries", strconv.Itoa(tuning.ModelMaxRetries))
	}
	return args
}

func buildAutorunRunArgs(root, request, harnessMode, positions, parallel string, retries int, retryBackoff time.Duration, sharedCache, routeHealth, deepSeekFallback bool, tuning nodeRuntimeTuning) []string {
	args := []string{
		"run",
		"-root", root,
		"-request", request,
		"-harness", harnessMode,
		"-positions", positions,
		"-parallel", parallel,
		"-retries", strconv.Itoa(retries),
		"-retry-backoff", retryBackoff.String(),
		"-shared-cache=" + strconv.FormatBool(sharedCache),
		"-route-health=" + strconv.FormatBool(routeHealth),
		"-deepseek-fallback=" + strconv.FormatBool(deepSeekFallback),
	}
	return appendNodeRuntimeTuningArgs(args, tuning)
}

func buildAutorunOneCycleArgs(root, request, harnessMode, positions, parallel string, sharedCache, routeHealth, deepSeekFallback bool, tuning nodeRuntimeTuning) []string {
	args := []string{
		"autorun",
		"-root", root,
		"-request", request,
		"-harness", harnessMode,
		"-positions", positions,
		"-parallel", parallel,
		"-max-cycles", "1",
		"-interval", "1s",
		"-shared-cache=" + strconv.FormatBool(sharedCache),
		"-route-health=" + strconv.FormatBool(routeHealth),
		"-deepseek-fallback=" + strconv.FormatBool(deepSeekFallback),
	}
	return appendNodeRuntimeTuningArgs(args, tuning)
}

func runAutorunCycle(exe, logPath string, timeout time.Duration, args []string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return -1, err
	}
	defer logFile.Close()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return -1, fmt.Errorf("cycle timed out after %s", timeout)
	}
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), err
	}
	return -1, err
}

func deploy(args []string) {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	harnessMode := fs.String("harness", "opencode", "harness mode to deploy-check: opencode or langchain")
	startRouter := fs.Bool("start-router", true, "start managed 9Router when it is not live")
	buildBinary := fs.Bool("build", true, "build ./qsm binary")
	wait := fs.Duration("wait", 90*time.Second, "time to wait for 9Router health")
	_ = fs.Parse(args)

	must(os.MkdirAll(filepath.Join(*root, ".state"), 0755))
	rt := qruntime.Load(*root, qruntime.HarnessMode(*harnessMode))
	configureOpenHarnessReference(rt)
	binaryPath := filepath.Join(*root, "qsm")
	if *buildBinary {
		cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/qsm")
		cmd.Dir = *root
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Fatalf("build failed: %v\n%s", err, string(out))
		}
	}

	pid := 0
	if routerLive(rt) {
		pid = readPID(routerPIDPath(*root))
	} else if *startRouter {
		var err error
		pid, err = startRouterProcess(*root, rt)
		must(err)
	}
	live := waitRouter(rt, *wait)

	state := DeployState{
		Root:            *root,
		BinaryPath:      binaryPath,
		NineRouterURL:   rt.NineRouterURL,
		NineRouterApp:   rt.NineRouterApp,
		NineRouterPID:   pid,
		NineRouterLog:   routerLogPath(*root),
		NineRouterLive:  live,
		OpenCodePath:    rt.OpenCodePath,
		OpenCodeConfig:  rt.OpenCodeConfig,
		OpenHarnessRoot: rt.OpenHarnessRoot,
		LangChainRunner: rt.LangChainRunner,
		DeployedAt:      time.Now().UTC(),
	}
	must(writeJSON(filepath.Join(*root, ".state", "deploy.json"), state))

	fmt.Printf("QSM deploy complete: binary=%s\n", binaryPath)
	fmt.Printf("9Router: live=%v url=%s pid=%d log=%s\n", live, rt.NineRouterURL, pid, routerLogPath(*root))
	if err := rt.ValidateForRealHarness(); err != nil {
		fmt.Printf("Real harness: not ready: %v\n", err)
	} else {
		fmt.Printf("Real harness: ready (%s)\n", rt.HarnessMode)
	}
}

func stop(args []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	_ = fs.Parse(args)
	pidPath := routerPIDPath(*root)
	pid := readPID(pidPath)
	if pid == 0 {
		fmt.Println("No managed 9Router PID found")
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	_ = os.Remove(pidPath)
	fmt.Printf("Stopped managed 9Router pid=%d\n", pid)
}

func status(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	var report swarm.RunReport
	must(readJSON(filepath.Join(*root, ".state", "run_report.json"), &report))
	var verdict collapse.Verdict
	_ = readJSON(filepath.Join(*root, ".state", "verdict.json"), &verdict)
	var planReport planning.Report
	_ = readJSON(filepath.Join(*root, ".state", "plan_report.json"), &planReport)
	var forceScore requirements.ScoreReport
	_ = readJSON(filepath.Join(*root, ".state", "force_score.json"), &forceScore)
	var lakeReport lakebrain.Report
	_ = readJSON(filepath.Join(*root, ".state", "lake_interaction_score.json"), &lakeReport)
	var lakeMaintenance lake.MaintenanceReport
	_ = readJSON(filepath.Join(*root, ".state", "lake_maintenance_report.json"), &lakeMaintenance)
	var lakePromotion lake.PromotionReport
	_ = readJSON(filepath.Join(*root, ".state", "lake_promotion_report.json"), &lakePromotion)
	var costReport costing.Report
	_ = readJSON(filepath.Join(*root, ".state", "cost_report.json"), &costReport)
	var sandboxReport sandbox.Report
	_ = readJSON(filepath.Join(*root, ".state", "sandbox_report.json"), &sandboxReport)
	var benchReport BenchmarkReport
	_ = readJSON(filepath.Join(*root, ".state", "benchmark_report.json"), &benchReport)
	roomStatuses := collectRoomStatuses(report)
	buildHealth := loadBuildHealthState(*root)

	if *jsonOut {
		data, err := json.MarshalIndent(map[string]any{"report": report, "verdict": verdict, "plan": planReport, "force_score": forceScore, "lake_interaction": lakeReport, "lake_maintenance": lakeMaintenance, "lake_promotion": lakePromotion, "cost": costReport, "sandbox": sandboxReport, "benchmark": benchReport, "room_statuses": roomStatuses, "build_health": buildHealth}, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}

	fmt.Printf("Objective: %s\n", report.ObjectiveID)
	fmt.Printf("Harness: %s\n", report.HarnessMode)
	if planReport.Schema != "" {
		if planReport.ObjectiveID == report.ObjectiveID {
			fmt.Printf("Plan: approved=%v materials=%d artifacts=%d blockers=%d warnings=%d\n",
				planReport.Approved, len(planReport.Materials), len(planReport.Artifacts), len(planReport.Blockers), len(planReport.Warnings))
		} else {
			fmt.Printf("Plan: latest_plan_objective=%s approved=%v blockers=%d (not current run)\n",
				planReport.ObjectiveID, planReport.Approved, len(planReport.Blockers))
		}
	}
	if forceScore.Schema != "" {
		if forceScore.ObjectiveID == report.ObjectiveID {
			fmt.Printf("Force score: average=%.1f top_tier=%v categories=%d\n", forceScore.AverageScore, forceScore.TopTier, len(forceScore.Checklist.Categories))
		} else {
			fmt.Printf("Force score: latest_score_objective=%s average=%.1f top_tier=%v (not current run)\n", forceScore.ObjectiveID, forceScore.AverageScore, forceScore.TopTier)
		}
	}
	fmt.Printf("Nodes: requested=%d started=%d succeeded=%d failed=%d parallel=%d accounted=%v\n",
		report.RequestedNodes, report.StartedNodes, report.SucceededNodes, report.FailedNodes, report.Concurrency, report.AllNodesAccounted)
	if len(report.CacheSummary) > 0 {
		fmt.Printf("Cache: %v\n", report.CacheSummary)
	} else if q, err := lake.Open(filepath.Join(*root, ".lake")); err == nil {
		if summary, err := q.CacheSummary(report.ObjectiveID); err == nil && len(summary) > 0 {
			fmt.Printf("Cache: %v\n", summary)
		}
	}
	if lakeReport.Schema != "" {
		if lakeReport.ObjectiveID == report.ObjectiveID {
			fmt.Printf("Lake interaction: avg_node_score=%.1f refresh=%d coverage=%.0f%% writes=%.0f%% citations=%.0f%% enterprise_ready=%v\n",
				lakeReport.AverageNodeScore, lakeReport.RefreshEvents, lakeReport.RefreshCoverage*100, lakeReport.CacheWriteCoverage*100, lakeReport.CacheCitationCoverage*100, lakeReport.EnterpriseReady)
		} else {
			fmt.Printf("Lake interaction: latest_objective=%s avg_node_score=%.1f enterprise_ready=%v (not current run)\n",
				lakeReport.ObjectiveID, lakeReport.AverageNodeScore, lakeReport.EnterpriseReady)
		}
	}
	if lakeMaintenance.Schema != "" {
		fmt.Printf("Lake maintenance: total=%d kept=%d quarantine_candidates=%d promotions=%d apply=%v\n",
			lakeMaintenance.TotalCacheItems, lakeMaintenance.KeptCacheItems, lakeMaintenance.QuarantineCount, len(lakeMaintenance.PromotionCandidates), lakeMaintenance.Apply)
	}
	if lakePromotion.Schema != "" {
		fmt.Printf("Lake promotion: reviewed=%d promoted=%d rejected=%d apply=%v\n",
			lakePromotion.Reviewed, lakePromotion.Promoted, lakePromotion.Rejected, lakePromotion.Apply)
	}
	if costReport.Schema != "" {
		if costReport.ObjectiveID == report.ObjectiveID {
			fmt.Printf("Cost: tokens=%d estimated_usd=%.6f cost_per_success=%.6f models=%d\n",
				costReport.TotalTokens, costReport.EstimatedUSD, costReport.CostPerSuccess, len(costReport.ModelSummaries))
		} else {
			fmt.Printf("Cost: latest_objective=%s tokens=%d estimated_usd=%.6f (not current run)\n",
				costReport.ObjectiveID, costReport.TotalTokens, costReport.EstimatedUSD)
		}
	}
	if sandboxReport.Schema != "" {
		fmt.Printf("Sandbox: level=%s hard_ready=%v docker=%v microvm=%v\n",
			sandboxReport.ReadinessLevel, sandboxReport.HardSandboxReady, sandboxReport.Docker.Available, sandboxReport.MicroVMRecommended)
	}
	if benchReport.Schema != "" {
		fmt.Printf("Benchmark: suite=%s passed=%d/%d style=%s\n",
			benchReport.Suite, benchReport.PassedTasks, len(benchReport.Tasks), benchReport.Style)
	}
	if verdict.Winner.PositionID != "" {
		fmt.Printf("Collapse: winner=%s approved=%v reason=%s\n", verdict.Winner.PositionID, verdict.Approved, verdict.Reason)
	}
	for _, result := range report.Results {
		state := "failed"
		if result.BuildPassed && result.TestPassed && result.LintPassed && result.Error == "" {
			state = "succeeded"
		}
		citationText := ""
		if len(result.Citations) > 0 {
			citationText = fmt.Sprintf(" citations=%d", len(result.Citations))
		}
		testText := ""
		if result.TestReport != nil {
			testText = fmt.Sprintf(" tests=%d/%d cmds=%d/%d", result.TestReport.Summary.PassedTests, result.TestReport.Summary.Tests, result.TestReport.Summary.PassedCommands, result.TestReport.Summary.Commands)
			testText += fmt.Sprintf(" security=C%d/H%d/M%d", result.TestReport.Security.CriticalCount, result.TestReport.Security.HighCount, result.TestReport.Security.MediumCount)
		}
		fmt.Printf("- %s %s agent=%s attempts=%d score=%.2f%s%s product=%s\n", result.PositionID, state, result.AgentID, result.Attempts, result.Score, citationText, testText, result.ProductPath)
		if result.Error != "" {
			fmt.Printf("  error: %s\n", result.Error)
		}
	}
	if len(roomStatuses) > 0 {
		fmt.Println("Room health:")
		for _, status := range roomStatuses {
			line := fmt.Sprintf("- %s state=%s phase=%s model=%s cache_refresh=%d product=%v evidence=%v test_cmds=%d failed_cmds=%d tests=%d security=C%d/H%d/M%d",
				status.PositionID, status.State, status.Phase, status.AgentModel, status.CacheRefreshCount, status.ProductReady, status.EvidenceReady, status.TestCommands, status.FailedTestCommands, status.TestCount, status.SecurityCritical, status.SecurityHigh, status.SecurityMedium)
			if status.Error != "" {
				line += " error=" + truncateStatusError(status.Error, 160)
			}
			fmt.Println(line)
		}
	}
	if len(buildHealth.Models) > 0 {
		fmt.Println("Build health:")
		for _, item := range buildHealthSummary(buildHealth.Models, 8) {
			blocked := ""
			if buildHealthBlocksRoute(item.Model, buildHealth.Models) {
				blocked = " blocked"
			}
			fmt.Printf("- %s success=%d/%d rate=%.0f%% last=%s/%s%s\n",
				item.Model, item.Succeeded, item.Attempts, item.SuccessRate*100, item.LastState, item.LastPositionID, blocked)
		}
	}
}

func lakeScore(args []string) {
	fs := flag.NewFlagSet("lake-score", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	var report swarm.RunReport
	must(readJSON(filepath.Join(*root, ".state", "run_report.json"), &report))
	q := mustOpen(*root)
	lakeReport, err := lakebrain.Analyze(q, report)
	must(err)
	must(lakebrain.Write(*root, lakeReport))
	if *jsonOut {
		data, err := json.MarshalIndent(lakeReport, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(lakebrain.Markdown(lakeReport))
}

func lakeMaintain(args []string) {
	fs := flag.NewFlagSet("lake-maintain", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	apply := fs.Bool("apply", false, "move prune candidates into .lake/quarantine/cache instead of only reporting")
	staleDays := fs.Int("stale-days", 30, "quarantine non-constraint cache items older than this many days")
	routeHealthStaleHours := fs.Int("route-health-stale-hours", 24, "quarantine route-health items older than this many hours")
	minConfidence := fs.Float64("min-confidence", 0.45, "quarantine cache items below this confidence")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	q := mustOpen(*root)
	report, err := q.MaintainCache(lake.MaintenancePolicy{
		Apply:                 *apply,
		StaleAfter:            time.Duration(*staleDays) * 24 * time.Hour,
		RouteHealthStaleAfter: time.Duration(*routeHealthStaleHours) * time.Hour,
		MinConfidence:         *minConfidence,
	})
	must(err)
	must(lake.WriteMaintenanceReport(*root, report))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(lake.MaintenanceMarkdown(report))
}

func lakePromote(args []string) {
	fs := flag.NewFlagSet("lake-promote", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	apply := fs.Bool("apply", false, "write accepted promotions into .lake/curated and .lake/artifacts")
	minRepeat := fs.Int("min-repeat", 3, "minimum repeated matching verified recipes before promotion")
	minConfidence := fs.Float64("min-confidence", 0.75, "minimum average confidence before promotion")
	maxPromotions := fs.Int("max-promotions", 12, "maximum accepted promotions")
	compile := fs.Bool("compile-wiki", true, "compile internal/wiki after apply")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	q := mustOpen(*root)
	report, err := q.PromoteCache(lake.PromotionPolicy{
		Apply:         *apply,
		MinRepeat:     *minRepeat,
		MinConfidence: *minConfidence,
		MaxPromotions: *maxPromotions,
	})
	must(err)
	must(lake.WritePromotionReport(*root, report))
	if *apply && *compile {
		artifacts, err := q.List()
		must(err)
		must(wiki.Compiler{OutDir: filepath.Join(*root, "internal", "wiki")}.Compile(artifacts))
	}
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(lake.PromotionMarkdown(report))
}

func forceScoreCmd(args []string) {
	fs := flag.NewFlagSet("force-score", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	var report swarm.RunReport
	must(readJSON(filepath.Join(*root, ".state", "run_report.json"), &report))
	var verdict collapse.Verdict
	_ = readJSON(filepath.Join(*root, ".state", "verdict.json"), &verdict)
	var planReport planning.Report
	_ = readJSON(filepath.Join(*root, ".state", "plan_report.json"), &planReport)
	score, err := writeForceScoreArtifacts(*root, mustOpen(*root), planReport, report, verdict)
	must(err)
	if *jsonOut {
		data, err := json.MarshalIndent(score, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(score.Markdown())
}

func costCmd(args []string) {
	fs := flag.NewFlagSet("cost", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	var report swarm.RunReport
	must(readJSON(filepath.Join(*root, ".state", "run_report.json"), &report))
	costReport := costing.Analyze(report)
	must(costing.Write(*root, costReport))
	if *jsonOut {
		data, err := json.MarshalIndent(costReport, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(costing.Markdown(costReport))
}

func sandboxCmd(args []string) {
	fs := flag.NewFlagSet("sandbox", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	probe := fs.Bool("probe", false, "run sandbox access/escape probes")
	backend := fs.String("backend", "auto", "sandbox probe backend: room, docker, or auto")
	image := fs.String("image", "", "Docker image for sandbox probe")
	profile := fs.String("profile", "default", "sandbox probe profile: default or omni")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	if strings.TrimSpace(*image) != "" {
		_ = os.Setenv("QSM_SANDBOX_DOCKER_IMAGE", strings.TrimSpace(*image))
	}

	report := sandbox.Inspect(*root)
	if *probe {
		report = sandbox.InspectWithProbe(*root, *backend)
		if strings.EqualFold(*profile, "omni") {
			report = enrichOmniSandboxProbe(report)
		}
	}
	must(sandbox.Write(*root, report))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(sandbox.Markdown(report))
}

func enrichOmniSandboxProbe(report sandbox.Report) sandbox.Report {
	report.Findings = append(report.Findings, "Omni sandbox profile requested: probing Go, Node/npm, Python, pytest, Playwright, git, curl, and Docker socket absence.")
	if report.Probe.Backend != sandbox.BackendDocker || !report.Probe.Passed {
		report.HardSandboxReady = false
		report.Findings = append(report.Findings, "Omni sandbox profile cannot pass until the Docker access/escape probe passes.")
		return report
	}
	room := filepath.Join(report.Root, ".state", "sandbox_probe", "omni-profile")
	_ = os.RemoveAll(room)
	if err := os.MkdirAll(room, 0755); err != nil {
		report.HardSandboxReady = false
		report.Probe.Passed = false
		report.Probe.Error = "omni probe setup failed: " + err.Error()
		return report
	}
	checks := []struct {
		name string
		cmd  []string
	}{
		{"go", []string{"go", "version"}},
		{"node", []string{"node", "--version"}},
		{"npm", []string{"npm", "--version"}},
		{"python", []string{"python3", "--version"}},
		{"pytest", []string{"python3", "-m", "pytest", "--version"}},
		{"playwright", []string{"node", "-e", `require("playwright"); console.log("playwright-ok")`}},
		{"git", []string{"git", "--version"}},
		{"curl", []string{"curl", "--version"}},
		{"docker-socket-absent", []string{"node", "-e", `process.exit(require("fs").existsSync("/var/run/docker.sock") ? 1 : 0)`}},
	}
	runner := sandbox.NewRunner(sandbox.BackendDocker)
	var failed []string
	for _, check := range checks {
		res := runner.Run(context.Background(), sandbox.Command{
			Name:    "omni " + check.name,
			Room:    room,
			CWD:     room,
			Cmd:     check.cmd,
			Timeout: 45 * time.Second,
		})
		if res.ExitCode != 0 || res.Error != "" {
			failed = append(failed, check.name)
		}
	}
	if len(failed) > 0 {
		report.HardSandboxReady = false
		report.Probe.Passed = false
		report.Probe.Error = "omni runtime probe failed: " + strings.Join(failed, ", ")
		report.Findings = append(report.Findings, report.Probe.Error)
		return report
	}
	report.ReadinessLevel = "omni-docker-probed"
	report.Findings = append(report.Findings, "Omni sandbox runtime probe passed.")
	return report
}

type TraceReport struct {
	Schema       string      `json:"schema"`
	Root         string      `json:"root"`
	ObjectiveID  string      `json:"objective_id,omitempty"`
	Passed       bool        `json:"passed"`
	Rooms        int         `json:"rooms"`
	MissingRooms int         `json:"missing_rooms"`
	Events       int         `json:"events"`
	Starts       int         `json:"starts"`
	Ends         int         `json:"ends"`
	Unmatched    int         `json:"unmatched"`
	CWDEscapes   int         `json:"cwd_escapes"`
	Errors       []string    `json:"errors,omitempty"`
	RoomReports  []TraceRoom `json:"room_reports,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
}

type TraceRoom struct {
	PositionID string `json:"position_id"`
	Room       string `json:"room"`
	TracePath  string `json:"trace_path"`
	Events     int    `json:"events"`
	Starts     int    `json:"starts"`
	Ends       int    `json:"ends"`
	Unmatched  int    `json:"unmatched"`
	CWDEscapes int    `json:"cwd_escapes"`
	Missing    bool   `json:"missing"`
	Error      string `json:"error,omitempty"`
}

func traceCmd(args []string) {
	fs := flag.NewFlagSet("trace", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	report, err := buildTraceReport(*root)
	must(err)
	must(writeJSON(filepath.Join(*root, ".state", "trace_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "trace_report.md"), []byte(traceMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(traceMarkdown(report))
}

func buildTraceReport(root string) (TraceReport, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return TraceReport{}, err
	}
	var runReport swarm.RunReport
	_ = readJSON(filepath.Join(rootAbs, ".state", "run_report.json"), &runReport)
	out := TraceReport{Schema: "qsm.trace_report.v1", Root: rootAbs, ObjectiveID: runReport.ObjectiveID, Passed: true, CreatedAt: time.Now().UTC()}
	for _, result := range runReport.Results {
		if !(result.BuildPassed && result.TestPassed && result.LintPassed && result.Error == "") {
			continue
		}
		room := result.Room
		tracePath := ""
		if result.TestReport != nil && result.TestReport.TracePath != "" {
			tracePath = result.TestReport.TracePath
		} else if room != "" {
			tracePath = filepath.Join(room, ".qsm_test", "trace.jsonl")
		}
		item := analyzeTraceRoom(rootAbs, result.PositionID, room, tracePath)
		out.RoomReports = append(out.RoomReports, item)
		out.Rooms++
		out.Events += item.Events
		out.Starts += item.Starts
		out.Ends += item.Ends
		out.Unmatched += item.Unmatched
		out.CWDEscapes += item.CWDEscapes
		if item.Missing {
			out.MissingRooms++
		}
		if item.Error != "" {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: %s", item.PositionID, item.Error))
		}
	}
	if out.Rooms == 0 {
		out.Passed = false
		out.Errors = append(out.Errors, "no successful rooms found to trace")
	}
	if out.MissingRooms > 0 || out.Unmatched > 0 || out.CWDEscapes > 0 || len(out.Errors) > 0 {
		out.Passed = false
	}
	return out, nil
}

func mustTraceReport(root string) TraceReport {
	report, err := buildTraceReport(root)
	if err != nil {
		return TraceReport{Schema: "qsm.trace_report.v1", Root: root, Passed: false, Errors: []string{err.Error()}, CreatedAt: time.Now().UTC()}
	}
	return report
}

func analyzeTraceRoom(root, positionID, room, tracePath string) TraceRoom {
	item := TraceRoom{PositionID: positionID, Room: room, TracePath: tracePath}
	data, err := os.ReadFile(tracePath)
	if err != nil {
		item.Missing = true
		item.Error = "missing trace file"
		return item
	}
	roomAbs, _ := filepath.Abs(room)
	starts := map[string]int{}
	ends := map[string]int{}
	scanner := bufioNewScanner(string(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) != nil {
			item.Error = "invalid trace jsonl"
			continue
		}
		item.Events++
		name := fmt.Sprint(ev["name"])
		if name == "" || name == "<nil>" {
			name = fmt.Sprintf("event-%d", item.Events)
		}
		switch fmt.Sprint(ev["type"]) {
		case "command_start":
			item.Starts++
			starts[name]++
			cwd := fmt.Sprint(ev["cwd"])
			if cwd != "" && cwd != "<nil>" {
				cwdPath := cwd
				if !filepath.IsAbs(cwdPath) {
					cwdPath = filepath.Join(roomAbs, cwdPath)
				}
				if abs, err := filepath.Abs(cwdPath); err == nil && !pathInside(roomAbs, abs) {
					item.CWDEscapes++
				}
			}
		case "command_end":
			item.Ends++
			ends[name]++
		}
	}
	for name, count := range starts {
		if ends[name] < count {
			item.Unmatched += count - ends[name]
		}
	}
	return item
}

func traceMarkdown(report TraceReport) string {
	var b strings.Builder
	b.WriteString("# QSM Trace Report\n\n")
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	fmt.Fprintf(&b, "- Objective: `%s`\n", report.ObjectiveID)
	fmt.Fprintf(&b, "- Rooms: `%d`\n", report.Rooms)
	fmt.Fprintf(&b, "- Events: `%d` starts=`%d` ends=`%d`\n", report.Events, report.Starts, report.Ends)
	fmt.Fprintf(&b, "- Missing rooms: `%d`\n", report.MissingRooms)
	fmt.Fprintf(&b, "- Unmatched starts: `%d`\n", report.Unmatched)
	fmt.Fprintf(&b, "- CWD escapes: `%d`\n", report.CWDEscapes)
	if len(report.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, err := range report.Errors {
			b.WriteString("- " + err + "\n")
		}
	}
	b.WriteString("\n## Rooms\n\n")
	b.WriteString("| Position | Missing | Events | Starts | Ends | Unmatched | Escapes |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: |\n")
	for _, room := range report.RoomReports {
		fmt.Fprintf(&b, "| %s | %v | %d | %d | %d | %d | %d |\n", room.PositionID, room.Missing, room.Events, room.Starts, room.Ends, room.Unmatched, room.CWDEscapes)
	}
	return b.String()
}

type CostBudgetReport struct {
	Schema            string    `json:"schema"`
	ObjectiveID       string    `json:"objective_id,omitempty"`
	Passed            bool      `json:"passed"`
	MaxTokens         int       `json:"max_tokens"`
	MaxUSD            float64   `json:"max_usd"`
	MaxDurationMS     int64     `json:"max_duration_ms"`
	MaxCostPerSuccess float64   `json:"max_cost_per_success_usd"`
	ObservedTokens    int       `json:"observed_tokens"`
	ObservedUSD       float64   `json:"observed_usd"`
	ObservedDuration  int64     `json:"observed_duration_ms"`
	ObservedCPS       float64   `json:"observed_cost_per_success_usd"`
	Warnings          []string  `json:"warnings,omitempty"`
	Errors            []string  `json:"errors,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

func costBudgetCmd(args []string) {
	fs := flag.NewFlagSet("cost-budget", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	report := buildCostBudgetReport(*root)
	must(writeJSON(filepath.Join(*root, ".state", "cost_budget_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "cost_budget_report.md"), []byte(costBudgetMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(costBudgetMarkdown(report))
}

func buildCostBudgetReport(root string) CostBudgetReport {
	var runReport swarm.RunReport
	_ = readJSON(filepath.Join(root, ".state", "run_report.json"), &runReport)
	var costReport costing.Report
	_ = readJSON(filepath.Join(root, ".state", "cost_report.json"), &costReport)
	out := CostBudgetReport{
		Schema:            "qsm.cost_budget_report.v1",
		ObjectiveID:       runReport.ObjectiveID,
		Passed:            true,
		MaxTokens:         envInt("QSM_BUDGET_MAX_TOKENS", 250000),
		MaxUSD:            envFloat("QSM_BUDGET_MAX_USD", 0),
		MaxDurationMS:     int64(envDurationDefault("QSM_BUDGET_MAX_DURATION", 30*time.Minute).Milliseconds()),
		MaxCostPerSuccess: envFloat("QSM_BUDGET_MAX_COST_PER_SUCCESS", 0),
		ObservedTokens:    costReport.TotalTokens,
		ObservedUSD:       costReport.EstimatedUSD,
		ObservedDuration:  runReport.DurationMS,
		ObservedCPS:       costReport.CostPerSuccess,
		CreatedAt:         time.Now().UTC(),
	}
	if costReport.Schema == "" {
		out.Errors = append(out.Errors, "missing .state/cost_report.json")
	}
	if out.ObservedTokens > out.MaxTokens {
		out.Errors = append(out.Errors, fmt.Sprintf("tokens %d exceed budget %d", out.ObservedTokens, out.MaxTokens))
	}
	if out.MaxUSD > 0 && out.ObservedUSD > out.MaxUSD {
		out.Errors = append(out.Errors, fmt.Sprintf("cost %.6f exceeds budget %.6f", out.ObservedUSD, out.MaxUSD))
	}
	if out.MaxDurationMS > 0 && out.ObservedDuration > out.MaxDurationMS {
		out.Errors = append(out.Errors, fmt.Sprintf("duration %dms exceeds budget %dms", out.ObservedDuration, out.MaxDurationMS))
	}
	if out.MaxCostPerSuccess > 0 && out.ObservedCPS > out.MaxCostPerSuccess {
		out.Errors = append(out.Errors, fmt.Sprintf("cost per success %.6f exceeds budget %.6f", out.ObservedCPS, out.MaxCostPerSuccess))
	}
	if out.ObservedUSD == 0 {
		out.Warnings = append(out.Warnings, "cost rates are unset or zero; USD budget is informational until QSM_COST_USD_PER_1M_* is configured")
	}
	out.Passed = len(out.Errors) == 0
	return out
}

func costBudgetMarkdown(report CostBudgetReport) string {
	var b strings.Builder
	b.WriteString("# QSM Cost Budget Report\n\n")
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	fmt.Fprintf(&b, "- Tokens: `%d / %d`\n", report.ObservedTokens, report.MaxTokens)
	fmt.Fprintf(&b, "- Cost: `$%.6f / $%.6f`\n", report.ObservedUSD, report.MaxUSD)
	fmt.Fprintf(&b, "- Duration: `%dms / %dms`\n", report.ObservedDuration, report.MaxDurationMS)
	if len(report.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, err := range report.Errors {
			b.WriteString("- " + err + "\n")
		}
	}
	if len(report.Warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, warning := range report.Warnings {
			b.WriteString("- " + warning + "\n")
		}
	}
	return b.String()
}

type QualityReport struct {
	Schema    string        `json:"schema"`
	Kind      string        `json:"kind"`
	Passed    bool          `json:"passed"`
	Sandbox   string        `json:"sandbox"`
	Items     []QualityItem `json:"items,omitempty"`
	Warnings  []string      `json:"warnings,omitempty"`
	Errors    []string      `json:"errors,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}

type QualityItem struct {
	PositionID string   `json:"position_id"`
	Product    string   `json:"product,omitempty"`
	Type       string   `json:"type,omitempty"`
	Command    []string `json:"command,omitempty"`
	ExitCode   int      `json:"exit_code,omitempty"`
	Passed     bool     `json:"passed"`
	Details    string   `json:"details,omitempty"`
	Runs       []int    `json:"runs,omitempty"`
	Mutation   string   `json:"mutation,omitempty"`
}

func coverageCmd(args []string) {
	fs := flag.NewFlagSet("coverage", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	backend := fs.String("sandbox", sandbox.BackendAuto, "sandbox backend: room, docker, or auto")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	report := buildCoverageReport(*root, *backend)
	must(writeJSON(filepath.Join(*root, ".state", "coverage_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "coverage_report.md"), []byte(qualityMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(qualityMarkdown(report))
}

func buildCoverageReport(root, backend string) QualityReport {
	var runReport swarm.RunReport
	_ = readJSON(filepath.Join(root, ".state", "run_report.json"), &runReport)
	runner := sandbox.NewRunner(backend)
	out := QualityReport{Schema: "qsm.coverage_report.v1", Kind: "coverage", Passed: true, Sandbox: runner.Backend(), CreatedAt: time.Now().UTC()}
	for _, result := range runReport.Results {
		if result.ProductPath == "" || !(result.BuildPassed && result.TestPassed && result.LintPassed && result.Error == "") {
			continue
		}
		item := coverageItem(context.Background(), runner, result)
		out.Items = append(out.Items, item)
		if !item.Passed {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: %s", item.PositionID, item.Details))
		}
	}
	if len(out.Items) == 0 {
		out.Errors = append(out.Errors, "no successful products available for coverage")
	}
	out.Passed = len(out.Errors) == 0
	return out
}

func coverageItem(ctx context.Context, runner sandbox.Runner, result swarm.BranchResult) QualityItem {
	item := QualityItem{PositionID: result.PositionID, Product: result.ProductPath, Passed: false}
	kind := productKind(result)
	item.Type = kind
	reportDir := filepath.Join(result.Room, ".qsm_test")
	if manifestCmd, ok := manifestCommandByKind(result, "coverage"); ok {
		item.Command = manifestCmd.Cmd
	} else {
		switch kind {
		case "go":
			item.Command = []string{"go", "test", "./...", "-coverprofile=.qsm_test/coverage.out"}
		case "python":
			item.Command = []string{pythonExecutableForMain(), "-m", "pytest", "--cov", "-q"}
		case "node":
			item.Command = []string{"node", "--test", "--experimental-test-coverage"}
		default:
			item.Details = "coverage unsupported for product type " + kind + "; provide a manifest coverage command in a later sprint"
			return item
		}
	}
	if len(item.Command) == 0 || item.Command[0] == "" {
		item.Details = "coverage tool executable unavailable"
		return item
	}
	res := runner.Run(ctx, sandbox.Command{
		Name:       "coverage " + result.PositionID,
		Cmd:        item.Command,
		CWD:        result.ProductPath,
		Room:       result.Room,
		Timeout:    2 * time.Minute,
		StdoutPath: filepath.Join(reportDir, "coverage.stdout.log"),
		StderrPath: filepath.Join(reportDir, "coverage.stderr.log"),
	})
	item.ExitCode = res.ExitCode
	item.Passed = res.ExitCode == 0 && res.Error == ""
	item.Details = strings.TrimSpace(firstNonEmpty(res.Error, res.Stderr, res.Stdout))
	if item.Details == "" && item.Passed {
		item.Details = "coverage command passed"
	}
	return item
}

func manifestCommandByKind(result swarm.BranchResult, kind string) (tester.ManifestCommand, bool) {
	for _, path := range []string{filepath.Join(result.Room, "test_manifest.json"), filepath.Join(result.ProductPath, "test_manifest.json")} {
		var manifest tester.Manifest
		if readJSON(path, &manifest) != nil {
			continue
		}
		for _, cmd := range manifest.Commands {
			if cmd.Kind == kind && len(cmd.Cmd) > 0 {
				return cmd, true
			}
		}
	}
	return tester.ManifestCommand{}, false
}

func flakeCmd(args []string) {
	fs := flag.NewFlagSet("flake", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	backend := fs.String("sandbox", sandbox.BackendAuto, "sandbox backend: room, docker, or auto")
	runs := fs.Int("runs", 3, "number of repeated test runs")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	report := buildFlakeReport(*root, *backend, *runs)
	must(writeJSON(filepath.Join(*root, ".state", "flake_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "flake_report.md"), []byte(qualityMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(qualityMarkdown(report))
}

func buildFlakeReport(root, backend string, runs int) QualityReport {
	if runs < 2 {
		runs = 2
	}
	var runReport swarm.RunReport
	_ = readJSON(filepath.Join(root, ".state", "run_report.json"), &runReport)
	runner := sandbox.NewRunner(backend)
	out := QualityReport{Schema: "qsm.flake_report.v1", Kind: "flake", Passed: true, Sandbox: runner.Backend(), CreatedAt: time.Now().UTC()}
	for _, result := range runReport.Results {
		if result.TestReport == nil || result.ProductPath == "" {
			continue
		}
		for _, cmd := range result.TestReport.Commands {
			if cmd.Kind != "test" && cmd.Kind != "browser" {
				continue
			}
			item := QualityItem{PositionID: result.PositionID, Product: result.ProductPath, Type: productKind(result), Command: cmd.Cmd, Passed: true}
			for i := 0; i < runs; i++ {
				cwd := commandCWD(result.Room, cmd.CWD)
				res := runner.Run(context.Background(), sandbox.Command{
					Name:       fmt.Sprintf("flake %s %s run %d", result.PositionID, cmd.Name, i+1),
					Cmd:        cmd.Cmd,
					CWD:        cwd,
					Room:       result.Room,
					Timeout:    2 * time.Minute,
					StdoutPath: filepath.Join(result.Room, ".qsm_test", fmt.Sprintf("flake-%d.stdout.log", i+1)),
					StderrPath: filepath.Join(result.Room, ".qsm_test", fmt.Sprintf("flake-%d.stderr.log", i+1)),
				})
				item.Runs = append(item.Runs, res.ExitCode)
			}
			allPass, allFail := allEqual(item.Runs, 0), noneEqual(item.Runs, 0)
			item.Passed = allPass
			switch {
			case allPass:
				item.Details = "deterministic-pass"
			case allFail:
				item.Details = "deterministic-fail"
				out.Errors = append(out.Errors, fmt.Sprintf("%s %s deterministic failure", result.PositionID, cmd.Name))
			default:
				item.Details = "flaky"
				out.Errors = append(out.Errors, fmt.Sprintf("%s %s flaky results %v", result.PositionID, cmd.Name, item.Runs))
			}
			out.Items = append(out.Items, item)
		}
	}
	if len(out.Items) == 0 {
		out.Errors = append(out.Errors, "no test/browser commands available for flake detection")
	}
	out.Passed = len(out.Errors) == 0
	return out
}

func mutationCmd(args []string) {
	fs := flag.NewFlagSet("mutation", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	backend := fs.String("sandbox", sandbox.BackendAuto, "sandbox backend: room, docker, or auto")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	report := buildMutationReport(*root, *backend)
	must(writeJSON(filepath.Join(*root, ".state", "mutation_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "mutation_report.md"), []byte(qualityMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(qualityMarkdown(report))
}

func buildMutationReport(root, backend string) QualityReport {
	var runReport swarm.RunReport
	_ = readJSON(filepath.Join(root, ".state", "run_report.json"), &runReport)
	runner := sandbox.NewRunner(backend)
	out := QualityReport{Schema: "qsm.mutation_report.v1", Kind: "mutation", Passed: true, Sandbox: runner.Backend(), CreatedAt: time.Now().UTC()}
	workRoot := filepath.Join(root, ".state", "mutation_work")
	_ = os.RemoveAll(workRoot)
	for _, result := range runReport.Results {
		if result.ProductPath == "" || result.TestReport == nil || !result.TestReport.Passed {
			continue
		}
		target := filepath.Join(workRoot, result.PositionID, "product")
		item := QualityItem{PositionID: result.PositionID, Product: target, Type: productKind(result)}
		if err := copyDir(result.ProductPath, target); err != nil {
			item.Details = err.Error()
			out.Errors = append(out.Errors, result.PositionID+": "+item.Details)
			out.Items = append(out.Items, item)
			continue
		}
		mutated, mutation, err := applySimpleMutation(target)
		item.Mutation = mutation
		if err != nil || !mutated {
			item.Details = firstNonEmpty(errString(err), "no safe mutation operator matched")
			out.Errors = append(out.Errors, result.PositionID+": "+item.Details)
			out.Items = append(out.Items, item)
			continue
		}
		testCmd, ok := firstRequiredTestCommand(result)
		if !ok {
			item.Details = "no test/browser command available to catch mutation"
			out.Errors = append(out.Errors, result.PositionID+": "+item.Details)
			out.Items = append(out.Items, item)
			continue
		}
		cwd := commandCWD(filepath.Join(workRoot, result.PositionID), testCmd.CWD)
		res := runner.Run(context.Background(), sandbox.Command{
			Name:       "mutation " + result.PositionID,
			Cmd:        testCmd.Cmd,
			CWD:        cwd,
			Room:       filepath.Join(workRoot, result.PositionID),
			Timeout:    2 * time.Minute,
			StdoutPath: filepath.Join(workRoot, result.PositionID, "mutation.stdout.log"),
			StderrPath: filepath.Join(workRoot, result.PositionID, "mutation.stderr.log"),
		})
		item.Command = testCmd.Cmd
		item.ExitCode = res.ExitCode
		item.Passed = res.ExitCode != 0 || res.Error != ""
		if item.Passed {
			item.Details = "mutation caught by test command"
		} else {
			item.Details = "mutation survived"
			out.Errors = append(out.Errors, result.PositionID+": mutation survived")
		}
		out.Items = append(out.Items, item)
	}
	if len(out.Items) == 0 {
		out.Errors = append(out.Errors, "no eligible tested products available for mutation")
	}
	out.Passed = len(out.Errors) == 0
	return out
}

type CIReleaseReport struct {
	Schema          string    `json:"schema"`
	Passed          bool      `json:"passed"`
	CI              bool      `json:"ci"`
	Provider        string    `json:"provider,omitempty"`
	CommitSHA       string    `json:"commit_sha,omitempty"`
	Branch          string    `json:"branch,omitempty"`
	Dirty           bool      `json:"dirty"`
	LocalAllowed    bool      `json:"local_allowed"`
	LatestQAPassed  bool      `json:"latest_qa_passed"`
	LatestQAProfile string    `json:"latest_qa_profile,omitempty"`
	LatestQASandbox string    `json:"latest_qa_sandbox,omitempty"`
	Errors          []string  `json:"errors,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

func ciReleaseCmd(args []string) {
	fs := flag.NewFlagSet("ci-release", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	report := buildCIReleaseReport(*root)
	must(writeJSON(filepath.Join(*root, ".state", "ci_release_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "ci_release_report.md"), []byte(ciReleaseMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(ciReleaseMarkdown(report))
}

func buildCIReleaseReport(root string) CIReleaseReport {
	out := CIReleaseReport{Schema: "qsm.ci_release_report.v1", CreatedAt: time.Now().UTC(), LocalAllowed: envBool("QSM_ALLOW_LOCAL_RELEASE_EVIDENCE")}
	out.CI = envBool("CI") || os.Getenv("GITHUB_ACTIONS") == "true"
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		out.Provider = "github-actions"
	}
	out.CommitSHA = gitOutput(root, "rev-parse", "HEAD")
	out.Branch = gitOutput(root, "rev-parse", "--abbrev-ref", "HEAD")
	out.Dirty = strings.TrimSpace(gitOutput(root, "status", "--porcelain")) != ""
	var qa QAReport
	if readJSON(filepath.Join(root, ".state", "qa_report.json"), &qa) == nil {
		out.LatestQAPassed = qa.Passed
		out.LatestQAProfile = qa.Profile
		out.LatestQASandbox = qa.SandboxBackend
		if qa.Profile != "production" {
			out.Errors = append(out.Errors, "latest QA profile is not production")
		}
	} else {
		out.Errors = append(out.Errors, "missing .state/qa_report.json")
	}
	if !out.CI && !out.LocalAllowed {
		out.Errors = append(out.Errors, "release evidence is local; set QSM_ALLOW_LOCAL_RELEASE_EVIDENCE=1 only for local-dev waivers")
	}
	out.Passed = len(out.Errors) == 0
	return out
}

func buildCIReleaseReportFromQA(root string, qa QAReport) CIReleaseReport {
	out := CIReleaseReport{Schema: "qsm.ci_release_report.v1", CreatedAt: time.Now().UTC(), LocalAllowed: envBool("QSM_ALLOW_LOCAL_RELEASE_EVIDENCE")}
	out.CI = envBool("CI") || os.Getenv("GITHUB_ACTIONS") == "true"
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		out.Provider = "github-actions"
	}
	out.CommitSHA = gitOutput(root, "rev-parse", "HEAD")
	out.Branch = gitOutput(root, "rev-parse", "--abbrev-ref", "HEAD")
	out.Dirty = strings.TrimSpace(gitOutput(root, "status", "--porcelain")) != ""
	out.LatestQAPassed = qa.Passed
	out.LatestQAProfile = qa.Profile
	out.LatestQASandbox = qa.SandboxBackend
	if qaProfileRank(qa.Profile) < qaProfileRank("production") {
		out.Errors = append(out.Errors, "latest QA profile is not production-grade")
	}
	if !out.CI {
		switch normalizeQAProfile(qa.Profile) {
		case "local-production":
			if !out.LocalAllowed {
				out.Errors = append(out.Errors, "local-production requires QSM_ALLOW_LOCAL_RELEASE_EVIDENCE=1")
			}
		default:
			out.Errors = append(out.Errors, "production release evidence must be generated in CI; local-production is the local waiver profile")
		}
	}
	if normalizeQAProfile(qa.Profile) == "production" && out.LocalAllowed {
		out.Errors = append(out.Errors, "production profile must not use QSM_ALLOW_LOCAL_RELEASE_EVIDENCE")
	}
	out.Passed = len(out.Errors) == 0
	return out
}

func ciReleaseMarkdown(report CIReleaseReport) string {
	var b strings.Builder
	b.WriteString("# QSM CI Release Report\n\n")
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	fmt.Fprintf(&b, "- CI: `%v` provider=`%s`\n", report.CI, report.Provider)
	fmt.Fprintf(&b, "- Commit: `%s`\n", report.CommitSHA)
	fmt.Fprintf(&b, "- Branch: `%s`\n", report.Branch)
	fmt.Fprintf(&b, "- Dirty: `%v`\n", report.Dirty)
	fmt.Fprintf(&b, "- Latest QA: profile=`%s` sandbox=`%s` passed=`%v`\n", report.LatestQAProfile, report.LatestQASandbox, report.LatestQAPassed)
	if len(report.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, err := range report.Errors {
			b.WriteString("- " + err + "\n")
		}
	}
	return b.String()
}

type CIBootstrapReport struct {
	Schema       string    `json:"schema"`
	Provider     string    `json:"provider"`
	RepoRoot     string    `json:"repo_root,omitempty"`
	WorkDir      string    `json:"work_dir,omitempty"`
	WorkflowPath string    `json:"workflow_path"`
	Dockerfile   string    `json:"dockerfile"`
	Passed       bool      `json:"passed"`
	Errors       []string  `json:"errors,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func ciBootstrapCmd(args []string) {
	fs := flag.NewFlagSet("ci-bootstrap", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	provider := fs.String("provider", "github", "CI provider: github")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	report := bootstrapCI(*root, *provider)
	must(writeJSON(filepath.Join(*root, ".state", "ci_bootstrap_report.json"), report))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	if report.Passed {
		fmt.Printf("CI bootstrap wrote %s\n", report.WorkflowPath)
	} else {
		fmt.Printf("CI bootstrap failed: %s\n", strings.Join(report.Errors, "; "))
		os.Exit(1)
	}
}

func bootstrapCI(root, provider string) CIBootstrapReport {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}
	report := CIBootstrapReport{
		Schema:     "qsm.ci_bootstrap_report.v1",
		Provider:   strings.ToLower(strings.TrimSpace(provider)),
		Dockerfile: filepath.Join(rootAbs, "deploy", "qsm-omni-sandbox.Dockerfile"),
		CreatedAt:  time.Now().UTC(),
	}
	if report.Provider != "github" {
		report.Errors = append(report.Errors, "only github provider is supported in v1")
		return report
	}
	repoRoot := gitTopLevel(rootAbs)
	if repoRoot == "" {
		repoRoot = rootAbs
	}
	report.RepoRoot = repoRoot
	workDir, err := filepath.Rel(repoRoot, rootAbs)
	if err != nil || workDir == "." {
		workDir = "."
	}
	report.WorkDir = filepath.ToSlash(workDir)
	report.WorkflowPath = filepath.Join(repoRoot, ".github", "workflows", "qsm-production-qa.yml")
	if err := os.MkdirAll(filepath.Dir(report.WorkflowPath), 0755); err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	if err := os.WriteFile(report.WorkflowPath, []byte(qsmProductionQAWorkflow(report.WorkDir)), 0644); err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	report.Passed = true
	return report
}

func gitTopLevel(root string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = root
	data, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func qsmProductionQAWorkflow(workDir string) string {
	if strings.TrimSpace(workDir) == "" {
		workDir = "."
	}
	workDir = filepath.ToSlash(workDir)
	prefix := ""
	if workDir != "." {
		prefix = workDir + "/"
	}
	return fmt.Sprintf(`name: QSM Production QA

on:
  push:
    branches: [ main, master, dev ]
  pull_request:
  workflow_dispatch:

jobs:
  production-qa:
    runs-on: ubuntu-latest
    timeout-minutes: 90
    env:
      QSM_ALLOW_LOCAL_RELEASE_EVIDENCE: "0"
      QSM_SANDBOX_DOCKER_IMAGE: qsm-omni-sandbox:local
      QSM_SHARED_CACHE: "1"
      FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: "true"
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: false
      - uses: actions/setup-node@v4
        with:
          node-version: "22"
      - uses: actions/setup-python@v5
        with:
          python-version: "3.12"
      - name: Build QSM
        working-directory: %s
        run: |
          go test ./...
          go build -o qsm ./cmd/qsm
      - name: Build omni sandbox image
        working-directory: %s
        run: docker build -t qsm-omni-sandbox:local -f deploy/qsm-omni-sandbox.Dockerfile .
      - name: Probe sandbox
        working-directory: %s
        run: ./qsm sandbox -probe -backend docker -image qsm-omni-sandbox:local -profile omni
      - name: Stress and recovery evidence
        working-directory: %s
        run: |
          ./qsm stress -sandbox docker -nodes 12 -parallel 6 -large-repo-files 1000
          ./qsm recovery -sandbox docker
          ./qsm contributor-smoke
      - name: Run omni benchmark
        working-directory: %s
        run: |
          ./qsm benchmark -suite omni-contract -sandbox docker -image qsm-omni-sandbox:local -positions 2 -parallel 2 || {
            python3 scripts/emit_benchmark_annotations.py .state/benchmark_report.json || true
            echo "::group::benchmark_report"
            cat .state/benchmark_report.md || true
            echo "::endgroup::"
            exit 1
          }
      - name: Run self improvement
        working-directory: %s
        run: |
          ./qsm self-improve -suite omni-contract -cycles 3 -sandbox docker -image qsm-omni-sandbox:local || {
            python3 scripts/emit_self_improve_annotations.py .state/self_improvement_report.json || true
            echo "::group::self_improvement_report"
            cat .state/self_improvement_report.md || true
            echo "::endgroup::"
            echo "::group::cycle benchmark reports"
            find .benchmarks -path '*/.state/benchmark_report.json' -print -exec cat {} \; || true
            echo "::endgroup::"
            exit 1
          }
      - name: Autonomy evidence
        working-directory: %s
        run: |
          ./qsm autorun-plist \
            -request "product_kind=cli-tool. Build an autonomous QA smoke product with tests and cache/wiki citations." \
            -harness simulated \
            -positions 2 \
            -parallel 2 \
            -route-health=false
          ./qsm autorun \
            -request "product_kind=cli-tool. Build an autonomous QA smoke product with tests and cache/wiki citations." \
            -harness simulated \
            -positions 2 \
            -parallel 2 \
            -max-cycles 1 \
            -interval 1s \
            -cycle-timeout 10m \
            -route-health=false \
            -deploy-router=false \
            -shared-cache=true
      - name: Production root run
        working-directory: %s
        run: |
          ./qsm run \
            -request "product_kind=node-fullstack. Build a production QA root product with tests, manifest, cache/wiki citations, traceable execution, and delivery evidence." \
            -harness simulated \
            -positions 7 \
            -parallel 4 \
            -sandbox docker \
            -shared-cache=true \
            -retries 1
      - name: Local evidence reports
        working-directory: %s
        run: |
          ./qsm production-gap || true
          ./qsm ops-readiness
          ./qsm compliance -sandbox docker -image qsm-omni-sandbox:local
      - name: Production QA
        working-directory: %s
        run: |
          ./qsm qa -profile production -sandbox docker -image qsm-omni-sandbox:local -refresh=true || {
            python3 scripts/emit_qa_annotations.py .state/qa_report.json || true
            echo "::group::qa_report"
            cat .state/qa_report.md || true
            echo "::endgroup::"
            echo "::group::production_gap"
            cat .state/production_gap_report.md || true
            echo "::endgroup::"
            exit 1
          }
      - name: Omni alpha QA
        working-directory: %s
        run: |
          ./qsm qa -profile omni-alpha -sandbox docker -image qsm-omni-sandbox:local -refresh=true || {
            python3 scripts/emit_qa_annotations.py .state/qa_report.json || true
            echo "::group::omni_alpha_qa_report"
            cat .state/qa_report.md || true
            echo "::endgroup::"
            exit 1
          }
      - name: Production gap after Omni alpha
        working-directory: %s
        if: always()
        run: ./qsm production-gap
      - name: Upload QSM evidence
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: qsm-production-evidence
          include-hidden-files: true
          path: |
            %s.state/**
            %s.benchmarks/**
          retention-days: 14
`, workDir, workDir, workDir, workDir, workDir, workDir, workDir, workDir, workDir, workDir, workDir, workDir, prefix, prefix)
}

type StressReport struct {
	Schema              string       `json:"schema"`
	Root                string       `json:"root"`
	Passed              bool         `json:"passed"`
	Backend             string       `json:"backend"`
	Nodes               int          `json:"nodes"`
	Parallel            int          `json:"parallel"`
	DurationMS          int64        `json:"duration_ms"`
	Items               []StressItem `json:"items,omitempty"`
	LargeRepoFiles      int          `json:"large_repo_files,omitempty"`
	LargeRepoPassed     bool         `json:"large_repo_passed,omitempty"`
	LargeRepoDurationMS int64        `json:"large_repo_duration_ms,omitempty"`
	Errors              []string     `json:"errors,omitempty"`
	CreatedAt           time.Time    `json:"created_at"`
}

type StressItem struct {
	ID         string `json:"id"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

func stressCmd(args []string) {
	fs := flag.NewFlagSet("stress", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	backend := fs.String("sandbox", sandbox.BackendAuto, "sandbox backend: room, docker, or auto")
	nodes := fs.Int("nodes", 7, "number of concurrent command probes")
	parallel := fs.Int("parallel", 4, "max concurrent probes")
	largeRepoFiles := fs.Int("large-repo-files", 0, "optional synthetic large-repo file count to create and verify inside the sandbox")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	report := buildStressReport(*root, *backend, *nodes, *parallel, *largeRepoFiles)
	must(writeJSON(filepath.Join(*root, ".state", "stress_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "stress_report.md"), []byte(stressMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(stressMarkdown(report))
}

func buildStressReport(root, backend string, nodes, parallel, largeRepoFiles int) StressReport {
	if nodes <= 0 {
		nodes = 1
	}
	if parallel <= 0 || parallel > nodes {
		parallel = nodes
	}
	rootAbs, _ := filepath.Abs(root)
	runner := sandbox.NewRunner(backend)
	out := StressReport{Schema: "qsm.stress_report.v1", Root: rootAbs, Passed: true, Backend: runner.Backend(), Nodes: nodes, Parallel: parallel, CreatedAt: time.Now().UTC()}
	start := time.Now()
	workRoot := filepath.Join(rootAbs, ".state", "stress_probe")
	_ = os.RemoveAll(workRoot)
	_ = os.MkdirAll(workRoot, 0755)
	items := make([]StressItem, nodes)
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i := 0; i < nodes; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			id := fmt.Sprintf("stress-%02d", i+1)
			room := filepath.Join(workRoot, id)
			_ = os.MkdirAll(room, 0755)
			res := runner.Run(context.Background(), sandbox.Command{
				Name:       id,
				Cmd:        []string{"node", "-e", `console.log("qsm-stress-ok")`},
				CWD:        room,
				Room:       room,
				Timeout:    45 * time.Second,
				StdoutPath: filepath.Join(room, "stdout.log"),
				StderrPath: filepath.Join(room, "stderr.log"),
			})
			items[i] = StressItem{ID: id, ExitCode: res.ExitCode, DurationMS: res.DurationMS, Error: strings.TrimSpace(res.Error)}
		}()
	}
	wg.Wait()
	out.DurationMS = time.Since(start).Milliseconds()
	out.Items = items
	for _, item := range items {
		if item.ExitCode != 0 || item.Error != "" {
			out.Errors = append(out.Errors, fmt.Sprintf("%s exit=%d error=%s", item.ID, item.ExitCode, item.Error))
		}
	}
	if largeRepoFiles > 0 {
		passed, duration, err := runLargeRepoStressProbe(context.Background(), runner, rootAbs, largeRepoFiles)
		out.LargeRepoFiles = largeRepoFiles
		out.LargeRepoPassed = passed
		out.LargeRepoDurationMS = duration
		if err != "" {
			out.Errors = append(out.Errors, err)
		}
	}
	out.Passed = len(out.Errors) == 0 && len(out.Items) == nodes
	return out
}

func runLargeRepoStressProbe(ctx context.Context, runner sandbox.Runner, root string, files int) (bool, int64, string) {
	if files < 1 {
		return true, 0, ""
	}
	room := filepath.Join(root, ".state", "stress_large_repo")
	repo := filepath.Join(room, "repo")
	_ = os.RemoveAll(room)
	if err := os.MkdirAll(repo, 0755); err != nil {
		return false, 0, "large-repo setup failed: " + err.Error()
	}
	for i := 0; i < files; i++ {
		dir := filepath.Join(repo, fmt.Sprintf("pkg%03d", i/100))
		if err := os.MkdirAll(dir, 0755); err != nil {
			return false, 0, "large-repo dir setup failed: " + err.Error()
		}
		body := fmt.Sprintf("module.exports.value%d = %d;\n", i, i)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%04d.js", i)), []byte(body), 0644); err != nil {
			return false, 0, "large-repo file setup failed: " + err.Error()
		}
	}
	script := `const fs=require("fs"); const path=require("path"); const root=process.argv[1]; let n=0; function walk(d){ for (const ent of fs.readdirSync(d,{withFileTypes:true})) { const p=path.join(d, ent.name); if (ent.isDirectory()) walk(p); else if (ent.name.endsWith(".js")) { const s=fs.readFileSync(p,"utf8"); if (!s.includes("module.exports")) throw new Error("bad file "+p); n++; } } } walk(root); console.log(n); if (n !== Number(process.env.QSM_EXPECT_FILES)) process.exit(2);`
	start := time.Now()
	res := runner.Run(ctx, sandbox.Command{
		Name:       "large repo stress",
		Cmd:        []string{"node", "-e", script, repo},
		CWD:        room,
		Room:       room,
		Timeout:    2 * time.Minute,
		Env:        []string{"QSM_EXPECT_FILES=" + strconv.Itoa(files)},
		StdoutPath: filepath.Join(room, "large-repo.stdout.log"),
		StderrPath: filepath.Join(room, "large-repo.stderr.log"),
	})
	duration := time.Since(start).Milliseconds()
	if res.ExitCode != 0 || res.Error != "" {
		return false, duration, fmt.Sprintf("large-repo probe failed exit=%d error=%s stderr=%s", res.ExitCode, res.Error, strings.TrimSpace(res.Stderr))
	}
	return true, duration, ""
}

func stressMarkdown(report StressReport) string {
	var b strings.Builder
	b.WriteString("# QSM Stress Report\n\n")
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	fmt.Fprintf(&b, "- Backend: `%s`\n", report.Backend)
	fmt.Fprintf(&b, "- Nodes: `%d` parallel=`%d`\n", report.Nodes, report.Parallel)
	fmt.Fprintf(&b, "- Duration: `%dms`\n", report.DurationMS)
	if report.LargeRepoFiles > 0 {
		fmt.Fprintf(&b, "- Large repo: files=`%d` passed=`%v` duration=`%dms`\n", report.LargeRepoFiles, report.LargeRepoPassed, report.LargeRepoDurationMS)
	}
	if len(report.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, err := range report.Errors {
			b.WriteString("- " + err + "\n")
		}
	}
	return b.String()
}

type RecoveryReport struct {
	Schema          string    `json:"schema"`
	Root            string    `json:"root"`
	Passed          bool      `json:"passed"`
	Backend         string    `json:"backend"`
	FailureCaptured bool      `json:"failure_captured"`
	RecoveryPassed  bool      `json:"recovery_passed"`
	RecoveryRate    float64   `json:"recovery_rate"`
	Errors          []string  `json:"errors,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

func recoveryCmd(args []string) {
	fs := flag.NewFlagSet("recovery", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	backend := fs.String("sandbox", sandbox.BackendAuto, "sandbox backend: room, docker, or auto")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	report := buildRecoveryReport(*root, *backend)
	must(writeJSON(filepath.Join(*root, ".state", "recovery_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "recovery_report.md"), []byte(recoveryMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(recoveryMarkdown(report))
}

func buildRecoveryReport(root, backend string) RecoveryReport {
	rootAbs, _ := filepath.Abs(root)
	runner := sandbox.NewRunner(backend)
	room := filepath.Join(rootAbs, ".state", "recovery_probe")
	_ = os.RemoveAll(room)
	_ = os.MkdirAll(room, 0755)
	out := RecoveryReport{Schema: "qsm.recovery_report.v1", Root: rootAbs, Backend: runner.Backend(), CreatedAt: time.Now().UTC()}
	fail := runner.Run(context.Background(), sandbox.Command{
		Name:       "recovery expected failure",
		Cmd:        []string{"node", "-e", "process.exit(7)"},
		CWD:        room,
		Room:       room,
		Timeout:    30 * time.Second,
		StdoutPath: filepath.Join(room, "failure.stdout.log"),
		StderrPath: filepath.Join(room, "failure.stderr.log"),
	})
	out.FailureCaptured = fail.ExitCode != 0 || fail.Error != ""
	pass := runner.Run(context.Background(), sandbox.Command{
		Name:       "recovery success after failure",
		Cmd:        []string{"node", "-e", "process.exit(0)"},
		CWD:        room,
		Room:       room,
		Timeout:    30 * time.Second,
		StdoutPath: filepath.Join(room, "recovery.stdout.log"),
		StderrPath: filepath.Join(room, "recovery.stderr.log"),
	})
	out.RecoveryPassed = pass.ExitCode == 0 && pass.Error == ""
	if out.FailureCaptured && out.RecoveryPassed {
		out.RecoveryRate = 1
	}
	if !out.FailureCaptured {
		out.Errors = append(out.Errors, "expected failure was not captured")
	}
	if !out.RecoveryPassed {
		out.Errors = append(out.Errors, fmt.Sprintf("recovery command failed exit=%d error=%s", pass.ExitCode, pass.Error))
	}
	out.Passed = len(out.Errors) == 0
	return out
}

func recoveryMarkdown(report RecoveryReport) string {
	var b strings.Builder
	b.WriteString("# QSM Recovery Report\n\n")
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	fmt.Fprintf(&b, "- Backend: `%s`\n", report.Backend)
	fmt.Fprintf(&b, "- Failure captured: `%v`\n", report.FailureCaptured)
	fmt.Fprintf(&b, "- Recovery passed: `%v`\n", report.RecoveryPassed)
	fmt.Fprintf(&b, "- Recovery rate: `%.0f%%`\n", report.RecoveryRate*100)
	if len(report.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, err := range report.Errors {
			b.WriteString("- " + err + "\n")
		}
	}
	return b.String()
}

type ContributorSmokeReport struct {
	Schema    string                  `json:"schema"`
	Root      string                  `json:"root"`
	Passed    bool                    `json:"passed"`
	Checks    []ContributorSmokeCheck `json:"checks,omitempty"`
	Errors    []string                `json:"errors,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
}

type ContributorSmokeCheck struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	ExitCode int    `json:"exit_code,omitempty"`
	Details  string `json:"details,omitempty"`
}

type OpsReadinessReport struct {
	Schema                 string                  `json:"schema"`
	Root                   string                  `json:"root"`
	Passed                 bool                    `json:"passed"`
	CIWorkflowPresent      bool                    `json:"ci_workflow_present"`
	CIArtifactRetention    bool                    `json:"ci_artifact_retention"`
	LaunchdPlistPresent    bool                    `json:"launchd_plist_present"`
	AutorunStatePresent    bool                    `json:"autorun_state_present"`
	RunbookPresent         bool                    `json:"runbook_present"`
	ApprovalGateDocumented bool                    `json:"approval_gate_documented"`
	ProductionGapPresent   bool                    `json:"production_gap_present"`
	ProductionGapTruthful  bool                    `json:"production_gap_truthful"`
	Checks                 []ContributorSmokeCheck `json:"checks,omitempty"`
	Errors                 []string                `json:"errors,omitempty"`
	CreatedAt              time.Time               `json:"created_at"`
}

type ComplianceReport struct {
	Schema              string          `json:"schema"`
	Root                string          `json:"root"`
	Passed              bool            `json:"passed"`
	SandboxPolicyPassed bool            `json:"sandbox_policy_passed"`
	SBOMGenerated       bool            `json:"sbom_generated"`
	GoModules           []string        `json:"go_modules,omitempty"`
	FilesInventoried    int             `json:"files_inventoried"`
	LicenseFilePresent  bool            `json:"license_file_present"`
	Checks              map[string]bool `json:"checks,omitempty"`
	Warnings            []string        `json:"warnings,omitempty"`
	Errors              []string        `json:"errors,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
}

func complianceCmd(args []string) {
	fs := flag.NewFlagSet("compliance", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	sandboxBackend := fs.String("sandbox", sandbox.BackendAuto, "sandbox backend used when refreshing missing sandbox evidence")
	image := fs.String("image", "", "Docker image used when refreshing missing sandbox evidence")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	if strings.TrimSpace(*image) != "" {
		_ = os.Setenv("QSM_SANDBOX_DOCKER_IMAGE", strings.TrimSpace(*image))
	}
	rootAbs, err := filepath.Abs(*root)
	must(err)
	if !fileExists(filepath.Join(rootAbs, ".state", "sandbox_report.json")) {
		_ = sandbox.Write(rootAbs, sandbox.InspectWithProbe(rootAbs, sandbox.NormalizeBackend(*sandboxBackend)))
	}
	report := buildComplianceReport(*root)
	must(writeJSON(filepath.Join(*root, ".state", "compliance_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "compliance_report.md"), []byte(complianceMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(complianceMarkdown(report))
	if !report.Passed {
		os.Exit(1)
	}
}

func buildComplianceReport(root string) ComplianceReport {
	rootAbs, _ := filepath.Abs(root)
	out := ComplianceReport{Schema: "qsm.compliance_report.v1", Root: rootAbs, Checks: map[string]bool{}, CreatedAt: time.Now().UTC()}
	var sandboxReport sandbox.Report
	if readJSON(filepath.Join(rootAbs, ".state", "sandbox_report.json"), &sandboxReport) == nil && sandboxReport.Schema != "" {
		out.Checks["hard_sandbox_ready"] = sandboxReport.HardSandboxReady
		out.Checks["network_none"] = sandboxReport.Policy.Network == "none"
		out.Checks["non_root_user"] = sandboxReport.Policy.User != "" && sandboxReport.Policy.User != "0" && sandboxReport.Policy.User != "0:0"
		out.Checks["drop_caps"] = sandboxReport.Policy.DropCaps
		out.Checks["read_only_root"] = sandboxReport.Policy.ReadOnly
		out.Checks["probe_valid"] = sandboxReport.Probe.Valid && sandboxReport.Probe.Passed
		out.SandboxPolicyPassed = true
		for _, key := range []string{"hard_sandbox_ready", "network_none", "non_root_user", "drop_caps", "read_only_root", "probe_valid"} {
			if !out.Checks[key] {
				out.SandboxPolicyPassed = false
			}
		}
	} else {
		out.Errors = append(out.Errors, "missing sandbox_report.json")
	}
	out.GoModules = goModuleInventory(rootAbs)
	out.FilesInventoried = sourceInventoryCount(rootAbs)
	out.SBOMGenerated = len(out.GoModules) > 0 && out.FilesInventoried > 0
	if !out.SBOMGenerated {
		out.Errors = append(out.Errors, "failed to generate local module/file inventory")
	}
	out.LicenseFilePresent = anyFileExists(rootAbs, "LICENSE", "LICENSE.md", "LICENSE.txt", "COPYING")
	if !out.LicenseFilePresent {
		out.Warnings = append(out.Warnings, "project license file is not present; local compliance inventory passes but external release should choose a license explicitly")
	}
	out.Passed = out.SandboxPolicyPassed && out.SBOMGenerated && len(out.Errors) == 0
	return out
}

func goModuleInventory(root string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "all")
	cmd.Dir = root
	data, err := cmd.Output()
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func sourceInventoryCount(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".lake", ".rooms", ".state", ".benchmarks", "node_modules", "__pycache__":
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".py", ".js", ".ts", ".json", ".md", ".yml", ".yaml", ".Dockerfile":
			count++
		default:
			if strings.HasSuffix(path, "Dockerfile") {
				count++
			}
		}
		return nil
	})
	return count
}

func anyFileExists(root string, names ...string) bool {
	for _, name := range names {
		if fileExists(filepath.Join(root, name)) {
			return true
		}
	}
	return false
}

func complianceMarkdown(report ComplianceReport) string {
	var b strings.Builder
	b.WriteString("# QSM Compliance Report\n\n")
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	fmt.Fprintf(&b, "- Sandbox policy: `%v`\n", report.SandboxPolicyPassed)
	fmt.Fprintf(&b, "- SBOM generated: `%v`\n", report.SBOMGenerated)
	fmt.Fprintf(&b, "- Go modules: `%d`\n", len(report.GoModules))
	fmt.Fprintf(&b, "- Files inventoried: `%d`\n", report.FilesInventoried)
	fmt.Fprintf(&b, "- License file present: `%v`\n", report.LicenseFilePresent)
	if len(report.Checks) > 0 {
		b.WriteString("\n## Checks\n\n")
		keys := make([]string, 0, len(report.Checks))
		for key := range report.Checks {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&b, "- `%s`: `%v`\n", key, report.Checks[key])
		}
	}
	if len(report.Warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, warning := range report.Warnings {
			b.WriteString("- " + warning + "\n")
		}
	}
	if len(report.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, err := range report.Errors {
			b.WriteString("- " + err + "\n")
		}
	}
	return b.String()
}

func opsReadinessCmd(args []string) {
	fs := flag.NewFlagSet("ops-readiness", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	report := buildOpsReadinessReport(*root)
	must(writeJSON(filepath.Join(*root, ".state", "ops_readiness_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "ops_readiness_report.md"), []byte(opsReadinessMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(opsReadinessMarkdown(report))
	if !report.Passed {
		os.Exit(1)
	}
}

func buildOpsReadinessReport(root string) OpsReadinessReport {
	rootAbs, _ := filepath.Abs(root)
	workflow := filepath.Join(rootAbs, ".github", "workflows", "qsm-production-qa.yml")
	runbook := filepath.Join(rootAbs, "docs", "OPERATIONAL_RUNBOOK_2026-05-07.md")
	out := OpsReadinessReport{Schema: "qsm.ops_readiness_report.v1", Root: rootAbs, CreatedAt: time.Now().UTC()}
	add := func(check ContributorSmokeCheck) {
		out.Checks = append(out.Checks, check)
		if !check.Passed {
			out.Errors = append(out.Errors, check.Name+": "+check.Details)
		}
	}
	add(fileCheck("ci-workflow", workflow))
	add(fileContainsCheck("ci-artifact-retention", workflow, "retention-days:"))
	add(fileContainsCheck("ci-production-qa", workflow, "qa -profile production"))
	add(fileContainsCheck("ci-omni-alpha-qa", workflow, "qa -profile omni-alpha"))
	add(fileContainsCheck("ci-omni-image", workflow, "qsm-omni-sandbox:local"))
	add(fileCheck("launchd-plist", filepath.Join(rootAbs, ".state", "qsm.autorun.plist")))
	add(fileCheck("autorun-state", filepath.Join(rootAbs, ".state", "autorun.json")))
	add(fileCheck("operational-runbook", runbook))
	add(fileContainsCheck("runbook-approval-gates", runbook, "approval gate"))
	add(fileContainsCheck("runbook-disaster-recovery", runbook, "RPO"))
	add(fileCheck("production-gap-report", filepath.Join(rootAbs, ".state", "production_gap_report.json")))

	var gap ProductionGapReport
	if readJSON(filepath.Join(rootAbs, ".state", "production_gap_report.json"), &gap) == nil && gap.Schema != "" {
		out.ProductionGapTruthful = !gap.ProductionReady || !gap.TopTierReady || len(gap.FailedGates) == 0
		if !out.ProductionGapTruthful {
			out.Errors = append(out.Errors, "production-gap-report: external readiness cannot be true while top-tier evidence is missing")
		}
	}
	out.CIWorkflowPresent = fileExists(workflow)
	out.CIArtifactRetention = containsFile(workflow, "retention-days:")
	out.LaunchdPlistPresent = fileExists(filepath.Join(rootAbs, ".state", "qsm.autorun.plist"))
	out.AutorunStatePresent = fileExists(filepath.Join(rootAbs, ".state", "autorun.json"))
	out.RunbookPresent = fileExists(runbook)
	out.ApprovalGateDocumented = containsFile(runbook, "approval gate")
	out.ProductionGapPresent = fileExists(filepath.Join(rootAbs, ".state", "production_gap_report.json"))
	out.Passed = len(out.Errors) == 0
	return out
}

func opsReadinessMarkdown(report OpsReadinessReport) string {
	var b strings.Builder
	b.WriteString("# QSM Operational Readiness Report\n\n")
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	fmt.Fprintf(&b, "- CI workflow: `%v`\n", report.CIWorkflowPresent)
	fmt.Fprintf(&b, "- CI artifact retention: `%v`\n", report.CIArtifactRetention)
	fmt.Fprintf(&b, "- Launchd plist: `%v`\n", report.LaunchdPlistPresent)
	fmt.Fprintf(&b, "- Autorun state: `%v`\n", report.AutorunStatePresent)
	fmt.Fprintf(&b, "- Runbook: `%v`\n", report.RunbookPresent)
	fmt.Fprintf(&b, "- Approval gates documented: `%v`\n", report.ApprovalGateDocumented)
	fmt.Fprintf(&b, "- Production gap present/truthful: `%v/%v`\n", report.ProductionGapPresent, report.ProductionGapTruthful)
	if len(report.Checks) > 0 {
		b.WriteString("\n## Checks\n\n")
		for _, check := range report.Checks {
			fmt.Fprintf(&b, "- `%s`: passed=`%v` %s\n", check.Name, check.Passed, check.Details)
		}
	}
	if len(report.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, err := range report.Errors {
			b.WriteString("- " + err + "\n")
		}
	}
	return b.String()
}

func contributorSmokeCmd(args []string) {
	fs := flag.NewFlagSet("contributor-smoke", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	report := buildContributorSmokeReport(*root)
	must(writeJSON(filepath.Join(*root, ".state", "contributor_smoke_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "contributor_smoke_report.md"), []byte(contributorSmokeMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(contributorSmokeMarkdown(report))
}

func buildContributorSmokeReport(root string) ContributorSmokeReport {
	rootAbs, _ := filepath.Abs(root)
	out := ContributorSmokeReport{Schema: "qsm.contributor_smoke_report.v1", Root: rootAbs, Passed: true, CreatedAt: time.Now().UTC()}
	addCheck := func(check ContributorSmokeCheck) {
		out.Checks = append(out.Checks, check)
		if !check.Passed {
			out.Errors = append(out.Errors, check.Name+": "+check.Details)
		}
	}
	addCheck(runContributorCommand(rootAbs, "go-test", []string{"go", "test", "./..."}, 2*time.Minute))
	addCheck(runContributorCommand(rootAbs, "go-build", []string{"go", "build", "-o", filepath.Join(rootAbs, ".state", "qsm-contributor-smoke"), "./cmd/qsm"}, 2*time.Minute))
	addCheck(fileCheck("readme", filepath.Join(rootAbs, "README.md")))
	addCheck(fileCheck("production-ci-workflow", filepath.Join(rootAbs, ".github", "workflows", "qsm-production-qa.yml")))
	addCheck(fileCheck("sandbox-dockerfile", filepath.Join(rootAbs, "docker", "qsm-sandbox", "Dockerfile")))
	addCheck(fileCheck("omni-sandbox-dockerfile", filepath.Join(rootAbs, "deploy", "qsm-omni-sandbox.Dockerfile")))
	addCheck(fileContainsCheck("ci-artifact-retention", filepath.Join(rootAbs, ".github", "workflows", "qsm-production-qa.yml"), "retention-days:"))
	out.Passed = len(out.Errors) == 0
	return out
}

func runContributorCommand(root, name string, argv []string, timeout time.Duration) ContributorSmokeCheck {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	check := ContributorSmokeCheck{Name: name, Passed: err == nil, Details: truncateStatusError(strings.TrimSpace(string(out)), 220)}
	if ctx.Err() == context.DeadlineExceeded {
		check.Details = "timed out"
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			check.ExitCode = exitErr.ExitCode()
		} else {
			check.ExitCode = -1
			if check.Details == "" {
				check.Details = err.Error()
			}
		}
	}
	return check
}

func fileCheck(name, path string) ContributorSmokeCheck {
	info, err := os.Stat(path)
	if err != nil {
		return ContributorSmokeCheck{Name: name, Passed: false, Details: err.Error()}
	}
	return ContributorSmokeCheck{Name: name, Passed: !info.IsDir(), Details: path}
}

func fileContainsCheck(name, path, token string) ContributorSmokeCheck {
	data, err := os.ReadFile(path)
	if err != nil {
		return ContributorSmokeCheck{Name: name, Passed: false, Details: err.Error()}
	}
	passed := strings.Contains(string(data), token)
	details := "found " + token
	if !passed {
		details = "missing " + token
	}
	return ContributorSmokeCheck{Name: name, Passed: passed, Details: details}
}

func containsFile(path, token string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), token)
}

func resultFailureSummary(result swarm.BranchResult) string {
	if strings.TrimSpace(result.Error) != "" {
		return result.Error
	}
	if result.TestReport != nil && len(result.TestReport.Errors) > 0 {
		return strings.Join(result.TestReport.Errors, "; ")
	}
	if result.Verification != nil && len(result.Verification.Errors) > 0 {
		return strings.Join(result.Verification.Errors, "; ")
	}
	return fmt.Sprintf("build=%v test=%v lint=%v", result.BuildPassed, result.TestPassed, result.LintPassed)
}

func contributorSmokeMarkdown(report ContributorSmokeReport) string {
	var b strings.Builder
	b.WriteString("# QSM Contributor Smoke Report\n\n")
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	b.WriteString("\n## Checks\n\n")
	for _, check := range report.Checks {
		fmt.Fprintf(&b, "- `%s`: passed=`%v` exit=`%d` %s\n", check.Name, check.Passed, check.ExitCode, check.Details)
	}
	if len(report.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, err := range report.Errors {
			b.WriteString("- " + err + "\n")
		}
	}
	return b.String()
}

type BenchmarkReport struct {
	Schema        string          `json:"schema"`
	Suite         string          `json:"suite"`
	Style         string          `json:"style"`
	Root          string          `json:"root"`
	HarnessMode   string          `json:"harness_mode"`
	Tasks         []BenchmarkTask `json:"tasks"`
	PassedTasks   int             `json:"passed_tasks"`
	FailedTasks   int             `json:"failed_tasks"`
	TotalDuration int64           `json:"total_duration_ms"`
	CreatedAt     time.Time       `json:"created_at"`
	Notes         []string        `json:"notes,omitempty"`
}

type BenchmarkTask struct {
	Name                      string            `json:"name"`
	Style                     string            `json:"style"`
	Request                   string            `json:"request"`
	Root                      string            `json:"root"`
	LogPath                   string            `json:"log_path"`
	ExitCode                  int               `json:"exit_code"`
	Passed                    bool              `json:"passed"`
	DurationMS                int64             `json:"duration_ms"`
	ObjectiveID               string            `json:"objective_id,omitempty"`
	RequestedNodes            int               `json:"requested_nodes,omitempty"`
	SucceededNodes            int               `json:"succeeded_nodes,omitempty"`
	FailedNodes               int               `json:"failed_nodes,omitempty"`
	CollapseApproved          bool              `json:"collapse_approved"`
	ForceAverage              float64           `json:"force_average,omitempty"`
	LakeScore                 float64           `json:"lake_score,omitempty"`
	LakeCacheCitationCoverage float64           `json:"lake_cache_citation_coverage,omitempty"`
	TracePassed               bool              `json:"trace_passed"`
	ManifestPassed            bool              `json:"manifest_passed"`
	ProductKind               string            `json:"product_kind,omitempty"`
	CostUSD                   float64           `json:"cost_usd,omitempty"`
	TotalTokens               int               `json:"total_tokens,omitempty"`
	Error                     string            `json:"error,omitempty"`
	Artifacts                 map[string]string `json:"artifacts,omitempty"`
}

func benchmarkCmd(args []string) {
	fs := flag.NewFlagSet("benchmark", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	suite := fs.String("suite", "local-smoke", "benchmark suite: local-smoke, terminal-contract, opencode-stress, or terminal-smoke")
	harnessMode := fs.String("harness", "simulated", "harness mode: simulated, opencode, or langchain")
	sandboxBackend := fs.String("sandbox", sandbox.BackendAuto, "sandbox backend passed to benchmark QSM runs")
	image := fs.String("image", "", "Docker image passed to sandboxed benchmark tasks")
	positionsFlag := fs.String("positions", "2", "positions passed to qsm run")
	parallelFlag := fs.String("parallel", "2", "parallel nodes passed to qsm run")
	timeout := fs.Duration("timeout", 20*time.Minute, "timeout for each benchmark task")
	retries := fs.Int("retries", 1, "retry count passed to qsm run")
	sharedCache := fs.Bool("shared-cache", true, "enable verified cross-node cache")
	routeHealthGate := fs.Bool("route-health", false, "probe route health before real harness tasks")
	deepSeekFallback := fs.Bool("deepseek-fallback", envBool("QSM_DEEPSEEK_FALLBACK"), "allow direct DeepSeek fallback when 9Router has no healthy routes")
	longRun := fs.Bool("long-run", false, "enable production long-run profile for benchmark tasks")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	if strings.TrimSpace(*image) != "" {
		_ = os.Setenv("QSM_SANDBOX_DOCKER_IMAGE", strings.TrimSpace(*image))
	}

	report := runBenchmarkSuite(*root, *suite, *harnessMode, *sandboxBackend, *positionsFlag, *parallelFlag, *timeout, *retries, *sharedCache, *routeHealthGate, *deepSeekFallback, *longRun)
	must(writeJSON(filepath.Join(*root, ".state", "benchmark_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "benchmark_report.md"), []byte(benchmarkMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(benchmarkMarkdown(report))
	if report.FailedTasks > 0 {
		os.Exit(1)
	}
}

func runBenchmarkSuite(root, suite, harnessMode, sandboxBackend, positions, parallel string, timeout time.Duration, retries int, sharedCache, routeHealth, deepSeekFallback, longRun bool) BenchmarkReport {
	rootAbs, err := filepath.Abs(root)
	must(err)
	exe, err := os.Executable()
	must(err)
	tasks := benchmarkTasks(suite)
	start := time.Now()
	runRoot := filepath.Join(rootAbs, ".benchmarks", time.Now().UTC().Format("20060102T150405.000000000Z")+"-"+sanitizeFileName(suite))
	must(os.MkdirAll(runRoot, 0755))
	out := BenchmarkReport{
		Schema:      "qsm.benchmark_report.v1",
		Suite:       suite,
		Style:       "SWE-bench/Terminal-Bench inspired local executable tasks",
		Root:        runRoot,
		HarnessMode: harnessMode,
		CreatedAt:   time.Now().UTC(),
		Notes: []string{
			"Local benchmark tasks are not official SWE-bench or Terminal-Bench scores.",
			"They enforce the same product shape: instruction, isolated task root, autonomous run, verifier evidence, and measurable result.",
		},
	}
	for i, task := range tasks {
		taskRoot := filepath.Join(runRoot, fmt.Sprintf("%02d-%s", i+1, sanitizeFileName(task.Name)))
		_ = os.RemoveAll(taskRoot)
		must(prepareBenchmarkRoot(rootAbs, taskRoot))
		logPath := filepath.Join(taskRoot, "benchmark.log")
		args := []string{
			"run",
			"-root", taskRoot,
			"-request", task.Request,
			"-harness", harnessMode,
			"-positions", positions,
			"-parallel", parallel,
			"-timeout", timeout.String(),
			"-sandbox", sandboxBackend,
			"-retries", strconv.Itoa(retries),
			"-shared-cache=" + strconv.FormatBool(sharedCache),
			"-route-health=" + strconv.FormatBool(routeHealth),
			"-deepseek-fallback=" + strconv.FormatBool(deepSeekFallback),
		}
		if longRun {
			args = append(args, "-long-run=true")
		}
		taskStart := time.Now()
		exitCode, runErr := runAutorunCycle(exe, logPath, timeout+30*time.Second, args)
		item := BenchmarkTask{
			Name:       task.Name,
			Style:      task.Style,
			Request:    task.Request,
			Root:       taskRoot,
			LogPath:    logPath,
			ExitCode:   exitCode,
			DurationMS: time.Since(taskStart).Milliseconds(),
			Artifacts: map[string]string{
				"run_report":       filepath.Join(taskRoot, ".state", "run_report.json"),
				"verdict":          filepath.Join(taskRoot, ".state", "verdict.json"),
				"force_score":      filepath.Join(taskRoot, ".state", "force_score.json"),
				"lake_interaction": filepath.Join(taskRoot, ".state", "lake_interaction_score.json"),
				"cost_report":      filepath.Join(taskRoot, ".state", "cost_report.json"),
				"trace_report":     filepath.Join(taskRoot, ".state", "trace_report.json"),
				"cost_budget":      filepath.Join(taskRoot, ".state", "cost_budget_report.json"),
				"manifest_report":  filepath.Join(taskRoot, ".state", "manifest_validation_report.json"),
			},
		}
		if runErr != nil {
			item.Error = runErr.Error()
		}
		if runErr == nil {
			traceReport := mustTraceReport(taskRoot)
			_ = writeJSON(filepath.Join(taskRoot, ".state", "trace_report.json"), traceReport)
			_ = os.WriteFile(filepath.Join(taskRoot, ".state", "trace_report.md"), []byte(traceMarkdown(traceReport)), 0644)
			budget := buildCostBudgetReport(taskRoot)
			_ = writeJSON(filepath.Join(taskRoot, ".state", "cost_budget_report.json"), budget)
			_ = os.WriteFile(filepath.Join(taskRoot, ".state", "cost_budget_report.md"), []byte(costBudgetMarkdown(budget)), 0644)
			manifestReport := winnerManifestReport(taskRoot)
			_ = writeJSON(filepath.Join(taskRoot, ".state", "manifest_validation_report.json"), manifestReport)
		}
		enrichBenchmarkTask(&item)
		item.Passed = exitCode == 0 && item.SucceededNodes > 0 && item.CollapseApproved && item.TracePassed && item.ManifestPassed && item.LakeCacheCitationCoverage >= 0.70
		if item.Passed {
			out.PassedTasks++
		} else {
			out.FailedTasks++
		}
		out.Tasks = append(out.Tasks, item)
	}
	out.TotalDuration = time.Since(start).Milliseconds()
	return out
}

type benchmarkTaskSpec struct {
	Name    string
	Style   string
	Request string
}

func benchmarkTasks(suite string) []benchmarkTaskSpec {
	switch strings.ToLower(strings.TrimSpace(suite)) {
	case "omni-contract":
		return []benchmarkTaskSpec{
			{Name: "omni-static-web", Style: "Omni-contract/static-web", Request: "product_kind=static-web. Build an interactive static browser app with local JavaScript, smoke tests, qsm_project_manifest.v1.json, and cache/wiki citations."},
			{Name: "omni-cli-tool", Style: "Omni-contract/cli-tool", Request: "product_kind=cli-tool. Build a command-line product with malformed input handling, real tests, qsm_project_manifest.v1.json, and cache/wiki citations."},
			{Name: "omni-go-service", Style: "Omni-contract/go-service", Request: "product_kind=go-service. Build a tiny Go HTTP service with unit/integration tests, qsm_project_manifest.v1.json, and cache/wiki citations."},
			{Name: "omni-python-package", Style: "Omni-contract/python-package", Request: "product_kind=python-package. Build a Python package with pytest tests and coverage-ready structure, qsm_project_manifest.v1.json, and cache/wiki citations."},
			{Name: "omni-node-fullstack", Style: "Omni-contract/node-fullstack", Request: "product_kind=node-fullstack. Build a minimal Node frontend/backend product with tests, qsm_project_manifest.v1.json, and cache/wiki citations."},
			{Name: "omni-data-transform", Style: "Omni-contract/data-transform", Request: "product_kind=data-transform. Build a parser/transformer with malformed input edge cases, tests, qsm_project_manifest.v1.json, and cache/wiki citations."},
		}
	case "terminal-contract":
		return []benchmarkTaskSpec{
			{Name: "terminal-cli-contract", Style: "Terminal-Bench-contract", Request: "Build a command-line notes product with unit tests, README, force checklist, QSM evidence, and a test_manifest verifier. The solution must be verified through real terminal command output."},
			{Name: "bugfix-red-green-contract", Style: "SWE-bench-contract", Request: "Build a tiny repo-style product that demonstrates a failing-test to passing-test bugfix workflow. Include the failing case, fixed implementation, tests, evidence, and citations to memory/cache facts used."},
			{Name: "static-web-contract", Style: "Terminal-Bench-contract", Request: "Build a static browser product with one real interaction, local assets, syntax checks, behavior smoke, test manifest, and QSM evidence."},
			{Name: "data-transform-contract", Style: "Terminal-Bench-contract", Request: "Build a small CSV/data transformer that handles malformed rows and edge cases with tests proving the behavior."},
		}
	case "opencode-stress":
		return []benchmarkTaskSpec{
			{Name: "opencode-terminal-product", Style: "Terminal-Bench-style", Request: "Build a tiny terminal-first notes CLI product. Include tests, README, force checklist, and evidence. Use terminal commands to verify it."},
			{Name: "opencode-recovery-loop", Style: "Terminal-Bench-style", Request: "Build a small file analyzer CLI with intentionally tricky edge cases. Discover edge cases through tests before final evidence."},
			{Name: "opencode-web-smoke", Style: "SWE-bench-style", Request: "Build a static browser product with a working interaction, auto tests, browser-safe assets, and QSM evidence."},
		}
	case "terminal-smoke":
		return []benchmarkTaskSpec{
			{Name: "terminal-cli", Style: "Terminal-Bench-style", Request: "Build a command-line checksum helper with tests and documented terminal verification."},
			{Name: "terminal-data-transform", Style: "Terminal-Bench-style", Request: "Build a small CSV transformer that handles malformed rows, with tests proving the behavior."},
		}
	default:
		return []benchmarkTaskSpec{
			{Name: "swe-bench-style-bugfix", Style: "SWE-bench-style", Request: "Build a tiny repo-style product that demonstrates a failing-test to passing-test bugfix workflow. Include tests, evidence, and force checklist."},
			{Name: "terminal-bench-style-cli", Style: "Terminal-Bench-style", Request: "Build a small CLI-style product that must be verified through real command output, not mental verification. Include test manifest and evidence."},
		}
	}
}

func prepareBenchmarkRoot(sourceRoot, taskRoot string) error {
	if err := os.MkdirAll(filepath.Join(taskRoot, "internal", "wiki"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(taskRoot, "harness"), 0755); err != nil {
		return err
	}
	_ = copyIfExists(filepath.Join(sourceRoot, "internal", "wiki", "wiki.md"), filepath.Join(taskRoot, "internal", "wiki", "wiki.md"))
	_ = copyIfExists(filepath.Join(sourceRoot, "harness", "langchain_runner.py"), filepath.Join(taskRoot, "harness", "langchain_runner.py"))
	if !fileExists(filepath.Join(taskRoot, "internal", "wiki", "wiki.md")) {
		return os.WriteFile(filepath.Join(taskRoot, "internal", "wiki", "wiki.md"), []byte("# QSM Benchmark Wiki\n\nNo prior memory.\n"), 0644)
	}
	return nil
}

func copyIfExists(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func winnerManifestReport(taskRoot string) productmanifest.ValidationReport {
	var verdict collapse.Verdict
	if err := readJSON(filepath.Join(taskRoot, ".state", "verdict.json"), &verdict); err == nil && verdict.Winner.ProductPath != "" {
		return productmanifest.Validate(verdict.Winner.ProductPath)
	}
	var runReport swarm.RunReport
	if err := readJSON(filepath.Join(taskRoot, ".state", "run_report.json"), &runReport); err == nil {
		for _, result := range runReport.Results {
			if result.ProductPath != "" {
				report := productmanifest.Validate(result.ProductPath)
				if report.Passed {
					return report
				}
			}
		}
	}
	return productmanifest.ValidationReport{
		Schema:      "qsm.project_manifest_validation.v1",
		ProductPath: filepath.Join(taskRoot, "product"),
		Passed:      false,
		Errors:      []string{"no winner product manifest available"},
	}
}

func enrichBenchmarkTask(task *BenchmarkTask) {
	var report swarm.RunReport
	if err := readJSON(task.Artifacts["run_report"], &report); err == nil {
		task.ObjectiveID = report.ObjectiveID
		task.RequestedNodes = report.RequestedNodes
		task.SucceededNodes = report.SucceededNodes
		task.FailedNodes = report.FailedNodes
	}
	var verdict collapse.Verdict
	if err := readJSON(task.Artifacts["verdict"], &verdict); err == nil {
		task.CollapseApproved = verdict.Approved
	}
	var force requirements.ScoreReport
	if err := readJSON(task.Artifacts["force_score"], &force); err == nil {
		task.ForceAverage = force.AverageScore
	}
	var lakeReport lakebrain.Report
	if err := readJSON(task.Artifacts["lake_interaction"], &lakeReport); err == nil {
		task.LakeScore = lakeReport.AverageNodeScore
		task.LakeCacheCitationCoverage = lakeReport.CacheCitationCoverage
	}
	var traceReport TraceReport
	if err := readJSON(filepath.Join(task.Root, ".state", "trace_report.json"), &traceReport); err == nil {
		task.TracePassed = traceReport.Passed
	}
	var manifestReport productmanifest.ValidationReport
	if err := readJSON(task.Artifacts["manifest_report"], &manifestReport); err == nil {
		task.ManifestPassed = manifestReport.Passed
		task.ProductKind = manifestReport.ProductKind
	}
	var costReport costing.Report
	if err := readJSON(task.Artifacts["cost_report"], &costReport); err == nil {
		task.CostUSD = costReport.EstimatedUSD
		task.TotalTokens = costReport.TotalTokens
	}
}

func benchmarkMarkdown(report BenchmarkReport) string {
	var b strings.Builder
	b.WriteString("# QSM Benchmark Report\n\n")
	fmt.Fprintf(&b, "- Suite: `%s`\n", report.Suite)
	fmt.Fprintf(&b, "- Style: `%s`\n", report.Style)
	fmt.Fprintf(&b, "- Harness: `%s`\n", report.HarnessMode)
	fmt.Fprintf(&b, "- Passed: `%d/%d`\n", report.PassedTasks, len(report.Tasks))
	fmt.Fprintf(&b, "- Duration: `%dms`\n", report.TotalDuration)
	b.WriteString("\n## Tasks\n\n")
	b.WriteString("| Task | Kind | Style | Passed | Manifest | Nodes | Force | Lake | Tokens | Cost | Log |\n")
	b.WriteString("| --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, task := range report.Tasks {
		fmt.Fprintf(&b, "| %s | %s | %s | %v | %v | %d/%d | %.1f | %.1f | %d | %.6f | `%s` |\n",
			task.Name, task.ProductKind, task.Style, task.Passed, task.ManifestPassed, task.SucceededNodes, task.RequestedNodes, task.ForceAverage, task.LakeScore, task.TotalTokens, task.CostUSD, task.LogPath)
		if task.Error != "" {
			fmt.Fprintf(&b, "\nTask `%s` error: `%s`\n", task.Name, truncateStatusError(task.Error, 220))
		}
	}
	if len(report.Notes) > 0 {
		b.WriteString("\n## Notes\n\n")
		for _, note := range report.Notes {
			b.WriteString("- " + note + "\n")
		}
	}
	return b.String()
}

type SelfImproveReport struct {
	Schema              string              `json:"schema"`
	Suite               string              `json:"suite"`
	Root                string              `json:"root"`
	Cycles              int                 `json:"cycles"`
	Passed              bool                `json:"passed"`
	BaselinePassedTasks int                 `json:"baseline_passed_tasks"`
	FinalPassedTasks    int                 `json:"final_passed_tasks"`
	ForceDelta          float64             `json:"force_delta"`
	RepeatedFailureRate float64             `json:"repeated_failure_rate"`
	FailedTasks         map[string]int      `json:"failed_tasks,omitempty"`
	TaskSummaries       []SelfImproveCycle  `json:"task_summaries,omitempty"`
	LessonsPromoted     []SelfImproveLesson `json:"lessons_promoted,omitempty"`
	CycleReports        []BenchmarkReport   `json:"cycle_reports,omitempty"`
	Errors              []string            `json:"errors,omitempty"`
	CreatedAt           time.Time           `json:"created_at"`
}

type SelfImproveCycle struct {
	Cycle int                      `json:"cycle"`
	Tasks []SelfImproveTaskSummary `json:"tasks"`
}

type SelfImproveTaskSummary struct {
	Name                      string  `json:"name"`
	Passed                    bool    `json:"passed"`
	ProductKind               string  `json:"product_kind,omitempty"`
	ExitCode                  int     `json:"exit_code"`
	SucceededNodes            int     `json:"succeeded_nodes"`
	FailedNodes               int     `json:"failed_nodes"`
	CollapseApproved          bool    `json:"collapse_approved"`
	TracePassed               bool    `json:"trace_passed"`
	ManifestPassed            bool    `json:"manifest_passed"`
	LakeCacheCitationCoverage float64 `json:"lake_cache_citation_coverage"`
	ForceAverage              float64 `json:"force_average"`
	Error                     string  `json:"error,omitempty"`
	LogTail                   string  `json:"log_tail,omitempty"`
}

type SelfImproveLesson struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Content  string `json:"content"`
	Citation string `json:"citation"`
}

func selfImproveCmd(args []string) {
	fs := flag.NewFlagSet("self-improve", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	suite := fs.String("suite", "omni-contract", "benchmark suite to self-improve")
	cycles := fs.Int("cycles", 3, "number of benchmark/improvement cycles")
	harnessMode := fs.String("harness", "simulated", "harness mode: simulated, opencode, or langchain")
	sandboxBackend := fs.String("sandbox", sandbox.BackendAuto, "sandbox backend passed to cycle runs")
	image := fs.String("image", "", "Docker image passed to sandboxed cycle runs")
	positionsFlag := fs.String("positions", "2", "positions passed to qsm run")
	parallelFlag := fs.String("parallel", "2", "parallel nodes passed to qsm run")
	timeout := fs.Duration("timeout", 20*time.Minute, "timeout for each benchmark task")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	if strings.TrimSpace(*image) != "" {
		_ = os.Setenv("QSM_SANDBOX_DOCKER_IMAGE", strings.TrimSpace(*image))
	}
	report := runSelfImprove(*root, *suite, *cycles, *harnessMode, *sandboxBackend, *positionsFlag, *parallelFlag, *timeout)
	must(writeJSON(filepath.Join(*root, ".state", "self_improvement_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "self_improvement_report.md"), []byte(selfImproveMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(selfImproveMarkdown(report))
	if !report.Passed {
		os.Exit(1)
	}
}

func runSelfImprove(root, suite string, cycles int, harnessMode, sandboxBackend, positions, parallel string, timeout time.Duration) SelfImproveReport {
	if cycles <= 0 {
		cycles = 1
	}
	rootAbs, err := filepath.Abs(root)
	must(err)
	out := SelfImproveReport{
		Schema:    "qsm.self_improvement_report.v1",
		Suite:     suite,
		Root:      rootAbs,
		Cycles:    cycles,
		CreatedAt: time.Now().UTC(),
	}
	q := mustOpen(rootAbs)
	failures := map[string]int{}
	firstForce := 0.0
	lastForce := 0.0
	for cycle := 1; cycle <= cycles; cycle++ {
		report := runBenchmarkSuite(rootAbs, suite, harnessMode, sandboxBackend, positions, parallel, timeout, 1, true, false, envBool("QSM_DEEPSEEK_FALLBACK"), false)
		out.CycleReports = append(out.CycleReports, report)
		if cycle == 1 {
			out.BaselinePassedTasks = report.PassedTasks
		}
		out.FinalPassedTasks = report.PassedTasks
		avgForce := averageBenchmarkForce(report)
		if cycle == 1 {
			firstForce = avgForce
		}
		lastForce = avgForce
		cycleSummary := SelfImproveCycle{Cycle: cycle}
		for _, task := range report.Tasks {
			if !task.Passed {
				failures[task.Name]++
			}
			summary := SelfImproveTaskSummary{
				Name:                      task.Name,
				Passed:                    task.Passed,
				ProductKind:               task.ProductKind,
				ExitCode:                  task.ExitCode,
				SucceededNodes:            task.SucceededNodes,
				FailedNodes:               task.FailedNodes,
				CollapseApproved:          task.CollapseApproved,
				TracePassed:               task.TracePassed,
				ManifestPassed:            task.ManifestPassed,
				LakeCacheCitationCoverage: task.LakeCacheCitationCoverage,
				ForceAverage:              task.ForceAverage,
				Error:                     task.Error,
			}
			if !task.Passed {
				summary.LogTail = tailTextFile(task.LogPath, 80)
			}
			cycleSummary.Tasks = append(cycleSummary.Tasks, summary)
		}
		out.TaskSummaries = append(out.TaskSummaries, cycleSummary)
		lesson := SelfImproveLesson{
			ID:       fmt.Sprintf("self-improve-cycle-%d", cycle),
			Source:   fmt.Sprintf("benchmark:%s:cycle:%d", suite, cycle),
			Content:  fmt.Sprintf("Cycle %d passed %d/%d tasks with average force %.2f. Preserve working manifests, sandbox traces, and cache/wiki citations.", cycle, report.PassedTasks, len(report.Tasks), avgForce),
			Citation: fmt.Sprintf("lake_artifact:self-improve-cycle-%d", cycle),
		}
		artifact, err := q.Put(lake.Artifact{
			Phase:      lake.PhaseResearch,
			Kind:       "self_improvement_lesson",
			Source:     lesson.Source,
			Claim:      "QSM benchmark cycle lesson promoted into lake memory",
			Content:    lesson.Content,
			Confidence: 0.82,
			Verified:   report.PassedTasks == len(report.Tasks),
			Metadata: map[string]string{
				"suite": suite,
				"cycle": strconv.Itoa(cycle),
			},
		})
		if err == nil {
			lesson.Citation = "lake_artifact:" + artifact.ID
		}
		if cacheID, err := q.PutCache(lake.CacheItem{
			Kind:        "self_improvement_lesson",
			ObjectiveID: "self-improve",
			PositionID:  fmt.Sprintf("cycle-%d", cycle),
			Producer:    "qsm self-improve",
			Content:     lesson.Content,
			Verified:    report.PassedTasks == len(report.Tasks),
			Confidence:  0.82,
			Metadata: map[string]string{
				"suite": suite,
				"cycle": strconv.Itoa(cycle),
			},
			CreatedAt: time.Now().UTC(),
		}); err == nil {
			lesson.Citation = "cache_item:" + cacheID.ID
		}
		out.LessonsPromoted = append(out.LessonsPromoted, lesson)
	}
	out.ForceDelta = lastForce - firstForce
	repeated := 0
	for _, count := range failures {
		if count > 1 {
			repeated++
		}
	}
	if len(failures) > 0 {
		out.FailedTasks = failures
	}
	if len(failures) > 0 {
		out.RepeatedFailureRate = float64(repeated) / float64(len(failures))
	}
	if out.FinalPassedTasks < out.BaselinePassedTasks {
		out.Errors = append(out.Errors, "self-improvement regressed passed task count")
	}
	finalTotalTasks := 0
	if len(out.CycleReports) > 0 {
		finalTotalTasks = len(out.CycleReports[len(out.CycleReports)-1].Tasks)
	}
	saturatedContract := finalTotalTasks > 0 && out.BaselinePassedTasks == finalTotalTasks && out.FinalPassedTasks == finalTotalTasks && len(failures) == 0
	if out.ForceDelta < 0.5 && lastForce < 9.5 && !saturatedContract {
		out.Errors = append(out.Errors, fmt.Sprintf("force delta %.2f is below +0.5 and final force %.2f is below 9.5", out.ForceDelta, lastForce))
	}
	if out.RepeatedFailureRate > 0 {
		out.Errors = append(out.Errors, fmt.Sprintf("repeated failure rate %.2f is above zero", out.RepeatedFailureRate))
	}
	if len(out.LessonsPromoted) < cycles {
		out.Errors = append(out.Errors, "not every cycle promoted a lesson")
	}
	for _, lesson := range out.LessonsPromoted {
		if !strings.HasPrefix(lesson.Citation, "cache_item:") && !strings.HasPrefix(lesson.Citation, "wiki_item:") {
			out.Errors = append(out.Errors, "promoted lesson lacks cache/wiki citation: "+lesson.ID)
		}
	}
	out.Passed = len(out.Errors) == 0
	return out
}

func averageBenchmarkForce(report BenchmarkReport) float64 {
	if len(report.Tasks) == 0 {
		return 0
	}
	total := 0.0
	for _, task := range report.Tasks {
		total += task.ForceAverage
	}
	return total / float64(len(report.Tasks))
}

func selfImproveMarkdown(report SelfImproveReport) string {
	var b strings.Builder
	b.WriteString("# QSM Self-Improvement Report\n\n")
	fmt.Fprintf(&b, "- Suite: `%s`\n", report.Suite)
	fmt.Fprintf(&b, "- Cycles: `%d`\n", report.Cycles)
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	fmt.Fprintf(&b, "- Passed tasks: `%d -> %d`\n", report.BaselinePassedTasks, report.FinalPassedTasks)
	fmt.Fprintf(&b, "- Force delta: `%.2f`\n", report.ForceDelta)
	fmt.Fprintf(&b, "- Repeated failure rate: `%.2f`\n", report.RepeatedFailureRate)
	if len(report.LessonsPromoted) > 0 {
		b.WriteString("\n## Lessons Promoted\n\n")
		for _, lesson := range report.LessonsPromoted {
			fmt.Fprintf(&b, "- `%s` %s (%s)\n", lesson.ID, lesson.Content, lesson.Citation)
		}
	}
	if len(report.FailedTasks) > 0 {
		b.WriteString("\n## Failed Tasks\n\n")
		names := make([]string, 0, len(report.FailedTasks))
		for name := range report.FailedTasks {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, "- `%s` failed in `%d` cycle(s)\n", name, report.FailedTasks[name])
		}
	}
	if len(report.TaskSummaries) > 0 {
		b.WriteString("\n## Cycle Task Summary\n\n")
		b.WriteString("| Cycle | Task | Passed | Exit | Nodes | Collapse | Trace | Manifest | Cache/wiki citations | Force | Error |\n")
		b.WriteString("| ---: | --- | --- | ---: | ---: | --- | --- | --- | ---: | ---: | --- |\n")
		for _, cycle := range report.TaskSummaries {
			for _, task := range cycle.Tasks {
				fmt.Fprintf(&b, "| %d | %s | %v | %d | %d/%d | %v | %v | %v | %.2f | %.1f | %s |\n",
					cycle.Cycle, task.Name, task.Passed, task.ExitCode, task.SucceededNodes, task.SucceededNodes+task.FailedNodes, task.CollapseApproved, task.TracePassed, task.ManifestPassed, task.LakeCacheCitationCoverage, task.ForceAverage, markdownTableCell(truncateStatusError(task.Error, 180)))
			}
		}
		for _, cycle := range report.TaskSummaries {
			for _, task := range cycle.Tasks {
				if task.LogTail == "" {
					continue
				}
				fmt.Fprintf(&b, "\n### Cycle %d `%s` Log Tail\n\n", cycle.Cycle, task.Name)
				b.WriteString("```text\n")
				b.WriteString(task.LogTail)
				if !strings.HasSuffix(task.LogTail, "\n") {
					b.WriteString("\n")
				}
				b.WriteString("```\n")
			}
		}
	}
	if len(report.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, err := range report.Errors {
			b.WriteString("- " + err + "\n")
		}
	}
	return b.String()
}

type QAReport struct {
	Schema         string    `json:"schema"`
	Root           string    `json:"root"`
	Profile        string    `json:"profile"`
	SandboxBackend string    `json:"sandbox_backend"`
	Passed         bool      `json:"passed"`
	Summary        QASummary `json:"summary"`
	Gates          []QAGate  `json:"gates"`
	CreatedAt      time.Time `json:"created_at"`
}

type QASummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Warned  int `json:"warned"`
	Skipped int `json:"skipped"`
}

type QAGate struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	Required       bool   `json:"required"`
	Evidence       string `json:"evidence"`
	Recommendation string `json:"recommendation,omitempty"`
}

func qaCmd(args []string) {
	fs := flag.NewFlagSet("qa", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	profile := fs.String("profile", "alpha", "QA profile: alpha, beta, local-production, production, or omni-alpha")
	sandboxBackend := fs.String("sandbox", "auto", "QA sandbox backend: room, docker, or auto")
	image := fs.String("image", "", "Docker image used for refreshed QA sandbox probes and quality gates")
	refresh := fs.Bool("refresh", true, "refresh sandbox and force-score artifacts before evaluating")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	report, err := runQA(*root, *profile, *refresh, *sandboxBackend, *image)
	must(err)
	must(writeJSON(filepath.Join(*root, ".state", "qa_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "qa_report.md"), []byte(qaMarkdown(report)), 0644))
	if qaProfileRank(report.Profile) >= qaProfileRank("production") {
		gapReport := buildProductionGapReport(*root)
		must(writeJSON(filepath.Join(*root, ".state", "production_gap_report.json"), gapReport))
		must(os.WriteFile(filepath.Join(*root, ".state", "production_gap_report.md"), []byte(productionGapMarkdown(gapReport)), 0644))
	}
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(qaMarkdown(report))
	if !report.Passed {
		os.Exit(1)
	}
}

func runQA(root, profile string, refresh bool, sandboxBackend string, image string) (QAReport, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return QAReport{}, err
	}
	profile = normalizeQAProfile(profile)
	sandboxBackend = sandbox.NormalizeBackend(sandboxBackend)
	if strings.TrimSpace(image) != "" {
		_ = os.Setenv("QSM_SANDBOX_DOCKER_IMAGE", strings.TrimSpace(image))
	}
	now := time.Now().UTC()
	out := QAReport{
		Schema:         "qsm.qa_report.v1",
		Root:           rootAbs,
		Profile:        profile,
		SandboxBackend: sandboxBackend,
		Passed:         true,
		CreatedAt:      now,
	}

	var runReport swarm.RunReport
	runErr := readJSON(filepath.Join(rootAbs, ".state", "run_report.json"), &runReport)
	var verdict collapse.Verdict
	_ = readJSON(filepath.Join(rootAbs, ".state", "verdict.json"), &verdict)
	var planReport planning.Report
	_ = readJSON(filepath.Join(rootAbs, ".state", "plan_report.json"), &planReport)
	var refreshedSandboxReport sandbox.Report

	if refresh && runErr == nil && runReport.ObjectiveID != "" {
		sandboxReport := sandbox.InspectWithProbe(rootAbs, sandboxBackend)
		if profile == "omni-alpha" {
			sandboxReport = enrichOmniSandboxProbe(sandboxReport)
		}
		_ = sandbox.Write(rootAbs, sandboxReport)
		refreshedSandboxReport = sandboxReport
		if traceReport, traceErr := buildTraceReport(rootAbs); traceErr == nil {
			_ = writeJSON(filepath.Join(rootAbs, ".state", "trace_report.json"), traceReport)
			_ = os.WriteFile(filepath.Join(rootAbs, ".state", "trace_report.md"), []byte(traceMarkdown(traceReport)), 0644)
		}
		if lakeReport, lakeErr := lakebrain.Analyze(mustOpen(rootAbs), runReport); lakeErr == nil {
			_ = lakebrain.Write(rootAbs, lakeReport)
		}
		budgetReport := buildCostBudgetReport(rootAbs)
		_ = writeJSON(filepath.Join(rootAbs, ".state", "cost_budget_report.json"), budgetReport)
		_ = os.WriteFile(filepath.Join(rootAbs, ".state", "cost_budget_report.md"), []byte(costBudgetMarkdown(budgetReport)), 0644)
		if qaProfileRank(profile) >= qaProfileRank("production") && sandboxReport.HardSandboxReady {
			coverageReport := buildCoverageReport(rootAbs, sandboxBackend)
			_ = writeJSON(filepath.Join(rootAbs, ".state", "coverage_report.json"), coverageReport)
			_ = os.WriteFile(filepath.Join(rootAbs, ".state", "coverage_report.md"), []byte(qualityMarkdown(coverageReport)), 0644)
			flakeReport := buildFlakeReport(rootAbs, sandboxBackend, 3)
			_ = writeJSON(filepath.Join(rootAbs, ".state", "flake_report.json"), flakeReport)
			_ = os.WriteFile(filepath.Join(rootAbs, ".state", "flake_report.md"), []byte(qualityMarkdown(flakeReport)), 0644)
			mutationReport := buildMutationReport(rootAbs, sandboxBackend)
			_ = writeJSON(filepath.Join(rootAbs, ".state", "mutation_report.json"), mutationReport)
			_ = os.WriteFile(filepath.Join(rootAbs, ".state", "mutation_report.md"), []byte(qualityMarkdown(mutationReport)), 0644)
		}
		if qaProfileRank(profile) >= qaProfileRank("production") {
			ciReport := buildCIReleaseReportFromQA(rootAbs, out)
			_ = writeJSON(filepath.Join(rootAbs, ".state", "ci_release_report.json"), ciReport)
			_ = os.WriteFile(filepath.Join(rootAbs, ".state", "ci_release_report.md"), []byte(ciReleaseMarkdown(ciReport)), 0644)
		}
		score, scoreErr := writeForceScoreArtifacts(rootAbs, mustOpen(rootAbs), planReport, runReport, verdict)
		if scoreErr == nil {
			_ = score
		}
	}

	var forceScore requirements.ScoreReport
	_ = readJSON(filepath.Join(rootAbs, ".state", "force_score.json"), &forceScore)
	var lakeReport lakebrain.Report
	_ = readJSON(filepath.Join(rootAbs, ".state", "lake_interaction_score.json"), &lakeReport)
	var costReport costing.Report
	_ = readJSON(filepath.Join(rootAbs, ".state", "cost_report.json"), &costReport)
	var sandboxReport sandbox.Report
	if refreshedSandboxReport.Schema != "" {
		sandboxReport = refreshedSandboxReport
	} else {
		_ = readJSON(filepath.Join(rootAbs, ".state", "sandbox_report.json"), &sandboxReport)
	}
	var benchReport BenchmarkReport
	_ = readJSON(filepath.Join(rootAbs, ".state", "benchmark_report.json"), &benchReport)
	roomStatuses := collectRoomStatuses(runReport)

	add := func(id, name, status string, required bool, evidence, recommendation string) {
		gate := QAGate{
			ID:             id,
			Name:           name,
			Status:         status,
			Required:       required,
			Evidence:       evidence,
			Recommendation: recommendation,
		}
		out.Gates = append(out.Gates, gate)
		out.Summary.Total++
		switch status {
		case "PASS":
			out.Summary.Passed++
		case "FAIL":
			out.Summary.Failed++
			if required {
				out.Passed = false
			}
		case "WARN":
			out.Summary.Warned++
		default:
			out.Summary.Skipped++
		}
	}
	requiredFor := func(minProfile string) bool {
		return qaProfileRank(profile) >= qaProfileRank(minProfile)
	}

	if runErr != nil || runReport.ObjectiveID == "" {
		add("run-report", "Run report exists", "FAIL", true, "missing .state/run_report.json", "Run qsm run before qsm qa.")
		return out, nil
	}
	add("run-report", "Run report exists", "PASS", true, fmt.Sprintf("objective=%s harness=%s", runReport.ObjectiveID, runReport.HarnessMode), "")

	if runReport.AllNodesAccounted {
		add("nodes-accounted", "All nodes accounted", "PASS", true, fmt.Sprintf("requested=%d started=%d succeeded=%d failed=%d", runReport.RequestedNodes, runReport.StartedNodes, runReport.SucceededNodes, runReport.FailedNodes), "")
	} else {
		add("nodes-accounted", "All nodes accounted", "FAIL", true, fmt.Sprintf("requested=%d started=%d succeeded=%d failed=%d", runReport.RequestedNodes, runReport.StartedNodes, runReport.SucceededNodes, runReport.FailedNodes), "Fix stale or orphaned room status before collapse.")
	}

	staleRooms := 0
	for _, status := range roomStatuses {
		if status.State == swarm.RoomStateRunning || status.State == swarm.RoomStateQueued {
			staleRooms++
		}
	}
	if staleRooms == 0 {
		add("no-stale-rooms", "No stale running rooms", "PASS", true, fmt.Sprintf("room_statuses=%d stale=%d", len(roomStatuses), staleRooms), "")
	} else {
		add("no-stale-rooms", "No stale running rooms", "FAIL", true, fmt.Sprintf("room_statuses=%d stale=%d", len(roomStatuses), staleRooms), "Run cleanup or improve node watchdog coverage.")
	}

	if runReport.SucceededNodes > 0 {
		add("node-success", "At least one node succeeded", "PASS", true, fmt.Sprintf("succeeded=%d/%d", runReport.SucceededNodes, runReport.RequestedNodes), "")
	} else {
		add("node-success", "At least one node succeeded", "FAIL", true, fmt.Sprintf("succeeded=%d/%d", runReport.SucceededNodes, runReport.RequestedNodes), "Fix route/harness/test failures before QA promotion.")
	}

	testCommands, passedCommands, failedCommands, reportCount, missingReports, weakReports := qaTestEvidence(runReport)
	if missingReports == 0 && weakReports == 0 && reportCount > 0 && failedCommands == 0 {
		add("qsm-test-report", "QSM-owned test reports pass", "PASS", true, fmt.Sprintf("reports=%d test_commands=%d passed=%d failed=%d missing_reports=%d weak_reports=%d", reportCount, testCommands, passedCommands, failedCommands, missingReports, weakReports), "")
	} else {
		add("qsm-test-report", "QSM-owned test reports pass", "FAIL", true, fmt.Sprintf("reports=%d test_commands=%d passed=%d failed=%d missing_reports=%d weak_reports=%d", reportCount, testCommands, passedCommands, failedCommands, missingReports, weakReports), "Every successful code/static-web node needs a passing .qsm_test report; generic document products still need a passed QSM report.")
	}

	critical, high, medium := qaSecurityCounts(runReport)
	if critical == 0 && high == 0 {
		add("security-basic", "No critical/high basic security findings", "PASS", true, fmt.Sprintf("critical=%d high=%d medium=%d", critical, high, medium), "")
	} else {
		add("security-basic", "No critical/high basic security findings", "FAIL", true, fmt.Sprintf("critical=%d high=%d medium=%d", critical, high, medium), "Remove secrets/dynamic execution hazards before delivery.")
	}

	forceMin := qaForceMinimum(profile)
	if forceScore.Schema != "" && forceScore.AverageScore >= forceMin {
		add("force-score", "Force requirements score threshold", "PASS", true, fmt.Sprintf("average=%.1f min=%.1f top_tier=%v", forceScore.AverageScore, forceMin, forceScore.TopTier), "")
	} else if forceScore.Schema != "" {
		add("force-score", "Force requirements score threshold", "FAIL", true, fmt.Sprintf("average=%.1f min=%.1f top_tier=%v", forceScore.AverageScore, forceMin, forceScore.TopTier), "Close mandatory force checklist gaps for this profile.")
	} else {
		add("force-score", "Force requirements score threshold", "FAIL", true, "missing .state/force_score.json", "Run qsm force-score or qsm qa -refresh=true.")
	}

	if verdict.Approved {
		add("collapse-approved", "Collapse approved a winner", "PASS", requiredFor("beta"), fmt.Sprintf("winner=%s approved=%v", verdict.Winner.PositionID, verdict.Approved), "")
	} else if requiredFor("beta") {
		add("collapse-approved", "Collapse approved a winner", "FAIL", true, fmt.Sprintf("winner=%s approved=%v reason=%s", verdict.Winner.PositionID, verdict.Approved, verdict.Reason), "Beta and production require an approved collapse.")
	} else {
		add("collapse-approved", "Collapse approved a winner", "WARN", false, fmt.Sprintf("winner=%s approved=%v reason=%s", verdict.Winner.PositionID, verdict.Approved, verdict.Reason), "Alpha can inspect failed runs, but beta requires approval.")
	}

	if costReport.Schema != "" {
		add("cost-accounting", "Cost/token accounting present", "PASS", requiredFor("beta"), fmt.Sprintf("tokens=%d estimated_usd=%.6f", costReport.TotalTokens, costReport.EstimatedUSD), "")
	} else if requiredFor("beta") {
		add("cost-accounting", "Cost/token accounting present", "FAIL", true, "missing .state/cost_report.json", "Run qsm cost after each run.")
	} else {
		add("cost-accounting", "Cost/token accounting present", "WARN", false, "missing .state/cost_report.json", "Alpha should still capture cost evidence.")
	}

	if benchReport.Schema != "" && len(benchReport.Tasks) > 0 && benchReport.PassedTasks == len(benchReport.Tasks) {
		add("benchmark", "Benchmark suite passed", "PASS", requiredFor("beta"), fmt.Sprintf("suite=%s passed=%d/%d", benchReport.Suite, benchReport.PassedTasks, len(benchReport.Tasks)), "")
	} else if requiredFor("beta") {
		add("benchmark", "Benchmark suite passed", "FAIL", true, fmt.Sprintf("suite=%s passed=%d/%d", benchReport.Suite, benchReport.PassedTasks, len(benchReport.Tasks)), "Run qsm benchmark and fix failures.")
	} else {
		add("benchmark", "Benchmark suite passed", "WARN", false, fmt.Sprintf("suite=%s passed=%d/%d", benchReport.Suite, benchReport.PassedTasks, len(benchReport.Tasks)), "Alpha should keep local benchmark evidence fresh.")
	}

	if qaProfileRank(profile) >= qaProfileRank("production") && !officialLikeBenchmark(benchReport) {
		add("official-shaped-benchmark", "Official-shaped benchmark evidence", "FAIL", true, fmt.Sprintf("suite=%s style=%s", benchReport.Suite, benchReport.Style), "Production requires official or official-shaped SWE-bench/Terminal-Bench adapters.")
	} else if qaProfileRank(profile) < qaProfileRank("production") {
		add("official-shaped-benchmark", "Official-shaped benchmark evidence", "WARN", false, fmt.Sprintf("suite=%s style=%s", benchReport.Suite, benchReport.Style), "Implement official benchmark adapters before production.")
	} else {
		add("official-shaped-benchmark", "Official-shaped benchmark evidence", "PASS", true, fmt.Sprintf("suite=%s style=%s", benchReport.Suite, benchReport.Style), "")
	}

	if sandboxReport.Schema != "" && sandboxReport.HardSandboxReady {
		add("hard-sandbox", "Hard sandbox ready", "PASS", requiredFor("production"), fmt.Sprintf("readiness=%s hard_ready=%v backend=%s probe=%v", sandboxReport.ReadinessLevel, sandboxReport.HardSandboxReady, sandboxReport.Probe.Backend, sandboxReport.Probe.Passed), "")
	} else if qaProfileRank(profile) >= qaProfileRank("production") {
		add("hard-sandbox", "Hard sandbox ready", "FAIL", true, fmt.Sprintf("readiness=%s hard_ready=%v docker_cli=%v docker_daemon=%v backend=%s probe=%v", sandboxReport.ReadinessLevel, sandboxReport.HardSandboxReady, sandboxReport.Docker.Available, sandboxReport.DockerDaemon.Available, sandboxReport.Probe.Backend, sandboxReport.Probe.Passed), "Wire Docker/microVM execution before production; room-only never satisfies hard sandbox.")
	} else if profile == "beta" && sandboxReport.Schema != "" && sandboxReport.Docker.Available {
		add("hard-sandbox", "Hard sandbox ready", "WARN", false, fmt.Sprintf("readiness=%s hard_ready=%v docker_cli=%v docker_daemon=%v backend=%s probe=%v", sandboxReport.ReadinessLevel, sandboxReport.HardSandboxReady, sandboxReport.Docker.Available, sandboxReport.DockerDaemon.Available, sandboxReport.Probe.Backend, sandboxReport.Probe.Passed), "Beta can proceed with warning; production needs enforced Docker/microVM isolation.")
	} else {
		add("hard-sandbox", "Hard sandbox ready", "WARN", false, fmt.Sprintf("readiness=%s hard_ready=%v docker_cli=%v docker_daemon=%v backend=%s probe=%v", sandboxReport.ReadinessLevel, sandboxReport.HardSandboxReady, sandboxReport.Docker.Available, sandboxReport.DockerDaemon.Available, sandboxReport.Probe.Backend, sandboxReport.Probe.Passed), "Install/wire a hard sandbox runner.")
	}

	if lakeReport.Schema != "" && (profile == "alpha" || lakeReport.CacheCitationCoverage >= 0.70) {
		status := "PASS"
		if profile == "alpha" && lakeReport.CacheCitationCoverage < 0.70 {
			status = "WARN"
		}
		add("lake-citations", "Lake/cache citation coverage", status, requiredFor("beta"), fmt.Sprintf("avg=%.1f refresh=%.0f%% writes=%.0f%% citations=%.0f%%", lakeReport.AverageNodeScore, lakeReport.RefreshCoverage*100, lakeReport.CacheWriteCoverage*100, lakeReport.CacheCitationCoverage*100), "Require nodes to consume and cite lake IDs before beta.")
	} else if requiredFor("beta") {
		add("lake-citations", "Lake/cache citation coverage", "FAIL", true, fmt.Sprintf("avg=%.1f refresh=%.0f%% writes=%.0f%% citations=%.0f%%", lakeReport.AverageNodeScore, lakeReport.RefreshCoverage*100, lakeReport.CacheWriteCoverage*100, lakeReport.CacheCitationCoverage*100), "Require nodes to consume and cite lake IDs.")
	} else {
		add("lake-citations", "Lake/cache citation coverage", "WARN", false, "missing .state/lake_interaction_score.json", "Run qsm lake-score.")
	}

	productionOnlyGates := []struct {
		id   string
		name string
		path string
		rec  string
	}{
		{"trace-replay", "Trace/replay evidence", filepath.Join(rootAbs, ".state", "trace_report.json"), "Emit LLM/tool/file-edit traces into the lake."},
		{"stress", "Concurrency stress evidence", filepath.Join(rootAbs, ".state", "stress_report.json"), "Run qsm stress with enough nodes/parallelism for the target profile."},
		{"recovery", "Self-healing recovery evidence", filepath.Join(rootAbs, ".state", "recovery_report.json"), "Run qsm recovery and verify failure capture plus recovery pass."},
		{"contributor-smoke", "Contributor setup smoke evidence", filepath.Join(rootAbs, ".state", "contributor_smoke_report.json"), "Run qsm contributor-smoke to prove a new developer can build/test from checkout."},
		{"coverage", "Coverage evidence", filepath.Join(rootAbs, ".state", "coverage_report.json"), "Add language-specific coverage collection and thresholds."},
		{"mutation", "Mutation/negative-control evidence", filepath.Join(rootAbs, ".state", "mutation_report.json"), "Run copied-sandbox mutation probes and require tests to catch intentional breaks."},
		{"flake", "Flake quarantine evidence", filepath.Join(rootAbs, ".state", "flake_report.json"), "Classify deterministic pass/fail vs flaky retries."},
		{"compliance", "Compliance/SBOM evidence", filepath.Join(rootAbs, ".state", "compliance_report.json"), "Generate sandbox policy and local SBOM/license inventory evidence."},
		{"ci-release", "CI/release gate evidence", filepath.Join(rootAbs, ".state", "ci_release_report.json"), "Add qsm qa to CI with the same production profile."},
		{"cost-budget", "Cost/latency/token budget evidence", filepath.Join(rootAbs, ".state", "cost_budget_report.json"), "Enforce per-objective token, cost, and latency ceilings."},
	}
	for _, gate := range productionOnlyGates {
		passed, evidence := qaReportFilePassed(gate.path)
		if passed {
			add(gate.id, gate.name, "PASS", qaProfileRank(profile) >= qaProfileRank("production"), evidence, "")
		} else if fileExists(gate.path) && qaProfileRank(profile) >= qaProfileRank("production") {
			add(gate.id, gate.name, "FAIL", true, evidence, gate.rec)
		} else if fileExists(gate.path) {
			add(gate.id, gate.name, "WARN", false, evidence, gate.rec)
		} else if qaProfileRank(profile) >= qaProfileRank("production") {
			add(gate.id, gate.name, "FAIL", true, filepath.Base(gate.path)+" missing", gate.rec)
		} else {
			add(gate.id, gate.name, "WARN", false, filepath.Base(gate.path)+" missing", gate.rec)
		}
	}
	if qaProfileRank(profile) >= qaProfileRank("omni-alpha") {
		addOmniAlphaGates(rootAbs, &out, add, benchReport, forceScore, sandboxReport)
	}

	return out, nil
}

func qaReportFilePassed(path string) (bool, string) {
	var raw map[string]any
	if err := readJSON(path, &raw); err != nil {
		return false, filepath.Base(path) + " missing"
	}
	passed, _ := raw["passed"].(bool)
	if passed {
		return true, filepath.Base(path) + " passed=true"
	}
	if errorsValue, ok := raw["errors"]; ok {
		data, _ := json.Marshal(errorsValue)
		return false, filepath.Base(path) + " passed=false errors=" + truncateStatusError(string(data), 180)
	}
	return false, filepath.Base(path) + " passed=false"
}

type ProductionGapReport struct {
	Schema          string              `json:"schema"`
	Root            string              `json:"root"`
	ProductionReady bool                `json:"production_ready"`
	TopTierReady    bool                `json:"top_tier_ready"`
	QAProfile       string              `json:"qa_profile,omitempty"`
	QAPassed        bool                `json:"qa_passed"`
	ForceAverage    float64             `json:"force_average"`
	ForceTopTier    bool                `json:"force_top_tier"`
	FailedGates     []ProductionGapItem `json:"failed_gates,omitempty"`
	CategoryGaps    []ProductionGapItem `json:"category_gaps,omitempty"`
	NextActions     []string            `json:"next_actions,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
}

type ProductionGapItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	Evidence       string `json:"evidence,omitempty"`
	Recommendation string `json:"recommendation,omitempty"`
}

func productionGapCmd(args []string) {
	fs := flag.NewFlagSet("production-gap", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)

	report := buildProductionGapReport(*root)
	must(writeJSON(filepath.Join(*root, ".state", "production_gap_report.json"), report))
	must(os.WriteFile(filepath.Join(*root, ".state", "production_gap_report.md"), []byte(productionGapMarkdown(report)), 0644))
	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Print(productionGapMarkdown(report))
}

func buildProductionGapReport(root string) ProductionGapReport {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}
	out := ProductionGapReport{Schema: "qsm.production_gap_report.v1", Root: rootAbs, CreatedAt: time.Now().UTC()}
	var qa QAReport
	if readJSON(filepath.Join(root, ".state", "qa_report.json"), &qa) == nil && qa.Schema != "" {
		out.QAProfile = qa.Profile
		out.QAPassed = qa.Passed
		for _, gate := range qa.Gates {
			if gate.Required && gate.Status == "FAIL" {
				out.FailedGates = append(out.FailedGates, ProductionGapItem{
					ID:             gate.ID,
					Name:           gate.Name,
					Status:         gate.Status,
					Evidence:       gate.Evidence,
					Recommendation: gate.Recommendation,
				})
				if gate.Recommendation != "" {
					out.NextActions = append(out.NextActions, gate.Recommendation)
				}
			}
		}
	} else {
		out.FailedGates = append(out.FailedGates, ProductionGapItem{ID: "qa-report", Name: "Production QA report", Status: "FAIL", Evidence: "missing .state/qa_report.json", Recommendation: "Run qsm qa -profile production -sandbox docker."})
		out.NextActions = append(out.NextActions, "Run qsm qa -profile production -sandbox docker.")
	}
	var score requirements.ScoreReport
	if readJSON(filepath.Join(root, ".state", "force_score.json"), &score) == nil && score.Schema != "" {
		out.ForceAverage = score.AverageScore
		out.ForceTopTier = score.TopTier
		for _, category := range score.Checklist.Categories {
			if category.Status == "PASS" {
				continue
			}
			out.CategoryGaps = append(out.CategoryGaps, ProductionGapItem{
				ID:             strconv.Itoa(category.ID),
				Name:           category.Name,
				Status:         category.Status,
				Evidence:       category.JustificationEvidence,
				Recommendation: category.Recommendations,
			})
			if category.Recommendations != "" {
				out.NextActions = append(out.NextActions, category.Recommendations)
			}
		}
	} else {
		out.CategoryGaps = append(out.CategoryGaps, ProductionGapItem{ID: "force-score", Name: "Force requirements score", Status: "FAIL", Evidence: "missing .state/force_score.json", Recommendation: "Run qsm force-score or qsm qa -refresh=true."})
		out.NextActions = append(out.NextActions, "Run qsm force-score or qsm qa -refresh=true.")
	}
	out.NextActions = uniqueStrings(out.NextActions)
	out.ProductionReady = out.QAProfile == "production" && out.QAPassed && out.ForceAverage >= 8.5
	out.TopTierReady = out.QAProfile == "omni-alpha" && out.QAPassed && out.ForceAverage >= 9.5 && out.ForceTopTier
	return out
}

func productionGapMarkdown(report ProductionGapReport) string {
	var b strings.Builder
	b.WriteString("# QSM Production Gap Report\n\n")
	fmt.Fprintf(&b, "- Production ready: `%v`\n", report.ProductionReady)
	fmt.Fprintf(&b, "- Top-tier ready: `%v`\n", report.TopTierReady)
	fmt.Fprintf(&b, "- QA: profile=`%s` passed=`%v`\n", report.QAProfile, report.QAPassed)
	fmt.Fprintf(&b, "- Force score: `%.1f/10` top_tier=`%v`\n\n", report.ForceAverage, report.ForceTopTier)
	if len(report.FailedGates) > 0 {
		b.WriteString("## Failed Production Gates\n\n")
		for _, item := range report.FailedGates {
			fmt.Fprintf(&b, "- `%s` %s: %s\n", item.ID, item.Name, item.Evidence)
			if item.Recommendation != "" {
				fmt.Fprintf(&b, "  Recommendation: %s\n", item.Recommendation)
			}
		}
		b.WriteString("\n")
	}
	if len(report.CategoryGaps) > 0 {
		b.WriteString("## Force Checklist Gaps\n\n")
		for _, item := range report.CategoryGaps {
			fmt.Fprintf(&b, "- `%s` %s [%s]: %s\n", item.ID, item.Name, item.Status, item.Evidence)
			if item.Recommendation != "" {
				fmt.Fprintf(&b, "  Recommendation: %s\n", item.Recommendation)
			}
		}
		b.WriteString("\n")
	}
	if len(report.NextActions) > 0 {
		b.WriteString("## Next Actions\n\n")
		for _, action := range report.NextActions {
			b.WriteString("- " + action + "\n")
		}
	}
	return b.String()
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeQAProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "beta", "local-production", "production", "omni-alpha":
		return strings.ToLower(strings.TrimSpace(profile))
	default:
		return "alpha"
	}
}

func qaProfileRank(profile string) int {
	switch normalizeQAProfile(profile) {
	case "omni-alpha":
		return 4
	case "local-production":
		return 3
	case "production":
		return 3
	case "beta":
		return 2
	default:
		return 1
	}
}

func qaForceMinimum(profile string) float64 {
	switch normalizeQAProfile(profile) {
	case "omni-alpha":
		return 9.5
	case "production", "local-production":
		return 8.5
	case "beta":
		return 7.0
	default:
		return 5.0
	}
}

func addOmniAlphaGates(root string, out *QAReport, add func(id, name, status string, required bool, evidence, recommendation string), benchReport BenchmarkReport, forceScore requirements.ScoreReport, sandboxReport sandbox.Report) {
	if benchReport.Suite == "omni-contract" && len(benchReport.Tasks) == 6 && benchReport.PassedTasks == 6 && omniBenchmarkKindsComplete(benchReport) {
		add("omni-contract", "Omni contract benchmark", "PASS", true, "suite=omni-contract passed=6/6 kinds=complete", "")
	} else {
		add("omni-contract", "Omni contract benchmark", "FAIL", true, fmt.Sprintf("suite=%s passed=%d/%d kinds_complete=%v", benchReport.Suite, benchReport.PassedTasks, len(benchReport.Tasks), omniBenchmarkKindsComplete(benchReport)), "Run qsm benchmark -suite omni-contract -sandbox docker -image qsm-omni-sandbox:local.")
	}
	var self SelfImproveReport
	if readJSON(filepath.Join(root, ".state", "self_improvement_report.json"), &self) == nil && self.Schema != "" && self.Passed {
		add("self-improvement", "Self-improvement evidence", "PASS", true, fmt.Sprintf("suite=%s cycles=%d force_delta=%.2f repeated_failure_rate=%.2f", self.Suite, self.Cycles, self.ForceDelta, self.RepeatedFailureRate), "")
	} else if self.Schema != "" {
		add("self-improvement", "Self-improvement evidence", "FAIL", true, fmt.Sprintf("suite=%s cycles=%d passed=%v errors=%s", self.Suite, self.Cycles, self.Passed, truncateStatusError(strings.Join(self.Errors, "; "), 180)), "Run qsm self-improve -suite omni-contract -cycles 3 -sandbox docker.")
	} else {
		add("self-improvement", "Self-improvement evidence", "FAIL", true, "missing .state/self_improvement_report.json", "Run qsm self-improve -suite omni-contract -cycles 3 -sandbox docker.")
	}
	if strings.Contains(sandboxReport.ReadinessLevel, "omni") && sandboxReport.HardSandboxReady {
		add("omni-sandbox", "Omni sandbox runtime profile", "PASS", true, fmt.Sprintf("readiness=%s image=%s", sandboxReport.ReadinessLevel, sandboxReport.Policy.Image), "")
	} else {
		add("omni-sandbox", "Omni sandbox runtime profile", "FAIL", true, fmt.Sprintf("readiness=%s hard_ready=%v image=%s", sandboxReport.ReadinessLevel, sandboxReport.HardSandboxReady, sandboxReport.Policy.Image), "Run qsm sandbox -probe -backend docker -image qsm-omni-sandbox:local -profile omni.")
	}
	if forceScore.Schema != "" && forceScore.AverageScore >= 9.5 {
		add("force-omni-threshold", "Omni force score threshold", "PASS", true, fmt.Sprintf("average=%.1f min=9.5 top_tier=%v", forceScore.AverageScore, forceScore.TopTier), "")
	} else {
		add("force-omni-threshold", "Omni force score threshold", "FAIL", true, fmt.Sprintf("average=%.1f min=9.5 top_tier=%v", forceScore.AverageScore, forceScore.TopTier), "Close remaining force checklist gaps before claiming Omni-Creator Alpha.")
	}
	_ = out
}

func omniBenchmarkKindsComplete(report BenchmarkReport) bool {
	want := map[string]bool{
		"static-web":     false,
		"cli-tool":       false,
		"go-service":     false,
		"python-package": false,
		"node-fullstack": false,
		"data-transform": false,
	}
	for _, task := range report.Tasks {
		if _, ok := want[task.ProductKind]; ok && task.Passed {
			want[task.ProductKind] = true
		}
	}
	for _, ok := range want {
		if !ok {
			return false
		}
	}
	return true
}

func qaTestEvidence(report swarm.RunReport) (commands, passed, failed, reportCount, missingReports, weakReports int) {
	for _, result := range report.Results {
		if result.BuildPassed && result.TestPassed && result.LintPassed && result.TestReport == nil {
			missingReports++
			continue
		}
		if result.TestReport == nil {
			continue
		}
		reportCount++
		commands += result.TestReport.Summary.Commands
		passed += result.TestReport.Summary.PassedCommands
		failed += result.TestReport.Summary.FailedCommands
		if result.TestReport.ProductType != "generic" && result.TestReport.Summary.Commands == 0 {
			weakReports++
		}
		if !result.TestReport.Passed {
			failed++
		}
	}
	return commands, passed, failed, reportCount, missingReports, weakReports
}

func qaSecurityCounts(report swarm.RunReport) (critical, high, medium int) {
	for _, result := range report.Results {
		if result.TestReport == nil {
			continue
		}
		critical += result.TestReport.Security.CriticalCount
		high += result.TestReport.Security.HighCount
		medium += result.TestReport.Security.MediumCount
	}
	return critical, high, medium
}

func officialLikeBenchmark(report BenchmarkReport) bool {
	if report.Schema == "" || report.PassedTasks == 0 || len(report.Tasks) == 0 || report.PassedTasks != len(report.Tasks) {
		return false
	}
	suite := strings.ToLower(report.Suite)
	style := strings.ToLower(report.Style)
	if suite == "local-smoke" {
		return false
	}
	return strings.Contains(suite, "terminal-contract") ||
		strings.Contains(suite, "omni-contract") ||
		strings.Contains(suite, "terminal-bench") ||
		strings.Contains(suite, "swe-bench") ||
		strings.Contains(style, "official")
}

func qaMarkdown(report QAReport) string {
	var b strings.Builder
	b.WriteString("# QSM QA Report\n\n")
	fmt.Fprintf(&b, "- Profile: `%s`\n", report.Profile)
	fmt.Fprintf(&b, "- Sandbox: `%s`\n", report.SandboxBackend)
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	fmt.Fprintf(&b, "- Root: `%s`\n", report.Root)
	fmt.Fprintf(&b, "- Gates: `%d pass / %d fail / %d warn / %d skip`\n", report.Summary.Passed, report.Summary.Failed, report.Summary.Warned, report.Summary.Skipped)
	b.WriteString("\n## Gates\n\n")
	b.WriteString("| Gate | Status | Required | Evidence | Recommendation |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, gate := range report.Gates {
		fmt.Fprintf(&b, "| `%s` %s | %s | %v | %s | %s |\n",
			gate.ID, gate.Name, gate.Status, gate.Required, markdownCell(gate.Evidence), markdownCell(gate.Recommendation))
	}
	return b.String()
}

func markdownCell(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func sanitizeFileName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "item"
	}
	return out
}

func showCapacity(args []string) {
	fs := flag.NewFlagSet("capacity", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	plan := capacity.Estimate(capacity.LocalHardware(), capacity.DefaultProfile())
	if *jsonOut {
		data, err := json.MarshalIndent(plan, "", "  ")
		must(err)
		fmt.Println(string(data))
		return
	}
	fmt.Println(plan.Summary())
	for _, note := range plan.Notes {
		fmt.Println("- " + note)
	}
}

func synthesize(args []string) {
	fs := flag.NewFlagSet("synthesize", flag.ExitOnError)
	request := fs.String("request", "Build a tested auditable product", "high-level request")
	root := fs.String("root", ".", "workspace root")
	_ = fs.Parse(args)
	hypotheses, err := swarm.Synthesizer{Lake: mustOpen(*root)}.BrainDump(objective(*request), defaultAgents())
	must(err)
	fmt.Printf("captured %d zero-shot hypotheses\n", len(hypotheses))
}

func hydrate(args []string) {
	fs := flag.NewFlagSet("hydrate", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	_ = fs.Parse(args)
	count, err := research.Hydrator{Lake: mustOpen(*root)}.DigestLocal(*root)
	must(err)
	fmt.Printf("hydrated %d local files\n", count)
}

func compileWiki(args []string) {
	fs := flag.NewFlagSet("wiki", flag.ExitOnError)
	root := fs.String("root", ".", "workspace root")
	_ = fs.Parse(args)
	q := mustOpen(*root)
	artifacts, err := q.List()
	must(err)
	must(wiki.Compiler{OutDir: filepath.Join(*root, "internal", "wiki")}.Compile(artifacts))
	fmt.Printf("compiled wiki from %d artifacts\n", len(artifacts))
}

func resolvePositions(value string, plan capacity.Plan) int {
	if value == "" || value == "auto" {
		return plan.RecommendedPositions
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		log.Fatalf("invalid -positions value %q; use auto or a positive integer", value)
	}
	return n
}

func resolveParallel(value string, positions int, mode qruntime.HarnessMode) int {
	if value == "" || value == "auto" {
		if mode != qruntime.HarnessSimulated && positions > 2 {
			return 2
		}
		return positions
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		log.Fatalf("invalid -parallel value %q; use auto or a positive integer", value)
	}
	if n > positions {
		return positions
	}
	return n
}

func routerPIDPath(root string) string {
	return filepath.Join(root, ".state", "9router.pid")
}

func routerLogPath(root string) string {
	return filepath.Join(root, ".state", "9router.log")
}

func startRouterProcess(root string, rt qruntime.Config) (int, error) {
	if rt.NineRouterApp == "" {
		return 0, fmt.Errorf("9Router app path is not configured")
	}
	if _, err := os.Stat(filepath.Join(rt.NineRouterApp, "package.json")); err != nil {
		return 0, fmt.Errorf("9Router package missing at %s: %w", rt.NineRouterApp, err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".state"), 0755); err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(routerLogPath(root), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, err
	}
	defer logFile.Close()
	cmd := exec.Command("npm", "run", "dev")
	cmd.Dir = rt.NineRouterApp
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	if err := os.WriteFile(routerPIDPath(root), []byte(strconv.Itoa(pid)+"\n"), 0644); err != nil {
		return 0, err
	}
	return pid, nil
}

func routerLive(rt qruntime.Config) bool {
	if rt.NineRouterURL == "" {
		return false
	}
	url := strings.TrimRight(rt.NineRouterURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	if rt.NineRouterKey != "" {
		req.Header.Set("Authorization", "Bearer "+rt.NineRouterKey)
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func waitRouter(rt qruntime.Config, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for {
		if routerLive(rt) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(1500 * time.Millisecond)
	}
}

func readPID(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	if syscall.Kill(pid, 0) != nil {
		return 0
	}
	return pid
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func envDurationDefault(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil && seconds > 0 {
		return time.Duration(seconds * float64(time.Second))
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func bufioNewScanner(value string) *bufio.Scanner {
	return bufio.NewScanner(strings.NewReader(value))
}

func pathInside(root, candidate string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	candAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, candAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".." && !filepath.IsAbs(rel))
}

func productKind(result swarm.BranchResult) string {
	if result.TestReport != nil && strings.TrimSpace(result.TestReport.ProductType) != "" {
		return result.TestReport.ProductType
	}
	if fileExists(filepath.Join(result.ProductPath, "go.mod")) {
		return "go"
	}
	if fileExists(filepath.Join(result.ProductPath, "package.json")) {
		return "node"
	}
	if fileExists(filepath.Join(result.ProductPath, "index.html")) {
		return "static-web"
	}
	foundPython := false
	_ = filepath.WalkDir(result.ProductPath, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".py") {
			foundPython = true
		}
		return nil
	})
	if foundPython {
		return "python"
	}
	return "generic"
}

func pythonExecutableForMain() string {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func commandCWD(room, cwd string) string {
	if filepath.IsAbs(cwd) {
		return cwd
	}
	if strings.TrimSpace(cwd) == "" {
		return filepath.Join(room, "product")
	}
	return filepath.Join(room, filepath.Clean(cwd))
}

func allEqual(values []int, target int) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if value != target {
			return false
		}
	}
	return true
}

func noneEqual(values []int, target int) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if value == target {
			return false
		}
	}
	return true
}

func firstRequiredTestCommand(result swarm.BranchResult) (tester.CommandResult, bool) {
	if result.TestReport == nil {
		return tester.CommandResult{}, false
	}
	for _, cmd := range result.TestReport.Commands {
		if (cmd.Kind == "test" || cmd.Kind == "browser") && cmd.Origin == "manifest" {
			return cmd, true
		}
	}
	for _, cmd := range result.TestReport.Commands {
		if cmd.Kind == "test" || cmd.Kind == "browser" {
			return cmd, true
		}
	}
	return tester.CommandResult{}, false
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "__pycache__", ".qsm_test":
				if rel != "." {
					return filepath.SkipDir
				}
			}
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func applySimpleMutation(product string) (bool, string, error) {
	var targets []string
	_ = filepath.WalkDir(product, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".js", ".mjs", ".cjs", ".py", ".go":
			if mutationCandidateFile(entry.Name()) {
				targets = append(targets, path)
			}
		}
		return nil
	})
	if len(targets) == 0 {
		return false, "", nil
	}
	replacements := [][2]string{
		{"===", "!=="},
		{"!==", "==="},
		{"==", "!="},
		{"!=", "=="},
		{" >= ", " < "},
		{" <= ", " > "},
		{" true", " false"},
		{" false", " true"},
		{"return 1", "return 0"},
		{"return true", "return false"},
		{"return false", "return true"},
	}
	for _, target := range targets {
		data, err := os.ReadFile(target)
		if err != nil {
			return false, "", err
		}
		text := string(data)
		for _, pair := range replacements {
			if strings.Contains(text, pair[0]) {
				mutated := strings.Replace(text, pair[0], pair[1], 1)
				if mutated == text {
					continue
				}
				if err := os.WriteFile(target, []byte(mutated), 0644); err != nil {
					return false, "", err
				}
				rel, _ := filepath.Rel(product, target)
				return true, rel + ": " + strings.TrimSpace(pair[0]) + " -> " + strings.TrimSpace(pair[1]), nil
			}
		}
	}
	return false, "", nil
}

func mutationCandidateFile(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, "_test.go") ||
		strings.Contains(lower, "test") ||
		strings.Contains(lower, "spec") ||
		strings.HasPrefix(lower, "qa_") ||
		strings.Contains(lower, "smoke") ||
		strings.Contains(lower, "coverage") {
		return false
	}
	return true
}

func qualityMarkdown(report QualityReport) string {
	var b strings.Builder
	b.WriteString("# QSM " + strings.Title(report.Kind) + " Report\n\n")
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	fmt.Fprintf(&b, "- Sandbox: `%s`\n", report.Sandbox)
	fmt.Fprintf(&b, "- Items: `%d`\n", len(report.Items))
	if len(report.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, err := range report.Errors {
			b.WriteString("- " + err + "\n")
		}
	}
	if len(report.Warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, warning := range report.Warnings {
			b.WriteString("- " + warning + "\n")
		}
	}
	b.WriteString("\n## Items\n\n")
	b.WriteString("| Position | Type | Passed | Details |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, item := range report.Items {
		fmt.Fprintf(&b, "| %s | %s | %v | %s |\n", item.PositionID, item.Type, item.Passed, markdownCell(item.Details))
	}
	return b.String()
}

func gitOutput(root string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func getenvDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func selectHarness(rt qruntime.Config, timeout time.Duration) swarm.Harness {
	switch rt.HarnessMode {
	case qruntime.HarnessSimulated:
		return swarm.SimulatedHarness{}
	case qruntime.HarnessOpenCode:
		return swarm.OpenCodeHarness{Config: rt, Timeout: timeout}
	case qruntime.HarnessLangChain:
		return swarm.LangChainHarness{Config: rt, Timeout: timeout}
	default:
		log.Fatalf("unknown harness mode %q", rt.HarnessMode)
		return nil
	}
}

func configureOpenHarnessReference(rt qruntime.Config) {
	if strings.TrimSpace(os.Getenv("QSM_OPENHARNESS_PATH")) == "" && rt.OpenHarnessRoot != "" {
		_ = os.Setenv("QSM_OPENHARNESS_PATH", rt.OpenHarnessRoot)
	}
	if strings.TrimSpace(os.Getenv("QSM_OPENHARNESS_COMMIT")) == "" && rt.OpenHarnessRoot != "" {
		if commit := gitShortCommit(rt.OpenHarnessRoot); commit != "" {
			_ = os.Setenv("QSM_OPENHARNESS_COMMIT", commit)
		}
	}
}

func gitShortCommit(root string) string {
	if strings.TrimSpace(root) == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func mustOpen(root string) *lake.Lake {
	q, err := lake.Open(filepath.Join(root, ".lake"))
	must(err)
	must(os.MkdirAll(filepath.Join(root, ".state"), 0755))
	return q
}

func objective(request string) swarm.Objective {
	return swarm.Objective{
		ID:        fmt.Sprintf("obj-%d", time.Now().Unix()),
		Request:   request,
		CreatedAt: time.Now().UTC(),
		Metadata:  map[string]string{"spec": "V3.0"},
	}
}

func defaultAgents() []swarm.Agent {
	return []swarm.Agent{
		{ID: "alpha", Role: "architecture", Provider: "oc", Model: "ling-2.6-flash-free", Strengths: []string{"system design", "test strategy"}},
		{ID: "beta", Role: "implementation", Provider: "oc", Model: "minimax-m2.5-free", Strengths: []string{"simple code", "parallel build"}},
		{ID: "gamma", Role: "audit", Provider: "or", Model: "inclusionai/ling-2.6-1t:free", Strengths: []string{"contradiction hunting", "risk review"}},
	}
}

func routeHealthModelsFromFlag(value string, agents []swarm.Agent, rt qruntime.Config, limit int) []string {
	value = strings.TrimSpace(value)
	switch value {
	case "", "auto":
		if rt.HarnessMode == qruntime.HarnessLangChain && strings.TrimSpace(os.Getenv("QSM_LANGCHAIN_MODEL")) == "" {
			models, err := discoverRouterModels(rt, routeModelFree, limit)
			if err == nil {
				return models
			}
			fmt.Printf("route-health discovery failed, falling back to default agents: %v\n", err)
		}
		return routeHealthModels(agents, rt)
	case "free":
		models, err := discoverRouterModels(rt, routeModelFree, limit)
		if err != nil {
			fmt.Printf("route-health discovery failed, falling back to default agents: %v\n", err)
			return routeHealthModels(agents, rt)
		}
		return models
	case "all":
		models, err := discoverRouterModels(rt, routeModelAll, limit)
		if err != nil {
			fmt.Printf("route-health discovery failed, falling back to default agents: %v\n", err)
			return routeHealthModels(agents, rt)
		}
		return models
	default:
		return splitCSV(value)
	}
}

func routeHealthModels(agents []swarm.Agent, rt qruntime.Config) []string {
	if rt.HarnessMode == qruntime.HarnessLangChain {
		if override := strings.TrimSpace(os.Getenv("QSM_LANGCHAIN_MODEL")); override != "" {
			return []string{override}
		}
	}
	var models []string
	for _, agent := range agents {
		models = append(models, agentRoute(agent))
	}
	return models
}

type routeModelMode string

const (
	routeModelFree routeModelMode = "free"
	routeModelAll  routeModelMode = "all"
)

func discoverRouterModels(rt qruntime.Config, mode routeModelMode, limit int) ([]string, error) {
	list, err := qruntime.ListRouterModels(context.Background(), rt, 10*time.Second)
	if err != nil {
		return nil, err
	}
	var models []string
	for _, preferred := range preferredFreeRoutes() {
		if routerModelAllowed(preferred, "combo", mode) && routerModelExists(list, preferred) {
			models = appendUnique(models, preferred)
		}
	}
	for _, item := range list {
		if routerModelAllowed(item.ID, item.OwnedBy, mode) {
			models = appendUnique(models, item.ID)
		}
	}
	if limit > 0 && len(models) > limit {
		models = models[:limit]
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no route models discovered for mode %s", mode)
	}
	return models, nil
}

func preferredFreeRoutes() []string {
	return []string{
		"wombo",
		"oc/ling-2.6-flash-free",
		"oc/minimax-m2.5-free",
		"oc/nemotron-3-super-free",
		"openrouter/qwen/qwen3-coder:free",
		"openrouter/qwen/qwen3-next-80b-a3b-instruct:free",
		"openrouter/google/gemma-4-31b-it:free",
		"openrouter/nvidia/nemotron-3-super-120b-a12b:free",
		"openrouter/openrouter/free",
	}
}

func routerModelExists(models []qruntime.RouterModel, id string) bool {
	for _, item := range models {
		if item.ID == id {
			return true
		}
	}
	return false
}

func routerModelAllowed(id, owner string, mode routeModelMode) bool {
	if id == "" {
		return false
	}
	lower := strings.ToLower(id)
	if strings.Contains(lower, "embed") || strings.Contains(lower, "tts") || strings.Contains(lower, "image") {
		return false
	}
	if mode == routeModelAll {
		return true
	}
	if id == "wombo" || strings.HasPrefix(id, "oc/") {
		return true
	}
	if strings.HasPrefix(id, "openrouter/") && strings.Contains(id, ":free") {
		return true
	}
	if owner == "combo" {
		return true
	}
	return false
}

func appendUnique(items []string, item string) []string {
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func agentRoute(agent swarm.Agent) string {
	model := strings.TrimSpace(agent.Model)
	provider := strings.TrimSpace(agent.Provider)
	if provider == "" || model == "" || strings.HasPrefix(model, provider+"/") {
		return model
	}
	return provider + "/" + model
}

func splitCSV(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func healthyAgents(agents []swarm.Agent, rt qruntime.Config, results []qruntime.RouteHealthResult, buildHealth map[string]BuildHealthModel) []swarm.Agent {
	healthy := map[string]bool{}
	chatHealthy := map[string]bool{}
	var healthyRoutes []string
	for _, result := range results {
		if result.OK {
			chatHealthy[result.Model] = true
			if !buildHealthBlocksRoute(result.Model, buildHealth) {
				healthy[result.Model] = true
				healthyRoutes = append(healthyRoutes, result.Model)
			}
		}
	}
	sortHealthyRoutes(healthyRoutes, results, buildHealth)
	if rt.HarnessMode == qruntime.HarnessLangChain {
		if override := strings.TrimSpace(os.Getenv("QSM_LANGCHAIN_MODEL")); override != "" {
			if chatHealthy[override] {
				return agentsWithRoute(agents, override)
			}
			log.Fatalf("route-health rejected QSM_LANGCHAIN_MODEL=%s", override)
		}
	}
	var out []swarm.Agent
	used := map[string]bool{}
	for _, agent := range agents {
		route := agentRoute(agent)
		if healthy[route] {
			out = append(out, agent)
			used[route] = true
		}
	}
	roles := []swarm.Agent{
		{ID: "alpha", Role: "architecture", Strengths: []string{"system design", "test strategy"}},
		{ID: "beta", Role: "implementation", Strengths: []string{"simple code", "parallel build"}},
		{ID: "gamma", Role: "audit", Strengths: []string{"contradiction hunting", "risk review"}},
	}
	for _, route := range healthyRoutes {
		if used[route] {
			continue
		}
		base := roles[len(out)%len(roles)]
		base.ID = fmt.Sprintf("%s-route-%02d", base.ID, len(out)+1)
		base.Provider, base.Model = splitRoute(route)
		out = append(out, base)
		used[route] = true
	}
	if len(out) == 0 {
		log.Fatalf("route-health found no healthy routes for %s", rt.HarnessMode)
	}
	return out
}

func sortHealthyRoutes(routes []string, results []qruntime.RouteHealthResult, buildHealth map[string]BuildHealthModel) {
	routeResult := map[string]qruntime.RouteHealthResult{}
	for _, result := range results {
		routeResult[result.Model] = result
	}
	sort.SliceStable(routes, func(i, j int) bool {
		left := routeSelectionScore(routes[i], routeResult[routes[i]], buildHealth)
		right := routeSelectionScore(routes[j], routeResult[routes[j]], buildHealth)
		if left != right {
			return left > right
		}
		return routeResult[routes[i]].LatencyMS < routeResult[routes[j]].LatencyMS
	})
}

func routeSelectionScore(model string, result qruntime.RouteHealthResult, buildHealth map[string]BuildHealthModel) float64 {
	score := 0.5
	if result.OK {
		score += 1.0
	}
	if result.ContentOK {
		score += 0.5
	}
	if entry, ok := buildHealth[model]; ok && entry.Attempts > 0 {
		score += entry.SuccessRate * 2.0
		if entry.Attempts >= 2 {
			score += 0.25
		}
		if entry.LastState == string(swarm.RoomStateFailed) || strings.TrimSpace(entry.LastError) != "" {
			score -= 0.5
		}
	}
	if result.LatencyMS > 0 {
		score -= minFloat(float64(result.LatencyMS)/30000.0, 0.5)
	}
	return score
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func agentsWithRoute(agents []swarm.Agent, route string) []swarm.Agent {
	provider, model := splitRoute(route)
	out := make([]swarm.Agent, 0, len(agents))
	for _, agent := range agents {
		agent.Provider = provider
		agent.Model = model
		out = append(out, agent)
	}
	return out
}

func splitRoute(route string) (string, string) {
	parts := strings.SplitN(route, "/", 2)
	if len(parts) == 1 {
		return "", route
	}
	return parts[0], parts[1]
}

func countHealthyRoutes(results []qruntime.RouteHealthResult) int {
	count := 0
	for _, result := range results {
		if result.OK {
			count++
		}
	}
	return count
}

func writeRouteHealthState(root string, rt qruntime.Config, primaryURL string, results []qruntime.RouteHealthResult, fallbackUsed bool, fallbackModel string, buildHealth map[string]BuildHealthModel) error {
	return writeJSON(filepath.Join(root, ".state", "route_health.json"), RouteHealthState{
		HarnessMode:   rt.HarnessMode,
		NineRouterURL: rt.NineRouterURL,
		PrimaryURL:    primaryURL,
		EffectiveURL:  rt.NineRouterURL,
		FallbackUsed:  fallbackUsed,
		FallbackModel: fallbackModel,
		Models:        resultModels(results),
		Results:       results,
		BuildHealth:   buildHealth,
		CheckedAt:     time.Now().UTC(),
	})
}

func resultModels(results []qruntime.RouteHealthResult) []string {
	models := make([]string, 0, len(results))
	for _, result := range results {
		models = append(models, result.Model)
	}
	return models
}

func writeRouteHealthCache(q *lake.Lake, objectiveID string, results []qruntime.RouteHealthResult) {
	for _, result := range results {
		status := "failed"
		if result.OK {
			status = "ok"
		}
		content := fmt.Sprintf("%s: status=%s shape=%s latency=%dms", result.Model, status, result.ResponseShape, result.LatencyMS)
		if result.Error != "" {
			content += " error=" + result.Error
		}
		_, _ = q.PutCache(lake.CacheItem{
			Kind:        "route_health",
			ObjectiveID: objectiveID,
			Producer:    "runtime/route-health",
			Content:     content,
			Verified:    true,
			Confidence:  routeHealthConfidence(result),
			Metadata: map[string]string{
				"model":          result.Model,
				"status":         status,
				"response_shape": result.ResponseShape,
				"latency_ms":     strconv.FormatInt(result.LatencyMS, 10),
			},
		})
	}
}

func routeHealthConfidence(result qruntime.RouteHealthResult) float64 {
	if result.OK {
		return 0.95
	}
	return 0.8
}

func collectRoomStatuses(report swarm.RunReport) []swarm.RoomStatus {
	statuses := make([]swarm.RoomStatus, 0, len(report.Results))
	for _, result := range report.Results {
		if strings.TrimSpace(result.Room) == "" {
			continue
		}
		status, err := swarm.ReadRoomStatus(result.Room)
		if err == nil {
			statuses = append(statuses, status)
		}
	}
	return statuses
}

func truncateStatusError(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	if limit <= 16 {
		return value[:limit]
	}
	return value[:limit-15] + "...[truncated]"
}

func markdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return value
}

func tailTextFile(path string, maxLines int) string {
	if strings.TrimSpace(path) == "" || maxLines <= 0 {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "unable to read log tail: " + err.Error()
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func defaultDeepSeekKeyFile() string {
	nemoRoot := getenvDefault("QSM_NEMOCLAW_ROOT", "/Users/nexus/Downloads/NemoClaw")
	envPath := filepath.Join(nemoRoot, ".env")
	if fileExists(envPath) {
		return envPath
	}
	return filepath.Join(nemoRoot, "scripts", "Deepseek_keys")
}

func deepSeekFallbackRuntime(rt qruntime.Config, keyFile, model string) (qruntime.Config, string, error) {
	key, err := loadSecretKeyFile(keyFile)
	if err != nil {
		return rt, "", err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "deepseek-chat"
	}
	rt.NineRouterURL = getenvDefault("QSM_DEEPSEEK_BASE_URL", "https://api.deepseek.com")
	rt.NineRouterKey = key
	return rt, model, nil
}

func loadSecretKeyFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("key file path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var rawSK string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			name := strings.ToLower(strings.TrimSpace(parts[0]))
			value := cleanSecretValue(parts[1])
			if value == "" {
				continue
			}
			if strings.Contains(name, "deepseek") {
				return value, nil
			}
			if rawSK == "" && strings.HasPrefix(value, "sk-") {
				rawSK = value
			}
			continue
		}
		value := cleanSecretValue(line)
		if rawSK == "" && strings.HasPrefix(value, "sk-") {
			rawSK = value
		}
	}
	if rawSK != "" {
		return rawSK, nil
	}
	return "", fmt.Errorf("no DeepSeek-style key found in %s", path)
}

func cleanSecretValue(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	value = strings.Trim(value, `"'`)
	return strings.TrimSpace(value)
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

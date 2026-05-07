package requirements

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const ScoreSchema = "qsm.force_requirements_score.v1"

type Evidence struct {
	ObjectiveID            string    `json:"objective_id"`
	HarnessMode            string    `json:"harness_mode"`
	PlanApproved           bool      `json:"plan_approved"`
	PlanBlockers           int       `json:"plan_blockers"`
	RunPresent             bool      `json:"run_present"`
	RequestedNodes         int       `json:"requested_nodes"`
	StartedNodes           int       `json:"started_nodes"`
	SucceededNodes         int       `json:"succeeded_nodes"`
	FailedNodes            int       `json:"failed_nodes"`
	Concurrency            int       `json:"concurrency"`
	MaxRetries             int       `json:"max_retries"`
	AllNodesAccounted      bool      `json:"all_nodes_accounted"`
	CollapseApproved       bool      `json:"collapse_approved"`
	TestCommands           int       `json:"test_commands"`
	PassedTestCommands     int       `json:"passed_test_commands"`
	FailedTestCommands     int       `json:"failed_test_commands"`
	BrowserCommands        int       `json:"browser_commands"`
	SecurityCritical       int       `json:"security_critical"`
	SecurityHigh           int       `json:"security_high"`
	SecurityMedium         int       `json:"security_medium"`
	CompliancePresent      bool      `json:"compliance_present"`
	CompliancePassed       bool      `json:"compliance_passed"`
	SBOMGenerated          bool      `json:"sbom_generated"`
	SandboxReportPresent   bool      `json:"sandbox_report_present"`
	HardSandboxReady       bool      `json:"hard_sandbox_ready"`
	CacheItems             int       `json:"cache_items"`
	CostReportPresent      bool      `json:"cost_report_present"`
	EstimatedUSD           float64   `json:"estimated_usd"`
	EstimatedTokens        int       `json:"estimated_tokens"`
	RouteHealthFresh       bool      `json:"route_health_fresh"`
	HealthyRoutes          int       `json:"healthy_routes"`
	TotalRoutes            int       `json:"total_routes"`
	AutorunStatePresent    bool      `json:"autorun_state_present"`
	LocalPackageAuditOK    bool      `json:"local_package_audit_ok"`
	BenchmarkPresent       bool      `json:"benchmark_present"`
	BenchmarkPassedTasks   int       `json:"benchmark_passed_tasks"`
	BenchmarkTotalTasks    int       `json:"benchmark_total_tasks"`
	OfficialBenchmark      bool      `json:"official_benchmark"`
	TraceReportPresent     bool      `json:"trace_report_present"`
	TracePassed            bool      `json:"trace_passed"`
	CoveragePresent        bool      `json:"coverage_present"`
	CoveragePassed         bool      `json:"coverage_passed"`
	MutationPresent        bool      `json:"mutation_present"`
	MutationPassed         bool      `json:"mutation_passed"`
	FlakePresent           bool      `json:"flake_present"`
	FlakePassed            bool      `json:"flake_passed"`
	CostBudgetPresent      bool      `json:"cost_budget_present"`
	CostBudgetPassed       bool      `json:"cost_budget_passed"`
	CIReleasePresent       bool      `json:"ci_release_present"`
	CIReleasePassed        bool      `json:"ci_release_passed"`
	CIReleaseCI            bool      `json:"ci_release_ci"`
	CIReleaseLocalAllowed  bool      `json:"ci_release_local_allowed"`
	CIWorkflowPresent      bool      `json:"ci_workflow_present"`
	StressPresent          bool      `json:"stress_present"`
	StressPassed           bool      `json:"stress_passed"`
	StressNodes            int       `json:"stress_nodes"`
	StressParallel         int       `json:"stress_parallel"`
	LargeRepoFiles         int       `json:"large_repo_files"`
	LargeRepoPassed        bool      `json:"large_repo_passed"`
	RecoveryPresent        bool      `json:"recovery_present"`
	RecoveryPassed         bool      `json:"recovery_passed"`
	RecoveryRate           float64   `json:"recovery_rate"`
	ContributorPresent     bool      `json:"contributor_present"`
	ContributorPassed      bool      `json:"contributor_passed"`
	LaunchdPlistPresent    bool      `json:"launchd_plist_present"`
	OpsReadinessPresent    bool      `json:"ops_readiness_present"`
	OpsReadinessPassed     bool      `json:"ops_readiness_passed"`
	ApprovalGateReady      bool      `json:"approval_gate_ready"`
	CIArtifactRetention    bool      `json:"ci_artifact_retention"`
	RunbookPresent         bool      `json:"runbook_present"`
	SelfImprovePresent     bool      `json:"self_improve_present"`
	SelfImprovePassed      bool      `json:"self_improve_passed"`
	SelfImproveCycles      int       `json:"self_improve_cycles"`
	SelfImproveForceDelta  float64   `json:"self_improve_force_delta"`
	SelfImproveFailureRate float64   `json:"self_improve_repeated_failure_rate"`
	SelfImproveLessons     int       `json:"self_improve_lessons"`
	LakeReportPresent      bool      `json:"lake_report_present"`
	LakeEnterpriseReady    bool      `json:"lake_enterprise_ready"`
	LakeCacheCitation      float64   `json:"lake_cache_citation_coverage"`
	LakeWikiCitation       float64   `json:"lake_wiki_citation_coverage"`
	LakeDecisionCitation   float64   `json:"lake_decision_citation_coverage"`
	DataLakeAtomicWrites   bool      `json:"data_lake_atomic_writes"`
	CreatedAt              time.Time `json:"created_at"`
}

type ScoreReport struct {
	Schema        string    `json:"schema"`
	ObjectiveID   string    `json:"objective_id"`
	HarnessMode   string    `json:"harness_mode"`
	AverageScore  float64   `json:"average_score"`
	TopTier       bool      `json:"top_tier"`
	PassThreshold string    `json:"pass_threshold"`
	Evidence      Evidence  `json:"evidence"`
	Checklist     Checklist `json:"checklist"`
	CreatedAt     time.Time `json:"created_at"`
}

func Score(e Evidence) ScoreReport {
	now := time.Now().UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	checklist := Checklist{
		Schema:     "qsm.force_requirements.v1",
		StatusRule: "Evidence-scored status values: PASS, PARTIAL, LOCAL-GAP, GAP, FAIL. Top-tier requires average >= 9.0 and no category below 7.0.",
	}
	scores := map[int]float64{}
	add := func(category Category, status string, score float64, evidence, implementation, gaps, recommendations string) {
		checklist.Categories = append(checklist.Categories, ChecklistEntry{
			ID:                    category.ID,
			Name:                  category.Name,
			Status:                status,
			EvidenceScore:         score,
			Target:                category.Target,
			Checks:                append([]string(nil), category.Items...),
			JustificationEvidence: evidence,
			LocalImplementation:   implementation,
			GapsEnterprise:        gaps,
			Recommendations:       recommendations,
		})
		scores[category.ID] = score
	}
	categories := Categories()
	add(categories[0], scalabilityStatus(e), scalabilityScore(e), nodeEvidence(e), "Capacity planner, room isolation, configurable positions/concurrency, local stress testing, and 1000-file synthetic repo evidence.", "100+ concurrent long-running hosted sessions are still not proven.", "Run hosted high-concurrency CI/runner benchmarks before enterprise-scale claims.")
	add(categories[1], reliabilityStatus(e), reliabilityScore(e), planRunEvidence(e), "Planning gate, route-health gate, autorun state/logs, recovery evidence, launchd plist, and operational runbook.", "No hosted SLA, failover, or zero-downtime rollout proof yet.", "Add hosted CI/release evidence and later cloud failover drills.")
	add(categories[2], selfHealingStatus(e), selfHealingScore(e), retryEvidence(e), "Retries, shared cache, route-health/build-health avoidance, QSM-owned verifier, recovery report, and self-improvement cycles.", "Recovery is proven locally, not yet across long-running hosted failure campaigns.", "Run repeated 24/7 recovery campaigns and record recovery-rate trend lines.")
	add(categories[3], observabilityStatus(e), observabilityScore(e), observabilityEvidence(e), "Run reports, room status, test reports, route health, trace aggregation, cache summaries, cost reports, logs.", "No dashboard/exporter or cross-process distributed trace viewer yet.", "Add metrics endpoint and trace replay viewer.")
	add(categories[4], securityStatus(e), securityScore(e), securityEvidence(e), "Secret scan, dynamic execution scan, dependency audit gates, path-contained manifest commands, enforced Docker sandbox readiness reports, and local SBOM inventory.", "License choice, external compliance attestation, and microVM isolation remain future hardening steps.", "Choose an explicit license before external release and evaluate microVM isolation after Docker maturity.")
	add(categories[5], costStatus(e), costScore(e), costEvidence(e), "Token, cost, and budget reports estimate per-node/model spend; route-health and build-health prevent known-bad route spend.", "Provider-reported token usage is not available from every harness route yet.", "Capture exact provider usage for every model response and enforce cost ceilings per objective.")
	add(categories[6], maintainabilityStatus(e), maintainabilityScore(e), maintainabilityEvidence(e), "Modular Go packages, tests, docs, local package audit, coverage, flake, mutation, contributor smoke, and CI workflow evidence.", "Contributor smoke is local; hosted onboarding and artifact-retention proof depend on CI running.", "Keep contributor smoke in CI and publish retained artifacts for new maintainer review.")
	add(categories[7], usabilityStatus(e), usabilityScore(e), "CLI commands: plan, run, status, route-health, deploy, autorun.", "CLI-first human workflow with visible status and saved artifacts.", "No IDE/web UI, pause/resume approval UI, or accessibility review.", "Add human approval checkpoints and UI/IDE surfaces.")
	add(categories[8], operationsStatus(e), operationsScore(e), operationEvidence(e), "Autorun loop, per-cycle logs, managed 9Router deploy check, launchd plist, recovery evidence, approval gates, and CI workflow template.", "Hosted CI release evidence is still required; multi-tenancy, quotas, and PR automation are future extensions.", "Run the generated workflow in GitHub Actions and add approval-gated PR workflow after alpha.")
	add(categories[9], businessStatus(e), businessScore(e), businessEvidence(e), "Local product delivery is measurable through run/test/delivery/benchmark artifacts, omni-contract, self-improvement, and production-gap reporting.", "External official SWE-bench/Terminal-Bench and ROI evidence are not yet run.", "Run official benchmark adapters and track velocity/churn once omni-alpha CI is green.")

	total := 0.0
	minScore := 10.0
	for _, score := range scores {
		total += score
		if score < minScore {
			minScore = score
		}
	}
	avg := total / float64(len(scores))
	return ScoreReport{
		Schema:        ScoreSchema,
		ObjectiveID:   e.ObjectiveID,
		HarnessMode:   e.HarnessMode,
		AverageScore:  avg,
		TopTier:       avg >= 9.0 && minScore >= 7.0,
		PassThreshold: "top_tier requires average >= 9.0 and no category below 7.0; local product readiness may still pass below this threshold",
		Evidence:      e,
		Checklist:     checklist,
		CreatedAt:     now,
	}
}

func (r ScoreReport) JSON() string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "{}\n"
	}
	return string(data) + "\n"
}

func (r ScoreReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Force Requirements Evidence Score\n\n")
	fmt.Fprintf(&b, "- Objective: `%s`\n", r.ObjectiveID)
	fmt.Fprintf(&b, "- Harness: `%s`\n", r.HarnessMode)
	fmt.Fprintf(&b, "- Average score: %.1f/10\n", r.AverageScore)
	fmt.Fprintf(&b, "- Top-tier: %v\n\n", r.TopTier)
	for _, category := range r.Checklist.Categories {
		fmt.Fprintf(&b, "## %d. %s\n", category.ID, category.Name)
		fmt.Fprintf(&b, "- Status: %s\n", category.Status)
		fmt.Fprintf(&b, "- Evidence score: %.2f/10\n", category.EvidenceScore)
		fmt.Fprintf(&b, "- Evidence: %s\n", category.JustificationEvidence)
		fmt.Fprintf(&b, "- Local implementation: %s\n", category.LocalImplementation)
		fmt.Fprintf(&b, "- Enterprise gaps: %s\n", category.GapsEnterprise)
		fmt.Fprintf(&b, "- Recommendation: %s\n\n", category.Recommendations)
	}
	return b.String()
}

func statusFromScore(score float64) string {
	switch {
	case score >= 9:
		return "PASS"
	case score >= 6:
		return "PARTIAL"
	case score >= 3:
		return "LOCAL-GAP"
	default:
		return "GAP"
	}
}

func scalabilityScore(e Evidence) float64 {
	if !e.RunPresent {
		return 1
	}
	score := 3.0
	if e.RequestedNodes >= 7 {
		score += 1.5
	}
	if e.Concurrency >= 2 {
		score += 1
	}
	if e.AllNodesAccounted {
		score += 1
	}
	if e.BenchmarkPresent && e.BenchmarkTotalTasks > 0 {
		score += 0.75
	}
	if e.OfficialBenchmark {
		score += 0.75
	}
	if e.CIReleasePassed && e.CIReleaseCI {
		score += 0.25
	}
	if e.StressPassed {
		if e.StressNodes >= 7 {
			score += 1
		}
		if e.StressParallel >= 4 {
			score += 0.75
		}
		if e.LargeRepoPassed && e.LargeRepoFiles >= 1000 {
			score += 0.75
		}
	}
	return clamp(score, 0, 10)
}

func scalabilityStatus(e Evidence) string { return statusFromScore(scalabilityScore(e)) }

func reliabilityScore(e Evidence) float64 {
	score := 0.0
	if e.PlanApproved {
		score += 2
	}
	if e.RunPresent && e.AllNodesAccounted {
		score += 2
	}
	if e.AutorunStatePresent {
		score += 2
	}
	if e.RouteHealthFresh && e.HealthyRoutes > 0 {
		score += 1
	}
	if e.CollapseApproved {
		score += 1
	}
	if e.TracePassed {
		score += 0.75
	}
	if e.FlakePassed {
		score += 0.75
	}
	if e.CostBudgetPassed {
		score += 0.5
	}
	if e.RecoveryPassed {
		score += 0.75
	}
	if e.LaunchdPlistPresent {
		score += 0.5
	}
	if e.OpsReadinessPassed && e.RunbookPresent {
		score += 0.75
	}
	return clamp(score, 0, 9.5)
}

func reliabilityStatus(e Evidence) string { return statusFromScore(reliabilityScore(e)) }

func selfHealingScore(e Evidence) float64 {
	score := 0.0
	if e.MaxRetries > 0 {
		score += 2
	}
	if e.CacheItems > 0 {
		score += 1.5
	}
	if e.TestCommands > 0 && e.FailedTestCommands == 0 {
		score += 2
	}
	if e.TotalRoutes > 0 {
		score += 1
	}
	if e.FlakePassed {
		score += 0.75
	}
	if e.MutationPassed {
		score += 0.75
	}
	if e.LakeEnterpriseReady || e.LakeDecisionCitation >= 0.70 {
		score += 0.75
	}
	if e.RecoveryPassed {
		score += 1
	}
	if e.RecoveryRate >= 0.90 {
		score += 0.5
	}
	if e.SelfImprovePassed && e.SelfImproveCycles >= 3 && e.SelfImproveFailureRate == 0 {
		score += 0.5
	}
	return clamp(score, 0, 9.5)
}

func selfHealingStatus(e Evidence) string { return statusFromScore(selfHealingScore(e)) }

func observabilityScore(e Evidence) float64 {
	score := 0.0
	if e.RunPresent {
		score += 2
	}
	if e.AllNodesAccounted {
		score += 1.5
	}
	if e.TestCommands > 0 {
		score += 1.5
	}
	if e.RouteHealthFresh || e.TotalRoutes > 0 {
		score += 1
	}
	if e.AutorunStatePresent {
		score += 1
	}
	if e.CostReportPresent {
		score += 1
	}
	if e.TracePassed {
		score += 1
	}
	if e.CostBudgetPassed {
		score += 0.75
	}
	if e.LakeReportPresent {
		score += 0.75
	}
	if e.CIReleasePassed && e.CIReleaseCI {
		score += 0.5
	}
	return clamp(score, 0, 9.5)
}

func observabilityStatus(e Evidence) string { return statusFromScore(observabilityScore(e)) }

func securityScore(e Evidence) float64 {
	score := 3.0
	if e.SecurityCritical == 0 && e.SecurityHigh == 0 {
		score += 2
	}
	if e.TestCommands > 0 {
		score += 1
	}
	if e.LocalPackageAuditOK {
		score += 1
	}
	if e.SandboxReportPresent {
		score += 0.5
	}
	if e.HardSandboxReady {
		score += 1.5
	}
	if e.CIWorkflowPresent {
		score += 0.5
	}
	if e.CompliancePassed && e.SBOMGenerated {
		score += 1
	}
	if e.CIReleasePassed && e.CIReleaseCI {
		score += 0.25
	}
	if e.HardSandboxReady && e.CompliancePassed {
		return clamp(score, 0, 9.75)
	}
	if e.HardSandboxReady {
		return clamp(score, 0, 8.5)
	}
	return clamp(score, 0, 7.5)
}

func securityStatus(e Evidence) string {
	if e.SecurityCritical > 0 || e.SecurityHigh > 0 {
		return "FAIL"
	}
	return statusFromScore(securityScore(e))
}

func costScore(e Evidence) float64 {
	score := 2.0
	if e.HarnessMode == "simulated" {
		score += 2
	}
	if e.CostReportPresent {
		score += 2
	}
	if e.EstimatedTokens > 0 {
		score += 1
	}
	if e.TotalRoutes > 0 {
		score += 1
	}
	if e.HealthyRoutes > 0 {
		score += 1
	}
	if e.CostBudgetPassed {
		score += 1
	}
	if e.CIReleasePassed && e.CIReleaseCI {
		score += 0.25
	}
	return clamp(score, 0, 9.5)
}

func costStatus(e Evidence) string { return statusFromScore(costScore(e)) }

func maintainabilityScore(e Evidence) float64 {
	score := 4.0
	if e.LocalPackageAuditOK {
		score += 1
	}
	if e.TestCommands > 0 {
		score += 1
	}
	if e.CoveragePassed {
		score += 0.75
	}
	if e.FlakePassed {
		score += 0.75
	}
	if e.MutationPassed {
		score += 0.75
	}
	if e.CIWorkflowPresent {
		score += 0.5
	}
	if e.CIArtifactRetention {
		score += 0.25
	}
	if e.OpsReadinessPassed {
		score += 0.25
	}
	if e.ContributorPassed {
		score += 0.75
	}
	if e.SelfImprovePassed {
		score += 0.5
	}
	if e.CIReleasePassed && e.CIReleaseCI {
		score += 0.75
	}
	return clamp(score, 0, 9.75)
}

func maintainabilityStatus(e Evidence) string { return statusFromScore(maintainabilityScore(e)) }

func usabilityScore(e Evidence) float64 {
	score := 4.0
	if e.PlanApproved {
		score += 1
	}
	if e.AutorunStatePresent {
		score += 1
	}
	if e.TraceReportPresent {
		score += 0.5
	}
	if e.BenchmarkPresent {
		score += 0.5
	}
	if e.ContributorPassed {
		score += 0.5
	}
	if e.ApprovalGateReady {
		score += 1
	}
	if e.OpsReadinessPassed {
		score += 0.75
	}
	if e.SelfImprovePassed {
		score += 0.5
	}
	if e.CIReleasePassed && e.CIReleaseCI {
		score += 0.5
	}
	return clamp(score, 0, 9.5)
}

func usabilityStatus(e Evidence) string { return statusFromScore(usabilityScore(e)) }

func operationsScore(e Evidence) float64 {
	score := 1.0
	if e.AutorunStatePresent {
		score += 3
	}
	if e.RouteHealthFresh {
		score += 1
	}
	if e.RunPresent && e.AllNodesAccounted {
		score += 1
	}
	if e.TracePassed {
		score += 1
	}
	if e.CIReleasePresent {
		score += 0.75
	}
	if e.CIReleasePassed && e.CIReleaseCI {
		score += 1.25
	}
	if e.LaunchdPlistPresent {
		score += 0.75
	}
	if e.RecoveryPassed {
		score += 0.5
	}
	if e.OpsReadinessPassed {
		score += 0.75
	}
	if e.ApprovalGateReady {
		score += 0.5
	}
	if e.CIArtifactRetention {
		score += 0.25
	}
	return clamp(score, 0, 9.5)
}

func operationsStatus(e Evidence) string { return statusFromScore(operationsScore(e)) }

func businessScore(e Evidence) float64 {
	score := 1.0
	if e.CollapseApproved && e.PassedTestCommands == e.TestCommands && e.TestCommands > 0 {
		score += 2
	}
	if e.BenchmarkPresent {
		score += 2
	}
	if e.BenchmarkTotalTasks > 0 && e.BenchmarkPassedTasks == e.BenchmarkTotalTasks {
		score += 1
	}
	if e.OfficialBenchmark {
		score += 1
	}
	if e.LakeEnterpriseReady {
		score += 0.75
	}
	if e.CostBudgetPassed {
		score += 0.5
	}
	if e.StressPassed {
		score += 0.5
	}
	if e.OpsReadinessPassed && e.OfficialBenchmark {
		score += 0.5
	}
	if e.LargeRepoPassed && e.LargeRepoFiles >= 1000 {
		score += 0.25
	}
	if e.SelfImprovePassed {
		score += 0.5
	}
	if e.CIReleasePassed && e.CIReleaseCI {
		score += 0.75
	}
	return clamp(score, 0, 10)
}

func businessStatus(e Evidence) string { return statusFromScore(businessScore(e)) }

func nodeEvidence(e Evidence) string {
	return fmt.Sprintf("requested=%d started=%d succeeded=%d failed=%d concurrency=%d accounted=%v stress=%v/%v nodes=%d parallel=%d large_repo=%v files=%d", e.RequestedNodes, e.StartedNodes, e.SucceededNodes, e.FailedNodes, e.Concurrency, e.AllNodesAccounted, e.StressPresent, e.StressPassed, e.StressNodes, e.StressParallel, e.LargeRepoPassed, e.LargeRepoFiles)
}

func planRunEvidence(e Evidence) string {
	return fmt.Sprintf("plan_approved=%v plan_blockers=%d run_present=%v collapse_approved=%v autorun_state=%v launchd_plist=%v recovery=%v/%v ops=%v/%v runbook=%v", e.PlanApproved, e.PlanBlockers, e.RunPresent, e.CollapseApproved, e.AutorunStatePresent, e.LaunchdPlistPresent, e.RecoveryPresent, e.RecoveryPassed, e.OpsReadinessPresent, e.OpsReadinessPassed, e.RunbookPresent)
}

func retryEvidence(e Evidence) string {
	return fmt.Sprintf("max_retries=%d cache_items=%d test_commands=%d/%d route_health=%d/%d recovery=%v/%v rate=%.0f%%", e.MaxRetries, e.CacheItems, e.PassedTestCommands, e.TestCommands, e.HealthyRoutes, e.TotalRoutes, e.RecoveryPresent, e.RecoveryPassed, e.RecoveryRate*100)
}

func observabilityEvidence(e Evidence) string {
	return fmt.Sprintf("run_report=%v all_nodes_accounted=%v test_commands=%d trace=%v/%v cost_report=%v cost_budget=%v/%v lake=%v decision_citations=%.0f%% security=C%d/H%d/M%d ci=%v/%v", e.RunPresent, e.AllNodesAccounted, e.TestCommands, e.TraceReportPresent, e.TracePassed, e.CostReportPresent, e.CostBudgetPresent, e.CostBudgetPassed, e.LakeReportPresent, e.LakeDecisionCitation*100, e.SecurityCritical, e.SecurityHigh, e.SecurityMedium, e.CIReleasePassed, e.CIReleaseCI)
}

func securityEvidence(e Evidence) string {
	return fmt.Sprintf("security_findings=C%d/H%d/M%d local_package_audit_ok=%v sandbox_report=%v hard_sandbox=%v compliance=%v/%v sbom=%v", e.SecurityCritical, e.SecurityHigh, e.SecurityMedium, e.LocalPackageAuditOK, e.SandboxReportPresent, e.HardSandboxReady, e.CompliancePresent, e.CompliancePassed, e.SBOMGenerated)
}

func routeEvidence(e Evidence) string {
	return fmt.Sprintf("harness=%s route_health_fresh=%v healthy_routes=%d/%d", e.HarnessMode, e.RouteHealthFresh, e.HealthyRoutes, e.TotalRoutes)
}

func costEvidence(e Evidence) string {
	return fmt.Sprintf("cost_report=%v budget=%v/%v estimated_tokens=%d estimated_usd=%.6f route_health=%d/%d", e.CostReportPresent, e.CostBudgetPresent, e.CostBudgetPassed, e.EstimatedTokens, e.EstimatedUSD, e.HealthyRoutes, e.TotalRoutes)
}

func maintainabilityEvidence(e Evidence) string {
	return fmt.Sprintf("local_package_audit_ok=%v tests=%d coverage=%v/%v flake=%v/%v mutation=%v/%v contributor=%v/%v ci_workflow=%v retention=%v ops=%v/%v self_improve=%v/%v cycles=%d", e.LocalPackageAuditOK, e.TestCommands, e.CoveragePresent, e.CoveragePassed, e.FlakePresent, e.FlakePassed, e.MutationPresent, e.MutationPassed, e.ContributorPresent, e.ContributorPassed, e.CIWorkflowPresent, e.CIArtifactRetention, e.OpsReadinessPresent, e.OpsReadinessPassed, e.SelfImprovePresent, e.SelfImprovePassed, e.SelfImproveCycles)
}

func operationEvidence(e Evidence) string {
	return fmt.Sprintf("autorun_state=%v launchd_plist=%v route_health_fresh=%v all_nodes_accounted=%v trace=%v/%v recovery=%v/%v ci_release=%v/%v ci=%v local_allowed=%v ops=%v/%v approval=%v retention=%v", e.AutorunStatePresent, e.LaunchdPlistPresent, e.RouteHealthFresh, e.AllNodesAccounted, e.TraceReportPresent, e.TracePassed, e.RecoveryPresent, e.RecoveryPassed, e.CIReleasePresent, e.CIReleasePassed, e.CIReleaseCI, e.CIReleaseLocalAllowed, e.OpsReadinessPresent, e.OpsReadinessPassed, e.ApprovalGateReady, e.CIArtifactRetention)
}

func businessEvidence(e Evidence) string {
	return fmt.Sprintf("collapse_approved=%v tests=%d/%d benchmark=%v official=%v passed=%d/%d lake_enterprise=%v cache_citations=%.0f%% cost_budget=%v ops=%v/%v large_repo=%v files=%d self_improve=%v/%v delta=%.2f ci=%v/%v", e.CollapseApproved, e.PassedTestCommands, e.TestCommands, e.BenchmarkPresent, e.OfficialBenchmark, e.BenchmarkPassedTasks, e.BenchmarkTotalTasks, e.LakeEnterpriseReady, e.LakeCacheCitation*100, e.CostBudgetPassed, e.OpsReadinessPresent, e.OpsReadinessPassed, e.LargeRepoPassed, e.LargeRepoFiles, e.SelfImprovePresent, e.SelfImprovePassed, e.SelfImproveForceDelta, e.CIReleasePassed, e.CIReleaseCI)
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

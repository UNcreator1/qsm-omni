package planning

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	"github.com/nemoclaws/quantum-swarm-v3/internal/requirements"
	qruntime "github.com/nemoclaws/quantum-swarm-v3/internal/runtime"
	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
)

type Status string

const (
	StatusFresh      Status = "fresh"
	StatusStale      Status = "stale"
	StatusUnverified Status = "unverified"
	StatusBlocked    Status = "blocked"
)

type Material struct {
	Name                   string    `json:"name"`
	Purpose                string    `json:"purpose"`
	Criticality            string    `json:"criticality"`
	LocalVersion           string    `json:"local_version,omitempty"`
	VerificationSource     string    `json:"verification_source"`
	CheckedAt              time.Time `json:"checked_at"`
	FreshnessStatus        Status    `json:"freshness_status"`
	AvailabilityStatus     string    `json:"availability_status"`
	CompatibilityNotes     string    `json:"compatibility_notes,omitempty"`
	SecurityNotes          string    `json:"security_notes,omitempty"`
	AlternativesConsidered []string  `json:"alternatives_considered,omitempty"`
	SelectionReason        string    `json:"selection_reason,omitempty"`
	BlockerReason          string    `json:"blocker_reason,omitempty"`
}

type ArtifactRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type Report struct {
	Schema         string        `json:"schema"`
	ObjectiveID    string        `json:"objective_id"`
	Request        string        `json:"request"`
	HarnessMode    string        `json:"harness_mode"`
	Approved       bool          `json:"approved"`
	Blockers       []string      `json:"blockers,omitempty"`
	Warnings       []string      `json:"warnings,omitempty"`
	Materials      []Material    `json:"materials"`
	Artifacts      []ArtifactRef `json:"artifacts"`
	AllowedProfile string        `json:"allowed_profile"`
	CreatedAt      time.Time     `json:"created_at"`
}

func Generate(root string, q *lake.Lake, obj swarm.Objective, rt qruntime.Config) (Report, error) {
	now := time.Now().UTC()
	report := Report{
		Schema:         "qsm.plan_report.v1",
		ObjectiveID:    obj.ID,
		Request:        obj.Request,
		HarnessMode:    string(rt.HarnessMode),
		Approved:       true,
		AllowedProfile: "local-dev",
		CreatedAt:      now,
	}
	report.Materials = materialChecks(root, rt, now)
	for _, material := range report.Materials {
		if material.Criticality == "critical" && (material.FreshnessStatus == StatusBlocked || material.FreshnessStatus == StatusUnverified || material.AvailabilityStatus != "available") {
			report.Blockers = append(report.Blockers, fmt.Sprintf("%s: %s", material.Name, material.BlockerReason))
		}
		if material.Criticality != "critical" && material.AvailabilityStatus != "available" {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s unavailable: %s", material.Name, material.BlockerReason))
		}
	}
	if rt.HarnessMode != qruntime.HarnessSimulated && !freshHealthyRouteState(root, rt.HarnessMode, now) {
		report.Blockers = append(report.Blockers, "real harness requires fresh route-health with at least one healthy route")
	}
	report.Approved = len(report.Blockers) == 0
	if q != nil {
		refs, err := writeArtifacts(q, report)
		if err != nil {
			return report, err
		}
		report.Artifacts = refs
	}
	return report, nil
}

func materialChecks(root string, rt qruntime.Config, now time.Time) []Material {
	var materials []Material
	addCommand := func(name, purpose, criticality string, command []string, alternatives []string) {
		materials = append(materials, checkCommand(name, purpose, criticality, command, alternatives, now))
	}
	addCommand("go", "Build and test the QSM Go CLI and Go products.", "critical", []string{"go", "version"}, []string{"system Go toolchain"})
	addCommand("node", "Run JavaScript syntax checks and static-web product verification.", "critical", []string{"node", "--version"}, []string{"bun", "deno"})
	addCommand("python", "Compile and run the LangChain/DeepAgents harness.", "critical", []string{pythonName(), "--version"}, []string{"python3", "uv"})
	addCommand("playwright-browser", "Run browser smoke tests for static web products.", "optional", []string{"node", "-e", playwrightProbe()}, []string{"manual browser QA", "plain JS syntax checks"})
	addCommand("git", "Inspect local repository state and support delivery/version workflows.", "critical", []string{"git", "--version"}, []string{"filesystem snapshot"})
	materials = append(materials, harnessMaterial(root, rt, now)...)
	return materials
}

func checkCommand(name, purpose, criticality string, command []string, alternatives []string, now time.Time) Material {
	m := Material{
		Name:                   name,
		Purpose:                purpose,
		Criticality:            criticality,
		VerificationSource:     strings.Join(command, " "),
		CheckedAt:              now,
		FreshnessStatus:        StatusFresh,
		AvailabilityStatus:     "available",
		AlternativesConsidered: alternatives,
		SelectionReason:        "Selected because it is the current local tool used by QSM verification.",
	}
	if len(command) == 0 || command[0] == "" {
		m.FreshnessStatus = StatusBlocked
		m.AvailabilityStatus = "missing"
		m.BlockerReason = "command is not configured"
		return m
	}
	if _, err := exec.LookPath(command[0]); err != nil {
		m.FreshnessStatus = StatusBlocked
		m.AvailabilityStatus = "missing"
		m.BlockerReason = err.Error()
		if criticality != "critical" {
			m.FreshnessStatus = StatusUnverified
		}
		return m
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	m.LocalVersion = strings.TrimSpace(string(out))
	if err != nil {
		m.AvailabilityStatus = "unknown"
		m.FreshnessStatus = StatusUnverified
		m.BlockerReason = strings.TrimSpace(err.Error() + " " + string(out))
	}
	return m
}

func harnessMaterial(root string, rt qruntime.Config, now time.Time) []Material {
	switch rt.HarnessMode {
	case qruntime.HarnessSimulated:
		return []Material{{
			Name:               "simulated-harness",
			Purpose:            "Deterministic local fixture harness for development smoke tests.",
			Criticality:        "critical",
			VerificationSource: "internal/swarm.SimulatedHarness",
			CheckedAt:          now,
			FreshnessStatus:    StatusFresh,
			AvailabilityStatus: "available",
			SelectionReason:    "Selected because this run uses simulated harness mode.",
		}}
	case qruntime.HarnessLangChain:
		return []Material{
			fileMaterial("langchain-runner", "Run DeepAgents/LangChain node loop.", "critical", rt.LangChainRunner, now),
			fileMaterial("wiki-memory", "Provide compiled shared memory to nodes.", "critical", rt.WikiPath, now),
		}
	case qruntime.HarnessOpenCode:
		return []Material{
			fileMaterial("opencode-cli", "Run OpenCode agent CLI.", "critical", rt.OpenCodePath, now),
			fileMaterial("opencode-config", "Configure OpenCode/9Router API access.", "critical", rt.OpenCodeConfig, now),
			fileMaterial("wiki-memory", "Provide compiled shared memory to nodes.", "critical", rt.WikiPath, now),
		}
	default:
		return []Material{{
			Name:               "harness-mode",
			Purpose:            "Select execution harness.",
			Criticality:        "critical",
			VerificationSource: string(rt.HarnessMode),
			CheckedAt:          now,
			FreshnessStatus:    StatusBlocked,
			AvailabilityStatus: "unknown",
			BlockerReason:      "unknown harness mode",
		}}
	}
}

func fileMaterial(name, purpose, criticality, path string, now time.Time) Material {
	m := Material{
		Name:               name,
		Purpose:            purpose,
		Criticality:        criticality,
		VerificationSource: path,
		CheckedAt:          now,
		FreshnessStatus:    StatusFresh,
		AvailabilityStatus: "available",
		SelectionReason:    "Selected from QSM runtime configuration.",
	}
	if strings.TrimSpace(path) == "" {
		m.FreshnessStatus = StatusBlocked
		m.AvailabilityStatus = "missing"
		m.BlockerReason = "path is not configured"
		return m
	}
	info, err := os.Stat(path)
	if err != nil {
		m.FreshnessStatus = StatusBlocked
		m.AvailabilityStatus = "missing"
		m.BlockerReason = err.Error()
		return m
	}
	m.LocalVersion = fmt.Sprintf("exists size=%d mode=%s", info.Size(), info.Mode())
	return m
}

func writeArtifacts(q *lake.Lake, report Report) ([]ArtifactRef, error) {
	type spec struct {
		kind       string
		claim      string
		content    string
		confidence float64
		verified   bool
	}
	artifacts := []spec{
		{"objective_contract", "Objective contract captured before Chop.", objectiveContract(report), 0.9, true},
		{"requirements_inventory", "Functional and non-functional requirements inventory captured.", requirementsInventory(report), 0.75, true},
		{"materials_inventory", "Build materials inventory captured.", materialsInventory(report), 0.9, true},
		{"materials_quality_audit", "Material quality and suitability audit captured.", materialsQualityAudit(report), 0.85, report.Approved},
		{"resource_freshness_report", "Resource freshness and availability report captured.", resourceFreshnessReport(report), 0.85, report.Approved},
		{"risk_register", "Planning risk register captured.", riskRegister(report), 0.75, true},
		{"test_strategy", "QSM-owned test strategy captured before build.", testStrategy(report), 0.8, true},
		{"cost_plan", "Initial cost and route plan captured.", costPlan(report), 0.65, true},
		{"force_requirements_baseline", "Force Requirements baseline initialized.", requirements.JSONTemplate(), 0.8, true},
		{"chop_readiness_verdict", "Chop readiness verdict captured.", chopVerdict(report), 1.0, report.Approved},
	}
	refs := make([]ArtifactRef, 0, len(artifacts))
	for _, item := range artifacts {
		artifact, err := q.Put(lake.Artifact{
			Phase:      lake.PhaseSynthesis,
			Kind:       item.kind,
			Source:     "qsm-planning",
			Claim:      item.claim,
			Content:    item.content,
			Confidence: item.confidence,
			Verified:   item.verified,
			Metadata: map[string]string{
				"objective_id": report.ObjectiveID,
				"harness":      report.HarnessMode,
			},
		})
		if err != nil {
			return refs, err
		}
		refs = append(refs, ArtifactRef{Kind: item.kind, ID: artifact.ID})
	}
	return refs, nil
}

func objectiveContract(report Report) string {
	return fmt.Sprintf("request: %s\nscope: build requested deliverable\nnon_goals: do not claim production readiness without force evidence\nexpected_deliverable: inferred by build nodes\nacceptance: QSM product verification, auto-test report, evidence, force checklist artifacts", report.Request)
}

func requirementsInventory(report Report) string {
	return "functional: deliver requested product\nnon_functional: deterministic evidence, room isolation, force checklist, no unsupported production claims\nforce_categories: all 10 initialized"
}

func materialsInventory(report Report) string {
	data, _ := json.MarshalIndent(report.Materials, "", "  ")
	return string(data)
}

func materialsQualityAudit(report Report) string {
	var b strings.Builder
	for _, m := range report.Materials {
		fmt.Fprintf(&b, "- %s: criticality=%s freshness=%s availability=%s source=%s reason=%s blocker=%s\n", m.Name, m.Criticality, m.FreshnessStatus, m.AvailabilityStatus, m.VerificationSource, m.SelectionReason, m.BlockerReason)
	}
	return b.String()
}

func resourceFreshnessReport(report Report) string {
	var b strings.Builder
	for _, m := range report.Materials {
		fmt.Fprintf(&b, "%s checked_at=%s status=%s availability=%s version=%q\n", m.Name, m.CheckedAt.Format(time.RFC3339), m.FreshnessStatus, m.AvailabilityStatus, truncate(m.LocalVersion, 160))
	}
	return b.String()
}

func riskRegister(report Report) string {
	risks := []string{"model/API route limits can block real harnesses", "browser and security tools may be missing locally", "path-based isolation is not a hard sandbox"}
	if len(report.Blockers) > 0 {
		risks = append(risks, "planning blockers: "+strings.Join(report.Blockers, "; "))
	}
	return strings.Join(risks, "\n")
}

func testStrategy(report Report) string {
	return "QSM-owned checks: product verifier, test report, syntax/test commands by product type, force checklist validation. Node-authored tests are advisory only."
}

func costPlan(report Report) string {
	return fmt.Sprintf("harness=%s\nprofile=%s\npolicy=use route-health/build-health before real model spend; simulated runs have no model token cost", report.HarnessMode, report.AllowedProfile)
}

func chopVerdict(report Report) string {
	data, _ := json.MarshalIndent(map[string]any{
		"approved":        report.Approved,
		"blockers":        report.Blockers,
		"warnings":        report.Warnings,
		"harness_mode":    report.HarnessMode,
		"objective_id":    report.ObjectiveID,
		"allowed_profile": report.AllowedProfile,
	}, "", "  ")
	return string(data)
}

func freshHealthyRouteState(root string, mode qruntime.HarnessMode, now time.Time) bool {
	data, err := os.ReadFile(filepath.Join(root, ".state", "route_health.json"))
	if err != nil {
		return false
	}
	var raw struct {
		HarnessMode string    `json:"harness_mode"`
		CheckedAt   time.Time `json:"checked_at"`
		Results     []struct {
			OK bool `json:"ok"`
		} `json:"results"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return false
	}
	if raw.HarnessMode != "" && raw.HarnessMode != string(mode) {
		return false
	}
	if raw.CheckedAt.IsZero() || now.Sub(raw.CheckedAt) > 30*time.Minute {
		return false
	}
	for _, result := range raw.Results {
		if result.OK {
			return true
		}
	}
	return false
}

func pythonName() string {
	if _, err := exec.LookPath("python3"); err == nil {
		return "python3"
	}
	return "python"
}

func playwrightProbe() string {
	return `const fs=require("fs");let pw;try{pw=require("playwright")}catch(e){process.exit(1)}const exe=pw.chromium&&pw.chromium.executablePath&&pw.chromium.executablePath();process.exit(exe&&fs.existsSync(exe)?0:1)`
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}

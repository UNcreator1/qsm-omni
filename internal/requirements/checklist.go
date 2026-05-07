package requirements

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Category struct {
	ID     int
	Name   string
	Target string
	Items  []string
}

type Checklist struct {
	Schema     string           `json:"schema"`
	StatusRule string           `json:"status_rule"`
	Categories []ChecklistEntry `json:"categories"`
}

type ChecklistEntry struct {
	ID                    int      `json:"id"`
	Name                  string   `json:"name"`
	Status                string   `json:"status"`
	EvidenceScore         float64  `json:"evidence_score,omitempty"`
	Target                string   `json:"target"`
	Checks                []string `json:"checks"`
	JustificationEvidence string   `json:"justification_evidence"`
	LocalImplementation   string   `json:"local_implementation"`
	GapsEnterprise        string   `json:"gaps_enterprise"`
	Recommendations       string   `json:"recommendations"`
}

var forceCategories = []Category{
	{ID: 1, Name: "Scalability & Performance", Target: "Handles enterprise codebases and 100+ concurrent long-running sessions with linear or sub-linear resource growth.", Items: []string{"100+ concurrent coding sessions", "massive context and 1000+ file repos", "standard issue latency under 5 minutes where measurable", "stateless modular deployment readiness"}},
	{ID: 2, Name: "Reliability, Availability & 24/7 Operation", Target: "Runs continuously with graceful degradation, disaster recovery, and no manual restart dependency.", Items: []string{"99.99% runtime target", "zero-downtime update path", "RPO/RTO story", "model/API limit degradation"}},
	{ID: 3, Name: "Self-Healing & Resilience", Target: "Detects, diagnoses, retries, and repairs failures with backoff and circuit breakers.", Items: []string{"failure detection loop", "auto-test and auto-fix before delivery", "back-pressure and exponential backoff", "self-recovery metric target"}},
	{ID: 4, Name: "Observability, Measurability & Countability", Target: "Every action is countable, replayable, and tied to metrics/logs/traces.", Items: []string{"tool-call and file-edit audit trail", "test and benchmark metrics", "token/cost/task accounting", "real-time status and alerting"}},
	{ID: 5, Name: "Security & Compliance", Target: "Zero-trust execution with secrets protection, sandboxing, scanning, and compliance readiness.", Items: []string{"auth/encryption/secrets handling", "sandboxed execution", "vulnerability and policy scanning", "SBOM/compliance path"}},
	{ID: 6, Name: "Cost-Effectiveness & Profitability", Target: "Tracks cost per task and ROI instead of only raw capability.", Items: []string{"token and model spend accounting", "ROI or cycle-time metric", "sub-linear cost scaling", "cost threshold per benchmark-style task"}},
	{ID: 7, Name: "Maintainability, Extensibility & Developer Experience", Target: "Modular, documented, testable, and easy for new maintainers.", Items: []string{"pluggable tools/memory/evaluators", "docs and CI templates", "unit/integration/trajectory tests", "new contributor path under 2 hours"}},
	{ID: 8, Name: "Usability, Accessibility & Portability", Target: "Works across CLI/IDE/web/cloud and supports pause/resume/human oversight.", Items: []string{"multi-interface support", "human-in-loop checkpoints", "multi-language support", "clear plan-act-reflect workflow"}},
	{ID: 9, Name: "Operational & Automation Excellence", Target: "Automates PR/review/merge workflows with durable state and admin controls.", Items: []string{"autonomous PR/review path", "approval gates", "multi-tenancy/quotas", "resume-after-crash"}},
	{ID: 10, Name: "Business & Strategic Readiness", Target: "Benchmarked, explainable, stable, and tied to measurable ROI.", Items: []string{"SWE-bench/Terminal-Bench or internal benchmark evidence", "community/adoption signal", "explainability hooks", "velocity/churn/defect ROI"}},
}

func Categories() []Category {
	out := make([]Category, len(forceCategories))
	copy(out, forceCategories)
	return out
}

func Prompt() string {
	return "MANDATORY FORCE REQUIREMENTS: Before any room output or collapse, keep FORCE_REQUIREMENTS_CHECKLIST.md and QSM_FORCE_CHECKLIST.json complete. Evaluate all 10 categories. Use GAP or N/A for anything beyond current local QSM evidence. Never claim top-tier or production-ready without evidence. Leverage QSM auto-test reports."
}

func FullPrompt() string {
	var b strings.Builder
	b.WriteString("FORCE REQUIREMENTS CHECKLIST - TOP OF BUILD:\n")
	b.WriteString("This is mandatory. Do not claim top-tier or production-ready unless every category is proven. For local or prototype builds, mark missing enterprise evidence as GAP/FAIL instead of pretending it passed. The product may still be delivered, but readiness claims must be honest.\n")
	for _, category := range forceCategories {
		b.WriteString("\n")
		b.WriteString(categoryLine(category))
	}
	b.WriteString("\nRequired artifact: update ./FORCE_REQUIREMENTS_CHECKLIST.md with PASS, FAIL, or GAP for each category and cite evidence paths or QSM test report paths. If evidence is unavailable, write GAP.\n")
	return b.String()
}

func JSONTemplate() string {
	checklist := Checklist{
		Schema:     "qsm.force_requirements.v1",
		StatusRule: "Allowed status values: PASS, FAIL, PARTIAL, N/A, LOCAL-GAP, GAP. Missing evidence is GAP. Do not claim top-tier or production-ready unless every mandatory category is PASS with evidence.",
	}
	for _, category := range forceCategories {
		checklist.Categories = append(checklist.Categories, ChecklistEntry{
			ID:                    category.ID,
			Name:                  category.Name,
			Status:                "GAP",
			Target:                category.Target,
			Checks:                append([]string(nil), category.Items...),
			JustificationEvidence: "pending",
			LocalImplementation:   "pending",
			GapsEnterprise:        "pending",
			Recommendations:       "pending",
		})
	}
	data, err := json.MarshalIndent(checklist, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data) + "\n"
}

func MarkdownTemplate() string {
	var b strings.Builder
	b.WriteString("# FORCE REQUIREMENTS CHECKLIST\n\n")
	b.WriteString("Status values: PASS, FAIL, GAP, or N/A with reason. Missing evidence is GAP. Do not claim top-tier or production-ready unless all mandatory categories are PASS with evidence.\n\n")
	for _, category := range forceCategories {
		b.WriteString("## ")
		b.WriteString(categoryLine(category))
		b.WriteString("\n")
		b.WriteString("- Status: GAP\n")
		b.WriteString("- Evidence: pending\n")
		b.WriteString("- Notes: pending\n\n")
	}
	return b.String()
}

func EnsureArtifacts(room string) error {
	if strings.TrimSpace(room) == "" {
		return nil
	}
	if err := os.MkdirAll(room, 0755); err != nil {
		return err
	}
	mdPath := filepath.Join(room, "FORCE_REQUIREMENTS_CHECKLIST.md")
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		if err := os.WriteFile(mdPath, []byte(MarkdownTemplate()), 0644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	jsonPath := filepath.Join(room, "QSM_FORCE_CHECKLIST.json")
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		if err := os.WriteFile(jsonPath, []byte(JSONTemplate()), 0644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return err
	}
	var checklist Checklist
	if err := json.Unmarshal(data, &checklist); err != nil {
		return fmt.Errorf("invalid QSM force checklist: %w", err)
	}
	if len(checklist.Categories) != 10 {
		return fmt.Errorf("QSM force checklist must contain 10 categories, got %d", len(checklist.Categories))
	}
	return nil
}

func categoryLine(category Category) string {
	var b strings.Builder
	b.WriteString(fmt.Sprint(category.ID))
	b.WriteString(". ")
	b.WriteString(category.Name)
	b.WriteString(" - ")
	b.WriteString(category.Target)
	if len(category.Items) > 0 {
		b.WriteString(" Checks: ")
		b.WriteString(strings.Join(category.Items, "; "))
		b.WriteString(".")
	}
	return b.String()
}

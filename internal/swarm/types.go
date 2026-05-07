package swarm

import (
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/grounding"
	"github.com/nemoclaws/quantum-swarm-v3/internal/tester"
)

type Agent struct {
	ID           string   `json:"id"`
	Role         string   `json:"role"`
	Model        string   `json:"model"`
	Provider     string   `json:"provider"`
	Strengths    []string `json:"strengths"`
	SystemPrompt string   `json:"system_prompt"`
}

type Objective struct {
	ID          string            `json:"id"`
	Request     string            `json:"request"`
	Constraints []string          `json:"constraints,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

type Hypothesis struct {
	AgentID      string   `json:"agent_id"`
	Blueprint    string   `json:"blueprint"`
	Assumptions  []string `json:"assumptions"`
	Risks        []string `json:"risks"`
	TestStrategy []string `json:"test_strategy"`
}

type Position struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Strategy    string   `json:"strategy"`
	Room        string   `json:"room"`
	SourceAgent string   `json:"source_agent,omitempty"`
	Tests       []string `json:"tests,omitempty"`
}

type BranchResult struct {
	PositionID   string               `json:"position_id"`
	AgentID      string               `json:"agent_id,omitempty"`
	AgentModel   string               `json:"agent_model,omitempty"`
	Room         string               `json:"room"`
	BuildPassed  bool                 `json:"build_passed"`
	TestPassed   bool                 `json:"test_passed"`
	LintPassed   bool                 `json:"lint_passed"`
	AuditPassed  bool                 `json:"audit_passed"`
	Score        float64              `json:"score"`
	EvidencePath string               `json:"evidence_path"`
	ProductPath  string               `json:"product_path,omitempty"`
	Error        string               `json:"error,omitempty"`
	Citations    []grounding.Citation `json:"citations,omitempty"`
	Verification *ProductVerification `json:"verification,omitempty"`
	TestReport   *tester.TestReport   `json:"test_report,omitempty"`
	Metadata     map[string]any       `json:"metadata,omitempty"`
	DurationMS   int64                `json:"duration_ms,omitempty"`
	Attempts     int                  `json:"attempts,omitempty"`
	CompletedAt  time.Time            `json:"completed_at"`
}

type RunReport struct {
	ObjectiveID       string         `json:"objective_id"`
	RequestedNodes    int            `json:"requested_nodes"`
	StartedNodes      int            `json:"started_nodes"`
	SucceededNodes    int            `json:"succeeded_nodes"`
	FailedNodes       int            `json:"failed_nodes"`
	Concurrency       int            `json:"concurrency"`
	MaxRetries        int            `json:"max_retries"`
	HarnessMode       string         `json:"harness_mode"`
	SandboxBackend    string         `json:"sandbox_backend,omitempty"`
	Results           []BranchResult `json:"results"`
	StartedAt         time.Time      `json:"started_at"`
	CompletedAt       time.Time      `json:"completed_at"`
	DurationMS        int64          `json:"duration_ms"`
	AllNodesAccounted bool           `json:"all_nodes_accounted"`
	CollapseEligible  bool           `json:"collapse_eligible"`
	CacheSummary      map[string]int `json:"cache_summary,omitempty"`
}

type AuditFinding struct {
	AuditorID  string `json:"auditor_id"`
	PositionID string `json:"position_id"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Approved   bool   `json:"approved"`
}

type ProductVerification struct {
	Type     string   `json:"type"`
	Passed   bool     `json:"passed"`
	Checks   []string `json:"checks,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Errors   []string `json:"errors,omitempty"`
}

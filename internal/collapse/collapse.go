package collapse

import (
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	"github.com/nemoclaws/quantum-swarm-v3/internal/productmanifest"
	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
)

type ConsensusEngine struct {
	Lake *lake.Lake
}

type Verdict struct {
	Winner       swarm.BranchResult   `json:"winner"`
	Approved     bool                 `json:"approved"`
	Reason       string               `json:"reason"`
	Ranked       []swarm.BranchResult `json:"ranked"`
	AuditSummary []swarm.AuditFinding `json:"audit_summary"`
}

func Audit(results []swarm.BranchResult) []swarm.AuditFinding {
	var findings []swarm.AuditFinding
	for _, r := range results {
		approved := r.BuildPassed && r.TestPassed && r.LintPassed
		severity := "none"
		msg := "branch has deterministic build, test, and lint evidence"
		if !approved {
			severity = "high"
			msg = "branch is missing deterministic evidence"
		}
		productApproved := approved && productExists(r.ProductPath)
		productSeverity := "none"
		productMsg := "branch produces a deliverable artifact"
		if r.Verification != nil {
			productApproved = approved && r.Verification.Passed
			productMsg = "branch product passed deterministic verifier"
			if !r.Verification.Passed && len(r.Verification.Errors) > 0 {
				productMsg = "branch product verifier failed: " + strings.Join(r.Verification.Errors, "; ")
			}
		}
		if !productApproved {
			productSeverity = "high"
			if r.Verification == nil {
				productMsg = "branch product path is missing or empty"
			}
		}
		manifestReport := productmanifest.Validate(r.ProductPath)
		manifestApproved := productApproved && manifestReport.Passed
		manifestSeverity := "none"
		manifestMsg := "branch declares a valid qsm_project_manifest.v1 contract"
		if !manifestApproved {
			manifestSeverity = "high"
			manifestMsg = "branch product manifest invalid: " + strings.Join(manifestReport.Errors, "; ")
			if len(manifestReport.Errors) == 0 {
				manifestMsg = "branch product manifest invalid"
			}
		}
		findings = append(findings,
			swarm.AuditFinding{AuditorID: "code", PositionID: r.PositionID, Severity: severity, Message: msg, Approved: approved},
			swarm.AuditFinding{AuditorID: "security", PositionID: r.PositionID, Severity: severity, Message: "no unsafe collapse action requested", Approved: approved},
			swarm.AuditFinding{AuditorID: "product", PositionID: r.PositionID, Severity: productSeverity, Message: productMsg, Approved: productApproved},
			swarm.AuditFinding{AuditorID: "product-manifest", PositionID: r.PositionID, Severity: manifestSeverity, Message: manifestMsg, Approved: manifestApproved},
		)
	}
	return findings
}

func productExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}

func (e ConsensusEngine) Collapse(results []swarm.BranchResult, findings []swarm.AuditFinding) (Verdict, error) {
	if len(results) == 0 {
		return Verdict{}, errors.New("no branch results to collapse")
	}
	approval := map[string]bool{}
	for _, r := range results {
		approval[r.PositionID] = true
	}
	for _, f := range findings {
		if !f.Approved {
			approval[f.PositionID] = false
		}
	}
	ranked := append([]swarm.BranchResult(nil), results...)
	for i := range ranked {
		ranked[i].AuditPassed = approval[ranked[i].PositionID]
		if ranked[i].AuditPassed {
			ranked[i].Score += 0.2
		}
		if ranked[i].Score > 1 {
			ranked[i].Score = 1
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].AuditPassed != ranked[j].AuditPassed {
			return ranked[i].AuditPassed
		}
		return ranked[i].Score > ranked[j].Score
	})
	winner := ranked[0]
	approved := winner.BuildPassed && winner.TestPassed && winner.LintPassed && winner.AuditPassed
	reason := "winner selected by deterministic score"
	if !approved {
		reason = "no branch survived deterministic collapse gates"
	}
	v := Verdict{Winner: winner, Approved: approved, Reason: reason, Ranked: ranked, AuditSummary: findings}
	if e.Lake != nil {
		var summary []string
		for _, r := range ranked {
			summary = append(summary, r.PositionID)
		}
		_, err := e.Lake.Put(lake.Artifact{
			Phase:      lake.PhaseCollapse,
			Kind:       "collapse_verdict",
			Source:     "consensus_engine",
			Claim:      reason,
			Content:    strings.Join(summary, "\n"),
			Confidence: winner.Score,
			Verified:   approved,
			Metadata: map[string]string{
				"winner": winner.PositionID,
			},
		})
		if err != nil {
			return v, err
		}
	}
	return v, nil
}

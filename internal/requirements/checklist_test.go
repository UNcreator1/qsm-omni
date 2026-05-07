package requirements

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptContainsMandatoryReference(t *testing.T) {
	prompt := Prompt()
	for _, want := range []string{"MANDATORY FORCE REQUIREMENTS", "FORCE_REQUIREMENTS_CHECKLIST.md", "QSM_FORCE_CHECKLIST.json", "Never claim top-tier"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q:\n%s", want, prompt)
		}
	}
}

func TestMarkdownTemplateDefaultsToGap(t *testing.T) {
	template := MarkdownTemplate()
	if count := strings.Count(template, "- Status: GAP"); count != 10 {
		t.Fatalf("expected 10 GAP placeholders, got %d:\n%s", count, template)
	}
}

func TestJSONTemplateContainsTenCategories(t *testing.T) {
	var checklist Checklist
	if err := json.Unmarshal([]byte(JSONTemplate()), &checklist); err != nil {
		t.Fatal(err)
	}
	if checklist.Schema != "qsm.force_requirements.v1" {
		t.Fatalf("unexpected schema: %s", checklist.Schema)
	}
	if len(checklist.Categories) != 10 {
		t.Fatalf("expected 10 categories, got %d", len(checklist.Categories))
	}
	for _, category := range checklist.Categories {
		if category.Status != "GAP" {
			t.Fatalf("expected default GAP, got %s for %s", category.Status, category.Name)
		}
	}
}

func TestEnsureArtifactsWritesAndValidatesChecklist(t *testing.T) {
	room := t.TempDir()
	if err := EnsureArtifacts(room); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"FORCE_REQUIREMENTS_CHECKLIST.md", "QSM_FORCE_CHECKLIST.json"} {
		if _, err := os.Stat(filepath.Join(room, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(room, "QSM_FORCE_CHECKLIST.json"), []byte(`{"schema":"bad","categories":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureArtifacts(room); err == nil {
		t.Fatal("expected invalid checklist to fail")
	}
}

func TestScoreEvidenceDoesNotClaimTopTierForLocalSmoke(t *testing.T) {
	report := Score(Evidence{
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
		BrowserCommands:      1,
		LocalPackageAuditOK:  true,
		DataLakeAtomicWrites: true,
	})
	if report.Schema != ScoreSchema {
		t.Fatalf("unexpected score schema: %s", report.Schema)
	}
	if report.TopTier {
		t.Fatal("local smoke must not claim top-tier readiness")
	}
	if len(report.Checklist.Categories) != 10 {
		t.Fatalf("expected 10 scored categories, got %d", len(report.Checklist.Categories))
	}
	security := report.Checklist.Categories[4]
	if security.Status == "GAP" || security.Status == "FAIL" {
		t.Fatalf("expected security to have local evidence, got %#v", security)
	}
	business := report.Checklist.Categories[9]
	if business.Status == "PASS" {
		t.Fatalf("business category must not pass without benchmark/ROI evidence: %#v", business)
	}
}

func TestScoreSecurityFailsOnHighFinding(t *testing.T) {
	report := Score(Evidence{
		ObjectiveID:         "obj-1",
		HarnessMode:         "simulated",
		RunPresent:          true,
		SecurityHigh:        1,
		TestCommands:        1,
		LocalPackageAuditOK: true,
	})
	security := report.Checklist.Categories[4]
	if security.Status != "FAIL" {
		t.Fatalf("expected security FAIL with high finding, got %#v", security)
	}
}

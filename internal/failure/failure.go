package failure

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
)

const Schema = "qsm.failure_report.v1"

type Class string

const (
	ClassRequirementMisunderstanding Class = "requirement_misunderstanding"
	ClassCompileTestRuntime          Class = "compile_test_runtime"
	ClassSandboxTooling              Class = "sandbox_tooling"
	ClassModelRoute                  Class = "model_route"
	ClassMissingKnowledgeMaterials   Class = "missing_knowledge_materials"
	ClassHallucinatedFilesTests      Class = "hallucinated_files_tests"
	ClassFlakyExternalDependency     Class = "flaky_external_dependency"
	ClassCostTimeout                 Class = "cost_timeout"
	ClassOther                       Class = "other"
)

type Report struct {
	Schema              string    `json:"schema"`
	Root                string    `json:"root"`
	ObjectiveID         string    `json:"objective_id,omitempty"`
	Passed              bool      `json:"passed"`
	TotalNodes          int       `json:"total_nodes"`
	SucceededNodes      int       `json:"succeeded_nodes"`
	FailedNodes         int       `json:"failed_nodes"`
	AnalyzedFailures    int       `json:"analyzed_failures"`
	LakeWrites          int       `json:"lake_writes"`
	RegrowthCandidates  int       `json:"regrowth_candidates"`
	ResearchRecommended int       `json:"research_recommended"`
	Failures            []Record  `json:"failures,omitempty"`
	Errors              []string  `json:"errors,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

type Record struct {
	ID                  string           `json:"id"`
	ObjectiveID         string           `json:"objective_id,omitempty"`
	PositionID          string           `json:"position_id"`
	AgentID             string           `json:"agent_id,omitempty"`
	AgentModel          string           `json:"agent_model,omitempty"`
	Room                string           `json:"room"`
	ProductPath         string           `json:"product_path,omitempty"`
	EvidencePath        string           `json:"evidence_path,omitempty"`
	EvidenceDir         string           `json:"evidence_dir,omitempty"`
	TestReportPath      string           `json:"test_report_path,omitempty"`
	TracePath           string           `json:"trace_path,omitempty"`
	Class               Class            `json:"class"`
	Confidence          float64          `json:"confidence"`
	Summary             string           `json:"summary"`
	RootCause           string           `json:"root_cause"`
	Reproducible        bool             `json:"reproducible"`
	ResearchRecommended bool             `json:"research_recommended"`
	RegrowRecommended   bool             `json:"regrow_recommended"`
	CheckpointPhase     string           `json:"checkpoint_phase,omitempty"`
	CheckpointPath      string           `json:"checkpoint_path,omitempty"`
	Lesson              string           `json:"lesson"`
	LessonCacheID       string           `json:"lesson_cache_id,omitempty"`
	Commands            []CommandFailure `json:"commands,omitempty"`
	EvidenceFiles       []string         `json:"evidence_files,omitempty"`
	ExistingCitations   []string         `json:"existing_citations,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
}

type CommandFailure struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind,omitempty"`
	Cmd        []string `json:"cmd,omitempty"`
	CWD        string   `json:"cwd,omitempty"`
	ExitCode   int      `json:"exit_code"`
	Error      string   `json:"error,omitempty"`
	StdoutPath string   `json:"stdout_path,omitempty"`
	StderrPath string   `json:"stderr_path,omitempty"`
}

type Options struct {
	Root        string
	Lake        *lake.Lake
	WriteLake   bool
	WriteRooms  bool
	Now         time.Time
	RunReport   swarm.RunReport
	ReportGiven bool
}

func Analyze(options Options) (Report, error) {
	rootAbs, err := filepath.Abs(firstNonEmpty(options.Root, "."))
	if err != nil {
		return Report{}, err
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runReport := options.RunReport
	if !options.ReportGiven {
		if err := readJSON(filepath.Join(rootAbs, ".state", "run_report.json"), &runReport); err != nil {
			return Report{
				Schema:    Schema,
				Root:      rootAbs,
				Passed:    false,
				Errors:    []string{"missing .state/run_report.json: " + err.Error()},
				CreatedAt: now,
			}, nil
		}
	}
	out := Report{
		Schema:         Schema,
		Root:           rootAbs,
		ObjectiveID:    runReport.ObjectiveID,
		Passed:         true,
		TotalNodes:     runReport.RequestedNodes,
		SucceededNodes: runReport.SucceededNodes,
		FailedNodes:    runReport.FailedNodes,
		CreatedAt:      now,
	}
	for _, result := range runReport.Results {
		if branchSucceeded(result) {
			continue
		}
		record := classify(rootAbs, runReport.ObjectiveID, result, now)
		if options.WriteRooms {
			if err := writeFailureEvidence(record); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: write failure evidence: %v", record.PositionID, err))
			}
		}
		if options.WriteLake && options.Lake != nil {
			cacheItem, err := options.Lake.PutCache(lake.CacheItem{
				Kind:        "failure_lesson",
				ObjectiveID: runReport.ObjectiveID,
				PositionID:  record.PositionID,
				Producer:    "qsm failure-analyze",
				Content:     record.Lesson,
				Verified:    record.Reproducible,
				Confidence:  record.Confidence,
				Metadata: map[string]string{
					"failure_id":           record.ID,
					"failure_class":        string(record.Class),
					"research_recommended": fmt.Sprint(record.ResearchRecommended),
					"regrow_recommended":   fmt.Sprint(record.RegrowRecommended),
				},
			})
			if err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: write lake lesson: %v", record.PositionID, err))
			} else {
				record.LessonCacheID = cacheItem.ID
				out.LakeWrites++
			}
			if err := writeLakeFailure(options.Lake.Root(), record); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: write lake failure: %v", record.PositionID, err))
			}
		}
		out.Failures = append(out.Failures, record)
		out.AnalyzedFailures++
		if record.RegrowRecommended {
			out.RegrowthCandidates++
		}
		if record.ResearchRecommended {
			out.ResearchRecommended++
		}
	}
	if out.FailedNodes > 0 && out.AnalyzedFailures < out.FailedNodes {
		out.Passed = false
		out.Errors = append(out.Errors, fmt.Sprintf("only analyzed %d/%d failed nodes", out.AnalyzedFailures, out.FailedNodes))
	}
	if len(out.Errors) > 0 {
		out.Passed = false
	}
	sort.SliceStable(out.Failures, func(i, j int) bool {
		return out.Failures[i].PositionID < out.Failures[j].PositionID
	})
	return out, nil
}

func classify(root, objectiveID string, result swarm.BranchResult, now time.Time) Record {
	record := Record{
		ObjectiveID:       objectiveID,
		PositionID:        result.PositionID,
		AgentID:           result.AgentID,
		AgentModel:        result.AgentModel,
		Room:              result.Room,
		ProductPath:       result.ProductPath,
		EvidencePath:      result.EvidencePath,
		Class:             ClassOther,
		Confidence:        0.62,
		Reproducible:      true,
		RegrowRecommended: true,
		CheckpointPhase:   "plan",
		Summary:           "node failed before satisfying build/test/lint gates",
		RootCause:         "insufficient evidence for a narrower classification",
		Lesson:            "Do not regrow this branch until the failure evidence is inspected and the next node is given the concrete failing command, missing artifact, or route/tool symptom.",
		ExistingCitations: citationIDs(result),
		CreatedAt:         now,
	}
	if record.PositionID == "" {
		record.PositionID = "unknown-position"
	}
	if record.Room != "" {
		record.EvidenceDir = filepath.Join(record.Room, "failure-evidence")
		record.EvidenceFiles = appendExisting(record.EvidenceFiles,
			record.EvidencePath,
			filepath.Join(record.Room, "room_status.json"),
			filepath.Join(record.Room, ".qsm_harness", "manifest.json"),
			filepath.Join(record.Room, ".qsm_memory", "CACHE.md"),
			filepath.Join(record.Room, ".qsm_memory", "AGENTS.md"),
		)
	}
	if result.TestReport != nil {
		record.TestReportPath = result.TestReport.Path
		record.TracePath = result.TestReport.TracePath
		record.EvidenceFiles = appendExisting(record.EvidenceFiles, result.TestReport.Path, result.TestReport.TracePath)
		for _, command := range result.TestReport.Commands {
			if command.ExitCode != 0 || strings.TrimSpace(command.Error) != "" {
				record.Commands = append(record.Commands, CommandFailure{
					Name:       command.Name,
					Kind:       command.Kind,
					Cmd:        command.Cmd,
					CWD:        command.CWD,
					ExitCode:   command.ExitCode,
					Error:      command.Error,
					StdoutPath: command.StdoutPath,
					StderrPath: command.StderrPath,
				})
				record.EvidenceFiles = appendExisting(record.EvidenceFiles, command.StdoutPath, command.StderrPath)
			}
		}
	}
	signal := failureSignal(result)
	switch {
	case containsAny(signal, "no_progress_timeout", "timeout", "deadline exceeded", "context deadline"):
		record.Class = ClassCostTimeout
		record.Confidence = 0.88
		record.RootCause = "node hit timeout/no-progress limits before producing valid delivery evidence"
		record.Summary = "timeout or no-progress failure"
		record.RegrowRecommended = true
		record.CheckpointPhase = "scaffold"
		record.Lesson = "Regrow from the last scaffold/build checkpoint with a smaller task slice, stricter time budget, and earlier test command execution. Do not spend research cycles unless the logs show a missing material."
	case containsAny(signal, "sandbox", "permission denied", "operation not permitted", "no such file or directory", "docker"):
		record.Class = ClassSandboxTooling
		record.Confidence = 0.82
		record.RootCause = "execution environment or sandbox/tool availability blocked node verification"
		record.Summary = "sandbox/tooling failure"
		record.ResearchRecommended = false
		record.RegrowRecommended = true
		record.CheckpointPhase = "scaffold"
		record.Lesson = "Fix sandbox/tool prerequisites first, then regrow from scaffold. Do not ask a new model to solve an infrastructure failure as if it were a product design problem."
	case containsAny(signal, "429", "rate limit", "route", "router", "model", "provider", "unauthorized", "api key"):
		record.Class = ClassModelRoute
		record.Confidence = 0.84
		record.RootCause = "model route or provider health prevented a complete node attempt"
		record.Summary = "model route/provider failure"
		record.Reproducible = false
		record.RegrowRecommended = true
		record.CheckpointPhase = "plan"
		record.Lesson = "Regrow on a healthy route after route-health confirms capacity. Preserve the original plan but avoid treating provider errors as product defects."
	case result.TestReport != nil && (result.TestReport.Summary.FailedCommands > 0 || len(record.Commands) > 0):
		record.Class = ClassCompileTestRuntime
		record.Confidence = 0.9
		record.RootCause = "QSM-owned command verification failed"
		record.Summary = "compile/test/runtime command failed"
		record.CheckpointPhase = "build"
		record.Lesson = commandLesson(record)
	case !result.BuildPassed || strings.Contains(signal, "missing expected") || strings.Contains(signal, "manifest"):
		record.Class = ClassHallucinatedFilesTests
		record.Confidence = 0.78
		record.RootCause = "node did not produce required product artifacts or manifest-backed verification evidence"
		record.Summary = "missing or hallucinated product/test artifacts"
		record.ResearchRecommended = true
		record.CheckpointPhase = "plan"
		record.Lesson = "Before regrowth, hydrate the lake with the product manifest contract and require the next node to create expected artifacts, real tests, and cache/wiki citations before collapse."
	case !result.TestPassed || !result.LintPassed || !result.AuditPassed:
		record.Class = ClassCompileTestRuntime
		record.Confidence = 0.76
		record.RootCause = "one or more build/test/lint/audit gates failed"
		record.Summary = "quality gate failure"
		record.CheckpointPhase = "build"
		record.Lesson = "Regrow from build checkpoint with the failed gates stated explicitly, then run QSM-owned tests before producing delivery evidence."
	}
	if containsAny(signal, "unknown requirement", "ambiguous", "not enough context", "missing knowledge", "research") {
		record.Class = ClassMissingKnowledgeMaterials
		record.Confidence = max(record.Confidence, 0.86)
		record.ResearchRecommended = true
		record.RootCause = "node appears blocked on missing domain knowledge or materials"
		record.Summary = "missing knowledge/materials"
		record.CheckpointPhase = "plan"
		record.Lesson = "Run targeted hydration for the missing domain/tooling materials, cite the new cache/wiki lesson, then regrow from the plan checkpoint."
	}
	record.ID = failureID(objectiveID, record.PositionID, string(record.Class), record.RootCause)
	if record.EvidenceDir == "" && record.Room != "" {
		record.EvidenceDir = filepath.Join(record.Room, "failure-evidence")
	}
	if checkpoint := bestCheckpoint(record.Room, record.CheckpointPhase); checkpoint != "" {
		record.CheckpointPath = checkpoint
	}
	return record
}

func branchSucceeded(result swarm.BranchResult) bool {
	return result.BuildPassed && result.TestPassed && result.LintPassed && result.Error == ""
}

func failureSignal(result swarm.BranchResult) string {
	var parts []string
	parts = append(parts, result.Error)
	if result.TestReport != nil {
		parts = append(parts, result.TestReport.Errors...)
		parts = append(parts, result.TestReport.Warnings...)
		for _, command := range result.TestReport.Commands {
			if command.ExitCode != 0 || command.Error != "" {
				parts = append(parts, command.Name, command.Kind, strings.Join(command.Cmd, " "), command.Error)
			}
		}
	}
	return strings.ToLower(strings.Join(parts, "\n"))
}

func commandLesson(record Record) string {
	if len(record.Commands) == 0 {
		return "Regrow from build checkpoint and run the failing test command early before adding new features."
	}
	command := record.Commands[0]
	cmd := strings.Join(command.Cmd, " ")
	if cmd == "" {
		cmd = command.Name
	}
	return fmt.Sprintf("Regrow from build checkpoint and make `%s` pass before expanding scope. Use the prior stderr/stdout paths as negative evidence and cite this failure lesson in the next node.", cmd)
}

func writeFailureEvidence(record Record) error {
	if strings.TrimSpace(record.EvidenceDir) == "" {
		return nil
	}
	if err := os.MkdirAll(record.EvidenceDir, 0755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(record.EvidenceDir, "failure.json"), record)
}

func writeLakeFailure(lakeRoot string, record Record) error {
	dir := filepath.Join(lakeRoot, "failures")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, record.ID+".json"), record); err != nil {
		return err
	}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "index.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func Markdown(report Report) string {
	var b strings.Builder
	b.WriteString("# QSM Failure Learning Report\n\n")
	fmt.Fprintf(&b, "- Passed: `%v`\n", report.Passed)
	fmt.Fprintf(&b, "- Objective: `%s`\n", report.ObjectiveID)
	fmt.Fprintf(&b, "- Nodes: total=`%d` succeeded=`%d` failed=`%d`\n", report.TotalNodes, report.SucceededNodes, report.FailedNodes)
	fmt.Fprintf(&b, "- Analyzed failures: `%d`\n", report.AnalyzedFailures)
	fmt.Fprintf(&b, "- Lake writes: `%d`\n", report.LakeWrites)
	fmt.Fprintf(&b, "- Regrowth candidates: `%d`\n", report.RegrowthCandidates)
	fmt.Fprintf(&b, "- Research recommended: `%d`\n", report.ResearchRecommended)
	if len(report.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, err := range report.Errors {
			b.WriteString("- " + err + "\n")
		}
	}
	if len(report.Failures) == 0 {
		b.WriteString("\nNo failed nodes were found. The learning loop is clean for this run.\n")
		return b.String()
	}
	b.WriteString("\n## Failed Nodes\n\n")
	b.WriteString("| Position | Class | Confidence | Regrow | Research | Lesson Cache |\n")
	b.WriteString("| --- | --- | ---: | --- | --- | --- |\n")
	for _, failure := range report.Failures {
		fmt.Fprintf(&b, "| %s | %s | %.2f | %v | %v | %s |\n", failure.PositionID, failure.Class, failure.Confidence, failure.RegrowRecommended, failure.ResearchRecommended, failure.LessonCacheID)
	}
	b.WriteString("\n## Lessons\n\n")
	for _, failure := range report.Failures {
		fmt.Fprintf(&b, "### %s / %s\n\n", failure.PositionID, failure.Class)
		fmt.Fprintf(&b, "- Summary: %s\n", failure.Summary)
		fmt.Fprintf(&b, "- Root cause: %s\n", failure.RootCause)
		if failure.CheckpointPhase != "" {
			fmt.Fprintf(&b, "- Regrow checkpoint: `%s` `%s`\n", failure.CheckpointPhase, failure.CheckpointPath)
		}
		if failure.EvidenceDir != "" {
			fmt.Fprintf(&b, "- Evidence: `%s`\n", failure.EvidenceDir)
		}
		b.WriteString("\n")
		b.WriteString(failure.Lesson)
		b.WriteString("\n\n")
	}
	return b.String()
}

func citationIDs(result swarm.BranchResult) []string {
	var out []string
	for _, citation := range result.Citations {
		if strings.TrimSpace(citation.ID) != "" {
			out = append(out, citation.ID)
		}
		if strings.TrimSpace(citation.Source) != "" {
			out = append(out, citation.Source)
		}
	}
	return unique(out)
}

func appendExisting(values []string, paths ...string) []string {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			values = append(values, path)
		}
	}
	return unique(values)
}

func bestCheckpoint(room, phase string) string {
	if strings.TrimSpace(room) == "" {
		return ""
	}
	names := []string{phase, "build", "scaffold", "plan"}
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		path := filepath.Join(room, "checkpoints", name+".tar.gz")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func failureID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "failure-v1-" + hex.EncodeToString(sum[:])[:16]
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func unique(values []string) []string {
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

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
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

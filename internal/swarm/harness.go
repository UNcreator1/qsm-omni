package swarm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/methodology"
	"github.com/nemoclaws/quantum-swarm-v3/internal/nodekit"
	"github.com/nemoclaws/quantum-swarm-v3/internal/requirements"
	qruntime "github.com/nemoclaws/quantum-swarm-v3/internal/runtime"
	"github.com/nemoclaws/quantum-swarm-v3/internal/tester"
)

type Harness interface {
	Execute(ctx context.Context, p Position, agent Agent, obj Objective) (BranchResult, error)
}

type OpenCodeHarness struct {
	Config  qruntime.Config
	Timeout time.Duration
}

func (h OpenCodeHarness) Execute(ctx context.Context, p Position, agent Agent, obj Objective) (BranchResult, error) {
	if err := h.Config.ValidateForRealHarness(); err != nil {
		return BranchResult{}, err
	}
	room, err := filepath.Abs(p.Room)
	if err != nil {
		return BranchResult{}, err
	}
	p.Room = room
	if h.Timeout <= 0 {
		h.Timeout = 20 * time.Minute
	}
	if err := os.MkdirAll(p.Room, 0755); err != nil {
		return BranchResult{}, err
	}
	if h.Config.OpenCodeConfig != "" {
		if err := copyFile(h.Config.OpenCodeConfig, filepath.Join(p.Room, "opencode.json")); err != nil {
			return BranchResult{}, err
		}
	}
	promptPath := filepath.Join(p.Room, "agent_prompt.md")
	prompt := realAgentPrompt(p, agent, obj, h.Config)
	if err := os.WriteFile(promptPath, []byte(prompt), 0644); err != nil {
		return BranchResult{}, err
	}
	MarkRoomPhase(p.Room, "opencode_launch")
	runCtx, cancel := context.WithTimeout(ctx, h.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, h.Config.OpenCodePath, "run", "--model", agent.Provider+"/"+agent.Model, "--format", "json", "--pure", prompt)
	cmd.Dir = p.Room
	cmd.Env = append(os.Environ(),
		"QSM_AGENT_ID="+agent.ID,
		"QSM_OBJECTIVE_ID="+obj.ID,
		"QSM_POSITION_ID="+p.ID,
		"QSM_9ROUTER_URL="+h.Config.NineRouterURL,
		"QSM_LAKE_PATH="+h.Config.LakePath,
		"QSM_WIKI_PATH="+h.Config.WikiPath,
		"QSM_ROOM_PATH="+p.Room,
		"QSM_CACHE_PATH="+filepath.Join(p.Room, ".qsm_memory", "CACHE.md"),
		"QSM_STATUS_PATH="+RoomStatusPath(p.Room),
		"QSM_HARNESS_KIT_PATH="+filepath.Join(p.Room, ".qsm_harness"),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	MarkRoomPhase(p.Room, "opencode_running")
	err = cmd.Run()
	MarkRoomPhase(p.Room, "opencode_verify")
	_ = os.WriteFile(filepath.Join(p.Room, "opencode.stdout.jsonl"), stdout.Bytes(), 0644)
	_ = os.WriteFile(filepath.Join(p.Room, "opencode.stderr.log"), stderr.Bytes(), 0644)
	result := BranchResult{
		PositionID:   p.ID,
		Room:         p.Room,
		EvidencePath: filepath.Join(p.Room, "evidence.json"),
		ProductPath:  filepath.Join(p.Room, "product"),
		CompletedAt:  time.Now().UTC(),
	}
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			_, _ = readAgentEvidence(&result)
		}
		if runCtx.Err() == context.DeadlineExceeded && verifyProductAndTests(&result) == nil {
			result.BuildPassed = true
			result.Score = 0.85
			ensureMetadata(&result)["warning"] = "opencode reached harness timeout after producing a non-empty product"
			_ = writeJSON(result.EvidencePath, map[string]any{
				"result":  result,
				"warning": "opencode reached harness timeout after producing a non-empty product",
				"stdout":  stdout.String(),
				"stderr":  stderr.String(),
			})
			MarkRoomResult(p.Room, result)
			return result, nil
		}
		result.BuildPassed = false
		result.TestPassed = false
		result.LintPassed = false
		result.Score = 0
		_ = writeJSON(result.EvidencePath, map[string]any{
			"result": result,
			"stdout": stdout.String(),
			"stderr": stderr.String(),
			"error":  err.Error(),
		})
		result.Error = err.Error()
		MarkRoomResult(p.Room, result)
		return result, fmt.Errorf("opencode harness failed for %s: %w: %s", p.ID, err, strings.TrimSpace(stderr.String()))
	}
	result.BuildPassed = true
	result.TestPassed = true
	result.LintPassed = true
	result.Score = 1.0
	evidenceOK, err := readAgentEvidence(&result)
	if err != nil {
		return result, err
	}
	if strictEvidenceEnabled() && !evidenceOK {
		return result, fmt.Errorf("strict evidence enabled and %s did not write evidence.json", p.ID)
	}
	if err := verifyProductAndTests(&result); err != nil {
		if result.Verification == nil || !result.Verification.Passed {
			result.BuildPassed = false
		}
		result.TestPassed = false
		result.LintPassed = false
		result.Score = 0
		_ = writeJSON(result.EvidencePath, result)
		result.Error = err.Error()
		MarkRoomResult(p.Room, result)
		return result, err
	}
	if err := writeJSON(result.EvidencePath, result); err != nil {
		result.Error = err.Error()
		MarkRoomResult(p.Room, result)
		return result, err
	}
	MarkRoomResult(p.Room, result)
	return result, nil
}

type LangChainHarness struct {
	Config  qruntime.Config
	Timeout time.Duration
}

func (h LangChainHarness) Execute(ctx context.Context, p Position, agent Agent, obj Objective) (BranchResult, error) {
	if err := h.Config.ValidateForRealHarness(); err != nil {
		return BranchResult{}, err
	}
	room, err := filepath.Abs(p.Room)
	if err != nil {
		return BranchResult{}, err
	}
	p.Room = room
	if h.Timeout <= 0 {
		h.Timeout = 20 * time.Minute
	}
	if err := os.MkdirAll(p.Room, 0755); err != nil {
		return BranchResult{}, err
	}
	MarkRoomPhase(p.Room, "langchain_prepare")
	payload := map[string]any{"position": p, "agent": agent, "objective": obj, "wiki_path": h.Config.WikiPath, "lake_path": h.Config.LakePath, "cache_path": filepath.Join(p.Room, ".qsm_memory", "CACHE.md"), "harness_kit_path": filepath.Join(p.Room, ".qsm_harness")}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return BranchResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, h.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, h.Config.PythonPath, h.Config.LangChainRunner)
	cmd.Dir = p.Room
	cmd.Stdin = bytes.NewReader(payloadBytes)
	cmd.Env = append(os.Environ(),
		"QSM_9ROUTER_URL="+h.Config.NineRouterURL,
		"QSM_9ROUTER_API_KEY="+h.Config.NineRouterKey,
		"QSM_AGENT_ID="+agent.ID,
		"QSM_OBJECTIVE_ID="+obj.ID,
		"QSM_POSITION_ID="+p.ID,
		"QSM_LAKE_PATH="+h.Config.LakePath,
		"QSM_WIKI_PATH="+h.Config.WikiPath,
		"QSM_ROOM_PATH="+p.Room,
		"QSM_CACHE_PATH="+filepath.Join(p.Room, ".qsm_memory", "CACHE.md"),
		"QSM_STATUS_PATH="+RoomStatusPath(p.Room),
		"QSM_HARNESS_KIT_PATH="+filepath.Join(p.Room, ".qsm_harness"),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	MarkRoomPhase(p.Room, "langchain_running")
	if err := cmd.Run(); err != nil {
		MarkRoomPhase(p.Room, "langchain_verify")
		_ = os.WriteFile(filepath.Join(p.Room, "langchain.stdout.log"), stdout.Bytes(), 0644)
		_ = os.WriteFile(filepath.Join(p.Room, "langchain.stderr.log"), stderr.Bytes(), 0644)
		harnessErr := fmt.Errorf("langchain harness failed for %s: %w: %s", p.ID, err, strings.TrimSpace(stderr.String()))
		result := BranchResult{
			PositionID:   p.ID,
			Room:         p.Room,
			EvidencePath: filepath.Join(p.Room, "evidence.json"),
			ProductPath:  filepath.Join(p.Room, "product"),
			CompletedAt:  time.Now().UTC(),
		}
		_, _ = readAgentEvidence(&result)
		if verifyErr := verifyProductAndTests(&result); verifyErr == nil {
			result.BuildPassed = true
			result.TestPassed = true
			result.LintPassed = true
			if result.Score <= 0 {
				result.Score = 0.72
			}
			ensureMetadata(&result)["warning"] = "langchain exited nonzero after producing a product that passed QSM verification"
			ensureMetadata(&result)["harness_error"] = truncateMeta(harnessErr.Error(), 1200)
			if writeErr := writeJSON(result.EvidencePath, result); writeErr != nil {
				result.Error = writeErr.Error()
				MarkRoomResult(p.Room, result)
				return result, writeErr
			}
			MarkRoomResult(p.Room, result)
			return result, nil
		}
		result.Error = harnessErr.Error()
		_ = writeJSON(result.EvidencePath, result)
		MarkRoomResult(p.Room, result)
		return result, harnessErr
	}
	MarkRoomPhase(p.Room, "langchain_verify")
	_ = os.WriteFile(filepath.Join(p.Room, "langchain.stdout.log"), stdout.Bytes(), 0644)
	_ = os.WriteFile(filepath.Join(p.Room, "langchain.stderr.log"), stderr.Bytes(), 0644)
	result := BranchResult{
		PositionID:   p.ID,
		Room:         p.Room,
		BuildPassed:  true,
		TestPassed:   true,
		LintPassed:   true,
		Score:        1.0,
		EvidencePath: filepath.Join(p.Room, "evidence.json"),
		ProductPath:  filepath.Join(p.Room, "product"),
		CompletedAt:  time.Now().UTC(),
	}
	evidenceOK, err := readAgentEvidence(&result)
	if err != nil {
		return result, err
	}
	if strictEvidenceEnabled() && !evidenceOK {
		return result, fmt.Errorf("strict evidence enabled and %s did not write evidence.json", p.ID)
	}
	if err := verifyProductAndTests(&result); err != nil {
		if result.Verification == nil || !result.Verification.Passed {
			result.BuildPassed = false
		}
		result.TestPassed = false
		result.LintPassed = false
		result.Score = 0
		_ = writeJSON(result.EvidencePath, result)
		result.Error = err.Error()
		MarkRoomResult(p.Room, result)
		return result, err
	}
	if err := writeJSON(result.EvidencePath, result); err != nil {
		result.Error = err.Error()
		MarkRoomResult(p.Room, result)
		return result, err
	}
	MarkRoomResult(p.Room, result)
	return result, nil
}

func realAgentPrompt(p Position, agent Agent, obj Objective, cfg qruntime.Config) string {
	return fmt.Sprintf(`# Quantum Swarm Real Agent Harness

Agent: %s
Role: %s
Model: %s/%s
Position: %s
Objective: %s

%s

You are running inside an isolated room. Use this loop:

1. Plan
2. Code into ./product
3. Test
4. Verify

%s

%s

Rules:

- Work only in the current directory and its ./product child.
- Do not spawn subagents or background tasks.
- Do not write to the repository root outside this room.
- Build the product with direct file edits or shell commands available to you.

Shared memory:

- Lake: %s
- Wiki LLM memory: %s
- Shared verified cache: %s

The shared cache may contain verified facts, negative lessons, dependency notes, and scheduler signals from earlier nodes. Do not copy sibling branch products or unverified strategies.

Grounding:

- If you make factual claims based on the lake, wiki, or shared cache, include short source notes in ./evidence.json.
- If shared memory affects your plan/build/test choices, include cache_item_ids_used, wiki_item_ids_used, or memory_citations with cache_item:<id> / wiki_item:<id> sources.
- Do not invent citations. If the room memory does not support a claim, say that no grounded evidence was available.
- QSM will independently map any source notes to exact local quotes after you finish.

Write ./evidence.json before exit with build_passed, test_passed, lint_passed, score, and product_path.
After ./product and ./evidence.json are complete, end the run immediately.
`, agent.ID, agent.Role, agent.Provider, agent.Model, p.ID, obj.Request, requirements.Prompt(), methodology.Prompt(methodology.PhasePlanning, methodology.PhaseBuild), nodekit.Prompt(".qsm_harness"), cfg.LakePath, cfg.WikiPath, filepath.Join(p.Room, ".qsm_memory", "CACHE.md"))
}

func readAgentEvidence(result *BranchResult) (bool, error) {
	data, err := os.ReadFile(result.EvidencePath)
	if err != nil {
		data, err = os.ReadFile(filepath.Join(result.ProductPath, "evidence.json"))
		if err != nil {
			return false, nil
		}
	}
	var incoming BranchResult
	if err := json.Unmarshal(data, &incoming); err != nil {
		return false, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err == nil {
		mergeEvidenceMetadata(result, raw)
	}
	if incoming.ProductPath != "" {
		result.ProductPath = resolveAgentProductPath(result.Room, incoming.ProductPath)
		result.ProductPath = normalizeProductPath(result.ProductPath)
	}
	if incoming.Score > 0 {
		result.Score = incoming.Score
		if result.Score > 1 {
			result.Score = result.Score / 100
		}
		if result.Score > 1 {
			result.Score = 1
		}
	}
	if _, ok := raw["build_passed"]; ok {
		result.BuildPassed = incoming.BuildPassed
	}
	if _, ok := raw["test_passed"]; ok {
		result.TestPassed = incoming.TestPassed
	}
	if _, ok := raw["lint_passed"]; ok {
		result.LintPassed = incoming.LintPassed
	}
	if incoming.Verification != nil {
		result.Verification = incoming.Verification
	}
	if incoming.TestReport != nil {
		result.TestReport = incoming.TestReport
	}
	if len(incoming.Citations) > 0 {
		result.Citations = incoming.Citations
	}
	for key, value := range incoming.Metadata {
		ensureMetadata(result)[key] = value
	}
	return true, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

func requireProduct(path string, result *BranchResult) error {
	verification := VerifyProduct(path)
	if result != nil {
		result.Verification = &verification
	}
	if verification.Passed {
		return nil
	}
	if len(verification.Errors) == 0 {
		return fmt.Errorf("agent product verification failed at %s", path)
	}
	return fmt.Errorf("agent product verification failed at %s: %s", path, strings.Join(verification.Errors, "; "))
}

func verifyProductAndTests(result *BranchResult) error {
	if err := requirements.EnsureArtifacts(result.Room); err != nil {
		result.BuildPassed = false
		result.TestPassed = false
		result.LintPassed = false
		return err
	}
	productErr := requireProduct(result.ProductPath, result)
	if productErr != nil {
		result.BuildPassed = false
	} else {
		result.BuildPassed = true
	}
	MarkRoomPhase(result.Room, "qsm_test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	report, err := tester.Verify(ctx, result.Room, result.ProductPath)
	if report != nil {
		result.TestReport = report
		meta := ensureMetadata(result)
		meta["qsm_test_report"] = report.Path
		meta["qsm_test_commands"] = fmt.Sprint(report.Summary.Commands)
		meta["qsm_test_passed_commands"] = fmt.Sprint(report.Summary.PassedCommands)
		meta["qsm_test_failed_commands"] = fmt.Sprint(report.Summary.FailedCommands)
		meta["qsm_test_count"] = fmt.Sprint(report.Summary.Tests)
	}
	if err != nil {
		result.TestPassed = false
		result.LintPassed = false
		return fmt.Errorf("qsm test runner failed: %w", err)
	}
	if report == nil {
		result.TestPassed = false
		result.LintPassed = false
		return fmt.Errorf("qsm test runner did not return a report")
	}
	result.TestPassed = report.Passed
	result.LintPassed = report.Passed
	if productErr != nil && !report.Passed && len(report.Errors) > 0 {
		return fmt.Errorf("%v; qsm test verification failed: %s", productErr, strings.Join(report.Errors, "; "))
	}
	if productErr != nil {
		return productErr
	}
	if report.Passed {
		return nil
	}
	if len(report.Errors) > 0 {
		return fmt.Errorf("qsm test verification failed: %s", strings.Join(report.Errors, "; "))
	}
	return fmt.Errorf("qsm test verification failed")
}

func normalizeProductPath(path string) string {
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		return filepath.Dir(path)
	}
	return path
}

func resolveAgentProductPath(room, productPath string) string {
	cleanProduct := filepath.Clean(productPath)
	if filepath.IsAbs(cleanProduct) {
		return cleanProduct
	}
	cleanRoom := filepath.Clean(room)
	if cleanProduct == cleanRoom || strings.HasPrefix(cleanProduct, cleanRoom+string(os.PathSeparator)) {
		return cleanProduct
	}
	return filepath.Clean(filepath.Join(cleanRoom, cleanProduct))
}

func strictEvidenceEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("QSM_STRICT_EVIDENCE")))
	return value == "1" || value == "true" || value == "yes"
}

func ensureMetadata(result *BranchResult) map[string]any {
	if result.Metadata == nil {
		result.Metadata = map[string]any{}
	}
	return result.Metadata
}

func mergeEvidenceMetadata(result *BranchResult, raw map[string]any) {
	meta := ensureMetadata(result)
	for _, key := range []string{"harness", "stop_reason", "warning", "final_message", "router_url", "effective_model"} {
		if value, ok := raw[key]; ok {
			meta[key] = truncateMeta(fmt.Sprint(value), 1200)
		}
	}
	for _, key := range []string{"cache_item_ids_used", "cache_items_used", "cache_item_ids_observed", "source_notes"} {
		if value, ok := raw[key]; ok {
			meta[key] = value
		}
	}
	if value, ok := raw["verification"]; ok && result.Verification == nil {
		data, err := json.Marshal(value)
		if err == nil {
			var verification ProductVerification
			if json.Unmarshal(data, &verification) == nil {
				result.Verification = &verification
			}
		}
	}
}

func truncateMeta(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}

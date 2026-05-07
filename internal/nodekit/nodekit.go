package nodekit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const Schema = "qsm.node_harness_kit.v1"

type Params struct {
	ObjectiveID       string `json:"objective_id"`
	Request           string `json:"request"`
	PositionID        string `json:"position_id"`
	PositionName      string `json:"position_name,omitempty"`
	Strategy          string `json:"strategy,omitempty"`
	AgentID           string `json:"agent_id"`
	AgentRole         string `json:"agent_role,omitempty"`
	AgentModel        string `json:"agent_model,omitempty"`
	HarnessMode       string `json:"harness_mode,omitempty"`
	LakePath          string `json:"lake_path,omitempty"`
	WikiPath          string `json:"wiki_path,omitempty"`
	CachePath         string `json:"cache_path,omitempty"`
	OpenHarnessPath   string `json:"openharness_path,omitempty"`
	OpenHarnessCommit string `json:"openharness_commit,omitempty"`
}

type Manifest struct {
	Schema             string            `json:"schema"`
	CreatedAt          time.Time         `json:"created_at"`
	Params             Params            `json:"params"`
	Skills             []Skill           `json:"skills"`
	Hooks              []Hook            `json:"hooks"`
	PermissionPolicy   PermissionPolicy  `json:"permission_policy"`
	MemoryEntrypoints  map[string]string `json:"memory_entrypoints"`
	OpenHarnessPattern string            `json:"openharness_pattern"`
}

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

type Hook struct {
	Event          string `json:"event"`
	Type           string `json:"type"`
	Command        string `json:"command,omitempty"`
	Matcher        string `json:"matcher,omitempty"`
	BlockOnFailure bool   `json:"block_on_failure"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type PermissionPolicy struct {
	Mode              string   `json:"mode"`
	AllowedWriteRoots []string `json:"allowed_write_roots"`
	AllowedReadRoots  []string `json:"allowed_read_roots"`
	DeniedPaths       []string `json:"denied_paths"`
	DeniedCommands    []string `json:"denied_commands"`
	Notes             []string `json:"notes"`
}

func Write(room string, params Params) (Manifest, error) {
	root := filepath.Join(room, ".qsm_harness")
	if err := os.MkdirAll(root, 0755); err != nil {
		return Manifest{}, err
	}
	skills, err := writeSkills(root)
	if err != nil {
		return Manifest{}, err
	}
	hooks := defaultHooks()
	policy := defaultPolicy()
	manifest := Manifest{
		Schema:           Schema,
		CreatedAt:        time.Now().UTC(),
		Params:           params,
		Skills:           skills,
		Hooks:            hooks,
		PermissionPolicy: policy,
		MemoryEntrypoints: map[string]string{
			"wiki":          params.WikiPath,
			"lake":          params.LakePath,
			"room_cache":    params.CachePath,
			"agents_memory": filepath.Join(room, ".qsm_memory", "AGENTS.md"),
			"kit_memory":    filepath.Join(root, "MEMORY.md"),
		},
		OpenHarnessPattern: "skills + hooks + permissions + persistent memory, adapted as room-local QSM node kit",
	}
	if err := writeJSON(filepath.Join(root, "manifest.json"), manifest); err != nil {
		return manifest, err
	}
	if err := writeJSON(filepath.Join(root, "hooks.json"), map[string]any{"schema": "qsm.node_hooks.v1", "hooks": hooks}); err != nil {
		return manifest, err
	}
	if err := os.WriteFile(filepath.Join(root, "PERMISSIONS.md"), []byte(policyMarkdown(policy)), 0644); err != nil {
		return manifest, err
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte(memoryMarkdown(params)), 0644); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func Prompt(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = ".qsm_harness"
	}
	return fmt.Sprintf(`OpenHarness-inspired node kit:
- Read %s/manifest.json before deep work.
- Use %s/skills for plan, test, review, lake, and force-requirements discipline.
- Follow %s/PERMISSIONS.md: write only in the room, ./product, force checklist files, and ./evidence.json.
- Treat %s/hooks.json as lifecycle expectations: pre-build planning, pre-test verification, post-test evidence.
- Use %s/MEMORY.md as the room-local persistent memory index.
- Every product node must write ./product/qsm_project_manifest.v1.json before evidence.json.
`, path, path, path, path, path)
}

func writeSkills(root string) ([]Skill, error) {
	specs := []struct {
		dir         string
		name        string
		description string
		content     string
	}{
		{"plan", "qsm-plan", "Design implementation before coding.", planSkill()},
		{"test", "qsm-test", "Write and run real tests; never fake verification.", testSkill()},
		{"review", "qsm-review", "Review product for bugs, security, and missing evidence.", reviewSkill()},
		{"lake", "qsm-lake-memory", "Use QSM wiki/cache/lake facts with citations.", lakeSkill()},
		{"force", "qsm-force-requirements", "Maintain force checklist and honest readiness claims.", forceSkill()},
	}
	var out []Skill
	for _, spec := range specs {
		path := filepath.Join(root, "skills", spec.dir, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return out, err
		}
		if err := os.WriteFile(path, []byte(spec.content), 0644); err != nil {
			return out, err
		}
		out = append(out, Skill{Name: spec.name, Description: spec.description, Path: path})
	}
	return out, nil
}

func defaultHooks() []Hook {
	return []Hook{
		{Event: "PreBuild", Type: "command", Command: "test -f PLAN.md && test -f QSM_FORCE_CHECKLIST.json", Matcher: "*", BlockOnFailure: true, TimeoutSeconds: 10},
		{Event: "PreTest", Type: "command", Command: "test -d product && find product -type f | head -n 1 | grep -q .", Matcher: "*", BlockOnFailure: true, TimeoutSeconds: 10},
		{Event: "PostTest", Type: "command", Command: "test -f evidence.json && test -f product/qsm_project_manifest.v1.json", Matcher: "*", BlockOnFailure: true, TimeoutSeconds: 10},
	}
}

func defaultPolicy() PermissionPolicy {
	return PermissionPolicy{
		Mode:              "room-default",
		AllowedWriteRoots: []string{"./product", "./product/qsm_project_manifest.v1.json", "./evidence.json", "./FORCE_REQUIREMENTS_CHECKLIST.md", "./QSM_FORCE_CHECKLIST.json", "./.qsm_memory", "./.qsm_status"},
		AllowedReadRoots:  []string{".", "./.qsm_memory", "./.qsm_harness", "./product"},
		DeniedPaths:       []string{"*/.ssh/*", "*/.aws/*", "*/.config/gcloud/*", "*/.azure/*", "*/.gnupg/*", "*/.docker/config.json", "*/.kube/config", "*/.env", "/Users/*"},
		DeniedCommands:    []string{"rm -rf /*", "sudo *", "chmod -R 777 *", "curl *|sh", "curl *| bash", "wget *|sh", "wget *| bash"},
		Notes: []string{
			"QSM currently enforces this mainly through prompt, room boundaries, and post-run verification; hard sandbox execution is a separate roadmap gate.",
			"Do not read or copy sibling rooms. Do not write outside the current room.",
		},
	}
}

func policyMarkdown(policy PermissionPolicy) string {
	var b strings.Builder
	b.WriteString("# QSM Node Permission Policy\n\n")
	fmt.Fprintf(&b, "- Mode: `%s`\n\n", policy.Mode)
	b.WriteString("## Allowed Write Roots\n\n")
	for _, item := range policy.AllowedWriteRoots {
		b.WriteString("- `" + item + "`\n")
	}
	b.WriteString("\n## Allowed Read Roots\n\n")
	for _, item := range policy.AllowedReadRoots {
		b.WriteString("- `" + item + "`\n")
	}
	b.WriteString("\n## Denied Paths\n\n")
	for _, item := range policy.DeniedPaths {
		b.WriteString("- `" + item + "`\n")
	}
	b.WriteString("\n## Denied Commands\n\n")
	for _, item := range policy.DeniedCommands {
		b.WriteString("- `" + item + "`\n")
	}
	b.WriteString("\n## Notes\n\n")
	for _, item := range policy.Notes {
		b.WriteString("- " + item + "\n")
	}
	return b.String()
}

func memoryMarkdown(params Params) string {
	var b strings.Builder
	b.WriteString("# QSM Node Memory\n\n")
	fmt.Fprintf(&b, "Objective: %s\n", params.ObjectiveID)
	fmt.Fprintf(&b, "Position: %s\n", params.PositionID)
	fmt.Fprintf(&b, "Agent: %s\n\n", params.AgentID)
	b.WriteString("## Entry Points\n\n")
	fmt.Fprintf(&b, "- Wiki: `%s`\n", params.WikiPath)
	fmt.Fprintf(&b, "- Lake: `%s`\n", params.LakePath)
	fmt.Fprintf(&b, "- Room cache: `%s`\n\n", params.CachePath)
	b.WriteString("## Rules\n\n")
	b.WriteString("- Prefer verified cache and wiki facts over speculation.\n")
	b.WriteString("- If a needed fact is missing, write the gap into evidence and continue with a locally testable product.\n")
	b.WriteString("- Record cache item IDs or source notes in evidence when using shared memory.\n")
	return b.String()
}

func planSkill() string {
	return `---
name: qsm-plan
description: Design implementation before coding.
---

# qsm-plan

Use before editing.

1. Restate the objective.
2. Define the product files under ./product.
3. Define the smallest real verification command.
4. Name risks and edge cases.
5. Keep the plan small enough to finish inside this room.
`
}

func testSkill() string {
	return `---
name: qsm-test
description: Write and run real tests; never fake verification.
---

# qsm-test

Rules:

- Do not mentally verify when a shell check is possible.
- Do not hard-code tests to match the implementation.
- Prefer deterministic local tests over screenshots or prose.
- Record commands and results in ./evidence.json.
`
}

func reviewSkill() string {
	return `---
name: qsm-review
description: Review product for bugs, security, and missing evidence.
---

# qsm-review

Check:

- Product files exist and are not placeholders.
- product/qsm_project_manifest.v1.json exists and names product_kind, expected_artifacts, test_commands, and cache_item:/wiki_item: memory citations.
- User workflow is complete.
- No secrets or host paths are copied.
- Evidence matches actual product state.
- FORCE checklist is honest about gaps.
`
}

func lakeSkill() string {
	return `---
name: qsm-lake-memory
description: Use QSM wiki/cache/lake facts with citations.
---

# qsm-lake-memory

Use QSM memory as a materials warehouse:

- Read .qsm_memory/AGENTS.md and .qsm_memory/CACHE.md.
- Reuse verified recipes.
- Avoid repeated failed attempts.
- Cite cache_item_ids_used, wiki_item_ids_used, or memory_citations in evidence for every factual build/test decision that uses shared memory.
- Use memory_citations entries like {"source":"cache_item:<id>","reason":"why this changed the plan"}.
- Also copy the same source IDs into product/qsm_project_manifest.v1.json memory_citations.
- If no grounded memory exists, say so.
`
}

func forceSkill() string {
	return `---
name: qsm-force-requirements
description: Maintain force checklist and honest readiness claims.
---

# qsm-force-requirements

Keep FORCE_REQUIREMENTS_CHECKLIST.md and QSM_FORCE_CHECKLIST.json updated.

- PASS only with evidence.
- GAP when enterprise evidence is missing.
- Never claim production/top-tier without QSM force score evidence.
`
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

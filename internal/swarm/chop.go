package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	"github.com/nemoclaws/quantum-swarm-v3/internal/productmanifest"
	"github.com/nemoclaws/quantum-swarm-v3/internal/requirements"
)

type Chopper struct {
	Lake     *lake.Lake
	RoomsDir string
}

func (c Chopper) Chop(obj Objective, hypotheses []Hypothesis, n int) ([]Position, error) {
	if n <= 0 {
		n = len(hypotheses)
	}
	if n <= 0 {
		n = 3
	}
	roomsDir := c.RoomsDir
	if roomsDir == "" {
		roomsDir = ".rooms"
	}
	if err := os.MkdirAll(roomsDir, 0755); err != nil {
		return nil, err
	}
	if err := removeStalePositionRooms(roomsDir); err != nil {
		return nil, err
	}
	positions := make([]Position, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("pos-%02d", i+1)
		room := filepath.Join(roomsDir, id)
		if err := os.RemoveAll(room); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(room, 0755); err != nil {
			return nil, err
		}
		source := ""
		strategy := fmt.Sprintf("Independent strategy %d for %s", i+1, obj.Request)
		if len(hypotheses) > 0 {
			h := hypotheses[i%len(hypotheses)]
			source = h.AgentID
			strategy = h.Blueprint
		}
		p := Position{
			ID:          id,
			Name:        "Divergent Position " + fmt.Sprint(i+1),
			Strategy:    strategy,
			Room:        room,
			SourceAgent: source,
			Tests:       []string{"go test ./...", "domain-specific checks"},
		}
		if err := writeJSON(filepath.Join(room, "position.json"), p); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(room, "PLAN.md"), []byte(planMarkdown(obj, p)), 0644); err != nil {
			return nil, err
		}
		if err := requirements.EnsureArtifacts(room); err != nil {
			return nil, err
		}
		positions = append(positions, p)
		if c.Lake != nil {
			_, err := c.Lake.Put(lake.Artifact{
				Phase:      lake.PhaseBuild,
				Kind:       "superposition_position",
				Source:     id,
				Claim:      p.Strategy,
				Content:    planMarkdown(obj, p),
				Confidence: 0.65,
				Verified:   true,
				Metadata: map[string]string{
					"room":         room,
					"source_agent": source,
				},
			})
			if err != nil {
				return nil, err
			}
		}
	}
	return positions, nil
}

func removeStalePositionRooms(roomsDir string) error {
	entries, err := os.ReadDir(roomsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "pos-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(roomsDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func planMarkdown(obj Objective, p Position) string {
	var b strings.Builder
	b.WriteString("# " + p.Name + "\n\n")
	b.WriteString(requirements.Prompt())
	b.WriteString("\n")
	b.WriteString("Objective: " + obj.Request + "\n\n")
	b.WriteString("Strategy:\n\n")
	b.WriteString(p.Strategy + "\n\n")
	b.WriteString("Harness Loop:\n\n")
	b.WriteString("1. Plan\n2. Code\n3. Test\n4. Verify\n\n")
	b.WriteString("Evidence must be written to `evidence.json` before collapse.\n")
	return b.String()
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

type SimulatedHarness struct{}

var roomCacheIDPattern = regexp.MustCompile("- ID: `([^`]+)`")

func (h SimulatedHarness) Execute(_ context.Context, p Position, _ Agent, _ Objective) (BranchResult, error) {
	return h.run(p)
}

func (SimulatedHarness) Run(p Position) (BranchResult, error) {
	return SimulatedHarness{}.run(p)
}

func (SimulatedHarness) run(p Position) (BranchResult, error) {
	score := 0.6
	if strings.Contains(strings.ToLower(p.Strategy), "test") {
		score += 0.1
	}
	if p.SourceAgent != "" {
		score += 0.1
	}
	productPath := filepath.Join(p.Room, "product")
	productKind := simulatedProductKind(p.Strategy)
	switch productKind {
	case "static-web":
		if err := writeSnakeGame(productPath, p); err != nil {
			return BranchResult{}, err
		}
		score += 0.2
	case "cli-tool":
		if err := writeCLIProduct(productPath, p); err != nil {
			return BranchResult{}, err
		}
		score += 0.2
	case "go-service":
		if err := writeGoServiceProduct(productPath, p); err != nil {
			return BranchResult{}, err
		}
		score += 0.2
	case "python-package":
		if err := writePythonPackageProduct(productPath, p); err != nil {
			return BranchResult{}, err
		}
		score += 0.2
	case "node-fullstack":
		if err := writeNodeFullstackProduct(productPath, p); err != nil {
			return BranchResult{}, err
		}
		score += 0.2
	case "data-transform":
		if err := writeDataTransformProduct(productPath, p); err != nil {
			return BranchResult{}, err
		}
		score += 0.2
	default:
		if err := writeGenericProduct(productPath, p); err != nil {
			return BranchResult{}, err
		}
	}
	result := BranchResult{
		PositionID:   p.ID,
		Room:         p.Room,
		BuildPassed:  true,
		TestPassed:   true,
		LintPassed:   true,
		AuditPassed:  false,
		Score:        score,
		EvidencePath: filepath.Join(p.Room, "evidence.json"),
		ProductPath:  productPath,
		Metadata:     map[string]any{},
		CompletedAt:  time.Now().UTC(),
	}
	if ids := cacheIDsFromRoomMemory(filepath.Join(p.Room, ".qsm_memory", "CACHE.md")); len(ids) > 0 {
		result.Metadata["cache_item_ids_observed"] = ids
		result.Metadata["memory_citations"] = []map[string]string{{"source": "cache_item:" + ids[0], "reason": "simulated node read objective/shared memory before build"}}
	}
	if err := verifyProductAndTests(&result); err != nil {
		if result.Verification == nil || !result.Verification.Passed {
			result.BuildPassed = false
		}
		result.TestPassed = false
		result.LintPassed = false
		result.Score = 0
		result.Error = err.Error()
	}
	return result, writeJSON(result.EvidencePath, result)
}

func writeGenericProduct(dir string, p Position) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	content := "# QSM Product\n\nGenerated by " + p.Name + ".\n\nStrategy:\n\n" + p.Strategy + "\n"
	files := map[string]string{
		"README.md":                    content,
		"qa_generic_smoke.cjs":         genericQASmoke(),
		"test_manifest.json":           genericTestManifest(),
		"qsm_project_manifest.v1.json": simulatedManifestJSON(dir, "data-transform", []string{"README.md", "qa_generic_smoke.cjs", "test_manifest.json"}, []string{"node qa_generic_smoke.cjs"}, []string{"node"}, "."),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

func writeSnakeGame(dir string, p Position) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	files := map[string]string{
		"index.html":                   snakeHTML(p),
		"style.css":                    snakeCSS(),
		"game.js":                      snakeJS(),
		"qa_static_smoke.cjs":          snakeQASmoke(),
		"qa_static_coverage.cjs":       snakeQACoverage(),
		"test_manifest.json":           snakeTestManifest(),
		"qsm_project_manifest.v1.json": simulatedManifestJSON(dir, "static-web", []string{"index.html", "style.css", "game.js", "test_manifest.json"}, []string{"node --check game.js", "node qa_static_smoke.cjs"}, []string{"node", "playwright"}, "index.html"),
		"README.md":                    snakeReadme(p),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

func snakeTestManifest() string {
	return `{"schema":"qsm.test_manifest.v1","product_type":"static-web","commands":[{"name":"static behavior smoke","kind":"browser","cmd":["node","qa_static_smoke.cjs"],"timeout_seconds":30},{"name":"static coverage contract","kind":"coverage","cmd":["node","qa_static_coverage.cjs"],"timeout_seconds":30}]}`
}

func genericTestManifest() string {
	return `{"schema":"qsm.test_manifest.v1","product_type":"node","commands":[{"name":"generic behavior smoke","kind":"test","cmd":["node","qa_generic_smoke.cjs"],"timeout_seconds":30}]}`
}

func simulatedProductKind(strategy string) string {
	lower := strings.ToLower(strategy)
	for _, kind := range []string{"static-web", "cli-tool", "go-service", "python-package", "node-fullstack", "data-transform"} {
		if strings.Contains(lower, kind) || strings.Contains(lower, "product_kind="+kind) || strings.Contains(lower, "product kind: "+kind) {
			return kind
		}
	}
	switch {
	case strings.Contains(lower, "snake") || strings.Contains(lower, "static browser") || strings.Contains(lower, "static web"):
		return "static-web"
	case strings.Contains(lower, "http service") || strings.Contains(lower, "go service"):
		return "go-service"
	case strings.Contains(lower, "python package"):
		return "python-package"
	case strings.Contains(lower, "node fullstack") || strings.Contains(lower, "frontend/backend"):
		return "node-fullstack"
	case strings.Contains(lower, "csv") || strings.Contains(lower, "data transform"):
		return "data-transform"
	case strings.Contains(lower, "cli") || strings.Contains(lower, "command-line"):
		return "cli-tool"
	default:
		return "generic"
	}
}

func simulatedManifestJSON(productPath, kind string, expected, tests, runtime []string, delivery string) string {
	citations := []string{"wiki_item:qsm-nodekit"}
	if ids := cacheIDsFromRoomMemory(filepath.Join(filepath.Dir(productPath), ".qsm_memory", "CACHE.md")); len(ids) > 0 {
		citations = []string{"cache_item:" + ids[0]}
	}
	data, _ := json.MarshalIndent(productmanifest.Manifest{
		Version:             productmanifest.Schema,
		ProductKind:         kind,
		Entrypoints:         []string{delivery},
		ExpectedArtifacts:   expected,
		BuildCommands:       []string{},
		TestCommands:        tests,
		CoverageCommand:     firstString(tests),
		SmokeCommands:       tests,
		RuntimeRequirements: runtime,
		DeliveryPath:        delivery,
		MemoryCitations:     citations,
	}, "", "  ")
	return string(data)
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func genericQASmoke() string {
	return `const fs = require("fs");
const readme = fs.readFileSync("README.md", "utf8");
for (const token of ["# QSM Product", "Generated by", "Strategy:"]) {
  if (!readme.includes(token)) throw new Error(` + "`README missing ${token}`" + `);
}
console.log("generic behavior smoke passed");
`
}

func writeCLIProduct(dir string, p Position) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	files := map[string]string{
		"README.md":                    "# QSM Notes CLI\n\nA small command-line notes product with malformed input handling.\n",
		"cli.js":                       cliJS(),
		"cli.test.js":                  cliTestJS(),
		"package.json":                 `{"name":"qsm-notes-cli","version":"0.1.0","type":"commonjs","scripts":{"test":"node --test"}}`,
		"test_manifest.json":           `{"schema":"qsm.test_manifest.v1","product_type":"node","commands":[{"name":"cli malformed input tests","kind":"test","cmd":["node","--test"],"timeout_seconds":30},{"name":"cli smoke","kind":"smoke","cmd":["node","cli.js","add","hello"],"timeout_seconds":30}]}`,
		"qsm_project_manifest.v1.json": simulatedManifestJSON(dir, "cli-tool", []string{"cli.js", "cli.test.js", "package.json", "test_manifest.json"}, []string{"node --test"}, []string{"node"}, "cli.js"),
	}
	return writeProductFiles(dir, files)
}

func cliJS() string {
	return `function run(argv) {
  const [cmd, ...rest] = argv;
  if (cmd === "add") {
    const note = rest.join(" ").trim();
    if (!note) return { code: 2, output: "error: note text required" };
    return { code: 0, output: "added: " + note };
  }
  if (cmd === "list") return { code: 0, output: "notes: []" };
  return { code: 2, output: "error: unknown command" };
}
if (require.main === module) {
  const result = run(process.argv.slice(2));
  console.log(result.output);
  process.exit(result.code);
}
module.exports = { run };
`
}

func cliTestJS() string {
	return `const test = require("node:test");
const assert = require("node:assert/strict");
const { run } = require("./cli.js");

test("adds note text", () => {
  assert.deepEqual(run(["add", "alpha"]), { code: 0, output: "added: alpha" });
});

test("rejects malformed add", () => {
  assert.equal(run(["add"]).code, 2);
});

test("rejects unknown command", () => {
  assert.equal(run(["wat"]).code, 2);
});
`
}

func writeGoServiceProduct(dir string, p Position) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	files := map[string]string{
		"README.md":                    "# QSM Go Service\n\nTiny HTTP service with health and echo handlers.\n",
		"go.mod":                       "module qsm-go-service\n\ngo 1.22\n",
		"main.go":                      goServiceMain(),
		"main_test.go":                 goServiceTest(),
		"test_manifest.json":           `{"schema":"qsm.test_manifest.v1","product_type":"go","commands":[{"name":"go service tests","kind":"test","cmd":["go","test","./..."],"timeout_seconds":60}]}`,
		"qsm_project_manifest.v1.json": simulatedManifestJSON(dir, "go-service", []string{"go.mod", "main.go", "main_test.go", "test_manifest.json"}, []string{"go test ./..."}, []string{"go"}, "main.go"),
	}
	return writeProductFiles(dir, files)
}

func goServiceMain() string {
	return `package main

import (
	"encoding/json"
	"net/http"
)

func health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func echo(w http.ResponseWriter, r *http.Request) {
	value := r.URL.Query().Get("q")
	if value == "" {
		http.Error(w, "missing q", http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"echo": value})
}

func routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/echo", echo)
	return mux
}

func main() {
	_ = http.ListenAndServe(":8080", routes())
}
`
}

func goServiceTest() string {
	return `package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	rr := httptest.NewRecorder()
	routes().ServeHTTP(rr, httptest.NewRequest("GET", "/health", nil))
	if rr.Code != http.StatusOK || strings.TrimSpace(rr.Body.String()) != "ok" {
		t.Fatalf("unexpected health response: %d %q", rr.Code, rr.Body.String())
	}
}

func TestEchoRequiresInput(t *testing.T) {
	rr := httptest.NewRecorder()
	routes().ServeHTTP(rr, httptest.NewRequest("GET", "/echo", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestEcho(t *testing.T) {
	rr := httptest.NewRecorder()
	routes().ServeHTTP(rr, httptest.NewRequest("GET", "/echo?q=alpha", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "alpha") {
		t.Fatalf("unexpected echo response: %d %q", rr.Code, rr.Body.String())
	}
}
`
}

func writePythonPackageProduct(dir string, p Position) error {
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0755); err != nil {
		return err
	}
	files := map[string]string{
		"README.md":                    "# QSM Python Package\n\nSmall parser package with pytest coverage hooks.\n",
		"pyproject.toml":               `[project]` + "\n" + `name = "qsm_textstats"` + "\n" + `version = "0.1.0"` + "\n",
		"textstats.py":                 pythonPackageCode(),
		"tests/test_textstats.py":      pythonPackageTests(),
		"test_manifest.json":           `{"schema":"qsm.test_manifest.v1","product_type":"python","commands":[{"name":"pytest package tests","kind":"test","cmd":["python3","-m","pytest","-q"],"timeout_seconds":60}]}`,
		"qsm_project_manifest.v1.json": simulatedManifestJSON(dir, "python-package", []string{"textstats.py", "tests/test_textstats.py", "pyproject.toml", "test_manifest.json"}, []string{"python3 -m pytest -q"}, []string{"python3", "pytest"}, "textstats.py"),
	}
	return writeProductFiles(dir, files)
}

func pythonPackageCode() string {
	return `def count_words(text: str) -> int:
    if not isinstance(text, str):
        raise TypeError("text must be a string")
    return len([part for part in text.split() if part])


def normalize_title(text: str) -> str:
    if not text or not text.strip():
        raise ValueError("title is required")
    return " ".join(text.strip().split()).title()
`
}

func pythonPackageTests() string {
	return `import pytest

from textstats import count_words, normalize_title


def test_count_words():
    assert count_words("alpha beta") == 2


def test_count_words_rejects_non_string():
    with pytest.raises(TypeError):
        count_words(None)


def test_normalize_title():
    assert normalize_title("  hello   world ") == "Hello World"


def test_normalize_title_rejects_empty():
    with pytest.raises(ValueError):
        normalize_title(" ")
`
}

func writeNodeFullstackProduct(dir string, p Position) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	files := map[string]string{
		"README.md":                    "# QSM Node Fullstack\n\nMinimal backend/frontend contract with node tests.\n",
		"server.js":                    nodeServerJS(),
		"server.test.js":               nodeServerTestJS(),
		"qa_frontend_smoke.cjs":        nodeFrontendSmokeJS(),
		"index.html":                   nodeFullstackHTML(),
		"package.json":                 `{"name":"qsm-node-fullstack","version":"0.1.0","type":"commonjs","scripts":{"test":"node --test"}}`,
		"test_manifest.json":           `{"schema":"qsm.test_manifest.v1","product_type":"node","commands":[{"name":"node fullstack tests","kind":"test","cmd":["node","--test"],"timeout_seconds":60},{"name":"frontend asset smoke","kind":"browser","cmd":["node","qa_frontend_smoke.cjs"],"timeout_seconds":30}]}`,
		"qsm_project_manifest.v1.json": simulatedManifestJSON(dir, "node-fullstack", []string{"server.js", "server.test.js", "qa_frontend_smoke.cjs", "index.html", "package.json", "test_manifest.json"}, []string{"node --test", "node qa_frontend_smoke.cjs"}, []string{"node"}, "server.js"),
	}
	return writeProductFiles(dir, files)
}

func nodeServerJS() string {
	return `function handle(pathname) {
  if (pathname === "/api/health") return { status: 200, body: { ok: true } };
  if (pathname === "/api/items") return { status: 200, body: { items: [] } };
  return { status: 404, body: { error: "not found" } };
}
module.exports = { handle };
`
}

func nodeServerTestJS() string {
	return `const test = require("node:test");
const assert = require("node:assert/strict");
const { handle } = require("./server.js");

test("health endpoint", () => {
  assert.deepEqual(handle("/api/health"), { status: 200, body: { ok: true } });
});

test("items endpoint", () => {
  assert.deepEqual(handle("/api/items").body.items, []);
});

test("404 endpoint", () => {
  assert.equal(handle("/missing").status, 404);
});
`
}

func nodeFrontendSmokeJS() string {
	return `const fs = require("fs");
const html = fs.readFileSync("index.html", "utf8");
for (const token of ["/api/health", "addEventListener", "QSM Fullstack"]) {
  if (!html.includes(token)) throw new Error("missing frontend token " + token);
}
console.log("frontend smoke passed");
`
}

func nodeFullstackHTML() string {
	return `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>QSM Fullstack</title></head>
<body>
  <main>
    <h1>QSM Fullstack</h1>
    <button id="health">Health</button>
    <pre id="out">/api/health</pre>
  </main>
  <script>
    document.getElementById("health").addEventListener("click", () => {
      document.getElementById("out").textContent = "/api/health ok";
    });
  </script>
</body>
</html>
`
}

func writeDataTransformProduct(dir string, p Position) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	files := map[string]string{
		"README.md":                    "# QSM Data Transform\n\nCSV transformer with malformed row handling.\n",
		"transform.js":                 dataTransformJS(),
		"transform.test.js":            dataTransformTestJS(),
		"package.json":                 `{"name":"qsm-data-transform","version":"0.1.0","type":"commonjs","scripts":{"test":"node --test"}}`,
		"test_manifest.json":           `{"schema":"qsm.test_manifest.v1","product_type":"node","commands":[{"name":"data transform edge tests","kind":"test","cmd":["node","--test"],"timeout_seconds":60}]}`,
		"qsm_project_manifest.v1.json": simulatedManifestJSON(dir, "data-transform", []string{"transform.js", "transform.test.js", "package.json", "test_manifest.json"}, []string{"node --test"}, []string{"node"}, "transform.js"),
	}
	return writeProductFiles(dir, files)
}

func dataTransformJS() string {
	return `function parseCSV(input) {
  if (typeof input !== "string") throw new TypeError("input must be a string");
  return input.trim().split(/\r?\n/).filter(Boolean).map((line, index) => {
    const parts = line.split(",");
    if (parts.length !== 2 || !parts[0] || !parts[1]) {
      return { index, ok: false, error: "malformed row" };
    }
    return { index, ok: true, name: parts[0].trim(), value: Number(parts[1]) };
  });
}
module.exports = { parseCSV };
`
}

func dataTransformTestJS() string {
	return `const test = require("node:test");
const assert = require("node:assert/strict");
const { parseCSV } = require("./transform.js");

test("parses valid rows", () => {
  assert.deepEqual(parseCSV("a,1")[0], { index: 0, ok: true, name: "a", value: 1 });
});

test("keeps malformed rows as errors", () => {
  assert.equal(parseCSV("bad")[0].ok, false);
});

test("rejects non-string input", () => {
  assert.throws(() => parseCSV(null), TypeError);
});
`
}

func writeProductFiles(dir string, files map[string]string) error {
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

func cacheIDsFromRoomMemory(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	matches := roomCacheIDPattern.FindAllStringSubmatch(string(data), -1)
	var ids []string
	seen := map[string]bool{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		id := strings.TrimSpace(match[1])
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func snakeQASmoke() string {
	return `const fs = require("fs");
const required = ["index.html", "style.css", "game.js"];
for (const file of required) {
  if (!fs.existsSync(file)) {
    throw new Error(` + "`missing ${file}`" + `);
  }
}
const html = fs.readFileSync("index.html", "utf8");
const js = fs.readFileSync("game.js", "utf8");
for (const token of ["<canvas", "game.js", "style.css"]) {
  if (!html.includes(token)) {
    throw new Error(` + "`index.html missing ${token}`" + `);
  }
}
for (const token of ["getContext", "addEventListener", "setInterval", "restart"]) {
  if (!js.includes(token)) {
    throw new Error(` + "`game.js missing behavior token ${token}`" + `);
  }
}
for (const snippet of [
  "part.x === food.x && part.y === food.y",
  "next.x === food.x && next.y === food.y",
  "event.code === \"Space\"",
  "index === 0"
]) {
  if (!js.includes(snippet)) {
    throw new Error(` + "`game.js missing critical behavior snippet ${snippet}`" + `);
  }
}
console.log("static behavior smoke passed");
`
}

func snakeQACoverage() string {
	return `const fs = require("fs");
const js = fs.readFileSync("game.js", "utf8");
const requiredFunctions = ["function reset()", "function placeFood()", "function step()", "function draw()"];
const requiredEvents = ["addEventListener(\"keydown\"", "addEventListener(\"click\"", "setInterval(step"];
const requiredBranches = ["hitWall", "hitSelf", "paused || gameOver", "reversing"];
for (const token of [...requiredFunctions, ...requiredEvents, ...requiredBranches]) {
  if (!js.includes(token)) throw new Error(` + "`coverage contract missing ${token}`" + `);
}
console.log("static coverage contract passed");
`
}

func snakeHTML(p Position) string {
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Quantum Snake - ` + p.ID + `</title>
  <link rel="stylesheet" href="style.css">
</head>
<body>
  <main class="shell">
    <header>
      <h1>Quantum Snake</h1>
      <p>` + p.Name + `</p>
    </header>
    <section class="hud">
      <span>Score: <strong id="score">0</strong></span>
      <span>Best: <strong id="best">0</strong></span>
      <button id="restart" type="button">Restart</button>
    </section>
    <canvas id="board" width="480" height="480" aria-label="Snake game board"></canvas>
    <script>
      (() => {
        const bootCanvas = document.getElementById("board");
        const bootCtx = bootCanvas && bootCanvas.getContext("2d");
        if (bootCtx) {
          bootCtx.fillStyle = "#0d1f1a";
          bootCtx.fillRect(0, 0, bootCanvas.width, bootCanvas.height);
          bootCtx.fillStyle = "#2a9d8f";
          bootCtx.fillRect(180, 180, 96, 96);
        }
      })();
    </script>
    <p class="hint">Use arrow keys or WASD. Space pauses.</p>
  </main>
  <script src="game.js"></script>
</body>
</html>
`
}

func snakeCSS() string {
	return `:root {
  color-scheme: dark;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  background: #08110f;
  color: #eefbf5;
}

* { box-sizing: border-box; }

body {
  margin: 0;
  min-height: 100vh;
  display: grid;
  place-items: center;
  background:
    linear-gradient(135deg, rgba(42, 157, 143, 0.18), transparent 45%),
    radial-gradient(circle at 75% 15%, rgba(233, 196, 106, 0.16), transparent 28%),
    #08110f;
}

.shell {
  width: min(92vw, 560px);
  display: grid;
  gap: 14px;
}

header {
  display: flex;
  justify-content: space-between;
  align-items: end;
  gap: 16px;
}

h1 { margin: 0; font-size: 2rem; letter-spacing: 0; }
p { margin: 0; color: #a8c8bd; }

.hud {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid rgba(238, 251, 245, 0.14);
  background: rgba(8, 17, 15, 0.72);
}

button {
  border: 1px solid rgba(238, 251, 245, 0.18);
  background: #e9c46a;
  color: #1b1b13;
  padding: 8px 12px;
  font-weight: 700;
  cursor: pointer;
}

canvas {
  width: 100%;
  aspect-ratio: 1;
  border: 1px solid rgba(238, 251, 245, 0.18);
  background: #0d1f1a;
}

.hint { text-align: center; font-size: 0.95rem; }
`
}

func snakeJS() string {
	return `const canvas = document.getElementById("board");
const ctx = canvas.getContext("2d");
const scoreEl = document.getElementById("score");
const bestEl = document.getElementById("best");
const restartBtn = document.getElementById("restart");

const size = 24;
const cells = canvas.width / size;
const dirs = {
  ArrowUp: { x: 0, y: -1 }, KeyW: { x: 0, y: -1 },
  ArrowDown: { x: 0, y: 1 }, KeyS: { x: 0, y: 1 },
  ArrowLeft: { x: -1, y: 0 }, KeyA: { x: -1, y: 0 },
  ArrowRight: { x: 1, y: 0 }, KeyD: { x: 1, y: 0 },
};

let snake;
let dir;
let nextDir;
let food;
let score;
let best = Number(localStorage.getItem("quantum-snake-best") || 0);
let paused = false;
let gameOver = false;

bestEl.textContent = best;

function reset() {
  snake = [{ x: 8, y: 10 }, { x: 7, y: 10 }, { x: 6, y: 10 }];
  dir = { x: 1, y: 0 };
  nextDir = dir;
  score = 0;
  paused = false;
  gameOver = false;
  scoreEl.textContent = score;
  placeFood();
  draw();
}

function placeFood() {
  do {
    food = {
      x: Math.floor(Math.random() * cells),
      y: Math.floor(Math.random() * cells),
    };
  } while (snake.some(part => part.x === food.x && part.y === food.y));
}

function step() {
  if (paused || gameOver) {
    draw();
    return;
  }
  dir = nextDir;
  const head = snake[0];
  const next = { x: head.x + dir.x, y: head.y + dir.y };
  const hitWall = next.x < 0 || next.y < 0 || next.x >= cells || next.y >= cells;
  const hitSelf = snake.some(part => part.x === next.x && part.y === next.y);
  if (hitWall || hitSelf) {
    gameOver = true;
    best = Math.max(best, score);
    localStorage.setItem("quantum-snake-best", String(best));
    bestEl.textContent = best;
    draw();
    return;
  }
  snake.unshift(next);
  if (next.x === food.x && next.y === food.y) {
    score += 10;
    scoreEl.textContent = score;
    placeFood();
  } else {
    snake.pop();
  }
  draw();
}

function draw() {
  ctx.fillStyle = "#0d1f1a";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.strokeStyle = "rgba(238, 251, 245, 0.05)";
  for (let i = 0; i <= cells; i++) {
    ctx.beginPath();
    ctx.moveTo(i * size, 0);
    ctx.lineTo(i * size, canvas.height);
    ctx.moveTo(0, i * size);
    ctx.lineTo(canvas.width, i * size);
    ctx.stroke();
  }
  ctx.fillStyle = "#e76f51";
  ctx.fillRect(food.x * size + 4, food.y * size + 4, size - 8, size - 8);
  snake.forEach((part, index) => {
    ctx.fillStyle = index === 0 ? "#2a9d8f" : "#7dd3c7";
    ctx.fillRect(part.x * size + 2, part.y * size + 2, size - 4, size - 4);
  });
  if (paused || gameOver) {
    ctx.fillStyle = "rgba(8, 17, 15, 0.72)";
    ctx.fillRect(0, canvas.height / 2 - 36, canvas.width, 72);
    ctx.fillStyle = "#eefbf5";
    ctx.font = "700 28px system-ui";
    ctx.textAlign = "center";
    ctx.fillText(gameOver ? "Game Over" : "Paused", canvas.width / 2, canvas.height / 2 + 9);
  }
}

document.addEventListener("keydown", event => {
  if (event.code === "Space") {
    paused = !paused;
    draw();
    return;
  }
  const wanted = dirs[event.code];
  if (!wanted) return;
  const reversing = wanted.x + dir.x === 0 && wanted.y + dir.y === 0;
  if (!reversing) nextDir = wanted;
});

restartBtn.addEventListener("click", reset);
reset();
setInterval(step, 120);
`
}

func snakeReadme(p Position) string {
	return "# Quantum Snake\n\nGenerated by " + p.ID + ". Open `index.html` in a browser and use arrow keys or WASD.\n"
}

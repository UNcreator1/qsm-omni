package tester

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/sandbox"
)

const (
	SchemaManifest = "qsm.test_manifest.v1"
	SchemaReport   = "qsm.test_report.v1"
)

type Manifest struct {
	Schema        string            `json:"schema"`
	ProductType   string            `json:"product_type,omitempty"`
	Commands      []ManifestCommand `json:"commands,omitempty"`
	ExpectedFiles []string          `json:"expected_files,omitempty"`
	Web           ManifestWeb       `json:"web,omitempty"`
}

type ManifestWeb struct {
	Entry               string `json:"entry,omitempty"`
	RequiresInteraction bool   `json:"requires_interaction,omitempty"`
}

type ManifestCommand struct {
	Name           string   `json:"name"`
	Cmd            []string `json:"cmd"`
	CWD            string   `json:"cwd,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

type TestReport struct {
	Schema      string          `json:"schema"`
	Passed      bool            `json:"passed"`
	ProductType string          `json:"product_type"`
	Sandbox     string          `json:"sandbox,omitempty"`
	Summary     TestSummary     `json:"summary"`
	Commands    []CommandResult `json:"commands,omitempty"`
	Security    SecurityReport  `json:"security"`
	Warnings    []string        `json:"warnings,omitempty"`
	Errors      []string        `json:"errors,omitempty"`
	Path        string          `json:"path,omitempty"`
	TracePath   string          `json:"trace_path,omitempty"`
}

type TestSummary struct {
	Commands       int `json:"commands"`
	PassedCommands int `json:"passed_commands"`
	FailedCommands int `json:"failed_commands"`
	Tests          int `json:"tests"`
	PassedTests    int `json:"passed_tests"`
	FailedTests    int `json:"failed_tests"`
	SkippedTests   int `json:"skipped_tests"`
}

type CommandResult struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind,omitempty"`
	Origin     string   `json:"origin,omitempty"`
	Sandbox    string   `json:"sandbox,omitempty"`
	Cmd        []string `json:"cmd"`
	CWD        string   `json:"cwd"`
	ExitCode   int      `json:"exit_code"`
	DurationMS int64    `json:"duration_ms"`
	StdoutPath string   `json:"stdout_path,omitempty"`
	StderrPath string   `json:"stderr_path,omitempty"`
	Tests      int      `json:"tests,omitempty"`
	Passed     int      `json:"passed,omitempty"`
	Failed     int      `json:"failed,omitempty"`
	Skipped    int      `json:"skipped,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type SecurityReport struct {
	Schema        string            `json:"schema"`
	Passed        bool              `json:"passed"`
	Findings      []SecurityFinding `json:"findings,omitempty"`
	CriticalCount int               `json:"critical_count"`
	HighCount     int               `json:"high_count"`
	MediumCount   int               `json:"medium_count"`
	LowCount      int               `json:"low_count"`
}

type SecurityFinding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
	Evidence string `json:"evidence,omitempty"`
}

type commandSpec struct {
	name    string
	kind    string
	cmd     []string
	cwd     string
	timeout time.Duration
	parser  string
	origin  string
}

type VerifyOptions struct {
	SandboxBackend string
}

type detector struct {
	productType             string
	fileCount               int
	implementationFileCount int
	hasIndex                bool
	hasGo                   bool
	hasGoMod                bool
	hasPython               bool
	hasPyTests              bool
	hasPackage              bool
	hasPkgLock              bool
	hasJS                   bool
	hasNodeTest             bool
	hasReqs                 bool
	jsFiles                 []string
	testFiles               []string
}

func Verify(ctx context.Context, room, productPath string) (*TestReport, error) {
	return VerifyWithOptions(ctx, room, productPath, VerifyOptions{})
}

func VerifyWithOptions(ctx context.Context, room, productPath string, options VerifyOptions) (*TestReport, error) {
	roomAbs, err := filepath.Abs(room)
	if err != nil {
		return nil, err
	}
	productAbs, err := filepath.Abs(productPath)
	if err != nil {
		return nil, err
	}
	if !inside(roomAbs, productAbs) {
		return nil, fmt.Errorf("product path escapes room: %s", productPath)
	}
	reportDir := filepath.Join(roomAbs, ".qsm_test")
	logDir := filepath.Join(reportDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	tracePath := filepath.Join(reportDir, "trace.jsonl")
	runner := sandbox.NewRunner(firstNonEmpty(options.SandboxBackend, os.Getenv("QSM_TEST_SANDBOX"), sandbox.BackendRoom))
	d, err := detect(productAbs)
	if err != nil {
		return nil, err
	}
	report := &TestReport{
		Schema:      SchemaReport,
		ProductType: d.productType,
		Sandbox:     runner.Backend(),
		Path:        filepath.Join(reportDir, "qsm_test_report.json"),
		TracePath:   tracePath,
	}
	report.Warnings = append(report.Warnings, fakeTestWarnings(productAbs, d.testFiles)...)
	if d.fileCount > 0 && d.implementationFileCount == 0 {
		report.Errors = append(report.Errors, "product contains only QSM evidence/checklist artifacts; missing deliverable files")
	}
	report.Security = scanSecurity(productAbs)
	for _, finding := range report.Security.Findings {
		switch finding.Severity {
		case "critical", "high":
			report.Errors = append(report.Errors, fmt.Sprintf("security %s %s:%d %s", finding.Severity, finding.File, finding.Line, finding.Message))
		case "medium", "low":
			report.Warnings = append(report.Warnings, fmt.Sprintf("security %s %s:%d %s", finding.Severity, finding.File, finding.Line, finding.Message))
		}
	}
	commands := autoCommands(ctx, roomAbs, productAbs, d, reportDir, report, runner)
	manifest, manifestWarnings, manifestErrors := readManifest(roomAbs, productAbs)
	report.Warnings = append(report.Warnings, manifestWarnings...)
	report.Errors = append(report.Errors, manifestErrors...)
	commands = append(commands, manifestCommands(roomAbs, productAbs, manifest, report)...)
	for i, spec := range commands {
		result := runCommand(ctx, roomAbs, logDir, tracePath, i+1, spec, runner)
		report.Commands = append(report.Commands, result)
		report.Summary.Commands++
		if result.ExitCode == 0 && result.Error == "" {
			report.Summary.PassedCommands++
		} else {
			report.Summary.FailedCommands++
			msg := result.Error
			if msg == "" {
				msg = fmt.Sprintf("%s exited with code %d", result.Name, result.ExitCode)
			}
			report.Errors = append(report.Errors, msg)
		}
		report.Summary.Tests += result.Tests
		report.Summary.PassedTests += result.Passed
		report.Summary.FailedTests += result.Failed
		report.Summary.SkippedTests += result.Skipped
	}
	enforceQualityGate(report, d)
	report.Passed = len(report.Errors) == 0 && report.Summary.FailedCommands == 0
	if err := writeJSON(report.Path, report); err != nil {
		return report, err
	}
	return report, nil
}

func enforceQualityGate(report *TestReport, d detector) {
	switch d.productType {
	case "generic":
		return
	case "static-web":
		if !hasPassedCommandKind(report, "browser") {
			report.Errors = append(report.Errors, "static-web product requires a passing browser smoke test or manifest browser command")
		}
		if d.hasJS && !hasPassedCommandKind(report, "lint") {
			report.Errors = append(report.Errors, "static-web product has JavaScript but no passing JavaScript lint/syntax check")
		}
	case "go", "python", "node":
		if len(d.testFiles) == 0 && !manifestPassedKind(report, "test") {
			report.Errors = append(report.Errors, d.productType+" product has no dedicated test files or manifest test command")
		}
		if !hasPassedCommandKind(report, "test") && !manifestPassedKind(report, "test") {
			report.Errors = append(report.Errors, d.productType+" product requires at least one passing test command")
		}
	}
}

func hasPassedCommandKind(report *TestReport, kind string) bool {
	for _, command := range report.Commands {
		if command.Kind == kind && command.ExitCode == 0 && command.Error == "" {
			return true
		}
	}
	return false
}

func manifestPassedKind(report *TestReport, kind string) bool {
	for _, command := range report.Commands {
		if command.Kind == kind && command.Origin == "manifest" && command.ExitCode == 0 && command.Error == "" {
			return true
		}
	}
	return false
}

func detect(product string) (detector, error) {
	d := detector{productType: "generic"}
	err := filepath.WalkDir(product, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "node_modules" || name == "__pycache__" || name == ".qsm_test" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(product, path)
		base := entry.Name()
		ext := strings.ToLower(filepath.Ext(base))
		d.fileCount++
		if !qsmEvidenceArtifact(rel) {
			d.implementationFileCount++
		}
		switch {
		case rel == "index.html":
			d.hasIndex = true
		case base == "go.mod":
			d.hasGoMod = true
		case ext == ".go":
			d.hasGo = true
			if strings.HasSuffix(base, "_test.go") {
				d.testFiles = append(d.testFiles, path)
			}
		case ext == ".py":
			d.hasPython = true
			if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") || strings.Contains(rel, string(os.PathSeparator)+"tests"+string(os.PathSeparator)) {
				d.hasPyTests = true
				d.testFiles = append(d.testFiles, path)
			}
		case base == "package.json":
			d.hasPackage = true
		case base == "package-lock.json" || base == "npm-shrinkwrap.json":
			d.hasPkgLock = true
		case base == "requirements.txt":
			d.hasReqs = true
		case ext == ".js" || ext == ".mjs" || ext == ".cjs":
			d.hasJS = true
			d.jsFiles = append(d.jsFiles, path)
			if strings.Contains(strings.ToLower(base), "test") || strings.Contains(strings.ToLower(base), "spec") {
				d.hasNodeTest = true
				d.testFiles = append(d.testFiles, path)
			}
		}
		return nil
	})
	if err != nil {
		return d, err
	}
	sort.Strings(d.jsFiles)
	sort.Strings(d.testFiles)
	switch {
	case d.hasIndex:
		d.productType = "static-web"
	case d.hasGo || d.hasGoMod:
		d.productType = "go"
	case d.hasPython:
		d.productType = "python"
	case d.hasPackage || d.hasJS:
		d.productType = "node"
	}
	return d, nil
}

func qsmEvidenceArtifact(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(rel))
	switch clean {
	case "FORCE_REQUIREMENTS_CHECKLIST.md", "QSM_FORCE_CHECKLIST.json", "evidence.json", "test_manifest.json", "qsm_project_manifest.v1.json", "qsm_project_manifest.json":
		return true
	default:
		return strings.HasPrefix(clean, ".qsm_test/")
	}
}

func autoCommands(ctx context.Context, room, product string, d detector, reportDir string, report *TestReport, runner sandbox.Runner) []commandSpec {
	var commands []commandSpec
	if d.hasGo || d.hasGoMod {
		if _, err := exec.LookPath("go"); err != nil {
			report.Errors = append(report.Errors, "go files detected but go executable is not available")
		} else if d.hasGoMod {
			commands = append(commands, commandSpec{name: "go tests", kind: "test", cmd: []string{"go", "test", "./...", "-json"}, cwd: product, timeout: 90 * time.Second, parser: "go-json", origin: "qsm"})
		} else {
			commands = append(commands, commandSpec{name: "go tests", kind: "test", cmd: []string{"go", "test", ".", "-json"}, cwd: product, timeout: 90 * time.Second, parser: "go-json", origin: "qsm"})
		}
		if _, err := exec.LookPath("gofmt"); err == nil {
			commands = append(commands, commandSpec{name: "gofmt check", kind: "lint", cmd: []string{"gofmt", "-l", "."}, cwd: product, timeout: 30 * time.Second, parser: "empty-stdout", origin: "qsm"})
		}
		if d.hasGoMod {
			commands = append(commands, commandSpec{name: "go vet", kind: "security", cmd: []string{"go", "vet", "./..."}, cwd: product, timeout: 90 * time.Second, origin: "qsm"})
			if _, err := exec.LookPath("govulncheck"); err == nil {
				commands = append(commands, commandSpec{name: "govulncheck", kind: "security", cmd: []string{"govulncheck", "./..."}, cwd: product, timeout: 120 * time.Second, origin: "qsm"})
			} else {
				report.Warnings = append(report.Warnings, "govulncheck is not available; skipped Go vulnerability scan")
			}
		}
	}
	if d.hasPython {
		py := pythonExecutableForRunner(runner)
		if py == "" {
			report.Errors = append(report.Errors, "python files detected but python executable is not available")
		} else {
			commands = append(commands, commandSpec{name: "python compileall", kind: "lint", cmd: []string{py, "-m", "compileall", "-q", "product"}, cwd: room, timeout: 60 * time.Second, origin: "qsm"})
			if d.hasPyTests {
				commands = append(commands, commandSpec{name: "pytest", kind: "test", cmd: []string{py, "-m", "pytest", "-q"}, cwd: product, timeout: 90 * time.Second, parser: "pytest", origin: "qsm"})
			}
			if d.hasReqs {
				if _, err := exec.LookPath("pip-audit"); err == nil {
					commands = append(commands, commandSpec{name: "pip-audit", kind: "security", cmd: []string{"pip-audit", "-r", "requirements.txt"}, cwd: product, timeout: 120 * time.Second, origin: "qsm"})
				} else {
					report.Warnings = append(report.Warnings, "pip-audit is not available; skipped Python dependency vulnerability scan")
				}
			}
		}
	}
	if d.hasJS || d.hasIndex {
		if _, err := exec.LookPath("node"); err != nil {
			report.Warnings = append(report.Warnings, "node is not available; skipped JavaScript syntax checks")
		} else {
			for _, path := range d.jsFiles {
				rel, _ := filepath.Rel(product, path)
				commands = append(commands, commandSpec{name: "javascript syntax " + rel, kind: "lint", cmd: []string{"node", "--check", rel}, cwd: product, timeout: 30 * time.Second, origin: "qsm"})
			}
			if d.hasNodeTest && !packageHasTestScript(filepath.Join(product, "package.json")) {
				commands = append(commands, commandSpec{name: "node tests", kind: "test", cmd: []string{"node", "--test"}, cwd: product, timeout: 90 * time.Second, parser: "node-test", origin: "qsm"})
			}
		}
	}
	if d.hasPackage {
		if packageHasTestScript(filepath.Join(product, "package.json")) {
			if _, err := exec.LookPath("npm"); err != nil {
				report.Warnings = append(report.Warnings, "package.json has a test script but npm is not available")
			} else {
				commands = append(commands, commandSpec{name: "npm test", kind: "test", cmd: []string{"npm", "test"}, cwd: product, timeout: 120 * time.Second, parser: "generic-test", origin: "qsm"})
			}
		}
	}
	if d.hasPkgLock {
		if _, err := exec.LookPath("npm"); err != nil {
			report.Warnings = append(report.Warnings, "package lock detected but npm is not available; skipped npm audit")
		} else {
			commands = append(commands, commandSpec{name: "npm audit high", kind: "security", cmd: []string{"npm", "audit", "--audit-level=high"}, cwd: product, timeout: 120 * time.Second, origin: "qsm"})
		}
	}
	if d.hasIndex {
		if playwrightAvailableForRunner(ctx, runner, room, product) {
			scriptPath, err := writeStaticWebSmokeScript(reportDir)
			if err != nil {
				report.Errors = append(report.Errors, "cannot write Playwright smoke script: "+err.Error())
			} else {
				commands = append(commands, commandSpec{name: "playwright static smoke", kind: "browser", cmd: []string{"node", scriptPath, filepath.Join(product, "index.html"), product}, cwd: product, timeout: 60 * time.Second, origin: "qsm"})
			}
		} else {
			report.Warnings = append(report.Warnings, "Playwright is not available; skipped browser smoke")
		}
	}
	return commands
}

func readManifest(room, product string) (Manifest, []string, []string) {
	var warnings []string
	var errorsOut []string
	for _, path := range []string{filepath.Join(room, "test_manifest.json"), filepath.Join(product, "test_manifest.json")} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			errorsOut = append(errorsOut, "invalid test manifest: "+err.Error())
			return Manifest{}, warnings, errorsOut
		}
		if manifest.Schema != "" && manifest.Schema != SchemaManifest {
			warnings = append(warnings, "unknown test manifest schema: "+manifest.Schema)
		}
		return manifest, warnings, errorsOut
	}
	return Manifest{}, warnings, errorsOut
}

func manifestCommands(room, product string, manifest Manifest, report *TestReport) []commandSpec {
	var specs []commandSpec
	for _, c := range manifest.Commands {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = "manifest command"
		}
		if len(c.Cmd) == 0 {
			report.Errors = append(report.Errors, name+" has empty cmd")
			continue
		}
		if manifestCommandBanned(c.Cmd) {
			report.Errors = append(report.Errors, name+" uses a banned shell/eval command")
			continue
		}
		cwd := product
		if c.CWD != "" {
			cwd = filepath.Join(room, filepath.Clean(c.CWD))
		}
		cwdAbs, err := filepath.Abs(cwd)
		if err != nil || !inside(room, cwdAbs) {
			report.Errors = append(report.Errors, name+" cwd escapes room")
			continue
		}
		timeout := time.Duration(c.TimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		if timeout > 120*time.Second {
			timeout = 120 * time.Second
		}
		specs = append(specs, commandSpec{name: name, kind: c.Kind, cmd: c.Cmd, cwd: cwdAbs, timeout: timeout, parser: "generic-test", origin: "manifest"})
	}
	return specs
}

func runCommand(parent context.Context, room, logDir, tracePath string, index int, spec commandSpec, runner sandbox.Runner) CommandResult {
	start := time.Now()
	name := sanitizeLogName(fmt.Sprintf("%02d-%s", index, spec.name))
	stdoutPath := filepath.Join(logDir, name+".stdout.log")
	stderrPath := filepath.Join(logDir, name+".stderr.log")
	result := CommandResult{
		Name:       spec.name,
		Kind:       spec.kind,
		Origin:     spec.origin,
		Sandbox:    runner.Backend(),
		Cmd:        spec.cmd,
		CWD:        relOrAbs(room, spec.cwd),
		ExitCode:   -1,
		StdoutPath: relOrAbs(room, stdoutPath),
		StderrPath: relOrAbs(room, stderrPath),
	}
	if len(spec.cmd) == 0 {
		result.Error = "empty command"
		return result
	}
	if spec.timeout <= 0 {
		spec.timeout = 60 * time.Second
	}
	appendTrace(tracePath, map[string]any{
		"type":    "command_start",
		"name":    spec.name,
		"kind":    spec.kind,
		"sandbox": runner.Backend(),
		"cmd":     spec.cmd,
		"cwd":     relOrAbs(room, spec.cwd),
		"at":      start.UTC(),
	})
	run := runner.Run(parent, sandbox.Command{
		Name:       spec.name,
		Cmd:        spec.cmd,
		CWD:        spec.cwd,
		Room:       room,
		Timeout:    spec.timeout,
		Env:        []string{"CI=1", "NO_COLOR=1"},
		StdoutPath: stdoutPath,
		StderrPath: stderrPath,
	})
	result.ExitCode = run.ExitCode
	result.DurationMS = run.DurationMS
	result.Error = run.Error
	appendTrace(tracePath, map[string]any{
		"type":        "command_end",
		"name":        spec.name,
		"kind":        spec.kind,
		"sandbox":     runner.Backend(),
		"exit_code":   run.ExitCode,
		"duration_ms": run.DurationMS,
		"error":       run.Error,
		"at":          time.Now().UTC(),
	})
	if strings.Contains(run.Error, "timed out") {
		result.Error = "command timed out: " + spec.name
		return result
	}
	parseCommandOutput(&result, spec.parser, run.Stdout, run.Stderr)
	if spec.parser == "empty-stdout" && strings.TrimSpace(run.Stdout) != "" && result.ExitCode == 0 {
		result.ExitCode = 1
		result.Error = "gofmt reported unformatted files"
	}
	return result
}

func parseCommandOutput(result *CommandResult, parser, stdout, stderr string) {
	switch parser {
	case "go-json":
		parseGoJSON(result, stdout)
	case "pytest":
		parsePytest(result, stdout+"\n"+stderr)
	case "node-test":
		parseNodeTest(result, stdout+"\n"+stderr)
	case "generic-test":
		parseGenericTest(result, stdout+"\n"+stderr)
	}
}

func parseGoJSON(result *CommandResult, output string) {
	type event struct {
		Action string `json:"Action"`
		Test   string `json:"Test"`
	}
	seen := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		var ev event
		if json.Unmarshal(scanner.Bytes(), &ev) != nil || ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "pass", "fail", "skip":
			seen[ev.Test] = ev.Action
		}
	}
	for _, action := range seen {
		result.Tests++
		switch action {
		case "pass":
			result.Passed++
		case "fail":
			result.Failed++
		case "skip":
			result.Skipped++
		}
	}
}

func parsePytest(result *CommandResult, output string) {
	parseWordCounts(result, output, map[string]*int{
		"passed":  &result.Passed,
		"failed":  &result.Failed,
		"skipped": &result.Skipped,
		"error":   &result.Failed,
		"errors":  &result.Failed,
	})
	result.Tests = result.Passed + result.Failed + result.Skipped
}

func parseNodeTest(result *CommandResult, output string) {
	parseWordCounts(result, output, map[string]*int{
		"pass": &result.Passed, "passes": &result.Passed, "passed": &result.Passed,
		"fail": &result.Failed, "fails": &result.Failed, "failed": &result.Failed,
		"skip": &result.Skipped, "skipped": &result.Skipped,
	})
	result.Tests = result.Passed + result.Failed + result.Skipped
}

func parseGenericTest(result *CommandResult, output string) {
	parsePytest(result, output)
}

func parseWordCounts(result *CommandResult, output string, words map[string]*int) {
	re := regexp.MustCompile(`(?i)(\d+)\s+([a-z]+)`)
	for _, match := range re.FindAllStringSubmatch(output, -1) {
		if len(match) != 3 {
			continue
		}
		word := strings.ToLower(match[2])
		target := words[word]
		if target == nil {
			continue
		}
		var n int
		_, _ = fmt.Sscanf(match[1], "%d", &n)
		*target += n
	}
}

func fakeTestWarnings(product string, files []string) []string {
	var warnings []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(data))
		rel, _ := filepath.Rel(product, path)
		if !strings.Contains(lower, "assert") && !strings.Contains(lower, "expect(") && !strings.Contains(lower, "t.error") && !strings.Contains(lower, "t.fatal") {
			warnings = append(warnings, "test file has no obvious assertion: "+rel)
		}
		if strings.Contains(lower, "assert true") || strings.Contains(lower, "1 == 1") || strings.Contains(lower, "toBe(true)") {
			warnings = append(warnings, "test file may contain trivial assertion: "+rel)
		}
	}
	return warnings
}

func scanSecurity(product string) SecurityReport {
	report := SecurityReport{
		Schema: "qsm.security_report.v1",
		Passed: true,
	}
	type rule struct {
		id       string
		severity string
		re       *regexp.Regexp
		message  string
		codeOnly bool
	}
	rules := []rule{
		{"secret-private-key", "critical", regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH |DSA |)?PRIVATE KEY-----`), "private key material committed to product", false},
		{"secret-provider-key", "critical", regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*['"][^'"]{16,}['"]`), "hard-coded credential-like value", false},
		{"secret-openai", "critical", regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`), "hard-coded OpenAI-style API key", false},
		{"secret-github", "critical", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{20,}`), "hard-coded GitHub token", false},
		{"js-eval", "high", regexp.MustCompile(`\beval\s*\(`), "dynamic eval execution", true},
		{"js-function-constructor", "high", regexp.MustCompile(`\bnew\s+Function\s*\(`), "dynamic Function constructor execution", true},
		{"shell-system", "high", regexp.MustCompile(`\b(os\.system|subprocess\.[A-Za-z_]+\([^)\n]*shell\s*=\s*True|child_process\.exec\s*\()`), "shell execution without a strict command boundary", true},
		{"html-remote-script-http", "medium", regexp.MustCompile(`(?i)<script[^>]+src=["']http://`), "remote script loaded over plain HTTP", false},
		{"dom-inner-html", "medium", regexp.MustCompile(`\.innerHTML\s*=`), "innerHTML assignment requires XSS review", true},
	}
	_ = filepath.WalkDir(product, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			report.Findings = append(report.Findings, SecurityFinding{ID: "scan-read-error", Severity: "high", File: relOrAbs(product, path), Message: err.Error()})
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "__pycache__", ".qsm_test", "dist", "build", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if !securityScannableFile(entry.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			report.Findings = append(report.Findings, SecurityFinding{ID: "scan-read-error", Severity: "high", File: relOrAbs(product, path), Message: err.Error()})
			return nil
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		rel := relOrAbs(product, path)
		for i, line := range lines {
			for _, r := range rules {
				if r.codeOnly && (!codeSecurityFile(entry.Name()) || commentOnlyLine(line)) {
					continue
				}
				if r.re.MatchString(line) {
					report.Findings = append(report.Findings, SecurityFinding{
						ID:       r.id,
						Severity: r.severity,
						File:     rel,
						Line:     i + 1,
						Message:  r.message,
						Evidence: truncateLine(line, 180),
					})
				}
			}
		}
		return nil
	})
	for _, finding := range report.Findings {
		switch finding.Severity {
		case "critical":
			report.CriticalCount++
		case "high":
			report.HighCount++
		case "medium":
			report.MediumCount++
		case "low":
			report.LowCount++
		}
	}
	report.Passed = report.CriticalCount == 0 && report.HighCount == 0
	return report
}

func codeSecurityFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".py", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".html", ".sh":
		return true
	default:
		return false
	}
}

func commentOnlyLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "*") ||
		strings.HasPrefix(trimmed, "#") ||
		strings.HasPrefix(trimmed, "<!--")
}

func securityScannableFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".py", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".html", ".css", ".json", ".yaml", ".yml", ".env", ".md", ".toml", ".sh":
		return true
	default:
		return strings.HasPrefix(name, ".env")
	}
}

func truncateLine(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}

func packageHasTestScript(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	return strings.TrimSpace(pkg.Scripts["test"]) != ""
}

func playwrightAvailable(product string) bool {
	if _, err := exec.LookPath("node"); err != nil {
		return false
	}
	code := `const fs=require("fs");const path=require("path");const{createRequire}=require("module");let pw;for(const base of [process.cwd(),process.argv[1]]){try{const r=createRequire(path.join(base,"package.json"));pw=r("playwright");break}catch(e){}}if(!pw){try{pw=require("playwright")}catch(e){process.exit(1)}}const exe=pw.chromium&&pw.chromium.executablePath&&pw.chromium.executablePath();process.exit(exe&&fs.existsSync(exe)?0:1)`
	cmd := exec.Command("node", "-e", code, product)
	cmd.Dir = product
	return cmd.Run() == nil
}

func playwrightAvailableForRunner(ctx context.Context, runner sandbox.Runner, room, product string) bool {
	if runner.Backend() == sandbox.BackendRoom {
		return playwrightAvailable(product)
	}
	code := `const fs=require("fs");const path=require("path");const{createRequire}=require("module");let pw;for(const base of [process.cwd(),process.argv[1]]){try{const r=createRequire(path.join(base,"package.json"));pw=r("playwright");break}catch(e){}}if(!pw){try{pw=require("playwright")}catch(e){process.exit(1)}}const exe=pw.chromium&&pw.chromium.executablePath&&pw.chromium.executablePath();process.exit(exe&&fs.existsSync(exe)?0:1)`
	res := runner.Run(ctx, sandbox.Command{
		Name:    "playwright availability",
		Room:    room,
		CWD:     product,
		Cmd:     []string{"node", "-e", code, product},
		Timeout: 20 * time.Second,
	})
	return res.ExitCode == 0 && res.Error == ""
}

func writeStaticWebSmokeScript(reportDir string) (string, error) {
	hiddenDir := filepath.Join(reportDir, "hidden")
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(hiddenDir, "static-web-smoke.cjs")
	script := `const path = require("path");
const fs = require("fs");
const http = require("http");
const { createRequire } = require("module");

const indexPath = path.resolve(process.argv[2]);
const productPath = path.resolve(process.argv[3] || path.dirname(indexPath));
let playwright;
for (const base of [productPath, process.cwd(), __dirname]) {
  try {
    const req = createRequire(path.join(base, "package.json"));
    playwright = req("playwright");
    break;
  } catch (error) {}
}
if (!playwright) {
  playwright = require("playwright");
}

function contentType(filePath) {
  switch (path.extname(filePath).toLowerCase()) {
    case ".html": return "text/html; charset=utf-8";
    case ".js": return "text/javascript; charset=utf-8";
    case ".css": return "text/css; charset=utf-8";
    case ".json": return "application/json; charset=utf-8";
    case ".svg": return "image/svg+xml";
    case ".png": return "image/png";
    case ".jpg":
    case ".jpeg": return "image/jpeg";
    default: return "application/octet-stream";
  }
}

function startServer(root) {
  const cleanRoot = path.resolve(root);
  const server = http.createServer((req, res) => {
    try {
      const url = new URL(req.url || "/", "http://127.0.0.1");
      let pathname = decodeURIComponent(url.pathname);
      if (pathname === "/") pathname = "/index.html";
      const target = path.resolve(cleanRoot, "." + pathname);
      if (target !== cleanRoot && !target.startsWith(cleanRoot + path.sep)) {
        res.writeHead(403);
        res.end("forbidden");
        return;
      }
      fs.stat(target, (statErr, stat) => {
        if (statErr || !stat.isFile()) {
          res.writeHead(404);
          res.end("not found");
          return;
        }
        res.writeHead(200, { "content-type": contentType(target) });
        fs.createReadStream(target).pipe(res);
      });
    } catch (error) {
      res.writeHead(500);
      res.end(String(error && error.message ? error.message : error));
    }
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      resolve({ server, origin: "http://127.0.0.1:" + address.port });
    });
  });
}

(async () => {
  const served = await startServer(productPath);
  const browser = await playwright.chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 960, height: 720 } });
    const errors = [];
    page.on("console", msg => {
      if (msg.type() === "error") errors.push(msg.text());
    });
    page.on("pageerror", error => errors.push(error.message));
    const relIndex = path.relative(productPath, indexPath).split(path.sep).join("/");
    await page.goto(served.origin + "/" + relIndex);
    await page.waitForLoadState("domcontentloaded");
    await page.waitForTimeout(100);
    const bodyText = (await page.textContent("body")) || "";
    if (!bodyText.trim()) throw new Error("body text is empty");
    const canvasCount = await page.locator("canvas").count();
    if (canvasCount > 0) {
      const canvasOk = await page.locator("canvas").first().evaluate(canvas => {
        if (!canvas.width || !canvas.height) return false;
        const ctx = canvas.getContext("2d");
        if (!ctx) return true;
        const probe = document.createElement("canvas");
        probe.width = Math.min(canvas.width, 64);
        probe.height = Math.min(canvas.height, 64);
        const probeCtx = probe.getContext("2d");
        if (!probeCtx) return true;
        probeCtx.drawImage(canvas, 0, 0, probe.width, probe.height);
        const data = probeCtx.getImageData(0, 0, probe.width, probe.height).data;
        for (let i = 0; i < data.length; i += 4) {
          if (data[i] !== 0 || data[i + 1] !== 0 || data[i + 2] !== 0 || data[i + 3] !== 0) return true;
        }
        return false;
      });
      if (!canvasOk) throw new Error("canvas appears blank or has zero dimensions");
    }
    const buttons = await page.locator("button").count();
    if (buttons > 0) {
      try {
        await page.locator("button").first().click({ timeout: 1000 });
      } catch (error) {
        console.warn("optional button interaction skipped: " + (error && error.message ? error.message : error));
      }
    }
    if (errors.length) throw new Error("browser console/page errors: " + errors.join("; "));
  } finally {
    await browser.close();
    await new Promise(resolve => served.server.close(resolve));
  }
})().catch(async error => {
  console.error(error && error.stack ? error.stack : String(error));
  process.exit(1);
});
`
	if err := os.WriteFile(path, []byte(script), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func pythonExecutable() string {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func pythonExecutableForRunner(runner sandbox.Runner) string {
	if runner != nil && runner.Backend() == sandbox.BackendDocker {
		return "python3"
	}
	return pythonExecutable()
}

func manifestCommandBanned(cmd []string) bool {
	if len(cmd) == 0 {
		return true
	}
	base := filepath.Base(cmd[0])
	if base == "sh" || base == "bash" || base == "zsh" || base == "fish" {
		return true
	}
	for _, arg := range cmd[1:] {
		if arg == "-c" || arg == "--command" || arg == "-e" || arg == "--eval" {
			return true
		}
	}
	return false
}

func inside(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if root == path {
		return true
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".."
}

func relOrAbs(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

func sanitizeLogName(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "command"
	}
	return out
}

func appendTrace(path string, event map[string]any) {
	if strings.TrimSpace(path) == "" {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

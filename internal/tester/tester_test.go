package tester

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyGenericProductWritesPassingReport(t *testing.T) {
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "README.md"), []byte("# ok\n"), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed {
		t.Fatalf("expected generic report to pass: %#v", report)
	}
	if report.Schema != SchemaReport || report.Path == "" {
		t.Fatalf("unexpected report identity: %#v", report)
	}
	if _, err := os.Stat(report.Path); err != nil {
		t.Fatalf("missing written report: %v", err)
	}
}

func TestVerifyStaticWebFailsBrokenJavaScript(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available")
	}
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "index.html"), []byte(`<!doctype html><html><body><script src="app.js"></script></body></html>`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "app.js"), []byte(`const broken = ;`), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatalf("expected broken JavaScript to fail: %#v", report)
	}
	if report.Summary.FailedCommands == 0 {
		t.Fatalf("expected failed command: %#v", report.Summary)
	}
}

func TestVerifyRejectsChecklistOnlyProduct(t *testing.T) {
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "FORCE_REQUIREMENTS_CHECKLIST.md"), []byte("# Force checklist\n"), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatalf("expected checklist-only product to fail: %#v", report)
	}
}

func TestVerifyRejectsCodeProductWithoutTests(t *testing.T) {
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "app.js"), []byte("console.log('ok')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatalf("expected code product without tests to fail: %#v", report)
	}
	if len(report.Errors) == 0 {
		t.Fatalf("expected quality gate errors: %#v", report)
	}
}

func TestVerifyAcceptsManifestTestForCodeProduct(t *testing.T) {
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "app.js"), []byte("console.log('ok')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"qsm.test_manifest.v1","commands":[{"name":"manifest smoke","kind":"test","cmd":["true"]}]}`
	if err := os.WriteFile(filepath.Join(room, "test_manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed {
		t.Fatalf("expected manifest test to satisfy code quality gate: %#v", report)
	}
}

func TestVerifyWithRoomSandboxRecordsCommandTrace(t *testing.T) {
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "app.js"), []byte("console.log('ok')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"qsm.test_manifest.v1","commands":[{"name":"manifest smoke","kind":"test","cmd":["true"]}]}`
	if err := os.WriteFile(filepath.Join(room, "test_manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := VerifyWithOptions(context.Background(), room, product, VerifyOptions{SandboxBackend: "room"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.Sandbox != "room" {
		t.Fatalf("expected passing room-sandbox report: %#v", report)
	}
	if report.TracePath == "" {
		t.Fatalf("expected trace path in report: %#v", report)
	}
	data, err := os.ReadFile(report.TracePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "command_start") || !strings.Contains(text, "command_end") {
		t.Fatalf("expected command trace events, got %s", text)
	}
}

func TestSecurityIgnoresDynamicEvalInCommentsAndDocs(t *testing.T) {
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "README.md"), []byte("- No eval() or dynamic code execution\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "game.js"), []byte("// No eval(), no new Function()\nconst ok = true;\n"), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if report.Security.HighCount != 0 {
		t.Fatalf("expected no high security findings from comments/docs: %#v", report.Security)
	}
}

func TestVerifyStaticWebAcceptsCenteredCanvasDrawing(t *testing.T) {
	if !playwrightAvailable(t.TempDir()) {
		t.Skip("playwright browser is not available")
	}
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "index.html"), []byte(`<!doctype html><html><body><canvas id="c" width="400" height="400"></canvas><script src="game.js"></script></body></html>`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "game.js"), []byte(`const c=document.getElementById("c");const x=c.getContext("2d");x.fillStyle="#00cc66";x.fillRect(180,180,40,40);`), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed {
		t.Fatalf("expected centered canvas drawing to pass: %#v", report)
	}
}

func TestVerifyStaticWebFailsBlankCanvas(t *testing.T) {
	if !playwrightAvailable(t.TempDir()) {
		t.Skip("playwright browser is not available")
	}
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "index.html"), []byte(`<!doctype html><html><body><canvas id="c" width="400" height="400"></canvas><script src="game.js"></script></body></html>`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "game.js"), []byte(`document.getElementById("c").getContext("2d");`), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatalf("expected blank canvas to fail: %#v", report)
	}
}

func TestVerifyPythonCompileFailsSyntaxError(t *testing.T) {
	if pythonExecutable() == "" {
		t.Skip("python not available")
	}
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "bad.py"), []byte("def nope(:\n"), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatalf("expected Python syntax failure: %#v", report)
	}
}

func TestVerifyRejectsManifestCommandOutsideRoom(t *testing.T) {
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "README.md"), []byte("# ok\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"qsm.test_manifest.v1","commands":[{"name":"escape","cmd":["true"],"cwd":"../outside"}]}`
	if err := os.WriteFile(filepath.Join(room, "test_manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatalf("expected manifest escape to fail: %#v", report)
	}
}

func TestVerifyFailsCommittedSecret(t *testing.T) {
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	source := `package main

	const apiKey = "test-secret-value-that-must-be-blocked-123456"

func main() {}
`
	if err := os.WriteFile(filepath.Join(product, "main.go"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatalf("expected committed secret to fail: %#v", report)
	}
	if report.Security.CriticalCount == 0 {
		t.Fatalf("expected critical security finding: %#v", report.Security)
	}
}

func TestVerifyWarnsMediumSecurityIssue(t *testing.T) {
	room := t.TempDir()
	product := filepath.Join(room, "product")
	if err := os.MkdirAll(product, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "index.html"), []byte(`<!doctype html><div id="app"></div><script src="app.js"></script>`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "app.js"), []byte(`document.getElementById("app").innerHTML = "<b>ok</b>";`), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), room, product)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Security.Passed {
		t.Fatalf("expected medium-only security issue not to fail scan: %#v", report.Security)
	}
	if report.Security.MediumCount == 0 {
		t.Fatalf("expected medium security finding: %#v", report.Security)
	}
}

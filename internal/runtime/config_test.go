package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSimulatedHarnessDoesNotRequireRealRuntime(t *testing.T) {
	cfg := Load(t.TempDir(), HarnessSimulated)
	if err := cfg.ValidateForRealHarness(); err != nil {
		t.Fatalf("simulated mode should not require real runtime: %v", err)
	}
}

func TestOpenCodeHarnessRequiresAPIKeyAndWiki(t *testing.T) {
	t.Setenv("QSM_9ROUTER_API_KEY", "")
	t.Setenv("QSM_OPENCODE_CONFIG", filepath.Join(t.TempDir(), "missing-opencode.json"))
	cfg := Load(t.TempDir(), HarnessOpenCode)
	if err := cfg.ValidateForRealHarness(); err == nil {
		t.Fatal("expected missing API key error")
	}
}

func TestDoctorRedactsAPIKey(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "wiki"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "wiki", "wiki.md"), []byte("# wiki"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QSM_9ROUTER_API_KEY", "secret-token")
	cfg := Load(root, HarnessOpenCode)
	checks := cfg.Doctor()
	for _, check := range checks {
		if check.Detail == "secret-token" {
			t.Fatal("doctor leaked API key")
		}
	}
}

func TestFileCheckResolvesPathExecutable(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go executable not available on PATH")
	}
	check := fileCheck("go", "go")
	if !check.OK || check.Detail == "go" {
		t.Fatalf("expected PATH executable to resolve, got %#v", check)
	}
}

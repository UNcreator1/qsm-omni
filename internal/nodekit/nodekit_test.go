package nodekit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesOpenHarnessInspiredRoomKit(t *testing.T) {
	room := t.TempDir()
	manifest, err := Write(room, Params{
		ObjectiveID: "obj-1",
		Request:     "build product",
		PositionID:  "pos-01",
		AgentID:     "alpha",
		LakePath:    filepath.Join(room, ".lake"),
		WikiPath:    filepath.Join(room, "wiki.md"),
		CachePath:   filepath.Join(room, ".qsm_memory", "CACHE.md"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != Schema || len(manifest.Skills) < 5 || len(manifest.Hooks) < 3 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	for _, path := range []string{
		filepath.Join(room, ".qsm_harness", "manifest.json"),
		filepath.Join(room, ".qsm_harness", "hooks.json"),
		filepath.Join(room, ".qsm_harness", "PERMISSIONS.md"),
		filepath.Join(room, ".qsm_harness", "MEMORY.md"),
		filepath.Join(room, ".qsm_harness", "skills", "test", "SKILL.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
	permissions, err := os.ReadFile(filepath.Join(room, ".qsm_harness", "PERMISSIONS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(permissions), "*/.ssh/*") || !strings.Contains(string(permissions), "./product") {
		t.Fatalf("permission policy missing expected rules:\n%s", permissions)
	}
}

func TestPromptMentionsKitArtifacts(t *testing.T) {
	prompt := Prompt(".qsm_harness")
	for _, want := range []string{"manifest.json", "skills", "PERMISSIONS.md", "hooks.json", "MEMORY.md"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

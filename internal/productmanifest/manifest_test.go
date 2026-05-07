package productmanifest

import (
	"path/filepath"
	"testing"
)

func TestValidateRequiresMemoryCitationAndArtifacts(t *testing.T) {
	dir := t.TempDir()
	if err := Write(filepath.Join(dir, "qsm_project_manifest.v1.json"), Manifest{
		Version:           Schema,
		ProductKind:       "cli-tool",
		ExpectedArtifacts: []string{"cli.js"},
		TestCommands:      []string{"node --test"},
		MemoryCitations:   []string{"lake_artifact:abc"},
	}); err != nil {
		t.Fatal(err)
	}
	report := Validate(dir)
	if report.Passed {
		t.Fatalf("expected missing artifact and weak memory citation to fail: %#v", report)
	}
}

func TestValidatePassesSupportedManifest(t *testing.T) {
	dir := t.TempDir()
	if err := Write(filepath.Join(dir, "qsm_project_manifest.v1.json"), Manifest{
		Version:           Schema,
		ProductKind:       "cli-tool",
		ExpectedArtifacts: []string{"cli.js"},
		TestCommands:      []string{"node --test"},
		MemoryCitations:   []string{"cache_item:abc"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := Write(filepath.Join(dir, "cli.js"), Manifest{Version: Schema, ProductKind: "data-transform", ExpectedArtifacts: []string{"cli.js"}, TestCommands: []string{"true"}, MemoryCitations: []string{"wiki_item:x"}}); err != nil {
		t.Fatal(err)
	}
	report := Validate(dir)
	if !report.Passed {
		t.Fatalf("expected valid manifest to pass: %#v", report)
	}
}

package checkpoint

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateWritesCheckpointAndManifest(t *testing.T) {
	room := t.TempDir()
	if err := os.WriteFile(filepath.Join(room, "plan.md"), []byte("plan"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(room, "product"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(room, "product", "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	entry, err := Create(room, "build")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Phase != "build" || entry.FileCount != 2 || entry.Bytes == 0 || entry.SHA256 == "" {
		t.Fatalf("unexpected entry: %#v", entry)
	}
	if _, err := os.Stat(filepath.Join(room, "checkpoints", "build.tar.gz")); err != nil {
		t.Fatalf("missing checkpoint: %v", err)
	}
	manifest, err := ReadManifest(room)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != Schema || len(manifest.Entries) != 1 || manifest.Entries[0].Phase != "build" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	names := tarNames(t, entry.Path)
	if !contains(names, "plan.md") || !contains(names, "product/main.go") {
		t.Fatalf("checkpoint missing expected files: %#v", names)
	}
	if contains(names, "checkpoints/manifest.json") {
		t.Fatalf("checkpoint recursively included checkpoints dir: %#v", names)
	}
}

func TestCreateReplacesPhaseEntryInManifest(t *testing.T) {
	room := t.TempDir()
	if err := os.WriteFile(filepath.Join(room, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	first, err := Create(room, "plan")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(room, "b.txt"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}
	second, err := Create(room, "plan")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ReadManifest(room)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != 1 {
		t.Fatalf("expected one latest phase entry, got %#v", manifest.Entries)
	}
	if manifest.Entries[0].ID == first.ID || manifest.Entries[0].ID != second.ID {
		t.Fatalf("expected manifest to keep latest checkpoint: first=%s second=%s entries=%#v", first.ID, second.ID, manifest.Entries)
	}
}

func tarNames(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var names []string
	for {
		header, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, header.Name)
	}
	return names
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

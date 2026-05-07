package research

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
)

func TestDigestLocalSkipsRuntimeAndHydratesSource(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".rooms"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".rooms", "noise.go"), []byte("package noise\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".archive"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".archive", "old.go"), []byte("package old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	q, err := lake.Open(filepath.Join(root, ".lake"))
	if err != nil {
		t.Fatal(err)
	}
	count, err := Hydrator{Lake: q}.DigestLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 hydrated source file, got %d", count)
	}
}

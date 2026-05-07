package research

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
)

type Hydrator struct {
	Lake *lake.Lake
}

const maxDigestibleBytes = 256 * 1024

func (h Hydrator) DigestLocal(root string) (int, error) {
	if h.Lake == nil {
		return 0, nil
	}
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".lake", ".rooms", ".state", ".archive", ".venv", "__pycache__", "node_modules", "dist", "bin", "build", "coverage", "deliveries":
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if skipGenerated(rel) {
			return nil
		}
		if !digestible(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxDigestibleBytes {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = h.Lake.Put(lake.Artifact{
			Phase:      lake.PhaseResearch,
			Kind:       "local_repo_evidence",
			Source:     rel,
			Claim:      "Local source evidence from " + rel,
			Content:    truncate(string(data), 6000),
			Confidence: 0.9,
			Verified:   true,
		})
		if err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

func skipGenerated(rel string) bool {
	rel = filepath.ToSlash(rel)
	switch rel {
	case "internal/wiki/wiki.md":
		return true
	default:
		return false
	}
}

func digestible(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".md", ".json", ".yaml", ".yml", ".txt", ".py", ".js", ".ts", ".tsx", ".jsx":
		return true
	default:
		return false
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]"
}

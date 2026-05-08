package checkpoint

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const Schema = "qsm.checkpoint_manifest.v1"

type Entry struct {
	ID        string    `json:"id"`
	Phase     string    `json:"phase"`
	Path      string    `json:"path"`
	FileCount int       `json:"file_count"`
	Bytes     int64     `json:"bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

type Manifest struct {
	Schema    string    `json:"schema"`
	Room      string    `json:"room"`
	Entries   []Entry   `json:"entries"`
	UpdatedAt time.Time `json:"updated_at"`
}

func Create(room, phase string) (Entry, error) {
	roomAbs, err := filepath.Abs(room)
	if err != nil {
		return Entry{}, err
	}
	phase = sanitizePhase(phase)
	if phase == "" {
		return Entry{}, errors.New("checkpoint phase is required")
	}
	if info, err := os.Stat(roomAbs); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("room is not a directory")
		}
		return Entry{}, err
	}
	dir := filepath.Join(roomAbs, "checkpoints")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Entry{}, err
	}
	now := time.Now().UTC()
	entry := Entry{
		ID:        "checkpoint-" + phase + "-" + now.Format("20060102T150405.000000000Z"),
		Phase:     phase,
		Path:      filepath.Join(dir, phase+".tar.gz"),
		CreatedAt: now,
	}
	tmp, err := os.CreateTemp(dir, phase+".*.tar.gz.tmp")
	if err != nil {
		return Entry{}, err
	}
	tmpPath := tmp.Name()
	hash := sha256.New()
	multi := io.MultiWriter(tmp, hash)
	gz := gzip.NewWriter(multi)
	tw := tar.NewWriter(gz)
	err = filepath.WalkDir(roomAbs, func(path string, de os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(roomAbs, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if shouldSkip(rel, de) {
			if de.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := de.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			n, copyErr := io.Copy(tw, f)
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			entry.FileCount++
			entry.Bytes += n
		}
		return nil
	})
	if closeErr := tw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := gz.Close(); err == nil {
		err = closeErr
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return Entry{}, err
	}
	entry.SHA256 = hex.EncodeToString(hash.Sum(nil))
	if err := os.Rename(tmpPath, entry.Path); err != nil {
		_ = os.Remove(tmpPath)
		return Entry{}, err
	}
	if err := appendManifest(roomAbs, entry); err != nil {
		return entry, err
	}
	return entry, nil
}

func ReadManifest(room string) (Manifest, error) {
	roomAbs, err := filepath.Abs(room)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	data, err := os.ReadFile(filepath.Join(roomAbs, "checkpoints", "manifest.json"))
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func appendManifest(room string, entry Entry) error {
	path := filepath.Join(room, "checkpoints", "manifest.json")
	manifest := Manifest{Schema: Schema, Room: room}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &manifest)
	}
	kept := []Entry{}
	for _, existing := range manifest.Entries {
		if existing.Phase == entry.Phase {
			continue
		}
		kept = append(kept, existing)
	}
	kept = append(kept, entry)
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].CreatedAt.Equal(kept[j].CreatedAt) {
			return kept[i].Phase < kept[j].Phase
		}
		return kept[i].CreatedAt.Before(kept[j].CreatedAt)
	})
	manifest.Schema = Schema
	manifest.Room = room
	manifest.Entries = kept
	manifest.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func shouldSkip(rel string, de os.DirEntry) bool {
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		switch part {
		case ".git", "node_modules", "__pycache__", ".pytest_cache", "checkpoints":
			return true
		}
	}
	if de.IsDir() {
		return false
	}
	return strings.HasSuffix(rel, ".tar.gz") || strings.HasSuffix(rel, ".tmp")
}

func sanitizePhase(phase string) string {
	phase = strings.ToLower(strings.TrimSpace(phase))
	phase = strings.ReplaceAll(phase, " ", "-")
	var b strings.Builder
	for _, r := range phase {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

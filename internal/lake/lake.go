package lake

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Phase string

const (
	PhaseSynthesis Phase = "phase_1_synthesis"
	PhaseResearch  Phase = "phase_2_research"
	PhaseBuild     Phase = "phase_3_superposition"
	PhaseAudit     Phase = "phase_4_audit"
	PhaseCollapse  Phase = "phase_5_collapse"
)

type Artifact struct {
	ID             string            `json:"id"`
	Phase          Phase             `json:"phase"`
	Kind           string            `json:"kind"`
	Source         string            `json:"source"`
	Claim          string            `json:"claim"`
	Content        string            `json:"content"`
	Confidence     float64           `json:"confidence"`
	Verified       bool              `json:"verified"`
	Contradictions []string          `json:"contradictions,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
}

type Lake struct {
	root string
}

func Open(root string) (*Lake, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("lake root is required")
	}
	if err := os.MkdirAll(filepath.Join(root, "artifacts"), 0755); err != nil {
		return nil, err
	}
	return &Lake{root: root}, nil
}

func (l *Lake) Root() string {
	return l.root
}

func (l *Lake) Put(a Artifact) (Artifact, error) {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	if a.Metadata == nil {
		a.Metadata = map[string]string{}
	}
	if a.ID == "" {
		sum := sha256.Sum256([]byte(string(a.Phase) + "\x00" + a.Kind + "\x00" + a.Source + "\x00" + a.Claim + "\x00" + a.Content))
		a.ID = hex.EncodeToString(sum[:])[:16]
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return a, err
	}
	artifactsDir := filepath.Join(l.root, "artifacts")
	tmp, err := os.CreateTemp(artifactsDir, a.ID+".*.tmp")
	if err != nil {
		return a, err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return a, err
	}
	if err := tmp.Close(); err != nil {
		return a, err
	}
	path := filepath.Join(artifactsDir, a.ID+".json")
	return a, os.Rename(tmpPath, path)
}

func (l *Lake) List() ([]Artifact, error) {
	entries, err := os.ReadDir(filepath.Join(l.root, "artifacts"))
	if err != nil {
		return nil, err
	}
	var out []Artifact
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(l.root, "artifacts", entry.Name()))
		if err != nil {
			return nil, err
		}
		var a Artifact
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (l *Lake) ByPhase(phase Phase) ([]Artifact, error) {
	all, err := l.List()
	if err != nil {
		return nil, err
	}
	var out []Artifact
	for _, a := range all {
		if a.Phase == phase {
			out = append(out, a)
		}
	}
	return out, nil
}

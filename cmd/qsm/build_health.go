package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/swarm"
)

type BuildHealthState struct {
	Models    map[string]BuildHealthModel `json:"models"`
	Seen      map[string]bool             `json:"seen,omitempty"`
	UpdatedAt time.Time                   `json:"updated_at"`
}

type BuildHealthModel struct {
	Model           string    `json:"model"`
	Attempts        int       `json:"attempts"`
	Succeeded       int       `json:"succeeded"`
	Failed          int       `json:"failed"`
	SuccessRate     float64   `json:"success_rate"`
	LastObjectiveID string    `json:"last_objective_id,omitempty"`
	LastPositionID  string    `json:"last_position_id,omitempty"`
	LastHarness     string    `json:"last_harness,omitempty"`
	LastState       string    `json:"last_state,omitempty"`
	LastPhase       string    `json:"last_phase,omitempty"`
	LastScore       float64   `json:"last_score,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
	LastSeenAt      time.Time `json:"last_seen_at"`
}

func buildHealthPath(root string) string {
	return filepath.Join(root, ".state", "build_health.json")
}

func loadBuildHealthState(root string) BuildHealthState {
	state := BuildHealthState{
		Models: map[string]BuildHealthModel{},
		Seen:   map[string]bool{},
	}
	data, err := os.ReadFile(buildHealthPath(root))
	if err != nil {
		return state
	}
	if json.Unmarshal(data, &state) != nil {
		return BuildHealthState{Models: map[string]BuildHealthModel{}, Seen: map[string]bool{}}
	}
	if state.Models == nil {
		state.Models = map[string]BuildHealthModel{}
	}
	if state.Seen == nil {
		state.Seen = map[string]bool{}
	}
	return state
}

func updateBuildHealthState(root string, report swarm.RunReport) (BuildHealthState, error) {
	state := loadBuildHealthState(root)
	if state.Models == nil {
		state.Models = map[string]BuildHealthModel{}
	}
	if state.Seen == nil {
		state.Seen = map[string]bool{}
	}
	for _, result := range report.Results {
		observationID := report.ObjectiveID + "/" + result.PositionID
		if state.Seen[observationID] {
			continue
		}
		status, _ := swarm.ReadRoomStatus(result.Room)
		model := buildHealthModelName(result, status)
		if strings.TrimSpace(model) == "" {
			continue
		}
		entry := state.Models[model]
		if entry.Model == "" {
			entry.Model = model
		}
		entry.Attempts++
		if buildHealthSucceeded(result, status) {
			entry.Succeeded++
		} else {
			entry.Failed++
		}
		if entry.Attempts > 0 {
			entry.SuccessRate = float64(entry.Succeeded) / float64(entry.Attempts)
		}
		entry.LastObjectiveID = report.ObjectiveID
		entry.LastPositionID = result.PositionID
		entry.LastHarness = report.HarnessMode
		entry.LastState = status.State
		entry.LastPhase = status.Phase
		entry.LastScore = result.Score
		entry.LastError = strings.TrimSpace(result.Error)
		if entry.LastError == "" {
			entry.LastError = strings.TrimSpace(status.Error)
		}
		entry.LastSeenAt = time.Now().UTC()
		state.Models[model] = entry
		state.Seen[observationID] = true
	}
	state.UpdatedAt = time.Now().UTC()
	if err := writeJSON(buildHealthPath(root), state); err != nil {
		return state, err
	}
	return state, nil
}

func buildHealthModelName(result swarm.BranchResult, status swarm.RoomStatus) string {
	if strings.TrimSpace(result.AgentModel) != "" {
		return strings.TrimSpace(result.AgentModel)
	}
	return strings.TrimSpace(status.AgentModel)
}

func buildHealthSucceeded(result swarm.BranchResult, status swarm.RoomStatus) bool {
	if status.State == swarm.RoomStateSucceeded && status.ProductReady && status.EvidenceReady {
		return true
	}
	return result.BuildPassed && result.TestPassed && result.LintPassed && strings.TrimSpace(result.Error) == ""
}

func buildHealthBlocksRoute(model string, health map[string]BuildHealthModel) bool {
	entry, ok := health[model]
	if !ok {
		return false
	}
	return entry.Attempts >= 2 && entry.Succeeded == 0
}

func buildHealthSummary(models map[string]BuildHealthModel, limit int) []BuildHealthModel {
	items := make([]BuildHealthModel, 0, len(models))
	for _, item := range models {
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SuccessRate == items[j].SuccessRate {
			return items[i].LastSeenAt.After(items[j].LastSeenAt)
		}
		return items[i].SuccessRate > items[j].SuccessRate
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func buildHealthForModels(models []string, health map[string]BuildHealthModel) map[string]BuildHealthModel {
	out := map[string]BuildHealthModel{}
	for _, model := range models {
		if item, ok := health[model]; ok {
			out[model] = item
		}
	}
	return out
}

func readBuildHealthState(root string) (BuildHealthState, error) {
	data, err := os.ReadFile(buildHealthPath(root))
	if err != nil {
		return BuildHealthState{}, err
	}
	var state BuildHealthState
	if err := json.Unmarshal(data, &state); err != nil {
		return BuildHealthState{}, err
	}
	if state.Models == nil {
		state.Models = map[string]BuildHealthModel{}
	}
	if state.Seen == nil {
		state.Seen = map[string]bool{}
	}
	return state, nil
}

func buildHealthStateExists(root string) bool {
	_, err := os.Stat(buildHealthPath(root))
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

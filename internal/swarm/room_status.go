package swarm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const RoomStatusVersion = 1

const (
	RoomStateQueued    = "queued"
	RoomStateRunning   = "running"
	RoomStateSucceeded = "succeeded"
	RoomStateFailed    = "failed"
)

type RoomStatus struct {
	SchemaVersion      int        `json:"schema_version"`
	ObjectiveID        string     `json:"objective_id,omitempty"`
	PositionID         string     `json:"position_id,omitempty"`
	AgentID            string     `json:"agent_id,omitempty"`
	AgentModel         string     `json:"agent_model,omitempty"`
	Harness            string     `json:"harness,omitempty"`
	State              string     `json:"state"`
	Phase              string     `json:"phase,omitempty"`
	Attempt            int        `json:"attempt,omitempty"`
	CacheRefreshCount  int        `json:"cache_refresh_count,omitempty"`
	ProductReady       bool       `json:"product_ready"`
	EvidenceReady      bool       `json:"evidence_ready"`
	BuildPassed        bool       `json:"build_passed"`
	TestPassed         bool       `json:"test_passed"`
	LintPassed         bool       `json:"lint_passed"`
	TestCommands       int        `json:"test_commands,omitempty"`
	TestCount          int        `json:"test_count,omitempty"`
	FailedTestCommands int        `json:"failed_test_commands,omitempty"`
	SecurityCritical   int        `json:"security_critical,omitempty"`
	SecurityHigh       int        `json:"security_high,omitempty"`
	SecurityMedium     int        `json:"security_medium,omitempty"`
	Score              float64    `json:"score,omitempty"`
	Error              string     `json:"error,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

func RoomStatusPath(room string) string {
	return filepath.Join(room, ".qsm_status", "status.json")
}

func NewRoomStatus(obj Objective, p Position, agent Agent, harness string) RoomStatus {
	now := time.Now().UTC()
	return RoomStatus{
		SchemaVersion: RoomStatusVersion,
		ObjectiveID:   obj.ID,
		PositionID:    p.ID,
		AgentID:       agent.ID,
		AgentModel:    AgentRouteLabel(agent),
		Harness:       harness,
		State:         RoomStateQueued,
		Phase:         "queued",
		StartedAt:     &now,
		UpdatedAt:     now,
	}
}

func ReadRoomStatus(room string) (RoomStatus, error) {
	data, err := os.ReadFile(RoomStatusPath(room))
	if err != nil {
		return RoomStatus{}, err
	}
	var status RoomStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return RoomStatus{}, err
	}
	return status, nil
}

func WriteRoomStatus(room string, status RoomStatus) error {
	if strings.TrimSpace(room) == "" {
		return nil
	}
	if status.SchemaVersion == 0 {
		status.SchemaVersion = RoomStatusVersion
	}
	status.UpdatedAt = time.Now().UTC()
	dir := filepath.Dir(RoomStatusPath(room))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "status-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, RoomStatusPath(room))
}

func UpdateRoomStatus(room string, mutate func(*RoomStatus)) error {
	status, err := ReadRoomStatus(room)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		status = RoomStatus{SchemaVersion: RoomStatusVersion, State: RoomStateQueued}
	}
	mutate(&status)
	return WriteRoomStatus(room, status)
}

func MarkRoomPhase(room, phase string) {
	_ = UpdateRoomStatus(room, func(status *RoomStatus) {
		status.State = RoomStateRunning
		status.Phase = phase
	})
}

func MarkRoomCacheRefresh(room string, attempt int) {
	_ = UpdateRoomStatus(room, func(status *RoomStatus) {
		status.State = RoomStateRunning
		status.Phase = "cache_refreshed"
		status.Attempt = attempt
		status.CacheRefreshCount++
	})
	appendRoomEvent(room, map[string]any{
		"type":    "cache_refresh",
		"attempt": attempt,
		"at":      time.Now().UTC(),
	})
}

func MarkRoomResult(room string, result BranchResult) {
	_ = UpdateRoomStatus(room, func(status *RoomStatus) {
		if result.AgentID != "" {
			status.AgentID = result.AgentID
		}
		if result.AgentModel != "" {
			status.AgentModel = result.AgentModel
		}
		status.ProductReady = result.ProductPath != "" && pathReady(result.ProductPath)
		status.EvidenceReady = result.EvidencePath != "" && fileReady(result.EvidencePath)
		status.BuildPassed = result.BuildPassed
		status.TestPassed = result.TestPassed
		status.LintPassed = result.LintPassed
		if result.TestReport != nil {
			status.TestCommands = result.TestReport.Summary.Commands
			status.TestCount = result.TestReport.Summary.Tests
			status.FailedTestCommands = result.TestReport.Summary.FailedCommands
			status.SecurityCritical = result.TestReport.Security.CriticalCount
			status.SecurityHigh = result.TestReport.Security.HighCount
			status.SecurityMedium = result.TestReport.Security.MediumCount
		}
		status.Score = result.Score
		status.Error = result.Error
		completedAt := result.CompletedAt
		if completedAt.IsZero() {
			completedAt = time.Now().UTC()
		}
		status.CompletedAt = &completedAt
		status.Phase = "complete"
		if result.BuildPassed && result.TestPassed && result.LintPassed && result.Error == "" {
			status.State = RoomStateSucceeded
		} else {
			status.State = RoomStateFailed
		}
	})
}

func pathReady(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}

func fileReady(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func AgentRouteLabel(agent Agent) string {
	model := strings.TrimSpace(agent.Model)
	provider := strings.TrimSpace(agent.Provider)
	if provider == "" || model == "" || strings.HasPrefix(model, provider+"/") {
		return model
	}
	return provider + "/" + model
}

func appendRoomEvent(room string, event map[string]any) {
	if strings.TrimSpace(room) == "" {
		return
	}
	dir := filepath.Join(room, ".qsm_status")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

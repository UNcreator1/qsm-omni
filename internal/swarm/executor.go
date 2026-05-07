package swarm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/grounding"
	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
	"github.com/nemoclaws/quantum-swarm-v3/internal/nodekit"
)

type Executor struct {
	Harness               Harness
	Agents                []Agent
	Concurrency           int
	HarnessMode           string
	SandboxBackend        string
	MaxRetries            int
	RetryBackoff          time.Duration
	NodeNoProgressTimeout time.Duration
	Lake                  *lake.Lake
	SharedCache           bool
}

func (e Executor) Run(ctx context.Context, obj Objective, positions []Position) RunReport {
	startedAt := time.Now().UTC()
	report := RunReport{
		ObjectiveID:    obj.ID,
		RequestedNodes: len(positions),
		Concurrency:    e.effectiveConcurrency(len(positions)),
		MaxRetries:     e.MaxRetries,
		HarnessMode:    e.HarnessMode,
		SandboxBackend: e.SandboxBackend,
		StartedAt:      startedAt,
	}
	if len(positions) == 0 || e.Harness == nil || len(e.Agents) == 0 {
		report.CompletedAt = time.Now().UTC()
		report.DurationMS = report.CompletedAt.Sub(startedAt).Milliseconds()
		return report
	}
	e.seedObjectiveCache(obj)

	sem := make(chan struct{}, report.Concurrency)
	results := make(chan BranchResult, len(positions))
	var wg sync.WaitGroup

	for i, p := range positions {
		agent := e.Agents[i%len(e.Agents)]
		sem <- struct{}{}
		wg.Add(1)
		go func(p Position, agent Agent) {
			defer wg.Done()
			defer func() { <-sem }()
			results <- e.runOne(ctx, obj, p, agent)
		}(p, agent)
	}

	wg.Wait()
	close(results)

	for result := range results {
		report.Results = append(report.Results, result)
		report.StartedNodes++
		if result.BuildPassed && result.TestPassed && result.LintPassed && result.Error == "" {
			report.SucceededNodes++
		} else {
			report.FailedNodes++
		}
	}
	sort.SliceStable(report.Results, func(i, j int) bool {
		return report.Results[i].PositionID < report.Results[j].PositionID
	})
	report.CompletedAt = time.Now().UTC()
	report.DurationMS = report.CompletedAt.Sub(startedAt).Milliseconds()
	report.AllNodesAccounted = report.StartedNodes == report.RequestedNodes
	report.CollapseEligible = report.SucceededNodes > 0
	if e.SharedCache && e.Lake != nil {
		if summary, err := e.Lake.CacheSummary(obj.ID); err == nil {
			report.CacheSummary = summary
		}
	}
	return report
}

func (e Executor) runOne(ctx context.Context, obj Objective, p Position, agent Agent) BranchResult {
	startedAt := time.Now()
	var result BranchResult
	var err error
	maxAttempts := 1 + e.MaxRetries
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	status := NewRoomStatus(obj, p, agent, e.HarnessMode)
	_ = WriteRoomStatus(p.Room, status)
	kit, kitErr := nodekit.Write(p.Room, nodekit.Params{
		ObjectiveID:       obj.ID,
		Request:           obj.Request,
		PositionID:        p.ID,
		PositionName:      p.Name,
		Strategy:          p.Strategy,
		AgentID:           agent.ID,
		AgentRole:         agent.Role,
		AgentModel:        AgentRouteLabel(agent),
		HarnessMode:       e.HarnessMode,
		LakePath:          lakePath(e.Lake),
		WikiPath:          strings.TrimSpace(os.Getenv("QSM_WIKI_PATH")),
		CachePath:         filepath.Join(p.Room, ".qsm_memory", "CACHE.md"),
		OpenHarnessPath:   strings.TrimSpace(os.Getenv("QSM_OPENHARNESS_PATH")),
		OpenHarnessCommit: strings.TrimSpace(os.Getenv("QSM_OPENHARNESS_COMMIT")),
	})
	if kitErr != nil {
		_ = UpdateRoomStatus(p.Room, func(status *RoomStatus) {
			status.State = RoomStateFailed
			status.Phase = "nodekit_failed"
			status.Error = kitErr.Error()
		})
		return BranchResult{
			PositionID:   p.ID,
			AgentID:      agent.ID,
			AgentModel:   AgentRouteLabel(agent),
			Room:         p.Room,
			EvidencePath: filepath.Join(p.Room, "evidence.json"),
			Error:        kitErr.Error(),
			CompletedAt:  time.Now().UTC(),
		}
	}
	_ = UpdateRoomStatus(p.Room, func(status *RoomStatus) {
		status.Phase = "nodekit_ready"
	})
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if e.writeRoomCache(obj, p) == nil && e.SharedCache {
			MarkRoomCacheRefresh(p.Room, attempt)
		}
		_ = UpdateRoomStatus(p.Room, func(status *RoomStatus) {
			status.State = RoomStateRunning
			status.Phase = "harness_execute"
			status.Attempt = attempt
		})
		attemptCtx, cancelAttempt := context.WithCancel(ctx)
		stopWatchdog := e.startNoProgressWatchdog(attemptCtx, p.Room, cancelAttempt)
		stopSupervisor := e.startCacheSupervisor(ctx, obj, p, attempt)
		result, err = e.Harness.Execute(attemptCtx, p, agent, obj)
		stopSupervisor()
		watchdog := stopWatchdog()
		cancelAttempt()
		if watchdog.Fired {
			err = fmt.Errorf("no_progress_timeout: %s", watchdog.Reason)
			e.putCache(obj, p, agent, "failed_attempt", err.Error(), true, 0.86, map[string]string{
				"attempt": fmt.Sprint(attempt),
				"model":   AgentRouteLabel(agent),
				"phase":   watchdog.Phase,
			})
		}
		result.Attempts = attempt
		if err == nil || attempt == maxAttempts || !retryableHarnessError(err) {
			break
		}
		_ = UpdateRoomStatus(p.Room, func(status *RoomStatus) {
			status.State = RoomStateRunning
			status.Phase = "retry_backoff"
			status.Attempt = attempt
			status.Error = err.Error()
		})
		e.putCache(obj, p, agent, "rate_limit_signal", err.Error(), true, 0.8, map[string]string{
			"attempt": fmt.Sprint(attempt),
			"model":   AgentRouteLabel(agent),
		})
		backoff := e.RetryBackoff
		if backoff <= 0 {
			backoff = 5 * time.Second
		}
		timer := time.NewTimer(backoff * time.Duration(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			err = ctx.Err()
			attempt = maxAttempts
		case <-timer.C:
		}
	}
	if result.PositionID == "" {
		result.PositionID = p.ID
	}
	if result.Room == "" {
		result.Room = p.Room
	}
	if result.EvidencePath == "" {
		result.EvidencePath = filepath.Join(p.Room, "evidence.json")
	}
	result.AgentID = agent.ID
	result.AgentModel = AgentRouteLabel(agent)
	if result.Metadata != nil {
		if effective, ok := result.Metadata["effective_model"].(string); ok && effective != "" {
			result.AgentModel = effective
		}
	}
	if result.Metadata == nil {
		result.Metadata = map[string]any{}
	}
	result.Metadata["qsm_harness_kit"] = filepath.Join(p.Room, ".qsm_harness", "manifest.json")
	result.Metadata["qsm_harness_kit_schema"] = kit.Schema
	result.Metadata["qsm_harness_skills"] = len(kit.Skills)
	result.Metadata["qsm_harness_hooks"] = len(kit.Hooks)
	result.DurationMS = time.Since(startedAt).Milliseconds()
	if result.CompletedAt.IsZero() {
		result.CompletedAt = time.Now().UTC()
	}
	if err != nil {
		result.BuildPassed = false
		result.TestPassed = false
		result.LintPassed = false
		result.Score = 0
		result.Error = err.Error()
	}
	e.enrichCitations(obj, p, &result)
	MarkRoomResult(p.Room, result)
	e.publishResultCache(obj, p, agent, result)
	return result
}

func lakePath(q *lake.Lake) string {
	if q == nil {
		return ""
	}
	return q.Root()
}

func (e Executor) enrichCitations(obj Objective, p Position, result *BranchResult) {
	if e.Lake == nil || result == nil {
		return
	}
	candidates := grounding.CandidatesFromLake(e.Lake, obj.ID)
	for _, path := range []string{
		filepath.Join(p.Room, ".qsm_memory", "CACHE.md"),
		filepath.Join(p.Room, ".qsm_memory", "AGENTS.md"),
	} {
		if candidate, ok := grounding.CandidateFromFile(path, "room_memory"); ok {
			candidates = append(candidates, candidate)
		}
	}
	queries := citationQueries(result)
	if len(candidates) == 0 || len(queries) == 0 {
		return
	}
	report := grounding.MapQueries(candidates, queries, 6)
	result.Citations = report.Citations
	meta := ensureMetadata(result)
	meta["citation_coverage"] = fmt.Sprintf("%.2f", report.Coverage)
	if len(report.Missing) > 0 {
		meta["citation_missing"] = strings.Join(report.Missing, "; ")
	}
	if result.EvidencePath != "" {
		_ = writeJSON(result.EvidencePath, result)
	}
}

func citationQueries(result *BranchResult) []grounding.Query {
	var queries []grounding.Query
	add := func(source, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		queries = append(queries, grounding.Query{Source: source, Text: text})
	}
	if result.Verification != nil {
		for _, check := range result.Verification.Checks {
			add("verification_check", check)
		}
		for _, warning := range result.Verification.Warnings {
			add("verification_warning", warning)
		}
	}
	for _, key := range []string{"final_message", "warning", "stop_reason"} {
		if value, ok := result.Metadata[key]; ok {
			add(key, fmt.Sprint(value))
		}
	}
	if result.ProductPath != "" {
		for _, name := range []string{"README.md", "SUMMARY.md", "REPORT.md"} {
			data, err := os.ReadFile(filepath.Join(result.ProductPath, name))
			if err == nil {
				add("product/"+name, string(data))
			}
		}
	}
	if result.Error != "" {
		add("error", result.Error)
	}
	return queries
}

func (e Executor) startCacheSupervisor(ctx context.Context, obj Objective, p Position, attempt int) func() {
	if !e.SharedCache || e.Lake == nil || strings.ToLower(strings.TrimSpace(e.HarnessMode)) != "opencode" {
		return func() {}
	}
	supervisorCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(cacheRefreshInterval())
		defer ticker.Stop()
		for {
			select {
			case <-supervisorCtx.Done():
				return
			case <-ticker.C:
				if e.writeRoomCache(obj, p) == nil {
					MarkRoomCacheRefresh(p.Room, attempt)
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

type noProgressWatchdogResult struct {
	Fired  bool
	Reason string
	Phase  string
}

func (e Executor) startNoProgressWatchdog(ctx context.Context, room string, cancel context.CancelFunc) func() noProgressWatchdogResult {
	timeout := e.effectiveNoProgressTimeout()
	if timeout <= 0 || strings.TrimSpace(room) == "" {
		return func() noProgressWatchdogResult { return noProgressWatchdogResult{} }
	}
	watchCtx, stop := context.WithCancel(ctx)
	done := make(chan noProgressWatchdogResult, 1)
	go func() {
		ticker := time.NewTicker(noProgressPollInterval(timeout))
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				done <- noProgressWatchdogResult{}
				return
			case <-ticker.C:
				status, err := ReadRoomStatus(room)
				if err != nil {
					continue
				}
				if status.State != "" && status.State != RoomStateRunning {
					done <- noProgressWatchdogResult{}
					return
				}
				if time.Since(status.UpdatedAt) < timeout {
					continue
				}
				reason := fmt.Sprintf("phase=%s stale_for=%s timeout=%s", status.Phase, time.Since(status.UpdatedAt).Round(time.Second), timeout)
				_ = UpdateRoomStatus(room, func(next *RoomStatus) {
					next.State = RoomStateFailed
					next.Phase = "no_progress_timeout"
					next.Error = reason
					now := time.Now().UTC()
					next.CompletedAt = &now
				})
				appendRoomEvent(room, map[string]any{
					"type":    "no_progress_timeout",
					"phase":   status.Phase,
					"timeout": timeout.String(),
					"at":      time.Now().UTC(),
				})
				cancel()
				done <- noProgressWatchdogResult{Fired: true, Reason: reason, Phase: status.Phase}
				return
			}
		}
	}()
	return func() noProgressWatchdogResult {
		stop()
		return <-done
	}
}

func (e Executor) effectiveNoProgressTimeout() time.Duration {
	if e.NodeNoProgressTimeout > 0 {
		return e.NodeNoProgressTimeout
	}
	raw := strings.TrimSpace(os.Getenv("QSM_NODE_NO_PROGRESS_TIMEOUT"))
	if raw == "" {
		return 120 * time.Second
	}
	if raw == "0" || strings.EqualFold(raw, "off") || strings.EqualFold(raw, "false") {
		return 0
	}
	if duration, err := time.ParseDuration(raw); err == nil {
		return duration
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil {
		return time.Duration(seconds * float64(time.Second))
	}
	return 120 * time.Second
}

func noProgressPollInterval(timeout time.Duration) time.Duration {
	interval := timeout / 4
	if interval < 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	if interval > 5*time.Second {
		return 5 * time.Second
	}
	return interval
}

func cacheRefreshInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("QSM_CACHE_REFRESH_SECONDS"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("QSM_OPENCODE_CACHE_REFRESH_SECONDS"))
	}
	if raw == "" {
		return 5 * time.Second
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil && seconds > 0 {
		return time.Duration(seconds * float64(time.Second))
	}
	if duration, err := time.ParseDuration(raw); err == nil && duration > 0 {
		return duration
	}
	return 5 * time.Second
}

func retryableHarnessError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "429") ||
		strings.Contains(text, "rate limit") ||
		strings.Contains(text, "too many requests") ||
		strings.Contains(text, "temporarily unavailable") ||
		strings.Contains(text, "timeout") ||
		strings.Contains(text, "no_progress_timeout") ||
		strings.Contains(text, "agent product missing") ||
		strings.Contains(text, "product directory is empty")
}

func (e Executor) effectiveConcurrency(positionCount int) int {
	if positionCount <= 0 {
		return 0
	}
	if e.Concurrency <= 0 || e.Concurrency > positionCount {
		return positionCount
	}
	return e.Concurrency
}

func (e Executor) writeRoomCache(obj Objective, p Position) error {
	if !e.SharedCache || e.Lake == nil || strings.TrimSpace(p.Room) == "" {
		return nil
	}
	verified := true
	return e.Lake.WriteCacheMarkdown(filepath.Join(p.Room, ".qsm_memory", "CACHE.md"), lake.CacheFilter{ObjectiveID: obj.ID, Verified: &verified}, 30)
}

func (e Executor) publishResultCache(obj Objective, p Position, agent Agent, result BranchResult) {
	if !e.SharedCache || e.Lake == nil {
		return
	}
	metadata := map[string]string{
		"agent":       agent.ID,
		"agent_model": result.AgentModel,
		"harness":     e.HarnessMode,
	}
	if result.Error != "" || !result.BuildPassed || !result.TestPassed || !result.LintPassed {
		content := result.Error
		if content == "" && result.Verification != nil && len(result.Verification.Errors) > 0 {
			content = strings.Join(result.Verification.Errors, "; ")
		}
		if content == "" && result.TestReport != nil && len(result.TestReport.Errors) > 0 {
			content = strings.Join(result.TestReport.Errors, "; ")
		}
		if content == "" {
			content = "branch failed deterministic build/test/lint gates"
		}
		e.putCache(obj, p, agent, "failed_attempt", content, true, 0.8, metadata)
		return
	}
	content := "branch produced a verified product"
	if result.Verification != nil && len(result.Verification.Checks) > 0 {
		content = strings.Join(result.Verification.Checks, "\n")
		metadata["verification_type"] = result.Verification.Type
	}
	if result.TestReport != nil {
		metadata["test_commands"] = fmt.Sprint(result.TestReport.Summary.Commands)
		metadata["test_count"] = fmt.Sprint(result.TestReport.Summary.Tests)
		if result.TestReport.Summary.Commands > 0 {
			content += fmt.Sprintf("\nQSM test commands passed: %d/%d", result.TestReport.Summary.PassedCommands, result.TestReport.Summary.Commands)
		}
	}
	e.putCache(obj, p, agent, "verified_recipe", content, true, result.Score, metadata)
}

func (e Executor) seedObjectiveCache(obj Objective) {
	if !e.SharedCache || e.Lake == nil || strings.TrimSpace(obj.Request) == "" {
		return
	}
	_, _ = e.Lake.PutCache(lake.CacheItem{
		Kind:        "constraint",
		ObjectiveID: obj.ID,
		Producer:    "executor/orchestrator",
		Content:     "Current objective request: " + obj.Request,
		Verified:    true,
		Confidence:  1.0,
		Metadata: map[string]string{
			"scope": "objective",
		},
	})
}

func (e Executor) putCache(obj Objective, p Position, agent Agent, kind, content string, verified bool, confidence float64, metadata map[string]string) {
	if !e.SharedCache || e.Lake == nil || strings.TrimSpace(content) == "" {
		return
	}
	if metadata == nil {
		metadata = map[string]string{}
	}
	_, _ = e.Lake.PutCache(lake.CacheItem{
		Kind:        kind,
		ObjectiveID: obj.ID,
		PositionID:  p.ID,
		Producer:    "executor/" + agent.ID,
		Content:     content,
		Verified:    verified,
		Confidence:  confidence,
		Metadata:    metadata,
	})
}

package swarm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nemoclaws/quantum-swarm-v3/internal/lake"
)

type stubHarness struct {
	fail map[string]bool
}

type slowHarness struct {
	delay time.Duration
}

type watchdogHarness struct {
	mu    sync.Mutex
	calls int
}

func TestExecutorSharedCachePublishesLessons(t *testing.T) {
	q, err := lake.Open(filepath.Join(t.TempDir(), ".lake"))
	if err != nil {
		t.Fatal(err)
	}
	positions := []Position{
		{ID: "pos-01", Room: t.TempDir()},
		{ID: "pos-02", Room: t.TempDir()},
	}
	agents := []Agent{{ID: "alpha", Provider: "oc", Model: "m1"}}
	report := Executor{
		Harness:     stubHarness{fail: map[string]bool{"pos-02": true}},
		Agents:      agents,
		Concurrency: 1,
		HarnessMode: "test",
		Lake:        q,
		SharedCache: true,
	}.Run(context.Background(), Objective{ID: "obj-cache"}, positions)
	if report.CacheSummary["verified_recipe"] != 1 || report.CacheSummary["failed_attempt"] != 1 {
		t.Fatalf("unexpected cache summary: %#v", report.CacheSummary)
	}
	status, err := ReadRoomStatus(positions[0].Room)
	if err != nil {
		t.Fatal(err)
	}
	if status.CacheRefreshCount != 1 || status.State != RoomStateSucceeded {
		t.Fatalf("unexpected room status: %#v", status)
	}
	items, err := q.ListCache(lake.CacheFilter{ObjectiveID: "obj-cache"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two cache items, got %#v", items)
	}
}

func (h stubHarness) Execute(_ context.Context, p Position, _ Agent, _ Objective) (BranchResult, error) {
	result := BranchResult{
		PositionID:   p.ID,
		Room:         p.Room,
		BuildPassed:  true,
		TestPassed:   true,
		LintPassed:   true,
		Score:        0.7,
		EvidencePath: filepath.Join(p.Room, "evidence.json"),
		CompletedAt:  time.Now().UTC(),
	}
	if h.fail[p.ID] {
		return result, errors.New("planned node failure")
	}
	return result, nil
}

func (h slowHarness) Execute(ctx context.Context, p Position, _ Agent, _ Objective) (BranchResult, error) {
	timer := time.NewTimer(h.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return BranchResult{PositionID: p.ID, Room: p.Room}, ctx.Err()
	case <-timer.C:
	}
	return BranchResult{
		PositionID:   p.ID,
		Room:         p.Room,
		BuildPassed:  true,
		TestPassed:   true,
		LintPassed:   true,
		Score:        0.7,
		EvidencePath: filepath.Join(p.Room, "evidence.json"),
		CompletedAt:  time.Now().UTC(),
	}, nil
}

func (h *watchdogHarness) Execute(ctx context.Context, p Position, _ Agent, _ Objective) (BranchResult, error) {
	h.mu.Lock()
	h.calls++
	call := h.calls
	h.mu.Unlock()
	if call == 1 {
		<-ctx.Done()
		return BranchResult{
			PositionID:   p.ID,
			Room:         p.Room,
			EvidencePath: filepath.Join(p.Room, "evidence.json"),
			CompletedAt:  time.Now().UTC(),
		}, ctx.Err()
	}
	return BranchResult{
		PositionID:   p.ID,
		Room:         p.Room,
		BuildPassed:  true,
		TestPassed:   true,
		LintPassed:   true,
		Score:        0.8,
		EvidencePath: filepath.Join(p.Room, "evidence.json"),
		CompletedAt:  time.Now().UTC(),
	}, nil
}

func TestExecutorAccountsForAllNodes(t *testing.T) {
	positions := []Position{
		{ID: "pos-01", Room: t.TempDir()},
		{ID: "pos-02", Room: t.TempDir()},
		{ID: "pos-03", Room: t.TempDir()},
	}
	agents := []Agent{{ID: "alpha", Provider: "oc", Model: "m1"}}
	report := Executor{
		Harness:     stubHarness{fail: map[string]bool{"pos-02": true}},
		Agents:      agents,
		Concurrency: 2,
		HarnessMode: "test",
	}.Run(context.Background(), Objective{ID: "obj-1"}, positions)

	if report.RequestedNodes != 3 || report.StartedNodes != 3 {
		t.Fatalf("expected all nodes accounted, got %#v", report)
	}
	if report.SucceededNodes != 2 || report.FailedNodes != 1 {
		t.Fatalf("unexpected success/fail counts: %#v", report)
	}
	if !report.AllNodesAccounted || !report.CollapseEligible {
		t.Fatalf("unexpected report gates: %#v", report)
	}
	if report.Results[1].PositionID != "pos-02" || report.Results[1].Error == "" {
		t.Fatalf("expected sorted failed pos-02 result, got %#v", report.Results)
	}
}

func TestExecutorCreatesNodeHarnessKit(t *testing.T) {
	position := Position{ID: "pos-01", Room: t.TempDir()}
	report := Executor{
		Harness:     stubHarness{},
		Agents:      []Agent{{ID: "alpha", Provider: "oc", Model: "m1"}},
		Concurrency: 1,
		HarnessMode: "test",
	}.Run(context.Background(), Objective{ID: "obj-kit", Request: "build"}, []Position{position})
	if report.SucceededNodes != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	for _, path := range []string{
		filepath.Join(position.Room, ".qsm_harness", "manifest.json"),
		filepath.Join(position.Room, ".qsm_harness", "hooks.json"),
		filepath.Join(position.Room, ".qsm_harness", "PERMISSIONS.md"),
		filepath.Join(position.Room, ".qsm_harness", "skills", "plan", "SKILL.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing node harness kit file %s: %v", path, err)
		}
	}
	meta := report.Results[0].Metadata
	if meta["qsm_harness_kit_schema"] != "qsm.node_harness_kit.v1" {
		t.Fatalf("missing nodekit metadata: %#v", meta)
	}
}

func TestExecutorOpenCodeCacheSupervisorRefreshesStatus(t *testing.T) {
	t.Setenv("QSM_CACHE_REFRESH_SECONDS", "0.01")
	q, err := lake.Open(filepath.Join(t.TempDir(), ".lake"))
	if err != nil {
		t.Fatal(err)
	}
	position := Position{ID: "pos-01", Room: t.TempDir()}
	report := Executor{
		Harness:     slowHarness{delay: 60 * time.Millisecond},
		Agents:      []Agent{{ID: "alpha", Provider: "oc", Model: "m1"}},
		Concurrency: 1,
		HarnessMode: "opencode",
		Lake:        q,
		SharedCache: true,
	}.Run(context.Background(), Objective{ID: "obj-supervisor"}, []Position{position})
	if report.SucceededNodes != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	status, err := ReadRoomStatus(position.Room)
	if err != nil {
		t.Fatal(err)
	}
	if status.CacheRefreshCount < 2 {
		t.Fatalf("expected supervisor to refresh cache, got status %#v", status)
	}
	if _, err := os.Stat(filepath.Join(position.Room, ".qsm_status", "events.jsonl")); err != nil {
		t.Fatalf("expected room event log: %v", err)
	}
}

func TestExecutorNoProgressWatchdogRetriesSilentNode(t *testing.T) {
	q, err := lake.Open(filepath.Join(t.TempDir(), ".lake"))
	if err != nil {
		t.Fatal(err)
	}
	harness := &watchdogHarness{}
	position := Position{ID: "pos-01", Room: t.TempDir()}
	report := Executor{
		Harness:               harness,
		Agents:                []Agent{{ID: "alpha", Provider: "oc", Model: "m1"}},
		Concurrency:           1,
		HarnessMode:           "langchain",
		MaxRetries:            1,
		NodeNoProgressTimeout: 30 * time.Millisecond,
		Lake:                  q,
		SharedCache:           true,
	}.Run(context.Background(), Objective{ID: "obj-watchdog"}, []Position{position})
	if report.SucceededNodes != 1 || report.FailedNodes != 0 {
		t.Fatalf("expected retry to recover silent node, got %#v", report)
	}
	if harness.calls != 2 {
		t.Fatalf("expected two harness calls, got %d", harness.calls)
	}
	status, err := ReadRoomStatus(position.Room)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != RoomStateSucceeded || status.Phase != "complete" {
		t.Fatalf("expected final succeeded status, got %#v", status)
	}
	items, err := q.ListCache(lake.CacheFilter{ObjectiveID: "obj-watchdog", Kinds: []string{"failed_attempt"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || !strings.Contains(items[0].Content, "no_progress_timeout") {
		t.Fatalf("expected no-progress failed_attempt cache item, got %#v", items)
	}
}

package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/msnotfound/fleetorch/internal/types"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	return New(filepath.Join(t.TempDir(), "state.json"))
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	st, err := testStore(t).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if st == nil {
		t.Fatal("Load() returned nil state")
	}
	if len(st.Tasks) != 0 {
		t.Fatalf("Load() tasks length = %d, want 0", len(st.Tasks))
	}
	if st.Ledger == nil {
		t.Fatal("Load() ledger is nil")
	}
	if len(st.Ledger) != 0 {
		t.Fatalf("Load() ledger length = %d, want 0", len(st.Ledger))
	}
}

func TestAddTaskPersistsAndIncrementsLedger(t *testing.T) {
	s := testStore(t)
	task := &types.Task{
		ID:        "task-1",
		Agent:     "claude-sonnet",
		Worktree:  "/tmp/worktree",
		Log:       "/tmp/log",
		PID:       123,
		StartedAt: time.Unix(100, 0).UTC(),
		Status:    types.StatusRunning,
		BudgetUSD: 2.5,
	}

	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	st, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(st.Tasks) != 1 {
		t.Fatalf("persisted task count = %d, want 1", len(st.Tasks))
	}
	if st.Tasks[0].ID != task.ID {
		t.Fatalf("persisted task ID = %q, want %q", st.Tasks[0].ID, task.ID)
	}
	if got := st.Ledger[types.LedgerClaudeSonnet]; got != 1 {
		t.Fatalf("ledger[%q] = %d, want 1", types.LedgerClaudeSonnet, got)
	}
}

func TestUpdateTaskDoesNotIncrementLedger(t *testing.T) {
	s := testStore(t)
	task := &types.Task{ID: "task-1", Agent: "codex", Status: types.StatusRunning}
	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	if err := s.UpdateTask(task.ID, func(t *types.Task) {
		t.Status = types.StatusDone
	}); err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}

	st, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := st.Ledger[types.LedgerCodex]; got != 1 {
		t.Fatalf("ledger[%q] = %d, want 1", types.LedgerCodex, got)
	}
	if st.Tasks[0].Status != types.StatusDone {
		t.Fatalf("task status = %q, want %q", st.Tasks[0].Status, types.StatusDone)
	}
}

func TestConcurrentUpdatesAreSerialized(t *testing.T) {
	s := testStore(t)
	task := &types.Task{ID: "task-1", Agent: "gemini", Status: types.StatusRunning}
	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	const goroutines = 10
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.UpdateTask(task.ID, func(t *types.Task) {
				t.PID++
			})
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("UpdateTask() error = %v", err)
		}
	}

	got, err := s.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if got.PID != goroutines {
		t.Fatalf("task PID = %d, want %d", got.PID, goroutines)
	}

	st, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := st.Ledger[types.LedgerGemini]; got != 1 {
		t.Fatalf("ledger[%q] = %d, want 1", types.LedgerGemini, got)
	}
}

func TestRemoveTaskMissingReturnsErrNotFound(t *testing.T) {
	err := testStore(t).RemoveTask("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("RemoveTask() error = %v, want %v", err, ErrNotFound)
	}
}

func TestUnknownAgentUsesAgentNameAsLedgerKey(t *testing.T) {
	s := testStore(t)
	agent := "custom-agent"
	if err := s.AddTask(&types.Task{ID: "task-1", Agent: agent}); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	st, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := st.Ledger[agent]; got != 1 {
		t.Fatalf("ledger[%q] = %d, want 1", agent, got)
	}
}

func TestListTasksReturnsPersistedTasks(t *testing.T) {
	s := testStore(t)
	for i := 0; i < 2; i++ {
		task := &types.Task{ID: fmt.Sprintf("task-%d", i), Agent: "codex"}
		if err := s.AddTask(task); err != nil {
			t.Fatalf("AddTask(%q) error = %v", task.ID, err)
		}
	}

	tasks, err := s.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("ListTasks() length = %d, want 2", len(tasks))
	}
}

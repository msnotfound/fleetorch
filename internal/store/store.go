// Package store provides atomic JSON persistence for the fleetorch task registry.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"
	"github.com/msnotfound/fleetorch/internal/types"
)

var ErrNotFound = errors.New("task not found")

type Store struct {
	path  string
	flock *flock.Flock
	mu    sync.Mutex // serializes Updates within this process; flock handles cross-process
}

func New(path string) *Store {
	return &Store{
		path:  path,
		flock: flock.New(path + ".lock"),
	}
}

func (s *Store) Load() (*types.State, error) {
	return s.load()
}

func (s *Store) Save(st *types.State) error {
	return s.save(st)
}

func (s *Store) Update(fn func(*types.State) error) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.flock.Lock(); err != nil {
		return err
	}
	defer s.flock.Unlock()

	st, err := s.load()
	if err != nil {
		return err
	}
	if err := fn(st); err != nil {
		return err
	}
	return s.save(st)
}

func (s *Store) AddTask(t *types.Task) error {
	return s.Update(func(st *types.State) error {
		st.Tasks = append(st.Tasks, t)
		if st.Ledger == nil {
			st.Ledger = types.Ledger{}
		}
		st.Ledger[ledgerKey(t.Agent)]++
		return nil
	})
}

func (s *Store) UpdateTask(id string, fn func(*types.Task)) error {
	return s.Update(func(st *types.State) error {
		task, ok := findTask(st.Tasks, id)
		if !ok {
			return ErrNotFound
		}
		fn(task)
		return nil
	})
}

func (s *Store) GetTask(id string) (*types.Task, error) {
	st, err := s.Load()
	if err != nil {
		return nil, err
	}
	task, ok := findTask(st.Tasks, id)
	if !ok {
		return nil, ErrNotFound
	}
	return task, nil
}

func (s *Store) ListTasks() ([]*types.Task, error) {
	st, err := s.Load()
	if err != nil {
		return nil, err
	}
	return st.Tasks, nil
}

func (s *Store) RemoveTask(id string) error {
	return s.Update(func(st *types.State) error {
		for i, task := range st.Tasks {
			if task.ID == id {
				st.Tasks = append(st.Tasks[:i], st.Tasks[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s *Store) load() (*types.State, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyState(), nil
	}
	if err != nil {
		return nil, err
	}

	var st types.State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	normalizeState(&st)
	return &st, nil
}

func (s *Store) save(st *types.State) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	normalizeState(st)

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func (s *Store) ensureDir() error {
	dir := filepath.Dir(s.path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func emptyState() *types.State {
	return &types.State{
		Tasks:  []*types.Task{},
		Ledger: types.Ledger{},
	}
}

func normalizeState(st *types.State) {
	if st.Tasks == nil {
		st.Tasks = []*types.Task{}
	}
	if st.Ledger == nil {
		st.Ledger = types.Ledger{}
	}
}

func findTask(tasks []*types.Task, id string) (*types.Task, bool) {
	for _, task := range tasks {
		if task.ID == id {
			return task, true
		}
	}
	return nil, false
}

func ledgerKey(agent string) string {
	switch agent {
	case "agy":
		return types.LedgerAgy
	case "codex":
		return types.LedgerCodex
	case "gemini":
		return types.LedgerGemini
	case "claude-haiku":
		return types.LedgerClaudeHaiku
	case "claude-sonnet":
		return types.LedgerClaudeSonnet
	case "claude-opus":
		return types.LedgerClaudeOpus
	default:
		return agent
	}
}

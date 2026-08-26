package persistence

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Load() (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked()
}

func (s *Store) View(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	return fn(st)
}

func (s *Store) Update(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	next, err := cloneState(st)
	if err != nil {
		return err
	}
	if err := fn(next); err != nil {
		return err
	}
	if err := next.TouchCheckpoint(); err != nil {
		return err
	}
	return s.saveUnlocked(next)
}

func (s *Store) loadUnlocked() (*State, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		st := NewState()
		if err := st.TouchCheckpoint(); err != nil {
			return nil, err
		}
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	st.Ensure()
	return &st, nil
}

func (s *Store) saveUnlocked(st *State) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func cloneState(st *State) (*State, error) {
	b, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}
	var out State
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	out.Ensure()
	return &out, nil
}

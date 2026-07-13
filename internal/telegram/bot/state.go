package bot

import (
	"context"
	"sync"
)

type DialogView string

const (
	ViewStart           DialogView = "start"
	ViewNewSummary      DialogView = "new_summary"
	ViewFolders         DialogView = "folders"
	ViewGroups          DialogView = "groups"
	ViewSelectedSources DialogView = "selected_sources"
	ViewHistory         DialogView = "history"
	ViewArticles        DialogView = "articles"
	ViewSettings        DialogView = "settings"
)

type DialogState struct {
	View DialogView
}

type StateStore interface {
	Set(ctx context.Context, userID int64, state DialogState) error
	Get(ctx context.Context, userID int64) (DialogState, bool, error)
}

type MemoryStateStore struct {
	mu     sync.RWMutex
	states map[int64]DialogState
}

func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{states: make(map[int64]DialogState)}
}

func (s *MemoryStateStore) Set(_ context.Context, userID int64, state DialogState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[userID] = state
	return nil
}

func (s *MemoryStateStore) Get(_ context.Context, userID int64) (DialogState, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.states[userID]
	return state, ok, nil
}

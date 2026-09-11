package core

import "sync"

// RuntimeState 描述 sbtun 当前运行状态。
type RuntimeState string

const (
	StateStopped  RuntimeState = "stopped"
	StateStarting RuntimeState = "starting"
	StateRunning  RuntimeState = "running"
	StateStopping RuntimeState = "stopping"
	StateError    RuntimeState = "error"
)

// StateStore 为 GUI、托盘和核心模块提供统一状态来源。
type StateStore struct {
	mu    sync.RWMutex
	state RuntimeState
	err   string
}

func NewStateStore() *StateStore {
	return &StateStore{state: StateStopped}
}

func (s *StateStore) Set(state RuntimeState, err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.err = err
}

func (s *StateStore) Get() (RuntimeState, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state, s.err
}

func (s *StateStore) IsRunning() bool {
	state, _ := s.Get()
	return state == StateRunning
}

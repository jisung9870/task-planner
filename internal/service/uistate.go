package service

import (
	"encoding/json"
	"path/filepath"
)

// uiStateFile lives in .index/ because it is exactly as disposable as the
// index: losing it costs the user one keypress, so it must never be something
// the vault's markdown depends on.
const uiStateFile = "ui.json"

// UIState is the handful of TUI preferences worth surviving a restart.
type UIState struct {
	TimelineActual bool `json:"timeline_actual,omitempty"`
	TimelineHourly bool `json:"timeline_hourly,omitempty"`
	// Tab is the tab index the session ended on.
	Tab int `json:"tab"`
	// WideDetail remembers whether the side panel was folded away.
	WideDetail bool `json:"wide_detail"`
	// ListGrouping selects how Today and All task lists are grouped.
	ListGrouping  string `json:"list_grouping,omitempty"`
	ExecutorScope string `json:"executor_scope,omitempty"`
}

// LoadUIState reads the saved state, returning the zero value when there is
// none. A corrupt file is not an error worth reporting - it just means the TUI
// opens on Today.
func (s *Service) LoadUIState() UIState {
	st := UIState{WideDetail: true}
	raw, ok := s.vault.ReadRaw(s.uiStatePath())
	if !ok {
		return st
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return UIState{WideDetail: true}
	}
	return st
}

// SaveUIState persists the state on exit.
func (s *Service) SaveUIState(st UIState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.vault.WriteRaw(s.uiStatePath(), raw)
}

func (s *Service) uiStatePath() string {
	return filepath.Join(s.vault.IndexDir(), uiStateFile)
}

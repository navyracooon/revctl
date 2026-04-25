package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type renderOptions struct {
	refreshFiles bool
	refreshDiffs bool
}

type SessionState struct {
	SessionName  string        `json:"session_name"`
	WindowTarget string        `json:"window_target"`
	RepoRoot     string        `json:"repo_root"`
	Base         string        `json:"base"`
	Head         string        `json:"head"`
	DiffRange    string        `json:"diff_range"`
	CurrentFile  int           `json:"current_file"`
	CursorFile   int           `json:"cursor_file"`
	Entries      []ReviewEntry `json:"entries"`
	UpdatedAt    string        `json:"updated_at"`
	ReuseSession bool          `json:"reuse_session"`
	Panes        PaneState     `json:"panes"`
	Tools        ToolState     `json:"tools"`
}

type PaneState struct {
	Files  string `json:"files"`
	Viewer string `json:"viewer"`
}

type diffSide struct {
	ref         string
	path        string
	useFilePath bool
	empty       bool
}

type diffPaneSpec struct {
	left  diffSide
	right diffSide
}

type ToolState struct {
	Editor string `json:"editor"`
}

type ReviewEntry struct {
	Path       string   `json:"path"`
	ChangeType string   `json:"change_type"`
	FirstLine  int      `json:"first_line"`
}

func runStateAction(ctx context.Context, sessionDir string, state *SessionState, opts renderOptions, mutate func(*SessionState)) (*SessionState, error) {
	mutate(state)
	if err := commitState(ctx, sessionDir, state, opts); err != nil {
		return state, err
	}
	return state, nil
}

func applyAction(state *SessionState, action string) error {
	switch action {
	case "next-file":
		state.moveFile(1)
	case "prev-file":
		state.moveFile(-1)
	case "cursor-up":
		state.moveCursor(-1)
	case "cursor-down":
		state.moveCursor(1)
	case "select-cursor":
		state.selectCursor()
	case "refresh":
	default:
		return fmt.Errorf("unknown action %q", action)
	}
	return nil
}

func commitState(ctx context.Context, sessionDir string, state *SessionState, opts renderOptions) error {
	state.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := saveState(sessionDir, state); err != nil {
		return err
	}
	return renderSessionWithOptions(ctx, sessionDir, state, opts)
}

func filterEntryIndices(entries []ReviewEntry, query string) []int {
	indices := make([]int, 0, len(entries))
	for i := range entries {
		indices = append(indices, i)
	}
	return indices
}

func entryIndices(entries []ReviewEntry) []int {
	indices := make([]int, 0, len(entries))
	for i := range entries {
		indices = append(indices, i)
	}
	return indices
}

func (s *SessionState) currentEntry() *ReviewEntry {
	if len(s.Entries) == 0 {
		return &ReviewEntry{}
	}
	if s.CurrentFile < 0 {
		s.CurrentFile = 0
	}
	if s.CurrentFile >= len(s.Entries) {
		s.CurrentFile = len(s.Entries) - 1
	}
	s.normalizeCursor()
	return &s.Entries[s.CurrentFile]
}

func (s *SessionState) moveFile(delta int) {
	filtered := s.filteredIndices()
	if len(filtered) == 0 {
		return
	}
	currentPos := s.positionInFiltered(s.CurrentFile, filtered)
	if currentPos < 0 {
		currentPos = s.positionInFiltered(s.CursorFile, filtered)
	}
	if currentPos < 0 {
		currentPos = 0
	}
	nextPos := (currentPos + delta + len(filtered)) % len(filtered)
	s.CurrentFile = filtered[nextPos]
	s.CursorFile = s.CurrentFile
}

func (s *SessionState) moveCursor(delta int) {
	filtered := s.filteredIndices()
	if len(filtered) == 0 {
		return
	}
	s.normalizeCursor()
	currentPos := s.positionInFiltered(s.CursorFile, filtered)
	if currentPos < 0 {
		currentPos = s.positionInFiltered(s.CurrentFile, filtered)
	}
	if currentPos < 0 {
		currentPos = 0
	}
	nextPos := (currentPos + delta + len(filtered)) % len(filtered)
	s.CursorFile = filtered[nextPos]
}

func (s *SessionState) selectCursor() {
	filtered := s.filteredIndices()
	if len(filtered) == 0 {
		return
	}
	s.normalizeCursor()
	s.CurrentFile = s.CursorFile
}

func (s *SessionState) filteredIndices() []int {
	return filterEntryIndices(s.Entries, "")
}

func (s *SessionState) normalizeCursor() {
	filtered := s.filteredIndices()
	if len(filtered) == 0 {
		s.CursorFile = -1
		return
	}
	if s.positionInFiltered(s.CursorFile, filtered) >= 0 {
		return
	}
	if s.positionInFiltered(s.CurrentFile, filtered) >= 0 {
		s.CursorFile = s.CurrentFile
		return
	}
	s.CursorFile = filtered[0]
}

func (s *SessionState) positionInFiltered(index int, filtered []int) int {
	for pos, candidate := range filtered {
		if candidate == index {
			return pos
		}
	}
	return -1
}

func saveState(dir string, state *SessionState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), raw, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

func loadState(dir string) (*SessionState, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var state SessionState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return &state, nil
}

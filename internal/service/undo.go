package service

import (
	"fmt"
	"time"
)

// undoDepth is how many user actions stay undoable. Undo is for the keystroke
// you regret on the way past, not for archaeology - git has that covered, and
// an unbounded stack would pin every version of every file in memory.
const undoDepth = 20

// undoFile is one file's state on both sides of an action: the bytes to put
// back, and what the file looked like afterwards so an edit made elsewhere in
// the meantime is not silently overwritten.
type undoFile struct {
	path   string
	before []byte
	had    bool
	// isTask marks a file the index tracks. A project file goes through the
	// same pre-image machinery but must never be handed to the task decoder.
	isTask bool

	postMod time.Time
	postHad bool
}

// undoStep is one user action - possibly several files, because finishing a
// task can release a dependent and spawn the next occurrence of a series.
type undoStep struct {
	label string
	files map[string]*undoFile
	ids   map[string]bool
}

// Undoable runs one user action as a single undo step.
//
// The boundary is the adapter's call, not the service's: "완료" is one step
// even though it writes three files, because that is what the user did. Nested
// calls join the outer step for the same reason.
func (s *Service) Undoable(label string, fn func() error) error {
	if s.undo != nil {
		return fn()
	}
	s.undo = &undoStep{label: label, files: map[string]*undoFile{}, ids: map[string]bool{}}
	err := fn()
	step := s.undo
	s.undo = nil
	// A bulk action that fails halfway has still written files. Keeping the
	// step is the difference between "되돌리기" working and the user being left
	// with three of five tasks changed and no way back.
	if len(step.files) == 0 {
		return err
	}
	for _, f := range step.files {
		f.postMod, f.postHad = s.vault.StatFile(f.path)
	}
	s.undoStack = append(s.undoStack, step)
	if len(s.undoStack) > undoDepth {
		s.undoStack = s.undoStack[1:]
	}
	return err
}

// ResetUndo drops the stack. A full re-index means the pre-images may no
// longer describe what is on disk.
func (s *Service) ResetUndo() { s.undoStack = nil }

// captureUndo records a file's pre-image, once per step. Every write goes
// through Service.save, Delete or the project writers, so hooking those covers
// the surface.
func (s *Service) captureUndo(path, id string, isTask bool) {
	if s.undo == nil || path == "" {
		return
	}
	if id != "" {
		s.undo.ids[id] = true
	}
	if _, seen := s.undo.files[path]; seen {
		return // the first pre-image of the step is the one to restore
	}
	raw, had := s.vault.ReadRaw(path)
	s.undo.files[path] = &undoFile{path: path, before: raw, had: had, isTask: isTask}
}

// CanUndo reports whether there is anything to undo.
func (s *Service) CanUndo() bool { return len(s.undoStack) > 0 }

// UndoLabel describes the change that Undo would reverse.
func (s *Service) UndoLabel() string {
	if !s.CanUndo() {
		return ""
	}
	return s.undoStack[len(s.undoStack)-1].label
}

// Undo reverses the most recent action and returns its label.
//
// It refuses when a file has changed since: the pre-image would throw away an
// edit made in an editor or another terminal, and that is a worse surprise
// than an undo that does not fire.
func (s *Service) Undo() (string, error) {
	if !s.CanUndo() {
		return "", fmt.Errorf("되돌릴 변경이 없습니다")
	}
	step := s.undoStack[len(s.undoStack)-1]
	for _, f := range step.files {
		mod, had := s.vault.StatFile(f.path)
		if had != f.postHad || (had && !mod.Equal(f.postMod)) {
			return "", fmt.Errorf("%s 이후 파일이 바깥에서 바뀌어 되돌릴 수 없습니다 (r 로 새로고침)", step.label)
		}
	}
	s.undoStack = s.undoStack[:len(s.undoStack)-1]

	// Drop the affected ids first: a restore may move a task back to a path the
	// index does not know, and a stale entry would resurrect it as a ghost.
	for id := range step.ids {
		s.idx.Remove(id)
	}
	for _, f := range step.files {
		if !f.had {
			if err := s.vault.RemoveFile(f.path); err != nil {
				return "", err
			}
			continue
		}
		if err := s.vault.WriteRaw(f.path, f.before); err != nil {
			return "", err
		}
		if !f.isTask {
			continue
		}
		t, err := s.vault.LoadTask(f.path)
		if err != nil {
			return "", fmt.Errorf("되돌린 파일을 읽을 수 없음: %w", err)
		}
		s.idx.Put(t)
	}
	if err := s.idx.Flush(); err != nil {
		return "", err
	}
	s.recordChange("되돌림: "+step.label, "undo: "+step.label)
	return step.label, nil
}

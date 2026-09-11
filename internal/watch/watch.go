// Package watch reports changes made to the vault from outside the process.
//
// Markdown is the source of truth, so an edit made in vim or on another
// terminal is as legitimate as one made in the TUI. Without this, the TUI would
// quietly show stale data until the next manual refresh.
package watch

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultDebounce coalesces the burst of events a single save produces
// (editors write via temp file + rename, which fires several events).
const DefaultDebounce = 300 * time.Millisecond

// Watcher emits one signal per settled burst of filesystem activity.
type Watcher struct {
	fsw      *fsnotify.Watcher
	events   chan struct{}
	errs     chan error
	root     string
	debounce time.Duration

	closeOnce sync.Once
	done      chan struct{}
}

// New starts watching a vault's task tree.
func New(tasksDir string, debounce time.Duration) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if debounce <= 0 {
		debounce = DefaultDebounce
	}
	w := &Watcher{
		fsw:      fsw,
		events:   make(chan struct{}, 1),
		errs:     make(chan error, 1),
		root:     tasksDir,
		debounce: debounce,
		done:     make(chan struct{}),
	}
	if err := w.addTree(tasksDir); err != nil {
		fsw.Close()
		return nil, err
	}
	go w.loop()
	return w, nil
}

// Events yields one value per settled change burst.
func (w *Watcher) Events() <-chan struct{} { return w.events }

// Errors yields watcher failures; the caller decides whether to surface them.
func (w *Watcher) Errors() <-chan error { return w.errs }

func (w *Watcher) Close() error {
	w.closeOnce.Do(func() { close(w.done) })
	return w.fsw.Close()
}

// addTree watches the directory and every existing subdirectory. fsnotify is
// not recursive, and the vault grows a new directory each month.
func (w *Watcher) addTree(dir string) error {
	return filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && p != dir {
			return filepath.SkipDir
		}
		return w.fsw.Add(p)
	})
}

func (w *Watcher) loop() {
	var timer *time.Timer
	var fire <-chan time.Time
	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if !w.interesting(ev) {
				continue
			}
			// A new month directory must be watched too, or the first task of
			// the month would go unnoticed.
			if ev.Has(fsnotify.Create) {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
					_ = w.addTree(ev.Name)
				}
			}
			if timer == nil {
				timer = time.NewTimer(w.debounce)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(w.debounce)
			}
			fire = timer.C
		case <-fire:
			fire = nil
			select {
			case w.events <- struct{}{}:
			default: // a signal is already pending; one is enough
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			select {
			case w.errs <- err:
			default:
			}
		}
	}
}

// interesting filters out the noise our own atomic writes generate.
func (w *Watcher) interesting(ev fsnotify.Event) bool {
	base := filepath.Base(ev.Name)
	if strings.HasPrefix(base, ".") {
		return false // temp files from WriteAtomic, editor swap files
	}
	if ev.Has(fsnotify.Chmod) {
		return false
	}
	if strings.HasSuffix(base, ".md") {
		return true
	}
	// Directory creation matters: it is a new month shard.
	st, err := os.Stat(ev.Name)
	return err == nil && st.IsDir()
}

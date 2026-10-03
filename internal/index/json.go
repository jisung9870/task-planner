package index

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"task-planner/internal/domain"
	"task-planner/internal/store"
)

const (
	indexFile = "index.json"
	metaFile  = "meta.json"
	lockName  = "lock"
)

type entry struct {
	Task    *domain.Task `json:"task"`
	ModTime time.Time    `json:"mtime"`
	Size    int64        `json:"size"`
}

type payload struct {
	Version int              `json:"version"`
	BuiltAt time.Time        `json:"built_at"`
	Entries map[string]entry `json:"entries"` // keyed by file path
}

// JSON is the default backend: the whole index is one file, loaded eagerly.
// At the scale this tool targets (a few thousand tasks) that is tens of
// milliseconds, and it keeps the vault free of binary state.
type JSON struct {
	mu    sync.RWMutex
	vault *store.Vault
	dir   string
	data  payload
	stats Stats
	dirty bool
}

var _ Index = (*JSON)(nil)

// NewJSON opens (or initialises) the index for a vault.
func NewJSON(v *store.Vault) (*JSON, error) {
	idx := &JSON{
		vault: v,
		dir:   v.IndexDir(),
		data:  payload{Version: SchemaVersion, Entries: map[string]entry{}},
	}
	if err := os.MkdirAll(idx.dir, 0o755); err != nil {
		return nil, fmt.Errorf("%s 생성 실패: %w", idx.dir, err)
	}
	if err := idx.load(); err != nil {
		// A corrupt or stale index is not an error: it is disposable.
		idx.data = payload{Version: SchemaVersion, Entries: map[string]entry{}}
	}
	return idx, nil
}

func (x *JSON) load() error {
	raw, err := os.ReadFile(filepath.Join(x.dir, indexFile))
	if err != nil {
		return err
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if p.Version != SchemaVersion || p.Entries == nil {
		return fmt.Errorf("index schema %d != %d", p.Version, SchemaVersion)
	}
	x.data = p
	x.stats.BuiltAt = p.BuiltAt
	return nil
}

// Sync re-parses only files whose size or mtime changed since the last run.
func (x *JSON) Sync() (Stats, error) { return x.sync(false) }

// Rebuild forces a full re-read.
func (x *JSON) Rebuild() (Stats, error) { return x.sync(true) }

func (x *JSON) sync(full bool) (Stats, error) {
	start := time.Now()
	files, err := x.vault.TaskFiles(false)
	if err != nil {
		return x.stats, err
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	if full {
		x.data.Entries = map[string]entry{}
	}

	seen := make(map[string]struct{}, len(files))
	scanned := 0
	var errs []error
	for _, path := range files {
		seen[path] = struct{}{}
		st, err := os.Stat(path)
		if err != nil {
			continue
		}
		if e, ok := x.data.Entries[path]; ok && e.Size == st.Size() && e.ModTime.Equal(st.ModTime()) {
			continue
		}
		t, err := x.vault.LoadTask(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		x.data.Entries[path] = entry{Task: t.Summary(), ModTime: st.ModTime(), Size: st.Size()}
		scanned++
	}

	removed := 0
	for path := range x.data.Entries {
		if _, ok := seen[path]; !ok {
			delete(x.data.Entries, path)
			removed++
		}
	}

	x.dirty = x.dirty || scanned > 0 || removed > 0
	x.data.BuiltAt = time.Now()
	x.stats = x.computeStats()
	x.stats.Scanned = scanned
	x.stats.Removed = removed
	x.stats.Duration = time.Since(start)
	x.stats.BuiltAt = x.data.BuiltAt

	if len(errs) > 0 {
		return x.stats, fmt.Errorf("%d개 파일 파싱 실패 (첫 오류: %w)", len(errs), errs[0])
	}
	return x.stats, nil
}

func (x *JSON) computeStats() Stats {
	s := Stats{Total: len(x.data.Entries)}
	for _, e := range x.data.Entries {
		if e.Task.IsOpen() {
			s.Open++
		}
	}
	return s
}

// Tasks returns summaries sorted by id for deterministic output.
func (x *JSON) Tasks() []*domain.Task {
	x.mu.RLock()
	defer x.mu.RUnlock()
	out := make([]*domain.Task, 0, len(x.data.Entries))
	for _, e := range x.data.Entries {
		out = append(out, e.Task)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (x *JSON) Get(id string) (*domain.Task, bool) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	for _, e := range x.data.Entries {
		if e.Task.ID == id {
			return e.Task, true
		}
	}
	return nil, false
}

// Put refreshes an entry right after a write so the caller sees its own change
// without waiting for the next Sync.
func (x *JSON) Put(t *domain.Task) {
	if t.Path == "" {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	for path, e := range x.data.Entries {
		if e.Task.ID == t.ID && path != t.Path {
			delete(x.data.Entries, path)
		}
	}
	e := entry{Task: t.Summary()}
	if st, err := os.Stat(t.Path); err == nil {
		e.ModTime, e.Size = st.ModTime(), st.Size()
	}
	x.data.Entries[t.Path] = e
	x.dirty = true
}

func (x *JSON) Remove(id string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	for path, e := range x.data.Entries {
		if e.Task.ID == id {
			delete(x.data.Entries, path)
			x.dirty = true
		}
	}
}

// Flush persists the index under an exclusive lock.
func (x *JSON) Flush() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.dirty {
		return nil
	}
	unlock, err := store.LockFile(filepath.Join(x.dir, lockName))
	if err != nil {
		return fmt.Errorf("인덱스 잠금 실패: %w", err)
	}
	defer unlock()

	raw, err := json.Marshal(x.data)
	if err != nil {
		return err
	}
	if err := store.WriteAtomic(filepath.Join(x.dir, indexFile), raw, 0o644); err != nil {
		return err
	}
	meta, _ := json.MarshalIndent(map[string]any{
		"schema_version": SchemaVersion,
		"built_at":       x.data.BuiltAt,
		"tasks":          len(x.data.Entries),
		"note":           "derived artifact - safe to delete, rebuilt from markdown",
	}, "", "  ")
	if err := store.WriteAtomic(filepath.Join(x.dir, metaFile), append(meta, '\n'), 0o644); err != nil {
		return err
	}
	x.dirty = false
	return nil
}

func (x *JSON) Stats() Stats {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.stats
}

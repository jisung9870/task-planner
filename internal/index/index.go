// Package index maintains a fast, disposable summary of the vault.
//
// The index is a derived artifact: deleting .index/ must never lose
// information. Any field that exists only here would break that invariant.
package index

import (
	"time"

	"task-planner/internal/domain"
)

// SchemaVersion is bumped whenever the on-disk entry shape changes; a mismatch
// triggers a full rebuild rather than a migration.
const SchemaVersion = 1

// Stats describes the last sync, for `tp index --status`.
type Stats struct {
	Total    int
	Open     int
	Scanned  int           // files re-parsed during the last sync
	Removed  int           // entries dropped because the file disappeared
	Duration time.Duration // wall clock of the last sync
	BuiltAt  time.Time
}

// Index is the query surface used by everything above the store.
//
// It is an interface so the JSON backend can be swapped for SQLite once the
// triggers in the planning doc fire (>10k tasks, cold start >300ms, or FTS).
type Index interface {
	// Sync reconciles the index with the vault, re-parsing only changed files.
	Sync() (Stats, error)
	// Rebuild discards everything and re-reads the vault.
	Rebuild() (Stats, error)
	// Tasks returns every summary; callers must not mutate the results.
	Tasks() []*domain.Task
	// Get looks up by task id.
	Get(id string) (*domain.Task, bool)
	// Put refreshes one entry after a write.
	Put(t *domain.Task)
	// Remove drops one entry.
	Remove(id string)
	// Flush persists the index to disk.
	Flush() error
	// Stats reports the last sync result.
	Stats() Stats
}

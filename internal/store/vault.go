package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"task-planner/internal/domain"
)

// Vault directory names. The layout is documented in
// docs/product-task-planner-202609.md.
const (
	DirTasks    = "tasks"
	DirProjects = "projects"
	DirArchive  = "archive"
	DirReports  = "reports"
	DirIndex    = ".index"
	FileInbox   = "inbox.md"
)

// Vault is the filesystem-backed task store.
type Vault struct{ root string }

func New(root string) *Vault { return &Vault{root: root} }

func (v *Vault) Root() string        { return v.root }
func (v *Vault) TasksDir() string    { return filepath.Join(v.root, DirTasks) }
func (v *Vault) ProjectsDir() string { return filepath.Join(v.root, DirProjects) }
func (v *Vault) ArchiveDir() string  { return filepath.Join(v.root, DirArchive) }
func (v *Vault) ReportsDir() string  { return filepath.Join(v.root, DirReports) }
func (v *Vault) IndexDir() string    { return filepath.Join(v.root, DirIndex) }
func (v *Vault) InboxPath() string   { return filepath.Join(v.root, FileInbox) }

// Init creates the directory skeleton. Safe to call repeatedly.
func (v *Vault) Init() error {
	for _, d := range []string{v.root, v.TasksDir(), v.ProjectsDir(), v.ArchiveDir(), v.ReportsDir(), v.IndexDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("%s 생성 실패: %w", d, err)
		}
	}
	return nil
}

// Exists reports whether the vault skeleton is present.
func (v *Vault) Exists() bool {
	st, err := os.Stat(v.TasksDir())
	return err == nil && st.IsDir()
}

// TaskPath builds the canonical location for a task: month-sharded so that no
// directory grows past a few hundred entries.
func (v *Vault) TaskPath(t *domain.Task) string {
	month := t.Created
	if month.IsZero() {
		month = domain.Today()
	}
	return filepath.Join(v.TasksDir(), month.Time().Format("2006-01"),
		fmt.Sprintf("%s-%s.md", t.ID, domain.Slug(t.Title)))
}

// TaskFiles lists every task file, optionally including the archive.
func (v *Vault) TaskFiles(includeArchive bool) ([]string, error) {
	dirs := []string{v.TasksDir()}
	if includeArchive {
		dirs = append(dirs, v.ArchiveDir())
	}
	var out []string
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && p != dir {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(d.Name(), ".md") && !strings.HasPrefix(d.Name(), ".") {
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("%s 탐색 실패: %w", dir, err)
		}
	}
	sort.Strings(out)
	return out, nil
}

// LoadTask reads and parses one task file.
func (v *Vault) LoadTask(path string) (*domain.Task, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeTask(raw, path)
}

// SaveTask writes a task atomically. The path is recomputed from id+title so a
// renamed task does not leave a stale filename behind.
func (v *Vault) SaveTask(t *domain.Task) error {
	if err := t.Validate(); err != nil {
		return err
	}
	want := v.TaskPath(t)
	raw, err := EncodeTask(t)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		return err
	}
	if err := WriteAtomic(want, raw, 0o644); err != nil {
		return err
	}
	if t.Path != "" && t.Path != want {
		// Title changed: drop the old file rather than leaving a duplicate that
		// the next index rebuild would resurrect.
		_ = os.Remove(t.Path)
	}
	t.Path = want
	return nil
}

// DeleteTask removes a task file.
func (v *Vault) DeleteTask(t *domain.Task) error {
	if t.Path == "" {
		return fmt.Errorf("%s: 파일 경로를 알 수 없음", t.ID)
	}
	return os.Remove(t.Path)
}

// ArchivePath is where a completed task is moved to, sharded by quarter.
func (v *Vault) ArchivePath(t *domain.Task) string {
	d := t.Completed
	if d.IsZero() {
		d = t.Updated
	}
	if d.IsZero() {
		d = domain.Today()
	}
	q := (int(d.Time().Month())-1)/3 + 1
	return filepath.Join(v.ArchiveDir(), fmt.Sprintf("%d-Q%d", d.Time().Year(), q), filepath.Base(v.TaskPath(t)))
}

// ArchiveTask moves a finished task out of tasks/ into archive/, returning its
// new path. The file keeps its name and frontmatter: archiving is a move, not
// a transformation, so `tp index --rebuild` on the archive would reproduce the
// same tasks.
func (v *Vault) ArchiveTask(t *domain.Task) (string, error) {
	if t.Path == "" {
		return "", fmt.Errorf("%s: 파일 경로를 알 수 없음", t.ID)
	}
	dst := v.ArchivePath(t)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(dst); err == nil {
		return "", fmt.Errorf("%s: 아카이브에 같은 이름이 이미 있음", dst)
	}
	// tasks/ and archive/ are both inside the vault root, so a rename is a
	// rename - no copy path to get wrong.
	if err := os.Rename(t.Path, dst); err != nil {
		return "", err
	}
	t.Path = dst
	return dst, nil
}

// ReadRaw returns a file's bytes and whether it existed. Undo works on whole
// files rather than on fields: a pre-image restores the log line and the
// completion date together with whatever changed, which field-by-field
// bookkeeping would have to reproduce by hand.
func (v *Vault) ReadRaw(path string) ([]byte, bool) {
	if !v.contains(path) {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return raw, true
}

// WriteRaw restores a file from a pre-image.
func (v *Vault) WriteRaw(path string, raw []byte) error {
	if !v.contains(path) {
		return fmt.Errorf("vault 바깥 경로에는 쓸 수 없음: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return WriteAtomic(path, raw, 0o644)
}

// RemoveFile deletes a file that a pre-image says should not exist.
func (v *Vault) RemoveFile(path string) error {
	if !v.contains(path) {
		return fmt.Errorf("vault 바깥 경로는 지울 수 없음: %s", path)
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// StatFile reports a file's modification time and whether it exists.
func (v *Vault) StatFile(path string) (time.Time, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	return st.ModTime(), true
}

// contains guards the raw helpers: every path they touch comes from a task, so
// one outside the vault means a bug, and the write would land somewhere the
// user never agreed to.
func (v *Vault) contains(path string) bool {
	if path == "" {
		return false
	}
	rel, err := filepath.Rel(v.root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// LoadProjects reads every projects/<slug>/project.md.
func (v *Vault) LoadProjects() ([]*domain.Project, error) {
	entries, err := os.ReadDir(v.ProjectsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*domain.Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(v.ProjectsDir(), e.Name(), "project.md")
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		p, err := DecodeProject(raw, path)
		if err != nil {
			return nil, err
		}
		if p.Slug == "" {
			p.Slug = e.Name()
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// ProjectPath is where a project's metadata lives.
func (v *Vault) ProjectPath(slug string) string {
	return filepath.Join(v.ProjectsDir(), slug, "project.md")
}

// SaveProject writes a project file atomically.
func (v *Vault) SaveProject(p *domain.Project) error {
	if strings.TrimSpace(p.Slug) == "" {
		return fmt.Errorf("프로젝트 slug 가 비어 있음")
	}
	raw, err := EncodeProject(p)
	if err != nil {
		return err
	}
	path := v.ProjectPath(p.Slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := WriteAtomic(path, raw, 0o644); err != nil {
		return err
	}
	p.Path = path
	return nil
}

package service

import (
	"fmt"
	"sort"
	"strings"

	"task-planner/internal/domain"
	"task-planner/internal/query"
)

// ProjectStatuses is the cycle a project moves through. Empty means active, so
// a hand-written project.md without the field still behaves.
var ProjectStatuses = []string{"active", "paused", "done"}

// ProjectRow merges the task rollup with the projects/<slug>/project.md
// metadata. The rollup alone cannot show a project that has no tasks yet -
// which is exactly the state a project is in the moment it is created.
type ProjectRow struct {
	query.ProjectCount
	Name   string
	Status string
	Owner  string
	Due    domain.Date
	// Defined is true when a project.md exists. A slug that only appears in a
	// task's frontmatter is still a real project for counting purposes, but it
	// has no metadata to edit.
	Defined bool
	Path    string
}

// Label prefers the display name over the slug.
func (r ProjectRow) Label() string {
	if r.Name != "" {
		return r.Name
	}
	return query.ProjectLabel(r.Slug)
}

// Active reports whether the project is still being worked on.
func (r ProjectRow) Active() bool { return r.Status == "" || r.Status == "active" }

// ProjectRows is the Projects view: every project that has tasks, plus every
// project that has a file, in one list.
func (s *Service) ProjectRows() ([]ProjectRow, error) {
	rows := map[string]*ProjectRow{}
	var order []string
	for _, c := range s.ProjectCounts() {
		rows[c.Slug] = &ProjectRow{ProjectCount: c}
		order = append(order, c.Slug)
	}
	defined, err := s.Projects()
	if err != nil {
		return nil, err
	}
	for _, p := range defined {
		r, ok := rows[p.Slug]
		if !ok {
			r = &ProjectRow{ProjectCount: query.ProjectCount{Slug: p.Slug}}
			rows[p.Slug] = r
			order = append(order, p.Slug)
		}
		r.Name, r.Status, r.Owner, r.Due = p.Name, p.Status, p.Owner, p.Due
		r.Defined, r.Path = true, p.Path
	}
	out := make([]ProjectRow, 0, len(order))
	for _, slug := range order {
		out = append(out, *rows[slug])
	}
	sort.SliceStable(out, func(i, j int) bool {
		// Work still open decides the order; a finished or empty project drops
		// to the bottom where it does not compete for attention.
		if out[i].Open != out[j].Open {
			return out[i].Open > out[j].Open
		}
		if out[i].Active() != out[j].Active() {
			return out[i].Active()
		}
		if (out[i].Slug == "") != (out[j].Slug == "") {
			return out[j].Slug == "" // 미지정 is a bucket, not a project
		}
		return out[i].Slug < out[j].Slug
	})
	return out, nil
}

// ProjectSlugs lists every slug in use, for prompt completion.
func (s *Service) ProjectSlugs() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range s.All() {
		if t.Project != "" && !seen[t.Project] {
			seen[t.Project] = true
			out = append(out, t.Project)
		}
	}
	if defined, err := s.Projects(); err == nil {
		for _, p := range defined {
			if !seen[p.Slug] {
				seen[p.Slug] = true
				out = append(out, p.Slug)
			}
		}
	}
	sort.Strings(out)
	return out
}

// ProjectBySlug reads one project file.
func (s *Service) ProjectBySlug(slug string) (*domain.Project, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, fmt.Errorf("(미지정) 은 프로젝트가 아닙니다")
	}
	defined, err := s.Projects()
	if err != nil {
		return nil, err
	}
	for _, p := range defined {
		if strings.EqualFold(p.Slug, slug) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("프로젝트 파일이 없음: %s", slug)
}

// CreateProject writes projects/<slug>/project.md. The slug is what tasks
// reference, so it is normalised the same way a task filename is.
func (s *Service) CreateProject(slug, name string) (*domain.Project, error) {
	slug = ProjectSlug(slug)
	if slug == "" {
		return nil, fmt.Errorf("프로젝트 slug 가 비어 있음")
	}
	if _, err := s.ProjectBySlug(slug); err == nil {
		return nil, fmt.Errorf("이미 있는 프로젝트: %s", slug)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = slug
	}
	p := &domain.Project{
		Slug:   slug,
		Name:   name,
		Status: "active",
		Body:   "## 목표\n\n## 메모\n",
	}
	if err := s.vault.SaveProject(p); err != nil {
		return nil, err
	}
	s.recordChange("프로젝트 "+slug+" 추가", fmt.Sprintf("project %s (%s): 추가", slug, name))
	return p, nil
}

// EnsureProject returns the project file for a slug, creating it when the slug
// so far exists only in task frontmatter. That is how most projects come into
// being: a task gets a project name long before anyone writes a project file.
//
// It is deliberately not what EditProject does - a CLI typo must not create a
// project, and there the slug is typed rather than picked off a row.
func (s *Service) EnsureProject(slug string) (*domain.Project, error) {
	p, err := s.ProjectBySlug(slug)
	if err == nil {
		return p, nil
	}
	return s.CreateProject(slug, "")
}

// ProjectEditInput carries partial project updates; nil means "leave alone".
type ProjectEditInput struct {
	Name   *string
	Status *string
	Owner  *string
	Due    *domain.Date
}

// EditProject updates project.md. Unlike a task there is no index to refresh:
// projects are read straight from the vault whenever the view needs them.
func (s *Service) EditProject(slug string, in ProjectEditInput) (*domain.Project, error) {
	p, err := s.ProjectBySlug(slug)
	if err != nil {
		return nil, err
	}
	var changes []string
	if in.Name != nil && *in.Name != p.Name {
		changes = append(changes, "name="+*in.Name)
		p.Name = *in.Name
	}
	if in.Status != nil && *in.Status != p.Status {
		if err := validProjectStatus(*in.Status); err != nil {
			return nil, err
		}
		changes = append(changes, "status="+*in.Status)
		p.Status = *in.Status
	}
	if in.Owner != nil && *in.Owner != p.Owner {
		changes = append(changes, "owner="+*in.Owner)
		p.Owner = *in.Owner
	}
	if in.Due != nil && !in.Due.Equal(p.Due) {
		changes = append(changes, "due="+in.Due.String())
		p.Due = *in.Due
	}
	if len(changes) == 0 {
		return p, nil
	}
	if err := s.vault.SaveProject(p); err != nil {
		return nil, err
	}
	s.recordChange("프로젝트 "+p.Slug+" 수정", fmt.Sprintf("project %s: %s", p.Slug, strings.Join(changes, " ")))
	return p, nil
}

// CycleProjectStatus advances active → paused → done → active. One key on the
// Projects tab is worth more than a prompt nobody remembers the values for.
func (s *Service) CycleProjectStatus(slug string) (*domain.Project, error) {
	p, err := s.ProjectBySlug(slug)
	if err != nil {
		return nil, err
	}
	cur := p.Status
	if cur == "" {
		cur = "active"
	}
	next := ProjectStatuses[0]
	for i, st := range ProjectStatuses {
		if st == cur {
			next = ProjectStatuses[(i+1)%len(ProjectStatuses)]
			break
		}
	}
	return s.EditProject(p.Slug, ProjectEditInput{Status: &next})
}

// ProjectSlug normalises a typed slug: lowercase, spaces to dashes. Hangul is
// kept - the vault is browsed by humans.
func ProjectSlug(s string) string { return domain.Slug(s) }

func validProjectStatus(st string) error {
	for _, x := range ProjectStatuses {
		if x == st {
			return nil
		}
	}
	return fmt.Errorf("프로젝트 상태는 %s 중 하나여야 함: %q", strings.Join(ProjectStatuses, " / "), st)
}

// Package store reads and writes the markdown files that are the single source
// of truth. Nothing above this package may touch the filesystem directly.
package store

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"github.com/goccy/go-yaml"

	"task-planner/internal/domain"
)

const fence = "---"

// splitFrontmatter separates the YAML block from the markdown body.
func splitFrontmatter(raw []byte) (fm []byte, body string, err error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	if !strings.HasPrefix(text, fence+"\n") {
		return nil, "", fmt.Errorf("frontmatter(---) 로 시작하지 않음")
	}
	rest := text[len(fence)+1:]
	end := strings.Index(rest, "\n"+fence)
	if end < 0 {
		return nil, "", fmt.Errorf("frontmatter 종료 표시(---) 를 찾을 수 없음")
	}
	fm = []byte(rest[:end+1])
	after := rest[end+1+len(fence):]
	return fm, strings.TrimLeft(after, "\n"), nil
}

// DecodeTask parses one task file.
func DecodeTask(raw []byte, path string) (*domain.Task, error) {
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var t domain.Task
	if err := yaml.Unmarshal(fm, &t); err != nil {
		return nil, fmt.Errorf("%s: frontmatter 파싱 실패: %w", path, err)
	}
	pruneExtra(&t.Extra, reflect.TypeOf(domain.Task{}))
	t.Body = body
	t.Path = path
	if t.Status == "" {
		t.Status = domain.StatusTodo
	}
	return &t, nil
}

// EncodeTask renders a task back to file bytes.
func EncodeTask(t *domain.Task) ([]byte, error) {
	fm, err := yaml.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("%s: frontmatter 직렬화 실패: %w", t.ID, err)
	}
	var buf bytes.Buffer
	buf.WriteString(fence + "\n")
	buf.Write(fm)
	if !bytes.HasSuffix(fm, []byte("\n")) {
		buf.WriteByte('\n')
	}
	buf.WriteString(fence + "\n")
	body := strings.TrimRight(t.Body, "\n")
	if body != "" {
		buf.WriteString("\n")
		buf.WriteString(body)
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}

// DecodeProject parses a project.md file.
func DecodeProject(raw []byte, path string) (*domain.Project, error) {
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var p domain.Project
	if err := yaml.Unmarshal(fm, &p); err != nil {
		return nil, fmt.Errorf("%s: frontmatter 파싱 실패: %w", path, err)
	}
	pruneExtra(&p.Extra, reflect.TypeOf(domain.Project{}))
	p.Body = body
	p.Path = path
	return &p, nil
}

// EncodeProject renders a project back to file bytes.
func EncodeProject(p *domain.Project) ([]byte, error) {
	fm, err := yaml.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("%s: frontmatter 직렬화 실패: %w", p.Slug, err)
	}
	var buf bytes.Buffer
	buf.WriteString(fence + "\n")
	buf.Write(fm)
	if !bytes.HasSuffix(fm, []byte("\n")) {
		buf.WriteByte('\n')
	}
	buf.WriteString(fence + "\n")
	body := strings.TrimRight(p.Body, "\n")
	if body != "" {
		buf.WriteString("\n" + body + "\n")
	}
	return buf.Bytes(), nil
}

// pruneExtra removes keys the struct already models. The inline map collects
// every key, so without this the "unknown keys" bag would shadow real fields
// and grow the file on every save.
func pruneExtra(extra *map[string]any, typ reflect.Type) {
	if *extra == nil {
		return
	}
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("yaml")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			delete(*extra, name)
		}
	}
	if len(*extra) == 0 {
		*extra = nil
	}
}

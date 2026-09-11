package query

import (
	"fmt"
	"strconv"
	"strings"

	"task-planner/internal/domain"
)

// Filter is a compiled query expression.
type Filter struct {
	terms []term
	src   string
}

// String returns the original expression.
func (f *Filter) String() string { return f.src }

// Empty reports whether the filter matches everything.
func (f *Filter) Empty() bool { return f == nil || len(f.terms) == 0 }

// Match reports whether a task satisfies every term (terms are ANDed; OR is
// deliberately absent - it has never been needed to answer the questions in
// the planning doc, and adding it would require precedence rules).
func (f *Filter) Match(t *domain.Task, today domain.Date, dueSoonDays int) bool {
	if f.Empty() {
		return true
	}
	for _, tm := range f.terms {
		if tm.match(t, today, dueSoonDays) == tm.negated {
			return false
		}
	}
	return true
}

// Apply filters and sorts a task list.
func (f *Filter) Apply(ts []*domain.Task, today domain.Date, dueSoonDays int) []*domain.Task {
	var out []*domain.Task
	for _, t := range ts {
		if f.Match(t, today, dueSoonDays) {
			out = append(out, t)
		}
	}
	domain.SortDefault(out, today)
	return out
}

type term struct {
	negated bool
	match   func(*domain.Task, domain.Date, int) bool
}

type cmp int

const (
	cmpEq cmp = iota
	cmpLT
	cmpLTE
	cmpGT
	cmpGTE
)

// ParseFilter compiles a query string.
//
//	status:doing project:infra due<7d tag:ops is:overdue -status:done 파이프라인
//
// A bare word is a case-insensitive substring match over title, project and
// tags - the common case has to be typeable without syntax.
func ParseFilter(expr string, today domain.Date) (*Filter, error) {
	f := &Filter{src: strings.TrimSpace(expr)}
	for _, tok := range tokenize(expr) {
		t, err := parseTerm(tok, today)
		if err != nil {
			return nil, err
		}
		f.terms = append(f.terms, t)
	}
	return f, nil
}

// tokenize splits on whitespace while keeping "quoted phrases" intact.
func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

func parseTerm(tok string, today domain.Date) (term, error) {
	t := term{}
	for len(tok) > 0 && (tok[0] == '-' || tok[0] == '!') {
		// A leading "-" only negates when a field follows; "-3d" is not a term.
		if tok[0] == '-' && !strings.ContainsAny(tok, ":<>=") {
			break
		}
		t.negated = !t.negated
		tok = tok[1:]
	}
	if tok == "" {
		return t, fmt.Errorf("빈 조건")
	}
	if strings.HasPrefix(tok, "#") {
		tag := tok[1:]
		t.match = func(task *domain.Task, _ domain.Date, _ int) bool { return task.HasTag(tag) }
		return t, nil
	}

	field, op, value, ok := splitTerm(tok)
	if !ok {
		needle := strings.ToLower(tok)
		t.match = func(task *domain.Task, _ domain.Date, _ int) bool {
			hay := strings.ToLower(task.Title + " " + task.Project + " " + strings.Join(task.Tags, " "))
			return strings.Contains(hay, needle)
		}
		return t, nil
	}

	switch field {
	case "status", "s":
		st, err := domain.ParseStatus(value)
		if err != nil {
			return t, err
		}
		t.match = func(task *domain.Task, _ domain.Date, _ int) bool { return task.Status == st }
	case "project", "proj", "p":
		want := value
		t.match = func(task *domain.Task, _ domain.Date, _ int) bool {
			if want == "none" || want == "" {
				return task.Project == ""
			}
			return strings.EqualFold(task.Project, want)
		}
	case "tag", "t":
		want := value
		t.match = func(task *domain.Task, _ domain.Date, _ int) bool { return task.HasTag(want) }
	case "priority", "prio":
		p, err := domain.ParsePriority(value)
		if err != nil {
			return t, err
		}
		t.match = func(task *domain.Task, _ domain.Date, _ int) bool { return task.Priority == p }
	case "due":
		return dateTerm(t, op, value, today, func(task *domain.Task) domain.Date { return task.Due })
	case "scheduled", "sched":
		return dateTerm(t, op, value, today, func(task *domain.Task) domain.Date { return task.Scheduled })
	case "rollover", "carry":
		n, err := strconv.Atoi(value)
		if err != nil {
			return t, fmt.Errorf("rollover 값은 숫자여야 함: %q", value)
		}
		t.match = func(task *domain.Task, _ domain.Date, _ int) bool {
			return compareInt(task.RolloverCount, op, n)
		}
	case "is":
		m, err := isTerm(value)
		if err != nil {
			return t, err
		}
		t.match = m
	case "id":
		want := strings.ToLower(value)
		t.match = func(task *domain.Task, _ domain.Date, _ int) bool {
			return strings.Contains(strings.ToLower(task.ID), want)
		}
	default:
		return t, fmt.Errorf("알 수 없는 필드: %q (status|project|tag|priority|due|scheduled|rollover|is|id)", field)
	}
	return t, nil
}

// splitTerm pulls "field<op>value" apart.
func splitTerm(tok string) (field string, op cmp, value string, ok bool) {
	for i := 0; i < len(tok); i++ {
		switch tok[i] {
		case ':':
			return strings.ToLower(tok[:i]), cmpEq, tok[i+1:], i > 0
		case '<':
			if i+1 < len(tok) && tok[i+1] == '=' {
				return strings.ToLower(tok[:i]), cmpLTE, tok[i+2:], i > 0
			}
			return strings.ToLower(tok[:i]), cmpLT, tok[i+1:], i > 0
		case '>':
			if i+1 < len(tok) && tok[i+1] == '=' {
				return strings.ToLower(tok[:i]), cmpGTE, tok[i+2:], i > 0
			}
			return strings.ToLower(tok[:i]), cmpGT, tok[i+1:], i > 0
		}
	}
	return "", cmpEq, "", false
}

func dateTerm(t term, op cmp, value string, today domain.Date, get func(*domain.Task) domain.Date) (term, error) {
	if value == "none" || value == "" {
		t.match = func(task *domain.Task, _ domain.Date, _ int) bool { return get(task).IsZero() }
		return t, nil
	}
	if value == "any" {
		t.match = func(task *domain.Task, _ domain.Date, _ int) bool { return !get(task).IsZero() }
		return t, nil
	}
	want, err := parseQueryDate(value, today)
	if err != nil {
		return t, err
	}
	t.match = func(task *domain.Task, _ domain.Date, _ int) bool {
		d := get(task)
		if d.IsZero() {
			return false // an absent date satisfies no comparison
		}
		return compareDate(d, op, want)
	}
	return t, nil
}

// parseQueryDate accepts absolute dates and the relative forms worth typing.
func parseQueryDate(v string, today domain.Date) (domain.Date, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "today", "오늘":
		return today, nil
	case "tomorrow", "내일":
		return today.AddDays(1), nil
	case "yesterday", "어제":
		return today.AddDays(-1), nil
	case "week":
		return today.WeekStart().AddDays(6), nil
	}
	// 7d / +7d / -7d / 2w / 1m relative to today.
	if n, unit, ok := splitRelative(v); ok {
		switch unit {
		case 'w':
			return today.AddDays(n * 7), nil
		case 'm':
			return today.AddMonths(n), nil
		default:
			return today.AddDays(n), nil
		}
	}
	return domain.ParseDate(v)
}

func splitRelative(v string) (n int, unit byte, ok bool) {
	if v == "" {
		return 0, 0, false
	}
	unit = 'd'
	body := v
	if last := body[len(body)-1]; last == 'd' || last == 'w' || last == 'm' {
		unit = last
		body = body[:len(body)-1]
	} else {
		return 0, 0, false // a bare number is ambiguous; require a unit
	}
	body = strings.TrimPrefix(body, "+")
	n, err := strconv.Atoi(body)
	return n, unit, err == nil
}

func isTerm(value string) (func(*domain.Task, domain.Date, int) bool, error) {
	switch strings.ToLower(value) {
	case "open", "열림":
		return func(t *domain.Task, _ domain.Date, _ int) bool { return t.IsOpen() }, nil
	case "closed", "done", "완료":
		return func(t *domain.Task, _ domain.Date, _ int) bool { return t.Status.Terminal() }, nil
	case "overdue", "마감초과":
		return func(t *domain.Task, today domain.Date, _ int) bool { return t.Overdue(today) }, nil
	case "duesoon", "due-soon", "임박":
		return func(t *domain.Task, today domain.Date, n int) bool { return t.DueSoon(today, n) }, nil
	case "blocked", "보류":
		return func(t *domain.Task, _ domain.Date, _ int) bool { return t.Status == domain.StatusBlocked }, nil
	case "carried", "이월":
		return func(t *domain.Task, _ domain.Date, _ int) bool { return t.RolloverCount > 0 }, nil
	case "unscheduled", "미배정":
		return func(t *domain.Task, _ domain.Date, _ int) bool { return t.Scheduled.IsZero() }, nil
	case "recurring", "반복":
		return func(t *domain.Task, _ domain.Date, _ int) bool { return t.Recur != "" }, nil
	}
	return nil, fmt.Errorf("알 수 없는 is 값: %q (open|closed|overdue|duesoon|blocked|carried|unscheduled|recurring)", value)
}

func compareDate(got domain.Date, op cmp, want domain.Date) bool {
	switch op {
	case cmpLT:
		return got.Before(want)
	case cmpLTE:
		return !got.After(want)
	case cmpGT:
		return got.After(want)
	case cmpGTE:
		return !got.Before(want)
	default:
		return got.Equal(want)
	}
}

func compareInt(got int, op cmp, want int) bool {
	switch op {
	case cmpLT:
		return got < want
	case cmpLTE:
		return got <= want
	case cmpGT:
		return got > want
	case cmpGTE:
		return got >= want
	default:
		return got == want
	}
}

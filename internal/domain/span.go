package domain

import (
	"fmt"
	"strings"
)

// Span is a task's 진행 기간 as typed at a prompt: 착수 예정일(scheduled) 부터
// 마감일(due) 까지. Deprecated with due (2026-10-03): only the MCP span
// argument still parses it, for one release. There is no separate pair of fields for it - a period is
// exactly "언제 시작해서 언제까지" , which the two existing dates already say.
//
// SetStart/SetEnd separate "이 필드는 건드리지 마" from "이 필드를 비워" ; a
// zero Date alone cannot express both.
type Span struct {
	Start, End       Date
	SetStart, SetEnd bool
}

// spanSeps are the range separators accepted at a prompt. '-' is not one of
// them: it already means "빼기" in +3d and sits inside every date.
var spanSeps = []string{"~", "..", "—"}

// ParseSpan reads a 기간 expression.
//
//	09-15~09-19     시작·마감 둘 다
//	today~+4d       오른쪽 상대값은 시작일 기준
//	09-15           시작만 (마감은 그대로)
//	~09-19          마감만
//	09-15~          시작만 두고 마감 해제
//	-               둘 다 해제
func ParseSpan(s string, today Date) (Span, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Span{}, fmt.Errorf("기간을 입력하세요 (예: 09-15~09-19, today~+4d, - 로 해제)")
	}
	switch strings.ToLower(s) {
	case "-", "none", "clear", "해제":
		return Span{SetStart: true, SetEnd: true}, nil
	}

	sep := ""
	for _, c := range spanSeps {
		if strings.Contains(s, c) {
			sep = c
			break
		}
	}
	if sep == "" {
		d, err := ParseDateRef(s, today)
		if err != nil {
			return Span{}, err
		}
		return Span{Start: d, SetStart: true}, nil
	}

	rawL, rawR, _ := strings.Cut(s, sep)
	left, right := strings.TrimSpace(rawL), strings.TrimSpace(rawR)
	if left == "" && right == "" {
		return Span{SetStart: true, SetEnd: true}, nil
	}

	sp := Span{SetStart: left != "", SetEnd: true}
	if left != "" {
		d, err := ParseDateRef(left, today)
		if err != nil {
			return Span{}, err
		}
		sp.Start = d
	}
	if right != "" {
		// "09-15~+4d" counts the tail from the start, which is how a period is
		// read aloud ("15일부터 나흘"). Without a start it falls back to today.
		base := today
		if sp.SetStart && !sp.Start.IsZero() && strings.HasPrefix(right, "+") {
			base = sp.Start
		}
		d, err := ParseDateRef(right, base)
		if err != nil {
			return Span{}, err
		}
		sp.End = d
	}
	if sp.SetStart && !sp.Start.IsZero() && !sp.End.IsZero() && sp.End.Before(sp.Start) {
		return Span{}, fmt.Errorf("기간의 끝(%s)이 시작(%s)보다 빠름", sp.End, sp.Start)
	}
	return sp, nil
}

// String renders a span the way the prompt accepts it back.
func (sp Span) String() string {
	switch {
	case sp.Start.IsZero() && sp.End.IsZero():
		return ""
	case sp.End.IsZero():
		return sp.Start.String()
	case sp.Start.IsZero():
		return "~" + sp.End.String()
	case sp.Start.Equal(sp.End):
		return sp.Start.String()
	}
	return sp.Start.String() + "~" + sp.End.String()
}

// Package style enforces how a task reads, not what it holds.
//
// The rules are adapter policy rather than domain truth: an agent writing over
// MCP gets them, a human typing `tp add` does not. A personal tracker fills up
// with rows nobody can read three weeks later one convenient shortcut at a
// time, and it is the model taking the shortcut that needs to be told.
//
// Structure is rejected, wording is only warned about. A regex cannot tell a
// stiff sentence from a careful one, and a model refused over tone rewrites
// forever without converging - so the hard rules are four patterns that mean
// "this is a document, not a task", and everything else rides back as advice.
package style

import (
	"fmt"
	"regexp"
	"strings"
)

// Field names an Issue's origin, matching the MCP argument it came from.
const (
	FieldTitle    = "title"
	FieldNote     = "note"
	FieldReason   = "reason"
	FieldNoteLine = "note_line"
)

// ConventionsURI is the MCP resource holding the full rules. Every hard
// rejection points at it: a convention document is read when the caller is
// blocked, never because a tool description asked it to be.
const ConventionsURI = "tp://conventions"

// Issue is one violation. Hard rejects the call, soft rides back as a warning.
type Issue struct {
	Field string
	Hard  bool
	// Msg says what is wrong and where the removed material belongs. A refusal
	// without a destination just gets retried verbatim.
	Msg string
}

const (
	// titleHardLimit rejects what is a paragraph in disguise; titleSoftLimit is
	// the length actually being asked for. The title is also the filename.
	titleHardLimit = 60
	titleSoftLimit = 40
	// A note is 2~4 lines. These bounds are where it has clearly become a doc.
	noteSoftLines = 6
	noteSoftRunes = 400
	// reasonVagueLimit: below this, with no name and no number, a hold reason
	// says nothing at all.
	reasonVagueLimit = 15
)

var (
	// The four hard patterns: each one is a document trying to be a task.
	reHeading  = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s`)
	reCheckbox = regexp.MustCompile(`(?m)^\s*[-*+]\s*\[[ xX]\]`)
	reNumbered = regexp.MustCompile(`(?m)^\s*\d+\.\s`)

	reBold      = regexp.MustCompile(`\*\*[^*\n]+\*\*`)
	reTicket    = regexp.MustCompile(`\b[A-Z]{2,}-\d+\b`)
	reBracket   = regexp.MustCompile(`^\s*\[[^\]]+\]`)
	reVerbEnd   = regexp.MustCompile(`(하기|한다|된다|하자|할 것|하기로|필요|요망|바람)$`)
	reConjoined = regexp.MustCompile(`(^|\s)(및|그리고)(\s|$)`)
	rePlan      = regexp.MustCompile(`(필요함|필요하다|해야 함|할 예정|예정임|예정\.|예정$)`)
	reDigit     = regexp.MustCompile(`\d`)
	reLatin     = regexp.MustCompile(`[A-Za-z]`)
)

// timeWords are the dates a person actually writes. "어제 롤백 때" pins a note
// in time as well as "09-12" does, and warning about it would train the caller
// to ignore the warnings.
var timeWords = []string{
	"어제", "오늘", "내일", "모레", "그제", "그저께", "방금", "아까",
	"아침", "점심", "저녁", "새벽", "오전", "오후",
	"지난주", "지난 주", "이번주", "이번 주", "다음주", "다음 주",
	"지난달", "이번달", "다음달", "월초", "월말", "분기", "주말",
}

// deadlineWords are the ways a hold announces when it ends.
var deadlineWords = []string{"까지", "내로", "내에", "D-", "d-", "안에"}

// hasFact reports whether the text pins anything down - a number or a moment.
// It is the machine-checkable half of "3주 뒤의 내가 이 줄을 쓸 수 있는가".
func hasFact(s string) bool {
	if reDigit.MatchString(s) {
		return true
	}
	return containsAny(s, timeWords)
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// CheckTitle validates a task title: one noun phrase, one job.
func CheckTitle(s string) []Issue {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil // service rejects an empty title with a better message
	}
	var is []Issue
	hard := func(msg string) { is = append(is, Issue{Field: FieldTitle, Hard: true, Msg: msg}) }
	soft := func(msg string) { is = append(is, Issue{Field: FieldTitle, Msg: msg}) }

	if strings.ContainsAny(s, "\n\r") {
		hard("제목은 한 줄 — 나머지는 note 로 옮길 것")
	}
	if n := len([]rune(s)); n > titleHardLimit {
		hard(fmt.Sprintf("제목이 너무 김 (%d자, 최대 %d자) — %d자 안쪽 명사구로 줄이고 상세는 note 로 옮길 것",
			n, titleHardLimit, titleSoftLimit))
	} else if n > titleSoftLimit {
		soft(fmt.Sprintf("제목이 깁니다 (%d자) — %d자 안쪽 명사구를 권장", n, titleSoftLimit))
	}
	if reVerbEnd.MatchString(s) {
		soft(`제목은 명사구로 — "~를 구현한다" 대신 무엇에 대한 일인지`)
	}
	if reConjoined.MatchString(s) {
		soft("한 태스크는 한 일 — 제목에 두 가지가 묶여 있으면 태스크를 나눌 것")
	}
	if reBracket.MatchString(s) {
		soft("대괄호 접두는 project·tags 필드로 (제목에서 지울 것)")
	}
	if m := reTicket.FindString(s); m != "" {
		soft(fmt.Sprintf("티켓 번호 %s 는 links 로 (jira:%s) — 제목에는 일의 내용만", m, m))
	}
	return is
}

// CheckNote validates the note body attached at capture time.
func CheckNote(s string) []Issue {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var is []Issue
	hard := func(msg string) { is = append(is, Issue{Field: FieldNote, Hard: true, Msg: msg}) }
	soft := func(msg string) { is = append(is, Issue{Field: FieldNote, Msg: msg}) }

	if reHeading.MatchString(s) {
		hard("note 에 문서 구조(##)를 만들지 않습니다 — 배경·계획·수용기준은 저장소 문서에 두고, 여기엔 사실 2~4줄")
	}
	if reCheckbox.MatchString(s) {
		hard("체크리스트는 태스크 쪼개기로 — 항목 하나가 태스크 하나")
	}
	if len(reNumbered.FindAllString(s, -1)) >= 3 {
		hard("단계별 계획은 note 가 아닙니다 — 지금 아는 사실만 남기고 순서는 태스크로")
	}
	if reBold.MatchString(s) {
		soft("강조 없이 평문으로")
	}
	if n, lines := len([]rune(s)), strings.Count(s, "\n")+1; n > noteSoftRunes || lines > noteSoftLines {
		soft(fmt.Sprintf("note 가 깁니다 (%d자 %d줄) — 태스크 메모는 2~4줄이면 충분합니다", n, lines))
	}
	if !hasFact(s) {
		soft("언제·얼마인지 한 가지는 남기면 3주 뒤에 쓸모가 있습니다 (날짜·수치·사람)")
	}
	if rePlan.MatchString(s) {
		soft("앞으로 할 일이 아니라 지금 아는 사실을 적습니다 — 할 일은 태스크로")
	}
	return is
}

// CheckReason validates a hold reason. The service already refuses an empty
// one; what is left is the reason that exists but explains nothing.
func CheckReason(s string) []Issue {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	vague := len([]rune(s)) < reasonVagueLimit && !reDigit.MatchString(s) && !reLatin.MatchString(s)
	if vague {
		return []Issue{{Field: FieldReason, Msg: "누구·무엇을 기다리는지 적습니다 — 상대와 기한이 없는 보류는 좀비가 됩니다"}}
	}
	if !hasFact(s) && !containsAny(s, deadlineWords) {
		return []Issue{{Field: FieldReason, Msg: "언제까지인지 붙이면 언제 쪼아야 할지 알 수 있습니다"}}
	}
	return nil
}

// CheckNoteLine validates a single appended note - one sentence, one fact.
func CheckNoteLine(s string) []Issue {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var is []Issue
	soft := func(msg string) { is = append(is, Issue{Field: FieldNoteLine, Msg: msg}) }

	if strings.Contains(s, "\n") {
		soft("한 번에 한 문장씩 쌓습니다 — 여러 건이면 task_note 를 나눠 부를 것")
	}
	if !hasFact(s) {
		soft("그때 안 사실을 숫자·날짜와 함께 적습니다 (측정값, 상대가 말한 날짜)")
	}
	if rePlan.MatchString(s) {
		soft("계획은 메모가 아니라 태스크로 — 지금 안 할 일이면 적지 않습니다")
	}
	return is
}

// Err folds the hard issues into one error, or nil when there are none.
func Err(is []Issue) error {
	var msgs []string
	for _, i := range is {
		if i.Hard {
			msgs = append(msgs, i.Msg)
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return fmt.Errorf("%s — 규약: %s", strings.Join(msgs, "; "), ConventionsURI)
}

// Warnings returns the soft messages, prefixed with the field they belong to
// so a caller mutating several fields can tell them apart.
func Warnings(is []Issue) []string {
	var out []string
	for _, i := range is {
		if !i.Hard {
			out = append(out, i.Field+": "+i.Msg)
		}
	}
	return out
}

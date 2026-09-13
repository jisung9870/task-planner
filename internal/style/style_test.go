package style

import (
	"strings"
	"testing"
)

// The fixtures are the 나쁨/중간/좋음 examples the conventions document is
// written from. The 좋음 rows matter as much as the 나쁨 ones: a warning that
// fires on a well-written line teaches the caller to ignore every warning.

type styleCase struct {
	name string
	in   string
	hard int
	// soft lists a fragment of each expected warning. The count must match, so
	// a new rule that fires on an existing fixture shows up as a failure.
	soft []string
}

func run(t *testing.T, check func(string) []Issue, cases []styleCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			is := check(c.in)
			var hard int
			var soft []string
			for _, i := range is {
				if i.Hard {
					hard++
				} else {
					soft = append(soft, i.Msg)
				}
			}
			if hard != c.hard {
				t.Errorf("hard %d, want %d: %v", hard, c.hard, is)
			}
			if len(soft) != len(c.soft) {
				t.Fatalf("soft %d개, want %d: %v", len(soft), len(c.soft), soft)
			}
			for _, want := range c.soft {
				found := false
				for _, got := range soft {
					if strings.Contains(got, want) {
						found = true
					}
				}
				if !found {
					t.Errorf("경고에 %q 가 없음: %v", want, soft)
				}
			}
		})
	}
}

func TestCheckTitle(t *testing.T) {
	run(t, CheckTitle, []styleCase{
		{name: "빈 제목은 service 가 거부한다", in: "   "},

		{name: "나쁨: 계획 문장", in: "k8s 클러스터 노드 풀 업그레이드를 위한 사전 영향도 분석 및 롤백 계획 수립",
			soft: []string{"제목이 깁니다", "한 태스크는 한 일"}},
		{name: "나쁨: 동사 종결", in: "사용자가 요청한 대로 task_add 에 중복 생성 방지 로직을 추가한다",
			soft: []string{"명사구"}},
		{name: "나쁨: 대괄호 접두", in: "[task-planner] MCP 서버 관련 이슈 대응",
			soft: []string{"대괄호"}},
		{name: "나쁨: 두 일을 묶음", in: "이번 주 팀 주간 업무 보고서 작성 및 공유 진행",
			soft: []string{"한 태스크는 한 일"}},
		{name: "나쁨: 티켓 번호", in: "Jira ABC-1234 이슈 처리 (긴급)",
			soft: []string{"links"}},
		{name: "나쁨: 문단", in: strings.Repeat("아주 긴 제목 ", 10), hard: 1},

		// 중간: 규칙은 지켰고 남은 문제는 고유명사·범위다. 기계가 잡을 수 없는
		// 것이라 경고하지 않는다 - 그 몫은 규약 문서에 있다.
		{name: "중간: 고유명사가 빠짐", in: "노드 풀 업그레이드 사전 점검"},
		{name: "중간: 해결책을 제목에", in: "task_add 중복 생성 방지 로직 추가"},
		{name: "중간: 끝을 알 수 없음", in: "GCP 비용 검토"},

		{name: "좋음: 대상이 분명함", in: "game-prod 노드 풀 1.29 업그레이드 점검"},
		{name: "좋음: 증상 그대로", in: "task_add 동시 호출 때 중복 생성"},
		{name: "좋음: 끝이 보임", in: "GCP 9월 청구서 급증분 원인 확인"},
		{name: "좋음: 반복은 recur 가 말한다", in: "주간보고"},
	})
}

const (
	noteDoc = `## Context
현재 task-planner MCP 서버는 task_add 호출 시 중복 태스크가 생성될 수 있는 문제가 있습니다.
## Approach
1. Sync 타이밍 조사
2. 채번 로직 수정
3. 테스트 추가
## Acceptance Criteria
- [ ] 동시 호출 시 중복 없음
- [ ] 회귀 테스트 통과`

	noteMid = `task_add 동시 호출 시 중복 생성 이슈. Sync 이전에 id 채번이 수행되는 것이 원인으로 추정됨.
수정 후 회귀 테스트 추가가 필요함.`

	noteGood = `9/12 Codex 세션에서 같은 태스크가 두 번 생겼다.
begin() 의 Sync 보다 채번이 먼저 도는 것 같다. 재현은 아직 한 번뿐.`
)

func TestCheckNote(t *testing.T) {
	run(t, CheckNote, []styleCase{
		{name: "나쁨: 문서가 됨", in: noteDoc, hard: 3, soft: []string{"note 가 깁니다"}},
		{name: "나쁨: 회의록", in: "## 회의 내용 정리\n- 일시: 2026-09-12\n- 결론: 정산 주기는 매월 25일", hard: 1},

		{name: "중간: 문체가 보고서", in: noteMid, soft: []string{"언제·얼마인지", "지금 아는 사실"}},
		{name: "중간: 값만 남고 변화가 빠짐", in: "9/12 A팀과 협의하여 정산 주기를 매월 25일로 확정함. 관련 문서 업데이트 예정.",
			soft: []string{"지금 아는 사실"}},
		{name: "중간: 제목의 반복", in: "배포 스크립트 확인 필요",
			soft: []string{"언제·얼마인지"}},

		{name: "좋음: 사실과 불확실함", in: noteGood},
		{name: "좋음: 바뀐 지점을 적음", in: "9/12 A팀 통화. 정산이 월말인 줄 알았는데 25일이라고 한다.\n25일 기준으로 배치 시각 다시 봐야 한다."},
		{name: "좋음: 숫자 대신 시점", in: "어제 롤백 때 스크립트가 old 심볼릭 링크를 안 지웠다. 그것 때문인지 확인."},
	})
}

func TestCheckReason(t *testing.T) {
	run(t, CheckReason, []styleCase{
		{name: "나쁨: 상대가 없음", in: "외부 의존성으로 인한 대기", soft: []string{"누구·무엇을"}},
		{name: "중간: 기한이 없음", in: "A팀 회신 대기", soft: []string{"언제까지"}},
		{name: "좋음", in: "A팀 스펙 대기 — 9/12 요청, 18일까지 준다고 함"},
		{name: "좋음: 날짜 대신 시점", in: "보안팀 승인 대기 — 다음 주 월요일 지나면 핑"},
	})
}

func TestCheckNoteLine(t *testing.T) {
	run(t, CheckNoteLine, []styleCase{
		{name: "나쁨: 아무 사실도 없음", in: "진행 상황을 지속적으로 모니터링 중", soft: []string{"숫자·날짜"}},
		{name: "중간: 값이 빠짐", in: "모니터링 중, 아직 이상 없음", soft: []string{"숫자·날짜"}},
		{name: "나쁨: 계획", in: "향후 개선 방향에 대한 검토가 필요함",
			soft: []string{"숫자·날짜", "계획은 메모가 아니라"}},
		{name: "좋음: 측정값", in: "2시간 부하 돌림. p99 180ms, 에러 0"},
		{name: "좋음: 상대와 날짜", in: "B팀 담당자: 이번 스프린트엔 못 본다고. 10월 첫 주에 다시 물어보기로"},
	})
}

func TestErrPointsAtConventions(t *testing.T) {
	err := Err(CheckNote("## Context\n배경"))
	if err == nil {
		t.Fatal("헤딩이 통과함")
	}
	if !strings.Contains(err.Error(), ConventionsURI) {
		t.Errorf("거부 메시지가 규약을 가리키지 않음: %v", err)
	}
	if Err(CheckNote(noteGood)) != nil {
		t.Error("좋은 메모가 거부됨")
	}
}

func TestWarningsCarryField(t *testing.T) {
	w := Warnings(CheckTitle("주간 보고서 작성 및 공유"))
	if len(w) != 1 || !strings.HasPrefix(w[0], FieldTitle+": ") {
		t.Fatalf("필드 접두가 없음: %v", w)
	}
}

package tui

import "strings"

// helpLine is the persistent footer. The TUI has no menus, so this line is the
// entire discovery surface for common actions.
const helpLine = "a 추가  e 편집  space 상태  s 진행  d 완료  b 보류  D 기간  p 프로젝트  [ ] 이동  enter 상세  r 새로고침  1-5 탭  q 종료  ? 도움말"

// helpLines splits the help text once per frame; it is short enough that the
// allocation does not matter and a cached copy would drift.
func helpLines() []string { return strings.Split(strings.TrimRight(helpFull, "\n"), "\n") }

// helpFull is shown by ?.
const helpFull = `키

  이동        ↑/k  ↓/j   g 맨 위   G 맨 아래
              ←/h  →/l   (Board·Week 에서 열 이동)
  탭          1 Today   2 Week   3 Board   4 Projects   5 All
  기간        D  진행 기간 입력
                09-15~09-19   시작~마감
                today~+4d     오른쪽 상대값은 시작일 기준
                09-15         시작만 (마감은 그대로)
                ~09-19        마감만
                -             기간 해제
              [ / ]  기간을 통째로 하루 앞/뒤로 (길이 유지)
              { / }  마감만 하루 앞/뒤로 (기간 늘이기·줄이기)
              날짜가 없는 항목은 ] 를 누르면 오늘로 들어옵니다.
  Week        요일 7컬럼 + 미배정 lane.
              기간이 여러 날인 태스크는 걸쳐 있는 모든 요일에 나오고,
              둘째 날부터는 ╌ 와 "3/5일" 로 표시됩니다.
  추가        a  (한 줄 캡처 — 나머지 필드는 나중에)
  상태        space 순환   s 진행중   d 완료   b 보류   x 취소   u 대기중
  프로젝트    p  선택한 태스크의 프로젝트 지정 ( - 입력 시 해제)
  Projects    n  새 프로젝트 (slug [이름])
              s  상태 순환 active → paused → done
              e  project.md 편집 (없으면 만들고 엽니다)
              enter 드릴인 — 그 안에서 a 로 추가하면 그 프로젝트로 들어갑니다
  상세        넓은 화면: 우측 고정 패널 (enter 로 접기/펼치기)
              좁은 화면: enter 로 하단 패널 토글
  편집        e  (외부 편집기 — 저장하고 나오면 자동 반영)
  필터        /  질의식 또는 자유 단어
              status:doing  project:infra  tag:ops  priority:P1
              due<7d  scheduled:today  rollover>2  id:0012
              is:open|closed|overdue|duesoon|blocked|carried|unscheduled
              앞에 - 를 붙이면 부정 (-status:done)
  새로고침    r      전체 재인덱싱  R
  종료        q / ctrl+c

요약 줄의 "배정 3h30m/6h" 는 오늘 기간이 걸친 태스크들의 예상 소요 합입니다.
여러 날짜리 태스크는 기간으로 나눈 몫만 셉니다. 한도는 config.yaml 의
daily_capacity 이고, 추정치가 있는 태스크만 계산에 들어갑니다.

보류(b)는 사유 없이 저장되지 않습니다.
진행 기간은 scheduled(착수)~due(마감) 입니다 — 별도 필드가 아니라 이미 있던
두 날짜이고, 하나만 채우면 그 날 하루짜리 일이 됩니다.
Board 는 열린 작업만 보여줍니다. 완료·취소는 하단에 건수로만 나옵니다 —
완료 더미가 계속 쌓이면 아직 손이 필요한 열이 눌립니다.
↻N 은 이월 횟수입니다. 반복해서 이월되는 항목은 쪼개거나 버릴 때입니다.
데이터는 vault 의 markdown 파일이 원천이고, 인덱스는 언제 지워도 재생성됩니다.`

package tui

import "strings"

// helpLine is the persistent footer. The TUI has no menus, so this line is the
// entire discovery surface for common actions.
const helpLine = "a 추가  N 메모  space 상태  d 완료  D 기간  m 선택  v 뷰  ! 다음할일  / 검색  ctrl+z 되돌리기  1-5 탭  ? 도움말"

// helpLines splits the help text once per frame; it is short enough that the
// allocation does not matter and a cached copy would drift.
func helpLines() []string { return strings.Split(strings.TrimRight(helpFull, "\n"), "\n") }

// helpFull is shown by ?.
const helpFull = `키

  이동        ↑/k  ↓/j   g 맨 위   G 맨 아래
              ←/h  →/l   (Board·Week 에서 열 이동)
              H/L        선택한 카드를 옆 열로 (Board=상태, Week=날짜)
              J/K        상세 패널 스크롤
  되돌리기    ctrl+z  직전 동작을 되돌립니다 (최근 20개, 파일 단위로 복원)
              밖에서 파일이 바뀌었으면 덮어쓰지 않고 거부합니다
  선택        m  현재 항목 선택/해제   M  전체 해제   esc 도 해제
              선택이 있으면 상태·기간·프로젝트·메모·삭제 키가 선택 전체에 적용
  뷰          v  저장된 질의 목록 (1-9 로 즉시 적용)
              v → s  현재 필터를 이름 붙여 저장 (config.yaml 의 views)
  탭          1 Today   2 Week   3 Board   4 Projects   5 All
  추가        a  (한 줄 캡처 — 나머지 필드는 나중에)
  상태        space 순환   s 진행중   d 완료   b 보류   x 취소   u 대기중
              S 반복 회차 건너뛰기   X 삭제 (확인 후, ctrl+z 로 복구)
  기간        D  진행 기간 입력
                09-15~09-19   시작~마감
                today~+4d     오른쪽 상대값은 시작일 기준
                09-15         시작만 (마감은 그대로)
                ~09-19        마감만
                -             기간 해제
              [ / ]  기간을 통째로 하루 앞/뒤로 (길이 유지)
              { / }  마감만 하루 앞/뒤로 (기간 늘이기·줄이기)
              날짜가 없는 항목은 ] 를 누르면 오늘로 들어옵니다.
  메모        N  본문 ## Note 에 시각과 함께 한 줄 추가 (편집기 없이)
  다음 할 일  !  막힌 것 없는 가장 급한 태스크로 커서 이동 + 이유 표시
  프로젝트    p  선택한 태스크의 프로젝트 지정 ( - 입력 시 해제)
  Week        요일 7컬럼 + 미배정 lane.
              기간이 여러 날인 태스크는 걸쳐 있는 모든 요일에 나오고,
              둘째 날부터는 ╌ 와 "3/5일" 로 표시됩니다.
  Projects    n  새 프로젝트 (slug [이름])
              s  상태 순환 active → paused → done
              D  프로젝트 마감일 (진행률 옆에 D-n 으로 표시)
              e  project.md 편집 (없으면 만들고 엽니다)
              enter 드릴인 — 그 안에서 a 로 추가하면 그 프로젝트로 들어갑니다
  상세        넓은 화면: 우측 고정 패널 (enter 로 접기/펼치기)
              좁은 화면: enter 로 하단 패널 토글
  편집        e  (외부 편집기 — 저장하고 나오면 자동 반영)
  필터        /  질의식 또는 자유 단어 — 입력하는 대로 걸러집니다
              ↑/↓ 로 이전에 쓴 질의를 불러옵니다
              status:doing  project:infra  tag:ops  priority:P1
              due<7d  scheduled:today  rollover>2  id:0012
              is:open|closed|overdue|duesoon|blocked|carried|unscheduled
              body:타임아웃   메모·로그 본문 검색 (파일을 읽으므로 조금 느림)
              앞에 - 를 붙이면 부정 (-status:done)
  새로고침    r      전체 재인덱싱  R
  종료        q / ctrl+c

진행중인 태스크가 있으면 요약 줄 맨 앞에 ▶ 와 경과 시간이 계속 보입니다.
타이머가 도는 중에 q 를 누르면 한 번 더 확인을 받습니다.

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

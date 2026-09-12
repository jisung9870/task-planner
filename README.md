# task-planner (`tp`)

터미널에서 오늘·이번 주 할 일과 그 상태를 보고 바꾸되, 데이터는 사람과 AI가
직접 읽고 쓸 수 있는 markdown 파일로 남는 개인 업무 관리 TUI.

기획·설계 배경은 [docs/product-task-planner-202609.md](docs/product-task-planner-202609.md) 에 있다.

## 설치

```bash
make install        # 빌드 후 ~/.local/bin/tp 에 설치
tp init             # vault 생성 (기본 ~/tasks)
```

`sudo` 가 필요 없는 경로를 기본값으로 쓴다. 개인 도구를 쓰려고 관리자 권한을
요구하는 순간 설치 자체가 마찰이 되기 때문이다.

| 명령 | 하는 일 |
|---|---|
| `make install` | 빌드 후 설치. 이미 설치돼 있으면 알리고 멈춘다 |
| `make upgrade` | 이 체크아웃에서 다시 빌드해 교체. vault 는 건드리지 않는다 |
| `make uninstall` | 바이너리만 제거. **vault 데이터는 남긴다** |
| `make status` | 설치 위치·버전·PATH·vault 상태 |
| `make build` | 설치 없이 `./tp` 만 생성 |

설치 위치는 `TP_INSTALL_DIR`, `$PREFIX/bin`, `~/.local/bin` 순으로 결정한다.

```bash
TP_INSTALL_DIR=/usr/local/bin scripts/install.sh install   # 다른 경로
scripts/install.sh uninstall --purge                       # vault 까지 삭제(확인을 받는다)
```

`--purge` 는 되돌릴 수 없으므로 vault 경로를 그대로 다시 입력해야 진행된다.
비대화형 환경에서는 아예 거부한다.

vault 경로는 `--vault`, `$TP_VAULT`, 기본값 `~/tasks` 순으로 결정된다.

## 사용

```bash
tp                                   # TUI
tp add "주간보고 초안" -s today -e 30m # 빠른 캡처
tp today                             # 오늘 할 일
tp week                              # 이번 주
tp start 1 / tp done 1               # 상태 전이 (#1 또는 제목 일부로 지정)
tp set 1 --span 09-15~09-19 --priority P1   # 필드 수정 (로그에 남음)
tp note 1 "보안팀 회신 대기, 담당 김OO"       # 시각이 붙은 한 줄 메모
tp next                              # 지금 바로 할 수 있는 일 추천
tp block 1 "인프라팀 회신 대기"        # 보류 - 사유 필수
tp block 2 --by 1                     # 선행 태스크로 보류 (완료 시 자동 해제)
tp projects                          # 프로젝트별 현황
tp projects new infra "인프라 개편"   # 프로젝트 생성
tp projects set infra --due +2w      # 프로젝트 마감(마일스톤)·상태
tp list status:doing project:infra   # 질의
tp list 'due<7d' -status:done
tp list is:overdue
tp add "주간보고" --recur weekly      # 반복 (완료 시 다음 회차 자동 생성)
tp skip 3                            # 이번 회차만 건너뛰기
tp time --week                       # 예상 대비 실소요
tp archive --dry-run                 # 오래된 완료분 정리 (기본 30일)
tp index --rebuild                   # 인덱스 재생성
tp version                           # 버전·커밋·빌드 시각
```

TUI 탭: `1` Today · `2` Week(요일 그리드) · `3` Board(칸반) · `4` Timeline(간트) ·
`5` Projects · `6` All
TUI 키: `a` 추가 · `A` 폼 캡처(제목·설명·기간·태그) · `N` 메모 · `space` 상태 순환 · `s/d/b/x` 진행/완료/보류/취소 ·
`D` 진행 기간 · `p` 프로젝트 지정 · `m` 선택 · `v` 저장된 뷰 · `!` 다음 할 일 ·
`ctrl+z` 되돌리기 · `S` 반복 건너뛰기 · `X` 삭제 · `enter` 상세 패널 ·
`J/K` 상세 스크롤 · `e` 편집기 · `/` 필터 · `h/l` 열 이동 · `H/L` 카드 이동 ·
`[`/`]` 기간 이동 · `{`/`}` 기간 늘이기/줄이기 · `?` 전체 도움말.
Week·Timeline 에서는 `,`/`.` 가 창을 한 주씩 옮기고 `t` 가 이번 주로 돌아온다
(Timeline 은 `h`/`l` 도 같다).
마우스 클릭·휠도 동작하고, `NO_COLOR=1` 이면 색 없이 그린다.

화면 상단에 요약 한 줄(오늘마감·마감초과·WIP·보류 경과·이월)이 뜬다. 폭 100자
이상이면 목록 우측에 상세 패널이 고정되고 커서를 따라온다.

### 진행 기간

`D` 로 기간을 입력한다. 별도 필드가 아니라 `scheduled`(시작)과 `due`(마감)이며,
하나만 채우면 그 날 하루짜리 일이 된다. 시작과 마감이 **같은 날**이어도 기간이고,
`09-14 하루` 로 표시된다 — 하루짜리도 "언제부터 언제까지"의 약속이다.

```
09-15~09-19   시작~마감        09-15    시작만 (마감은 그대로)
today~+4d     오른쪽 상대값은  ~09-19   마감만
              시작일 기준      -        해제
```

Week 탭은 요일 7컬럼 그리드다. 기간이 여러 날인 태스크는 걸쳐 있는 모든 요일에
나오고 둘째 날부터 `╌` 와 `3/5일` 로 표시된다. `,`/`.` 로 지난 주·다음 주를 보고
`t` 로 돌아온다 — 이번 주가 아닌 동안에는 진행중·마감초과가 따라오지 않는다.
그 주의 계획을 보는 화면이지 오늘의 잔업 목록이 아니기 때문이다.

Timeline 탭은 같은 기간을 간트로 그린다 — 한 줄에 태스크 하나, 가로축이 날짜다.
창 길이는 터미널 폭에 맞춰 1~4주로 정해지고, `,`/`.`(또는 `h`/`l`) 로 한 주씩
옮기며 `t` 로 이번 주에 돌아온다. `◀`/`▶` 는 기간이 창 밖으로 이어진다는 뜻이고 `▼`·`┊` 가
오늘이다. 커서·선택·상태·기간 키는 목록 탭과 똑같이 듣는다. 날짜가 없는 태스크는
그릴 자리가 없으므로 맨 아래에 건수로만 알린다. `[`/`]` 는 기간을 통째로 하루씩
옮기고(길이 유지), `{`/`}` 는 마감만 옮겨 기간을 늘이거나 줄인다. 기간 중인
태스크는 이월되지 않는다 — 아직 늦은 게 아니라 진행 중이기 때문이다.

### 되돌리기와 선택

`ctrl+z` 는 직전 동작을 되돌린다. 파일 단위로 복원하므로 상태·완료일·로그 줄이
함께 돌아오고, 한 동작이 여러 파일을 건드렸어도(완료 → 후행 보류 해제 → 다음
회차 생성) 한 단계로 취급한다. 최근 20개까지 쌓이며 **세션 안에서만** 유효하다.
되돌릴 파일이 밖에서 바뀌었으면 덮어쓰지 않고 거부한다.

`m` 으로 여러 건을 고르면 상태·기간·프로젝트·메모·삭제 키가 **선택 전체**에
적용되고, 그 전체가 되돌리기 한 단계가 된다. `M` 또는 `esc` 로 해제.

### 저장된 뷰

`/` 로 만든 질의는 `v` → `s` 로 이름을 붙여 저장하면 `config.yaml` 의 `views:`
에 남는다. 이후 `v` 를 누르고 번호 하나로 불러온다. 필터 입력 중에는 `↑`/`↓` 로
이전에 쓴 질의를 꺼낼 수 있고, 입력하는 대로 결과가 걸러진다(`body:` 만 enter
까지 기다린다 — 파일을 읽기 때문이다).

```yaml
views:
  - name: 진행중
    query: status:doing
  - name: 반복 이월
    query: is:carried rollover>2
```

### 하루 용량

요약 줄의 `배정 3h30m/6h` 는 오늘 기간이 걸친 열린 태스크들의 예상 소요 합이다.
여러 날짜리 태스크는 **기간으로 나눈 몫**만 센다 — 2주짜리 일을 매일 2주로
계산하면 숫자가 없는 것만 못하다. 한도는 `config.yaml` 의 `daily_capacity`
(기본 6h)이고, 넘으면 빨갛게 표시된다. Week 그리드는 요일 밑줄에 그 날의 배정
시간을 단다.

추정치가 없는 태스크는 건수만 세고 합계에는 못 들어간다. `3h30m (2/5)` 처럼
몇 건이 추정됐는지 같이 나오는 이유다 — 추정 없는 날은 한가한 날이 아니라
모르는 날이다.

### 프로젝트

Projects 탭에서 `n` 으로 프로젝트를 만들고(`slug [이름]`), `s` 로 상태를
순환하며(active → paused → done), `D` 로 프로젝트 마감일을, `e` 로
`project.md` 를 연다. 행에는 진행률 막대(취소는 양쪽에서 제외), 남은 예상
시간, 마감까지 남은 날이 함께 나온다. 태스크 쪽은 `p` 로 프로젝트를 지정한다.
프로젝트 행에 커서를 둔 채 `a` 로 캡처하면 그 프로젝트로 들어간다.

### 다음 할 일

`!`(TUI) 또는 `tp next` 는 **선행이 남지 않은** 열린 태스크를 급한 순으로
보여주고, 왜 그게 먼저인지 한 줄로 말한다. 보류는 제외된다 — 선행이 끝나면
자동으로 대기중으로 돌아오므로, 아직 보류인 항목은 이 도구가 모르는 무언가를
기다리는 중이다.

## 질의 문법

CLI(`tp list`)와 TUI(`/`)가 같은 문법을 쓴다.

| | |
|---|---|
| 필드 | `status` `project` `tag` `priority` `due` `scheduled` `rollover` `is` `id` `body` |
| 연산 | `:` 같음, `<` `<=` `>` `>=` 비교(날짜·숫자) |
| 날짜 | `2026-09-15` `today` `tomorrow` `+7d` `2w` `1m` `week` `none` `any` |
| `is:` | `open` `closed` `overdue` `duesoon` `blocked` `carried` `unscheduled` `recurring` |
| 부정 | 앞에 `-` 또는 `!` (`-status:done`) |
| 자유어 | 제목·프로젝트·태그 부분일치. `"따옴표"` 로 구 묶기 |
| `body:` | 메모·로그 본문 검색. 인덱스에 본문이 없어 파일을 읽으므로, 다른 조건으로 먼저 좁히면 빠르다 |

조건은 모두 AND 로 묶인다.

## 아카이브

`tp archive` 는 끝난 지 오래된(기본 30일) 완료·취소 태스크를
`archive/<연도>-Q<분기>/` 로 옮긴다. 인덱스는 `tasks/` 아래를 전부 훑으므로
끝난 일이 쌓이면 모든 명령이 조금씩 느려진다. 옮기는 것뿐이라 markdown 은
그대로이고, 되돌리려면 파일을 `tasks/` 로 다시 옮기고 `tp index --rebuild`
하면 된다. 옮겨진 태스크는 조회·리포트 대상에서 빠지므로 먼저 `--dry-run`
으로 확인한다.

## 데이터

```
~/tasks/
├── config.yaml           # 설정
├── tasks/2026-09/*.md    # 태스크 1개 = 파일 1개 (원천)
├── projects/<slug>/project.md
├── archive/2026-Q3/*.md  # tp archive 로 옮겨진 완료분
├── reports/
└── .index/               # 파생물 - 지워도 markdown 에서 재생성
```

태스크 파일은 YAML frontmatter + markdown 본문이다. `scheduled`(착수 예정일)와
`due`(마감일)는 다른 필드이며, 이 구분이 Today 뷰가 쓸모 있는 이유다. 둘을
합치면 진행 기간이 되고, Week 그리드는 그 기간을 요일에 걸쳐 그린다.
`status: blocked` 는 `blocked_reason` 또는 `blocked_by` 없이는 저장되지 않는다.
`completed` 는 완료·취소 **둘 다**의 종료일이다 — "그 날 끝난 일"을 묻는 뷰가
전부 이 필드 하나를 읽는다.

진행중인 태스크는 요약 줄 맨 앞에 `▶` 와 경과 시간으로 계속 보이고, 타이머가
도는 중에 `q` 를 누르면 한 번 더 확인한다.

진행중으로 바꾸면 타이머가 돌고, 벗어날 때 `actual` 에 누적된다. 한 세션은
`session_cap`(기본 8h)으로 제한된다 — 밤새 켜둔 태스크가 예상 대비 실소요
데이터를 망치지 않게 하기 위해서다.

## WIP 한도

`config.yaml` 의 `wip_limit`(기본 3) 을 넘겨 진행중으로 바꾸면 경고한다. **막지는
않는다** — 한도는 권한 체계가 아니라 주의력에 대한 신호이고, 실제로 하고 있는 일을
기록하길 거부하는 도구는 그냥 안 쓰게 된다. 경고에는 이미 진행중인 태스크 번호가
같이 나오므로 무엇을 먼저 끝낼지 바로 고를 수 있다. `0` 으로 두면 비활성.

## 반복 규칙 (`recur`)

`daily` `weekly` `monthly` `weekdays` `every 3 days` `every 2 weeks`
`every monday` `mon,thu` `monthly on 15` (한국어 `매일` `매주` `평일` `매월 15일` 도 가능)

완료하면 다음 회차가 **새 파일**로 생긴다. 회차마다 파일이 따로 남아야 주간
리포트가 "그 주에 실제로 무슨 일이 있었는지" 말할 수 있다. 취소하면 시리즈가
끝나고, 이번 회차만 건너뛰려면 `tp skip` 을 쓴다.

## MCP 서버 모드

Claude Code 같은 MCP 클라이언트가 vault 를 직접 읽고 조작하게 한다. stdio
transport 라서 **데몬이 없다** — 클라이언트가 `tp mcp` 를 자식 프로세스로 띄우고
세션이 끝나면 함께 종료된다.

```bash
claude mcp add task-planner -- tp mcp             # 현재 프로젝트
claude mcp add -s user task-planner -- tp mcp     # 모든 프로젝트
```

| 종류 | 도구 |
|---|---|
| 읽기 | `task_today` `task_week` `task_query` `task_get` `task_next` `day_load` `summary` `project_status` `time_summary` `report_week` |
| 쓰기 | `task_add` `task_status` `task_skip` `task_edit` `task_note` `project_create` `project_set` `rollover` `archive` |

모든 도구가 TUI·CLI 와 같은 service 계층을 통과하므로 도메인 규칙(보류 사유
강제, 전이 로그, 반복 회차 생성, 후행 자동 해제, WIP 경고)이 그대로 지켜진다.
**삭제 도구는 없다** — 접을 일은 `cancelled` 로 남기고, 파일 삭제는 사람이
CLI 에서 한다. git auto_commit 이 켜져 있으면 쓰기 도구 호출마다 커밋된다.

## git 동기화

vault 를 git 저장소로 두면 이력·백업·기기 간 동기화가 한 번에 해결된다.

```bash
tp git init        # vault 를 저장소로 만들고 .index/ 를 무시 목록에 추가
tp git status      # 설정 확인
tp git sync -m "메시지"   # 즉시 커밋
```

`config.yaml` 에서 `git.auto_commit: true` 로 두면 **세션마다 한 번** 커밋한다
(CLI 명령 1회 = 커밋 1개, TUI 종료 시 그 세션의 변경을 묶어 커밋 1개).
`git.auto_push: true` 면 커밋 후 push 한다.

## 개발

```bash
make test   # go test ./...
make vet
make scan   # 의존성 취약점·시크릿 스캔 (trivy)
make help   # 전체 타깃 목록
```

`make build` 는 `git describe` 로 버전을, `git rev-parse` 로 커밋을 바이너리에
박아 넣는다. 태그가 없으면 커밋 해시를 쓴다 — 태그가 없다고 `v0.0.0` 같은 값을
지어내면 나중에 어느 코드가 설치돼 있는지 알 수 없다.

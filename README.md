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
tp block 1 "인프라팀 회신 대기"        # 보류 - 사유 필수
tp block 2 --by 1                     # 선행 태스크로 보류 (완료 시 자동 해제)
tp projects                          # 프로젝트별 현황
tp list status:doing project:infra   # 질의
tp list 'due<7d' -status:done
tp list is:overdue
tp add "주간보고" --recur weekly      # 반복 (완료 시 다음 회차 자동 생성)
tp skip 3                            # 이번 회차만 건너뛰기
tp time --week                       # 예상 대비 실소요
tp index --rebuild                   # 인덱스 재생성
tp version                           # 버전·커밋·빌드 시각
```

TUI 탭: `1` Today · `2` Week(요일 그리드) · `3` Board(칸반) · `4` Projects · `5` All
TUI 키: `a` 추가 · `space` 상태 순환 · `s/d/b/x` 진행/완료/보류/취소 ·
`enter` 상세 패널 · `e` 편집기 · `/` 필터 · `h/l` 열 이동 · `[`/`]` 예정일 이동 ·
`?` 전체 도움말.

화면 상단에 요약 한 줄(오늘마감·마감초과·WIP·보류 경과·이월)이 뜬다. 폭 100자
이상이면 목록 우측에 상세 패널이 고정되고 커서를 따라온다. Week 탭은 요일
7컬럼 그리드이며 `[`/`]` 로 선택 태스크의 예정일을 하루씩 옮긴다 — 이월을
손으로 계획하는 조작이다.

## 질의 문법

CLI(`tp list`)와 TUI(`/`)가 같은 문법을 쓴다.

| | |
|---|---|
| 필드 | `status` `project` `tag` `priority` `due` `scheduled` `rollover` `is` `id` |
| 연산 | `:` 같음, `<` `<=` `>` `>=` 비교(날짜·숫자) |
| 날짜 | `2026-09-15` `today` `tomorrow` `+7d` `2w` `1m` `week` `none` `any` |
| `is:` | `open` `closed` `overdue` `duesoon` `blocked` `carried` `unscheduled` `recurring` |
| 부정 | 앞에 `-` 또는 `!` (`-status:done`) |
| 자유어 | 제목·프로젝트·태그 부분일치. `"따옴표"` 로 구 묶기 |

조건은 모두 AND 로 묶인다.

## 데이터

```
~/tasks/
├── config.yaml           # 설정
├── tasks/2026-09/*.md    # 태스크 1개 = 파일 1개 (원천)
├── projects/<slug>/project.md
├── archive/, reports/
└── .index/               # 파생물 - 지워도 markdown 에서 재생성
```

태스크 파일은 YAML frontmatter + markdown 본문이다. `scheduled`(착수 예정일)와
`due`(마감일)는 다른 필드이며, 이 구분이 Today 뷰가 쓸모 있는 이유다.
`status: blocked` 는 `blocked_reason` 또는 `blocked_by` 없이는 저장되지 않는다.

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

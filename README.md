# task-planner (`tp`)

터미널에서 오늘·이번 주 할 일과 그 상태를 보고 바꾸되, 데이터는 사람과 AI가
직접 읽고 쓸 수 있는 markdown 파일로 남는 개인 업무 관리 TUI.

기획·설계 배경은 [docs/product-task-planner-202609.md](docs/product-task-planner-202609.md) 에 있다.

## 설치

```bash
make build          # ./tp 생성
./tp init           # vault 생성 (기본 ~/tasks)
```

vault 경로는 `--vault`, `$TP_VAULT`, 기본값 `~/tasks` 순으로 결정된다.

## 사용

```bash
tp                                   # TUI
tp add "주간보고 초안" -s today -e 30m # 빠른 캡처
tp today                             # 오늘 할 일
tp week                              # 이번 주
tp start 1 / tp done 1               # 상태 전이 (#1 또는 제목 일부로 지정)
tp block 1 "인프라팀 회신 대기"        # 보류 - 사유 필수
tp projects                          # 프로젝트별 현황
tp index --rebuild                   # 인덱스 재생성
```

TUI 키: `a` 추가 · `space` 상태 순환 · `s/d/b/x` 진행/완료/보류/취소 ·
`enter` 상세 · `/` 검색 · `1`~`4` 탭 · `?` 전체 도움말.

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
```

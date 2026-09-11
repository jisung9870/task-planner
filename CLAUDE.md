# task-planner 작업 규약

"왜 이렇게 만드는가"는 `docs/product-task-planner-202609.md` 에 있다. 이 파일은
"어떻게 작업하는가"만 다룬다. 결정이 바뀌면 기획서를 먼저 고친다.

## 계층 규칙 (이것만 지키면 나머지는 유연하다)

```
cmd/tp → internal/cli, internal/tui → internal/service → internal/query → internal/domain
                                            ↓
                                   internal/store, internal/index
```

1. **어댑터(`cli`, `tui`, 향후 `server`/`mcp`)는 `service` 만 호출한다.** 파일이나
   인덱스를 직접 건드리면 API·MCP 확장 시 같은 로직을 두 번 구현하게 된다.
2. **`domain` 은 외부 패키지를 import 하지 않는다.** 표준 라이브러리만.
3. **`index` 는 파생물이다.** `.index/` 를 지워도 markdown 에서 완전히 재생성되어야
   한다. 인덱스에만 존재하는 필드를 만들지 않는다.
4. **상태 변경은 `Task.Transition` 으로만 한다.** `t.Status = ...` 직접 대입 금지 —
   로그·완료일·보류 메타가 함께 갱신되지 않는다.

## 빌드·테스트

```bash
make build   # go build -o tp ./cmd/tp
make test    # go test ./...
make vet
```

TUI 수동 확인은 tmux 로 한다.

```bash
tmux new-session -d -s tp -x 110 -y 30 "TP_VAULT=/tmp/tpvault $PWD/tp"
tmux capture-pane -t tp -p
```

## 알려진 함정

- `%-24s` 는 바이트 폭이라 한글에서 어긋난다. 정렬은 `pad()`(runewidth) 를 쓴다.
- `goccy/go-yaml` 의 `,inline` 맵은 **모든** 키를 담는다. 파싱 직후 `pruneExtra`
  로 알려진 키를 지워야 저장 시 중복이 생기지 않는다.
- 날짜는 `domain.Date`(YYYY-MM-DD, UTC 고정)다. `time.Time` 을 그대로 쓰면 DST·
  타임존에서 하루씩 어긋난다.
- 파일명은 제목에서 만들어지므로 제목을 바꾸면 경로가 바뀐다. `SaveTask` 가 이전
  파일을 지우는 이유이며, 이 동작을 없애면 인덱스에 유령 항목이 생긴다.

## 커밋

기능 단위로 끊는다. 한 커밋에 여러 기능을 섞지 않는다.
`feat(scope):` / `fix(scope):` / `test(scope):` / `docs:` / `chore:` 를 쓴다.

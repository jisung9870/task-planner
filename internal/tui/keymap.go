package tui

// helpLine is the persistent footer. The TUI has no menus, so this line is the
// entire discovery surface for common actions.
const helpLine = "a 추가  e 편집  space 상태  s 진행  d 완료  b 보류  x 취소  enter 상세  r 새로고침  1-4 탭  q 종료  ? 도움말"

// helpFull is shown by ?.
const helpFull = `키

  이동        ↑/k  ↓/j   g 맨 위   G 맨 아래
  탭          1 Today   2 Week   3 Projects   4 All
  추가        a  (한 줄 캡처 — 나머지 필드는 나중에)
  상태        space 순환   s 진행중   d 완료   b 보류   x 취소   u 대기중
  상세        enter (다시 누르면 닫힘)
  편집        e  (외부 편집기)
  검색        /  (제목·프로젝트·태그)
  새로고침    r      전체 재인덱싱  R
  종료        q / ctrl+c

보류(b)는 사유 없이 저장되지 않습니다.
데이터는 vault 의 markdown 파일이 원천이고, 인덱스는 언제 지워도 재생성됩니다.`

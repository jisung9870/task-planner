#!/bin/sh
# task-planner 설치 관리자.
#
#   scripts/install.sh install      빌드 후 설치
#   scripts/install.sh upgrade      이 체크아웃에서 다시 빌드해 교체
#   scripts/install.sh uninstall    바이너리 제거 (vault 는 건드리지 않음)
#   scripts/install.sh status       설치 위치·버전 확인
#
# 설치 위치는 $TP_INSTALL_DIR, $PREFIX/bin, ~/.local/bin 순으로 결정한다.
# 기본값에 sudo 가 필요 없는 경로를 쓰는 이유는, 개인 도구를 쓰려고 관리자
# 권한을 요구하는 순간 설치 자체가 마찰이 되기 때문이다.
set -eu

BIN_NAME=tp
REPO_ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)

# ---------------------------------------------------------------- 출력 헬퍼

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
	C_OK=$(printf '\033[32m'); C_WARN=$(printf '\033[33m')
	C_ERR=$(printf '\033[31m'); C_DIM=$(printf '\033[2m'); C_OFF=$(printf '\033[0m')
else
	C_OK=''; C_WARN=''; C_ERR=''; C_DIM=''; C_OFF=''
fi

say()  { printf '%s\n' "$*"; }
ok()   { printf '%s✓%s %s\n' "$C_OK" "$C_OFF" "$*"; }
warn() { printf '%s!%s %s\n' "$C_WARN" "$C_OFF" "$*" >&2; }
die()  { printf '%s오류:%s %s\n' "$C_ERR" "$C_OFF" "$*" >&2; exit 1; }
dim()  { printf '%s%s%s\n' "$C_DIM" "$*" "$C_OFF"; }

usage() {
	cat <<'USAGE'
task-planner 설치 관리자

사용법:
  scripts/install.sh install [--force]     빌드 후 설치
  scripts/install.sh upgrade               다시 빌드해 교체
  scripts/install.sh uninstall [--purge]   바이너리 제거
  scripts/install.sh status                설치 상태 확인

옵션:
  --force     이미 설치돼 있어도 덮어쓴다 (install)
  --purge     vault 데이터까지 지운다 (uninstall, 확인을 받는다)
  --dir PATH  설치 위치 지정 ($TP_INSTALL_DIR 와 동일)

환경변수:
  TP_INSTALL_DIR   설치 위치 (기본: $PREFIX/bin 또는 ~/.local/bin)
  TP_VAULT         vault 위치 (기본: ~/tasks)
USAGE
}

# ---------------------------------------------------------------- 경로 결정

install_dir() {
	if [ -n "${TP_INSTALL_DIR:-}" ]; then printf '%s\n' "$TP_INSTALL_DIR"; return; fi
	if [ -n "${PREFIX:-}" ]; then printf '%s/bin\n' "$PREFIX"; return; fi
	printf '%s/.local/bin\n' "$HOME"
}

vault_dir() {
	if [ -n "${TP_VAULT:-}" ]; then printf '%s\n' "$TP_VAULT"; return; fi
	printf '%s/tasks\n' "$HOME"
}

# ---------------------------------------------------------------- 빌드

# build_version 은 태그가 있으면 태그를, 없으면 커밋 해시를 쓴다. 태그가 없다고
# v0.0.0 같은 값을 지어내면 나중에 어느 코드가 설치돼 있는지 알 수 없다.
build_version() {
	if git -C "$REPO_ROOT" rev-parse --git-dir >/dev/null 2>&1; then
		git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null && return
	fi
	printf 'dev\n'
}

build_commit() {
	git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || printf ''
}

build() {
	out=$1
	command -v go >/dev/null 2>&1 || die "go 가 설치되어 있지 않습니다 (https://go.dev/dl/)"
	version=$(build_version)
	commit=$(build_commit)
	built=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
	pkg=task-planner/internal/cli

	say "빌드 중... ($version)"
	( cd "$REPO_ROOT" && go build \
		-trimpath \
		-ldflags "-s -w -X ${pkg}.Version=${version} -X ${pkg}.Commit=${commit} -X ${pkg}.BuildDate=${built}" \
		-o "$out" ./cmd/tp ) || die "빌드 실패"
}

# ---------------------------------------------------------------- 설치 상태

target_path() { printf '%s/%s\n' "$(install_dir)" "$BIN_NAME"; }

# installed_version 은 바이너리에게 직접 물어본다. 파일이 존재한다는 것만으로는
# 그게 우리 바이너리인지 알 수 없고, 남의 파일을 지우거나 덮어쓰면 안 된다.
installed_version() {
	path=$1
	[ -x "$path" ] || return 1
	json=$("$path" version --json 2>/dev/null) || return 1
	case "$json" in
		*'"name":"task-planner"'*) ;;
		*) return 1 ;;
	esac
	printf '%s\n' "$json" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p'
}

# 원자적 교체. 실행 중인 바이너리를 덮어쓰면 Linux 에서 "text file busy" 가 나고,
# TUI 가 떠 있는 동안 파일이 반쯤 쓰인 상태가 될 수 있다. rename 은 안전하다.
place() {
	src=$1 dst=$2
	dir=$(dirname -- "$dst")
	mkdir -p -- "$dir" || die "$dir 를 만들 수 없습니다"
	[ -w "$dir" ] || die "$dir 에 쓸 권한이 없습니다 (TP_INSTALL_DIR 로 다른 경로를 지정하세요)"
	tmp="$dir/.$BIN_NAME.new.$$"
	cp -- "$src" "$tmp" || die "복사 실패"
	chmod 755 "$tmp"
	mv -f -- "$tmp" "$dst" || { rm -f -- "$tmp"; die "설치 실패"; }
}

# PATH 진단. 설치했는데 실행이 안 되는 상태로 끝나는 게 가장 흔한 실패다.
check_path() {
	dir=$1
	case ":$PATH:" in
		*":$dir:"*) ;;
		*)
			warn "$dir 가 PATH 에 없습니다. 셸 설정에 아래를 추가하세요:"
			say ""
			say "    export PATH=\"$dir:\$PATH\""
			say ""
			return
			;;
	esac
	resolved=$(command -v "$BIN_NAME" 2>/dev/null || true)
	if [ -n "$resolved" ] && [ "$resolved" != "$dir/$BIN_NAME" ]; then
		warn "PATH 에서 먼저 잡히는 $BIN_NAME 이 따로 있습니다: $resolved"
		warn "방금 설치한 것은 $dir/$BIN_NAME 입니다."
	fi
}

# ---------------------------------------------------------------- 명령

cmd_install() {
	force=${1:-}
	dir=$(install_dir)
	dst="$dir/$BIN_NAME"

	if current=$(installed_version "$dst"); then
		if [ "$force" != "--force" ]; then
			say "이미 설치되어 있습니다: $dst ($current)"
			say "다시 빌드해 교체하려면: scripts/install.sh upgrade"
			exit 0
		fi
	elif [ -e "$dst" ]; then
		[ "$force" = "--force" ] || die "$dst 에 task-planner 가 아닌 파일이 있습니다 (--force 로 덮어쓰기)"
	fi

	tmpdir=$(mktemp -d) || die "임시 디렉토리 생성 실패"
	trap 'rm -rf -- "$tmpdir"' EXIT INT TERM
	build "$tmpdir/$BIN_NAME"
	place "$tmpdir/$BIN_NAME" "$dst"

	ok "설치 완료: $dst ($("$dst" version --json | sed -n 's/.*"version":"\([^"]*\)".*/\1/p'))"
	check_path "$dir"

	vault=$(vault_dir)
	if [ -d "$vault/tasks" ]; then
		dim "vault: $vault (기존)"
	else
		say ""
		say "다음 단계:"
		say "  tp init        vault 생성 ($vault)"
		say "  tp             TUI 실행"
		say "  tp --help      명령 목록"
	fi
	dim "셸 자동완성: tp completion zsh --help"
}

cmd_upgrade() {
	dir=$(install_dir)
	dst="$dir/$BIN_NAME"

	before=$(installed_version "$dst") || {
		[ -e "$dst" ] && die "$dst 가 task-planner 바이너리가 아닙니다"
		die "설치된 바이너리가 없습니다: $dst (먼저 scripts/install.sh install)"
	}

	tmpdir=$(mktemp -d) || die "임시 디렉토리 생성 실패"
	trap 'rm -rf -- "$tmpdir"' EXIT INT TERM
	build "$tmpdir/$BIN_NAME"
	after=$("$tmpdir/$BIN_NAME" version --json | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')

	if [ "$before" = "$after" ]; then
		say "이미 최신입니다 ($before) — 재설치합니다"
	fi
	place "$tmpdir/$BIN_NAME" "$dst"
	ok "업그레이드 완료: $before → $after"
	dim "vault 데이터는 변경되지 않았습니다: $(vault_dir)"
	check_path "$dir"
}

cmd_uninstall() {
	purge=${1:-}
	dir=$(install_dir)
	dst="$dir/$BIN_NAME"

	if current=$(installed_version "$dst"); then
		rm -f -- "$dst" || die "$dst 를 지울 수 없습니다"
		ok "제거 완료: $dst ($current)"
	elif [ -e "$dst" ]; then
		die "$dst 는 task-planner 바이너리가 아닙니다 — 직접 확인 후 지우세요"
	else
		say "설치된 바이너리가 없습니다: $dst"
	fi

	vault=$(vault_dir)
	if [ ! -d "$vault" ]; then
		return
	fi
	if [ "$purge" != "--purge" ]; then
		dim "vault 는 그대로 둡니다: $vault"
		dim "데이터까지 지우려면: scripts/install.sh uninstall --purge"
		return
	fi

	# 데이터 삭제는 되돌릴 수 없다. 대화형 확인 없이는 절대 진행하지 않는다.
	count=$(find "$vault" -name '*.md' -type f 2>/dev/null | wc -l | tr -d ' ')
	say ""
	warn "vault 를 영구 삭제합니다: $vault (markdown 파일 ${count}개)"
	[ -t 0 ] || die "확인을 받을 수 없는 환경입니다 (대화형 터미널에서 실행하세요)"
	printf 'vault 경로를 그대로 입력하면 삭제합니다: '
	read -r answer
	[ "$answer" = "$vault" ] || die "입력이 일치하지 않아 취소했습니다"
	rm -rf -- "$vault"
	ok "vault 삭제 완료: $vault"
}

cmd_status() {
	dir=$(install_dir)
	dst="$dir/$BIN_NAME"
	say "설치 위치   $dst"
	if current=$(installed_version "$dst"); then
		say "설치 버전   $current"
	elif [ -e "$dst" ]; then
		say "설치 버전   (task-planner 가 아닌 파일)"
	else
		say "설치 버전   (미설치)"
	fi
	say "저장소 버전 $(build_version)"
	resolved=$(command -v "$BIN_NAME" 2>/dev/null || printf '(PATH 에 없음)')
	say "PATH 해석   $resolved"
	vault=$(vault_dir)
	if [ -d "$vault/tasks" ]; then
		n=$(find "$vault/tasks" -name '*.md' -type f 2>/dev/null | wc -l | tr -d ' ')
		say "vault       $vault (태스크 ${n}개)"
	else
		say "vault       $vault (미생성 — tp init)"
	fi
}

# ---------------------------------------------------------------- 인자 처리

action=''
opt=''
while [ $# -gt 0 ]; do
	case "$1" in
		install|upgrade|uninstall|status) action=$1 ;;
		--force|--purge) opt=$1 ;;
		--dir) [ $# -ge 2 ] || die "--dir 에 경로가 필요합니다"; TP_INSTALL_DIR=$2; export TP_INSTALL_DIR; shift ;;
		-h|--help|help) usage; exit 0 ;;
		*) die "알 수 없는 인자: $1 (--help)" ;;
	esac
	shift
done

case "${action:-}" in
	install)   cmd_install "$opt" ;;
	upgrade)   cmd_upgrade ;;
	uninstall) cmd_uninstall "$opt" ;;
	status)    cmd_status ;;
	*)         usage; exit 1 ;;
esac

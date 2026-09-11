BIN       := tp
PKG       := ./...
CLI_PKG   := task-planner/internal/cli
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null)
BUILDDATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS   := -s -w -X $(CLI_PKG).Version=$(VERSION) -X $(CLI_PKG).Commit=$(COMMIT) -X $(CLI_PKG).BuildDate=$(BUILDDATE)

.PHONY: build test fmt vet run clean tidy install upgrade uninstall status help

## build: ./tp 생성 (버전 정보 포함)
build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/tp

## install: 빌드 후 ~/.local/bin 에 설치 (TP_INSTALL_DIR 로 변경 가능)
install:
	@scripts/install.sh install

## upgrade: 이 체크아웃에서 다시 빌드해 설치된 바이너리를 교체
upgrade:
	@scripts/install.sh upgrade

## uninstall: 설치된 바이너리 제거 (vault 는 건드리지 않음)
uninstall:
	@scripts/install.sh uninstall

## status: 설치 위치·버전·vault 상태 확인
status:
	@scripts/install.sh status

test:
	go test $(PKG)

fmt:
	gofmt -w .

vet:
	go vet $(PKG)

run: build
	./$(BIN)

tidy:
	go mod tidy

clean:
	rm -f $(BIN)

## help: 이 목록
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

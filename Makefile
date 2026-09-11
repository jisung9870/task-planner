BIN := tp
PKG := ./...

.PHONY: build test fmt vet run clean tidy

build:
	go build -o $(BIN) ./cmd/tp

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

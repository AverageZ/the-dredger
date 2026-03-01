.PHONY: all build run test lint fmt tidy clean install-tools install-hooks

all: fmt lint test build

build:
	go build -o dredger ./cmd/dredger

run:
	go run ./cmd/dredger

test:
	go test ./... -v

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -f dredger

install-tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

install-hooks:
	cp .githooks/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit

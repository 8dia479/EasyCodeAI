.PHONY: deps build test vet check fmt

deps:
	go mod tidy

build: deps
	go build -trimpath -o easycode ./cmd/easycode

test: deps
	go test -race ./...

vet: deps
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

check: deps
	bash -n scripts/install-termux.sh
	go test -race ./...
	go vet ./...
	go build ./...

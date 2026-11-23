.PHONY: build test vet check fmt

build:
	go build -trimpath -o easycode ./cmd/easycode

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

check:
	go test -race ./...
	go vet ./...
	go build ./...

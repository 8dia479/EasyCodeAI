.PHONY: deps build build-android-armv7 test vet check fmt

deps:
	go mod tidy

build: deps
	go build -trimpath -o easycode ./cmd/easycode

build-android-armv7: deps
	bash scripts/build-android-armv7.sh

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

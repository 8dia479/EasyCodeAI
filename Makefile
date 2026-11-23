.PHONY: deps build build-android-armv7 test vet check fmt

deps:
	go mod tidy

build: deps
	go build -trimpath -o easycode ./cmd/easycode

build-android-armv7: deps
	mkdir -p dist
	GOOS=android GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/easycode-android-armv7 ./cmd/easycode

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

BINARY_NAME=trokky
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X github.com/trokky/cli/cmd.version=$(VERSION)"

.PHONY: build install clean test

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) .

install:
	go install $(LDFLAGS) .

clean:
	rm -rf bin/

test:
	go test ./...

# Cross-compile for release
release-dry:
	goreleaser release --snapshot --clean

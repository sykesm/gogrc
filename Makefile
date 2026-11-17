VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOFLAGS := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test lint cover dist clean

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o gogrc .

test:
	go vet ./...
	go test ./...

lint:
	go vet ./...
	gopls check -severity=hint $$(git ls-files '*.go')
	golangci-lint run ./...

# Coverage of unit tests and of the gogrc binaries the end-to-end tests run.
COVERDIR := $(CURDIR)/.cover
cover:
	rm -rf $(COVERDIR) && mkdir -p $(COVERDIR)
	GOGRC_COVERDIR=$(COVERDIR) go test -cover -coverpkg=./... ./... -args -test.gocoverdir=$(COVERDIR)
	go tool covdata percent -i $(COVERDIR)
	go tool covdata textfmt -i $(COVERDIR) -o cover.out
	go tool cover -func=cover.out | tail -1

# Release archives for every platform in dist/, built as the release
# workflow builds them. Needs goreleaser.
dist:
	goreleaser release --snapshot --clean

clean:
	rm -rf gogrc dist .cover cover.out

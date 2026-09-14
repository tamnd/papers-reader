GO ?= go
BIN ?= bin/papers
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/tamnd/papers-reader.Version=$(VERSION)

.PHONY: all
all: check build

.PHONY: build
build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/papers

.PHONY: install
install:
	$(GO) install -trimpath -ldflags '$(LDFLAGS)' ./cmd/papers

.PHONY: test
test:
	$(GO) test ./...

.PHONY: race
race:
	$(GO) test -race ./...

.PHONY: cover
cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: check
check:
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || (echo "run make fmt" && false)
	$(GO) vet ./...
	$(GO) test ./...

.PHONY: audit
audit: build
	$(BIN) audit

# Builds the site and validates it without writing anything, which is what
# CI wants: the question is whether the corpus can produce a site, not
# whether this machine has one lying around.
.PHONY: emit
emit: build
	$(BIN) emit -check

.PHONY: clean
clean:
	rm -rf bin dist coverage.out

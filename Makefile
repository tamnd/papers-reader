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

# The TypeScript the app reads the build with, generated from the same
# schema the Go emitter validates against. Committed, so the app builds
# without a generator, and checked in CI, so nobody can hand edit an
# interface out of agreement with the emitter.
.PHONY: schema
schema:
	cd web && npm run schema

# Builds the site into web/public and then builds the app over it. The
# corpus is an argument because the app repository does not hold one.
.PHONY: site
site: build
	$(BIN) emit -corpus $(CORPUS) -out web/public
	cd web && npm run build

.PHONY: clean
clean:
	rm -rf bin dist coverage.out web/dist

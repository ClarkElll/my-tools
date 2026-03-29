GO ?= go
TOOL ?=
EXAMPLE_ARGS ?= -name Clark
GOCACHE ?= $(CURDIR)/.cache/go-build
GOMODCACHE ?= $(CURDIR)/.cache/gomod
BIN_DIR ?= $(CURDIR)/bin
REMOTE_WRITE_BENCH_BIN ?= $(BIN_DIR)/remote-write-bench
IMAGE ?= my-tools/remote-write-bench
TAG ?= latest
PLATFORM ?= linux/amd64
REMOTE_WRITE_BENCH_GOOS ?= $(shell $(GO) env GOOS)
REMOTE_WRITE_BENCH_GOARCH ?= $(shell $(GO) env GOARCH)
REMOTE_WRITE_BENCH_IMAGE_OS ?= $(word 1,$(subst /, ,$(PLATFORM)))
REMOTE_WRITE_BENCH_IMAGE_ARCH ?= $(word 2,$(subst /, ,$(PLATFORM)))

export GOCACHE
export GOMODCACHE

.PHONY: fmt test build build-remote-write-bench docker-build-remote-write-bench run-example new-tool

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

build:
	$(GO) build ./...

build-remote-write-bench:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(REMOTE_WRITE_BENCH_GOOS) GOARCH=$(REMOTE_WRITE_BENCH_GOARCH) $(GO) build -o $(REMOTE_WRITE_BENCH_BIN) ./app/remote-write-bench

docker-build-remote-write-bench:
	$(MAKE) build-remote-write-bench REMOTE_WRITE_BENCH_GOOS=$(REMOTE_WRITE_BENCH_IMAGE_OS) REMOTE_WRITE_BENCH_GOARCH=$(REMOTE_WRITE_BENCH_IMAGE_ARCH)
	docker build --platform $(PLATFORM) -f Dockerfile.remote-write-bench -t $(IMAGE):$(TAG) .

run-example:
	$(GO) run ./app/example $(EXAMPLE_ARGS)

new-tool:
	./scripts/new-tool.sh $(TOOL)

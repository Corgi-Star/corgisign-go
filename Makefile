# CorgiSign Go SDK + CLI build targets.

BIN     ?= bin/corgisign
PKG     := ./cmd/corgisign
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build cli install test vet fmt clean

## build: compile the corgisign CLI to ./bin/corgisign
build cli:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

## install: go install the CLI into $GOBIN (or $GOPATH/bin)
install:
	go install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

## test: run the SDK test suite
test:
	go test ./...

## vet: static analysis
vet:
	go vet ./...

## fmt: format the module
fmt:
	go fmt ./...

## clean: remove build artifacts
clean:
	rm -rf bin

MODULE  := github.com/xhelodev/tg
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
CGO     := 0
BIN     := tg

.PHONY: build test lint vet install cross smoke clean

build:
	CGO_ENABLED=$(CGO) go build -ldflags "$(LDFLAGS)" -o bin/$(BIN) ./cmd/tg

test:
	go test ./...

lint vet:
	go vet ./...

install:
	CGO_ENABLED=$(CGO) go install -ldflags "$(LDFLAGS)" ./cmd/tg

cross:
	@mkdir -p dist
	@for os in linux darwin windows; do \
	  for arch in amd64 arm64; do \
	    out=dist/$(BIN)_$${os}_$${arch}; \
	    if [ "$$os" = windows ]; then out=$$out.exe; fi; \
	    echo "→ $$out"; \
	    CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o $$out ./cmd/tg; \
	  done; \
	done

smoke:
	@if [ -z "$$TG_BOT_TOKEN" ] || [ -z "$$TG_TEST_CHAT" ]; then \
	  echo "smoke requiere TG_BOT_TOKEN y TG_TEST_CHAT"; exit 1; \
	fi
	$(MAKE) build
	./bin/$(BIN) me
	./bin/$(BIN) send --chat "$$TG_TEST_CHAT" "smoke $$(date -Iseconds)"

clean:
	rm -rf bin dist

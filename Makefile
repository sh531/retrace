# Single source of truth for the checks run by CI and the pre-commit hook.
# .golangci-lint-version is also read by CI and the dev container Dockerfile;
# .node-version is read by CI and matches the dev container's Node feature.
GOLANGCI_LINT_VERSION := $(shell cat .golangci-lint-version)
NODE_VERSION := $(shell cat .node-version)

.PHONY: check tidy-check fmt-check vet lint test test-js build clean

## check: run everything CI runs
check: tidy-check fmt-check vet lint test test-js build

tidy-check:
	go mod tidy -diff

fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; echo "run: gofmt -w ."; \
		exit 1; \
	fi

vet:
	go vet ./...

lint:
	@command -v golangci-lint >/dev/null || { \
		echo "golangci-lint not found; install $(GOLANGCI_LINT_VERSION): https://golangci-lint.run/docs/welcome/install/local/"; \
		exit 1; \
	}
	@installed=v$$(golangci-lint version --short); \
	if [ "$$installed" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "warning: golangci-lint $$installed installed, CI uses $(GOLANGCI_LINT_VERSION)"; \
	fi
	golangci-lint run

test:
	go test -race ./...

## test-js: the page's JavaScript tests, in Node; times and numbers are formatted for en-US
test-js:
	@command -v node >/dev/null || { \
		echo "node not found; install Node $(NODE_VERSION): https://nodejs.org/en/download"; \
		exit 1; \
	}
	LC_ALL=en_US.UTF-8 node --test 'internal/site/webtest/*.test.mjs'

build:
	go build -o bin/retrace ./cmd/retrace

clean:
	rm -rf bin

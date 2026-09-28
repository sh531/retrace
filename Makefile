# Single source of truth for the checks run by CI and the pre-commit hook.
# .golangci-lint-version is also read by CI and the dev container Dockerfile.
GOLANGCI_LINT_VERSION := $(shell cat .golangci-lint-version)

.PHONY: check tidy-check fmt-check vet lint test build clean

## check: run everything CI runs
check: tidy-check fmt-check vet lint test build

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

build:
	go build -o bin/retrace ./cmd/retrace

clean:
	rm -rf bin

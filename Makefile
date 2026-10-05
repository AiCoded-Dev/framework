GO ?= go
MODULES := . cmd/aicoded examples/contacts examples/people examples/room-maintenance
TIDY := $(MODULES) $(patsubst %/go.mod,%,$(wildcard cmd/aicoded/internal/*/testdata/*/go.mod cmd/aicoded/internal/*/testdata/*/*/go.mod lint/testdata/*/go.mod))

.PHONY: check test lint vuln secrets tidy tools proto proto-check install devui docs

check: proto-check test lint vuln secrets

test:
	@for m in $(MODULES); do (cd $$m && $(GO) test -race ./...) || exit 1; done

lint:
	@for m in $(MODULES); do (cd $$m && golangci-lint run ./...) || exit 1; done

vuln:
	@for m in $(MODULES); do (cd $$m && govulncheck ./...) || exit 1; done

secrets:
	@set -e; \
	if [ -e .gitleaksignore ] && grep -Eqv '^(#.*)?$$|^[0-9a-f]{40}:[^:]+:[^:]+:[0-9]+$$' .gitleaksignore; then \
		echo ".gitleaksignore may list only commit fingerprints: <commit>:<file>:<rule>:<line>"; \
		exit 1; \
	fi; \
	tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	printf '[extend]\nuseDefault = true\n' >"$$tmp/gitleaks.toml"; \
	gitleaks git --no-banner --no-color --redact --verbose --ignore-gitleaks-allow --config "$$tmp/gitleaks.toml" \
		--log-opts="--full-history --diff-filter=tuxdb HEAD" .; \
	git ls-files -z --cached --others --exclude-standard >"$$tmp/listed"; \
	xargs -0 sh -c 'for f; do if [ -f "$$f" ]; then printf "%s\0" "$$f"; fi; done' sh <"$$tmp/listed" >"$$tmp/files"; \
	if grep -qz : "$$tmp/files"; then \
		echo "file paths must not contain a colon: rename the file"; \
		exit 1; \
	fi; \
	tar -cf "$$tmp/tree.tar" --null -T "$$tmp/files"; \
	mkdir "$$tmp/tree"; \
	tar -xf "$$tmp/tree.tar" -C "$$tmp/tree"; \
	cd "$$tmp/tree"; \
	gitleaks dir --no-banner --no-color --redact --verbose --ignore-gitleaks-allow --config "$$tmp/gitleaks.toml" .

tidy:
	@for m in $(TIDY); do (cd $$m && $(GO) mod tidy) || exit 1; done

install:
	cd cmd/aicoded && $(GO) install -ldflags "-X 'main.frameworkDir=$(CURDIR)'" .

devui:
	cd cmd/aicoded && $(GO) test ./internal/devui -run '^TestPagesAreGenerated$$' -update

docs:
	$(GO) test ./docs -run '^TestDocsAreCurrent$$' -update

tools:
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
	$(GO) install golang.org/x/vuln/cmd/govulncheck@v1.8.0
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
	$(GO) install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.21.0
	$(GO) install github.com/bufbuild/buf/cmd/buf@v1.66.1
	$(GO) install github.com/zricethezav/gitleaks/v8@v8.30.1

proto:
	buf lint
	buf generate

proto-check:
	buf lint
	@set -e; \
	tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	buf generate --output "$$tmp"; \
	diff -r "$$tmp/runnerproto/runnerv1" runnerproto/runnerv1 || { echo "generated code is stale: run make proto"; exit 1; }

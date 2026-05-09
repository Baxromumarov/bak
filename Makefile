SHELL := /usr/bin/env bash
.SHELLFLAGS := -e -o pipefail -c

GO ?= go
BINDIR ?= bin

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/baxromumarov/bak/internal/cliapp.Version=$(VERSION) -X github.com/baxromumarov/bak/internal/cliapp.Commit=$(COMMIT) -X github.com/baxromumarov/bak/internal/cliapp.Date=$(DATE)

define go_build
	@echo "$(GO) build -mod=readonly -ldflags '$(LDFLAGS)' -o $(1) $(2)"
	@$(GO) build -mod=readonly -ldflags "$(LDFLAGS)" -o $(1) $(2) 2> >(grep -v '^go: writing stat cache: .*read-only file system$$' >&2)
endef

BAK_BIN := $(BINDIR)/bak
BAKFMT_BIN := $(BINDIR)/bakfmt
BAKLINT_BIN := $(BINDIR)/baklint
BAKCHECK_BIN := $(BINDIR)/bakcheck
DUMPBC_BIN := $(BINDIR)/dump_bc
LSP_BIN := $(BINDIR)/bak-lsp

TEST_SCRIPTS := \
	tests/run_alias_type_tests.sh \
	tests/run_defer_panic_conformance.sh \
	tests/run_func_arg_tests.sh \
	tests/run_typechecker_tests.sh

COMPREHENSIVE_SCRIPT := tests/run_comprehensive_tests.sh

FORMAT_CHECK_FILES := \
	src/std/collections/hashmap.bak \
	src/std/collections/vec.bak \
	src/std/errors/errors.bak \
	src/std/io/io.bak \
	src/std/path/path.bak \
	src/std/strings/strings.bak \
	tests/test_go_style_import.bak \
	examples/imports.bak

.PHONY: help all build rebuild build-tools build-bak build-root-bak build-bakfmt build-baklint build-bakcheck build-dump-bc build-lsp build-bak-fmt test test-unit test-scripts test-negative test-imports test-stdlib examples-check format-check test-comprehensive test-parity test-lsp-verify test-lanes language-stability test-all test-all-go release-check clean clean-binaries clean-cache distclean

define run_script_list
	@for script in $(1); do \
		echo "==> $$script"; \
		bash "$$script"; \
	done
endef

help:
	@echo "Bak project Makefile"
	@echo ""
	@echo "Build targets:"
	@echo "  make build            Build all project binaries into $(BINDIR)/"
	@echo "  make build-tools      Same as build"
	@echo "  make build-bak        Build $(BAK_BIN)"
	@echo "  make build-bakfmt     Build $(BAKFMT_BIN)"
	@echo "  make build-bak-fmt    Alias for build-bakfmt"
	@echo "  make build-baklint    Build $(BAKLINT_BIN)"
	@echo "  make build-bakcheck   Build $(BAKCHECK_BIN)"
	@echo "  make build-dump-bc    Build $(DUMPBC_BIN)"
	@echo "  make build-lsp        Build $(LSP_BIN)"
	@echo ""
	@echo "Test targets:"
	@echo "  make test             Run go test ./..."
	@echo "  make test-unit        Run go test ./..."
	@echo "  make test-scripts     Run focused shell guardrails under tests/"
	@echo "  make test-negative    Run expected-failure compiler guardrails"
	@echo "  make test-imports     Run package/import guardrails"
	@echo "  make test-stdlib      Run Bak stdlib tests"
	@echo "  make examples-check   Check stable v0.1 examples"
	@echo "  make format-check     Verify bakfmt output for stable language files"
	@echo "  make test-parity      Run VM/native parity guardrails"
	@echo "  make test-lsp-verify  Run the LSP smoke verifier"
	@echo "  make test-lanes       Run unit + scripts + stdlib + parity"
	@echo "  make language-stability Run release gate + LSP + formatter checks"
	@echo "  make test-comprehensive Run the comprehensive release gate"
	@echo "  make release-check    Build tools and run release-quality checks"
	@echo "  make test-all         Alias for test-lanes"
	@echo "  make test-all-go      Run Go tests + parity"
	@echo ""
	@echo "Cleanup targets:"
	@echo "  make clean            Remove binaries and local build artifacts"
	@echo "  make clean-binaries   Remove generated binaries only"
	@echo "  make clean-cache      Clear Go test/build caches"
	@echo "  make distclean        clean + clean-cache"

all: build

build: build-bak build-bakfmt build-baklint build-bakcheck build-dump-bc build-lsp

build-tools: build

rebuild: clean build

$(BINDIR):
	@mkdir -p $(BINDIR)

build-bak: | $(BINDIR)
	$(call go_build,$(BAK_BIN),./cmd/bak)

build-root-bak:
	$(call go_build,bak,./cmd/bak)

build-bakfmt: | $(BINDIR)
	$(call go_build,$(BAKFMT_BIN),./cmd/bakfmt)

build-bak-fmt: build-bakfmt

build-baklint: | $(BINDIR)
	$(call go_build,$(BAKLINT_BIN),./cmd/baklint)

build-bakcheck: | $(BINDIR)
	$(call go_build,$(BAKCHECK_BIN),./cmd/bakcheck)

build-dump-bc: | $(BINDIR)
	$(call go_build,$(DUMPBC_BIN),./cmd/dump_bc)

build-lsp: | $(BINDIR)
	$(call go_build,$(LSP_BIN),./lsp)

test: test-unit

test-unit:
	$(GO) test ./...

test-scripts: build-root-bak
	$(call run_script_list,$(TEST_SCRIPTS))

test-negative: build-root-bak
	@bash tests/run_typechecker_tests.sh
	@bash tests/run_func_arg_tests.sh

test-imports: build-root-bak
	$(GO) test ./pkg/packages ./pkg/typechecker -run 'Import|Resolve|Cyclic|Visibility|Alias'
	@bash tests/run_alias_type_tests.sh

test-stdlib: build-root-bak
	@./bak test src/std

examples-check: build-root-bak
	@BAK_BIN=./bak bash scripts/check_examples.sh

format-check: build-bakfmt
	@if ! out="$$( $(BAKFMT_BIN) -l $(FORMAT_CHECK_FILES) )"; then \
		echo "$$out"; \
		exit 1; \
	fi; \
	if [ -n "$$out" ]; then \
		echo "$$out"; \
		echo "Run: $(BAKFMT_BIN) -w $(FORMAT_CHECK_FILES)"; \
		exit 1; \
	fi

test-comprehensive: build-root-bak
	@echo "==> $(COMPREHENSIVE_SCRIPT)"
	@bash $(COMPREHENSIVE_SCRIPT)


test-parity:
	$(GO) test ./pkg/backend/native -run 'TestVMNative.*Parity|TestNativeSmoke'

test-lsp-verify: build-lsp
	$(GO) run ./scripts/verify_lsp

test-lanes: test-unit test-scripts test-imports test-stdlib examples-check test-parity

language-stability: test-comprehensive test-lsp-verify format-check

test-all: test-lanes

test-all-go: test-unit test-parity

release-check: build language-stability

clean: clean-binaries
	@rm -f coverage.out *.prof *.cov
	@find . -type d -name '.bak-cache' -prune -exec rm -rf {} +

clean-binaries:
	@rm -rf $(BINDIR)
	@rm -f bak bakfmt baklint bakcheck bak-lsp dump_bc
	@rm -f bak_test_binary

clean-cache:
	$(GO) clean -cache -testcache

distclean: clean clean-cache

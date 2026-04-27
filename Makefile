SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

GO ?= go
BINDIR ?= bin

GO_BUILD := $(GO) build -o

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

.PHONY: help all build rebuild build-tools build-bak build-root-bak build-bakfmt build-baklint build-bakcheck build-dump-bc build-lsp build-bak-fmt test test-unit test-scripts test-comprehensive test-frozen test-parity test-lanes test-all clean clean-binaries clean-cache distclean

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
	@echo "  make test-scripts     Run executable test scripts under tests/"
	@echo "  make test-comprehensive Run legacy broad-pattern comprehensive script"
	@echo "  make test-frozen      Run frozen-surface language and docs guardrails"
	@echo "  make test-parity      Run evaluator/vm/native parity matrix"
	@echo "  make test-lanes       Run frozen + parity lanes"
	@echo "  make test-all         Run unit tests + test scripts"
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
	$(GO_BUILD) $(BAK_BIN) ./cmd/bak

build-root-bak:
	$(GO_BUILD) bak ./cmd/bak

build-bakfmt: | $(BINDIR)
	$(GO_BUILD) $(BAKFMT_BIN) ./cmd/bakfmt

build-bak-fmt: build-bakfmt

build-baklint: | $(BINDIR)
	$(GO_BUILD) $(BAKLINT_BIN) ./cmd/baklint

build-bakcheck: | $(BINDIR)
	$(GO_BUILD) $(BAKCHECK_BIN) ./cmd/bakcheck

build-dump-bc: | $(BINDIR)
	$(GO_BUILD) $(DUMPBC_BIN) ./cmd/dump_bc

build-lsp: | $(BINDIR)
	$(GO_BUILD) $(LSP_BIN) ./lsp

test: test-unit

test-unit:
	$(GO) test ./...

test-scripts: build-root-bak
	$(call run_script_list,$(TEST_SCRIPTS))

test-comprehensive: build-root-bak
	@echo "==> $(COMPREHENSIVE_SCRIPT)"
	@bash $(COMPREHENSIVE_SCRIPT)

test-frozen:
	$(GO) test ./pkg/typechecker -run 'TestFrozenV01StableSurfaceParsesAndTypechecksWithoutExperimentalFlags|TestExperimentalUnsafeRequiresOptIn|TestExperimentalUserGenericsRequireOptIn|TestTypecheckerExperimentalFeatureGuardrailIncludesCodeAndHint'
	$(GO) test ./cmd/bak -run 'TestPublicDocsAndExamplesLabelExperimentalSurface|TestPublicConformanceTestsLabelExperimentalSurface|TestResolveProjectFeaturesByLanguageMode'

test-parity:
	$(GO) test ./pkg/backend/native -run TestEvaluatorVMNativeParityMatrix

test-lanes: test-frozen test-parity

test-all: test-unit test-scripts

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

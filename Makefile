SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

GO ?= go
BINDIR ?= bin

BAK_BIN := $(BINDIR)/bak
BAKFMT_BIN := $(BINDIR)/bakfmt
BAKLINT_BIN := $(BINDIR)/baklint
BAKCHECK_BIN := $(BINDIR)/bakcheck
BAKCTEST_BIN := $(BINDIR)/bakc-test
DUMPBC_BIN := $(BINDIR)/dump_bc
LSP_BIN := $(BINDIR)/bak-lsp

TEST_SCRIPTS := \
	tests/run_alias_type_tests.sh \
	tests/run_defer_panic_conformance.sh \
	tests/run_func_arg_tests.sh \
	tests/run_typechecker_tests.sh

LEGACY_TEST_SCRIPTS := \
	tests/run_native_bytes_tests.sh \
	tests/run_native_enum_tests.sh \
	tests/run_native_strconv_tests.sh \
	tests/run_native_string_tests.sh \
	tests/run_native_strings_std_tests.sh \
	tests/run_native_time_tests.sh \
	tests/run_native_vec_tests.sh

COMPREHENSIVE_SCRIPT := tests/run_comprehensive_tests.sh

.PHONY: help all build rebuild build-tools build-bak build-root-bak build-bakfmt build-baklint build-bakcheck build-bakc-test build-dump-bc build-lsp build-bak-fmt test test-unit test-scripts test-scripts-legacy test-comprehensive test-frozen test-parity test-lanes test-all clean clean-binaries clean-cache distclean

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
	@echo "  make build-bakc-test  Build $(BAKCTEST_BIN)"
	@echo "  make build-dump-bc    Build $(DUMPBC_BIN)"
	@echo "  make build-lsp        Build $(LSP_BIN)"
	@echo ""
	@echo "Test targets:"
	@echo "  make test             Run go test ./..."
	@echo "  make test-unit        Run go test ./..."
	@echo "  make test-scripts     Run executable test scripts under tests/"
	@echo "  make test-scripts-legacy Run legacy native/self-host script sweep"
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

build: build-bak build-bakfmt build-baklint build-bakcheck build-bakc-test build-dump-bc build-lsp

build-tools: build

rebuild: clean build

$(BINDIR):
	@mkdir -p $(BINDIR)

build-bak: | $(BINDIR)
	$(GO) build -o $(BAK_BIN) ./cmd/bak

build-root-bak:
	$(GO) build -o bak ./cmd/bak

build-bakfmt: | $(BINDIR)
	$(GO) build -o $(BAKFMT_BIN) ./cmd/bakfmt

build-bak-fmt: build-bakfmt

build-baklint: | $(BINDIR)
	$(GO) build -o $(BAKLINT_BIN) ./cmd/baklint

build-bakcheck: | $(BINDIR)
	$(GO) build -o $(BAKCHECK_BIN) ./cmd/bakcheck

build-bakc-test: | $(BINDIR)
	$(GO) build -o $(BAKCTEST_BIN) ./cmd/bakc-test

build-dump-bc: | $(BINDIR)
	$(GO) build -o $(DUMPBC_BIN) ./cmd/dump_bc

build-lsp: | $(BINDIR)
	$(GO) build -o $(LSP_BIN) ./lsp

test: test-unit

test-unit:
	$(GO) test ./...

test-scripts: build-root-bak
	@for script in $(TEST_SCRIPTS); do \
		echo "==> $$script"; \
		bash $$script; \
	done

test-scripts-legacy: build-root-bak
	@for script in $(LEGACY_TEST_SCRIPTS); do \
		echo "==> $$script"; \
		bash $$script; \
	done

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
	@rm -f bak bakfmt baklint bakcheck bakc-test bak-lsp dump_bc
	@rm -f bak_test_binary bakc-stage0 bakc-stage1 bakc-stage2 bakc

clean-cache:
	$(GO) clean -cache -testcache

distclean: clean clean-cache

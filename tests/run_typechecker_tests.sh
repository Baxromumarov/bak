#!/bin/bash
# =============================================================================
# Bak Compiler Type Checker Test Suite
# =============================================================================
# This script runs comprehensive tests for the type checker, verifying that:
# 1. Error tests (err_*.bak) fail with type errors
# 2. Pass tests (pass_*.bak) compile and run without type errors

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BAK_COMPILER="$PROJECT_ROOT/bak"
TEST_DIR="$SCRIPT_DIR/typechecker"

# Counters
PASSED=0
FAILED=0
TOTAL=0

# Build the compiler first
echo -e "${BLUE}=== Building Bak Compiler ===${NC}"
cd "$PROJECT_ROOT"
if ! go build -mod=readonly -o bak ./cmd/bak 2> >(grep -v '^go: writing stat cache: .*read-only file system$' >&2); then
    echo -e "${RED}ERROR: Failed to build compiler${NC}"
    exit 1
fi
echo -e "${GREEN}Compiler built successfully${NC}\n"

echo -e "${BLUE}=== Running Type Checker Tests ===${NC}\n"

# =============================================================================
# Error Tests - These should FAIL with type errors
# =============================================================================
echo -e "${YELLOW}--- Error Tests (should produce type errors) ---${NC}"

for test_file in "$TEST_DIR"/err_*.bak; do
    if [ ! -f "$test_file" ]; then
        continue
    fi
    
    TOTAL=$((TOTAL + 1))
    test_name=$(basename "$test_file")
    
    # Run the compiler and capture output
    output=$("$BAK_COMPILER" "$test_file" 2>&1 || true)
    
    # Check if it contains type errors
    if echo "$output" | grep -qi "type error\|cannot return\|cannot assign\|type mismatch"; then
        echo -e "${GREEN}✓ PASS${NC}: $test_name (correctly produced type error)"
        PASSED=$((PASSED + 1))
    else
        echo -e "${RED}✗ FAIL${NC}: $test_name (expected type error, but got:)"
        echo "    $output"
        FAILED=$((FAILED + 1))
    fi
done

echo ""

# =============================================================================
# Pass Tests - These should compile WITHOUT type errors
# =============================================================================
echo -e "${YELLOW}--- Pass Tests (should compile without errors) ---${NC}"

for test_file in "$TEST_DIR"/pass_*.bak; do
    if [ ! -f "$test_file" ]; then
        continue
    fi
    
    TOTAL=$((TOTAL + 1))
    test_name=$(basename "$test_file")
    
    # Run the compiler and capture output
    output=$("$BAK_COMPILER" "$test_file" 2>&1 || true)
    
    # Check if it ran without type errors (we look for output or no errors)
    if echo "$output" | grep -qi "type error\|cannot return\|cannot assign"; then
        echo -e "${RED}✗ FAIL${NC}: $test_name (unexpected type error:)"
        echo "    $(echo "$output" | head -5)"
        FAILED=$((FAILED + 1))
    else
        echo -e "${GREEN}✓ PASS${NC}: $test_name (compiled without type errors)"
        PASSED=$((PASSED + 1))
    fi
done

echo ""

# =============================================================================
# Summary
# =============================================================================
echo -e "${BLUE}=== Test Summary ===${NC}"
echo -e "Total:  $TOTAL"
echo -e "${GREEN}Passed: $PASSED${NC}"
echo -e "${RED}Failed: $FAILED${NC}"

if [ $FAILED -eq 0 ]; then
    echo -e "\n${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "\n${RED}Some tests failed!${NC}"
    exit 1
fi

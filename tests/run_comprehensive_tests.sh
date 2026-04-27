#!/bin/bash
# =============================================================================
# Bak Compiler Comprehensive Test Suite
# =============================================================================
# Tests all aspects of the Bak compiler:
# - Syntax & Parsing
# - Type Checking (Result, Option, strict types)
# - Ownership & Borrowing
# - Mutability
# - Structs & Impl blocks
# - Enums
# - Functions & Methods
# - Imports & Modules
# - Visibility (pub/private)
# - Aliases & Type definitions
# - Control Flow
# - Error Handling

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BAK_COMPILER="$PROJECT_ROOT/bak"
TESTS_DIR="$SCRIPT_DIR"

# Global counters
TOTAL_PASSED=0
TOTAL_FAILED=0
TOTAL_XFAIL=0
TOTAL_TESTS=0

# Expected failures (tests that should fail with errors)
EXPECTED_FAILS=(
    "test_import_cycle_a.bak"
    "test_import_cycle_b.bak"
    "err_private_const.bak"
    "err_private_func.bak"
    "err_private_struct.bak"
)

# Build compiler
build_compiler() {
    cd "$PROJECT_ROOT"
    echo -e "${BLUE}Building Bak compiler...${NC}"
    if ! go build -mod=readonly -o bak ./cmd/bak 2> >(grep -v '^go: writing stat cache: .*read-only file system$' >&2); then
        echo -e "${RED}ERROR: Failed to build compiler${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Compiler built successfully${NC}\n"
}

# Check if a test is expected to fail
is_expected_fail() {
    local test_name="$1"
    for xfail in "${EXPECTED_FAILS[@]}"; do
        if [[ "$test_name" == *"$xfail"* ]]; then
            return 0
        fi
    done
    # err_ prefix tests are expected to produce errors
    if [[ "$test_name" == err_* ]]; then
        return 0
    fi
    return 1
}

# Run a single test file
run_test() {
    local test_file="$1"
    local test_name=$(basename "$test_file")
    local expected_fail=false
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    if is_expected_fail "$test_name"; then
        expected_fail=true
    fi
    
    # Run the test
    local output
    output=$("$BAK_COMPILER" "$test_file" 2>&1 || true)
    local has_error=false
    
    if echo "$output" | grep -qi "error\|panic\|undefined\|cannot"; then
        has_error=true
    fi
    
    printf "  %-50s " "$test_name"
    
    if [ "$expected_fail" = true ]; then
        if [ "$has_error" = true ]; then
            echo -e "[${YELLOW}XFAIL${NC}] (expected error)"
            TOTAL_XFAIL=$((TOTAL_XFAIL + 1))
        else
            echo -e "[${GREEN}PASS${NC}]"
            TOTAL_PASSED=$((TOTAL_PASSED + 1))
        fi
    else
        if [ "$has_error" = true ]; then
            echo -e "[${RED}FAIL${NC}]"
            echo "    Error: $(echo "$output" | head -1)"
            TOTAL_FAILED=$((TOTAL_FAILED + 1))
        else
            echo -e "[${GREEN}PASS${NC}]"
            TOTAL_PASSED=$((TOTAL_PASSED + 1))
        fi
    fi
}

# Run tests in a category
run_category() {
    local category="$1"
    local pattern="$2"
    local files
    
    echo -e "\n${CYAN}━━━ $category ━━━${NC}"
    
    files=($(find "$TESTS_DIR" -name "$pattern" -type f 2>/dev/null | sort))
    
    if [ ${#files[@]} -eq 0 ]; then
        echo -e "  ${YELLOW}No tests found${NC}"
        return
    fi
    
    for test_file in "${files[@]}"; do
        run_test "$test_file"
    done
}

# Main test execution
main() {
    echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║       BAK COMPILER COMPREHENSIVE TEST SUITE                    ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}\n"
    
    build_compiler
    
    # =========================================================================
    # TYPE CHECKING TESTS
    # =========================================================================
    echo -e "${BLUE}▶ TYPE CHECKING${NC}"
    run_category "Result/Option Types" "typechecker/*.bak"
    run_category "Type Aliases" "*alias*.bak"
    run_category "Type Definitions" "*type*.bak"
    run_category "Copy Types" "*copy*.bak"
    
    # =========================================================================
    # OWNERSHIP & BORROWING TESTS
    # =========================================================================
    echo -e "\n${BLUE}▶ OWNERSHIP & BORROWING${NC}"
    run_category "Ownership" "*ownership*.bak"
    run_category "Borrowing" "*borrow*.bak"
    run_category "Move Semantics" "*move*.bak"
    run_category "Mutability" "*mut*.bak"
    
    # =========================================================================
    # DATA STRUCTURES TESTS
    # =========================================================================
    echo -e "\n${BLUE}▶ DATA STRUCTURES${NC}"
    run_category "Structs" "*struct*.bak"
    run_category "Enums" "*enum*.bak"
    run_category "Vectors" "*vec*.bak"
    run_category "Generics" "*generic*.bak"
    
    # =========================================================================
    # FUNCTIONS & CONTROL FLOW TESTS
    # =========================================================================
    echo -e "\n${BLUE}▶ FUNCTIONS & CONTROL FLOW${NC}"
    run_category "Functions" "*function*.bak"
    run_category "Control Flow" "*control*.bak"
    run_category "Error Handling" "*error*.bak"
    
    # =========================================================================
    # MODULES & VISIBILITY TESTS
    # =========================================================================
    echo -e "\n${BLUE}▶ MODULES & VISIBILITY${NC}"
    run_category "Imports" "*import*.bak"
    run_category "Visibility" "*visibility*.bak"
    run_category "Private Access" "*private*.bak"
    run_category "Public Access" "*pub*.bak"
    
    # =========================================================================
    # LANGUAGE FEATURES TESTS
    # =========================================================================
    echo -e "\n${BLUE}▶ LANGUAGE FEATURES${NC}"
    run_category "Constants" "*constant*.bak"
    run_category "Strings" "*string*.bak"
    run_category "Numeric Literals" "*numeric*.bak"
    run_category "Bitwise Operations" "*bitwise*.bak"
    run_category "Language Features" "*language*.bak"
    run_category "Basics" "*basic*.bak"
    
    # =========================================================================
    # FUNCTION ARGUMENTS TESTS
    # =========================================================================
    echo -e "\n${BLUE}▶ FUNCTION ARGUMENTS${NC}"
    run_category "Argument Count" "*args*.bak"
    run_category "Multi-return" "*multireturn*.bak"
    run_category "Void Returns" "*void*.bak"
    
    # =========================================================================
    # ALGORITHM TESTS
    # =========================================================================
    echo -e "\n${BLUE}▶ ALGORITHMS${NC}"
    run_category "Algorithms" "*algorithm*.bak"
    
    # =========================================================================
    # SUMMARY
    # =========================================================================
    echo -e "\n${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║                        TEST SUMMARY                             ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}\n"
    
    echo -e "  Total Tests:    $TOTAL_TESTS"
    echo -e "  ${GREEN}Passed:         $TOTAL_PASSED${NC}"
    echo -e "  ${YELLOW}Expected Fails: $TOTAL_XFAIL${NC}"
    echo -e "  ${RED}Failed:         $TOTAL_FAILED${NC}"
    echo ""
    
    local success_rate=$(( (TOTAL_PASSED + TOTAL_XFAIL) * 100 / TOTAL_TESTS ))
    echo -e "  Success Rate:   ${success_rate}%"
    echo ""
    
    if [ $TOTAL_FAILED -eq 0 ]; then
        echo -e "${GREEN}✓ All tests passed!${NC}"
        exit 0
    else
        echo -e "${RED}✗ Some tests failed!${NC}"
        exit 1
    fi
}

# Run main
main "$@"

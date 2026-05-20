#!/bin/bash
# Test runner for alias, type, and visibility tests
# Run from the bak project root: ./tests/run_alias_type_tests.sh

BAK="${BAK:-./bak}"
TESTS_DIR="tests"
PASSED=0
FAILED=0

echo "=============================================="
echo "Alias, Type Definition, and Visibility Tests"
echo "=============================================="
echo ""

# Helper function to run a test that should succeed
run_success_test() {
    local name="$1"
    local file="$2"
    
    echo "🧪 Test: $name"
    output=$($BAK check "$file" 2>&1)
    if [ $? -eq 0 ]; then
        echo "   ✅ PASSED"
        ((PASSED++))
    else
        echo "   ❌ FAILED - expected success"
        echo "   Error: $output"
        ((FAILED++))
    fi
}

# Helper function to run a test that should fail with an error
run_error_test() {
    local name="$1"
    local file="$2"
    local expected_pattern="$3"
    
    echo "🧪 Test: $name"
    output=$($BAK check "$file" 2>&1 || true)
    if echo "$output" | grep -Eq "$expected_pattern"; then
        echo "   ✅ PASSED - correctly detected error"
        ((PASSED++))
    else
        echo "   ❌ FAILED - expected pattern '$expected_pattern'"
        echo "   Got: $output"
        ((FAILED++))
    fi
}

echo "=== SUCCESS TESTS ==="
echo ""

run_success_test "Local alias and type definitions" \
    "$TESTS_DIR/test_alias_local.bak"

run_success_test "Imported type aliases" \
    "$TESTS_DIR/test_alias_imported.bak"

run_success_test "Public visibility access" \
    "$TESTS_DIR/test_visibility.bak"

echo ""
echo "=== ERROR TESTS (should fail) ==="
echo ""

run_error_test "Private struct access" \
    "$TESTS_DIR/err_private_struct.bak" \
    "private"

run_error_test "Private function access" \
    "$TESTS_DIR/err_private_func.bak" \
    "private"

run_error_test "Private constant access" \
    "$TESTS_DIR/err_private_const.bak" \
    "private"

echo ""
echo "=============================================="
echo "Results: $PASSED passed, $FAILED failed"
echo "=============================================="

if [ $FAILED -eq 0 ]; then
    echo "🎉 All alias/type/visibility tests passed!"
    exit 0
else
    echo "❌ Some tests failed"
    exit 1
fi

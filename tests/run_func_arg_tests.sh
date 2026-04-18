#!/bin/bash
# Test runner for function argument validation tests
# Run from the bak project root: ./tests/run_func_arg_tests.sh

BAK="./bak"
TESTS_DIR="tests"
PASSED=0
FAILED=0

echo "=============================================="
echo "Function Argument Validation Test Suite"
echo "=============================================="
echo ""

# Test 1: Correct usage (should pass)
echo "🧪 Test: Correct usage (all valid calls)"
if $BAK $TESTS_DIR/func_args_test.bak 2>&1 | grep -q "All correct usage tests passed!"; then
    echo "   ✅ PASSED"
    ((PASSED++))
else
    echo "   ❌ FAILED - correct usage should work"
    ((FAILED++))
fi

# Error tests - each should produce an error
run_error_test() {
    local name="$1"
    local file="$2"
    local expected_pattern="$3"
    
    echo "🧪 Test: $name"
    output=$($BAK "$file" 2>&1 || true)
    if echo "$output" | grep -q "$expected_pattern"; then
        echo "   ✅ PASSED - correctly detected error"
        ((PASSED++))
    else
        echo "   ❌ FAILED - expected pattern '$expected_pattern'"
        echo "   Got: $output"
        ((FAILED++))
    fi
}

run_error_test "noParams() called with 1 arg" \
    "$TESTS_DIR/err_too_many_args_to_noparams.bak" \
    "expects 0 argument(s), but got 1"

run_error_test "oneParam() called with 0 args" \
    "$TESTS_DIR/err_zero_args_to_oneparam.bak" \
    "expects 1 argument(s), but got 0"

run_error_test "twoParams() called with 1 arg (partial)" \
    "$TESTS_DIR/err_partial_args_2_got_1.bak" \
    "expects 2 argument(s), but got 1"

run_error_test "fourParams() called with 2 args (half)" \
    "$TESTS_DIR/err_partial_args_4_got_2.bak" \
    "expects 4 argument(s), but got 2"

run_error_test "threeParams() called with 5 args (too many)" \
    "$TESTS_DIR/err_too_many_args_3_got_5.bak" \
    "expects 3 argument(s), but got 5"

run_error_test "sixParams() called with 0 args" \
    "$TESTS_DIR/err_zero_args_to_sixparams.bak" \
    "expects 6 argument(s), but got 0"

run_error_test "Tuple destructuring with 0 args" \
    "$TESTS_DIR/err_multireturn_zero_args.bak" \
    "expects 2 argument(s), but got 0"

run_error_test "Tuple destructuring with too many args" \
    "$TESTS_DIR/err_multireturn_too_many_args.bak" \
    "expects 2 argument(s), but got 4"

run_error_test "var assignment with 0 args" \
    "$TESTS_DIR/err_var_assign_zero_args.bak" \
    "expects 1 argument(s), but got 0"

run_error_test "var assignment with too many args" \
    "$TESTS_DIR/err_var_assign_too_many_args.bak" \
    "expects 1 argument(s), but got 3"

run_error_test "void function with 3 args" \
    "$TESTS_DIR/err_void_with_args.bak" \
    "expects 0 argument(s), but got 3"

# Multiple errors test
echo "🧪 Test: Multiple errors in one file"
output=$($BAK $TESTS_DIR/err_multiple_errors.bak 2>&1 || true)
error_count=$(echo "$output" | grep -c "expects.*argument(s), but got" || true)
if [ "$error_count" -eq 4 ]; then
    echo "   ✅ PASSED - detected all 4 errors"
    ((PASSED++))
else
    echo "   ❌ FAILED - expected 4 errors, got $error_count"
    ((FAILED++))
fi

echo ""
echo "=============================================="
echo "Results: $PASSED passed, $FAILED failed"
echo "=============================================="

if [ $FAILED -eq 0 ]; then
    echo "🎉 All tests passed!"
    exit 0
else
    echo "❌ Some tests failed"
    exit 1
fi

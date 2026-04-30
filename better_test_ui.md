Current UI:
./bin/bak test dyn_arr_test.bak
running  1  tests
 PASS    dyn_arr_test.bak:test_sum_function


 ok ( 1  tests) 

Test file summary: total=1 executed=1 skipped=0 passed=1 failed=0
~/g/s/gith/bax/bak main !53 ?1 > ./bin/bak test dyn_arr_test.bak
running  1  tests
 PASS    dyn_arr_test.bak:test_sum_function


 ok ( 1  tests) 

Test file summary: total=1 executed=1 skipped=0 passed=1 failed=0


Task: I want you to improve the result of test outputs and also during running the test adding better UI and information
about tests, why it failed, where it failed, how to fix or find the issue, etc.

here is some better ui:

1. Add Visual Hierarchy & Color

━━ Running 1 test ━━

📁 dyn_arr_test.bak

    ✓  test_sum_function                                    2.4ms
━━ Summary ━━
✔ Passed   1
✖ Failed   0
↷ Skipped  0
──────────────
Total      1

2. Use Compact Table Format (for multiple tests)
$ ./bin/bak test

dyn_arr_test.bak
  ✓ test_sum_function        2.4ms    [math/array]
  ✓ test_resize_empty        1.1ms    [math/array]
  ✗ test_overflow_guard      0.8ms    [math/array]  ← ASSERT_EQ failed: expected 42, got 0

────────────────────────────
━━ Summary ━━
✔ Passed   2
✖ Failed   1
↷ Skipped  0
──────────────
Total      2


3. Show Progress (for large suites)
$ ./bin/bak test --all

[░░░░░░░░░░░░░░░░░░] 0/47
[██░░░░░░░░░░░░░░░░] 5/47
[██████████████████] 47/47


4. Add Context on Failure
# Before (frustrating)
 FAIL  dyn_arr_test.bak:test_sum_function

# After (actionable)
 FAIL  dyn_arr_test.bak:test_sum_function
       │
       ├── src/dyn_arr.bak:42  sum(arr)
       │                       expected: 15
       │                       actual:   12
       │
       └── hint: Array length was 0, loop never executed

5. Minimalist "Quiet" Mode

$ ./bin/bak test --quiet
.
1 ok

$ ./bin/bak test --quiet
F
1 failed  dyn_arr_test.bak:test_sum_function

6. Summary Block Redesign
Instead of:
Test file summary: total=1 executed=1 skipped=0 passed=1 failed=0
Try:
╭────────────────────────────────╮
│  🎉 All green!                 │
│                                │
│  1 test  •  0 skipped  •  4ms  │
╰────────────────────────────────╯
Or for failures:
╭────────────────────────────────────────╮
│  ❌ 2 failures in 1 file               │
│                                        │
│  dyn_arr_test.bak                      │
│    └─ test_sum_function (line 42)      │
│    └─ test_resize (line 67)            │
│                                        │
│  Run with --verbose for full traces    │
╰────────────────────────────────────────╯

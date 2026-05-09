package test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// TestRunEndToEnd_SimplePassing verifies that internal/test.Run can discover and execute
// a simple *_test.bak file using the AST-based runner.
func TestRunEndToEnd_SimplePassing(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import test "src/std/test/test.bak"

func test_one() -> (void) {
    mut var t: test.T = test.new("test_one")
    t.finish()
}
`

	if err := os.WriteFile(filepath.Join(dir, "simple_test.bak"), []byte(src), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Capture stdout/stderr
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr
	defer func() {
		wOut.Close()
		wErr.Close()
		os.Stdout = oldOut
		os.Stderr = oldErr
	}()

	opts := Options{Targets: []string{dir}}
	err := Run([]string{dir}, runtimecap.Permissions{}, opts)

	// Flush pipes
	wOut.Close()
	wErr.Close()
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	io.Copy(&buf, rErr)
	out := buf.String()

	if err != nil {
		t.Fatalf("Run returned error: %v\nOutput:\n%s", err, out)
	}
	if !strings.Contains(out, "━━ Summary ━━") {
		t.Fatalf("expected test summary block in output, got:\n%s", out)
	}
	if strings.Contains(out, "File summary:") {
		t.Fatalf("did not expect file summary for a single fully-executed file, got:\n%s", out)
	}
}

func TestRunEndToEnd_FailingTest(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import test "src/std/test/test.bak"

func test_bad() -> (void) {
    // does not call t.finish()
    mut var t: test.T = test.new("test_bad")
}
`
	if err := os.WriteFile(filepath.Join(dir, "bad_test.bak"), []byte(src), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr
	defer func() {
		wOut.Close()
		wErr.Close()
		os.Stdout = oldOut
		os.Stderr = oldErr
	}()

	opts := Options{Targets: []string{dir}}
	err := Run([]string{dir}, runtimecap.Permissions{}, opts)

	wOut.Close()
	wErr.Close()
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	io.Copy(&buf, rErr)
	out := buf.String()

	if err == nil {
		t.Fatalf("Run returned nil error for failing test\nOutput:\n%s", out)
	}
	if !errors.Is(err, ErrTestsFailed) {
		t.Fatalf("expected ErrTestsFailed, got: %v", err)
	}
	if !strings.Contains(out, "FAIL") {
		t.Fatalf("expected test failure markers in output, got:\n%s", out)
	}
}

func TestRunEndToEnd_RunPatternFilter(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import test "src/std/test/test.bak"

func test_alpha() -> (void) {
    mut var t: test.T = test.new("test_alpha")
    t.finish()
}

func test_beta() -> (void) {
    mut var t: test.T = test.new("test_beta")
    t.finish()
}
`
	if err := os.WriteFile(filepath.Join(dir, "two_test.bak"), []byte(src), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr
	defer func() {
		wOut.Close()
		wErr.Close()
		os.Stdout = oldOut
		os.Stderr = oldErr
	}()

	opts := Options{Targets: []string{dir}, RunPattern: "alpha"}
	err := Run([]string{dir}, runtimecap.Permissions{}, opts)

	wOut.Close()
	wErr.Close()
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	io.Copy(&buf, rErr)
	out := buf.String()

	if err != nil {
		t.Fatalf("Run returned error: %v\nOutput:\n%s", err, out)
	}
	if !strings.Contains(out, "test_alpha") || strings.Contains(out, "test_beta") {
		t.Fatalf("expected only test_alpha to run, got:\n%s", out)
	}
}

func TestRunEndToEnd_PackageFilter(t *testing.T) {
	root := t.TempDir()
	aDir := filepath.Join(root, "pa")
	bDir := filepath.Join(root, "pb")
	if err := os.MkdirAll(aDir, 0o755); err != nil {
		t.Fatalf("mkdir a: %v", err)
	}
	if err := os.MkdirAll(bDir, 0o755); err != nil {
		t.Fatalf("mkdir b: %v", err)
	}
	aSrc := `package a

import test "src/std/test/test.bak"

func test_a() -> (void) {
    mut var t: test.T = test.new("test_a")
    t.finish()
}
`
	bSrc := `package b

import test "src/std/test/test.bak"

func test_b() -> (void) {
    mut var t: test.T = test.new("test_b")
    t.finish()
}
`
	if err := os.WriteFile(filepath.Join(aDir, "a_test.bak"), []byte(aSrc), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bDir, "b_test.bak"), []byte(bSrc), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr
	defer func() {
		wOut.Close()
		wErr.Close()
		os.Stdout = oldOut
		os.Stderr = oldErr
	}()

	opts := Options{Targets: []string{root}, PackageFilters: []string{"a"}}
	err := Run([]string{root}, runtimecap.Permissions{}, opts)

	wOut.Close()
	wErr.Close()
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	io.Copy(&buf, rErr)
	out := buf.String()

	if err != nil {
		t.Fatalf("Run returned error: %v\nOutput:\n%s", err, out)
	}
	if !strings.Contains(out, "test_a") || strings.Contains(out, "test_b") {
		t.Fatalf("expected only package a tests to run, got:\n%s", out)
	}
}

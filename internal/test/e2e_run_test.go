package test

import (
	"bytes"
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

import "src/std/test/test.bak" as test

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
	err := Run([]string{dir}, runtimecap.Permissions{}, nil, opts)

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
	if !strings.Contains(out, "Test file summary") {
		t.Fatalf("expected summary in output, got:\n%s", out)
	}
}

func TestRunEndToEnd_FailingTest(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import "src/std/test/test.bak" as test

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
	err := Run([]string{dir}, runtimecap.Permissions{}, nil, opts)

	wOut.Close()
	wErr.Close()
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	io.Copy(&buf, rErr)
	out := buf.String()

	if err != nil {
		t.Fatalf("Run returned unexpected error: %v\nOutput:\n%s", err, out)
	}
	if !strings.Contains(out, "test result: FAILED") && !strings.Contains(out, "FAIL") {
		t.Fatalf("expected test failure markers in output, got:\n%s", out)
	}
}

func TestRunEndToEnd_RunPatternFilter(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import "src/std/test/test.bak" as test

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
	err := Run([]string{dir}, runtimecap.Permissions{}, nil, opts)

	wOut.Close()
	wErr.Close()
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	io.Copy(&buf, rErr)
	out := buf.String()

	if err != nil {
		t.Fatalf("Run returned error: %v\nOutput:\n%s", err, out)
	}
	if !strings.Contains(out, "executed=1") {
		t.Fatalf("expected executed=1 in output, got:\n%s", out)
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

import "src/std/test/test.bak" as test

func test_a() -> (void) {
    mut var t: test.T = test.new("test_a")
    t.finish()
}
`
	bSrc := `package b

import "src/std/test/test.bak" as test

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
	err := Run([]string{root}, runtimecap.Permissions{}, nil, opts)

	wOut.Close()
	wErr.Close()
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	io.Copy(&buf, rErr)
	out := buf.String()

	if err != nil {
		t.Fatalf("Run returned error: %v\nOutput:\n%s", err, out)
	}
	if !strings.Contains(out, "executed=1") {
		t.Fatalf("expected executed=1 in output, got:\n%s", out)
	}
}

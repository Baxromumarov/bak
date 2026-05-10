package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaklintRequiresPaths(t *testing.T) {
	_, stderr, exitCode := runBaklint(t, nil)
	if exitCode != 2 {
		t.Fatalf("unexpected exit code: got %d stderr=%q", exitCode, stderr)
	}
	if !strings.Contains(stderr, "baklint: requires file or directory arguments") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestBaklintReportsFindings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_name.bak")
	if err := os.WriteFile(path, []byte("package main\n\nfunc BadName() -> (void) {\n    return void\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	_, stderr, exitCode := runBaklint(t, []string{path})
	if exitCode != 1 {
		t.Fatalf("unexpected exit code: got %d stderr=%q", exitCode, stderr)
	}
	if !strings.Contains(stderr, path+":3:6: function 'BadName' should be camelCase [naming-convention]") {
		t.Fatalf("expected lint finding, got %q", stderr)
	}
	if !strings.Contains(stderr, "1 finding(s) in 1 file(s)") {
		t.Fatalf("expected summary, got %q", stderr)
	}
}

func TestBaklintDisableSuppressesRule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_name.bak")
	if err := os.WriteFile(path, []byte("package main\n\nfunc BadName() -> (void) {\n    return void\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stdout, stderr, exitCode := runBaklint(t, []string{"--disable", "naming-convention", path})
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: got %d stdout=%q stderr=%q", exitCode, stdout, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("expected disabled-rule run to be quiet, stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestBaklintWalksDirectoriesAndSkipsBakCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_name.bak")
	cacheDir := filepath.Join(dir, ".bak-cache")
	cachePath := filepath.Join(cacheDir, "ignored.bak")
	if err := os.WriteFile(path, []byte("package main\n\nfunc BadName() -> (void) {\n    return void\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("package main\n\nfunc AlsoBad() -> (void) {\n    return void\n}\n"), 0o644); err != nil {
		t.Fatalf("write cached file: %v", err)
	}

	_, stderr, exitCode := runBaklint(t, []string{dir})
	if exitCode != 1 {
		t.Fatalf("unexpected exit code: got %d stderr=%q", exitCode, stderr)
	}
	if !strings.Contains(stderr, path) {
		t.Fatalf("expected real file finding, got %q", stderr)
	}
	if strings.Contains(stderr, cachePath) {
		t.Fatalf("expected cache file to be skipped, got %q", stderr)
	}
}

func TestBaklintListRules(t *testing.T) {
	stdout, stderr, exitCode := runBaklint(t, []string{"--list-rules"})
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: got %d stdout=%q stderr=%q", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	got := strings.Fields(stdout)
	want := []string{"complexity", "empty-block", "import-style", "naming-convention", "public-api-style", "style"}
	if len(got) != len(want) {
		t.Fatalf("unexpected rules: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected rule at index %d: got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestBaklintHelperProcess(t *testing.T) {
	if os.Getenv("BAKLINT_HELPER_PROCESS") != "1" {
		return
	}

	helperArgs := helperProcessArgs(os.Args)
	flag.CommandLine = flag.NewFlagSet("baklint", flag.ExitOnError)
	os.Args = append([]string{"baklint"}, helperArgs...)
	main()
	os.Exit(0)
}

func runBaklint(t *testing.T, args []string) (string, string, int) {
	t.Helper()

	cmdArgs := append([]string{"-test.run=TestBaklintHelperProcess", "--"}, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), "BAKLINT_HELPER_PROCESS=1")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run helper: %v", err)
	}
	return stdout.String(), stderr.String(), exitErr.ExitCode()
}

func helperProcessArgs(argv []string) []string {
	for i, arg := range argv {
		if arg == "--" {
			return argv[i+1:]
		}
	}
	return nil
}

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

func TestBakfmtFormatsStdin(t *testing.T) {
	stdout, stderr, exitCode := runBakfmt(t, "package main\nfunc main()->(void){return void}", nil)
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: got %d stderr=%q", exitCode, stderr)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "func main() -> (void) {") {
		t.Fatalf("expected formatted stdout, got %q", stdout)
	}
}

func TestBakfmtWriteRewritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.bak")
	if err := os.WriteFile(path, []byte("package main\nfunc main()->(void){return void}"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stdout, stderr, exitCode := runBakfmt(t, "", []string{"-w", path})
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: got %d stderr=%q", exitCode, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("expected quiet rewrite, stdout=%q stderr=%q", stdout, stderr)
	}

	rewritten, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}
	if !strings.Contains(string(rewritten), "func main() -> (void) {") {
		t.Fatalf("expected rewritten file to be formatted, got %q", string(rewritten))
	}
}

func TestBakfmtListWalksDirectories(t *testing.T) {
	dir := t.TempDir()
	needsFormat := filepath.Join(dir, "needs_format.bak")
	alreadyFormat := filepath.Join(dir, "already_format.bak")
	ignored := filepath.Join(dir, "notes.txt")

	if err := os.WriteFile(needsFormat, []byte("package main\nfunc main()->(void){return void}"), 0o644); err != nil {
		t.Fatalf("write needs_format: %v", err)
	}
	if err := os.WriteFile(alreadyFormat, []byte("package main\n\nfunc main() -> (void) {\n    return void\n}\n"), 0o644); err != nil {
		t.Fatalf("write already_format: %v", err)
	}
	if err := os.WriteFile(ignored, []byte("not bak"), 0o644); err != nil {
		t.Fatalf("write ignored file: %v", err)
	}

	stdout, stderr, exitCode := runBakfmt(t, "", []string{"-l", dir})
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: got %d stderr=%q", exitCode, stderr)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	lines := strings.Fields(strings.TrimSpace(stdout))
	if len(lines) != 1 || lines[0] != needsFormat {
		t.Fatalf("unexpected listed files: %q", stdout)
	}
}

func TestBakfmtReportsParseErrorsForFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.bak")
	if err := os.WriteFile(path, []byte("package main\nfunc main("), 0o644); err != nil {
		t.Fatalf("write broken file: %v", err)
	}

	_, stderr, exitCode := runBakfmt(t, "", []string{path})
	if exitCode != 1 {
		t.Fatalf("unexpected exit code: got %d stderr=%q", exitCode, stderr)
	}
	if !strings.Contains(stderr, "bakfmt: "+path+":") {
		t.Fatalf("expected file parse error, got %q", stderr)
	}
}

func TestBakfmtHelperProcess(t *testing.T) {
	if os.Getenv("BAKFMT_HELPER_PROCESS") != "1" {
		return
	}

	helperArgs := helperProcessArgs(os.Args)
	flag.CommandLine = flag.NewFlagSet("bakfmt", flag.ExitOnError)
	os.Args = append([]string{"bakfmt"}, helperArgs...)
	main()
	os.Exit(0)
}

func runBakfmt(t *testing.T, stdin string, args []string) (string, string, int) {
	t.Helper()

	cmdArgs := append([]string{"-test.run=TestBakfmtHelperProcess", "--"}, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), "BAKFMT_HELPER_PROCESS=1")
	cmd.Stdin = strings.NewReader(stdin)
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

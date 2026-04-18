package runtimecap

import (
	"testing"
	"time"
)

func TestExecuteCommandCapturesOutputAndExitCode(t *testing.T) {
	result, err := ExecuteCommand("sh", []string{"-c", "printf out; printf err >&2; exit 7"}, Permissions{ExecTimeout: DefaultExecTimeout, ExecMaxOutput: DefaultExecMaxOutputBytes})
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("unexpected exit code: got %d want %d", result.ExitCode, 7)
	}
	if result.TimedOut {
		t.Fatalf("did not expect timeout")
	}
	if result.Truncated {
		t.Fatalf("did not expect truncation")
	}
	if result.Stdout != "out" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if result.Stderr != "err" {
		t.Fatalf("unexpected stderr: %q", result.Stderr)
	}
	if result.Output != "outerr" {
		t.Fatalf("unexpected combined output: %q", result.Output)
	}
}

func TestExecuteCommandTruncatesCombinedOutput(t *testing.T) {
	result, err := ExecuteCommand("sh", []string{"-c", "printf abcdef"}, Permissions{ExecTimeout: DefaultExecTimeout, ExecMaxOutput: 3})
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code: got %d want %d", result.ExitCode, 0)
	}
	if !result.Truncated {
		t.Fatalf("expected truncation flag")
	}
	if result.Stdout != "abc" {
		t.Fatalf("unexpected truncated stdout: %q", result.Stdout)
	}
	if result.Output != "abc" {
		t.Fatalf("unexpected truncated combined output: %q", result.Output)
	}
}

func TestExecuteCommandTimesOutWithStableExitCode(t *testing.T) {
	result, err := ExecuteCommand("sh", []string{"-c", "sleep 1"}, Permissions{ExecTimeout: 20 * time.Millisecond, ExecMaxOutput: DefaultExecMaxOutputBytes})
	if err != nil {
		t.Fatalf("ExecuteCommand failed: %v", err)
	}
	if !result.TimedOut {
		t.Fatalf("expected timeout flag")
	}
	if result.ExitCode != -1 {
		t.Fatalf("unexpected timeout exit code: got %d want %d", result.ExitCode, -1)
	}
}

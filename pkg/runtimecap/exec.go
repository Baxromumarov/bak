package runtimecap

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sync"
)

type ExecResult struct {
	Stdout    string
	Stderr    string
	Output    string
	ExitCode  int64
	TimedOut  bool
	Truncated bool
}

type outputBudget struct {
	mu        sync.Mutex
	remaining int64
	truncated bool
}

type limitedCapture struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	budget *outputBudget
}

func (c *limitedCapture) Write(p []byte) (int, error) {
	if c.budget == nil {
		c.mu.Lock()
		_, _ = c.buf.Write(p)
		c.mu.Unlock()
		return len(p), nil
	}

	allowed := len(p)

	c.budget.mu.Lock()
	switch {
	case c.budget.remaining <= 0:
		allowed = 0
		c.budget.truncated = true
	case int64(allowed) > c.budget.remaining:
		allowed = int(c.budget.remaining)
		c.budget.remaining = 0
		c.budget.truncated = true
	default:
		c.budget.remaining -= int64(allowed)
	}
	c.budget.mu.Unlock()

	if allowed > 0 {
		c.mu.Lock()
		_, _ = c.buf.Write(p[:allowed])
		c.mu.Unlock()
	}

	return len(p), nil
}

func (c *limitedCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.buf.String()
}

func ExecuteCommand(
	cmdName string,
	cmdArgs []string,
	permissions Permissions,
) (
	ExecResult,
	error,
) {
	ctx, cancel := context.WithTimeout(context.Background(), permissions.EffectiveExecTimeout())
	defer cancel()

	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)

	budget := &outputBudget{remaining: permissions.EffectiveExecMaxOutputBytes()}
	stdout := &limitedCapture{budget: budget}
	stderr := &limitedCapture{budget: budget}

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()

	result := ExecResult{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		ExitCode:  0,
		Truncated: budget.truncated,
	}
	result.Output = result.Stdout + result.Stderr

	switch {
	case err == nil:
		return result, nil
	case ctx.Err() == context.DeadlineExceeded:
		result.TimedOut = true
		result.ExitCode = -1
		return result, nil
	default:
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			result.ExitCode = int64(exitErr.ExitCode())
			return result, nil
		}

		return ExecResult{}, err
	}
}

package native

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/runtimecap"
)

func TestVMNativeDropParity(t *testing.T) {
	root := findRepoRoot(t)
	sourcePath := filepath.Join(root, "tests", "native_drop_parity.bak")
	perms := runtimecap.Permissions{}

	vmExit, vmOut := runVMProgramFromFileWithOutput(t, sourcePath, perms)
	nativeExit, nativeOut := runNativeProgramFromFileWithOutput(t, sourcePath, perms)
	if vmExit != nativeExit || vmOut != nativeOut {
		t.Fatalf("VM/native drop mismatch: vm=%d %q native=%d %q", vmExit, vmOut, nativeExit, nativeOut)
	}
	if vmExit != 0 || !strings.Contains(vmOut, "drop-ok") {
		t.Fatalf("drop parity program failed: exit=%d output=%q", vmExit, vmOut)
	}
}

func TestVMNativeAllocatorReuseParity(t *testing.T) {
	root := findRepoRoot(t)
	sourcePath := filepath.Join(root, "tests", "native_drop_stress.bak")
	perms := runtimecap.Permissions{}

	vmExit, vmOut := runVMProgramFromFileWithOutput(t, sourcePath, perms)
	nativeExit, nativeOut := runNativeProgramFromFileWithOutput(t, sourcePath, perms)
	if vmExit != nativeExit || vmOut != nativeOut {
		t.Fatalf("VM/native reuse mismatch: vm=%d %q native=%d %q", vmExit, vmOut, nativeExit, nativeOut)
	}
	if vmExit != 0 || !strings.Contains(vmOut, "reuse-ok") {
		t.Fatalf("reuse program failed: exit=%d output=%q", vmExit, vmOut)
	}

	program := loadProgramFromFile(t, sourcePath)
	binary, err := BuildExecutableWithOptions(program, BuildOptions{AllocStats: true})
	if err != nil {
		t.Fatalf("native build with alloc stats: %v", err)
	}
	binPath := filepath.Join(t.TempDir(), "drop_stress.bin")
	if err := os.WriteFile(binPath, binary, 0755); err != nil {
		t.Fatalf("writing binary: %v", err)
	}

	cmd := exec.Command(binPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() < 0 {
			t.Fatalf("alloc stats run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
		}
		t.Fatalf("alloc stats exit %d\nstdout:\n%s\nstderr:\n%s", exitErr.ExitCode(), stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "reuse-ok") {
		t.Fatalf("alloc stats stdout = %q, want reuse-ok", stdout.String())
	}

	allocs, frees, reused, doubleFree := parseAllocStats(t, stderr.String())
	if frees == 0 || reused == 0 || doubleFree != 0 {
		t.Fatalf("allocator stats allocs=%d frees=%d reused=%d double_free=%d\nstderr:\n%s", allocs, frees, reused, doubleFree, stderr.String())
	}
}

func parseAllocStats(t *testing.T, stderr string) (allocs, frees, reused, doubleFree int) {
	t.Helper()
	line := ""
	for _, candidate := range strings.Split(stderr, "\n") {
		if strings.Contains(candidate, "allocs=") {
			line = candidate
		}
	}
	if line == "" {
		t.Fatalf("missing allocator stats in stderr:\n%s", stderr)
	}
	fields := strings.Fields(line)
	if len(fields) != 4 {
		t.Fatalf("allocator stats %q", line)
	}
	allocs = statValue(t, fields[0], "allocs")
	frees = statValue(t, fields[1], "frees")
	reused = statValue(t, fields[2], "reused")
	doubleFree = statValue(t, fields[3], "double_free")
	return allocs, frees, reused, doubleFree
}

func statValue(t *testing.T, field string, name string) int {
	t.Helper()
	prefix := name + "="
	if !strings.HasPrefix(field, prefix) {
		t.Fatalf("stat field %q, want %s", field, prefix)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(field, prefix))
	if err != nil {
		t.Fatalf("stat %s: %v", field, err)
	}
	return n
}

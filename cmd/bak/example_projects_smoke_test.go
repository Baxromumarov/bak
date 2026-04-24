package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func runBakCheckThroughHelper(t *testing.T, repoRoot, target string) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess", "--", "bak", "check", target)
	cmd.Env = append(os.Environ(), "BAK_TEST_MAIN_HELPER=1")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bak check failed for %s:\n%s", target, string(out))
	}
}

func TestExampleProjectsSmokeCheck(t *testing.T) {
	root := findRepoRootForGuardrail(t)
	targets := []string{
		filepath.Join(root, "example-projects", "http-api-server", "main.bak"),
		filepath.Join(root, "example-projects", "cli-tool", "main.bak"),
	}

	for _, target := range targets {
		name := filepath.Base(filepath.Dir(target))
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(target); err != nil {
				t.Skipf("example project is not present in this checkout: %s", target)
			}
			runBakCheckThroughHelper(t, root, target)
		})
	}
}

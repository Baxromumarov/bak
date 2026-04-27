package main

import (
	"path/filepath"
	"testing"

	"github.com/baxromumarov/bak/pkg/runtimecap"
)

func TestStdlibCoreExamplesCompileAndRun(t *testing.T) {
	root := findRepoRootForGuardrail(t)
	targets := []string{
		filepath.Join(root, "src", "std", "path", "path_test.bak"),
		filepath.Join(root, "tests", "std_path_ci_test.bak"),
		filepath.Join(root, "tests", "std_strings_ci_test.bak"),
		filepath.Join(root, "tests", "std_strings_path_enhancements_test.bak"),
		filepath.Join(root, "tests", "std_db_ci_test.bak"),
		filepath.Join(root, "tests", "std_db_docs_ci_test.bak"),
		filepath.Join(root, "src", "std", "db", "postgres_test.bak"),
	}

	for _, target := range targets {
		t.Run(filepath.Base(target), func(t *testing.T) {
			result := runTestFile(target, runtimecap.Permissions{}, "")
			if !result.Executed || !result.Passed {
				t.Fatalf("stdlib example failed: %s", target)
			}
		})
	}
}

func TestStdlibCollectionsExamplesCompileAndRun(t *testing.T) {
	root := findRepoRootForGuardrail(t)
	target := filepath.Join(root, "tests", "std_collections_ci_test.bak")
	result := runTestFile(target, runtimecap.Permissions{}, "")
	if !result.Executed || !result.Passed {
		t.Fatalf("stdlib collection example failed: %s", target)
	}
}

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var experimentalSurfacePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bunsafe\b`),
	regexp.MustCompile(`\bbox\?\b`),
	regexp.MustCompile(`\bbox\b`),
	regexp.MustCompile(`\btrait\b`),
	regexp.MustCompile(`\bstruct\s+[A-Za-z_][A-Za-z0-9_]*\s*<`),
	regexp.MustCompile(`\benum\s+[A-Za-z_][A-Za-z0-9_]*\s*<`),
	regexp.MustCompile(`\bfunc\s+[A-Za-z_][A-Za-z0-9_]*\s*<`),
	regexp.MustCompile(`\bimpl\s+[A-Za-z_][A-Za-z0-9_]*\s*<`),
}

var experimentalLabelPhrases = []string{
	"experimental",
	"outside frozen",
	"outside the frozen",
	"not part of the frozen",
	"not the frozen language contract",
	"not frozen",
}

func TestPublicDocsAndExamplesLabelExperimentalSurface(t *testing.T) {
	root := findRepoRootForGuardrail(t)
	targets := []string{
		filepath.Join(root, "README.md"),
		filepath.Join(root, "docs"),
		filepath.Join(root, "examples"),
		filepath.Join(root, "example-projects"),
	}

	var unlabeled []string
	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("stat %s: %v", target, err)
		}
		if !info.IsDir() {
			if fileNeedsExperimentalLabel(t, target) {
				unlabeled = append(unlabeled, trimRepoRoot(root, target))
			}
			continue
		}

		err = filepath.WalkDir(target, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			switch filepath.Ext(path) {
			case ".bak", ".md", ".txt":
			default:
				return nil
			}
			if fileNeedsExperimentalLabel(t, path) {
				unlabeled = append(unlabeled, trimRepoRoot(root, path))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", target, err)
		}
	}

	if len(unlabeled) > 0 {
		t.Fatalf("public docs/examples use experimental surface without a visible experimental-status note: %s", strings.Join(unlabeled, ", "))
	}
}

func TestPublicConformanceTestsLabelExperimentalSurface(t *testing.T) {
	root := findRepoRootForGuardrail(t)
	targets := []string{
		filepath.Join(root, "tests", "IMPORT_TESTS.md"),
	}

	matches, err := filepath.Glob(filepath.Join(root, "tests", "test_*.bak"))
	if err != nil {
		t.Fatalf("glob test fixtures: %v", err)
	}
	targets = append(targets, matches...)

	var unlabeled []string
	for _, target := range targets {
		if fileNeedsExperimentalLabel(t, target) {
			unlabeled = append(unlabeled, trimRepoRoot(root, target))
		}
	}

	if len(unlabeled) > 0 {
		t.Fatalf("public conformance tests use experimental surface without a visible experimental-status note: %s", strings.Join(unlabeled, ", "))
	}
}

func fileNeedsExperimentalLabel(t *testing.T, path string) bool {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	content := string(data)
	if !containsExperimentalSurface(content) {
		return false
	}

	return !hasExperimentalLabel(content)
}

func containsExperimentalSurface(content string) bool {
	for _, pattern := range experimentalSurfacePatterns {
		if pattern.MatchString(content) {
			return true
		}
	}
	return false
}

func hasExperimentalLabel(content string) bool {
	lines := strings.Split(content, "\n")
	if len(lines) > 60 {
		lines = lines[:60]
	}
	head := strings.ToLower(strings.Join(lines, "\n"))
	for _, phrase := range experimentalLabelPhrases {
		if strings.Contains(head, phrase) {
			return true
		}
	}
	return false
}

func findRepoRootForGuardrail(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root starting from %s", dir)
		}
		dir = parent
	}
}

func trimRepoRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

// Package packages provides package management and import resolution for the bak language.
package packages

import (
	"os"
	"path/filepath"
	"strings"
)

var fileExtn = ".bak"

// ResolveImportPath resolves an import path to an absolute file path.
// It handles "std/" prefix expansion, .bak extension appending, directory
// imports, and legacy github.com path resolution.
// Returns "" if the module cannot be found.
func ResolveImportPath(importPath string) string {
	return ResolveImportPathFrom(importPath, "")
}

// ResolveImportPathFrom resolves an import path relative to the importing
// source path when one is available. fromPath may refer to either a file or a
// directory; empty means "resolve from the current working directory only".
func ResolveImportPathFrom(importPath, fromPath string) string {
	// 1. Alias Expansion
	// "std/" prefix -> "src/std/"
	searchPath := importPath
	if strings.HasPrefix(importPath, "std/") {
		searchPath = filepath.Join("src", importPath)
	}

	// 2. Candidate Generation
	// We try:
	// a) The path exactly as is (e.g. valid relative path or absolute path)
	// b) The path + .bak (if not present)
	// c) The path + "/" + basename + .bak (directory import)
	candidates := []string{searchPath}

	if !strings.HasSuffix(searchPath, fileExtn) {
		candidates = append(candidates, searchPath+fileExtn)
		base := filepath.Base(searchPath)
		candidates = append(candidates, filepath.Join(searchPath, base+fileExtn))
	}

	// 3. Resolution
	cwd, _ := os.Getwd()
	baseDir := importBaseDir(fromPath)
	projectRoot := findProjectRoot(baseDir)
	if projectRoot == "" {
		projectRoot = findProjectRoot(cwd)
	}

	for _, path := range candidates {
		// For repository-rooted imports like src/std/..., resolve from project root.
		if projectRoot != "" {
			candidate := filepath.Join(projectRoot, path)
			if resolved := existingPath(candidate); resolved != "" {
				return resolved
			}
		}

		// Try relative to the importing file/directory first.
		if baseDir != "" {
			candidate := filepath.Join(baseDir, path)
			if resolved := existingPath(candidate); resolved != "" {
				return resolved
			}
		}

		// Try relative to CWD
		if resolved := existingPath(filepath.Join(cwd, path)); resolved != "" {
			return resolved
		}

		// If path was already absolute or relative to where we are running
		if resolved := existingPath(path); resolved != "" {
			return resolved
		}
	}

	// Fallback for legacy "simple" imports (e.g. import "fmt")
	if !strings.Contains(importPath, "/") {
		legacyPath := filepath.Join("src", "std", importPath, importPath+fileExtn)
		if baseDir != "" {
			if resolved := existingFilePath(filepath.Join(baseDir, legacyPath)); resolved != "" {
				return resolved
			}
		}
		if resolved := existingFilePath(filepath.Join(cwd, legacyPath)); resolved != "" {
			return resolved
		}
	}

	// Fallback for full github path (legacy)
	if after, ok := strings.CutPrefix(importPath, "github.com/baxromumarov/bak/"); ok {
		rest := after
		if rest != "" {
			base := filepath.Base(rest)
			legacyPath := filepath.Join("src", rest, base+fileExtn)
			if baseDir != "" {
				if resolved := existingFilePath(filepath.Join(baseDir, legacyPath)); resolved != "" {
					return resolved
				}
			}
			if resolved := existingFilePath(filepath.Join(cwd, legacyPath)); resolved != "" {
				return resolved
			}
		}
	}

	return ""
}

func importBaseDir(fromPath string) string {
	fromPath = strings.TrimSpace(fromPath)
	if fromPath == "" {
		return ""
	}
	if info, err := os.Stat(fromPath); err == nil {
		if info.IsDir() {
			abs, absErr := filepath.Abs(fromPath)
			if absErr == nil {
				return abs
			}
			return fromPath
		}
		abs, absErr := filepath.Abs(filepath.Dir(fromPath))
		if absErr == nil {
			return abs
		}
		return filepath.Dir(fromPath)
	}
	if strings.HasSuffix(fromPath, fileExtn) {
		abs, absErr := filepath.Abs(filepath.Dir(fromPath))
		if absErr == nil {
			return abs
		}
		return filepath.Dir(fromPath)
	}
	abs, absErr := filepath.Abs(fromPath)
	if absErr == nil {
		return abs
	}
	return fromPath
}

func existingPath(path string) string {
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func existingFilePath(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	abs, absErr := filepath.Abs(path)
	if absErr != nil {
		return path
	}
	return abs
}

func findProjectRoot(start string) string {
	start = strings.TrimSpace(start)
	if start == "" {
		return ""
	}

	current, err := filepath.Abs(start)
	if err != nil {
		current = start
	}

	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return ""
}

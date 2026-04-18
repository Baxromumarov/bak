// Package packages provides package management and import resolution for the bak language.
package packages

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveImportPath resolves an import path to an absolute file path.
// It handles "std/" prefix expansion, .bak extension appending, directory
// imports, and legacy github.com path resolution.
// Returns "" if the module cannot be found.
func ResolveImportPath(importPath string) string {
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

	if !strings.HasSuffix(searchPath, ".bak") {
		candidates = append(candidates, searchPath+".bak")
		base := filepath.Base(searchPath)
		candidates = append(candidates, filepath.Join(searchPath, base+".bak"))
	}

	// 3. Resolution
	cwd, _ := os.Getwd()

	for _, path := range candidates {
		// Try relative to CWD
		absPath := filepath.Join(cwd, path)
		if _, err := os.Stat(absPath); err == nil {
			return absPath
		}

		// If path was already absolute or relative to where we are running
		if _, err := os.Stat(path); err == nil {
			abs, _ := filepath.Abs(path)
			return abs
		}
	}

	// Fallback for legacy "simple" imports (e.g. import "fmt")
	if !strings.Contains(importPath, "/") {
		legacyPath := filepath.Join("src", "std", importPath, importPath+".bak")
		absLegacy := filepath.Join(cwd, legacyPath)
		if info, err := os.Stat(absLegacy); err == nil && !info.IsDir() {
			return absLegacy
		}
	}

	// Fallback for full github path (legacy)
	if after, ok :=strings.CutPrefix(importPath, "github.com/baxromumarov/bak/"); ok  {
		rest := after
		if rest != "" {
			base := filepath.Base(rest)
			legacyPath := filepath.Join("src", rest, base+".bak")
			absLegacy := filepath.Join(cwd, legacyPath)
			if _, err := os.Stat(absLegacy); err == nil {
				return absLegacy
			}
		}
	}

	return ""
}

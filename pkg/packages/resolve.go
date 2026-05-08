// Package packages provides package management and import resolution for the bak language.
package packages

import (
	"os"
	"path/filepath"
	"strings"
)

var fileExtn = ".bak"

// ImportResolution describes how an import path resolved, including every
// concrete file path that was considered. Resolved is empty when not found.
type ImportResolution struct {
	Requested string
	Resolved  string
	Tried     []string
	Hint      string
}

// ResolveImportPath resolves an import path to an absolute package path.
// Import paths are Go-like: "x" names a package path, not necessarily a file.
// The resolver still accepts direct .bak file imports for compatibility.
// Returns "" if the package cannot be found.
func ResolveImportPath(importPath string) string {
	return ResolveImportPathFrom(importPath, "")
}

// ResolveImportPathFrom resolves an import path relative to the importing
// source path when one is available. fromPath may refer to either a file or a
// directory; empty means "resolve from the current working directory only".
func ResolveImportPathFrom(importPath, fromPath string) string {
	return ResolveImportPathDetailedFrom(importPath, fromPath).Resolved
}

// ResolveImportPathDetailedFrom resolves an import path and preserves the
// concrete paths considered along the way for diagnostics.
func ResolveImportPathDetailedFrom(importPath, fromPath string) ImportResolution {
	result := ImportResolution{
		Requested: importPath,
		Hint:      importHint(importPath),
	}

	searchPath := importSearchPath(importPath)
	candidates := importCandidates(searchPath)

	cwd, _ := os.Getwd()
	baseDir := importBaseDir(fromPath)
	projectRoot := findProjectRoot(baseDir)
	if projectRoot == "" {
		projectRoot = findProjectRoot(cwd)
	}

	if resolved := resolveCandidates(&result, candidates, resolutionBases(importPath, projectRoot, baseDir, cwd)); resolved != "" {
		result.Resolved = resolved
		return result
	}

	// Standard-library shorthand, Go-style: import "fmt" resolves like std/fmt.
	if !strings.Contains(importPath, "/") {
		stdPath := filepath.Join("src", "std", filepath.FromSlash(importPath))
		stdCandidates := importCandidates(stdPath)
		if resolved := resolveCandidates(&result, stdCandidates, resolutionBases(stdPath, projectRoot, "", cwd)); resolved != "" {
			result.Resolved = resolved
			return result
		}
	}

	// Fallback for full github path (legacy)
	if after, ok := strings.CutPrefix(importPath, "github.com/baxromumarov/bak/"); ok {
		rest := after
		if rest != "" {
			base := filepath.Base(rest)
			legacyPath := filepath.Join("src", rest, base+fileExtn)
			if resolved := resolveCandidates(&result, []string{legacyPath}, resolutionBases(legacyPath, projectRoot, baseDir, cwd)); resolved != "" {
				result.Resolved = resolved
				return result
			}
		}
	}

	return result
}

func importSearchPath(importPath string) string {
	importPath = filepath.FromSlash(strings.TrimSpace(importPath))
	if strings.HasPrefix(importPath, "std"+string(filepath.Separator)) {
		return filepath.Join("src", importPath)
	}
	return importPath
}

func importCandidates(path string) []string {
	path = filepath.Clean(path)
	candidates := []string{path}
	if strings.HasSuffix(path, fileExtn) {
		return candidates
	}
	candidates = append(candidates, path+fileExtn)
	base := filepath.Base(path)
	if base != "." && base != string(filepath.Separator) {
		candidates = append(candidates, filepath.Join(path, base+fileExtn))
	}
	return candidates
}

func resolutionBases(importPath, projectRoot, baseDir, cwd string) []string {
	if filepath.IsAbs(importPath) {
		return []string{""}
	}
	if isRelativeImport(importPath) {
		return uniquePaths(baseDir, cwd, "")
	}
	return uniquePaths(projectRoot, cwd, baseDir, "")
}

func isRelativeImport(importPath string) bool {
	importPath = filepath.FromSlash(importPath)
	return strings.HasPrefix(importPath, "."+string(filepath.Separator)) ||
		strings.HasPrefix(importPath, ".."+string(filepath.Separator)) ||
		importPath == "." ||
		importPath == ".."
}

func uniquePaths(paths ...string) []string {
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			path = filepath.Clean(path)
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func resolveCandidates(result *ImportResolution, candidates, bases []string) string {
	for _, path := range candidates {
		for _, base := range bases {
			candidate := path
			if base != "" && !filepath.IsAbs(path) {
				candidate = filepath.Join(base, path)
			}
			result.addTried(candidate)
			if resolved := existingPackagePath(candidate); resolved != "" {
				return resolved
			}
		}
	}
	return ""
}

func (r *ImportResolution) addTried(path string) {
	path = filepath.Clean(path)
	for _, existing := range r.Tried {
		if existing == path {
			return
		}
	}
	r.Tried = append(r.Tried, path)
}

func importHint(importPath string) string {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return "use a non-empty import path"
	}
	if strings.HasPrefix(importPath, "std/") {
		return "use a standard-library package path such as std/path or std/collections/vec"
	}
	if !strings.Contains(importPath, "/") {
		return "use a package directory named x, a file named x.bak, or a standard-library package named x"
	}
	if !strings.HasSuffix(importPath, fileExtn) {
		return "use a package directory, a matching .bak file, or a directory containing <name>.bak"
	}
	return "check that the path is correct relative to the module root or the importing file"
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

func existingPackagePath(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		if !dirHasBakSource(path) {
			return ""
		}
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			return path
		}
		return abs
	}
	return existingFilePath(path)
}

func dirHasBakSource(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, fileExtn) {
			continue
		}
		if strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.bak") {
			continue
		}
		return true
	}
	return false
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

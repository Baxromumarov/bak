package test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

func collectTestFilesForTargets(paths []string) ([]string, []error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	seen := make(map[string]struct{})
	files := make([]string, 0)
	pathErrors := make([]error, 0)

	for _, path := range paths {
		targetFiles, err := collectTestFiles(path)
		if err != nil {
			pathErrors = append(pathErrors, fmt.Errorf("%s: %w", path, err))
			continue
		}
		if len(targetFiles) == 0 {
			pathErrors = append(pathErrors, fmt.Errorf("%s: no .bak files found", path))
			continue
		}
		for _, file := range targetFiles {
			clean := filepath.Clean(file)
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			files = append(files, clean)
		}
	}

	sort.Strings(files)
	return files, pathErrors
}

func collectTestFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}

	var testFiles []string
	var bakFiles []string
	walkErr := filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".bak") {
			bakFiles = append(bakFiles, p)
		}
		if strings.HasSuffix(p, "_test.bak") {
			testFiles = append(testFiles, p)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	if len(testFiles) > 0 {
		sort.Strings(testFiles)
		return testFiles, nil
	}

	sort.Strings(bakFiles)
	return bakFiles, nil
}

func filterTestFilesByPackage(files []string, packageFilters []string) ([]string, []error) {
	if len(packageFilters) == 0 {
		return files, nil
	}

	filterSet := make(map[string]struct{}, len(packageFilters))
	for _, name := range packageFilters {
		filterSet[name] = struct{}{}
	}

	filtered := make([]string, 0, len(files))
	errs := make([]error, 0)
	for _, file := range files {
		pkgName, err := packageNameFromFile(file)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		if _, ok := filterSet[pkgName]; ok {
			filtered = append(filtered, file)
		}
	}

	return filtered, errs
}

func packageNameFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	l := lexer.New(string(data))
	p := parser.New(l)
	p.SetFilename(path)
	program := p.ParseProgram()
	for _, stmt := range program.Statements {
		if pkgStmt, ok := stmt.(*ast.PackageStatement); ok && pkgStmt.Name != nil {
			return pkgStmt.Name.Value, nil
		}
	}
	if len(p.Errors()) > 0 {
		return "", fmt.Errorf("unable to resolve package name (%s)", p.Errors()[0])
	}
	return "", fmt.Errorf("missing package declaration")
}

func discoverTestFunctions(program *ast.Program) []testFunctionInfo {
	tests := make([]testFunctionInfo, 0)
	for _, stmt := range program.Statements {
		if fn, ok := stmt.(*ast.FunctionDecl); ok && strings.HasPrefix(fn.Name.Value, "test_") {
			tests = append(tests, testFunctionInfo{name: fn.Name.Value, arity: len(fn.Parameters)})
		}
	}
	return tests
}

func filterTestsByNamePattern(tests []testFunctionInfo, runPattern string) []testFunctionInfo {
	if runPattern == "" {
		return tests
	}
	filtered := make([]testFunctionInfo, 0, len(tests))
	for _, t := range tests {
		if strings.Contains(t.name, runPattern) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

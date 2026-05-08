package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/prelude"
)

// StdlibSymbol maps an exported symbol to its import path and alias.
type StdlibSymbol struct {
	Name       string // e.g., "StringBuilder"
	ImportPath string // e.g., "src/std/fmt/fmt.bak"
	Alias      string // e.g., "fmt"
	Kind       string // "func", "struct", "enum"
}

var (
	stdlibIndexOnce sync.Once
	stdlibIndex     map[string][]StdlibSymbol // symbol name -> possible imports
)

// getStdlibIndex returns the stdlib symbol index, building it lazily.
func getStdlibIndex() map[string][]StdlibSymbol {
	stdlibIndexOnce.Do(func() {
		stdlibIndex = buildStdlibIndex()
	})
	return stdlibIndex
}

func buildStdlibIndex() map[string][]StdlibSymbol {
	index := make(map[string][]StdlibSymbol)
	stdPath := prelude.GetStdLibPath()
	baseDir := filepath.Dir(stdPath)

	// Map of module directory -> alias name
	modules := map[string]string{
		"fmt":         "fmt",
		"strings":     "strings",
		"io":          "io",
		"os":          "os",
		"fs":          "fs",
		"math":        "math",
		"time":        "time",
		"rand":        "rand",
		"sort":        "sort",
		"bytes":       "bytes",
		"strconv":     "strconv",
		"filepath":    "filepath",
		"path":        "path",
		"net":         "net",
		"http":        "http",
		"json":        "json",
		"log":         "log",
		"sync":        "sync",
		"thread":      "thread",
		"crypto":      "crypto",
		"errors":      "errors",
		"encoding":    "encoding",
		"test":        "test",
		"collections": "collections",
		"any":         "any",
		"db":          "db",
	}

	for dir, alias := range modules {
		dirPath := filepath.Join(stdPath, dir)
		info, err := os.Stat(dirPath)
		if err != nil {
			continue
		}

		var bakFiles []string
		if info.IsDir() {
			entries, err := os.ReadDir(dirPath)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".bak") {
					bakFiles = append(bakFiles, filepath.Join(dirPath, e.Name()))
				}
			}
		} else if strings.HasSuffix(dirPath, ".bak") {
			bakFiles = append(bakFiles, dirPath)
		}

		for _, bakFile := range bakFiles {
			data, err := os.ReadFile(bakFile)
			if err != nil {
				continue
			}
			l := lexer.New(string(data))
			p := parser.New(l)
			prog := p.ParseProgram()
			if len(p.Errors()) > 0 {
				continue
			}

			importPath := canonicalStdlibImportPath(baseDir, bakFile)
			pkgName := packageNameForProgram(prog, alias)

			// Helper to avoid repeating the append logic for every declaration kind.
			record := func(name, kind string) {
				if name == "" {
					return
				}
				index[name] = append(index[name], StdlibSymbol{
					Name:       name,
					ImportPath: importPath,
					Alias:      pkgName,
					Kind:       kind,
				})
			}

			for _, stmt := range prog.Statements {
				switch s := stmt.(type) {
				case *ast.FunctionDecl:
					if s.Visibility == ast.Public && s.Name != nil {
						record(s.Name.Value, "func")
					}
				case *ast.StructDecl:
					if s.Visibility == ast.Public && s.Name != nil {
						record(s.Name.Value, "struct")
					}
				case *ast.EnumDecl:
					if s.Visibility == ast.Public && s.Name != nil {
						record(s.Name.Value, "enum")
					}
				}
			}
		}
	}

	return index
}

func canonicalStdlibImportPath(baseDir, bakFile string) string {
	dir := filepath.Dir(bakFile)
	if _, err := packages.ParseProgram(dir); err == nil {
		if rel, relErr := filepath.Rel(baseDir, dir); relErr == nil {
			return filepath.ToSlash(rel)
		}
	}
	if rel, err := filepath.Rel(baseDir, bakFile); err == nil {
		return strings.TrimSuffix(filepath.ToSlash(rel), ".bak")
	}
	return strings.TrimSuffix(filepath.ToSlash(bakFile), ".bak")
}

func packageNameForProgram(prog *ast.Program, fallback string) string {
	if prog == nil {
		return fallback
	}
	for _, stmt := range prog.Statements {
		if ps, ok := stmt.(*ast.PackageStatement); ok && ps.Name != nil && ps.Name.Value != "" {
			return ps.Name.Value
		}
	}
	return fallback
}

// lookupStdlibSymbol returns all stdlib symbols matching the given name.
func lookupStdlibSymbol(name string) []StdlibSymbol {
	idx := getStdlibIndex()
	return idx[name]
}

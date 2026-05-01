// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"maps"
	"path/filepath"

	"github.com/baxromumarov/bak/pkg/packages"
)

func InvalidatePackage(path string) {
	if path == "" {
		return
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	packages.GlobalRegistry.RemovePackage(absPath)
}

// ResetCache clears legacy package registry state. TypeChecker import caches
// are now scoped to each checker instance.
func ResetCache() {
	packages.GlobalRegistry.Reset()
}

func (tc *TypeChecker) markImportedSymbolUsed(alias, name string) {
	if alias == "" || name == "" {
		return
	}
	tc.markImportUsed(alias)

	importPath := tc.importedPkgPaths[alias]
	if importPath == "" {
		// fallback: try to find alias in importAliases map
		for path, a := range tc.importAliases {
			if a == alias {
				importPath = path
				break
			}
		}
	}
	if importPath == "" {
		return
	}

	if pkg, exists := tc.registry.GetPackage(importPath); exists {
		pkg.MarkUsed(name)
	}

	if modTC, ok := tc.packageCheckers[importPath]; ok {
		if modTC.env != nil {
			modTC.env.MarkUsed(name)
		}
	}
}
func (tc *TypeChecker) markImportUsed(alias string) {
	if alias == "" {
		return
	}
	tc.usedImports[alias] = true
}
func (tc *TypeChecker) finalizeImportedModules() {
	// Finalize unused checks for any imported modules we've loaded.
	// This runs their unused-element checks now that importers have had
	// a chance to mark used exported symbols.
	packageCheckersSnapshot := make(map[string]*TypeChecker, len(tc.packageCheckers))
	maps.Copy(packageCheckersSnapshot, tc.packageCheckers)
	for importPath, modTC := range packageCheckersSnapshot {
		if modTC == nil || modTC.finalized {
			continue
		}
		modTC.checkUnusedElements()
		modTC.finalized = true
		// Seed back any used marks into the package registry as well.
		if pkg, exists := tc.registry.GetPackage(importPath); exists {
			for name := range modTC.env.used {
				pkg.MarkUsed(name)
			}
		}
	}
}

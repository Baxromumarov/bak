// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"maps"
	"path/filepath"
	"sync"

	"github.com/baxromumarov/bak/pkg/packages"
)

var loadedPackageCheckers = make(map[string]*TypeChecker)

// packageCheckersMu guards concurrent access to loadedPackageCheckers.
var packageCheckersMu sync.RWMutex

func InvalidatePackage(path string) {
	if path == "" {
		return
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	packageCheckersMu.Lock()
	delete(loadedPackageCheckers, absPath)
	packageCheckersMu.Unlock()
	packages.GlobalRegistry.RemovePackage(absPath)
}

// ResetCache clears all cached typechecker instances.
func ResetCache() {
	packageCheckersMu.Lock()
	clear(loadedPackageCheckers)
	packageCheckersMu.Unlock()
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

	if pkg, exists := packages.GlobalRegistry.GetPackage(importPath); exists {
		pkg.MarkUsed(name)
	}

	packageCheckersMu.RLock()
	modTC, ok := loadedPackageCheckers[importPath]
	packageCheckersMu.RUnlock()
	if ok {
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
	packageCheckersMu.RLock()
	packageCheckersSnapshot := make(map[string]*TypeChecker, len(loadedPackageCheckers))
	maps.Copy(packageCheckersSnapshot, loadedPackageCheckers)
	packageCheckersMu.RUnlock()
	for importPath, modTC := range packageCheckersSnapshot {
		if modTC == nil || modTC.finalized {
			continue
		}
		modTC.checkUnusedElements()
		modTC.finalized = true
		// Seed back any used marks into the package registry as well.
		if pkg, exists := packages.GlobalRegistry.GetPackage(importPath); exists {
			for name := range modTC.env.used {
				pkg.MarkUsed(name)
			}
		}
	}
}

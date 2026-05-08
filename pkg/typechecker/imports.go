package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (tc *TypeChecker) checkPackageStatement(ps *ast.PackageStatement) {
	if ps.Name != nil {
		tc.currentPkgName = ps.Name.Value
	}
}

func (tc *TypeChecker) checkImportStatement(is *ast.ImportStatement) {
	resolution := packages.ResolveImportPathDetailedFrom(is.Path, tc.currentPkgPath)
	if resolution.Resolved == "" {
		tc.emitImportNotFound(is, resolution)
		return
	}
	importPath := resolution.Resolved

	if tc.currentPkgPath != "" {
		tc.registry.RecordResolvedImport(tc.currentPkgPath, importPath)
	}

	// Check for cyclic imports
	if tc.currentPkgPath != "" {
		visited := make(map[string]bool)
		visited[tc.currentPkgPath] = true

		if err := tc.registry.CheckCyclicImport(
			tc.currentPkgPath,
			importPath,
			visited,
		); err != nil {
			tc.addErrorWithHelp(is.Token.Line, is.Token.Column, "check for a circular dependency chain or simplify the module graph", err.Error())
			return
		}
	}

	// Determine the alias to use for this import
	alias := is.Alias
	if alias == "" {
		// Use the package name from the path (last component)
		alias = extractPackageName(importPath)
	}

	// Store the alias mapping
	tc.importAliases[importPath] = alias
	// Also store reverse mapping: alias -> importPath
	tc.importedPkgPaths[alias] = importPath
	if alias != "" {
		tc.imports[alias] = ImportInfo{
			Path:   is.Path,
			Alias:  alias,
			Line:   is.Token.Line,
			Column: is.Token.Column,
		}
		if strings.HasPrefix(alias, "_") {
			tc.usedImports[alias] = true
		}
	}

	// Try to get symbols from the package registry
	pkg, exists := tc.registry.GetPackage(importPath)
	if !exists {
		// Recursive loading: If package not found, load and check it
		modProg, err := packages.ParseProgram(importPath)
		if err != nil {
			tc.addErrorWithHelp(is.Token.Line, is.Token.Column, "check the import path exists and is accessible", strfmt.Named("cannot read import file: {err}", "Err", err))
			return
		}

		// Register the package
		pkgName := extractPackageName(importPath)
		pkg = packages.NewPackage(pkgName, importPath, modProg)
		tc.registry.RegisterPackage(pkg)

		// Type check the imported module recursively
		// We use a new TypeChecker for the module and suppress its unused-symbol warnings
		modTC := NewWithPathAndRegistry(importPath, tc.registry)
		modTC.packageCheckers = tc.packageCheckers
		modTC.suppressUnused = true
		modErrors := modTC.Check(modProg)
		// Store the module TypeChecker so we can finalize unused checks later.
		tc.packageCheckers[importPath] = modTC

		// Propagate any parse/type errors from the module
		if len(modErrors) > 0 {
			for _, modErr := range modErrors {
				tc.addErrorWithHelp(is.Token.Line, is.Token.Column, "fix errors in the imported module before running this program", strfmt.Named("error in module {importPath}: {modErr}", "ImportPath", importPath, "ModErr", modErr))
			}
			return
		}

		// Seed the package-level used map with what the module checker already considered used
		// so that later importers can add to it.
		for name := range modTC.env.used {
			pkg.MarkUsed(name)
		}
	}

	// Retrieve potentially newly created package
	pkg, exists = tc.registry.GetPackage(importPath)
	if exists {
		// Store the public symbols for this import
		publicSymbols := pkg.GetPublicSymbols()
		tc.importedSymbols[alias] = publicSymbols
	}
}

func (tc *TypeChecker) checkImportBlock(ib *ast.ImportBlock) {
	for _, imp := range ib.Imports {
		tc.checkImportStatement(imp)
	}
}

// extractPackageName extracts the package name from an import path
func extractPackageName(path string) string {
	// Remove quotes if present
	path = strings.Trim(path, "\"")

	// Get the last path component
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]

	// Remove .bak extension if present
	name = strings.TrimSuffix(name, ".bak")

	return name
}

func (tc *TypeChecker) emitImportNotFound(is *ast.ImportStatement, resolution packages.ImportResolution) {
	notes := []diagnostics.Note{
		{
			Message: strfmt.Named("requested by {file}", "File", tc.currentPkgPath),
			File:    tc.currentPkgPath,
			Line:    is.Token.Line,
			Column:  is.Token.Column,
		},
	}
	for _, tried := range resolution.Tried {
		notes = append(notes, diagnostics.Note{
			Message: strfmt.Named("tried {path}", "Path", tried),
			File:    tried,
		})
	}

	diag := tc.baseDiagnostic(
		diagnostics.ErrImportNotFound,
		ast.Position{Line: is.Token.Line, Column: is.Token.Column},
		strfmt.Named("import not found: '{importPath}'", "ImportPath", resolution.Requested),
	)
	diag.Help = resolution.Hint
	diag.Notes = notes
	tc.emitError(diag)
}

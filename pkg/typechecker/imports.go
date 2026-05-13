package typechecker

import (
	"errors"
	"path/filepath"
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
	if tc.checkCanceled() {
		return
	}
	resolution := packages.ResolveImportPathDetailedFromRoot(is.Path, tc.currentPkgPath, tc.registry.ProjectRoot())
	if resolution.Resolved == "" {
		tc.emitImportNotFound(is, resolution)
		return
	}
	importPath := resolution.Resolved

	if sameResolvedImportPath(tc.currentPkgPath, importPath) {
		diag := tc.baseDiagnostic(
			diagnostics.ErrSelfImport,
			ast.Position{Line: is.Token.Line, Column: is.Token.Column},
			strfmt.Named("package cannot import itself: {importPath}", "ImportPath", is.Path),
		)
		diag.Help = "remove the self import or move shared code to another package"
		tc.emitError(diag)
		return
	}

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
			tc.emitImportCycleError(is, err)
			return
		}
	}

	// Try to get symbols from the package registry
	pkg, exists := tc.registry.GetPackage(importPath)
	if !exists {
		// Recursive loading: If package not found, load and check it
		modProg, err := packages.ParseProgramContext(tc.ctx, importPath)
		if err != nil {
			if tc.checkCanceled() {
				return
			}
			tc.addErrorWithHelp(is.Token.Line, is.Token.Column, "check the import path exists and is accessible", strfmt.Named("cannot read import file: {err}", "Err", err))
			return
		}
		if tc.checkCanceled() {
			return
		}

		// Register the package
		pkgName := packageNameFromProgram(modProg)
		if pkgName == "" {
			pkgName = extractPackageName(importPath)
		}
		pkg = packages.NewPackage(pkgName, importPath, modProg)
		tc.registry.RegisterPackage(pkg)

		// Type check the imported module recursively
		// We use a new TypeChecker for the module and suppress its unused-symbol warnings
		modTC := NewWithPathAndRegistry(importPath, tc.registry)
		modTC.SetContext(tc.ctx)
		modTC.packageCheckers = tc.packageCheckers
		modTC.suppressUnused = true
		modErrors := modTC.Check(modProg)
		// Store the module TypeChecker so we can finalize unused checks later.
		tc.packageCheckers[importPath] = modTC

		// Propagate any parse/type errors from the module
		if len(modErrors) > 0 {
			tc.emitImportedModuleErrors(is, importPath, modErrors, modTC.GetErrors())
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
		// Determine the alias to use for this import. Like Go, an unaliased
		// import binds to the imported file's declared package name.
		alias := is.Alias
		if alias == "" {
			alias = pkg.Name
		}
		if alias == "" {
			alias = extractPackageName(importPath)
		}

		importFile := is.Token.Filename
		if importFile == "" {
			importFile = tc.currentPkgPath
		}

		if prior, ok := tc.imports[alias]; ok && sameResolvedImportPath(prior.File, importFile) {
			diag := tc.baseDiagnostic(
				diagnostics.ErrDuplicateImport,
				ast.Position{Line: is.Token.Line, Column: is.Token.Column},
				strfmt.Named("duplicate import alias: '{alias}'", "Alias", alias),
			)
			diag.Help = "use one import per alias or rename one import"
			diag.Notes = []diagnostics.Note{{
				Message: strfmt.Named("first imported here as '{alias}'", "Alias", alias),
				File:    prior.File,
				Line:    prior.Line,
				Column:  prior.Column,
			}}
			tc.emitError(diag)
			return
		}

		tc.importAliases[importPath] = alias
		tc.importedPkgPaths[alias] = importPath
		if alias != "" {
			tc.imports[alias] = ImportInfo{
				Path:   is.Path,
				Alias:  alias,
				File:   importFile,
				Line:   is.Token.Line,
				Column: is.Token.Column,
			}
			if strings.HasPrefix(alias, "_") {
				tc.usedImports[alias] = true
			}
		}

		// Store the public symbols for this import
		publicSymbols := pkg.GetPublicSymbols()
		tc.importedSymbols[alias] = publicSymbols
	}
}

func sameResolvedImportPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aa, err := filepath.Abs(a)
	if err == nil {
		a = aa
	}
	bb, err := filepath.Abs(b)
	if err == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func (tc *TypeChecker) checkImportBlock(ib *ast.ImportBlock) {
	for _, imp := range ib.Imports {
		if tc.checkCanceled() {
			return
		}
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

func packageNameFromProgram(program *ast.Program) string {
	if program == nil {
		return ""
	}
	for _, stmt := range program.Statements {
		if ps, ok := stmt.(*ast.PackageStatement); ok && ps.Name != nil {
			return ps.Name.Value
		}
	}
	return ""
}

func (tc *TypeChecker) emitImportedModuleErrors(
	is *ast.ImportStatement,
	importPath string,
	formatted []string,
	structured []TypeError,
) {
	help := "fix errors in the imported module before running this program"
	if len(structured) == 0 {
		for _, modErr := range formatted {
			tc.addErrorWithHelp(
				is.Token.Line,
				is.Token.Column,
				help,
				strfmt.Named(
					"error in module {importPath}: {modErr}",
					"ImportPath", importPath,
					"ModErr", modErr,
				),
			)
		}
		return
	}

	for _, modErr := range structured {
		code := diagnostics.ErrImportedModule
		if modErr.Code == diagnostics.ErrImportCycle {
			code = diagnostics.ErrImportCycle
		}
		diag := tc.baseDiagnostic(
			code,
			ast.Position{Line: is.Token.Line, Column: is.Token.Column},
			strfmt.Named(
				"error in module {importPath}: {message}",
				"ImportPath", importPath,
				"Message", modErr.Message,
			),
		)
		diag.Help = help
		if modErr.File != "" || modErr.Line > 0 || modErr.Column > 0 {
			diag.Notes = append(diag.Notes, diagnostics.Note{
				Message: "imported module error originates here",
				File:    modErr.File,
				Line:    modErr.Line,
				Column:  modErr.Column,
			})
		}
		diag.Notes = append(diag.Notes, modErr.Notes...)
		tc.emitError(diag)
	}
}

func (tc *TypeChecker) emitImportCycleError(is *ast.ImportStatement, err error) {
	diag := tc.baseDiagnostic(
		diagnostics.ErrImportCycle,
		ast.Position{Line: is.Token.Line, Column: is.Token.Column},
		err.Error(),
	)
	diag.Help = "check for a circular dependency chain or simplify the module graph"

	var cycleErr *packages.ImportCycleError
	if errors.As(err, &cycleErr) {
		diag.Notes = append(diag.Notes, tc.importCycleNotes(is, cycleErr.Chain)...)
	}
	tc.emitError(diag)
}

func (tc *TypeChecker) importCycleNotes(
	currentImport *ast.ImportStatement,
	chain []string,
) []diagnostics.Note {
	notes := make([]diagnostics.Note, 0, len(chain))
	seen := make(map[string]struct{}, len(chain))
	for i := 0; i+1 < len(chain); i++ {
		fromPath := chain[i]
		toPath := chain[i+1]
		note := tc.importEdgeNote(currentImport, fromPath, toPath)
		key := strfmt.S(note.File, ":", note.Line, ":", note.Column, ":", note.Message)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		notes = append(notes, note)
	}
	return notes
}

func (tc *TypeChecker) importEdgeNote(
	currentImport *ast.ImportStatement,
	fromPath,
	toPath string,
) diagnostics.Note {
	if currentImport != nil && sameResolvedImportPath(fromPath, tc.currentPkgPath) {
		if currentResolved := packages.ResolveImportPathDetailedFromRoot(
			currentImport.Path,
			tc.currentPkgPath,
			tc.registry.ProjectRoot(),
		).Resolved; sameResolvedImportPath(currentResolved, toPath) {
			return importStatementNote(currentImport, toPath)
		}
	}

	if pkg, ok := tc.registry.GetPackage(fromPath); ok && pkg != nil {
		if imp := tc.findImportEdgeStatement(pkg.Program, fromPath, toPath); imp != nil {
			return importStatementNote(imp, toPath)
		}
	}

	return diagnostics.Note{
		Message: strfmt.Named("cycle includes package {path}", "Path", fromPath),
		File:    fromPath,
		Line:    1,
		Column:  1,
	}
}

func (tc *TypeChecker) findImportEdgeStatement(
	program *ast.Program,
	fromPath,
	toPath string,
) *ast.ImportStatement {
	for _, imp := range importStatementsInProgram(program) {
		basePath := imp.Token.Filename
		if basePath == "" {
			basePath = fromPath
		}
		resolved := packages.ResolveImportPathDetailedFromRoot(
			imp.Path,
			basePath,
			tc.registry.ProjectRoot(),
		).Resolved
		if sameResolvedImportPath(resolved, toPath) {
			return imp
		}
	}
	return nil
}

func importStatementsInProgram(program *ast.Program) []*ast.ImportStatement {
	if program == nil {
		return nil
	}
	var imports []*ast.ImportStatement
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			if s != nil {
				imports = append(imports, s)
			}
		case *ast.ImportBlock:
			for _, imp := range s.Imports {
				if imp != nil {
					imports = append(imports, imp)
				}
			}
		}
	}
	return imports
}

func importStatementNote(imp *ast.ImportStatement, toPath string) diagnostics.Note {
	file := imp.Token.Filename
	return diagnostics.Note{
		Message: strfmt.Named("imports {path} here", "Path", toPath),
		File:    file,
		Line:    imp.Token.Line,
		Column:  imp.Token.Column,
	}
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

package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/token"
)

func (tc *TypeChecker) privateImportedSymbol(pkgAlias, name string) (*packages.Symbol, bool) {
	pkgPath, ok := tc.importedPkgPaths[pkgAlias]
	if !ok {
		return nil, false
	}
	if tc.registry.SamePackageScope(pkgPath, tc.currentPkgPath) {
		return nil, false
	}

	sym, ok := tc.registry.GetAnySymbolFromPackage(pkgPath, name)
	if !ok || sym.Visibility == ast.Public {
		return nil, false
	}
	return sym, true
}

func (tc *TypeChecker) emitPrivateImportedSymbolAt(pkgAlias, name string, pos ast.Position) bool {
	sym, ok := tc.privateImportedSymbol(pkgAlias, name)
	if !ok {
		return false
	}

	tc.markImportUsed(pkgAlias)
	nameTok := privateSymbolNameToken(sym)
	declTok := privateSymbolDeclToken(sym)
	declFile := nameTok.Filename
	if declFile == "" {
		declFile = declTok.Filename
	}
	if declFile == "" {
		declFile = tc.importedPkgPaths[pkgAlias]
	}

	notes := []diagnostics.Note{}
	if nameTok.Line > 0 {
		notes = append(notes, diagnostics.Note{
			Message: "declared here",
			File:    declFile,
			Line:    nameTok.Line,
			Column:  nameTok.Column,
		})
	}
	fixes := []diagnostics.Fix{}
	if declTok.Line > 0 && declTok.Column > 0 {
		fixes = append(fixes, diagnostics.Fix{
			Title:       strfmt.Named("Make {name} public", "Name", name),
			File:        declFile,
			Replacement: "pub ",
			StartLine:   declTok.Line,
			StartColumn: declTok.Column,
			EndLine:     declTok.Line,
			EndColumn:   declTok.Column,
		})
	}

	tc.emitError(diagnostics.Diagnostic{
		Code:   diagnostics.ErrPrivateImport,
		Level:  diagnostics.LevelError,
		Line:   pos.Line,
		Column: pos.Column,
		File:   tc.currentPkgPath,
		Message: strfmt.Named(
			"{kind} '{alias}.{name}' is private; export it with pub if it should be accessible from other packages",
			"Kind", sym.Kind.String(),
			"Alias", pkgAlias,
			"Name", name,
		),
		Help:  "add pub to the declaration in the imported module or use a public wrapper",
		Notes: notes,
		Fixes: fixes,
	})
	return true
}

func privateSymbolDeclToken(sym *packages.Symbol) token.Token {
	if sym == nil || sym.Node == nil {
		return token.Token{}
	}
	if withToken, ok := sym.Node.(interface{ GetToken() token.Token }); ok {
		return withToken.GetToken()
	}
	return token.Token{}
}

func privateSymbolNameToken(sym *packages.Symbol) token.Token {
	if sym == nil || sym.Node == nil {
		return token.Token{}
	}
	switch n := sym.Node.(type) {
	case *ast.FunctionDecl:
		if n.Name != nil {
			return n.Name.Token
		}
	case *ast.StructDecl:
		if n.Name != nil {
			return n.Name.Token
		}
	case *ast.EnumDecl:
		if n.Name != nil {
			return n.Name.Token
		}
	case *ast.ConstStatement:
		if n.Name != nil {
			return n.Name.Token
		}
	case *ast.TypeDecl:
		if n.Name != nil {
			return n.Name.Token
		}
	case *ast.AliasDecl:
		if n.Name != nil {
			return n.Name.Token
		}
	}
	return privateSymbolDeclToken(sym)
}

func isImportedTypeKind(kind packages.SymbolKind) bool {
	switch kind {
	case packages.SymbolStruct,
		packages.SymbolEnum,
		packages.SymbolType,
		packages.SymbolAlias:
		return true
	default:
		return false
	}
}

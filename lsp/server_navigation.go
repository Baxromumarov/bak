package main

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/prelude"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/token"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

// LSP SymbolKind constants
const (
	SymbolKindFile = iota + 1
	SymbolKindModule
	SymbolKindNamespace
	SymbolKindPackage
	SymbolKindClass
	SymbolKindMethod
	SymbolKindProperty
	SymbolKindField
	SymbolKindConstructor
	SymbolKindEnum
	SymbolKindInterface
	SymbolKindFunction
	SymbolKindVariable
	SymbolKindConstant
	SymbolKindString
	SymbolKindNumber
	SymbolKindBoolean
	SymbolKindArray
	SymbolKindObject
	SymbolKindKey
	SymbolKindNull
	SymbolKindEnumMember
	SymbolKindStruct
	SymbolKindEvent
	SymbolKindOperator
	SymbolKindTypeAlias
)

func (s *Server) documentText(uri string) string {
	if text, ok := s.document(uri); ok && text != "" {
		return text
	}
	data, err := os.ReadFile(uriToPath(uri))
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *Server) importedModuleIndex(result *AnalysisResult, originURI, alias string) *FileIndex {
	if result == nil || result.Imports == nil {
		return nil
	}
	importPath, ok := result.Imports[alias]
	if !ok {
		return nil
	}
	path := s.resolveImportPath(uriToPath(originURI), importPath)
	if path == "" {
		return nil
	}
	return s.getOrIndexFile(path)
}

func (s *Server) lookupImportedSymbol(
	result *AnalysisResult,
	originURI,
	alias,
	symbolName string,
) (Location, bool) {
	modIndex := s.importedModuleIndex(result, originURI, alias)
	if modIndex == nil {
		return Location{}, false
	}
	sym, ok := modIndex.Symbols[symbolName]
	if !ok {
		return Location{}, false
	}
	return sym.Location, true
}

func (s *Server) lookupSymbolInImports(
	result *AnalysisResult,
	originURI,
	key,
	fallback string,
) (Location, bool) {
	if result == nil || result.Imports == nil {
		return Location{}, false
	}
	for _, importPath := range result.Imports {
		path := s.resolveImportPath(uriToPath(originURI), importPath)
		if path == "" {
			continue
		}
		modIndex := s.getOrIndexFile(path)
		if modIndex == nil {
			continue
		}
		if sym, ok := modIndex.Symbols[key]; ok {
			return sym.Location, true
		}
		if fallback != "" {
			if sym, ok := modIndex.Symbols[fallback]; ok {
				return sym.Location, true
			}
		}
	}
	return Location{}, false
}

func (s *Server) preludePaths(includeBuiltins bool) []string {
	stdLibPath := prelude.GetStdLibPath()
	paths := make([]string, 0, 4)
	if includeBuiltins {
		paths = append(paths, filepath.Join(stdLibPath, "builtins.bak"))
	}
	paths = append(paths,
		filepath.Join(stdLibPath, "collections", "vec.bak"),
		filepath.Join(stdLibPath, "collections", "hashmap.bak"),
		filepath.Join(stdLibPath, "result.bak"),
	)
	return paths
}

func (s *Server) lookupPreludeSymbol(key, method string, includeBuiltins bool) (Location, bool) {
	for _, path := range s.preludePaths(includeBuiltins) {
		modIndex := s.getOrIndexFile(path)
		if modIndex == nil {
			continue
		}
		if sym, ok := modIndex.Symbols[key]; ok {
			return sym.Location, true
		}
		if method == "" {
			continue
		}
		log.Printf("DEBUG(ID): Searching for %s in %s (found %d symbols)\n", key, path, len(modIndex.Symbols))
		if sym, ok := modIndex.Symbols[method]; ok {
			return sym.Location, true
		}
		if strings.Contains(path, "builtins.bak") {
			for k := range modIndex.Symbols {
				log.Printf("DEBUG(ID): Found symbol key: %s\n", k)
			}
		}
	}
	return Location{}, false
}

func (s *Server) definitionFromRefs(result *AnalysisResult, node ast.Node) []Location {
	if result == nil || result.RefByPos == nil || result.Defs == nil {
		return nil
	}
	key := nodeRefKey(node)
	if key == "" {
		return nil
	}
	id, ok := result.RefByPos[key]
	if !ok {
		return nil
	}
	defLoc, ok := result.Defs[id]
	if !ok {
		return nil
	}
	return []Location{defLoc}
}

func (s *Server) definitionForIdentifier(
	result *AnalysisResult,
	originURI string,
	position Position,
	ident *ast.Identifier,
) []Location {
	if ident == nil {
		return nil
	}
	if parts := strings.SplitN(ident.Value, ".", 2); len(parts) == 2 {
		if loc, ok := s.lookupImportedSymbol(result, originURI, parts[0], parts[1]); ok {
			return []Location{loc}
		}
	}

	if sym, ok := result.Index.Symbols[ident.Value]; ok {
		return []Location{sym.Location}
	}

	text := s.documentText(originURI)
	if text == "" {
		return nil
	}
	lineText := lineAt(text, position.Line)
	word, start := wordAt(lineText, position.Character)
	if word != ident.Value || start <= 0 || lineText[start-1] != '.' {
		return nil
	}

	qualifier := qualifierBefore(lineText, start-1)
	if qualifier == "" {
		return nil
	}

	if loc, ok := s.lookupImportedSymbol(result, originURI, qualifier, word); ok {
		return []Location{loc}
	}

	if result.TC == nil {
		return nil
	}
	qualCol := start - len(qualifier)
	if qualCol <= 0 {
		return nil
	}
	qNode := findNode(result.AST, position.Line+1, qualCol)
	if qNode == nil {
		return nil
	}
	t := result.TC.GetNodeType(qNode)
	if t == "" {
		return nil
	}
	t = resolveAliasTypeString(result, t)
	key := mapBuiltinType(baseTypeName(t)) + "." + word
	if sym, ok := result.Index.Symbols[key]; ok {
		return []Location{sym.Location}
	}
	if loc, ok := s.lookupSymbolInImports(result, originURI, key, ""); ok {
		return []Location{loc}
	}
	if loc, ok := s.lookupPreludeSymbol(key, "", false); ok {
		return []Location{loc}
	}
	return nil
}

func (s *Server) definitionForFieldAccess(
	result *AnalysisResult,
	originURI string,
	fieldAccess *ast.FieldAccessExpression,
) []Location {
	if fieldAccess == nil {
		return nil
	}
	if ident, ok := fieldAccess.Object.(*ast.Identifier); ok {
		if loc, ok := s.lookupImportedSymbol(
			result,
			originURI,
			ident.Value,
			fieldAccess.Field.Value,
		); ok {
			return []Location{loc}
		}
	}
	if result.TC == nil {
		return nil
	}
	if t := result.TC.GetNodeType(fieldAccess.Object); t != "" {
		t = resolveAliasTypeString(result, t)
		key := baseTypeName(t) + "." + fieldAccess.Field.Value
		if sym, ok := result.Index.Symbols[key]; ok {
			return []Location{sym.Location}
		}
	}
	return nil
}

func (s *Server) definitionForMethodCall(
	result *AnalysisResult,
	originURI string,
	methodCall *ast.MethodCallExpression,
) []Location {
	if methodCall == nil {
		return nil
	}
	if ident, ok := methodCall.Object.(*ast.Identifier); ok {
		if loc, ok := s.lookupImportedSymbol(
			result,
			originURI,
			ident.Value,
			methodCall.Method.Value,
		); ok {
			return []Location{loc}
		}
	}
	if result.TC == nil {
		return nil
	}
	t := result.TC.GetNodeType(methodCall.Object)
	if t == "" {
		return nil
	}
	t = resolveAliasTypeString(result, t)
	baseType := baseTypeName(t)
	key := baseType + "." + methodCall.Method.Value
	if sym, ok := result.Index.Symbols[key]; ok {
		return []Location{sym.Location}
	}
	if loc, ok := s.lookupSymbolInImports(result, originURI, key, methodCall.Method.Value); ok {
		return []Location{loc}
	}
	if loc, ok := s.lookupPreludeSymbol(key, methodCall.Method.Value, false); ok {
		return []Location{loc}
	}
	return nil
}

func (s *Server) definitionForSimpleType(
	result *AnalysisResult,
	originURI string,
	simpleType *ast.SimpleType,
) []Location {
	if simpleType == nil {
		return nil
	}
	name := simpleType.Name
	if idx := strings.LastIndex(name, "."); idx != -1 {
		pkgAlias := name[:idx]
		typeName := name[idx+1:]
		if loc, ok := s.lookupImportedSymbol(result, originURI, pkgAlias, typeName); ok {
			return []Location{loc}
		}
	} else if sym, ok := result.Index.Symbols[name]; ok {
		return []Location{sym.Location}
	}
	return nil
}

func implementationTypeName(node ast.Node, result *AnalysisResult) string {
	switch n := node.(type) {
	case *ast.Identifier:
		return n.Value
	case *ast.StructDecl:
		if n.Name != nil {
			return n.Name.Value
		}
	case *ast.EnumDecl:
		if n.Name != nil {
			return n.Name.Value
		}
	default:
		if result != nil && result.TC != nil {
			t := result.TC.GetNodeType(node)
			if t != "" {
				return baseTypeName(resolveAliasTypeString(result, t))
			}
		}
	}
	return ""
}

func (s *Server) implementationLocations(typeName string) []Location {
	if typeName == "" {
		return nil
	}
	locs := []Location{}
	for uri, res := range s.cacheSnapshot() {
		if res == nil || res.AST == nil {
			continue
		}
		for _, stmt := range res.AST.Statements {
			impl, ok := stmt.(*ast.ImplDecl)
			if !ok || impl.TypeName == nil {
				continue
			}

			if impl.TypeName.Value == typeName ||
				strings.HasSuffix(impl.TypeName.Value, "."+typeName) {
				locs = append(locs, locationFromToken(uri, impl.Token, impl.Token.Literal))
			}
		}
	}
	return locs
}

func (s *Server) lookupWorkspaceSymbol(result *AnalysisResult, originURI, name string) (Location, bool) {
	if sym, ok := result.Index.Symbols[name]; ok {
		return sym.Location, true
	}
	originPath := uriToPath(originURI)
	for _, res := range s.cacheSnapshot() {
		if res == nil || res.Index == nil {
			continue
		}
		sym, ok := res.Index.Symbols[name]
		if !ok {
			continue
		}
		if sym.Exported || uriToPath(sym.Location.URI) == originPath {
			return sym.Location, true
		}
	}
	return Location{}, false
}

func (s *Server) handleDefinition(req Request) []Location {
	params, ok := requestParams[DefinitionParams](req)
	if !ok {
		return nil
	}
	result, ok := s.analysisResult(params.TextDocument.URI)
	if !ok || result.AST == nil || result.Index == nil {
		return nil
	}

	line := params.Position.Line + 1
	char := params.Position.Character + 1
	node := findNode(result.AST, line, char)
	if node == nil || isNil(node) {
		return nil
	}
	if locs := s.definitionFromRefs(result, node); len(locs) > 0 {
		return locs
	}
	if mce, ok := node.(*ast.MethodCallExpression); ok && result.TC != nil {
		t := result.TC.GetNodeType(mce.Object)
		if t != "" {
			t = resolveAliasTypeString(result, t)
			key := mapBuiltinType(baseTypeName(t)) + "." + mce.Method.Value
			if loc, ok := s.lookupPreludeSymbol(key, "", true); ok {
				return []Location{loc}
			}
		}
	}

	switch n := node.(type) {
	case *ast.Identifier:
		if locs := s.definitionForIdentifier(result, params.TextDocument.URI, params.Position, n); len(locs) > 0 {
			return locs
		}
	case *ast.FieldAccessExpression:
		if locs := s.definitionForFieldAccess(result, params.TextDocument.URI, n); len(locs) > 0 {
			return locs
		}
	case *ast.MethodCallExpression:
		if locs := s.definitionForMethodCall(result, params.TextDocument.URI, n); len(locs) > 0 {
			return locs
		}
	case *ast.SimpleType:
		if locs := s.definitionForSimpleType(result, params.TextDocument.URI, n); len(locs) > 0 {
			return locs
		}
	}
	return nil
}

func (s *Server) handleTypeDefinition(req Request) []Location {
	params, ok := requestParams[DefinitionParams](req)
	if !ok {
		return nil
	}
	result, ok := s.analysisResult(params.TextDocument.URI)
	if !ok || result.AST == nil || result.TC == nil {
		return nil
	}

	line := params.Position.Line + 1
	char := params.Position.Character + 1
	node := findNode(result.AST, line, char)
	if node == nil || isNil(node) {
		return nil
	}

	t := result.TC.GetNodeType(node)
	if t == "" {
		return nil
	}

	t = resolveAliasTypeString(result, t)
	base := baseTypeName(t)
	if base == "" {
		return nil
	}
	return s.findSymbolLocations(params.TextDocument.URI, base, result)
}

func (s *Server) findSymbolLocations(originURI, name string, result *AnalysisResult) []Location {
	if parts := strings.SplitN(name, ".", 2); len(parts) == 2 {
		if loc, ok := s.lookupImportedSymbol(result, originURI, parts[0], parts[1]); ok {
			return []Location{loc}
		}
	}

	if loc, ok := s.lookupWorkspaceSymbol(result, originURI, name); ok {
		return []Location{loc}
	}

	return nil
}

func (s *Server) handleImplementation(req Request) []Location {
	params, ok := requestParams[DefinitionParams](req)
	if !ok {
		return nil
	}
	result, ok := s.analysisResult(params.TextDocument.URI)
	if !ok || result.AST == nil {
		return nil
	}

	line := params.Position.Line + 1
	char := params.Position.Character + 1
	node := findNode(result.AST, line, char)
	if node == nil || isNil(node) {
		return nil
	}

	typeName := implementationTypeName(node, result)
	if typeName == "" {
		return nil
	}

	return s.implementationLocations(typeName)
}

func (s *Server) handleReferences(req Request) []Location {
	ctx := requestContext(req)
	params, ok := requestParams[ReferenceParams](req)
	if !ok {
		return nil
	}
	result, ok := s.analysisResult(params.TextDocument.URI)
	if !ok || result.AST == nil {
		return nil
	}
	line := params.Position.Line + 1
	char := params.Position.Character + 1
	node := findNode(result.AST, line, char)
	if node == nil {
		return nil
	}
	if result.RefByPos != nil {
		if key := nodeRefKey(node); key != "" {
			if id, ok := result.RefByPos[key]; ok {
				s.ensureWorkspaceRefIndex(ctx)
				if ctx.Err() != nil {
					return nil
				}
				refs := []Location{}
				if params.Context.IncludeDeclaration {
					for _, res := range s.cacheSnapshot() {
						if ctx.Err() != nil {
							return nil
						}
						if res == nil || res.Defs == nil {
							continue
						}
						if loc, ok := res.Defs[id]; ok {
							refs = append(refs, loc)
							break
						}
					}
				}
				for _, res := range s.cacheSnapshot() {
					if ctx.Err() != nil {
						return nil
					}
					if res == nil || res.RefIndex == nil {
						continue
					}
					if hits, ok := res.RefIndex[id]; ok {
						refs = append(refs, hits...)
					}
				}
				return refs
			}
		}
	}

	return collectWorkspaceReferences(s, params.TextDocument.URI, node)
}

func (s *Server) handleDocumentSymbol(req Request) []DocumentSymbol {
	params, ok := requestParams[DocumentSymbolParams](req)
	if !ok {
		return nil
	}

	result, ok := s.analysisResult(params.TextDocument.URI)
	if !ok || result.AST == nil {
		return nil
	}

	return collectDocumentSymbols(result.AST, result.TC)
}

func (s *Server) handleWorkspaceSymbol(req Request) []SymbolInformation {
	ctx := requestContext(req)
	params, ok := requestParams[WorkspaceSymbolParams](req)
	if !ok {
		return nil
	}
	s.ensureWorkspaceIndexes(ctx)
	if ctx.Err() != nil {
		return nil
	}
	query := strings.ToLower(params.Query)
	items := []SymbolInformation{}
	for _, idx := range s.indexSnapshot() {
		if ctx.Err() != nil {
			return nil
		}
		if idx == nil {
			continue
		}
		for _, sym := range idx.Symbols {
			if ctx.Err() != nil {
				return nil
			}
			if query != "" && !strings.Contains(strings.ToLower(sym.Name), query) {
				continue
			}
			items = append(items, workspaceSymbolInformation(sym))
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	return items
}

func (s *Server) handleRename(req Request) *WorkspaceEdit {
	ctx := requestContext(req)
	params, ok := requestParams[RenameParams](req)
	if !ok {
		return nil
	}

	s.ensureWorkspaceRefIndex(ctx)
	if ctx.Err() != nil {
		return &WorkspaceEdit{Changes: map[string][]TextEdit{}}
	}

	result, ok := s.analysisResult(params.TextDocument.URI)
	if !ok || result == nil || result.AST == nil || !isRenameableIdentifier(params.NewName) {
		return &WorkspaceEdit{Changes: map[string][]TextEdit{}}
	}

	if _, _, ok := renameTargetAt(result.AST, params.Position); !ok {
		return &WorkspaceEdit{Changes: map[string][]TextEdit{}}
	}
	node := findNode(result.AST, params.Position.Line+1, params.Position.Character+1)
	if node == nil || isNil(node) {
		return &WorkspaceEdit{Changes: map[string][]TextEdit{}}
	}
	if result.RefByPos == nil {
		return &WorkspaceEdit{Changes: map[string][]TextEdit{}}
	}
	key := nodeRefKey(node)
	if key == "" {
		return &WorkspaceEdit{Changes: map[string][]TextEdit{}}
	}
	if _, ok := result.RefByPos[key]; !ok {
		return &WorkspaceEdit{Changes: map[string][]TextEdit{}}
	}

	refParams := ReferenceParams{
		TextDocument: params.TextDocument,
		Position:     params.Position,
		Context:      ReferenceContext{IncludeDeclaration: true},
	}

	refs := s.handleReferences(Request{ParamsValue: refParams, Context: ctx})
	if len(refs) == 0 {
		return &WorkspaceEdit{Changes: map[string][]TextEdit{}}
	}

	changes := make(map[string][]TextEdit)
	seen := map[string]bool{}
	for _, loc := range refs {
		key := strfmt.Named(
			"{URI}:{SL}:{SC}:{EL}:{EC}",
			"URI", loc.URI,
			"SL", loc.Range.Start.Line,
			"SC", loc.Range.Start.Character,
			"EL", loc.Range.End.Line,
			"EC", loc.Range.End.Character,
		)
		if seen[key] {
			continue
		}
		seen[key] = true
		changes[loc.URI] = append(changes[loc.URI], TextEdit{
			Range:   loc.Range,
			NewText: params.NewName,
		})
	}

	return &WorkspaceEdit{Changes: changes}
}

func (s *Server) handleDocumentHighlight(req Request) []DocumentHighlight {
	params, ok := requestParams[DocumentHighlightParams](req)
	if !ok {
		return nil
	}
	refParams := ReferenceParams{
		TextDocument: params.TextDocument,
		Position:     params.Position,
		Context:      ReferenceContext{IncludeDeclaration: true},
	}
	refs := s.handleReferences(Request{ParamsValue: refParams, Context: requestContext(req)})

	highlights := []DocumentHighlight{}
	for _, loc := range refs {
		if loc.URI == params.TextDocument.URI {
			highlights = append(highlights, DocumentHighlight{
				Range: loc.Range,
				Kind:  1, // Text
			})
		}
	}
	return highlights
}

func (s *Server) handleCodeAction(req Request) []CodeAction {
	params, ok := requestParams[CodeActionParams](req)
	if !ok {
		return nil
	}
	ctx := requestContext(req)

	actions := []CodeAction{}
	text, hasDocument := s.document(params.TextDocument.URI)
	result := s.analysisResultOrNil(params.TextDocument.URI)
	for _, diag := range params.Context.Diagnostics {
		actions = addQuickFixActions(actions, params.TextDocument.URI, diag)
		actions = addRemoveUnusedAction(actions, params.TextDocument.URI, diag)
		actions = addCreateMissingImportFileAction(actions, diag)
		if hasDocument {
			actions = addRemoveImportDiagnosticAction(actions, params.TextDocument.URI, text, diag)
		}
	}

	actions = append(actions, CodeAction{
		Title: "Organize Imports",
		Kind:  "source.organizeImports",
		Edit:  s.getOrganizeImportsEdit(params.TextDocument.URI),
	})

	actions = addRemoveAllUnusedAction(
		actions,
		params.TextDocument.URI,
		params.Context.Diagnostics,
	)

	actions = addAutoImportActions(
		actions,
		result,
		params.TextDocument.URI,
		params.Context.Diagnostics,
	)
	if result != nil {
		s.ensureWorkspaceIndexes(ctx)
		actions = s.addLocalAutoImportActions(
			actions,
			result,
			params.TextDocument.URI,
			params.Context.Diagnostics,
		)
	}
	if hasDocument {
		actions = addMutabilityActions(actions, params.TextDocument.URI, text, params.Context.Diagnostics)
		actions = addCreateMissingFieldActions(actions, params.TextDocument.URI, text, result, params.Context.Diagnostics)
		actions = addCreateMissingMethodActions(actions, params.TextDocument.URI, text, result, params.Context.Diagnostics)
		actions = addSwitchCaseActions(actions, params.TextDocument.URI, text, result, params.Range.Start)
	}

	return actions
}

func symbolKindFromKind(kind string) int {
	switch kind {
	case "func":
		return 12
	case "method":
		return 6
	case "struct":
		return 23
	case "enum":
		return 10
	case "type", "alias":
		return 26
	case "var":
		return 13
	case "const":
		return 14
	case "field":
		return 8
	default:
		return 1
	}
}

func workspaceSymbolInformation(sym SymbolInfo) SymbolInformation {
	name := sym.Name
	container := ""
	if parts := strings.SplitN(sym.Name, ".", 2); len(parts) == 2 {
		container = parts[0]
		name = parts[1]
	}
	return SymbolInformation{
		Name:          name,
		Kind:          symbolKindFromKind(sym.Kind),
		Location:      sym.Location,
		ContainerName: container,
	}
}

func documentSymbolFromToken(
	tok token.Token,
	name string,
	kind int,
	detail string,
	children []DocumentSymbol,
	span ast.Span,
) *DocumentSymbol {
	fullRange := rangeFromToken(tok, name)
	if r, ok := rangeFromSpan(span); ok {
		fullRange = r
	}

	selectionRange := rangeFromToken(tok, name)
	return &DocumentSymbol{
		Name:           name,
		Kind:           kind,
		Detail:         detail,
		Range:          fullRange,
		SelectionRange: selectionRange,
		Children:       children,
	}
}

func fieldDocumentSymbol(field *ast.StructField) *DocumentSymbol {
	if field == nil || field.Name == nil {
		return nil
	}

	detail := ""
	if field.Type != nil {
		detail = field.Type.String()
	}

	return documentSymbolFromToken(
		field.Name.Token,
		field.Name.Value,
		SymbolKindField,
		detail,
		nil,
		field.Span,
	)
}

func variantDocumentSymbol(variant *ast.EnumVariant) *DocumentSymbol {
	if variant == nil || variant.Name == nil {
		return nil
	}
	detail := ""
	if len(variant.Fields) > 0 {
		parts := make([]string, 0, len(variant.Fields))
		for _, f := range variant.Fields {
			if f != nil {
				parts = append(parts, f.String())
			}
		}
		detail = "(" + strings.Join(parts, ", ") + ")"
	}

	return documentSymbolFromToken(
		variant.Name.Token,
		variant.Name.Value,
		SymbolKindEnumMember,
		detail,
		nil,
		variant.Span,
	)
}

func implMethodDocumentSymbol(method *ast.MethodDecl) *DocumentSymbol {
	if method == nil || method.Name == nil {
		return nil
	}
	return documentSymbolFromToken(
		method.Name.Token,
		method.Name.Value,
		SymbolKindMethod,
		formatFuncDetail(
			method.Parameters,
			method.ReturnType,
			method.Mutable,
		),
		nil,
		method.Span,
	)
}

func mergeDocumentSymbolMethods(
	structs map[string]*DocumentSymbol,
	methods map[string][]*DocumentSymbol,
	entries []*DocumentSymbol,
) []*DocumentSymbol {
	for typeName, ms := range methods {
		if st, ok := structs[typeName]; ok {
			for _, m := range ms {
				if m != nil {
					st.Children = append(st.Children, *m)
				}
			}
			continue
		}
		for _, m := range ms {
			if m == nil {
				continue
			}
			m.Name = typeName + "." + m.Name
			entries = append(entries, m)
		}
	}
	return entries
}

func collectDocumentSymbols(prog *ast.Program, tc *typechecker.TypeChecker) []DocumentSymbol {
	if prog == nil {
		return nil
	}
	entries := []*DocumentSymbol{}
	structs := make(map[string]*DocumentSymbol)
	methods := make(map[string][]*DocumentSymbol)

	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			if s == nil || s.Name == nil {
				continue
			}
			sym := documentSymbolFromToken(
				s.Name.Token,
				s.Name.Value,
				SymbolKindFunction,
				formatFuncDetail(
					s.Parameters,
					s.ReturnType,
					false,
				),
				nil,
				s.Span,
			)
			entries = append(entries, sym)
		case *ast.StructDecl:
			if s == nil || s.Name == nil {
				continue
			}
			children := []DocumentSymbol{}
			for _, f := range s.Fields {
				if sym := fieldDocumentSymbol(f); sym != nil {
					children = append(children, *sym)
				}
			}
			sym := documentSymbolFromToken(
				s.Name.Token,
				s.Name.Value,
				SymbolKindStruct,
				"",
				children,
				s.Span,
			)
			entries = append(entries, sym)
			structs[s.Name.Value] = sym
		case *ast.EnumDecl:
			if s == nil || s.Name == nil {
				continue
			}
			children := []DocumentSymbol{}
			for _, v := range s.Variants {
				if sym := variantDocumentSymbol(v); sym != nil {
					children = append(children, *sym)
				}
			}
			sym := documentSymbolFromToken(
				s.Name.Token,
				s.Name.Value,
				SymbolKindEnum,
				"",
				children,
				s.Span,
			)
			entries = append(entries, sym)
		case *ast.TypeDecl:
			if s == nil || s.Name == nil {
				continue
			}
			detail := ""
			if s.Underlying != nil {
				detail = s.Underlying.String()
			}
			sym := documentSymbolFromToken(
				s.Name.Token,
				s.Name.Value,
				SymbolKindTypeAlias,
				detail,
				nil,
				s.Span,
			)
			entries = append(entries, sym)
		case *ast.AliasDecl:
			if s == nil || s.Name == nil {
				continue
			}
			detail := ""
			if s.Underlying != nil {
				detail = s.Underlying.String()
			}
			sym := documentSymbolFromToken(
				s.Name.Token,
				s.Name.Value,
				SymbolKindTypeAlias,
				detail,
				nil,
				s.Span,
			)
			entries = append(entries, sym)
		case *ast.ConstStatement:
			if s == nil || s.Name == nil {
				continue
			}
			detail := ""
			if s.Type != nil {
				detail = s.Type.String()
			} else if tc != nil {
				if t := tc.GetNodeType(s.Name); t != "" {
					detail = t
				}
			}
			sym := documentSymbolFromToken(
				s.Name.Token,
				s.Name.Value,
				SymbolKindConstant,
				detail,
				nil,
				s.Span,
			)
			entries = append(entries, sym)
		case *ast.VarStatement:
			if s == nil || s.Name == nil {
				continue
			}
			detail := ""
			if s.Type != nil {
				detail = s.Type.String()
			} else if tc != nil {
				if t := tc.GetNodeType(s.Name); t != "" {
					detail = t
				}
			}
			sym := documentSymbolFromToken(
				s.Name.Token,
				s.Name.Value,
				SymbolKindVariable,
				detail,
				nil,
				s.Span,
			)
			entries = append(entries, sym)
		case *ast.ImplDecl:
			if s == nil || s.TypeName == nil {
				continue
			}
			typeName := s.TypeName.Value
			for _, m := range s.Methods {
				if sym := implMethodDocumentSymbol(m); sym != nil {
					methods[typeName] = append(methods[typeName], sym)
				}
			}
		}
	}

	entries = mergeDocumentSymbolMethods(structs, methods, entries)

	out := make([]DocumentSymbol, 0, len(entries))
	for _, sym := range entries {
		if sym != nil {
			out = append(out, *sym)
		}
	}
	return out
}

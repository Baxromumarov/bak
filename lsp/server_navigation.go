package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/prelude"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func (s *Server) handleDefinition(req Request) []Location {
	var params DefinitionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	result, ok := s.Cache[params.TextDocument.URI]
	if !ok || result.AST == nil || result.Index == nil {
		return nil
	}

	line := params.Position.Line + 1
	char := params.Position.Character + 1
	node := findNode(result.AST, line, char)
	if node == nil || isNil(node) {
		return nil
	}
	if result.RefByPos != nil && result.Defs != nil {
		if key := nodeRefKey(node); key != "" {
			if id, ok := result.RefByPos[key]; ok {
				if defLoc, ok := result.Defs[id]; ok {
					return []Location{defLoc}
				}
			}
		}
	}
	if mce, ok := node.(*ast.MethodCallExpression); ok {
		if result.TC != nil {
			t := result.TC.GetNodeType(mce.Object)
			if t != "" {
				t = resolveAliasTypeString(result, t)
				baseType := mapBuiltinType(baseTypeName(t))
				key := baseType + "." + mce.Method.Value
				stdLibPath := prelude.GetStdLibPath()
				preludeFiles := []string{
					filepath.Join(stdLibPath, "builtins.bak"),
					filepath.Join(stdLibPath, "collections", "vec.bak"),
					filepath.Join(stdLibPath, "collections", "hashmap.bak"),
					filepath.Join(stdLibPath, "result.bak"),
				}
				for _, path := range preludeFiles {
					modIndex := s.getOrIndexFile(path)
					if modIndex != nil {
						if sym, ok := modIndex.Symbols[key]; ok {
							return []Location{sym.Location}
						}
					}
				}
			}
		}
	}

	switch n := node.(type) {
	case *ast.Identifier:
		// Check if it's a composite identifier like "http.Server" from the parser
		if parts := strings.SplitN(n.Value, ".", 2); len(parts) == 2 {
			moduleName := parts[0]
			typeName := parts[1]
			if importPath, ok := result.Imports[moduleName]; ok {
				path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
				if path != "" {
					modIndex := s.getOrIndexFile(path)
					if modIndex != nil {
						if sym, ok := modIndex.Symbols[typeName]; ok {
							return []Location{sym.Location}
						}
					}
				}
			}
		}

		if sym, ok := result.Index.Symbols[n.Value]; ok {
			return []Location{sym.Location}
		}
		// If the identifier is a member name (obj.member), try resolving via the qualifier.
		text := s.Documents[params.TextDocument.URI]
		if text == "" {
			if data, err := os.ReadFile(uriToPath(params.TextDocument.URI)); err == nil {
				text = string(data)
			}
		}
		if text != "" {
			lineText := lineAt(text, params.Position.Line)
			word, start := wordAt(lineText, params.Position.Character)
			if word == n.Value && start > 0 && lineText[start-1] == '.' {
				qualifier := qualifierBefore(lineText, start-1)
				if qualifier != "" {
					// Module-qualified member (pkg.Symbol)
					if importPath, ok := result.Imports[qualifier]; ok {
						path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
						if path != "" {
							modIndex := s.getOrIndexFile(path)
							if modIndex != nil {
								if sym, ok := modIndex.Symbols[word]; ok {
									return []Location{sym.Location}
								}
							}
						}
					}

					// Struct member (Type.member) resolved via qualifier type.
					if result.TC != nil {
						line := params.Position.Line + 1
						qualCol := start - 1 - len(qualifier) + 1
						if qualCol > 0 {
							if qNode := findNode(result.AST, line, qualCol); qNode != nil {
								if t := result.TC.GetNodeType(qNode); t != "" {
									t = resolveAliasTypeString(result, t)
									baseType := mapBuiltinType(baseTypeName(t))
									key := baseType + "." + word
									if sym, ok := result.Index.Symbols[key]; ok {
										return []Location{sym.Location}
									}
									for _, importPath := range result.Imports {
										path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
										if path != "" {
											modIndex := s.getOrIndexFile(path)
											if modIndex != nil {
												if sym, ok := modIndex.Symbols[key]; ok {
													return []Location{sym.Location}
												}
											}
										}
									}
									stdLibPath := prelude.GetStdLibPath()
									preludeFiles := []string{
										filepath.Join(stdLibPath, "builtins.bak"),
										filepath.Join(stdLibPath, "collections", "vec.bak"),
										filepath.Join(stdLibPath, "collections", "hashmap.bak"),
										filepath.Join(stdLibPath, "result.bak"),
									}
									for _, path := range preludeFiles {
										modIndex := s.getOrIndexFile(path)
										if modIndex != nil {
											log.Printf("DEBUG(ID): Searching for %s in %s (found %d symbols)\n", key, path, len(modIndex.Symbols))
											if sym, ok := modIndex.Symbols[key]; ok {
												return []Location{sym.Location}
											}
											if strings.Contains(path, "builtins.bak") {
												for k := range modIndex.Symbols {
													log.Printf("DEBUG(ID): Found symbol key: %s\n", k)
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	case *ast.FieldAccessExpression:
		if ident, ok := n.Object.(*ast.Identifier); ok {
			if importPath, ok := result.Imports[ident.Value]; ok {
				path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
				if path != "" {
					modIndex := s.getOrIndexFile(path)
					if modIndex != nil {
						if sym, ok := modIndex.Symbols[n.Field.Value]; ok {
							return []Location{sym.Location}
						}
					}
				}
			}
		}
		if result.TC != nil {
			if t := result.TC.GetNodeType(n.Object); t != "" {
				t = resolveAliasTypeString(result, t)
				key := baseTypeName(t) + "." + n.Field.Value
				if sym, ok := result.Index.Symbols[key]; ok {
					return []Location{sym.Location}
				}
			}
		}
	case *ast.MethodCallExpression:
		if ident, ok := n.Object.(*ast.Identifier); ok {
			if importPath, ok := result.Imports[ident.Value]; ok {
				path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
				if path != "" {
					modIndex := s.getOrIndexFile(path)
					if modIndex != nil {
						if sym, ok := modIndex.Symbols[n.Method.Value]; ok {
							return []Location{sym.Location}
						}
					}
				}
			}
		}
		if result.TC != nil {
			if t := result.TC.GetNodeType(n.Object); t != "" {
				t = resolveAliasTypeString(result, t)
				baseType := baseTypeName(t)
				key := baseType + "." + n.Method.Value
				// Try local index first
				if sym, ok := result.Index.Symbols[key]; ok {
					return []Location{sym.Location}
				}
				// Try imported modules - the type might be defined in an import
				for _, importPath := range result.Imports {
					path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
					if path != "" {
						modIndex := s.getOrIndexFile(path)
						if modIndex != nil {
							if sym, ok := modIndex.Symbols[key]; ok {
								return []Location{sym.Location}
							}
							// Also check just the method name (for methods in same file as struct)
							if sym, ok := modIndex.Symbols[n.Method.Value]; ok {
								return []Location{sym.Location}
							}
						}
					}
				}

				// Key fallback: Check Standard Library Prelude (Vec, HashMap, etc.)
				stdLibPath := prelude.GetStdLibPath()
				preludeFiles := []string{
					filepath.Join(stdLibPath, "collections", "vec.bak"),
					filepath.Join(stdLibPath, "collections", "hashmap.bak"),
					filepath.Join(stdLibPath, "result.bak"),
				}

				for _, path := range preludeFiles {
					modIndex := s.getOrIndexFile(path)
					if modIndex != nil {
						if sym, ok := modIndex.Symbols[key]; ok {
							return []Location{sym.Location}
						}
						// Also check just the method name
						if sym, ok := modIndex.Symbols[n.Method.Value]; ok {
							return []Location{sym.Location}
						}
					}
				}
			}
		}
	case *ast.SimpleType:
		name := n.Name
		if idx := strings.LastIndex(name, "."); idx != -1 {
			pkgAlias := name[:idx]
			typeName := name[idx+1:]
			if importPath, ok := result.Imports[pkgAlias]; ok {
				path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
				if path != "" {
					modIndex := s.getOrIndexFile(path)
					if modIndex != nil {
						if sym, ok := modIndex.Symbols[typeName]; ok {
							return []Location{sym.Location}
						}
					}
				}
			}
		} else if sym, ok := result.Index.Symbols[name]; ok {
			return []Location{sym.Location}
		}
	}
	return nil
}

func (s *Server) handleTypeDefinition(req Request) []Location {
	var params DefinitionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	result, ok := s.Cache[params.TextDocument.URI]
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

	// Try to find the symbol 'base'
	return s.findSymbolLocations(params.TextDocument.URI, base, result)
}

func (s *Server) findSymbolLocations(originURI, name string, result *AnalysisResult) []Location {
	// Handle module-qualified search if 'name' contains a dot (e.g., "http.Server")
	if parts := strings.SplitN(name, ".", 2); len(parts) == 2 {
		moduleName := parts[0]
		typeName := parts[1]
		if importPath, ok := result.Imports[moduleName]; ok {
			path := s.resolveImportPath(uriToPath(originURI), importPath)
			if path != "" {
				modIndex := s.getOrIndexFile(path)
				if modIndex != nil {
					if sym, ok := modIndex.Symbols[typeName]; ok {
						return []Location{sym.Location}
					}
				}
			}
		}
	}

	// Local lookup
	if sym, ok := result.Index.Symbols[name]; ok {
		return []Location{sym.Location}
	}

	// Workspace-wide lookup for the symbol
	for _, res := range s.Cache {
		if res == nil || res.Index == nil {
			continue
		}
		if sym, ok := res.Index.Symbols[name]; ok {
			if sym.Exported || uriToPath(res.Index.Symbols[name].Location.URI) == uriToPath(originURI) {
				return []Location{sym.Location}
			}
		}
	}

	return nil
}

func (s *Server) handleImplementation(req Request) []Location {
	var params DefinitionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	result, ok := s.Cache[params.TextDocument.URI]
	if !ok || result.AST == nil {
		return nil
	}

	line := params.Position.Line + 1
	char := params.Position.Character + 1
	node := findNode(result.AST, line, char)
	if node == nil || isNil(node) {
		return nil
	}

	// Identify the type we are interested in.
	typeName := ""
	if ident, ok := node.(*ast.Identifier); ok {
		typeName = ident.Value
	} else if structDecl, ok := node.(*ast.StructDecl); ok {
		typeName = structDecl.Name.Value
	} else if enumDecl, ok := node.(*ast.EnumDecl); ok {
		typeName = enumDecl.Name.Value
	} else {
		// Fallback to type of node.
		if result.TC != nil {
			t := result.TC.GetNodeType(node)
			typeName = baseTypeName(resolveAliasTypeString(result, t))
		}
	}

	if typeName == "" {
		return nil
	}

	// Find all 'impl typeName' blocks in the workspace.
	locs := []Location{}
	for uri, res := range s.Cache {
		if res == nil || res.AST == nil {
			continue
		}
		for _, stmt := range res.AST.Statements {
			if impl, ok := stmt.(*ast.ImplDecl); ok {
				if impl.TypeName != nil && (impl.TypeName.Value == typeName || strings.HasSuffix(impl.TypeName.Value, "."+typeName)) {
					locs = append(locs, Location{
						URI: uri,
						Range: Range{
							Start: Position{Line: impl.Token.Line - 1, Character: impl.Token.Column - 1},
							End:   Position{Line: impl.Token.Line - 1, Character: impl.Token.Column - 1 + 4}, // length of "impl"
						},
					})
				}
			}
		}
	}

	return locs
}

func (s *Server) handleReferences(req Request) []Location {
	var params ReferenceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	result, ok := s.Cache[params.TextDocument.URI]
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
				s.ensureWorkspaceRefIndex()
				refs := []Location{}
				if params.Context.IncludeDeclaration {
					for _, res := range s.Cache {
						if res == nil || res.Defs == nil {
							continue
						}
						if loc, ok := res.Defs[id]; ok {
							refs = append(refs, loc)
							break
						}
					}
				}
				for _, res := range s.Cache {
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
	return collectReferencesWorkspace(s, params.TextDocument.URI, node)
}

func (s *Server) handleDocumentSymbol(req Request) []DocumentSymbol {
	var params DocumentSymbolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	result, ok := s.Cache[params.TextDocument.URI]
	if !ok || result.AST == nil {
		return nil
	}
	return collectDocumentSymbols(result.AST, result.TC)
}

func (s *Server) handleWorkspaceSymbol(req Request) []SymbolInformation {
	var params WorkspaceSymbolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	s.ensureWorkspaceIndexes()
	query := strings.ToLower(params.Query)
	items := []SymbolInformation{}
	for _, idx := range s.Indexes {
		if idx == nil {
			continue
		}
		for _, sym := range idx.Symbols {
			if query != "" && !strings.Contains(strings.ToLower(sym.Name), query) {
				continue
			}
			name := sym.Name
			container := ""
			if parts := strings.SplitN(sym.Name, ".", 2); len(parts) == 2 {
				container = parts[0]
				name = parts[1]
			}
			items = append(items, SymbolInformation{
				Name:          name,
				Kind:          symbolKindFromKind(sym.Kind),
				Location:      sym.Location,
				ContainerName: container,
			})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (s *Server) handleRename(req Request) *WorkspaceEdit {
	var params RenameParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	refParams := ReferenceParams{
		TextDocument: params.TextDocument,
		Position:     params.Position,
		Context:      ReferenceContext{IncludeDeclaration: true},
	}
	refParamsBytes, _ := json.Marshal(refParams)
	refs := s.handleReferences(Request{Params: refParamsBytes})
	if len(refs) == 0 {
		return &WorkspaceEdit{Changes: map[string][]TextEdit{}}
	}
	changes := make(map[string][]TextEdit)
	for _, loc := range refs {
		changes[loc.URI] = append(changes[loc.URI], TextEdit{
			Range:   loc.Range,
			NewText: params.NewName,
		})
	}
	return &WorkspaceEdit{Changes: changes}
}

func (s *Server) handleDocumentHighlight(req Request) []DocumentHighlight {
	var params DocumentHighlightParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	refParams := ReferenceParams{
		TextDocument: params.TextDocument,
		Position:     params.Position,
		Context:      ReferenceContext{IncludeDeclaration: true},
	}
	refParamsBytes, _ := json.Marshal(refParams)
	refs := s.handleReferences(Request{Params: refParamsBytes})

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
	var params CodeActionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}

	actions := []CodeAction{}
	for _, diag := range params.Context.Diagnostics {
		for _, fix := range diagnosticFixesFromData(diag.Data) {
			title := strings.TrimSpace(fix.Title)
			if title == "" {
				title = "Apply suggested fix"
			}
			actions = append(actions, CodeAction{
				Title:       title,
				Kind:        "quickfix",
				Diagnostics: []Diagnostic{diag},
				Edit: &WorkspaceEdit{
					Changes: map[string][]TextEdit{
						params.TextDocument.URI: {
							{
								Range:   fix.Range,
								NewText: fix.NewText,
							},
						},
					},
				},
			})
		}
		if strings.Contains(diag.Message, "unused") {
			actions = append(actions, CodeAction{
				Title:       "Remove unused declaration",
				Kind:        "quickfix",
				Diagnostics: []Diagnostic{diag},
				Edit: &WorkspaceEdit{
					Changes: map[string][]TextEdit{
						params.TextDocument.URI: {
							{
								Range:   diag.Range,
								NewText: "",
							},
						},
					},
				},
			})
		}
	}

	// Add "Organize Imports" action
	actions = append(actions, CodeAction{
		Title: "Organize Imports",
		Kind:  "source.organizeImports",
		Edit:  s.getOrganizeImportsEdit(params.TextDocument.URI),
	})

	// Add "Remove all unused" if there are multiple unused diagnostics
	unusedCount := 0
	for _, diag := range params.Context.Diagnostics {
		if strings.Contains(diag.Message, "unused") {
			unusedCount++
		}
	}
	if unusedCount > 1 {
		edits := []TextEdit{}
		diags := []Diagnostic{}
		for _, diag := range params.Context.Diagnostics {
			if strings.Contains(diag.Message, "unused") {
				edits = append(edits, TextEdit{Range: diag.Range, NewText: ""})
				diags = append(diags, diag)
			}
		}
		actions = append(actions, CodeAction{
			Title:       "Remove all unused declarations",
			Kind:        "quickfix",
			Diagnostics: diags,
			Edit: &WorkspaceEdit{
				Changes: map[string][]TextEdit{
					params.TextDocument.URI: edits,
				},
			},
		})
	}

	// Auto-import: suggest imports for undefined symbols
	result := s.Cache[params.TextDocument.URI]
	for _, diag := range params.Context.Diagnostics {
		// Match "undefined: X" or "undefined type: X"
		symbolName := extractUndefinedSymbol(diag.Message)
		if symbolName == "" {
			continue
		}
		candidates := lookupStdlibSymbol(symbolName)
		if len(candidates) == 0 {
			continue
		}
		insertPos := findImportInsertPosition(result)
		for _, candidate := range candidates {
			importLine := strfmt.Named("import \"{ImportPath}\" as {Alias}\n", "ImportPath", candidate.ImportPath, "Alias", candidate.Alias)
			actions = append(actions, CodeAction{
				Title: strfmt.Named(
					"Import '{SymbolName}' from {Alias}",
					"SymbolName", symbolName,
					"Alias", candidate.Alias,
				),
				Kind:        "quickfix",
				Diagnostics: []Diagnostic{diag},
				Edit: &WorkspaceEdit{
					Changes: map[string][]TextEdit{
						params.TextDocument.URI: {
							{
								Range:   Range{Start: insertPos, End: insertPos},
								NewText: importLine,
							},
						},
					},
				},
			})
		}
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

func collectDocumentSymbols(prog *ast.Program, tc *typechecker.TypeChecker) []DocumentSymbol {
	if prog == nil {
		return nil
	}
	entries := []*DocumentSymbol{}
	structs := make(map[string]*DocumentSymbol)
	methods := make(map[string][]DocumentSymbol)

	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			if s == nil || s.Name == nil {
				continue
			}
			fullRange := rangeFromToken(s.Name.Token, s.Name.Value)
			if r, ok := rangeFromSpan(s.Span); ok {
				fullRange = r
			}
			sym := &DocumentSymbol{
				Name:           s.Name.Value,
				Kind:           12,
				Detail:         formatFuncDetail(s.Parameters, s.ReturnType, false),
				Range:          fullRange,
				SelectionRange: rangeFromToken(s.Name.Token, s.Name.Value),
			}
			entries = append(entries, sym)
		case *ast.StructDecl:
			if s == nil || s.Name == nil {
				continue
			}
			fullRange := rangeFromToken(s.Name.Token, s.Name.Value)
			if r, ok := rangeFromSpan(s.Span); ok {
				fullRange = r
			}
			children := []DocumentSymbol{}
			for _, f := range s.Fields {
				if f == nil || f.Name == nil {
					continue
				}
				fieldRange := rangeFromToken(f.Name.Token, f.Name.Value)
				if r, ok := rangeFromSpan(f.Span); ok {
					fieldRange = r
				}
				detail := ""
				if f.Type != nil {
					detail = f.Type.String()
				}
				children = append(children, DocumentSymbol{
					Name:           f.Name.Value,
					Kind:           8,
					Detail:         detail,
					Range:          fieldRange,
					SelectionRange: rangeFromToken(f.Name.Token, f.Name.Value),
				})
			}
			sym := &DocumentSymbol{
				Name:           s.Name.Value,
				Kind:           23,
				Range:          fullRange,
				SelectionRange: rangeFromToken(s.Name.Token, s.Name.Value),
				Children:       children,
			}
			entries = append(entries, sym)
			structs[s.Name.Value] = sym
		case *ast.EnumDecl:
			if s == nil || s.Name == nil {
				continue
			}
			fullRange := rangeFromToken(s.Name.Token, s.Name.Value)
			if r, ok := rangeFromSpan(s.Span); ok {
				fullRange = r
			}
			children := []DocumentSymbol{}
			for _, v := range s.Variants {
				if v == nil || v.Name == nil {
					continue
				}
				varRange := rangeFromToken(v.Name.Token, v.Name.Value)
				if r, ok := rangeFromSpan(v.Span); ok {
					varRange = r
				}
				detail := ""
				if len(v.Fields) > 0 {
					parts := make([]string, 0, len(v.Fields))
					for _, f := range v.Fields {
						if f != nil {
							parts = append(parts, f.String())
						}
					}
					detail = "(" + strings.Join(parts, ", ") + ")"
				}
				children = append(children, DocumentSymbol{
					Name:           v.Name.Value,
					Kind:           22,
					Detail:         detail,
					Range:          varRange,
					SelectionRange: rangeFromToken(v.Name.Token, v.Name.Value),
				})
			}
			sym := &DocumentSymbol{
				Name:           s.Name.Value,
				Kind:           10,
				Range:          fullRange,
				SelectionRange: rangeFromToken(s.Name.Token, s.Name.Value),
				Children:       children,
			}
			entries = append(entries, sym)
		case *ast.TypeDecl:
			if s == nil || s.Name == nil {
				continue
			}
			fullRange := rangeFromToken(s.Name.Token, s.Name.Value)
			if r, ok := rangeFromSpan(s.Span); ok {
				fullRange = r
			}
			detail := ""
			if s.Underlying != nil {
				detail = s.Underlying.String()
			}
			sym := &DocumentSymbol{
				Name:           s.Name.Value,
				Kind:           26,
				Detail:         detail,
				Range:          fullRange,
				SelectionRange: rangeFromToken(s.Name.Token, s.Name.Value),
			}
			entries = append(entries, sym)
		case *ast.AliasDecl:
			if s == nil || s.Name == nil {
				continue
			}
			fullRange := rangeFromToken(s.Name.Token, s.Name.Value)
			if r, ok := rangeFromSpan(s.Span); ok {
				fullRange = r
			}
			detail := ""
			if s.Underlying != nil {
				detail = s.Underlying.String()
			}
			sym := &DocumentSymbol{
				Name:           s.Name.Value,
				Kind:           26,
				Detail:         detail,
				Range:          fullRange,
				SelectionRange: rangeFromToken(s.Name.Token, s.Name.Value),
			}
			entries = append(entries, sym)
		case *ast.ConstStatement:
			if s == nil || s.Name == nil {
				continue
			}
			fullRange := rangeFromToken(s.Name.Token, s.Name.Value)
			if r, ok := rangeFromSpan(s.Span); ok {
				fullRange = r
			}
			detail := ""
			if s.Type != nil {
				detail = s.Type.String()
			} else if tc != nil {
				if t := tc.GetNodeType(s.Name); t != "" {
					detail = t
				}
			}
			sym := &DocumentSymbol{
				Name:           s.Name.Value,
				Kind:           14,
				Detail:         detail,
				Range:          fullRange,
				SelectionRange: rangeFromToken(s.Name.Token, s.Name.Value),
			}
			entries = append(entries, sym)
		case *ast.VarStatement:
			if s == nil || s.Name == nil {
				continue
			}
			fullRange := rangeFromToken(s.Name.Token, s.Name.Value)
			if r, ok := rangeFromSpan(s.Span); ok {
				fullRange = r
			}
			detail := ""
			if s.Type != nil {
				detail = s.Type.String()
			} else if tc != nil {
				if t := tc.GetNodeType(s.Name); t != "" {
					detail = t
				}
			}
			sym := &DocumentSymbol{
				Name:           s.Name.Value,
				Kind:           13,
				Detail:         detail,
				Range:          fullRange,
				SelectionRange: rangeFromToken(s.Name.Token, s.Name.Value),
			}
			entries = append(entries, sym)
		case *ast.ImplDecl:
			if s == nil || s.TypeName == nil {
				continue
			}
			typeName := s.TypeName.Value
			for _, m := range s.Methods {
				if m == nil || m.Name == nil {
					continue
				}
				methodRange := rangeFromToken(m.Name.Token, m.Name.Value)
				if r, ok := rangeFromSpan(m.Span); ok {
					methodRange = r
				}
				methods[typeName] = append(methods[typeName], DocumentSymbol{
					Name:           m.Name.Value,
					Kind:           6,
					Detail:         formatFuncDetail(m.Parameters, m.ReturnType, m.Mutable),
					Range:          methodRange,
					SelectionRange: rangeFromToken(m.Name.Token, m.Name.Value),
				})
			}
		}
	}

	for typeName, ms := range methods {
		if st, ok := structs[typeName]; ok {
			st.Children = append(st.Children, ms...)
			continue
		}
		for _, m := range ms {
			name := typeName + "." + m.Name
			m.Name = name
			entries = append(entries, &m)
		}
	}

	out := make([]DocumentSymbol, 0, len(entries))
	for _, sym := range entries {
		if sym != nil {
			out = append(out, *sym)
		}
	}
	return out
}

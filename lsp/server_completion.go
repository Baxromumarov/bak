package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/builtins"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func (s *Server) handleCompletion(req Request) CompletionList {
	out := CompletionList{Items: []CompletionItem{}}

	params, ok := requestParams[CompletionParams](req)
	if !ok {
		return out
	}
	ctx := requestContext(req)

	text, ok := s.document(params.TextDocument.URI)
	if !ok {
		return out
	}

	if isPositionInComment(text, params.Position) {
		return out
	}

	lineText := lineAt(text, params.Position.Line)
	if prefix, ok := importPathPrefix(lineText, params.Position.Character); ok {
		items := []CompletionItem{}
		for _, path := range s.getStdImportPaths() {
			if strings.HasPrefix(path, prefix) {
				items = append(items, CompletionItem{
					Label:  path,
					Kind:   9,
					Detail: "std import",
				})
			}
		}

		out.Items = rankCompletionItems(items, prefix, importPathCompletionPriority)
		return out
	}

	qualifier := ""
	memberPrefix := ""
	isDotCompletion := false
	if q, p, ok := memberAccessContext(lineText, params.Position.Character); ok {
		qualifier = q
		memberPrefix = p
		isDotCompletion = true
	} else {
		qualifier = qualifierAt(lineText, params.Position.Character)
	}

	items := []CompletionItem{}
	if !isDotCompletion {
		result := s.completionAnalysisResult(params.TextDocument.URI)
		if structItems := s.completeStructLiteralFields(ctx, result, text, params.TextDocument.URI, params.Position); len(structItems) > 0 {
			out.Items = rankCompletionItems(structItems, qualifier, fieldCompletionPriority)
			return out
		}
		if switchItems := completeSwitchCaseVariants(text, params.Position); len(switchItems) > 0 {
			out.Items = rankCompletionItems(switchItems, wordPrefixAt(lineText, params.Position.Character), memberCompletionPriority)
			return out
		}
	}

	if qualifier != "" || isDotCompletion {
		result := s.completionAnalysisResult(params.TextDocument.URI)
		if result != nil {
			if qualifier != "" && isDotCompletion {
				if importItems, handled := s.completeImportedModuleMembers(result, params.TextDocument.URI, qualifier, memberPrefix); handled {
					out.Items = importItems
					return out
				}
			}

			tc := result.TC
			astRoot := result.AST
			if tc == nil || astRoot == nil || isDotCompletion {
				completionTC, completionAST := s.typecheckForCompletion(ctx, text, params.TextDocument.URI, params.Position)
				if completionTC != nil && completionAST != nil {
					tc, astRoot = completionTC, completionAST
				}
			}

			if astRoot != nil || result.Index != nil || qualifier != "" {
				currentPkg := ""
				if astRoot != nil {
					currentPkg = currentPackageName(astRoot)
				}

				receiverMutable := true
				addMembers := func(typeStr string, isStatic bool) {
					if typeStr == "" {
						return
					}

					typeStr = strings.TrimSpace(resolveAliasTypeString(result, typeStr))
					baseType := typeStr

					if before, _, ok0 := strings.Cut(typeStr, "<"); ok0 {
						baseType = strings.TrimSpace(before)
					}

					structDef, hasStructDef := (*typechecker.StructDef)(nil), false
					if tc != nil {
						structDef, hasStructDef = tc.GetStruct(baseType)
					}
					if hasStructDef {
						isSamePkg := structDef.Package == currentPkg

						if isSamePkg {
							if (strings.HasSuffix(baseType, "HashMap") || strings.HasSuffix(baseType, "Vec")) && currentPkg != "collections" {
								isSamePkg = false
							}
						}

						for methodName, methodSig := range structDef.Methods {
							if methodSig == nil {
								continue
							}
							if methodSig.Visibility == ast.Private && !isSamePkg {
								continue
							}

							if isStatic {
								if methodSig.IsInstance {
									continue
								}
							} else {
								if !methodSig.IsInstance {
									continue
								}
								if !receiverMutable && (methodSig.Mutable || isBuiltinMutatingMethod(baseType, methodName)) {
									continue
								}
							}

							insertText := methodName
							insertFormat := 1
							insertFormat = 2
							if len(methodSig.Parameters) == 0 {
								insertText = methodName + "()"
							} else {
								insertText = methodName + "($0)"
							}

							detail := methodCompletionDetail(baseType, methodName, methodSig, isStatic)
							detail = specializeGenericSignatureWithParams(detail, typeStr, structDef.TypeParams)
							items = append(items, CompletionItem{
								Label:            methodName,
								Kind:             2,
								Detail:           detail,
								InsertText:       insertText,
								InsertTextFormat: insertFormat,
								Data: completionResolveData{
									Kind:      "method",
									Symbol:    methodName,
									TypeName:  typeStr,
									Signature: detail,
									Mutable:   methodSig.Mutable,
								},
							})
						}

						if !isStatic {
							for fieldName, fieldDef := range structDef.Fields {
								if fieldDef.Visibility == ast.Private && !isSamePkg {
									continue
								}
								detail := specializeGenericSignatureWithParams(fieldDef.Type.String(), typeStr, structDef.TypeParams)
								items = append(items, CompletionItem{
									Label:         fieldName,
									Kind:          5,
									Detail:        detail,
									Documentation: completionDoc(strfmt.Named("Field `{field}` on `{typeName}`.", "field", fieldName, "typeName", baseType)),
								})
							}
						}
					}

					if !hasStructDef {
						if appendBuiltinTypeMethodCompletionsForType(&items, baseType, typeStr, isStatic, receiverMutable) {
							return
						}
					}
					if !hasStructDef || !isStatic {
						if result.Index != nil && result.Index.Structs != nil {
							if st, ok := result.Index.Structs[baseType]; ok && !isStatic {
								for _, f := range st.Fields {
									fieldName, fieldType := splitIndexedFieldCompletion(f)
									fieldType = specializeGenericSignatureWithParams(fieldType, typeStr, st.TypeParams)
									items = append(items, CompletionItem{
										Label:         fieldName,
										Kind:          5,
										Detail:        fieldType,
										Documentation: completionDoc(strfmt.Named("Field on `{typeName}`.", "typeName", baseType)),
									})
								}
							}
						}
						appendIndexedMethodCompletions(&items, result.Index, baseType, isStatic)
						appendTextualTypeMemberCompletions(&items, text, baseType, isStatic)
					}
				}

				typeStr := ""
				isStatic := false

				if astRoot != nil && isDotCompletion && (qualifier == "" || result.Imports[qualifier] == "") {
					node := findNode(astRoot, params.Position.Line+1, params.Position.Character+1)

					switch n := node.(type) {
					case *ast.FieldAccessExpression:
						typeStr = tc.GetNodeType(n.Object)
						if ident, ok := n.Object.(*ast.Identifier); ok {
							locals := collectLocalSymbols(astRoot, params.Position.Line+1)
							local, isLocal := locals[ident.Value]
							if isLocal {
								receiverMutable = local.Mutable
							}

							isGlobalVar := false
							if result.Index != nil && result.Index.Vars != nil {
								_, isGlobalVar = result.Index.Vars[ident.Value]
							}

							if !isLocal && !isGlobalVar {
								base := typeStr
								if bracket := strings.Index(base, "<"); bracket != -1 {
									base = base[:bracket]
								}
								if _, ok := tc.GetStruct(base); ok {
									isStatic = true
								} else if result.Index != nil && result.Index.Structs != nil {
									if _, ok := result.Index.Structs[base]; ok {
										isStatic = true
									}
								}
								if base == ident.Value {
									isStatic = true
								}
							}
						}
					case *ast.MethodCallExpression:
						typeStr = tc.GetNodeType(n.Object)
					}
				}

				if typeStr == "" && qualifier != "" && astRoot != nil {
					locals := collectLocalSymbols(astRoot, params.Position.Line+1)
					if local, ok := locals[qualifier]; ok && local.Node != nil {
						receiverMutable = local.Mutable
						if tc != nil {
							typeStr = tc.GetNodeType(local.Node)
						}
						if typeStr == "" {
							typeStr = local.Type
						}
						isStatic = false
					}
				}
				if typeStr == "" && qualifier != "" {
					typeStr = declaredLocalTypeBefore(text, params.Position.Line, qualifier)
					if typeStr != "" {
						isStatic = false
						if mutable, ok := declaredLocalMutableBefore(text, params.Position.Line, qualifier); ok {
							receiverMutable = mutable
						}
					}
				}
				if typeStr == "" && qualifier != "" && hasBuiltinStaticCompletionType(qualifier) {
					typeStr = qualifier
					isStatic = true
				}

				if isDotCompletion && qualifier != "" {
					s.appendEnumVariantCompletions(&items, result, params.TextDocument.URI, qualifier)
				}

				addMembers(typeStr, isStatic)

				if len(items) == 0 && qualifier != "" && hasBuiltinStaticCompletionType(qualifier) {
					appendBuiltinTypeMethodCompletions(&items, qualifier, true)
				}

				if isDotCompletion && memberPrefix != "" {
					items = filterCompletionItemsByPrefix(items, memberPrefix)
				}

				prefix := memberPrefix
				if prefix == "" && !isDotCompletion {
					prefix = qualifier
				}
				out.Items = rankCompletionItems(items, prefix, memberCompletionPriority)
				return out
			}
		}
	}

	if result := s.completionAnalysisResult(params.TextDocument.URI); result != nil && result.Index != nil {
		seen := make(map[string]bool)
		for _, sym := range sortedSymbols(result.Index) {
			insertText := sym.Name
			insertFormat := 1
			sig := SignatureInfo{}
			if result.Index.Sigs != nil {
				sig = result.Index.Sigs[sym.Name]
			}
			doc := ""
			if result.Index.Docs != nil {
				doc = result.Index.Docs[sym.Name]
			}
			if sym.Kind == "func" || sym.Kind == "method" {
				insertFormat = 2
				insertText = sym.Name + "($0)"
			}

			items = append(items, CompletionItem{
				Label:            sym.Name,
				Detail:           sym.Kind,
				Kind:             completionKind(sym.Kind),
				InsertText:       insertText,
				InsertTextFormat: insertFormat,
				Data: completionResolveData{
					Kind:      sym.Kind,
					Symbol:    sym.Name,
					Signature: sig.Label,
					Doc:       doc,
					Mutable:   sig.Mutable,
					Source:    sym.Location.URI,
				},
			})

			seen[sym.Name] = true
		}

		if result.AST != nil {
			locals := collectLocalSymbols(result.AST, params.Position.Line+1)
			for name, local := range locals {
				if seen[name] {
					continue
				}
				detail := local.Detail
				if result.TC != nil && local.Node != nil {
					if t := result.TC.GetNodeType(local.Node); t != "" {
						detail = t
					}
				}
				if detail == "var" || detail == "const" || detail == "param" {
					if local.Type != "" {
						detail = local.Type
					}
				}
				items = append(items, CompletionItem{
					Label:  name,
					Detail: detail,
					Kind:   6,
				})
				seen[name] = true
			}
		}
		for alias, importPath := range result.Imports {
			if alias == "" || seen[alias] {
				continue
			}

			items = append(items, CompletionItem{
				Label:  alias,
				Kind:   9,
				Detail: "import " + importPath,
			})

			seen[alias] = true
		}

		for name := range builtins.Modules {
			if seen[name] {
				continue
			}

			items = append(items, CompletionItem{
				Label:  name,
				Kind:   9,
				Detail: "builtin module",
			})

			seen[name] = true
		}
		for name := range builtins.TypeConstructors {
			if seen[name] {
				continue
			}
			items = append(items, CompletionItem{
				Label:  name,
				Kind:   25,
				Detail: "builtin type",
			})
			seen[name] = true
		}
		for name := range builtins.Builtins {
			if seen[name] || strings.HasPrefix(name, "__") {
				continue
			}
			detail := "builtin func"
			insertText := name
			insertFormat := 1

			if sig, ok := builtinSignatures[name]; ok {
				detail = sig
				// Add snippet formatting for functions
				if strings.Contains(sig, "->") {
					insertFormat = 2
					if !strings.Contains(sig, "()") {
						insertText = name + "($0)"
					} else {
						insertText = name + "()"
					}
				}
			} else if strings.Contains(detail, "(") {
				insertFormat = 2
				insertText = name + "($0)"
			}

			items = append(items, CompletionItem{
				Label:            name,
				Kind:             3,
				Detail:           detail,
				Documentation:    completionDoc("Built-in function `" + name + "`."),
				InsertText:       insertText,
				InsertTextFormat: insertFormat,
				Data: completionResolveData{
					Kind:      "builtin",
					Symbol:    name,
					Signature: detail,
				},
			})
			seen[name] = true
		}
		for _, name := range s.getStdPackages() {
			if seen[name] {
				continue
			}
			items = append(items, CompletionItem{
				Label:  name,
				Kind:   9,
				Detail: "std package (import required)",
			})
			seen[name] = true
		}
	}

	keywords := []string{
		"package",
		"import",
		"as",
		"struct",
		"enum",
		"impl",
		"func",
		"pub",
		"var",
		"mut",
		"const",
		"return",
		"try",
		"if",
		"else",
		"while",
		"for",
		"in",
		"switch",
		"case",
		"default",
		"break",
		"continue",
		"defer",
		"panic",
		"unsafe",
		"type",
		"alias",
		"true",
		"false",
		"nil",
		"void",
	}

	for _, kw := range keywords {
		items = append(items, CompletionItem{
			Label: kw,
			Kind:  14,
		})
	}

	for _, typ := range completionTypeNames {
		kind := completionKind("type")
		detail := "type"
		insertText := typ
		insertFormat := 1

		// Special handling for generic types
		switch typ {
		case "Vec":
			insertText = "Vec<${1:T}, ${2:_}>"
			insertFormat = 2
			detail = "Vec<T, Size> - dynamic or fixed-size vector"
		case "HashMap":
			insertText = "HashMap<${1:K}, ${2:V}>"
			insertFormat = 2
			detail = "HashMap<K, V> - hash map with keys and values"
		case "Result":
			insertText = "Result<${1:T}, ${2:E}>"
			insertFormat = 2
			detail = "Result<T, E> - success or error value"
		case "Map":
			insertText = "Map<${1:K}, ${2:V}>"
			insertFormat = 2
			detail = "Map<K, V> - ordered map"
		case "Array":
			insertText = "Array<${1:T}, ${2:N}>"
			insertFormat = 2
			detail = "Array<T, N> - fixed-size array"
		case "Slice":
			insertText = "Slice<${1:T}>"
			insertFormat = 2
			detail = "Slice<T> - view into array elements"
		}

		items = append(items, CompletionItem{
			Label:            typ,
			Kind:             kind,
			Detail:           detail,
			Documentation:    completionDoc(detail),
			InsertText:       insertText,
			InsertTextFormat: insertFormat,
			Data: completionResolveData{
				Kind:   "type",
				Symbol: typ,
			},
		})
	}

	autoImportPrefix := qualifier
	if autoImportPrefix == "" {
		autoImportPrefix = wordPrefixAt(lineText, params.Position.Character)
	}
	s.appendAutoImportCompletions(&items, s.completionAnalysisResult(params.TextDocument.URI), params.TextDocument.URI, autoImportPrefix)
	if autoImportPrefix != "" {
		s.ensureWorkspaceIndexes(ctx)
		s.appendLocalAutoImportCompletions(&items, s.completionAnalysisResult(params.TextDocument.URI), params.TextDocument.URI, autoImportPrefix)
	}

	snippets := []struct {
		label  string
		text   string
		detail string
	}{
		// Function definitions
		{"func", "func ${1:name}(${2:params}) -> ${3:void} {\n\t$0\n}", "Function definition"},
		{"pubfunc", "pub func ${1:name}(${2:params}) -> ${3:void} {\n\t$0\n}", "Public function"},
		{"mutfunc", "mut func ${1:name}(${2:params}) -> ${3:void} {\n\t$0\n}", "Mutable function"},

		// Control flow
		{"if", "if ${1:condition} {\n\t$0\n}", "If statement"},
		{"else", "else {\n\t$0\n}", "Else block"},
		{"for", "for ${1:item} in ${2:iter} {\n\t$0\n}", "For loop"},
		{"while", "while ${1:condition} {\n\t$0\n}", "While loop"},
		{"switch", "switch ${1:expr} {\ncase ${2:pattern} {\n\t$0\n}\n}", "Switch statement"},
		{"match", "match ${1:value} {\n\t${2:pattern} => $0\n}", "Match expression"},

		// Data structures
		{"struct", "struct ${1:Name} {\n\t${2:field}: ${3:Type},\n}", "Struct definition"},
		{"enum", "enum ${1:Name} {\n\t${2:Case},\n}", "Enum definition"},
		{"impl", "impl ${1:Name} {\n\tpub func ${2:name}(mut self) {\n\t\t$0\n\t}\n}", "Implementation block"},

		// Variable declarations
		{"var", "var ${1:name}: ${2:Type} = ${3:value}", "Variable declaration"},
		{"const", "const ${1:NAME}: ${2:Type} = ${3:value}", "Constant declaration"},
		{"mut", "mut var ${1:name}: ${2:Type} = ${3:value}", "Mutable variable"},

		// Modifiers
		{"pub", "pub ", "Public visibility"},
		{"priv", "priv ", "Private visibility"},

		// Common patterns
		{"println", "println(${1:values})", "Print line"},
		{"dbg", "dbg(${1:values})", "Debug inspect value"},
		{"result", "Result<${1:T}, ${2:E}>", "Result type"},
		{"vec", "Vec<${1:T}, ${2:_}>", "Vector type"},
		{"map", "HashMap<${1:K}, ${2:V}>", "Hash map type"},
	}

	for _, s := range snippets {
		items = append(items, CompletionItem{
			Label:            s.label,
			Kind:             15, // Snippet
			Detail:           s.detail,
			InsertText:       s.text,
			InsertTextFormat: 2, // Snippet
		})
	}

	prefix := qualifier
	out.Items = rankCompletionItems(items, prefix, generalCompletionPriority)
	return out
}

func (s *Server) completionAnalysisResult(uri string) *AnalysisResult {
	if result, _ := s.analysisResult(uri); result != nil {
		return result
	}
	return s.resultForDocument(uri)
}

func completeSwitchCaseVariants(text string, pos Position) []CompletionItem {
	line := lineAt(text, pos.Line)
	before := line
	if pos.Character >= 0 && pos.Character < len(line) {
		before = line[:pos.Character]
	}
	trimmed := strings.TrimSpace(before)
	if trimmed != "case" && !strings.HasPrefix(trimmed, "case ") {
		return nil
	}
	switchLine, switchVar, closeLine, ok := textualSwitchAt(text, pos.Line)
	if !ok || switchVar == "" {
		return nil
	}
	typeName := baseTypeName(declaredLocalTypeBefore(text, switchLine, switchVar))
	if typeName == "" {
		return nil
	}
	seen := textualSwitchCaseNames(text, switchLine, closeLine)
	items := []CompletionItem{}
	for _, variant := range textualEnumVariantInfos(text, typeName) {
		if variant.Name == "" || seen[variant.Name] {
			continue
		}
		insertText := variant.Name
		insertFormat := 1
		detail := typeName + "." + variant.Name
		if len(variant.Fields) > 0 {
			args := make([]string, len(variant.Fields))
			for i := range args {
				args[i] = "${" + strconv.Itoa(i+1) + ":_}"
			}
			insertText += "(" + strings.Join(args, ", ") + ")"
			insertFormat = 2
			detail += "(" + strings.Join(variant.Fields, ", ") + ")"
		}
		items = append(items, CompletionItem{
			Label:            variant.Name,
			Kind:             completionKind("enumMember"),
			Detail:           detail,
			InsertText:       insertText,
			InsertTextFormat: insertFormat,
		})
	}
	return items
}

var primitiveTypeNames = []string{
	"any",
	"bool",
	"int",
	"int8",
	"int16",
	"int32",
	"int64",
	"uint",
	"uint8",
	"uint16",
	"uint32",
	"uint64",
	"float32",
	"float64",
	"char",
	"string",
	"void",
}

var builtinGenericTypeNames = []string{
	"Vec",
	"HashMap",
	"Map",
	"Array",
	"Slice",
	"Range",
	"Result",
	"Option",
	"Error",
	"null",
}

var completionTypeNames = append(
	append([]string{}, primitiveTypeNames...),
	builtinGenericTypeNames...,
)

func (s *Server) completeStructLiteralFields(
	ctx context.Context,
	result *AnalysisResult,
	text,
	uri string,
	pos Position,
) []CompletionItem {
	if result == nil {
		return nil
	}

	tc := result.TC
	astRoot := result.AST
	if tc == nil || astRoot == nil {
		tc, astRoot = s.typecheckForCompletion(ctx, text, uri, pos)
	}

	if tc == nil || astRoot == nil {
		return nil
	}

	line := pos.Line + 1
	col := pos.Character + 1
	lit := findStructLiteralAt(astRoot, line, col)
	typeName := structLiteralTypeName(lit, text, pos)
	if typeName == "" {
		return nil
	}

	typeName = resolveAliasTypeString(result, typeName)
	currentPkg := currentPackageName(astRoot)
	existing := existingStructLiteralFields(lit)

	structDef, ok := tc.GetStruct(typeName)
	if !ok {
		base := baseTypeName(typeName)
		if base != typeName {
			structDef, ok = tc.GetStruct(base)
		}
	}
	if ok {
		items := make([]CompletionItem, 0, len(structDef.Fields))
		isSamePkg := structDef.Package == currentPkg
		for fieldName, fieldDef := range structDef.Fields {
			if existing[fieldName] {
				continue
			}
			if fieldDef.Visibility == ast.Private && !isSamePkg {
				continue
			}
			detail := specializeGenericSignatureWithParams(fieldDef.Type.String(), typeName, structDef.TypeParams)
			items = append(items, CompletionItem{
				Label:         fieldName,
				Kind:          5, // Field
				Detail:        detail,
				Documentation: completionDoc(strfmt.Named("Field `{field}` on `{typeName}`.", "field", fieldName, "typeName", typeName)),
			})
		}
		if len(items) > 0 {
			return items
		}
	}

	st, ok := s.findStructInfoByType(result, uri, typeName)
	if !ok {
		return nil
	}
	items := make([]CompletionItem, 0, len(st.Fields))
	for _, field := range st.Fields {
		name := field
		detail := "field"
		if parts := strings.SplitN(field, ":", 2); len(parts) == 2 {
			name = strings.TrimSpace(parts[0])
			detail = strings.TrimSpace(parts[1])
		}
		detail = specializeGenericSignatureWithParams(detail, typeName, st.TypeParams)
		if name == "" || existing[name] {
			continue
		}
		items = append(items, CompletionItem{
			Label:         name,
			Kind:          5, // Field
			Detail:        detail,
			Documentation: completionDoc(strfmt.Named("Field `{field}` on `{typeName}`.", "field", name, "typeName", typeName)),
		})
	}
	return items
}

func (s *Server) completeImportedModuleMembers(
	result *AnalysisResult,
	uri,
	qualifier,
	memberPrefix string,
) (
	[]CompletionItem,
	bool,
) {
	if result == nil || result.Imports == nil {
		return nil, false
	}
	importPath, ok := result.Imports[qualifier]
	if !ok {
		return nil, false
	}

	path := s.resolveImportPath(uriToPath(uri), importPath)
	if path == "" {
		return nil, true
	}

	modIndex := s.getOrIndexFileIncludingPrivate(path)
	items := make([]CompletionItem, 0)
	for _, sym := range sortedSymbols(modIndex) {
		if strings.Contains(sym.Name, ".") {
			continue
		}

		insertText := sym.Name
		insertFormat := 1
		detail := sym.Kind
		tags := []int(nil)
		docPrefix := ""
		if !sym.Exported {
			detail = "private " + detail
			tags = []int{1}
			docPrefix = strfmt.Named("`{symbol}` is private to module `{module}`.\n\n", "Symbol", sym.Name, "Module", qualifier)
		}
		if sym.Kind == "func" {
			insertFormat = 2
			insertText = sym.Name + "($0)"
			if modIndex.Sigs != nil {
				if sig, ok := modIndex.Sigs[sym.Name]; ok && sig.Label != "" {
					detail = sig.Label
				}
			}
		}
		doc := ""
		if modIndex.Docs != nil {
			doc = modIndex.Docs[sym.Name]
		}
		doc = docPrefix + doc

		items = append(items, CompletionItem{
			Label:            sym.Name,
			Detail:           detail,
			Documentation:    completionDoc(doc),
			Kind:             completionKind(sym.Kind),
			InsertText:       insertText,
			InsertTextFormat: insertFormat,
			Tags:             tags,
			Data: completionResolveData{
				Kind:   sym.Kind,
				Symbol: sym.Name,
				Module: qualifier,
			},
		})
	}

	if memberPrefix != "" {
		items = filterCompletionItemsByPrefix(items, memberPrefix)
	}
	items = rankCompletionItems(items, memberPrefix, memberCompletionPriority)

	return items, true
}

func (s *Server) appendEnumVariantCompletions(
	items *[]CompletionItem,
	result *AnalysisResult,
	uri,
	qualifier string,
) {
	if result == nil || qualifier == "" {
		return
	}
	enumName, enumInfo, ok := s.enumInfoForQualifier(result, uri, qualifier)
	if !ok {
		return
	}
	for _, variant := range enumVariantDetails(enumInfo) {
		if variant.Name == "" {
			continue
		}
		insertText := variant.Name
		insertFormat := 1
		detail := enumName + "." + variant.Name
		if len(variant.Fields) > 0 {
			insertText = variant.Name + "($0)"
			insertFormat = 2
			detail = strfmt.Named(
				"{Enum}.{Variant}({Fields})",
				"Enum", enumName,
				"Variant", variant.Name,
				"Fields", strings.Join(variant.Fields, ", "),
			)
		}
		*items = append(*items, CompletionItem{
			Label:            variant.Name,
			Kind:             completionKind("enumMember"),
			Detail:           detail,
			Documentation:    completionDoc(strfmt.Named("Variant `{Variant}` of enum `{Enum}`.", "Variant", variant.Name, "Enum", enumName)),
			InsertText:       insertText,
			InsertTextFormat: insertFormat,
			Data: completionResolveData{
				Kind:     "enumMember",
				Symbol:   variant.Name,
				TypeName: enumName,
			},
		})
	}
}

func (s *Server) enumInfoForQualifier(
	result *AnalysisResult,
	uri,
	qualifier string,
) (string, EnumInfo, bool) {
	if qualifier == "Result" {
		return qualifier, EnumInfo{
			Name:     "Result",
			Variants: []string{"Ok", "Err"},
			VariantDetails: []EnumVariantInfo{
				{Name: "Ok", Fields: []string{"T"}},
				{Name: "Err", Fields: []string{"E"}},
			},
		}, true
	}
	if result != nil && result.Index != nil && result.Index.Enums != nil {
		if enumInfo, ok := result.Index.Enums[qualifier]; ok {
			return qualifier, enumInfo, true
		}
	}

	alias, enumName, ok := strings.Cut(qualifier, ".")
	if !ok || alias == "" || enumName == "" || result == nil || result.Imports == nil {
		return "", EnumInfo{}, false
	}
	importPath, ok := result.Imports[alias]
	if !ok {
		return "", EnumInfo{}, false
	}
	path := s.resolveImportPath(uriToPath(uri), importPath)
	if path == "" {
		return "", EnumInfo{}, false
	}
	modIndex := s.getOrIndexFile(path)
	if modIndex == nil || modIndex.Enums == nil {
		return "", EnumInfo{}, false
	}
	enumInfo, ok := modIndex.Enums[enumName]
	if !ok {
		return "", EnumInfo{}, false
	}
	return enumName, enumInfo, true
}

func enumVariantDetails(enumInfo EnumInfo) []EnumVariantInfo {
	if len(enumInfo.VariantDetails) > 0 {
		return enumInfo.VariantDetails
	}
	variants := make([]EnumVariantInfo, 0, len(enumInfo.Variants))
	for _, name := range enumInfo.Variants {
		variants = append(variants, EnumVariantInfo{Name: name})
	}
	return variants
}

var generalCompletionPriority = map[int]int{
	6:  0,  // Variable
	5:  1,  // Field
	3:  2,  // Function
	2:  3,  // Method
	9:  4,  // Module
	25: 5,  // Type
	21: 6,  // Constant
	22: 7,  // Struct
	13: 8,  // Enum
	14: 9,  // Keyword
	15: 10, // Snippet
}

var memberCompletionPriority = map[int]int{
	2:  0, // Method
	5:  1, // Field
	20: 2, // Enum member
	3:  3, // Function
	21: 4, // Constant
	25: 5, // Type
	22: 6, // Struct
	13: 7, // Enum
}

var fieldCompletionPriority = map[int]int{
	5: 0, // Field
}

var importPathCompletionPriority = map[int]int{
	9: 0, // Module/import path
}

func rankCompletionItems(items []CompletionItem, prefix string, kindPriority map[int]int) []CompletionItem {
	items = dedupeCompletionItems(items)
	if len(items) <= 1 {
		return items
	}

	typed := strings.TrimSpace(prefix)
	typedFolded := strings.ToLower(typed)

	scoreLabel := func(label string) int {
		if typed == "" {
			return 4
		}
		if label == typed {
			return 0
		}
		if strings.HasPrefix(label, typed) {
			return 1
		}
		folded := strings.ToLower(label)
		if strings.HasPrefix(folded, typedFolded) {
			return 2
		}
		if strings.Contains(folded, typedFolded) {
			return 3
		}
		return 4
	}

	kindRank := func(kind int) int {
		if rank, ok := kindPriority[kind]; ok {
			return rank
		}
		return 99
	}

	sort.SliceStable(items, func(i, j int) bool {
		li := items[i].Label
		lj := items[j].Label

		si := scoreLabel(li)
		sj := scoreLabel(lj)
		if si != sj {
			return si < sj
		}

		ki := kindRank(items[i].Kind)
		kj := kindRank(items[j].Kind)
		if ki != kj {
			return ki < kj
		}

		ii := completionIntentRank(items[i])
		ij := completionIntentRank(items[j])
		if ii != ij {
			return ii < ij
		}

		if len(li) != len(lj) {
			return len(li) < len(lj)
		}

		return strings.ToLower(li) < strings.ToLower(lj)
	})

	return items
}

func completionIntentRank(item CompletionItem) int {
	typeName := ""
	switch data := item.Data.(type) {
	case completionResolveData:
		typeName = data.TypeName
	case map[string]any:
		if raw, ok := data["typeName"].(string); ok {
			typeName = raw
		}
	}
	if typeName == "" {
		return 100
	}
	if before, _, ok := strings.Cut(typeName, "<"); ok {
		typeName = strings.TrimSpace(before)
	}
	if typeName != "Result" {
		return 100
	}
	switch item.Label {
	case "isOk":
		return 0
	case "isErr":
		return 1
	case "unwrap":
		return 2
	case "unwrapOr":
		return 3
	case "unwrapErr":
		return 4
	default:
		return 50
	}
}

func splitIndexedFieldCompletion(field string) (string, string) {
	name, typ, ok := strings.Cut(field, ":")
	if !ok {
		return strings.TrimSpace(field), "field"
	}
	name = strings.TrimSpace(name)
	typ = strings.TrimSpace(typ)
	if name == "" {
		name = strings.TrimSpace(field)
	}
	if typ == "" {
		typ = "field"
	}
	return name, typ
}

func dedupeCompletionItems(items []CompletionItem) []CompletionItem {
	if len(items) <= 1 {
		return items
	}
	out := make([]CompletionItem, 0, len(items))
	seen := make(map[string]int, len(items))
	for _, item := range items {
		key := item.Label + "\x00" + strconv.Itoa(item.Kind)
		if idx, ok := seen[key]; ok {
			if completionItemScore(item) > completionItemScore(out[idx]) {
				out[idx] = item
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, item)
	}
	return out
}

func completionItemScore(item CompletionItem) int {
	score := 0
	if item.Detail != "" && item.Detail != "field" && item.Detail != "method" {
		score += 4
	}
	if item.Documentation != nil && item.Documentation.Value != "" {
		score += 2
	}
	if item.InsertText != "" {
		score++
	}
	if len(item.AdditionalTextEdits) > 0 {
		score++
	}
	return score
}

func structLiteralTypeName(
	lit *ast.StructLiteral,
	text string,
	pos Position,
) string {

	if lit != nil && lit.Name != nil {
		return lit.Name.Value
	}

	return structLiteralTypeAt(text, pos)
}

func existingStructLiteralFields(lit *ast.StructLiteral) map[string]bool {
	existing := map[string]bool{}
	if lit == nil {
		return existing
	}

	for name := range lit.Fields {
		existing[name] = true
	}

	return existing
}

func currentPackageName(prog *ast.Program) string {
	if prog == nil {
		return ""
	}
	for _, stmt := range prog.Statements {
		if pkgStmt, ok := stmt.(*ast.PackageStatement); ok {
			return pkgStmt.Name.Value
		}
	}
	return ""
}

func methodDetail(sig *typechecker.FunctionSig) string {
	params := []string{}
	for _, p := range sig.Parameters {
		if p != nil {
			params = append(params, p.String())
		}
	}
	ret := "void"
	if sig.ReturnType != nil {
		ret = sig.ReturnType.String()
	}
	mut := ""
	if sig.Mutable {
		mut = "mut "
	}
	return strfmt.Named(
		"{mut}func({params}) -> ({ret})",
		"mut", mut,
		"params", strings.Join(params, ", "),
		"ret", ret,
	)
}

func methodCompletionDetail(
	baseType,
	methodName string,
	sig *typechecker.FunctionSig,
	isStatic bool,
) string {
	if isStatic {
		if info, ok := builtinStaticMethods[baseType][methodName]; ok && info.Signature != "" {
			return info.Signature
		}
	} else if info, ok := builtinMethods[baseType][methodName]; ok && info.Signature != "" {
		return info.Signature
	}
	return methodDetail(sig)
}

func hasBuiltinStaticCompletionType(typeName string) bool {
	_, ok := builtinStaticMethods[typeName]
	return ok
}

func appendBuiltinTypeMethodCompletions(
	items *[]CompletionItem,
	typeName string,
	isStatic bool,
) bool {
	return appendBuiltinTypeMethodCompletionsForType(items, typeName, typeName, isStatic, true)
}

func appendBuiltinTypeMethodCompletionsForType(
	items *[]CompletionItem,
	typeName string,
	fullType string,
	isStatic bool,
	receiverMutable bool,
) bool {
	var methods map[string]builtinMethodInfo
	if isStatic {
		methods = builtinStaticMethods[typeName]
	} else {
		methods = builtinMethods[typeName]
	}

	if len(methods) == 0 {
		return false
	}

	for methodName, info := range methods {
		if strings.HasPrefix(info.Doc, "Deprecated:") {
			continue
		}
		if !isStatic && !receiverMutable && isBuiltinMutatingMethod(typeName, methodName) {
			continue
		}

		insertText, insertFormat := completionInsertTextFromSignature(methodName, info.Signature)
		detail := specializeGenericSignature(info.Signature, fullType)
		*items = append(*items, CompletionItem{
			Label:            methodName,
			Kind:             2, // Method
			Detail:           detail,
			Documentation:    completionDoc(info.Doc),
			InsertText:       insertText,
			InsertTextFormat: insertFormat,
			Data: completionResolveData{
				Kind:      "method",
				Symbol:    methodName,
				TypeName:  fullType,
				Signature: detail,
				Doc:       info.Doc,
				Mutable:   isBuiltinMutatingMethod(typeName, methodName),
			},
		})
	}

	return true
}

func isBuiltinMutatingMethod(typeName, methodName string) bool {
	switch typeName {
	case "Vec":
		switch methodName {
		case "append", "clear", "pop", "push", "remove", "reverse", "set":
			return true
		}
	case "HashMap":
		switch methodName {
		case "clear", "insert", "remove":
			return true
		}
	case "Map":
		switch methodName {
		case "clear", "remove":
			return true
		}
	}
	return false
}

func appendIndexedMethodCompletions(
	items *[]CompletionItem,
	index *FileIndex,
	typeName string,
	isStatic bool,
) {
	if index == nil || index.Symbols == nil {
		return
	}

	prefix := typeName + "."
	for _, sym := range sortedSymbols(index) {
		if sym.Kind != "method" || !strings.HasPrefix(sym.Name, prefix) {
			continue
		}
		methodName := strings.TrimPrefix(sym.Name, prefix)
		if methodName == "" || strings.Contains(methodName, ".") {
			continue
		}

		detail := "method"
		doc := ""
		mutable := false
		insertText := methodName
		insertFormat := 1
		if index.Sigs != nil {
			if sig, ok := index.Sigs[sym.Name]; ok && sig.Label != "" {
				detail = sig.Label
				insertText = methodName + "($0)"
				insertFormat = 2
				doc = sig.Doc
				mutable = sig.Mutable
			}
		}
		if index.Docs != nil && doc == "" {
			doc = index.Docs[sym.Name]
		}
		mutable = mutable || strings.HasPrefix(strings.TrimSpace(detail), "mut ")

		if isStatic {
			// Syntax-only indexes do not yet distinguish static methods from
			// receiver methods. Static built-ins are handled separately.
			continue
		}

		*items = append(*items, CompletionItem{
			Label:            methodName,
			Kind:             2,
			Detail:           detail,
			Documentation:    completionDoc(index.Docs[sym.Name]),
			InsertText:       insertText,
			InsertTextFormat: insertFormat,
			Data: completionResolveData{
				Kind:      "method",
				Symbol:    methodName,
				TypeName:  typeName,
				Signature: detail,
				Doc:       doc,
				Mutable:   mutable,
			},
		})
	}
}

func appendTextualTypeMemberCompletions(
	items *[]CompletionItem,
	text string,
	typeName string,
	isStatic bool,
) {
	if text == "" || typeName == "" || isStatic {
		return
	}
	for _, field := range textualStructFields(text, typeName) {
		if field.Name == "" {
			continue
		}
		*items = append(*items, CompletionItem{
			Label:         field.Name,
			Kind:          5,
			Detail:        field.Type,
			Documentation: completionDoc(strfmt.Named("Field `{field}` on `{typeName}`.", "field", field.Name, "typeName", typeName)),
		})
	}
	for _, method := range textualImplMethods(text, typeName) {
		if method == "" {
			continue
		}
		*items = append(*items, CompletionItem{
			Label:            method,
			Kind:             2,
			Detail:           "method",
			Documentation:    completionDoc(strfmt.Named("Method `{method}` on `{typeName}`.", "method", method, "typeName", typeName)),
			InsertText:       method + "($0)",
			InsertTextFormat: 2,
			Data: completionResolveData{
				Kind:     "method",
				Symbol:   method,
				TypeName: typeName,
				Mutable:  true,
			},
		})
	}
}

type textualField struct {
	Name string
	Type string
}

func textualStructFields(text, typeName string) []textualField {
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "struct "+typeName) ||
			!isWordBoundary(trimmed, len("struct "), len(typeName)) {
			continue
		}
		fields := []textualField{}
		for i++; i < len(lines); i++ {
			fieldLine := strings.TrimSpace(lines[i])
			if strings.HasPrefix(fieldLine, "}") {
				return fields
			}
			name, typ, ok := strings.Cut(fieldLine, ":")
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			typ = strings.TrimSpace(strings.TrimRight(typ, ","))
			if name != "" && bakIdentifierPattern.MatchString(name) {
				fields = append(fields, textualField{Name: name, Type: typ})
			}
		}
		return fields
	}
	return nil
}

func textualImplMethods(text, typeName string) []string {
	lines := strings.Split(text, "\n")
	methods := []string{}
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "impl "+typeName) ||
			!isWordBoundary(trimmed, len("impl "), len(typeName)) {
			continue
		}
		for i++; i < len(lines); i++ {
			methodLine := strings.TrimSpace(lines[i])
			if strings.HasPrefix(methodLine, "}") {
				break
			}
			if name := textualMethodName(methodLine); name != "" {
				methods = append(methods, name)
			}
		}
	}
	return methods
}

func textualMethodName(line string) string {
	line = strings.TrimPrefix(line, "pub ")
	line = strings.TrimPrefix(line, "mut ")
	if !strings.HasPrefix(line, "func ") {
		return ""
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "func "))
	open := strings.Index(rest, "(")
	if open <= 0 {
		return ""
	}
	name := strings.TrimSpace(rest[:open])
	if !bakIdentifierPattern.MatchString(name) {
		return ""
	}
	return name
}

func completionInsertTextFromSignature(methodName, signature string) (string, int) {
	if strings.Contains(signature, "()") {
		return methodName + "()", 2
	}
	if signature != "" {
		return methodName + "($0)", 2
	}
	return methodName, 1
}

type completionResolveData struct {
	Kind       string `json:"kind,omitempty"`
	Symbol     string `json:"symbol,omitempty"`
	Module     string `json:"module,omitempty"`
	TypeName   string `json:"typeName,omitempty"`
	ImportPath string `json:"importPath,omitempty"`
	Alias      string `json:"alias,omitempty"`
	Signature  string `json:"signature,omitempty"`
	Doc        string `json:"doc,omitempty"`
	Source     string `json:"source,omitempty"`
	Mutable    bool   `json:"mutable,omitempty"`
}

func completionDoc(doc string) *MarkupContent {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return nil
	}
	return &MarkupContent{Kind: "markdown", Value: doc}
}

func (s *Server) handleCompletionResolve(req Request) CompletionItem {
	item, ok := requestParams[CompletionItem](req)
	if !ok {
		return CompletionItem{}
	}

	data := completionResolveDataFromAny(item.Data)
	if data.Kind == "" && item.Documentation != nil {
		return item
	}
	switch data.Kind {
	case "autoImport":
		lines := []string{strfmt.Named("Auto-imports `{symbol}` from `{alias}` (`{path}`).", "symbol", data.Symbol, "alias", data.Alias, "path", data.ImportPath)}
		if data.Signature != "" {
			lines = append(lines, "", "```bak\n"+data.Signature+"\n```")
		}
		if data.Doc != "" {
			lines = append(lines, "", data.Doc)
		}
		item.Documentation = completionDoc(strings.Join(lines, "\n"))
	case "method":
		lines := []string{}
		if data.Signature != "" {
			lines = append(lines, "```bak\n"+data.Signature+"\n```")
		}
		if data.TypeName != "" {
			lines = append(lines, strfmt.Named("Receiver: `{typeName}`.", "typeName", data.TypeName))
		}
		if data.Mutable {
			lines = append(lines, "Mutates the receiver.")
		}
		if data.Doc != "" {
			lines = append(lines, "", data.Doc)
		}
		if len(lines) == 0 && data.TypeName != "" {
			lines = append(lines, strfmt.Named("Method `{symbol}` on `{typeName}`.", "symbol", data.Symbol, "typeName", data.TypeName))
		}
		item.Documentation = completionDoc(strings.Join(lines, "\n"))
	case "func":
		lines := []string{}
		if data.Signature != "" {
			lines = append(lines, "```bak\n"+data.Signature+"\n```")
		}
		if data.Doc != "" {
			lines = append(lines, data.Doc)
		}
		if data.Source != "" {
			lines = append(lines, strfmt.Named("Source: `{source}`.", "source", data.Source))
		}
		item.Documentation = completionDoc(strings.Join(lines, "\n"))
	case "builtin":
		lines := []string{"Built-in function `" + data.Symbol + "`."}
		if data.Signature != "" {
			lines = append(lines, "", "```bak\n"+data.Signature+"\n```")
		}
		item.Documentation = completionDoc(strings.Join(lines, "\n"))
	case "type":
		lines := []string{"Type `" + data.Symbol + "`."}
		if data.Doc != "" {
			lines = append(lines, "", data.Doc)
		}
		item.Documentation = completionDoc(strings.Join(lines, "\n"))
	}
	return item
}

func completionResolveDataFromAny(data any) completionResolveData {
	if data == nil {
		return completionResolveData{}
	}
	if typed, ok := data.(completionResolveData); ok {
		return typed
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return completionResolveData{}
	}
	var out completionResolveData
	_ = json.Unmarshal(raw, &out)
	return out
}

func (s *Server) appendAutoImportCompletions(items *[]CompletionItem, result *AnalysisResult, uri string, prefix string) {
	if result == nil {
		return
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return
	}
	imported := map[string]bool{}
	for alias := range result.Imports {
		imported[alias] = true
	}
	currentDir := filepath.Dir(uriToPath(uri))
	insertPos := findImportInsertPosition(result)
	seen := map[string]bool{}
	for _, importPath := range s.getStdImportPaths() {
		path := s.resolveImportPath(uriToPath(uri), importPath)
		if path == "" {
			continue
		}
		idx := s.getOrIndexFile(path)
		if idx == nil {
			continue
		}
		alias := importAliasForPath(importPath)
		if alias == "" || importPathAlreadyPresent(result.Imports, importPath, currentDir) {
			continue
		}
		var ok bool
		alias, ok = uniqueImportAlias(imported, alias)
		if !ok {
			continue
		}
		for _, sym := range sortedSymbols(idx) {
			if sym.Name == "" || !sym.Exported || strings.Contains(sym.Name, ".") {
				continue
			}
			if prefix != "" && !strings.HasPrefix(strings.ToLower(sym.Name), strings.ToLower(prefix)) {
				continue
			}
			key := alias + "." + sym.Name
			if seen[key] {
				continue
			}
			seen[key] = true

			insertText := alias + "." + sym.Name
			insertFormat := 1
			if sym.Kind == "func" {
				insertText += "($0)"
				insertFormat = 2
			}
			sig := SignatureInfo{}
			if idx.Sigs != nil {
				sig = idx.Sigs[sym.Name]
			}
			doc := ""
			if idx.Docs != nil {
				doc = idx.Docs[sym.Name]
			}

			importLine := strfmt.Named("import {Alias} \"{ImportPath}\"\n", "Alias", alias, "ImportPath", importPath)
			*items = append(*items, CompletionItem{
				Label:            sym.Name,
				Kind:             completionKind(sym.Kind),
				Detail:           "auto import from " + alias,
				Documentation:    completionDoc(strfmt.Named("Auto-import from `{alias}`.", "alias", alias)),
				InsertText:       insertText,
				InsertTextFormat: insertFormat,
				AdditionalTextEdits: []TextEdit{{
					Range:   Range{Start: insertPos, End: insertPos},
					NewText: importLine,
				}},
				Data: completionResolveData{
					Kind:       "autoImport",
					Symbol:     sym.Name,
					Alias:      alias,
					ImportPath: importPath,
					Signature:  sig.Label,
					Doc:        doc,
					Mutable:    sig.Mutable,
				},
			})
		}
	}
}

func (s *Server) appendLocalAutoImportCompletions(items *[]CompletionItem, result *AnalysisResult, uri string, prefix string) {
	if result == nil || strings.TrimSpace(prefix) == "" {
		return
	}
	prefixFolded := strings.ToLower(strings.TrimSpace(prefix))
	currentPath := uriToPath(uri)
	currentDir := filepath.Dir(currentPath)
	imported := map[string]bool{}
	for alias := range result.Imports {
		imported[alias] = true
	}
	seen := map[string]bool{}
	for _, idx := range s.indexSnapshot() {
		if idx == nil || idx.Symbols == nil {
			continue
		}
		for _, sym := range sortedSymbols(idx) {
			if !sym.Exported || sym.Location.URI == "" || sym.Location.URI == uri || strings.Contains(sym.Name, ".") {
				continue
			}
			if !strings.HasPrefix(strings.ToLower(sym.Name), prefixFolded) {
				continue
			}
			targetPath := uriToPath(sym.Location.URI)
			importPath, alias := localImportPathAndAlias(currentDir, targetPath)
			if importPath == "" || alias == "" || importPathAlreadyPresent(result.Imports, importPath, currentDir) {
				continue
			}
			var ok bool
			alias, ok = uniqueImportAlias(imported, alias)
			if !ok {
				continue
			}
			key := alias + "." + sym.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			insertText := alias + "." + sym.Name
			insertFormat := 1
			if sym.Kind == "func" {
				insertText += "($0)"
				insertFormat = 2
			}
			sig := SignatureInfo{}
			if idx.Sigs != nil {
				sig = idx.Sigs[sym.Name]
			}
			doc := ""
			if idx.Docs != nil {
				doc = idx.Docs[sym.Name]
			}
			insertPos := findImportInsertPosition(result)
			*items = append(*items, CompletionItem{
				Label:            sym.Name,
				Kind:             completionKind(sym.Kind),
				Detail:           "auto import from " + alias,
				Documentation:    completionDoc(strfmt.Named("Auto-import from `{Alias}`.", "Alias", alias)),
				InsertText:       insertText,
				InsertTextFormat: insertFormat,
				AdditionalTextEdits: []TextEdit{{
					Range:   Range{Start: insertPos, End: insertPos},
					NewText: strfmt.Named("import {Alias} \"{ImportPath}\"\n", "Alias", alias, "ImportPath", importPath),
				}},
				Data: completionResolveData{
					Kind:       "autoImport",
					Symbol:     sym.Name,
					Alias:      alias,
					ImportPath: importPath,
					Signature:  sig.Label,
					Doc:        doc,
					Mutable:    sig.Mutable,
				},
			})
		}
	}
}

func importAliasForPath(importPath string) string {
	parts := strings.Split(strings.Trim(importPath, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	alias := strings.TrimSuffix(parts[len(parts)-1], ".bak")
	if alias == "" && len(parts) > 1 {
		alias = parts[len(parts)-2]
	}
	return alias
}

func specializeGenericSignature(signature, fullType string) string {
	return specializeGenericSignatureWithParams(signature, fullType, builtinGenericParamNames(baseTypeName(fullType)))
}

func builtinGenericParamNames(base string) []string {
	switch base {
	case "Vec":
		return []string{"T", "N"}
	case "HashMap", "Map":
		return []string{"K", "V"}
	case "Result":
		return []string{"T", "E"}
	case "Option":
		return []string{"T"}
	default:
		return nil
	}
}

func specializeGenericSignatureWithParams(signature, fullType string, params []string) string {
	args := genericTypeArgs(fullType)
	if len(args) == 0 || len(params) == 0 {
		return signature
	}
	replacements := map[string]string{}
	for i, param := range params {
		if i >= len(args) {
			break
		}
		if param != "" && args[i] != "" {
			replacements[param] = args[i]
		}
	}
	for from, to := range replacements {
		signature = replaceTypeParam(signature, from, to)
	}
	return signature
}

func genericTypeArgs(typeName string) []string {
	start := strings.Index(typeName, "<")
	end := strings.LastIndex(typeName, ">")
	if start == -1 || end == -1 || end <= start {
		return nil
	}
	body := typeName[start+1 : end]
	args := []string{}
	depth := 0
	last := 0
	for i, ch := range body {
		switch ch {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(body[last:i]))
				last = i + 1
			}
		}
	}
	args = append(args, strings.TrimSpace(body[last:]))
	return args
}

func replaceTypeParam(signature, from, to string) string {
	if from == "" {
		return signature
	}
	var out strings.Builder
	for i := 0; i < len(signature); {
		if strings.HasPrefix(signature[i:], from) &&
			(i == 0 || !isWordChar(signature[i-1])) &&
			(i+len(from) == len(signature) || !isWordChar(signature[i+len(from)])) {
			out.WriteString(to)
			i += len(from)
			continue
		}
		out.WriteByte(signature[i])
		i++
	}
	return out.String()
}

var builtinSignatures = map[string]string{
	// Conversion functions
	"fromChars": "fromChars(chars: Vec<char, _>) -> (string)",
	"int":       "int(value: any) -> (int | Result<int,string>)",
	"float":     "float(value: any) -> (float64 | Result<float64,string>)",
	"string":    "string(value: any) -> (string)",
	"char":      "char(value: int|char) -> (char)",

	// Output functions
	"print":    "print(values: any...) -> (void)",
	"println":  "println(values: any...) -> (void)",
	"dbg":      "dbg(values: any...) -> (void)",
	"eprint":   "eprint(values: any...) -> (void)",
	"eprintln": "eprintln(values: any...) -> (void)",

	// Type inspection
	"type":    "type(value: any) -> (string)",
	"typeof":  "typeof(value: any) -> (string)",
	"fields":  "fields(typeOrValue: any) -> (Vec<string, _>)",
	"methods": "methods(typeOrValue: any) -> (Vec<string, _>)",

	// String operations
	"concat": "concat(values: string...) -> (string)",

	// Error handling
	"panic": "panic(msg: string) -> (void)",
	"error": "error(msg: string) -> (Result<any,string>)",

	// OS/System functions
	"args":       "args() -> (Vec<string, _>)",
	"exit":       "exit(code: int) -> (void)",
	"getenv":     "getenv(name: string) -> (Result<string, string>)",
	"setenv":     "setenv(name: string, value: string) -> (void)",
	"cwd":        "cwd() -> (Result<string, string>)",
	"chdir":      "chdir(path: string) -> (Result<void, string>)",
	"executable": "executable() -> (Result<string, string>)",
	"hostname":   "hostname() -> (Result<string, string>)",
	"tempDir":    "tempDir() -> (string)",
	"homeDir":    "homeDir() -> (Result<string, string>)",
	"sleep":      "sleep(ms: int) -> (void)",
	"timeNow":    "timeNow() -> (int)",
	"threadID":   "threadID() -> (int)",

	// File operations
	"readFile":   "readFile(path: string) -> (Result<string, string>)",
	"writeFile":  "writeFile(path: string, content: string) -> (Result<void, string>)",
	"appendFile": "appendFile(path: string, content: string) -> (Result<void, string>)",
	"fileExists": "fileExists(path: string) -> (bool)",
	"isFile":     "isFile(path: string) -> (bool)",
	"isDir":      "isDir(path: string) -> (bool)",
	"readDir":    "readDir(path: string) -> (Result<Vec<string, _>, string>)",
	"remove":     "remove(path: string) -> (Result<void, string>)",
	"mkdir":      "mkdir(path: string) -> (Result<void, string>)",
	"chmod":      "chmod(path: string, mode: int) -> (Result<void, string>)",

	// Command execution
	"exec": "exec(command: string, args: Vec<string, _>) -> (Result<string, string>)",

	// Network functions
	"socketConnect":    "socketConnect(host: string, port: int) -> (Result<int, string>)",
	"socketRead":       "socketRead(fd: int, size: int) -> (Result<Vec<int, _>, string>)",
	"socketWrite":      "socketWrite(fd: int, data: Vec<int, _>) -> (Result<int, string>)",
	"socketClose":      "socketClose(fd: int) -> (void)",
	"socketBind":       "socketBind(port: int) -> (Result<int, string>)",
	"socketAccept":     "socketAccept(fd: int) -> (Result<int, string>)",
	"socketSetTimeout": "socketSetTimeout(fd: int, ms: int) -> (Result<void, string>)",

	// Mutex/synchronization
	"mutexNew":    "mutexNew() -> (int)",
	"mutexLock":   "mutexLock(id: int) -> (void)",
	"mutexUnlock": "mutexUnlock(id: int) -> (void)",
}

func completionKind(kind string) int {
	switch kind {
	case "func":
		return 3
	case "method":
		return 2
	case "struct":
		return 22
	case "enum":
		return 13
	case "enumMember":
		return 20
	case "type", "alias":
		return 25
	case "var":
		return 6
	case "const":
		return 21
	default:
		return 1
	}
}

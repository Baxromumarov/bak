package main

import (
	"encoding/json"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/builtins"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func (s *Server) handleCompletion(req Request) CompletionList {
	var params CompletionParams
	out := CompletionList{Items: []CompletionItem{}}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return out
	}

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

		out.Items = items
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
		result, _ := s.analysisResult(params.TextDocument.URI)
		if structItems := s.completeStructLiteralFields(result, text, params.TextDocument.URI, params.Position); len(structItems) > 0 {
			out.Items = structItems
			return out
		}
	}

	if qualifier != "" || isDotCompletion {
		result, _ := s.analysisResult(params.TextDocument.URI)
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
				tc, astRoot = s.typecheckForCompletion(text, params.TextDocument.URI, params.Position)
			}

			if tc != nil && astRoot != nil {
				currentPkg := currentPackageName(astRoot)

				addMembers := func(typeStr string, isStatic bool) {
					if typeStr == "" {
						return
					}

					typeStr = resolveAliasTypeString(result, typeStr)
					baseType := typeStr

					if before, _, ok0 := strings.Cut(typeStr, "<"); ok0 {
						baseType = before
					}

					if structDef, ok := tc.GetStruct(baseType); ok {
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
							}

							insertText := methodName
							insertFormat := 1
							insertFormat = 2
							if len(methodSig.Parameters) == 0 {
								insertText = methodName + "()"
							} else {
								insertText = methodName + "($0)"
							}

							items = append(items, CompletionItem{
								Label:            methodName,
								Kind:             2,
								Detail:           methodCompletionDetail(baseType, methodName, methodSig, isStatic),
								InsertText:       insertText,
								InsertTextFormat: insertFormat,
							})
						}

						if !isStatic {
							for fieldName, fieldDef := range structDef.Fields {
								if fieldDef.Visibility == ast.Private && !isSamePkg {
									continue
								}
								items = append(items, CompletionItem{
									Label:  fieldName,
									Kind:   5,
									Detail: fieldDef.Type.String(),
								})
							}
						}
					} else {
						if appendBuiltinTypeMethodCompletions(&items, baseType, isStatic) {
							return
						}
						if result.Index != nil && result.Index.Structs != nil {
							if st, ok := result.Index.Structs[baseType]; ok && !isStatic {
								for _, f := range st.Fields {
									items = append(items, CompletionItem{
										Label:  f,
										Kind:   5,
										Detail: "field",
									})
								}
							}
						}
					}
				}

				typeStr := ""
				isStatic := false

				if isDotCompletion && (qualifier == "" || result.Imports[qualifier] == "") {
					node := findNode(astRoot, params.Position.Line+1, params.Position.Character+1)

					switch n := node.(type) {
					case *ast.FieldAccessExpression:
						typeStr = tc.GetNodeType(n.Object)
						if ident, ok := n.Object.(*ast.Identifier); ok {
							locals := collectLocalSymbols(astRoot, params.Position.Line+1)
							_, isLocal := locals[ident.Value]

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

				if typeStr == "" && qualifier != "" {
					locals := collectLocalSymbols(astRoot, params.Position.Line+1)
					if local, ok := locals[qualifier]; ok && local.Node != nil {
						typeStr = tc.GetNodeType(local.Node)
						isStatic = false
					}
				}
				if typeStr == "" && qualifier != "" && hasBuiltinStaticCompletionType(qualifier) {
					typeStr = qualifier
					isStatic = true
				}

				addMembers(typeStr, isStatic)

				if len(items) == 0 && qualifier != "" && hasBuiltinStaticCompletionType(qualifier) {
					appendBuiltinTypeMethodCompletions(&items, qualifier, true)
				}

				if isDotCompletion && memberPrefix != "" {
					items = filterCompletionItemsByPrefix(items, memberPrefix)
				}

				out.Items = items
				return out
			}
		}
	}

	if result, _ := s.analysisResult(params.TextDocument.URI); result != nil && result.Index != nil {
		seen := make(map[string]bool)
		for _, sym := range sortedSymbols(result.Index) {
			insertText := sym.Name
			insertFormat := 1
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
			if sig, ok := builtinSignatures[name]; ok {
				detail = sig
			}
			items = append(items, CompletionItem{
				Label:  name,
				Kind:   3,
				Detail: detail,
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

	typeKeywords := []string{
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
		"Vec",
		"HashMap",
		"Result",
		"Range",
	}

	for _, typ := range typeKeywords {
		items = append(items, CompletionItem{
			Label:  typ,
			Kind:   25, // Type
			Detail: "type",
		})
	}

	snippets := []struct {
		label  string
		text   string
		detail string
	}{
		{"func", "func ${1:name}(${2:params}) -> ${3:void} {\n\t$0\n}", "Function definition"},
		{"if", "if ${1:condition} {\n\t$0\n}", "If statement"},
		{"else", "else {\n\t$0\n}", "Else block"},
		{"for", "for ${1:item} in ${2:iter} {\n\t$0\n}", "For loop"},
		{"while", "while ${1:condition} {\n\t$0\n}", "While loop"},
		{"match", "switch ${1:expr} {\ncase ${2:pattern}:\n\t$0\n}", "Switch statement"},
		{"struct", "struct ${1:Name} {\n\t${2:field}: ${3:Type},\n}", "Struct definition"},
		{"impl", "impl ${1:Name} {\n\tpub func ${2:name}(mut self) {\n\t\t$0\n\t}\n}", "Implementation block"},
		{"pub", "pub ", "Public visibility"},
		{"mut", "mut ", "Mutable modifier"},
		{"var", "var ${1:name} = ${2:value}", "Variable declaration"},
		{"const", "const ${1:NAME} = ${2:value}", "Constant declaration"},
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

	out.Items = items
	return out
}

func (s *Server) completeStructLiteralFields(
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
		tc, astRoot = s.typecheckForCompletion(text, uri, pos)
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
			items = append(items, CompletionItem{
				Label:  fieldName,
				Kind:   5, // Field
				Detail: fieldDef.Type.String(),
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
		if name == "" || existing[name] {
			continue
		}
		items = append(items, CompletionItem{
			Label:  name,
			Kind:   5, // Field
			Detail: detail,
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

	modIndex := s.getOrIndexFile(path)
	items := make([]CompletionItem, 0)
	for _, sym := range sortedSymbols(modIndex) {
		if !sym.Exported || strings.Contains(sym.Name, ".") {
			continue
		}

		insertText := sym.Name
		insertFormat := 1
		detail := sym.Kind
		if sym.Kind == "func" {
			insertFormat = 2
			insertText = sym.Name + "($0)"
			if modIndex.Sigs != nil {
				if sig, ok := modIndex.Sigs[sym.Name]; ok && sig.Label != "" {
					detail = sig.Label
				}
			}
		}

		items = append(items, CompletionItem{
			Label:            sym.Name,
			Detail:           detail,
			Kind:             completionKind(sym.Kind),
			InsertText:       insertText,
			InsertTextFormat: insertFormat,
		})
	}

	if memberPrefix != "" {
		items = filterCompletionItemsByPrefix(items, memberPrefix)
	}

	return items, true
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

		insertText, insertFormat := completionInsertTextFromSignature(methodName, info.Signature)
		*items = append(*items, CompletionItem{
			Label:            methodName,
			Kind:             2, // Method
			Detail:           info.Signature,
			InsertText:       insertText,
			InsertTextFormat: insertFormat,
		})
	}

	return true
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

var builtinSignatures = map[string]string{
	"fromChars": "fromChars(chars: Vec<char, _>) -> (string)",
	"print":     "print(values: any...) -> (void)",
	"println":   "println(values: any...) -> (void)",
	"type":      "type(value: any) -> (string)",
	"typeof":    "typeof(value: any) -> (string)",
	"int":       "int(value: any) -> (int | Result<int,string>)",
	"float":     "float(value: any) -> (float64 | Result<float64,string>)",
	"string":    "string(value: any) -> (string)",
	"char":      "char(value: int|char) -> (char)",
	"concat":    "concat(values: string...) -> (string)",
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

package main

import (
	"maps"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/prelude"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (s *Server) handleHover(req Request) *Hover {
	params, ok := requestParams[HoverParams](req)
	if !ok {
		return nil
	}
	text, _ := s.document(params.TextDocument.URI)

	result, ok := s.analysisResult(params.TextDocument.URI)
	if !ok || result.AST == nil {
		return nil
	}

	// Position is 0-indexed. Token lines are 1-based (usually).
	line := params.Position.Line + 1
	char := params.Position.Character + 1

	node := findNode(result.AST, line, char)
	if node == nil || isNil(node) {
		return nil
	}

	if hover := hoverForBuiltinMethod(node, result); hover != nil {
		return hover
	}
	if ident, ok := node.(*ast.Identifier); ok {
		if hover := hoverForQualifiedBuiltinIdentifier(ident, result, text, params.Position); hover != nil {
			return hover
		}
	}
	if hover := hoverForBuiltinIdentifier(node, result); hover != nil {
		return hover
	}

	typeStr := ""
	if result.TC != nil {
		typeStr = result.TC.GetNodeType(node)
	}

	switch n := node.(type) {
	case *ast.Identifier:
		return buildHover(
			s.hoverInfoForIdentifier(
				n,
				result,
				params.TextDocument.URI,
				text,
				params.Position,
			),
			typeStr,
			node,
			result,
			params.Position,
		)
	case *ast.FieldAccessExpression:
		return buildHover(
			s.hoverInfoForFieldAccess(
				n,
				result,
				params.TextDocument.URI,
			),
			typeStr,
			node,
			result,
			params.Position,
		)
	case *ast.MethodCallExpression:
		return buildHover(
			s.hoverInfoForMethodCall(
				n,
				result,
				params.TextDocument.URI,
			),
			typeStr,
			node,
			result,
			params.Position,
		)
	case *ast.CallExpression:
		return buildHover(
			s.hoverInfoForCallExpression(
				n,
				result,
				params.TextDocument.URI,
			),
			typeStr,
			node,
			result,
			params.Position,
		)
	case *ast.SimpleType:
		return buildHover(
			s.hoverInfoForSimpleType(
				n,
				result,
				params.TextDocument.URI,
			),
			typeStr,
			node,
			result,
			params.Position,
		)
	default:
		return buildHover(
			hoverInfo{},
			typeStr,
			node,
			result,
			params.Position,
		)
	}
}

type hoverInfo struct {
	doc           string
	sig           string
	structInfo    StructInfo
	hasStructInfo bool
}

func (h hoverInfo) empty() bool {
	return h.doc == "" && h.sig == "" && !h.hasStructInfo
}

func buildHover(h hoverInfo, typeStr string, node ast.Node, result *AnalysisResult, pos Position) *Hover {
	hoverType := formatHoverType(typeStr)
	if ident, ok := node.(*ast.Identifier); ok && isDynamicVecType(typeStr) && result != nil {
		line := pos.Line + 1
		char := pos.Character + 1
		vecLen, hasLen := inferDynamicVecLengthAtPosition(result.AST, ident.Value, line, char)
		hoverType = formatDynamicVecTypeWithLength(hoverType, vecLen, hasLen)
	}
	if h.empty() && typeStr == "" {
		return nil
	}

	var body string
	switch {
	case h.sig != "":
		body = strfmt.Named("```bak\n{sig}\n```", "Sig", h.sig)
	case h.hasStructInfo:
		lines := []string{"struct " + h.structInfo.Name}
		for _, f := range h.structInfo.Fields {
			lines = append(lines, "  "+f)
		}
		body = "```bak\n" + strings.Join(lines, "\n") + "\n```"
	case typeStr != "":
		body = strfmt.Named(
			"```bak\n{tokenLiteral}: {hoverType}\n```",
			"TokenLiteral", node.TokenLiteral(),
			"HoverType", hoverType,
		)
	}
	if h.doc != "" {
		if body == "" {
			body = h.doc
		} else {
			body += "\n\n" + h.doc
		}
	}

	return &Hover{
		Contents: MarkupContent{
			Kind:  "markdown",
			Value: body,
		},
		Range: hoverRange(ast.SpanOf(node)),
	}
}

func (s *Server) hoverInfoForIdentifier(
	ident *ast.Identifier,
	result *AnalysisResult,
	uri string,
	text string,
	pos Position,
) hoverInfo {
	h := lookupSymbol(result.Index, ident.Value)
	if result != nil && result.Index != nil && result.Index.Aliases != nil {
		if alias, ok := result.Index.Aliases[ident.Value]; ok && alias.Underlying != "" && !h.hasStructInfo {
			if st, ok := s.findStructInfoByType(result, uri, alias.Underlying); ok {
				h.structInfo = st
				h.hasStructInfo = true
			}
		}
	}

	if pkgInfo := s.packageHoverInfo(result, uri, ident.Value); !pkgInfo.empty() {
		if h.sig == "" {
			h.sig = pkgInfo.sig
		}
		if h.doc == "" {
			h.doc = pkgInfo.doc
		}
	}
	if !h.empty() {
		return h
	}
	return s.qualifiedIdentifierHoverInfo(
		result,
		uri,
		text,
		pos,
		ident.Value,
	)
}

func (s *Server) hoverInfoForFieldAccess(n *ast.FieldAccessExpression, result *AnalysisResult, uri string) hoverInfo {
	if ident, ok := n.Object.(*ast.Identifier); ok {
		if modIndex := s.resolveModule(result, uri, ident.Value); modIndex != nil {
			if h := lookupSymbol(modIndex, n.Field.Value); !h.empty() {
				return h
			}
		}
		key := ident.Value + "." + n.Field.Value
		if h := lookupSymbol(result.Index, key); !h.empty() {
			return h
		}
	}
	return hoverInfo{}
}

func (s *Server) hoverInfoForMethodCall(n *ast.MethodCallExpression, result *AnalysisResult, uri string) hoverInfo {
	if ident, ok := n.Object.(*ast.Identifier); ok {
		if modIndex := s.resolveModule(result, uri, ident.Value); modIndex != nil {
			if h := lookupSymbol(modIndex, n.Method.Value); !h.empty() {
				return h
			}
		}
	}
	if result == nil || result.TC == nil {
		return hoverInfo{}
	}

	t := result.TC.GetNodeType(n.Object)
	if t == "" {
		return hoverInfo{}
	}
	key := baseTypeName(t) + "." + n.Method.Value
	mutatesReceiver := methodDeclIsMutable(result, baseTypeName(t), n.Method.Value)

	if h := lookupSymbol(result.Index, key); !h.empty() {
		if h.sig != "" {
			if structDef, ok := result.TC.GetStruct(baseTypeName(t)); ok {
				h.sig = specializeGenericSignatureWithParams(h.sig, t, structDef.TypeParams)
			}
		}
		return enrichMethodHover(h, t, mutatesReceiver)
	}
	if h := s.lookupInImportedModules(result, uri, key, n.Method.Value); !h.empty() {
		return enrichMethodHover(h, t, mutatesReceiver)
	}
	return enrichMethodHover(s.lookupInPrelude(key, n.Method.Value), t, mutatesReceiver)
}

func methodDeclIsMutable(result *AnalysisResult, typeName, methodName string) bool {
	if result == nil || result.AST == nil || typeName == "" || methodName == "" {
		return false
	}
	for _, stmt := range result.AST.Statements {
		impl, ok := stmt.(*ast.ImplDecl)
		if !ok || impl == nil || impl.TypeName == nil || impl.TypeName.Value != typeName {
			continue
		}
		for _, method := range impl.Methods {
			if method != nil && method.Name != nil && method.Name.Value == methodName {
				return method.Mutable
			}
		}
	}
	return false
}

func enrichMethodHover(h hoverInfo, receiverType string, mutatesReceiver bool) hoverInfo {
	if h.empty() {
		return h
	}
	lines := []string{}
	if receiverType != "" {
		lines = append(lines, strfmt.Named("Receiver: `{Receiver}`.", "Receiver", receiverType))
	}
	if mutatesReceiver || strings.HasPrefix(strings.TrimSpace(h.sig), "mut func") {
		lines = append(lines, "Mutates the receiver.")
	}
	if len(lines) == 0 {
		return h
	}
	extra := strings.Join(lines, "\n")
	if h.doc == "" {
		h.doc = extra
	} else if !strings.Contains(h.doc, extra) {
		h.doc += "\n\n" + extra
	}
	return h
}

func (s *Server) hoverInfoForCallExpression(n *ast.CallExpression, result *AnalysisResult, uri string) hoverInfo {
	fa, ok := n.Function.(*ast.FieldAccessExpression)
	if !ok {
		return hoverInfo{}
	}
	ident, ok := fa.Object.(*ast.Identifier)
	if !ok {
		return hoverInfo{}
	}
	modIndex := s.resolveModule(result, uri, ident.Value)
	if modIndex == nil || modIndex.Sigs == nil {
		return hoverInfo{}
	}
	sig, ok := modIndex.Sigs[fa.Field.Value]
	if !ok {
		return hoverInfo{}
	}
	doc := sig.Doc
	if doc == "" && modIndex.Docs != nil {
		doc = modIndex.Docs[fa.Field.Value]
	}
	return hoverInfo{sig: sig.Label, doc: doc}
}

func (s *Server) hoverInfoForSimpleType(n *ast.SimpleType, result *AnalysisResult, uri string) hoverInfo {
	st, ok := s.findStructInfoByType(result, uri, n.Name)
	if !ok {
		return hoverInfo{}
	}
	return hoverInfo{structInfo: st, hasStructInfo: true}
}

func (s *Server) packageHoverInfo(result *AnalysisResult, uri, alias string) hoverInfo {
	if result == nil || result.Imports == nil {
		return hoverInfo{}
	}
	importPath, ok := result.Imports[alias]
	if !ok {
		return hoverInfo{}
	}
	path := s.resolveImportPath(uriToPath(uri), importPath)
	if path == "" {
		return hoverInfo{}
	}
	pkgName, pkgDoc := s.packageDoc(path)
	if pkgName == "" && pkgDoc == "" {
		return hoverInfo{}
	}
	info := hoverInfo{doc: pkgDoc}
	if pkgName != "" {
		info.sig = strfmt.Named("package {pkgName}", "PkgName", pkgName)
	}
	return info
}

func (s *Server) qualifiedIdentifierHoverInfo(
	result *AnalysisResult,
	uri string,
	text string,
	pos Position,
	expectedWord string,
) hoverInfo {
	if text == "" {
		return hoverInfo{}
	}
	lineText := lineAt(text, pos.Line)
	word, start := wordAt(lineText, pos.Character)
	if word != expectedWord || start <= 0 || lineText[start-1] != '.' {
		return hoverInfo{}
	}
	qualifier := qualifierBefore(lineText, start-1)
	if qualifier == "" {
		return hoverInfo{}
	}
	if receiverType := receiverTypeForQualifier(result, text, pos, qualifier); receiverType != "" {
		baseType := baseTypeName(receiverType)
		if result != nil && result.Index != nil && result.Index.Sigs != nil {
			if sig, ok := result.Index.Sigs[baseType+"."+word]; ok {
				info := hoverInfo{sig: sig.Label, doc: sig.Doc}
				if result.TC != nil {
					if structDef, ok := result.TC.GetStruct(baseType); ok {
						info.sig = specializeGenericSignatureWithParams(info.sig, receiverType, structDef.TypeParams)
					}
				}
				return info
			}
		}
	}
	modIndex := s.resolveModule(result, uri, qualifier)
	if modIndex == nil {
		return hoverInfo{}
	}
	return lookupSymbol(modIndex, word)
}

func (s *Server) resolveModule(result *AnalysisResult, uri, alias string) *FileIndex {
	if result == nil || result.Imports == nil {
		return nil
	}
	importPath, ok := result.Imports[alias]
	if !ok {
		return nil
	}
	path := s.resolveImportPath(uriToPath(uri), importPath)
	if path == "" {
		return nil
	}
	return s.getOrIndexFile(path)
}

func (s *Server) lookupInImportedModules(result *AnalysisResult, uri, key, method string) hoverInfo {
	if result == nil || result.Imports == nil {
		return hoverInfo{}
	}
	for _, importPath := range result.Imports {
		path := s.resolveImportPath(uriToPath(uri), importPath)
		if path == "" {
			continue
		}
		modIndex := s.getOrIndexFile(path)
		if modIndex == nil {
			continue
		}
		if h := lookupSymbol(modIndex, key); !h.empty() {
			return h
		}
		if h := lookupSymbol(modIndex, method); !h.empty() {
			return h
		}
	}
	return hoverInfo{}
}

func (s *Server) lookupInPrelude(key, method string) hoverInfo {
	stdLibPath := prelude.GetStdLibPath()
	preludeFiles := []string{
		filepath.Join(stdLibPath, "collections", "vec.bak"),
		filepath.Join(stdLibPath, "collections", "hashmap.bak"),
		filepath.Join(stdLibPath, "result.bak"),
	}
	for _, path := range preludeFiles {
		modIndex := s.getOrIndexFile(path)
		if modIndex == nil {
			continue
		}
		if key != "" {
			if h := lookupSymbol(modIndex, key); !h.empty() {
				return h
			}
		}
		if h := lookupSymbol(modIndex, method); !h.empty() {
			return h
		}
	}
	return hoverInfo{}
}

func lookupSymbol(idx *FileIndex, name string) hoverInfo {
	if idx == nil || name == "" {
		return hoverInfo{}
	}

	h := hoverInfo{}
	if idx.Docs != nil {
		h.doc = idx.Docs[name]
	}
	if idx.Sigs != nil {
		if sig, ok := idx.Sigs[name]; ok {
			h.sig = sig.Label
			if h.doc == "" {
				h.doc = sig.Doc
			}
		}
	}
	if idx.Structs != nil {
		if st, ok := idx.Structs[name]; ok {
			h.structInfo = st
			h.hasStructInfo = true
			if h.doc == "" {
				h.doc = st.Doc
			}
		}
	}
	if h.sig == "" && idx.Consts != nil {
		if c, ok := idx.Consts[name]; ok {
			h.sig = strfmt.Named(
				"const {Name}: {Type} = {Value}",
				"Name", c.Name,
				"Type", c.Type,
				"Value", c.Value,
			)
			if h.doc == "" {
				h.doc = c.Doc
			}
		}
	}
	if h.sig == "" && idx.Types != nil {
		if t, ok := idx.Types[name]; ok {
			h.sig = strfmt.Named(
				"type {Name} = {Underlying}",
				"Name", t.Name,
				"Underlying", t.Underlying,
			)
			if h.doc == "" {
				h.doc = t.Doc
			}
		}
	}
	if h.sig == "" && idx.Aliases != nil {
		if a, ok := idx.Aliases[name]; ok {
			h.sig = strfmt.Named(
				"alias {Name} = {Underlying}",
				"Name", a.Name,
				"Underlying", a.Underlying,
			)
			if h.doc == "" {
				h.doc = a.Doc
			}
		}
	}
	if h.sig == "" && !h.hasStructInfo && idx.Enums != nil {
		if e, ok := idx.Enums[name]; ok {
			variants := strings.Join(e.Variants, ", ")
			h.sig = strfmt.Named(
				"enum {Name} {{ {variants} }}",
				"Name", e.Name,
				"Variants", variants,
			)
			if h.doc == "" {
				h.doc = e.Doc
			}
		}
	}
	if h.sig == "" && idx.Vars != nil {
		if v, ok := idx.Vars[name]; ok {
			mutPrefix := ""
			if v.Mutable {
				mutPrefix = "mut "
			}
			h.sig = strfmt.Named(
				"{mutPrefix}var {Name}: {Type}",
				"MutPrefix", mutPrefix,
				"Name", v.Name,
				"Type", v.Type,
			)
			if h.doc == "" {
				h.doc = v.Doc
			}
		}
	}
	return h
}

func hoverForBuiltinMethod(node ast.Node, result *AnalysisResult) *Hover {
	mce, ok := node.(*ast.MethodCallExpression)
	if !ok {
		return nil
	}

	if result != nil && result.TC != nil {
		typeStr := result.TC.GetNodeType(mce.Object)
		baseType := baseTypeName(typeStr)
		if methods, ok := builtinMethods[baseType]; ok {
			if info, ok := methods[mce.Method.Value]; ok {
				return builtinMethodHoverForType(baseType, typeStr, mce.Method.Value, info)
			}
		}
	}

	if info, ok := findBuiltinMethodByName(mce.Method.Value); ok {
		return builtinMethodHover(info)
	}
	return nil
}

func hoverForBuiltinIdentifier(node ast.Node, result *AnalysisResult) *Hover {
	ident, ok := node.(*ast.Identifier)
	if !ok {
		return nil
	}
	if result != nil && result.Index != nil && result.Index.Symbols[ident.Value].Location.Range.Start.Line != 0 {
		return nil
	}
	info, ok := findBuiltinMethodByName(ident.Value)
	if !ok {
		return nil
	}
	return builtinMethodHover(info)
}

func hoverForQualifiedBuiltinIdentifier(
	ident *ast.Identifier,
	result *AnalysisResult,
	text string,
	pos Position,
) *Hover {
	if ident == nil || result == nil || text == "" {
		return nil
	}
	lineText := lineAt(text, pos.Line)
	word, start := wordAt(lineText, pos.Character)
	if word != ident.Value || start <= 0 || lineText[start-1] != '.' {
		return nil
	}
	qualifier := qualifierBefore(lineText, start-1)
	receiverType := receiverTypeForQualifier(result, text, pos, qualifier)
	if receiverType == "" {
		return nil
	}
	baseType := baseTypeName(receiverType)
	methods, ok := builtinMethods[baseType]
	if !ok {
		return nil
	}
	info, ok := methods[ident.Value]
	if !ok {
		return nil
	}
	return builtinMethodHoverForType(baseType, receiverType, ident.Value, info)
}

func builtinMethodHover(info builtinMethodInfo) *Hover {
	return builtinMethodHoverForType("", "", "", info)
}

func builtinMethodHoverForType(typeName, fullType, methodName string, info builtinMethodInfo) *Hover {
	signature := info.Signature
	if fullType != "" {
		signature = specializeGenericSignature(signature, fullType)
	}
	if typeName != "" && methodName != "" {
		label := signature
		if after, ok := strings.CutPrefix(label, "func "); ok {
			label = after
		}
		if open := strings.Index(label, "("); open >= 0 {
			signature = "func " + typeName + "." + methodName + label[open:]
		}
	}
	return &Hover{
		Contents: MarkupContent{
			Kind:  "markdown",
			Value: strfmt.Named("```bak\n{Signature}\n```\n{Doc}", "Signature", signature, "Doc", info.Doc),
		},
	}
}

func findBuiltinMethodByName(name string) (builtinMethodInfo, bool) {
	for _, methods := range builtinMethods {
		if info, ok := methods[name]; ok {
			return info, true
		}
	}
	return builtinMethodInfo{}, false
}

func mapBuiltinType(t string) string {
	switch t {
	case "string", "String":
		return "StringBuiltins"
	case "Vec", "vec":
		return "VecBuiltins"
	case "Map", "map":
		return "MapBuiltins"
	}
	return t
}

func baseTypeName(typeStr string) string {
	t := strings.TrimSpace(typeStr)
	if t == "" {
		return ""
	}
	t = strings.TrimPrefix(t, "&")
	t = strings.TrimSpace(strings.TrimPrefix(t, "mut"))
	if idx := strings.Index(t, "<"); idx != -1 {
		t = t[:idx]
	}
	// Strip package prefix (e.g., "time.Time" -> "Time")
	if idx := strings.LastIndex(t, "."); idx != -1 {
		t = t[idx+1:]
	}
	return strings.TrimSpace(t)
}

func resolveAliasTypeString(result *AnalysisResult, typeStr string) string {
	if result == nil || result.Index == nil || result.Index.Aliases == nil {
		return typeStr
	}
	base := baseTypeName(typeStr)
	if base == "" {
		return typeStr
	}
	if aliasInfo, ok := result.Index.Aliases[base]; ok && aliasInfo.Underlying != "" {
		return aliasInfo.Underlying
	}
	return typeStr
}

func formatHoverType(typeStr string) string {
	t := strings.TrimSpace(typeStr)
	if t == "" {
		return ""
	}

	return t
}

func isDynamicVecType(typeStr string) bool {
	t := strings.TrimSpace(typeStr)
	if t == "" || baseTypeName(t) != "Vec" {
		return false
	}
	compact := strings.ReplaceAll(t, " ", "")
	if strings.Contains(compact, ",_>") {
		return true
	}
	// Some paths represent dynamic vectors as Vec<T, _>.
	return strings.HasPrefix(compact, "Vec<") && !strings.Contains(compact, ",")
}

func formatDynamicVecTypeWithLength(typeStr string, vecLen int, hasLen bool) string {
	t := strings.TrimSpace(typeStr)
	if !strings.HasPrefix(t, "Vec<") || !strings.HasSuffix(t, ">") {
		return typeStr
	}

	inner := strings.TrimSpace(t[len("Vec<") : len(t)-1])
	if inner == "" {
		return typeStr
	}

	parts := splitTopLevelTypeArgs(inner)
	if len(parts) == 0 {
		return typeStr
	}

	if hasLen {
		sizeToken := strconv.Itoa(vecLen)
		if len(parts) >= 2 {
			parts[len(parts)-1] = sizeToken
		} else {
			parts = append(parts, sizeToken)
		}
	} else {
		if len(parts) >= 2 {
			parts[len(parts)-1] = "_"
		} else {
			parts = append(parts, "_")
		}
	}

	return "Vec<" + strings.Join(parts, ", ") + ">"
}

func splitTopLevelTypeArgs(inner string) []string {
	args := []string{}
	depth := 0
	start := 0

	for i, ch := range inner {
		switch ch {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(inner[start:i]))
				start = i + 1
			}
		}
	}

	args = append(args, strings.TrimSpace(inner[start:]))
	return args
}

type vecLenState struct {
	known  bool
	length int
}

func inferDynamicVecLengthAtPosition(prog *ast.Program, varName string, line, col int) (int, bool) {
	if prog == nil || varName == "" || line <= 0 || col <= 0 {
		return 0, false
	}
	states := make(map[string]vecLenState)
	funcs := vecLengthFunctionIndex(prog)
	if !walkProgramVecLengthPath(prog, line, col, states, funcs) {
		return 0, false
	}

	st, ok := states[varName]
	if !ok || !st.known {
		return 0, false
	}
	return st.length, true
}

func vecLengthFunctionIndex(prog *ast.Program) map[string]*ast.FunctionDecl {
	funcs := make(map[string]*ast.FunctionDecl)
	if prog == nil {
		return funcs
	}
	for _, stmt := range prog.Statements {
		if fn, ok := stmt.(*ast.FunctionDecl); ok && fn != nil && fn.Name != nil {
			funcs[fn.Name.Value] = fn
		}
	}
	return funcs
}

func walkProgramVecLengthPath(
	prog *ast.Program,
	line, col int,
	states map[string]vecLenState,
	funcs map[string]*ast.FunctionDecl,
) bool {
	if prog == nil {
		return false
	}
	return walkStatementsVecLengthPath(prog.Statements, line, col, states, funcs)
}

func walkStatementsVecLengthPath(
	stmts []ast.Statement,
	line, col int,
	states map[string]vecLenState,
	funcs map[string]*ast.FunctionDecl,
) bool {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}

		span := ast.SpanOf(stmt)
		if positionBefore(line, col, span.Start.Line, span.Start.Column) {
			return true
		}

		if spanContainsPosition(span, line, col) {
			switch s := stmt.(type) {
			case *ast.FunctionDecl:
				if s != nil && s.Body != nil {
					return walkStatementsVecLengthPath(s.Body.Statements, line, col, states, funcs)
				}
				return true
			case *ast.ImplDecl:
				if s == nil {
					return true
				}
				for _, m := range s.Methods {
					if m != nil && m.Body != nil && spanContainsPosition(m.Span, line, col) {
						return walkStatementsVecLengthPath(m.Body.Statements, line, col, states, funcs)
					}
				}
				return true
			case *ast.IfStatement:
				if s != nil {
					if s.Consequence != nil && spanContainsPosition(ast.SpanOf(s.Consequence), line, col) {
						return walkStatementsVecLengthPath(s.Consequence.Statements, line, col, states, funcs)
					}
					if s.Alternative != nil && spanContainsPosition(ast.SpanOf(s.Alternative), line, col) {
						return walkStatementsVecLengthPath(s.Alternative.Statements, line, col, states, funcs)
					}
				}
				return true
			case *ast.WhileStatement:
				if s != nil && s.Body != nil {
					return walkStatementsVecLengthPath(s.Body.Statements, line, col, states, funcs)
				}
				return true
			case *ast.ForStatement:
				if s != nil && s.Body != nil {
					return walkStatementsVecLengthPath(s.Body.Statements, line, col, states, funcs)
				}
				return true
			case *ast.SwitchStatement:
				if s != nil {
					for _, c := range s.Cases {
						if c != nil && c.Body != nil && spanContainsPosition(ast.SpanOf(c.Body), line, col) {
							return walkStatementsVecLengthPath(c.Body.Statements, line, col, states, funcs)
						}
					}
				}
				return true
			case *ast.UnsafeBlock:
				if s != nil && s.Body != nil {
					return walkStatementsVecLengthPath(s.Body.Statements, line, col, states, funcs)
				}
				return true
			case *ast.BlockStatement:
				if s != nil {
					return walkStatementsVecLengthPath(s.Statements, line, col, states, funcs)
				}
				return true
			default:
				applyVecLengthStatement(stmt, states, funcs)
				return true
			}
		}

		applyVecLengthStatement(stmt, states, funcs)
		if isLoopBoundary(stmt) {
			invalidateAllVecStates(states)
		}
	}

	return true
}

func applyVecLengthStatement(
	stmt ast.Statement,
	states map[string]vecLenState,
	funcs map[string]*ast.FunctionDecl,
) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ast.VarStatement:
		if s == nil || s.Name == nil {
			return
		}
		name := s.Name.Value
		if !isVecVariableDeclaration(s, states) {
			delete(states, name)
			return
		}
		if l, ok := inferVecLengthFromExpr(s.Value, states); ok {
			states[name] = vecLenState{known: true, length: l}
			return
		}
		states[name] = vecLenState{known: false, length: 0}
	case *ast.AssignmentStatement:
		if s == nil {
			return
		}
		ident, ok := s.Left.(*ast.Identifier)
		if !ok || ident == nil {
			return
		}
		name := ident.Value
		if _, tracked := states[name]; !tracked {
			return
		}
		if l, ok := inferVecLengthFromExpr(s.Value, states); ok {
			states[name] = vecLenState{known: true, length: l}
			return
		}
		states[name] = vecLenState{known: false, length: 0}
	case *ast.ExpressionStatement:
		if s == nil {
			return
		}
		applyVecLengthExpr(s.Expression, states, funcs)
	case *ast.IfStatement:
		if s == nil {
			return
		}
		base := cloneVecLenStates(states)

		consequenceStates := cloneVecLenStates(base)
		if s.Consequence != nil {
			applyVecLengthStatements(s.Consequence.Statements, consequenceStates, funcs)
		}

		alternativeStates := cloneVecLenStates(base)
		if s.Alternative != nil {
			applyVecLengthStatements(s.Alternative.Statements, alternativeStates, funcs)
		}

		merged := mergeBranchStates(base, []map[string]vecLenState{consequenceStates, alternativeStates})
		maps.Copy(states, merged)
	case *ast.SwitchStatement:
		if s == nil {
			return
		}
		base := cloneVecLenStates(states)
		branchStates := make([]map[string]vecLenState, 0, len(s.Cases)+1)
		hasDefault := false
		for _, c := range s.Cases {
			if c == nil || c.Body == nil {
				continue
			}
			if c.Default {
				hasDefault = true
			}
			caseStates := cloneVecLenStates(base)
			applyVecLengthStatements(c.Body.Statements, caseStates, funcs)
			branchStates = append(branchStates, caseStates)
		}
		if len(branchStates) == 0 {
			return
		}
		if !hasDefault {
			// Without default, keep a conservative no-match path.
			branchStates = append(branchStates, cloneVecLenStates(base))
		}
		merged := mergeBranchStates(base, branchStates)
		maps.Copy(states, merged)
	case *ast.UnsafeBlock:
		if s != nil && s.Body != nil {
			applyVecLengthStatements(s.Body.Statements, states, funcs)
		}
	case *ast.BlockStatement:
		if s != nil {
			applyVecLengthStatements(s.Statements, states, funcs)
		}
	}
}

func applyVecLengthStatements(
	stmts []ast.Statement,
	states map[string]vecLenState,
	funcs map[string]*ast.FunctionDecl,
) {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		applyVecLengthStatement(stmt, states, funcs)
		if isLoopBoundary(stmt) {
			invalidateAllVecStates(states)
		}
	}
}

func cloneVecLenStates(states map[string]vecLenState) map[string]vecLenState {
	out := make(map[string]vecLenState, len(states))
	maps.Copy(out, states)
	return out
}

func mergeBranchStates(base map[string]vecLenState, branches []map[string]vecLenState) map[string]vecLenState {
	merged := cloneVecLenStates(base)
	if len(branches) == 0 {
		return merged
	}

	for name, baseState := range base {
		states := make([]vecLenState, 0, len(branches))
		for _, b := range branches {
			if st, ok := b[name]; ok {
				states = append(states, st)
			} else {
				// Missing means the branch did not semantically update this outer variable.
				states = append(states, baseState)
			}
		}

		first := states[0]
		allKnown := first.known
		sameLen := true
		for _, st := range states[1:] {
			if st.known != first.known {
				allKnown = false
				sameLen = false
				continue
			}
			if st.known && first.known && st.length != first.length {
				sameLen = false
			}
			if !st.known {
				allKnown = false
			}
		}

		if allKnown && sameLen {
			merged[name] = vecLenState{known: true, length: first.length}
		} else {
			merged[name] = vecLenState{known: false, length: 0}
		}
	}

	return merged
}

func isVecVariableDeclaration(stmt *ast.VarStatement, states map[string]vecLenState) bool {
	if stmt == nil {
		return false
	}
	if stmt.Type != nil {
		return isDynamicVecType(stmt.Type.String())
	}
	if _, ok := inferVecLengthFromExpr(stmt.Value, states); ok {
		return true
	}
	return isPotentialVecExpr(stmt.Value, states)
}

func isPotentialVecExpr(expr ast.Expression, states map[string]vecLenState) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.VecLiteral:
		return true
	case *ast.Identifier:
		_, ok := states[e.Value]
		return ok
	case *ast.MethodCallExpression:
		if e == nil || e.Method == nil {
			return false
		}
		if obj, ok := e.Object.(*ast.Identifier); ok {
			if obj.Value == "Vec" {
				switch e.Method.Value {
				case "new", "withCap", "from":
					return true
				}
			}
			if _, ok := states[obj.Value]; ok && e.Method.Value == "clone" {
				return true
			}
		}
	}
	return false
}

func applyVecLengthExpr(
	expr ast.Expression,
	states map[string]vecLenState,
	funcs map[string]*ast.FunctionDecl,
) {
	if ce, ok := expr.(*ast.CallExpression); ok {
		applyVecLengthCallExpr(ce, states, funcs)
		return
	}

	mc, ok := expr.(*ast.MethodCallExpression)
	if !ok || mc == nil || mc.Method == nil {
		return
	}
	ident, ok := mc.Object.(*ast.Identifier)
	if !ok || ident == nil {
		return
	}
	st, tracked := states[ident.Value]
	if !tracked {
		return
	}

	switch mc.Method.Value {
	case "push", "insert":
		if st.known {
			st.length = st.length + 1
		}
	case "pop", "remove":
		if st.known {
			if st.length > 0 {
				st.length = st.length - 1
			} else {
				st.length = 0
			}
		}
	case "clear":
		st.known = true
		st.length = 0
	case "append":
		if len(mc.Arguments) == 1 && st.known {
			if rhsLen, ok := inferVecLengthFromExpr(mc.Arguments[0], states); ok {
				st.length = st.length + rhsLen
			} else {
				st.known = false
			}
		} else {
			st.known = false
		}
	}

	states[ident.Value] = st
}

func applyVecLengthCallExpr(
	ce *ast.CallExpression,
	states map[string]vecLenState,
	funcs map[string]*ast.FunctionDecl,
) {
	if ce == nil || funcs == nil {
		return
	}
	callee, ok := ce.Function.(*ast.Identifier)
	if !ok || callee == nil {
		return
	}
	fn := funcs[callee.Value]
	if fn == nil || fn.Body == nil {
		return
	}

	localStates := cloneVecLenStates(states)
	copies := make(map[string]string)
	for i, param := range fn.Parameters {
		if param == nil || param.Name == nil || !param.Mutable || i >= len(ce.Arguments) {
			continue
		}
		argName := vecLengthArgName(ce.Arguments[i])
		if argName == "" {
			continue
		}
		if st, ok := states[argName]; ok {
			localStates[param.Name.Value] = st
			copies[param.Name.Value] = argName
		}
	}
	if len(copies) == 0 {
		return
	}

	applyVecLengthStatements(fn.Body.Statements, localStates, funcs)
	for paramName, argName := range copies {
		if st, ok := localStates[paramName]; ok {
			states[argName] = st
		} else {
			states[argName] = vecLenState{known: false}
		}
	}
}

func vecLengthArgName(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.Identifier:
		if e != nil {
			return e.Value
		}
	case *ast.MutableIdentifier:
		if e != nil {
			return e.Value
		}
	}
	return ""
}

func inferVecLengthFromExpr(expr ast.Expression, states map[string]vecLenState) (int, bool) {
	if expr == nil {
		return 0, false
	}

	switch e := expr.(type) {
	case *ast.VecLiteral:
		return len(e.Elements), true
	case *ast.Identifier:
		if st, ok := states[e.Value]; ok && st.known {
			return st.length, true
		}
		return 0, false
	case *ast.MethodCallExpression:
		if e == nil || e.Method == nil {
			return 0, false
		}
		obj, ok := e.Object.(*ast.Identifier)
		if !ok || obj == nil {
			return 0, false
		}

		if obj.Value == "Vec" {
			switch e.Method.Value {
			case "new", "withCap":
				return 0, true
			case "from":
				if len(e.Arguments) == 1 {
					return inferVecLengthFromExpr(e.Arguments[0], states)
				}
			}
			return 0, false
		}

		if st, ok := states[obj.Value]; ok && e.Method.Value == "clone" && st.known {
			return st.length, true
		}
	}

	return 0, false
}

func isLoopBoundary(stmt ast.Statement) bool {
	switch stmt.(type) {
	case *ast.WhileStatement, *ast.ForStatement:
		return true
	default:
		return false
	}
}

func invalidateAllVecStates(states map[string]vecLenState) {
	for name, st := range states {
		st.known = false
		states[name] = st
	}
}

func spanContainsPosition(span ast.Span, line, col int) bool {
	if span.Start.Line <= 0 || line <= 0 || col <= 0 {
		return false
	}
	if positionBefore(line, col, span.Start.Line, span.Start.Column) {
		return false
	}
	if span.End.Line <= 0 {
		return true
	}
	return positionBefore(line, col, span.End.Line, span.End.Column)
}

func positionBefore(lineA, colA, lineB, colB int) bool {
	if lineA < lineB {
		return true
	}
	if lineA > lineB {
		return false
	}
	return colA < colB
}

func (s *Server) findStructInfoByType(result *AnalysisResult, uri, typeStr string) (StructInfo, bool) {
	if result == nil || result.Index == nil {
		return StructInfo{}, false
	}
	resolved := resolveAliasTypeString(result, typeStr)
	if resolved == "" {
		return StructInfo{}, false
	}
	if idx := strings.LastIndex(resolved, "."); idx != -1 {
		pkgAlias := resolved[:idx]
		typeName := resolved[idx+1:]
		if importPath, ok := result.Imports[pkgAlias]; ok {
			path := s.resolveImportPath(uriToPath(uri), importPath)
			if path != "" {
				modIndex := s.getOrIndexFile(path)
				if modIndex != nil && modIndex.Structs != nil {
					if st, ok := modIndex.Structs[typeName]; ok {
						return st, true
					}
				}
			}
		}
		return StructInfo{}, false
	}
	base := baseTypeName(resolved)
	if result.Index.Structs != nil {
		if st, ok := result.Index.Structs[base]; ok {
			return st, true
		}
	}
	return StructInfo{}, false
}

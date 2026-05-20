package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func diagnosticFixesFromData(data any) []DiagnosticFix {
	payload, ok := diagnosticDataFromAny(data)
	if !ok {
		return nil
	}

	fixes := make([]DiagnosticFix, 0, len(payload.Fixes))
	for _, fix := range payload.Fixes {
		if fix.NewText == "" {
			continue
		}

		fixes = append(fixes, fix)
	}

	return fixes
}

func diagnosticDataFromAny(data any) (DiagnosticData, bool) {
	if data == nil {
		return DiagnosticData{}, false
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return DiagnosticData{}, false
	}

	var payload DiagnosticData

	if err := json.Unmarshal(raw, &payload); err != nil {
		return DiagnosticData{}, false
	}
	return payload, true
}

func extractUndefinedSymbol(msg string) string {
	if strings.HasPrefix(msg, "undefined: ") {
		return strings.TrimSpace(msg[len("undefined: "):])
	}

	if strings.HasPrefix(msg, "undefined type: ") {
		return strings.TrimSpace(msg[len("undefined type: "):])
	}

	return ""
}

func extractUndefinedMethod(msg string) (string, string, bool) {
	const prefix = "undefined method '"
	if !strings.HasPrefix(msg, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(msg, prefix)
	method, rest, ok := strings.Cut(rest, "' for ")
	if !ok || method == "" {
		return "", "", false
	}
	typeName := strings.TrimSpace(rest)
	if typeName == "" {
		return "", "", false
	}
	return method, typeName, true
}

func extractMissingField(msg string) (string, string, bool) {
	for _, prefix := range []string{"struct '", "type '"} {
		if !strings.HasPrefix(msg, prefix) {
			continue
		}
		rest := strings.TrimPrefix(msg, prefix)
		typeName, rest, ok := strings.Cut(rest, "' has no field '")
		if !ok {
			continue
		}
		field, _, ok := strings.Cut(rest, "'")
		if !ok || typeName == "" || field == "" {
			continue
		}
		return field, typeName, true
	}
	return "", "", false
}

func extractMutabilityVariable(msg string) string {
	const marker = " immutable variable '"
	idx := strings.Index(msg, marker)
	if idx == -1 {
		return ""
	}
	rest := msg[idx+len(marker):]
	name, _, ok := strings.Cut(rest, "'")
	if !ok {
		return ""
	}
	return strings.TrimSpace(name)
}

func findImportInsertPosition(result *AnalysisResult) Position {
	if result == nil || result.AST == nil {
		return Position{
			Line:      1,
			Character: 0,
		}
	}

	lastImportLine := 0
	for _, stmt := range result.AST.Statements {
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			if s == nil {
				continue
			}
			if s.Token.Line > lastImportLine {
				lastImportLine = s.Token.Line
			}
		case *ast.PackageStatement:
			if s == nil {
				continue
			}
			if s.Token.Line > lastImportLine {
				lastImportLine = s.Token.Line
			}
		}
	}

	return Position{
		Line:      lastImportLine + 1,
		Character: 0,
	}
}

func workspaceEditForTextEdit(uri string, edit TextEdit) *WorkspaceEdit {
	return &WorkspaceEdit{
		Changes: map[string][]TextEdit{
			uri: {edit},
		},
	}
}

func codeActionWithTextEdit(
	title,
	kind,
	uri string,
	diagnostics []Diagnostic,
	edit TextEdit,
) CodeAction {
	return CodeAction{
		Title:       title,
		Kind:        kind,
		Diagnostics: diagnostics,
		Edit:        workspaceEditForTextEdit(uri, edit),
	}
}

func codeActionWithInsertion(
	title,
	kind,
	uri string,
	diagnostics []Diagnostic,
	position Position,
	newText string,
) CodeAction {
	return codeActionWithTextEdit(
		title,
		kind,
		uri,
		diagnostics,
		TextEdit{
			Range: Range{
				Start: position,
				End:   position,
			},
			NewText: newText,
		})
}

func addQuickFixActions(
	actions []CodeAction,
	uri string,
	diag Diagnostic,
) []CodeAction {
	for _, fix := range diagnosticFixesFromData(diag.Data) {
		title := strings.TrimSpace(fix.Title)
		if title == "" {
			title = "Apply suggested fix"
		}
		targetURI := uri
		if fix.URI != "" {
			targetURI = fix.URI
		}
		actions = append(actions, codeActionWithTextEdit(title, "quickfix", targetURI, []Diagnostic{diag}, TextEdit{
			Range:   fix.Range,
			NewText: fix.NewText,
		}))
	}

	return actions
}

func addRemoveUnusedAction(
	actions []CodeAction,
	uri string,
	diag Diagnostic,
) []CodeAction {
	if !strings.Contains(diag.Message, "unused") {
		return actions
	}

	return append(actions, codeActionWithTextEdit(
		"Remove unused declaration",
		"quickfix",
		uri,
		[]Diagnostic{diag},
		TextEdit{
			Range:   diag.Range,
			NewText: "",
		},
	))
}

func addRemoveImportDiagnosticAction(
	actions []CodeAction,
	uri string,
	text string,
	diag Diagnostic,
) []CodeAction {
	title, ok := removeImportDiagnosticTitle(fmt.Sprint(diag.Code))
	if !ok {
		return actions
	}

	line := diag.Range.Start.Line
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[line]), "import ") {
		return actions
	}

	editRange, ok := fullLineDeletionRange(text, line)
	if !ok {
		return actions
	}

	return append(actions, codeActionWithTextEdit(
		title,
		"quickfix",
		uri,
		[]Diagnostic{diag},
		TextEdit{Range: editRange, NewText: ""},
	))
}

func addCreateMissingImportFileAction(
	actions []CodeAction,
	diag Diagnostic,
) []CodeAction {
	if fmt.Sprint(diag.Code) != "E0701" {
		return actions
	}

	targetURI, ok := firstMissingImportFileURI(diag.Data)
	if !ok {
		return actions
	}
	path := uriToPath(targetURI)
	return append(actions, CodeAction{
		Title:       "Create missing import file",
		Kind:        "quickfix",
		Diagnostics: []Diagnostic{diag},
		Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{
			targetURI: {{
				Range: Range{
					Start: Position{Line: 0, Character: 0},
					End:   Position{Line: 0, Character: 0},
				},
				NewText: strfmt.Named("package {name}\n\n", "Name", packageNameFromImportFile(path)),
			}},
		}},
	})
}

func firstMissingImportFileURI(data any) (string, bool) {
	payload, ok := diagnosticDataFromAny(data)
	if !ok {
		return "", false
	}
	for _, note := range payload.Notes {
		if !strings.HasPrefix(note.Message, "tried ") || note.URI == "" {
			continue
		}
		path := uriToPath(note.URI)
		if path == "" || !strings.HasSuffix(path, ".bak") {
			continue
		}
		return note.URI, true
	}
	return "", false
}

func packageNameFromImportFile(path string) string {
	base := filepathBaseNoExt(path)
	if base == "" {
		return "main"
	}
	return sanitizePackageName(base)
}

func filepathBaseNoExt(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	base := path
	if idx := strings.LastIndexAny(base, `/\`); idx >= 0 {
		base = base[idx+1:]
	}
	return strings.TrimSuffix(base, ".bak")
}

func sanitizePackageName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "main"
	}
	var out strings.Builder
	for i, r := range name {
		ok := r == '_' ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(i > 0 && r >= '0' && r <= '9')
		if ok {
			out.WriteRune(r)
			continue
		}
		if i > 0 {
			out.WriteByte('_')
		}
	}
	if out.Len() == 0 {
		return "main"
	}
	return out.String()
}

func removeImportDiagnosticTitle(code string) (string, bool) {
	switch code {
	case "E0701":
		return "Remove unresolved import", true
	case "E0703":
		return "Remove self import", true
	case "E0704":
		return "Remove import cycle edge", true
	default:
		return "", false
	}
}

func fullLineDeletionRange(text string, line int) (Range, bool) {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return Range{}, false
	}
	end := Position{Line: line, Character: len(lines[line])}
	if line+1 < len(lines) {
		end = Position{Line: line + 1, Character: 0}
	}
	return Range{
		Start: Position{Line: line, Character: 0},
		End:   end,
	}, true
}

func addRemoveAllUnusedAction(
	actions []CodeAction,
	uri string,
	diagnostics []Diagnostic,
) []CodeAction {
	unused := make([]Diagnostic, 0, len(diagnostics))
	edits := make([]TextEdit, 0, len(diagnostics))

	for _, diag := range diagnostics {
		if !strings.Contains(diag.Message, "unused") {
			continue
		}
		unused = append(unused, diag)
		edits = append(edits, TextEdit{
			Range:   diag.Range,
			NewText: "",
		})
	}
	if len(unused) <= 1 {
		return actions
	}

	return append(actions, CodeAction{
		Title:       "Remove all unused declarations",
		Kind:        "quickfix",
		Diagnostics: unused,
		Edit:        &WorkspaceEdit{Changes: map[string][]TextEdit{uri: edits}},
	})
}

func addAutoImportActions(actions []CodeAction, result *AnalysisResult, uri string, diagnostics []Diagnostic) []CodeAction {
	insertPos := findImportInsertPosition(result)
	for _, diag := range diagnostics {
		symbolName := extractUndefinedSymbol(diag.Message)
		if symbolName == "" {
			continue
		}
		candidates := lookupStdlibSymbol(symbolName)
		if len(candidates) == 0 {
			continue
		}
		for _, candidate := range candidates {
			importLine := strfmt.Named("import {Alias} \"{ImportPath}\"\n", "ImportPath", candidate.ImportPath, "Alias", candidate.Alias)
			actions = append(actions, codeActionWithInsertion(
				strfmt.Named(
					"Import '{SymbolName}' from {Alias}",
					"SymbolName", symbolName,
					"Alias", candidate.Alias,
				),
				"quickfix",
				uri,
				[]Diagnostic{diag},
				insertPos,
				importLine,
			))
		}
	}
	return actions
}

func (s *Server) addLocalAutoImportActions(actions []CodeAction, result *AnalysisResult, uri string, diagnostics []Diagnostic) []CodeAction {
	if result == nil {
		return actions
	}
	insertPos := findImportInsertPosition(result)
	currentPath := uriToPath(uri)
	currentDir := filepath.Dir(currentPath)
	imported := map[string]bool{}
	for alias := range result.Imports {
		imported[alias] = true
	}

	for _, diag := range diagnostics {
		symbolName := extractUndefinedSymbol(diag.Message)
		if symbolName == "" {
			continue
		}
		for _, idx := range s.indexSnapshot() {
			if idx == nil || idx.Symbols == nil {
				continue
			}
			sym, ok := idx.Symbols[symbolName]
			if !ok || !sym.Exported || sym.Location.URI == "" || sym.Location.URI == uri {
				continue
			}
			targetPath := uriToPath(sym.Location.URI)
			if targetPath == "" {
				continue
			}
			importPath, alias := localImportPathAndAlias(currentDir, targetPath)
			if importPath == "" || alias == "" || importPathAlreadyPresent(result.Imports, importPath, currentDir) {
				continue
			}
			alias, ok = uniqueImportAlias(imported, alias)
			if !ok {
				continue
			}
			importLine := strfmt.Named("import {Alias} \"{ImportPath}\"\n", "Alias", alias, "ImportPath", importPath)
			actions = append(actions, codeActionWithInsertion(
				strfmt.Named("Import '{SymbolName}' from {Alias}", "SymbolName", symbolName, "Alias", alias),
				"quickfix",
				uri,
				[]Diagnostic{diag},
				insertPos,
				importLine,
			))
		}
	}
	return actions
}

func localImportPathAndAlias(currentDir, targetPath string) (string, string) {
	rel, err := filepath.Rel(currentDir, targetPath)
	if err != nil || rel == "" || strings.HasPrefix(rel, "..") {
		return "", ""
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	alias := localPackageName(targetPath)
	if alias == "" {
		alias = strings.TrimSuffix(filepath.Base(targetPath), ".bak")
	}
	alias = sanitizePackageName(alias)
	return rel, alias
}

func localPackageName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[0] == "package" {
			return fields[1]
		}
	}
	return ""
}

func importPathAlreadyPresent(imports map[string]string, importPath, currentDir string) bool {
	for _, existing := range imports {
		if existing == importPath {
			return true
		}
		if currentDir == "" {
			continue
		}
		resolved := existing
		if !filepath.IsAbs(resolved) && strings.HasPrefix(existing, ".") {
			resolved = filepath.ToSlash(filepath.Clean(filepath.Join(currentDir, existing)))
		}
		target := importPath
		if !filepath.IsAbs(target) && strings.HasPrefix(importPath, ".") {
			target = filepath.ToSlash(filepath.Clean(filepath.Join(currentDir, importPath)))
		}
		if resolved == target {
			return true
		}
	}
	return false
}

func uniqueImportAlias(imported map[string]bool, alias string) (string, bool) {
	alias = sanitizePackageName(alias)
	if alias == "" {
		return "", false
	}
	if !imported[alias] {
		imported[alias] = true
		return alias, true
	}
	for i := 2; i < 100; i++ {
		candidate := strfmt.Named("{Alias}{N}", "Alias", alias, "N", i)
		if !imported[candidate] {
			imported[candidate] = true
			return candidate, true
		}
	}
	return "", false
}

func addMutabilityActions(actions []CodeAction, uri, text string, diagnostics []Diagnostic) []CodeAction {
	for _, diag := range diagnostics {
		varName := extractMutabilityVariable(diag.Message)
		if varName == "" {
			continue
		}
		if edit, ok := makeVariableMutableEdit(text, varName); ok {
			actions = append(actions, codeActionWithTextEdit(
				strfmt.Named("Make '{Var}' mutable", "Var", varName),
				"quickfix",
				uri,
				[]Diagnostic{diag},
				edit,
			))
		}
	}
	return actions
}

func makeVariableMutableEdit(text, varName string) (TextEdit, bool) {
	lines := strings.Split(text, "\n")
	for lineNo, line := range lines {
		idx := strings.Index(line, "var "+varName)
		if idx == -1 {
			continue
		}
		prefix := line[:idx]
		if strings.Contains(prefix, "mut ") {
			continue
		}
		afterStart := idx + len("var "+varName)
		if afterStart < len(line) && isWordChar(line[afterStart]) {
			continue
		}
		return TextEdit{
			Range: Range{
				Start: Position{Line: lineNo, Character: idx},
				End:   Position{Line: lineNo, Character: idx + len("var")},
			},
			NewText: "mut var",
		}, true
	}
	return TextEdit{}, false
}

func addCreateMissingMethodActions(actions []CodeAction, uri, text string, result *AnalysisResult, diagnostics []Diagnostic) []CodeAction {
	for _, diag := range diagnostics {
		methodName, typeName, ok := extractUndefinedMethod(diag.Message)
		if !ok {
			continue
		}
		call := methodCallAtDiagnostic(text, diag.Range.Start.Line, methodName)
		if call.receiver == "" {
			continue
		}
		if declared := declaredLocalTypeBefore(text, diag.Range.Start.Line, call.receiver); declared != "" {
			typeName = baseTypeName(declared)
		}
		if typeName == "" {
			continue
		}
		methodText := missingMethodText(typeName, methodName, call.args)
		if methodText == "" {
			continue
		}
		insertPos, prefix := missingMethodInsertPosition(text, result, typeName)
		actions = append(actions, codeActionWithInsertion(
			strfmt.Named("Create method '{Method}' on {Type}", "Method", methodName, "Type", typeName),
			"quickfix",
			uri,
			[]Diagnostic{diag},
			insertPos,
			prefix+methodText,
		))
	}
	return actions
}

func addCreateMissingFieldActions(actions []CodeAction, uri, text string, result *AnalysisResult, diagnostics []Diagnostic) []CodeAction {
	for _, diag := range diagnostics {
		fieldName, typeName, ok := extractMissingField(diag.Message)
		if !ok {
			continue
		}
		fieldType := inferMissingFieldType(text, diag.Range.Start.Line, fieldName)
		line, ok := structClosingLine(text, result, typeName)
		if !ok {
			continue
		}
		actions = append(actions, codeActionWithInsertion(
			strfmt.Named("Add field '{Field}' to {Type}", "Field", fieldName, "Type", typeName),
			"quickfix",
			uri,
			[]Diagnostic{diag},
			Position{Line: line, Character: 0},
			strfmt.Named("    {Field}: {Type}\n", "Field", fieldName, "Type", fieldType),
		))
	}
	return actions
}

type missingMethodCall struct {
	receiver string
	args     []string
}

func methodCallAtDiagnostic(text string, lineNo int, methodName string) missingMethodCall {
	line := lineAt(text, lineNo)
	methodIdx := strings.Index(line, "."+methodName+"(")
	if methodIdx == -1 {
		return missingMethodCall{}
	}
	receiver := strings.TrimSpace(line[:methodIdx])
	if fields := strings.Fields(receiver); len(fields) > 0 {
		receiver = fields[len(fields)-1]
	}
	argsStart := methodIdx + len("."+methodName+"(")
	argsEnd := strings.LastIndex(line, ")")
	if argsEnd < argsStart {
		argsEnd = len(line)
	}
	return missingMethodCall{
		receiver: receiver,
		args:     splitCallArgs(line[argsStart:argsEnd]),
	}
}

func splitCallArgs(args string) []string {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil
	}
	var out []string
	depth := 0
	start := 0
	for i, ch := range args {
		switch ch {
		case '(', '{', '[':
			depth++
		case ')', '}', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(args[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(args[start:]))
	return out
}

func missingMethodText(typeName, methodName string, args []string) string {
	receiver := receiverNameForType(typeName)
	paramNames := missingMethodParamNames(methodName, len(args))
	params := make([]string, 0, len(args))
	for i, arg := range args {
		name := strfmt.Named("arg{N}", "N", i+1)
		if i < len(paramNames) && paramNames[i] != "" {
			name = paramNames[i]
		}
		params = append(params, strfmt.Named("{Name}: {Type}", "Name", name, "Type", inferLiteralType(arg)))
	}
	body := "        return void"
	if len(params) == 1 && strings.HasPrefix(methodName, "set") && len(methodName) > 3 {
		body = fmt.Sprintf("        %s.%s = %s\n        return void", receiver, paramNames[0], paramNames[0])
	}
	return fmt.Sprintf(
		"impl %s as %s {\n    mut func %s(%s) -> (void) {\n%s\n    }\n}\n",
		typeName,
		receiver,
		methodName,
		strings.Join(params, ", "),
		body,
	)
}

func receiverNameForType(typeName string) string {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return "self"
	}
	return strings.ToLower(typeName[:1])
}

func missingMethodParamNames(methodName string, count int) []string {
	if count == 1 && strings.HasPrefix(methodName, "set") && len(methodName) > 3 {
		field := methodName[3:]
		return []string{strings.ToLower(field[:1]) + field[1:]}
	}
	names := make([]string, count)
	for i := range names {
		names[i] = strfmt.Named("arg{N}", "N", i+1)
	}
	return names
}

func inferLiteralType(arg string) string {
	arg = strings.TrimSpace(arg)
	switch {
	case strings.HasPrefix(arg, "\"") || strings.HasPrefix(arg, "`"):
		return "string"
	case arg == "true" || arg == "false":
		return "bool"
	case strings.Contains(arg, "."):
		return "float64"
	case arg == "":
		return "any"
	default:
		return "int"
	}
}

func missingMethodInsertPosition(text string, result *AnalysisResult, typeName string) (Position, string) {
	if result != nil && result.AST != nil {
		for _, stmt := range result.AST.Statements {
			if st, ok := stmt.(*ast.StructDecl); ok && st != nil && st.Name != nil && st.Name.Value == typeName {
				if r, ok := rangeFromSpan(st.Span); ok {
					return Position{Line: r.End.Line + 1, Character: 0}, "\n"
				}
			}
		}
	}
	lines := strings.Split(text, "\n")
	line := len(lines)
	if line > 0 && strings.TrimSpace(lines[line-1]) == "" {
		line--
	}
	return Position{Line: line, Character: 0}, "\n"
}

func inferMissingFieldType(text string, lineNo int, fieldName string) string {
	line := lineAt(text, lineNo)
	if idx := strings.Index(line, fieldName+":"); idx != -1 {
		value := strings.TrimSpace(line[idx+len(fieldName)+1:])
		value = strings.TrimRight(value, ",")
		return inferLiteralType(value)
	}
	if idx := strings.Index(line, "."+fieldName); idx != -1 {
		rest := line[idx+len(fieldName)+1:]
		if eq := strings.Index(rest, "="); eq != -1 {
			return inferLiteralType(rest[eq+1:])
		}
	}
	return "any"
}

func structClosingLine(text string, result *AnalysisResult, typeName string) (int, bool) {
	if result != nil && result.AST != nil {
		for _, stmt := range result.AST.Statements {
			st, ok := stmt.(*ast.StructDecl)
			if !ok || st == nil || st.Name == nil || st.Name.Value != typeName {
				continue
			}
			if r, ok := rangeFromSpan(st.Span); ok {
				return r.End.Line, true
			}
		}
	}
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "struct "+typeName) || !strings.Contains(trimmed, "{") {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "}") {
				return j, true
			}
		}
	}
	return 0, false
}

func addSwitchCaseActions(actions []CodeAction, uri, text string, result *AnalysisResult, pos Position) []CodeAction {
	if result == nil || result.AST == nil || result.TC == nil {
		return addTextualSwitchCaseActions(actions, uri, text, pos)
	}
	sw := switchAtPosition(result.AST, pos)
	if sw == nil {
		return addTextualSwitchCaseActions(actions, uri, text, pos)
	}
	typeStr := result.TC.GetNodeType(sw.Value)
	if typeStr == "" {
		if ident, ok := sw.Value.(*ast.Identifier); ok && ident != nil {
			typeStr = declaredLocalTypeBefore(text, pos.Line, ident.Value)
		}
	}
	enumInfo, ok := enumInfoForSwitchType(result, typeStr)
	if !ok {
		return addTextualSwitchCaseActions(actions, uri, text, pos)
	}
	missing := missingSwitchVariants(sw, enumInfo)
	if len(missing) == 0 {
		return actions
	}
	r, ok := rangeFromSpan(sw.Span)
	if !ok {
		return actions
	}
	return append(actions, codeActionWithInsertion(
		"Add missing enum cases",
		"quickfix",
		uri,
		nil,
		Position{Line: r.End.Line, Character: 0},
		formatMissingSwitchCases(missing),
	))
}

func addTextualSwitchCaseActions(actions []CodeAction, uri, text string, pos Position) []CodeAction {
	switchLine, switchVar, closeLine, ok := textualSwitchAt(text, pos.Line)
	if !ok || switchVar == "" {
		return actions
	}
	typeName := baseTypeName(declaredLocalTypeBefore(text, switchLine, switchVar))
	if typeName == "" {
		return actions
	}
	variants := textualEnumVariantInfos(text, typeName)
	if len(variants) == 0 {
		return actions
	}
	seen := textualSwitchCaseNames(text, switchLine, closeLine)
	missing := []EnumVariantInfo{}
	for _, variant := range variants {
		if !seen[variant.Name] {
			missing = append(missing, variant)
		}
	}
	if len(missing) == 0 {
		return actions
	}
	return append(actions, codeActionWithInsertion(
		"Add missing enum cases",
		"quickfix",
		uri,
		nil,
		Position{Line: closeLine, Character: 0},
		formatMissingSwitchCases(missing),
	))
}

func textualSwitchAt(text string, line int) (int, string, int, bool) {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return 0, "", 0, false
	}
	start := line
	for start >= 0 && !strings.Contains(lines[start], "switch ") {
		start--
	}
	if start < 0 {
		return 0, "", 0, false
	}
	trimmed := strings.TrimSpace(lines[start])
	if !strings.HasPrefix(trimmed, "switch ") {
		return 0, "", 0, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "switch "))
	rest = strings.TrimSpace(strings.TrimSuffix(rest, "{"))
	if rest == "" {
		return 0, "", 0, false
	}
	depth := 0
	for i := start; i < len(lines); i++ {
		depth += strings.Count(lines[i], "{")
		depth -= strings.Count(lines[i], "}")
		if i > start && depth <= 0 {
			return start, rest, i, true
		}
	}
	return start, rest, len(lines) - 1, true
}

func textualEnumVariantInfos(text, typeName string) []EnumVariantInfo {
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "enum "+typeName) || !strings.Contains(trimmed, "{") {
			continue
		}
		var variants []EnumVariantInfo
		for i++; i < len(lines); i++ {
			line := strings.TrimSpace(strings.TrimRight(lines[i], ","))
			if strings.HasPrefix(line, "}") {
				return variants
			}
			name := line
			var fields []string
			if before, after, ok := strings.Cut(line, "("); ok {
				name = strings.TrimSpace(before)
				fieldsText := strings.TrimSuffix(after, ")")
				fields = splitCallArgs(fieldsText)
			}
			if name != "" && bakIdentifierPattern.MatchString(name) {
				variants = append(variants, EnumVariantInfo{Name: name, Fields: fields})
			}
		}
	}
	return nil
}

func textualSwitchCaseNames(text string, startLine, endLine int) map[string]bool {
	lines := strings.Split(text, "\n")
	seen := map[string]bool{}
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}
	for i := startLine + 1; i <= endLine; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "case ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "case "))
		rest = strings.TrimSpace(strings.TrimSuffix(rest, "{"))
		for _, part := range strings.Split(rest, ",") {
			name := strings.TrimSpace(part)
			if before, _, ok := strings.Cut(name, "("); ok {
				name = strings.TrimSpace(before)
			}
			if name != "" {
				seen[name] = true
			}
		}
	}
	return seen
}

func switchAtPosition(prog *ast.Program, pos Position) *ast.SwitchStatement {
	if prog == nil {
		return nil
	}
	line := pos.Line + 1
	col := pos.Character + 1
	var found *ast.SwitchStatement
	var fallback *ast.SwitchStatement
	ast.Walk(prog, func(n ast.Node) {
		if found != nil {
			return
		}
		sw, ok := n.(*ast.SwitchStatement)
		if !ok || sw == nil {
			return
		}
		if sw.Token.Line <= line {
			fallback = sw
		}
		if spanContainsPosition(sw.Span, line, col) {
			found = sw
		}
	})
	if found == nil {
		return fallback
	}
	return found
}

func enumInfoForSwitchType(result *AnalysisResult, typeStr string) (EnumInfo, bool) {
	base := baseTypeName(typeStr)
	if base == "" {
		return EnumInfo{}, false
	}
	if base == "Result" {
		return EnumInfo{
			Name: "Result",
			VariantDetails: []EnumVariantInfo{
				{Name: "Ok", Fields: []string{"T"}},
				{Name: "Err", Fields: []string{"E"}},
			},
		}, true
	}
	if result.Index != nil && result.Index.Enums != nil {
		if info, ok := result.Index.Enums[base]; ok {
			return info, true
		}
	}
	return EnumInfo{}, false
}

func missingSwitchVariants(sw *ast.SwitchStatement, enumInfo EnumInfo) []EnumVariantInfo {
	seen := map[string]bool{}
	for _, c := range sw.Cases {
		if c == nil {
			continue
		}
		if c.Default {
			return nil
		}
		for _, value := range c.Values {
			if name := switchCaseVariantName(value); name != "" {
				seen[name] = true
			}
		}
	}
	missing := []EnumVariantInfo{}
	for _, variant := range enumVariantDetails(enumInfo) {
		if variant.Name != "" && !seen[variant.Name] {
			missing = append(missing, variant)
		}
	}
	return missing
}

func switchCaseVariantName(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.Identifier:
		if e != nil {
			return e.Value
		}
	case *ast.CallExpression:
		if ident, ok := e.Function.(*ast.Identifier); ok && ident != nil {
			return ident.Value
		}
	case *ast.MethodCallExpression:
		if e != nil && e.Method != nil {
			return e.Method.Value
		}
	}
	return ""
}

func formatMissingSwitchCases(variants []EnumVariantInfo) string {
	var b strings.Builder
	for _, variant := range variants {
		label := variant.Name
		if len(variant.Fields) > 0 {
			args := make([]string, len(variant.Fields))
			for i := range args {
				args[i] = "_"
			}
			label += "(" + strings.Join(args, ", ") + ")"
		}
		b.WriteString("    case ")
		b.WriteString(label)
		b.WriteString(" {\n    }\n")
	}
	return b.String()
}

func (s *Server) getOrganizeImportsEdit(uri string) *WorkspaceEdit {
	result := s.analysisResultOrNil(uri)
	if result == nil || result.AST == nil {
		return nil
	}

	var firstImport *ast.ImportStatement
	var lastImport *ast.ImportStatement
	imports := []*ast.ImportStatement{}

	for _, stmt := range result.AST.Statements {
		if imp, ok := stmt.(*ast.ImportStatement); ok {
			if imp == nil {
				continue
			}
			if firstImport == nil {
				firstImport = imp
			}
			lastImport = imp
			imports = append(imports, imp)
		}
	}

	if len(imports) == 0 {
		return nil
	}

	sort.Slice(imports, func(i, j int) bool {
		return imports[i].Path < imports[j].Path
	})

	var sb strings.Builder
	seen := make(map[string]bool)
	for _, imp := range imports {
		if imp == nil {
			continue
		}
		path := imp.Path
		if seen[path] {
			continue
		}
		seen[path] = true
		sb.WriteString("import ")
		if imp.Alias != "" {
			sb.WriteString(imp.Alias)
			sb.WriteString(" ")
		}
		sb.WriteString("\"")
		sb.WriteString(path)
		sb.WriteString("\"\n")
	}

	start := positionFromLineCol(firstImport.Token.Line, firstImport.Token.Column)
	end := positionFromLineCol(lastImport.Token.Line, 1)
	end.Character = 1000

	return &WorkspaceEdit{
		Changes: map[string][]TextEdit{
			uri: {
				{
					Range:   rangeFromPositions(start, end),
					NewText: sb.String(),
				},
			},
		},
	}
}

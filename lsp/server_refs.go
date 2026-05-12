package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/formatter"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/token"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func rangeFromToken(tok token.Token, name string) (out Range) {
	if tok.Line <= 0 || tok.Column <= 0 {
		return out
	}

	length := len(name)
	if length == 0 {
		length = 1
	}
	return rangeFromLineCol(tok.Line, tok.Column, length)
}

func positionFromLineCol(line, col int) Position {
	return Position{
		Line:      max(line-1, 0),
		Character: max(col-1, 0),
	}
}

func rangeFromLineCol(line, col, length int) Range {
	if length <= 0 {
		length = 1
	}
	start := positionFromLineCol(line, col)
	return Range{
		Start: start,
		End: Position{
			Line:      start.Line,
			Character: start.Character + length,
		},
	}
}

func rangeFromLineColBounds(startLine, startCol, endLine, endCol int) Range {
	start := positionFromLineCol(startLine, startCol)
	endLine = endLine - 1
	endCharacter := endCol - 1
	if endLine < start.Line {
		endLine = start.Line
	}
	if endCharacter < 0 {
		endCharacter = start.Character + 1
	}
	if endLine == start.Line && endCharacter <= start.Character {
		endCharacter = start.Character + 1
	}
	end := Position{Line: endLine, Character: endCharacter}
	return rangeFromPositions(start, end)
}

func rangeFromPositions(start, end Position) Range {
	return Range{Start: start, End: end}
}

func rangeFromSpan(span ast.Span) (Range, bool) {
	if span.Start.Line == 0 || span.End.Line == 0 {
		return Range{}, false
	}

	return rangeFromPositions(
		positionFromLineCol(span.Start.Line, span.Start.Column),
		positionFromLineCol(span.End.Line, span.End.Column),
	), true
}

func positionInSpan(span ast.Span, line, col int) bool {
	if span.Start.Line == 0 || span.End.Line == 0 {
		return false
	}
	if line < span.Start.Line || line > span.End.Line {
		return false
	}
	if line == span.Start.Line && col < span.Start.Column {
		return false
	}
	if line == span.End.Line && col >= span.End.Column {
		return false
	}
	return true
}

func hoverRange(span ast.Span) *Range {
	if r, ok := rangeFromSpan(span); ok {
		return &r
	}
	return nil
}

func locationFromToken(uri string, tok token.Token, name string) Location {
	if name == "" {
		name = tok.Literal
	}
	return Location{
		URI:   uri,
		Range: rangeFromToken(tok, name),
	}
}

func symbolID(kind string, loc Location) string {
	return strfmt.Named(
		"{kind}:{URI}:{Line}:{Character}",
		"kind", kind,
		"URI", loc.URI,
		"Line", loc.Range.Start.Line,
		"Character", loc.Range.Start.Character,
	)
}

func tokenKey(tok token.Token) string {
	return strfmt.Named(
		"{Line}:{Column}",
		"Line", tok.Line,
		"Column", tok.Column,
	)
}

func symbolDefFromInfo(sym SymbolInfo, name, container string) *symbolDef {
	if name == "" {
		name = sym.Name
	}
	return &symbolDef{
		ID:        symbolID(sym.Kind, sym.Location),
		Name:      name,
		Kind:      sym.Kind,
		Location:  sym.Location,
		Container: container,
	}
}

func nodeRefKey(node ast.Node) string {
	switch n := node.(type) {
	case *ast.Identifier:
		return tokenKey(n.Token)
	case *ast.FieldAccessExpression:
		if n.Field != nil {
			return tokenKey(n.Field.Token)
		}
	case *ast.MethodCallExpression:
		if n.Method != nil {
			return tokenKey(n.Method.Token)
		}
	}
	return ""
}

// makeLocation builds an LSP Location from 1-based line/col and a name length.
func makeLocation(uri string, line, col, length int) Location {
	if line <= 0 || col <= 0 {
		return Location{}
	}
	return Location{
		URI:   uri,
		Range: rangeFromLineCol(line, col, length),
	}
}

// forEachBakFile walks the workspace and calls fn for every .bak file.
func (s *Server) forEachBakFile(fn func(path, uri string)) {
	root := s.rootPath()
	if root == "" {
		return
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".bak") {
			return nil
		}
		fn(path, pathToURI(path))
		return nil
	})
}

func (s *Server) ensureWorkspaceRefIndex() {
	s.forEachBakFile(func(path, uri string) {
		res := s.analysisResultOrNil(uri)
		if res != nil && res.RefIndex != nil && res.Defs != nil {
			return
		}

		var prog *ast.Program
		var idx *FileIndex
		var imports map[string]string
		var tc *typechecker.TypeChecker

		if res != nil && res.AST != nil {
			prog = res.AST
			tc = res.TC
			idx = res.Index
			imports = res.Imports
		} else {
			data, err := os.ReadFile(path)
			if err != nil {
				return
			}
			text := string(data)
			l := lexer.New(text)
			p := parser.New(l)
			prog = p.ParseProgram()
			comments := formatter.ScanComments(text)
			idx = indexProgram(prog, uri, comments, true)
			imports = collectImports(prog)
		}

		if idx == nil {
			idx = &FileIndex{
				Symbols: make(map[string]SymbolInfo),
				Docs:    make(map[string]string),
				Sigs:    make(map[string]SignatureInfo),
				Structs: make(map[string]StructInfo),
			}
		}
		updated := res
		if updated == nil {
			updated = &AnalysisResult{}
		}
		refIndex, refByPos, defs := buildReferenceIndex(prog, tc, uri, imports, idx, s)
		updated.AST = prog
		updated.TC = tc
		updated.Index = idx
		updated.Imports = imports
		updated.RefIndex = refIndex
		updated.RefByPos = refByPos
		updated.Defs = defs
		s.setAnalysisResult(uri, idx, updated)
	})
}

func (s *Server) ensureWorkspaceIndexes() {
	s.forEachBakFile(func(path, uri string) {
		if _, ok := s.indexSnapshot()[uri]; ok {
			return
		}
		if idx := s.getOrIndexFile(path); idx != nil {
			s.setIndex(uri, idx)
		}
	})
}

func formatFuncDetail(params []*ast.Parameter, ret ast.TypeExpression, mutable bool) string {
	paramLabels := make([]string, 0, len(params))
	for i, p := range params {
		if p == nil {
			continue
		}
		paramName := strfmt.Named("arg{Expr}", "Expr", i+1)
		if p.Name != nil && p.Name.Value != "" {
			paramName = p.Name.Value
		}
		if p.Mutable {
			paramName = "mut " + paramName
		}
		typ := "void"
		if p.Type != nil {
			typ = p.Type.String()
		}
		paramLabels = append(paramLabels, strfmt.Named(
			"{paramName}: {typ}",
			"paramName", paramName,
			"typ", typ,
		))
	}
	retType := "void"
	if ret != nil {
		retType = ret.String()
	}
	mut := ""
	if mutable {
		mut = "mut "
	}
	return strfmt.Named(
		"{mut}func({paramLabels}) -> ({retType})",
		"mut", mut,
		"paramLabels", strings.Join(paramLabels, ", "),
		"retType", retType,
	)
}

func collectReferences(prog *ast.Program, uri string, name string) []Location {
	var locs []Location
	ast.Walk(prog, func(node ast.Node) {
		if n, ok := node.(*ast.Identifier); ok && n.Value == name {
			locs = append(locs, locationFromToken(uri, n.Token, n.Value))
		}
	})
	return locs
}

func referenceTarget(s *Server, uri string, node ast.Node) (name, modulePath string, localOnly, ok bool) {
	switch n := node.(type) {
	case *ast.MethodCallExpression:
		if n.Method == nil {
			return "", "", false, false
		}
		name = n.Method.Value
		if ident, ok := n.Object.(*ast.Identifier); ok {
			if res, ok := s.analysisResult(uri); ok && res.Imports != nil {
				if importPath, ok := res.Imports[ident.Value]; ok {
					modulePath = s.resolveImportPath(uriToPath(uri), importPath)
				}
			}
		}
		return name, modulePath, false, true
	case *ast.Identifier:
		name = n.Value
		if res, ok := s.analysisResult(uri); ok && res.Index != nil {
			if sym, ok := res.Index.Symbols[name]; ok {
				if sym.Location.URI == uri && sym.Location.Range.Start.Line == n.Token.Line-1 {
					return name, uriToPath(uri), false, true
				}
			}
		}
		return name, "", true, true
	default:
		return "", "", false, false
	}
}

func referencesFromFile(s *Server, fileURI string, res *AnalysisResult, modulePath, name string) []Location {
	if res == nil || res.AST == nil {
		return nil
	}
	if uriToPath(fileURI) == modulePath {
		return collectReferences(res.AST, fileURI, name)
	}
	if modulePath == "" || res.Imports == nil {
		return nil
	}
	refs := []Location{}
	for alias, importPath := range res.Imports {
		resolved := s.resolveImportPath(uriToPath(fileURI), importPath)
		if resolved != modulePath {
			continue
		}
		refs = append(refs, collectQualifiedMethodRefs(res.AST, fileURI, alias, name)...)
	}
	return refs
}

func collectWorkspaceReferences(s *Server, uri string, node ast.Node) []Location {
	name, modulePath, localOnly, ok := referenceTarget(s, uri, node)
	if !ok || name == "" {
		return nil
	}

	if localOnly {
		if res, ok := s.analysisResult(uri); ok && res.AST != nil {
			return collectReferences(res.AST, uri, name)
		}
		return nil
	}

	var refs []Location
	for fileURI, res := range s.cacheSnapshot() {
		refs = append(refs, referencesFromFile(s, fileURI, res, modulePath, name)...)
	}
	return refs
}

type symbolDef struct {
	ID        string
	Name      string
	Kind      string
	Location  Location
	Container string
}

type scope struct {
	parent  *scope
	symbols map[string]*symbolDef
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, symbols: make(map[string]*symbolDef)}
}

func (s *scope) define(def *symbolDef) {
	if def == nil {
		return
	}
	s.symbols[def.Name] = def
}

func (s *scope) lookup(name string) *symbolDef {
	for sc := s; sc != nil; sc = sc.parent {
		if def, ok := sc.symbols[name]; ok {
			return def
		}
	}
	return nil
}

type refBuilder struct {
	uri           string
	imports       map[string]string
	srv           *Server
	tc            *typechecker.TypeChecker
	refs          map[string][]Location
	refByPos      map[string]string
	defs          map[string]Location
	structFields  map[string]map[string]*symbolDef
	structMethods map[string]map[string]*symbolDef
	typeDefs      map[string]*symbolDef
	global        *scope
}

func (b *refBuilder) newDef(name, kind string, tok token.Token, container string) *symbolDef {
	loc := locationFromToken(b.uri, tok, name)
	def := &symbolDef{
		ID:        symbolID(kind, loc),
		Name:      name,
		Kind:      kind,
		Location:  loc,
		Container: container,
	}
	b.defs[def.ID] = def.Location
	b.refByPos[tokenKey(tok)] = def.ID
	return def
}

func (b *refBuilder) defineStructField(structName, fieldName string, tok token.Token) {
	if b.structFields[structName] == nil {
		b.structFields[structName] = make(map[string]*symbolDef)
	}
	b.structFields[structName][fieldName] = b.newDef(fieldName, "field", tok, structName)
}

func (b *refBuilder) defineStructMethod(structName, methodName string, tok token.Token) {
	if b.structMethods[structName] == nil {
		b.structMethods[structName] = make(map[string]*symbolDef)
	}
	b.structMethods[structName][methodName] = b.newDef(methodName, "method", tok, structName)
}

func (b *refBuilder) recordRef(def *symbolDef, node ast.Node, tok token.Token) {
	if def == nil {
		return
	}
	loc := locationFromToken(b.uri, tok, tok.Literal)
	if node != nil {
		if r, ok := rangeFromSpan(ast.SpanOf(node)); ok {
			loc = Location{URI: b.uri, Range: r}
		}
	}
	b.refs[def.ID] = append(b.refs[def.ID], loc)
	b.refByPos[tokenKey(tok)] = def.ID
}

func (b *refBuilder) recordRefTok(def *symbolDef, tok token.Token) {
	b.recordRef(def, nil, tok)
}

func (b *refBuilder) resolveModuleSymbol(alias, name string) *symbolDef {
	if b.srv == nil {
		return nil
	}
	importPath, ok := b.imports[alias]
	if !ok {
		return nil
	}
	path := b.srv.resolveImportPath(uriToPath(b.uri), importPath)
	if path == "" {
		return nil
	}
	modIndex := b.srv.getOrIndexFile(path)
	if modIndex == nil {
		return nil
	}
	if sym, ok := modIndex.Symbols[name]; ok {
		return symbolDefFromInfo(sym, name, "")
	}
	return nil
}

func (b *refBuilder) resolveStructMember(structName, memberName, kind string) *symbolDef {
	if structName == "" || memberName == "" {
		return nil
	}
	if kind == "field" {
		if fields := b.structFields[structName]; fields != nil {
			if def, ok := fields[memberName]; ok {
				return def
			}
		}
	}
	if kind == "method" {
		if methods := b.structMethods[structName]; methods != nil {
			if def, ok := methods[memberName]; ok {
				return def
			}
		}
	}
	if b.srv == nil {
		return nil
	}
	key := structName + "." + memberName
	for _, index := range b.srv.Indexes {
		if index == nil {
			continue
		}
		if sym, ok := index.Symbols[key]; ok {
			return symbolDefFromInfo(sym, memberName, structName)
		}
	}
	return nil
}

func (b *refBuilder) resolveTypeRef(name string, tok token.Token) {
	if def, ok := b.typeDefs[name]; ok {
		b.recordRefTok(def, tok)
	} else if def := b.global.lookup(name); def != nil && (def.Kind == "struct" || def.Kind == "enum" || def.Kind == "type" || def.Kind == "alias") {
		b.recordRefTok(def, tok)
	}
}

func (b *refBuilder) indexGlobals(prog *ast.Program) {
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			if s != nil && s.Name != nil {
				b.global.define(b.newDef(s.Name.Value, "func", s.Name.Token, ""))
			}
		case *ast.StructDecl:
			if s != nil && s.Name != nil {
				def := b.newDef(s.Name.Value, "struct", s.Name.Token, "")
				b.global.define(def)
				b.typeDefs[s.Name.Value] = def
				for _, f := range s.Fields {
					if f == nil || f.Name == nil {
						continue
					}
					b.defineStructField(s.Name.Value, f.Name.Value, f.Name.Token)
				}
			}
		case *ast.EnumDecl:
			if s != nil && s.Name != nil {
				def := b.newDef(s.Name.Value, "enum", s.Name.Token, "")
				b.global.define(def)
				b.typeDefs[s.Name.Value] = def
			}
		case *ast.TypeDecl:
			if s != nil && s.Name != nil {
				def := b.newDef(s.Name.Value, "type", s.Name.Token, "")
				b.global.define(def)
				b.typeDefs[s.Name.Value] = def
			}
		case *ast.AliasDecl:
			if s != nil && s.Name != nil {
				def := b.newDef(s.Name.Value, "alias", s.Name.Token, "")
				b.global.define(def)
				b.typeDefs[s.Name.Value] = def
			}
		case *ast.ConstStatement:
			if s != nil && s.Name != nil {
				b.global.define(b.newDef(s.Name.Value, "const", s.Name.Token, ""))
			}
		case *ast.VarStatement:
			if s != nil && s.Name != nil {
				b.global.define(b.newDef(s.Name.Value, "var", s.Name.Token, ""))
			}
		case *ast.ConstBlock:
			for _, c := range s.Constants {
				if c != nil && c.Name != nil {
					b.global.define(b.newDef(c.Name.Value, "const", c.Name.Token, ""))
				}
			}
		case *ast.VarBlock:
			for _, v := range s.Variables {
				if v != nil && v.Name != nil {
					b.global.define(b.newDef(v.Name.Value, "var", v.Name.Token, ""))
				}
			}
		case *ast.ImplDecl:
			if s == nil || s.TypeName == nil {
				continue
			}
			typeName := s.TypeName.Value
			for _, m := range s.Methods {
				if m == nil || m.Name == nil {
					continue
				}
				b.defineStructMethod(typeName, m.Name.Value, m.Name.Token)
			}
		}
	}
}

func (b *refBuilder) walkType(t ast.TypeExpression, sc *scope) {
	if t == nil {
		return
	}
	switch tt := t.(type) {
	case *ast.SimpleType:
		if tt != nil && !token.IsType(tt.Token.Type) && tt.Name != "" {
			b.resolveTypeRef(tt.Name, tt.Token)
		}
	case *ast.GenericType:
		if tt != nil {
			if !token.IsType(tt.Token.Type) && tt.Name != "" {
				b.resolveTypeRef(tt.Name, tt.Token)
			}
			for _, param := range tt.TypeParams {
				b.walkType(param, sc)
			}
		}
	case *ast.BorrowType:
		if tt != nil {
			b.walkType(tt.Inner, sc)
		}
	case *ast.TupleType:
		if tt != nil {
			for _, el := range tt.Elements {
				b.walkType(el, sc)
			}
		}
	case *ast.FunctionType:
		if tt != nil {
			for _, p := range tt.Params {
				b.walkType(p, sc)
			}
			b.walkType(tt.ReturnType, sc)
		}
	case *ast.NamedType:
		if tt != nil {
			b.walkType(tt.Type, sc)
		}
	}
}

func (b *refBuilder) walkExpr(expr ast.Expression, sc *scope) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		if def := sc.lookup(e.Value); def != nil {
			b.recordRef(def, e, e.Token)
		}
	case *ast.MutableIdentifier:
		// Mutable identifiers don't store the name token; skip precise ref tracking.
	case *ast.FieldAccessExpression:
		if e != nil {
			b.walkExpr(e.Object, sc)
			if objIdent, ok := e.Object.(*ast.Identifier); ok {
				if def := b.resolveModuleSymbol(objIdent.Value, e.Field.Value); def != nil {
					b.recordRef(def, e.Field, e.Field.Token)
					return
				}
			}
			if b.tc != nil {
				if t := b.tc.GetNodeType(e.Object); t != "" {
					base := baseTypeName(t)
					if def := b.resolveStructMember(base, e.Field.Value, "field"); def != nil {
						b.recordRef(def, e.Field, e.Field.Token)
						return
					}
				}
			}
		}
	case *ast.MethodCallExpression:
		if e != nil {
			b.walkExpr(e.Object, sc)
			for _, arg := range e.Arguments {
				b.walkExpr(arg, sc)
			}
			if objIdent, ok := e.Object.(*ast.Identifier); ok {
				if def := b.resolveModuleSymbol(objIdent.Value, e.Method.Value); def != nil {
					b.recordRef(def, e.Method, e.Method.Token)
					return
				}
			}
			if b.tc != nil {
				if t := b.tc.GetNodeType(e.Object); t != "" {
					base := baseTypeName(t)
					if def := b.resolveStructMember(base, e.Method.Value, "method"); def != nil {
						b.recordRef(def, e.Method, e.Method.Token)
						return
					}
				}
			}
		}
	case *ast.CallExpression:
		if e != nil {
			b.walkExpr(e.Function, sc)
			for _, arg := range e.Arguments {
				b.walkExpr(arg, sc)
			}
		}
	case *ast.IndexExpression:
		if e != nil {
			b.walkExpr(e.Left, sc)
			b.walkExpr(e.Index, sc)
		}
	case *ast.InfixExpression:
		if e != nil {
			b.walkExpr(e.Left, sc)
			b.walkExpr(e.Right, sc)
		}
	case *ast.PrefixExpression:
		if e != nil {
			b.walkExpr(e.Right, sc)
		}
	case *ast.StructLiteral:
		if e != nil {
			if e.Name != nil {
				b.resolveTypeRef(e.Name.Value, e.Name.Token)
			}
			for _, val := range e.Fields {
				b.walkExpr(val, sc)
			}
		}
	case *ast.VecLiteral:
		if e != nil {
			for _, el := range e.Elements {
				b.walkExpr(el, sc)
			}
		}
	case *ast.RangeExpression:
		if e != nil {
			b.walkExpr(e.Start, sc)
			b.walkExpr(e.End, sc)
		}
	case *ast.TupleExpression:
		if e != nil {
			for _, el := range e.Elements {
				b.walkExpr(el, sc)
			}
		}
	case *ast.FunctionLiteral:
		if e != nil {
			fnScope := newScope(sc)
			for _, p := range e.Parameters {
				if p != nil && p.Name != nil {
					fnScope.define(b.newDef(p.Name.Value, "param", p.Name.Token, ""))
					b.walkType(p.Type, sc)
				}
			}
			b.walkStmt(e.Body, fnScope)
		}
	case *ast.TypeConversion:
		if e != nil {
			b.walkExpr(e.Value, sc)
		}
	case *ast.EnumVariantExpression:
		if e != nil {
			for _, v := range e.Values {
				b.walkExpr(v, sc)
			}
		}
	case *ast.BorrowExpression:
		if e != nil {
			b.walkExpr(e.Value, sc)
		}
	case *ast.DerefExpression:
		if e != nil {
			b.walkExpr(e.Value, sc)
		}
	}
}

func (b *refBuilder) walkStmt(stmt ast.Statement, sc *scope) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.VarStatement:
		if s != nil {
			b.walkExpr(s.Value, sc)
			b.walkType(s.Type, sc)
			if s.Name != nil {
				sc.define(b.newDef(s.Name.Value, "var", s.Name.Token, ""))
			}
		}
	case *ast.ConstStatement:
		if s != nil {
			b.walkExpr(s.Value, sc)
			b.walkType(s.Type, sc)
			if s.Name != nil {
				sc.define(b.newDef(s.Name.Value, "const", s.Name.Token, ""))
			}
		}
	case *ast.MultiVarStatement:
		if s != nil {
			b.walkExpr(s.Value, sc)
			for _, n := range s.Names {
				if n != nil {
					sc.define(b.newDef(n.Value, "var", n.Token, ""))
				}
			}
			for _, t := range s.Types {
				b.walkType(t, sc)
			}
		}
	case *ast.VarBlock:
		if s != nil {
			for _, v := range s.Variables {
				b.walkStmt(v, sc)
			}
		}
	case *ast.ConstBlock:
		if s != nil {
			for _, c := range s.Constants {
				b.walkStmt(c, sc)
			}
		}
	case *ast.AssignmentStatement:
		if s != nil {
			b.walkExpr(s.Left, sc)
			b.walkExpr(s.Value, sc)
		}
	case *ast.ExpressionStatement:
		if s != nil {
			b.walkExpr(s.Expression, sc)
		}
	case *ast.ReturnStatement:
		if s != nil {
			b.walkExpr(s.ReturnValue, sc)
		}
	case *ast.BlockStatement:
		if s != nil {
			blockScope := newScope(sc)
			for _, st := range s.Statements {
				b.walkStmt(st, blockScope)
			}
		}
	case *ast.FunctionDecl:
		if s != nil && s.Body != nil {
			fnScope := newScope(sc)
			for _, p := range s.Parameters {
				if p != nil && p.Name != nil {
					fnScope.define(b.newDef(p.Name.Value, "param", p.Name.Token, ""))
					b.walkType(p.Type, sc)
				}
			}
			b.walkType(s.ReturnType, sc)
			b.walkStmt(s.Body, fnScope)
		}
	case *ast.WhileStatement:
		if s != nil {
			b.walkExpr(s.Condition, sc)
			b.walkStmt(s.Body, newScope(sc))
		}
	case *ast.ForStatement:
		if s != nil {
			b.walkExpr(s.Iterable, sc)
			loopScope := newScope(sc)
			if s.Variable != nil {
				loopScope.define(b.newDef(s.Variable.Value, "var", s.Variable.Token, ""))
			}
			b.walkStmt(s.Body, loopScope)
		}
	case *ast.IfStatement:
		if s != nil {
			b.walkExpr(s.Condition, sc)
			b.walkStmt(s.Consequence, newScope(sc))
			if s.Alternative != nil {
				b.walkStmt(s.Alternative, newScope(sc))
			}
		}
	case *ast.SwitchStatement:
		if s != nil {
			b.walkExpr(s.Value, sc)
			for _, c := range s.Cases {
				if c == nil {
					continue
				}
				caseScope := newScope(sc)
				for _, v := range c.Values {
					b.walkExpr(v, caseScope)
				}
				b.walkStmt(c.Body, caseScope)
			}
		}
	case *ast.DeferStatement:
		if s != nil {
			b.walkStmt(s.Body, newScope(sc))
		}
	case *ast.ImplDecl:
		if s != nil {
			for _, m := range s.Methods {
				if m == nil {
					continue
				}
				methodScope := newScope(sc)
				if s.Receiver != nil {
					methodScope.define(b.newDef(s.Receiver.Value, "var", s.Receiver.Token, ""))
				}
				for _, p := range m.Parameters {
					if p != nil && p.Name != nil {
						methodScope.define(b.newDef(p.Name.Value, "param", p.Name.Token, ""))
						b.walkType(p.Type, sc)
					}
				}
				b.walkType(m.ReturnType, sc)
				b.walkStmt(m.Body, methodScope)
			}
		}
	case *ast.PanicStatement:
		if s != nil {
			b.walkExpr(s.Message, sc)
		}
	case *ast.UnsafeBlock:
		if s != nil {
			b.walkStmt(s.Body, newScope(sc))
		}
	}
}

func buildReferenceIndex(
	prog *ast.Program,
	tc *typechecker.TypeChecker,
	uri string,
	imports map[string]string,
	_ *FileIndex,
	srv *Server,
) (
	map[string][]Location,
	map[string]string,
	map[string]Location,
) {
	b := &refBuilder{
		uri:           uri,
		imports:       imports,
		srv:           srv,
		tc:            tc,
		refs:          make(map[string][]Location),
		refByPos:      make(map[string]string),
		defs:          make(map[string]Location),
		structFields:  make(map[string]map[string]*symbolDef),
		structMethods: make(map[string]map[string]*symbolDef),
		typeDefs:      make(map[string]*symbolDef),
		global:        newScope(nil),
	}

	if prog == nil {
		return b.refs, b.refByPos, b.defs
	}

	if srv != nil {
		srv.ensureWorkspaceIndexes()
	}

	b.indexGlobals(prog)

	fileScope := newScope(b.global)
	for _, stmt := range prog.Statements {
		b.walkStmt(stmt, fileScope)
	}

	return b.refs, b.refByPos, b.defs
}

func collectQualifiedMethodRefs(prog *ast.Program, uri string, qualifier string, name string) []Location {
	var locs []Location
	ast.Walk(prog, func(node ast.Node) {
		n, ok := node.(*ast.MethodCallExpression)
		if !ok {
			return
		}
		ident, ok := n.Object.(*ast.Identifier)
		if !ok || ident.Value != qualifier {
			return
		}
		if n.Method != nil && n.Method.Value == name {
			locs = append(locs, locationFromToken(uri, n.Method.Token, n.Method.Value))
		}
	})
	return locs
}

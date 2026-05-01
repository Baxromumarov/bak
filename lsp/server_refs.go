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
	startLine := tok.Line - 1
	startCol := tok.Column - 1

	out.Start.Line = startLine
	out.Start.Character = startCol
	out.End.Line = startLine
	out.End.Character = startCol + length

	return out
}

func rangeFromSpan(span ast.Span) (Range, bool) {
	if span.Start.Line == 0 ||
		span.End.Line == 0 {
		return Range{}, false
	}

	return Range{
		Start: Position{
			Line:      span.Start.Line - 1,
			Character: span.Start.Column - 1,
		},
		End: Position{
			Line:      span.End.Line - 1,
			Character: span.End.Column - 1,
		},
	}, true
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

func symbolDefFromInfo(
	sym SymbolInfo,
	name,
	container string,
) *symbolDef {
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

func (s *Server) ensureWorkspaceRefIndex() {
	if s.RootPath == "" {
		return
	}
	_ = filepath.WalkDir(s.RootPath, func(path string, d fs.DirEntry, err error) error {
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
		uri := pathToURI(path)
		res := s.Cache[uri]
		if res != nil && res.RefIndex != nil && res.Defs != nil {
			return nil
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
				return nil
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
		s.Indexes[uri] = idx
		refIndex, refByPos, defs := buildReferenceIndex(prog, tc, uri, imports, idx, s)
		if res == nil {
			res = &AnalysisResult{}
			s.Cache[uri] = res
		}
		res.AST = prog
		res.TC = tc
		res.Index = idx
		res.Imports = imports
		res.RefIndex = refIndex
		res.RefByPos = refByPos
		res.Defs = defs
		return nil
	})
}

func formatFuncDetail(
	params []*ast.Parameter,
	ret ast.TypeExpression,
	mutable bool,
) string {
	paramLabels := make([]string, 0, len(params))
	for i, p := range params {
		if p == nil {
			continue
		}
		paramName := strfmt.Named("arg{expr}", "Expr", i+1)
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
		"Mut", mut,
		"ParamLabels", strings.Join(paramLabels, ", "),
		"RetType", retType,
	)
}

func (s *Server) ensureWorkspaceIndexes() {
	if s.RootPath == "" {
		return
	}
	_ = filepath.WalkDir(s.RootPath, func(path string, d fs.DirEntry, err error) error {
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
		if strings.HasSuffix(path, ".bak") {
			uri := pathToURI(path)
			if _, ok := s.Indexes[uri]; !ok {
				if idx := s.getOrIndexFile(path); idx != nil {
					s.Indexes[uri] = idx
				}
			}
		}
		return nil
	})
}

func collectReferences(
	prog *ast.Program,
	uri string,
	name string,
) []Location {
	locs := []Location{}
	add := func(line, col, length int) {
		if line <= 0 || col <= 0 {
			return
		}
		locs = append(locs, Location{
			URI: uri,
			Range: Range{
				Start: Position{
					Line:      line - 1,
					Character: col - 1,
				},
				End: Position{
					Line:      line - 1,
					Character: col - 1 + length,
				},
			},
		})
	}
	var walk func(node ast.Node)
	walk = func(node ast.Node) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case *ast.Identifier:
			if n.Value == name {
				add(n.Token.Line, n.Token.Column, len(n.Value))
			}
		case *ast.MethodCallExpression:
			if n.Method != nil && n.Method.Value == name {
				add(
					n.Method.Token.Line,
					n.Method.Token.Column,
					len(n.Method.Value),
				)
			}
			walk(n.Object)
			for _, arg := range n.Arguments {
				walk(arg)
			}
		case *ast.Program:
			for _, stmt := range n.Statements {
				walk(stmt)
			}
		case *ast.ExpressionStatement:
			walk(n.Expression)
		case *ast.VarStatement:
			walk(n.Name)
			walk(n.Value)
		case *ast.ConstStatement:
			walk(n.Name)
			walk(n.Value)
		case *ast.ReturnStatement:
			walk(n.ReturnValue)
		case *ast.InfixExpression:
			walk(n.Left)
			walk(n.Right)
		case *ast.PrefixExpression:
			walk(n.Right)
		case *ast.CallExpression:
			walk(n.Function)
			for _, arg := range n.Arguments {
				walk(arg)
			}
		case *ast.BlockStatement:
			for _, stmt := range n.Statements {
				walk(stmt)
			}
		case *ast.IfStatement:
			walk(n.Condition)
			walk(n.Consequence)
			walk(n.Alternative)
		case *ast.WhileStatement:
			walk(n.Condition)
			walk(n.Body)
		case *ast.ForStatement:
			walk(n.Iterable)
			walk(n.Body)
		case *ast.SwitchStatement:
			walk(n.Value)
			for _, c := range n.Cases {
				for _, expr := range c.Values {
					walk(expr)
				}
				if c.Body != nil {
					walk(c.Body)
				}
			}
		case *ast.FunctionDecl:
			if n == nil || n.Body == nil {
				return
			}
			walk(n.Body)
		case *ast.ImplDecl:
			if n == nil {
				return
			}
			walk(n.TypeName)
			for _, m := range n.Methods {
				if m == nil || m.Body == nil {
					continue
				}
				walk(m.Body)
			}
		case *ast.StructDecl:
			if n == nil {
				return
			}
			walk(n.Name)
			for _, f := range n.Fields {
				walk(f.Type)
			}
		case *ast.EnumDecl:
			if n == nil {
				return
			}
			walk(n.Name)
		case *ast.TypeDecl:
			if n == nil {
				return
			}
			walk(n.Name)
			walk(n.Underlying)
		case *ast.AliasDecl:
			if n == nil {
				return
			}
			walk(n.Name)
			walk(n.Underlying)
		}
	}
	walk(prog)
	return locs
}

func collectReferencesWorkspace(s *Server, uri string, node ast.Node) []Location {
	name := ""
	modulePath := ""
	localOnly := false

	switch n := node.(type) {
	case *ast.MethodCallExpression:
		if n.Method == nil {
			return nil
		}
		name = n.Method.Value
		if ident, ok := n.Object.(*ast.Identifier); ok {
			if res, ok := s.Cache[uri]; ok && res.Imports != nil {
				if importPath, ok := res.Imports[ident.Value]; ok {
					modulePath = s.resolveImportPath(uriToPath(uri), importPath)
				}
			}
		}
	case *ast.Identifier:
		name = n.Value
		if res, ok := s.Cache[uri]; ok && res.Index != nil {
			if sym, ok := res.Index.Symbols[name]; ok {
				if sym.Location.URI == uri && sym.Location.Range.Start.Line == n.Token.Line-1 {
					modulePath = uriToPath(uri)
				}
			}
		}
		if modulePath == "" {
			localOnly = true
		}
	default:
		return nil
	}

	if name == "" {
		return nil
	}

	if localOnly {
		if res, ok := s.Cache[uri]; ok && res.AST != nil {
			return collectReferences(res.AST, uri, name)
		}
		return nil
	}

	refs := []Location{}
	for fileURI, res := range s.Cache {
		if res == nil || res.AST == nil {
			continue
		}
		if uriToPath(fileURI) == modulePath {
			refs = append(refs, collectReferences(res.AST, fileURI, name)...)
			continue
		}
		if modulePath == "" || res.Imports == nil {
			continue
		}
		for alias, importPath := range res.Imports {
			resolved := s.resolveImportPath(uriToPath(fileURI), importPath)
			if resolved != modulePath {
				continue
			}
			refs = append(refs, collectQualifiedMethodRefs(res.AST, fileURI, alias, name)...)
		}
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

func buildReferenceIndex(
	prog *ast.Program,
	tc *typechecker.TypeChecker,
	uri string,
	idxImports map[string]string,
	_ *FileIndex,
	srv *Server,
) (
	map[string][]Location,
	map[string]string,
	map[string]Location,
) {
	refs := make(map[string][]Location)
	refByPos := make(map[string]string)
	defs := make(map[string]Location)

	if prog == nil {
		return refs, refByPos, defs
	}

	if srv != nil {
		srv.ensureWorkspaceIndexes()
	}

	structFields := make(map[string]map[string]*symbolDef)
	structMethods := make(map[string]map[string]*symbolDef)
	typeDefs := make(map[string]*symbolDef)

	global := newScope(nil)

	newDef := func(name, kind string, tok token.Token, container string) *symbolDef {
		loc := locationFromToken(uri, tok, name)
		def := &symbolDef{
			ID:        symbolID(kind, loc),
			Name:      name,
			Kind:      kind,
			Location:  loc,
			Container: container,
		}
		defs[def.ID] = def.Location
		refByPos[tokenKey(tok)] = def.ID
		return def
	}

	defineStructField := func(structName, fieldName string, tok token.Token) {
		if structFields[structName] == nil {
			structFields[structName] = make(map[string]*symbolDef)
		}
		structFields[structName][fieldName] = newDef(
			fieldName,
			"field",
			tok,
			structName,
		)
	}

	defineStructMethod := func(structName, methodName string, tok token.Token) {
		if structMethods[structName] == nil {
			structMethods[structName] = make(map[string]*symbolDef)
		}
		structMethods[structName][methodName] = newDef(
			methodName,
			"method",
			tok,
			structName,
			)
	}

	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			if s != nil && s.Name != nil {
				global.define(newDef(s.Name.Value, "func", s.Name.Token, ""))
			}
		case *ast.StructDecl:
			if s != nil && s.Name != nil {
				def := newDef(s.Name.Value, "struct", s.Name.Token, "")
				global.define(def)
				typeDefs[s.Name.Value] = def
				for _, f := range s.Fields {
					if f == nil || f.Name == nil {
						continue
					}
					defineStructField(s.Name.Value, f.Name.Value, f.Name.Token)
				}
			}
		case *ast.EnumDecl:
			if s != nil && s.Name != nil {
				def := newDef(s.Name.Value, "enum", s.Name.Token, "")
				global.define(def)
				typeDefs[s.Name.Value] = def
			}
		case *ast.TypeDecl:
			if s != nil && s.Name != nil {
				def := newDef(s.Name.Value, "type", s.Name.Token, "")
				global.define(def)
				typeDefs[s.Name.Value] = def
			}
		case *ast.AliasDecl:
			if s != nil && s.Name != nil {
				def := newDef(s.Name.Value, "alias", s.Name.Token, "")
				global.define(def)
				typeDefs[s.Name.Value] = def
			}
		case *ast.ConstStatement:
			if s != nil && s.Name != nil {
				global.define(newDef(s.Name.Value, "const", s.Name.Token, ""))
			}
		case *ast.VarStatement:
			if s != nil && s.Name != nil {
				global.define(newDef(s.Name.Value, "var", s.Name.Token, ""))
			}
		case *ast.ConstBlock:
			for _, c := range s.Constants {
				if c != nil && c.Name != nil {
					global.define(newDef(c.Name.Value, "const", c.Name.Token, ""))
				}
			}
		case *ast.VarBlock:
			for _, v := range s.Variables {
				if v != nil && v.Name != nil {
					global.define(newDef(v.Name.Value, "var", v.Name.Token, ""))
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
				defineStructMethod(typeName, m.Name.Value, m.Name.Token)
			}
		}
	}

	recordRef := func(def *symbolDef, node ast.Node, tok token.Token) {
		if def == nil {
			return
		}
		loc := locationFromToken(uri, tok, tok.Literal)
		if node != nil {
			if r, ok := rangeFromSpan(ast.SpanOf(node)); ok {
				loc = Location{URI: uri, Range: r}
			}
		}
		refs[def.ID] = append(refs[def.ID], loc)
		refByPos[tokenKey(tok)] = def.ID
	}

	recordRefTok := func(def *symbolDef, tok token.Token) {
		recordRef(def, nil, tok)
	}

	resolveModuleSymbol := func(alias, name string) *symbolDef {
		if srv == nil {
			return nil
		}
		importPath, ok := idxImports[alias]
		if !ok {
			return nil
		}
		path := srv.resolveImportPath(uriToPath(uri), importPath)
		if path == "" {
			return nil
		}
		modIndex := srv.getOrIndexFile(path)
		if modIndex == nil {
			return nil
		}
		if sym, ok := modIndex.Symbols[name]; ok {
			return symbolDefFromInfo(sym, name, "")
		}
		return nil
	}

	resolveStructMember := func(structName, memberName string, kind string) *symbolDef {
		if structName == "" || memberName == "" {
			return nil
		}
		if kind == "field" {
			if fields := structFields[structName]; fields != nil {
				if def, ok := fields[memberName]; ok {
					return def
				}
			}
		}
		if kind == "method" {
			if methods := structMethods[structName]; methods != nil {
				if def, ok := methods[memberName]; ok {
					return def
				}
			}
		}
		if srv == nil {
			return nil
		}
		key := structName + "." + memberName
		for _, index := range srv.Indexes {
			if index == nil {
				continue
			}
			if sym, ok := index.Symbols[key]; ok {
				return symbolDefFromInfo(sym, memberName, structName)
			}
		}
		return nil
	}

	resolveTypeRef := func(name string, tok token.Token) {
		if def, ok := typeDefs[name]; ok {
			recordRefTok(def, tok)
		} else if def := global.lookup(name); def != nil && (def.Kind == "struct" || def.Kind == "enum" || def.Kind == "type" || def.Kind == "alias") {
			recordRefTok(def, tok)
		}
	}

	var walkExpr func(expr ast.Expression, scope *scope)
	var walkType func(t ast.TypeExpression, scope *scope)
	var walkStmt func(stmt ast.Statement, scope *scope)

	walkType = func(t ast.TypeExpression, scope *scope) {
		switch tt := t.(type) {
		case *ast.SimpleType:
			if tt != nil && !token.IsType(tt.Token.Type) && tt.Name != "" {
				resolveTypeRef(tt.Name, tt.Token)
			}
		case *ast.GenericType:
			if tt != nil {
				if !token.IsType(tt.Token.Type) && tt.Name != "" {
					resolveTypeRef(tt.Name, tt.Token)
				}
				for _, param := range tt.TypeParams {
					walkType(param, scope)
				}
			}
		case *ast.BorrowType:
			if tt != nil {
				walkType(tt.Inner, scope)
			}
		case *ast.TupleType:
			if tt != nil {
				for _, el := range tt.Elements {
					walkType(el, scope)
				}
			}
		case *ast.FunctionType:
			if tt != nil {
				for _, p := range tt.Params {
					walkType(p, scope)
				}
				walkType(tt.ReturnType, scope)
			}
		case *ast.NamedType:
			if tt != nil {
				walkType(tt.Type, scope)
			}
		}
	}

	walkExpr = func(expr ast.Expression, scope *scope) {
		switch e := expr.(type) {
		case *ast.Identifier:
			if def := scope.lookup(e.Value); def != nil {
				recordRef(def, e, e.Token)
			}
		case *ast.MutableIdentifier:
			// Mutable identifiers don't store the name token; skip precise ref tracking.
		case *ast.FieldAccessExpression:
			if e != nil {
				walkExpr(e.Object, scope)
				if objIdent, ok := e.Object.(*ast.Identifier); ok {
					if def := resolveModuleSymbol(objIdent.Value, e.Field.Value); def != nil {
						recordRef(def, e.Field, e.Field.Token)
						return
					}
				}
				if tc != nil {
					if t := tc.GetNodeType(e.Object); t != "" {
						base := baseTypeName(t)
						if def := resolveStructMember(base, e.Field.Value, "field"); def != nil {
							recordRef(def, e.Field, e.Field.Token)
							return
						}
					}
				}
			}
		case *ast.MethodCallExpression:
			if e != nil {
				walkExpr(e.Object, scope)
				for _, arg := range e.Arguments {
					walkExpr(arg, scope)
				}
				if objIdent, ok := e.Object.(*ast.Identifier); ok {
					if def := resolveModuleSymbol(objIdent.Value, e.Method.Value); def != nil {
						recordRef(def, e.Method, e.Method.Token)
						return
					}
				}
				if tc != nil {
					if t := tc.GetNodeType(e.Object); t != "" {
						base := baseTypeName(t)
						if def := resolveStructMember(base, e.Method.Value, "method"); def != nil {
							recordRef(def, e.Method, e.Method.Token)
							return
						}
					}
				}
			}
		case *ast.CallExpression:
			if e != nil {
				walkExpr(e.Function, scope)
				for _, arg := range e.Arguments {
					walkExpr(arg, scope)
				}
			}
		case *ast.IndexExpression:
			if e != nil {
				walkExpr(e.Left, scope)
				walkExpr(e.Index, scope)
			}
		case *ast.InfixExpression:
			if e != nil {
				walkExpr(e.Left, scope)
				walkExpr(e.Right, scope)
			}
		case *ast.PrefixExpression:
			if e != nil {
				walkExpr(e.Right, scope)
			}
		case *ast.StructLiteral:
			if e != nil {
				if e.Name != nil {
					resolveTypeRef(e.Name.Value, e.Name.Token)
				}
				for _, val := range e.Fields {
					walkExpr(val, scope)
				}
			}
		case *ast.VecLiteral:
			if e != nil {
				for _, el := range e.Elements {
					walkExpr(el, scope)
				}
			}
		case *ast.RangeExpression:
			if e != nil {
				walkExpr(e.Start, scope)
				walkExpr(e.End, scope)
			}
		case *ast.TupleExpression:
			if e != nil {
				for _, el := range e.Elements {
					walkExpr(el, scope)
				}
			}
		case *ast.FunctionLiteral:
			if e != nil {
				fnScope := newScope(scope)
				for _, p := range e.Parameters {
					if p != nil && p.Name != nil {
						fnScope.define(newDef(p.Name.Value, "param", p.Name.Token, ""))
						walkType(p.Type, scope)
					}
				}
				walkStmt(e.Body, fnScope)
			}
		case *ast.TypeConversion:
			if e != nil {
				walkExpr(e.Value, scope)
			}
		case *ast.EnumVariantExpression:
			if e != nil {
				for _, v := range e.Values {
					walkExpr(v, scope)
				}
			}
		case *ast.BorrowExpression:
			if e != nil {
				walkExpr(e.Value, scope)
			}
		case *ast.DerefExpression:
			if e != nil {
				walkExpr(e.Value, scope)
			}
		}
	}

	walkStmt = func(stmt ast.Statement, scope *scope) {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			if s != nil {
				walkExpr(s.Value, scope)
				walkType(s.Type, scope)
				if s.Name != nil {
					scope.define(newDef(s.Name.Value, "var", s.Name.Token, ""))
				}
			}
		case *ast.ConstStatement:
			if s != nil {
				walkExpr(s.Value, scope)
				walkType(s.Type, scope)
				if s.Name != nil {
					scope.define(newDef(s.Name.Value, "const", s.Name.Token, ""))
				}
			}
		case *ast.MultiVarStatement:
			if s != nil {
				walkExpr(s.Value, scope)
				for _, n := range s.Names {
					if n != nil {
						scope.define(newDef(n.Value, "var", n.Token, ""))
					}
				}
				for _, t := range s.Types {
					walkType(t, scope)
				}
			}
		case *ast.VarBlock:
			if s != nil {
				for _, v := range s.Variables {
					walkStmt(v, scope)
				}
			}
		case *ast.ConstBlock:
			if s != nil {
				for _, c := range s.Constants {
					walkStmt(c, scope)
				}
			}
		case *ast.AssignmentStatement:
			if s != nil {
				walkExpr(s.Left, scope)
				walkExpr(s.Value, scope)
			}
		case *ast.ExpressionStatement:
			if s != nil {
				walkExpr(s.Expression, scope)
			}
		case *ast.ReturnStatement:
			if s != nil {
				walkExpr(s.ReturnValue, scope)
			}
		case *ast.BlockStatement:
			if s != nil {
				blockScope := newScope(scope)
				for _, st := range s.Statements {
					walkStmt(st, blockScope)
				}
			}
		case *ast.FunctionDecl:
			if s != nil && s.Body != nil {
				fnScope := newScope(scope)
				for _, p := range s.Parameters {
					if p != nil && p.Name != nil {
						fnScope.define(newDef(p.Name.Value, "param", p.Name.Token, ""))
						walkType(p.Type, scope)
					}
				}
				walkType(s.ReturnType, scope)
				walkStmt(s.Body, fnScope)
			}
		case *ast.WhileStatement:
			if s != nil {
				walkExpr(s.Condition, scope)
				walkStmt(s.Body, newScope(scope))
			}
		case *ast.ForStatement:
			if s != nil {
				walkExpr(s.Iterable, scope)
				loopScope := newScope(scope)
				if s.Variable != nil {
					loopScope.define(newDef(s.Variable.Value, "var", s.Variable.Token, ""))
				}
				walkStmt(s.Body, loopScope)
			}
		case *ast.IfStatement:
			if s != nil {
				walkExpr(s.Condition, scope)
				walkStmt(s.Consequence, newScope(scope))
				if s.Alternative != nil {
					walkStmt(s.Alternative, newScope(scope))
				}
			}
		case *ast.SwitchStatement:
			if s != nil {
				walkExpr(s.Value, scope)
				for _, c := range s.Cases {
					if c == nil {
						continue
					}
					caseScope := newScope(scope)
					for _, v := range c.Values {
						walkExpr(v, caseScope)
					}
					walkStmt(c.Body, caseScope)
				}
			}
		case *ast.DeferStatement:
			if s != nil {
				walkStmt(s.Body, newScope(scope))
			}
		case *ast.ImplDecl:
			if s != nil {
				for _, m := range s.Methods {
					if m == nil {
						continue
					}
					methodScope := newScope(scope)
					if s.Receiver != nil {
						methodScope.define(newDef(s.Receiver.Value, "var", s.Receiver.Token, ""))
					}
					for _, p := range m.Parameters {
						if p != nil && p.Name != nil {
							methodScope.define(newDef(p.Name.Value, "param", p.Name.Token, ""))
							walkType(p.Type, scope)
						}
					}
					walkType(m.ReturnType, scope)
					walkStmt(m.Body, methodScope)
				}
			}
		case *ast.PanicStatement:
			if s != nil {
				walkExpr(s.Message, scope)
			}
		case *ast.UnsafeBlock:
			if s != nil {
				walkStmt(s.Body, newScope(scope))
			}
		}
	}

	fileScope := newScope(global)
	for _, stmt := range prog.Statements {
		walkStmt(stmt, fileScope)
	}

	return refs, refByPos, defs
}

func collectQualifiedMethodRefs(prog *ast.Program, uri string, qualifier string, name string) []Location {
	locs := []Location{}
	add := func(line, col, length int) {
		if line <= 0 || col <= 0 {
			return
		}
		locs = append(locs, Location{
			URI: uri,
			Range: Range{
				Start: Position{Line: line - 1, Character: col - 1},
				End:   Position{Line: line - 1, Character: col - 1 + length},
			},
		})
	}
	var walk func(node ast.Node)
	walk = func(node ast.Node) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case *ast.MethodCallExpression:
			if ident, ok := n.Object.(*ast.Identifier); ok && ident.Value == qualifier {
				if n.Method != nil && n.Method.Value == name {
					add(n.Method.Token.Line, n.Method.Token.Column, len(n.Method.Value))
				}
			}
			walk(n.Object)
			for _, arg := range n.Arguments {
				walk(arg)
			}
		case *ast.Program:
			for _, stmt := range n.Statements {
				walk(stmt)
			}
		case *ast.ExpressionStatement:
			walk(n.Expression)
		case *ast.VarStatement:
			walk(n.Name)
			walk(n.Value)
		case *ast.ConstStatement:
			walk(n.Name)
			walk(n.Value)
		case *ast.ReturnStatement:
			walk(n.ReturnValue)
		case *ast.InfixExpression:
			walk(n.Left)
			walk(n.Right)
		case *ast.PrefixExpression:
			walk(n.Right)
		case *ast.CallExpression:
			walk(n.Function)
			for _, arg := range n.Arguments {
				walk(arg)
			}
		case *ast.BlockStatement:
			for _, stmt := range n.Statements {
				walk(stmt)
			}
		case *ast.IfStatement:
			walk(n.Condition)
			walk(n.Consequence)
			walk(n.Alternative)
		case *ast.WhileStatement:
			walk(n.Condition)
			walk(n.Body)
		case *ast.ForStatement:
			walk(n.Iterable)
			walk(n.Body)
		case *ast.SwitchStatement:
			walk(n.Value)
			for _, c := range n.Cases {
				for _, expr := range c.Values {
					walk(expr)
				}
				walk(c.Body)
			}
		case *ast.FunctionDecl:
			walk(n.Body)
		case *ast.ImplDecl:
			for _, m := range n.Methods {
				if m != nil {
					walk(m.Body)
				}
			}
		}
	}
	walk(prog)
	return locs
}

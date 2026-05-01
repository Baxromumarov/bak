package main

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/formatter"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/token"
)

func pathToURI(path string) string {
	if strings.HasPrefix(path, "file://") {
		return path
	}
	return "file://" + path
}

func (s *Server) resolveImportPath(baseFile, importPath string) string {
	return packages.ResolveImportPathFrom(importPath, baseFile)
}

func (s *Server) getStdPackages() []string {
	if s.stdPackages != nil {
		return s.stdPackages
	}
	root := s.RootPath
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil
		}
		root = cwd
	}
	stdPath := filepath.Join(root, "src", "std")
	entries, err := os.ReadDir(stdPath)
	if err != nil {
		return nil
	}
	pkgs := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			pkgs = append(pkgs, name)
			continue
		}
		if before, ok := strings.CutSuffix(name, ".bak"); ok {
			pkgs = append(pkgs, before)
		}
	}
	sort.Strings(pkgs)
	s.stdPackages = pkgs
	return pkgs
}

func (s *Server) getStdImportPaths() []string {
	if s.stdImportPaths != nil {
		return s.stdImportPaths
	}
	root := s.RootPath
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil
		}
		root = cwd
	}
	stdPath := filepath.Join(root, "src", "std")
	paths := []string{}
	_ = filepath.WalkDir(stdPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".bak") ||
			strings.HasSuffix(d.Name(), "_test.bak") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		rel = strings.TrimPrefix(rel, "src/")
		rel = strings.TrimSuffix(rel, ".bak")
		paths = append(paths, rel)
		return nil
	})
	sort.Strings(paths)
	s.stdImportPaths = paths
	return paths
}

func (s *Server) packageDoc(path string) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	l := lexer.New(string(data))
	p := parser.New(l)
	prog := p.ParseProgram()
	if prog == nil {
		return "", ""
	}
	comments := formatter.ScanComments(string(data))
	lineComment := make(map[int]string)
	for _, c := range comments {
		if after, ok := strings.CutPrefix(c.Text, "//"); ok {
			lineComment[c.Line] = strings.TrimSpace(after)
		}
	}
	for _, stmt := range prog.Statements {
		if ps, ok := stmt.(*ast.PackageStatement); ok {
			pkgName := ""
			if ps.Name != nil {
				pkgName = ps.Name.Value
			}
			return pkgName, collectDoc(lineComment, ps.Token.Line)
		}
	}
	return "", ""
}

func (s *Server) getOrIndexFile(path string) *FileIndex {
	// Handle directory-based packages (e.g. std/http with multiple .bak files)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return s.getOrIndexDir(path)
	}
	uri := pathToURI(path)
	// Use PublicIndexes for external module lookups (public symbols only)
	if idx, ok := s.PublicIndexes[uri]; ok {
		return idx
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	l := lexer.New(string(data))
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return nil
	}
	comments := formatter.ScanComments(string(data))
	idx := indexProgram(prog, uri, comments, false)
	s.PublicIndexes[uri] = idx
	return idx
}

func (s *Server) getOrIndexDir(dirPath string) *FileIndex {
	uri := pathToURI(dirPath)
	if idx, ok := s.PublicIndexes[uri]; ok {
		return idx
	}
	merged := &FileIndex{
		Symbols: make(map[string]SymbolInfo),
		Docs:    make(map[string]string),
		Sigs:    make(map[string]SignatureInfo),
		Structs: make(map[string]StructInfo),
		Consts:  make(map[string]ConstInfo),
		Types:   make(map[string]TypeDeclInfo),
		Aliases: make(map[string]AliasInfo),
		Enums:   make(map[string]EnumInfo),
		Vars:    make(map[string]VarInfo),
	}
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bak") {
			continue
		}
		filePath := filepath.Join(dirPath, entry.Name())
		fileURI := pathToURI(filePath)
		if idx, ok := s.PublicIndexes[fileURI]; ok {
			mergeFileIndex(merged, idx)
			continue
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		l := lexer.New(string(data))
		p := parser.New(l)
		prog := p.ParseProgram()
		if len(p.Errors()) > 0 {
			continue
		}
		comments := formatter.ScanComments(string(data))
		idx := indexProgram(prog, fileURI, comments, false)
		s.PublicIndexes[fileURI] = idx
		mergeFileIndex(merged, idx)
	}
	s.PublicIndexes[uri] = merged
	return merged
}

func mergeFileIndex(dst, src *FileIndex) {
	if src == nil {
		return
	}
	maps.Copy(dst.Symbols, src.Symbols)
	maps.Copy(dst.Docs, src.Docs)
	maps.Copy(dst.Sigs, src.Sigs)
	maps.Copy(dst.Structs, src.Structs)
	if src.Consts != nil {
		maps.Copy(dst.Consts, src.Consts)
	}
	if src.Types != nil {
		maps.Copy(dst.Types, src.Types)
	}
	if src.Aliases != nil {
		maps.Copy(dst.Aliases, src.Aliases)
	}
	if src.Enums != nil {
		maps.Copy(dst.Enums, src.Enums)
	}
	if src.Vars != nil {
		maps.Copy(dst.Vars, src.Vars)
	}
}

func collectImports(prog *ast.Program) map[string]string {
	out := make(map[string]string)
	if prog == nil {
		return out
	}
	for _, stmt := range prog.Statements {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			if s == nil {
				continue
			}
			alias := s.Alias
			if alias == "" {
				parts := strings.Split(s.Path, "/")
				if len(parts) == 0 {
					continue
				}
				alias = parts[len(parts)-1]
				if before, ok := strings.CutSuffix(alias, ".bak"); ok {
					alias = before
				}
			}
			out[alias] = s.Path
		case *ast.ImportBlock:
			if s == nil {
				continue
			}
			for _, imp := range s.Imports {
				if imp == nil {
					continue
				}
				alias := imp.Alias
				if alias == "" {
					parts := strings.Split(imp.Path, "/")
					if len(parts) == 0 {
						continue
					}
					alias = parts[len(parts)-1]
					alias = strings.TrimSuffix(alias, ".bak")
				}
				out[alias] = imp.Path
			}
		}
	}
	return out
}

func indexProgram(
	prog *ast.Program,
	uri string,
	comments []formatter.Comment,
	includePrivate bool,
) *FileIndex {
	index := &FileIndex{
		Symbols: make(map[string]SymbolInfo),
		Sigs:    make(map[string]SignatureInfo),
		Structs: make(map[string]StructInfo),
		Consts:  make(map[string]ConstInfo),
		Types:   make(map[string]TypeDeclInfo),
		Aliases: make(map[string]AliasInfo),
		Enums:   make(map[string]EnumInfo),
		Vars:    make(map[string]VarInfo),
	}
	if prog == nil {
		index.Docs = make(map[string]string)
		return index
	}
	symbolWriter := newIndexSymbolWriter(index, uri)
	index.Docs = buildDocIndex(prog, comments, includePrivate)
	for _, stmt := range prog.Statements {
		if stmt == nil {
			continue
		}

		switch s := stmt.(type) {

		case *ast.FunctionDecl:
			if s == nil || s.Name == nil {
				continue
			}
			if includePrivate || s.Visibility == ast.Public {
				symbolWriter.addSymbol(
					symbolParamsFromToken(
						s.Name.Value,
						"func",
						s.Name.Token,
						s.Visibility == ast.Public,
					),
				)
				if sig := buildFuncSignature(
					s.Name.Value,
					s.Parameters,
					s.ReturnType,
					index.Docs[s.Name.Value],
				); sig.Label != "" {
					index.Sigs[s.Name.Value] = sig
				}
			}
		case *ast.StructDecl:
			if s == nil || s.Name == nil {
				continue
			}
			if includePrivate || s.Visibility == ast.Public {
				symbolWriter.addSymbol(symbolParamsFromToken(
					s.Name.Value,
					"struct",
					s.Name.Token,
					s.Visibility == ast.Public,
				))

				fields := make([]string, 0, len(s.Fields))
				for _, f := range s.Fields {
					if f == nil || f.Name == nil || f.Type == nil {
						continue
					}
					// Only add public fields to the display list when not including private
					if includePrivate || f.Visibility == ast.Public {
						fields = append(fields, strfmt.Named(
							"{Value}: {string}",
							"Value", f.Name.Value,
							"string", f.Type.String(),
						))
						fieldName := s.Name.Value + "." + f.Name.Value
						symbolWriter.addSymbol(
							symbolParamsFromTokenWithLength(
								fieldName,
								"field",
								f.Name.Token,
								len(f.Name.Value),
								f.Visibility == ast.Public,
							),
						)
					}
				}
				index.Structs[s.Name.Value] = StructInfo{
					Name:   s.Name.Value,
					Fields: fields,
					Doc:    index.Docs[s.Name.Value],
				}
			}
		case *ast.EnumDecl:
			if s == nil || s.Name == nil {
				continue
			}

			if includePrivate || s.Visibility == ast.Public {

				symbolWriter.addSymbol(
					symbolParamsFromToken(
						s.Name.Value,
						"enum",
						s.Name.Token,
						s.Visibility == ast.Public,
					),
				)

				variants := make([]string, 0, len(s.Variants))
				for _, v := range s.Variants {
					if v != nil && v.Name != nil {
						variants = append(variants, v.Name.Value)
					}
				}
				index.Enums[s.Name.Value] = EnumInfo{
					Name:     s.Name.Value,
					Variants: variants,
					Doc:      index.Docs[s.Name.Value],
				}
			}
		case *ast.TypeDecl:
			if s == nil || s.Name == nil {
				continue
			}
			if includePrivate || s.Visibility == ast.Public {

				symbolWriter.addSymbol(
					symbolParamsFromToken(
						s.Name.Value,
						"type",
						s.Name.Token,
						s.Visibility == ast.Public,
					),
				)

				underlying := ""
				if s.Underlying != nil {
					underlying = s.Underlying.String()
				}

				index.Types[s.Name.Value] = TypeDeclInfo{
					Name:       s.Name.Value,
					Underlying: underlying,
					Doc:        index.Docs[s.Name.Value],
				}
			}
		case *ast.AliasDecl:
			if s == nil || s.Name == nil {
				continue
			}
			if includePrivate || s.Visibility == ast.Public {
				symbolWriter.addSymbol(symbolParamsFromToken(
					s.Name.Value,
					"alias",
					s.Name.Token,
					s.Visibility == ast.Public,
				))
				underlying := ""
				if s.Underlying != nil {
					underlying = s.Underlying.String()
				}
				index.Aliases[s.Name.Value] = AliasInfo{
					Name:       s.Name.Value,
					Underlying: underlying,
					Doc:        index.Docs[s.Name.Value],
				}
			}
		case *ast.VarStatement:
			if s == nil || s.Name == nil {
				continue
			}
			symbolWriter.addSymbol(symbolParamsFromToken(
				s.Name.Value,
				"var",
				s.Name.Token,
				false,
			))
			typeStr := ""
			if s.Type != nil {
				typeStr = s.Type.String()
			}
			index.Vars[s.Name.Value] = VarInfo{
				Name:    s.Name.Value,
				Type:    typeStr,
				Mutable: s.Mutable,
				Doc:     index.Docs[s.Name.Value],
			}
		case *ast.ConstStatement:
			if s == nil || s.Name == nil {
				continue
			}
			if includePrivate || s.Visibility == ast.Public {
				symbolWriter.addSymbol(symbolParamsFromToken(
					s.Name.Value,
					"const",
					s.Name.Token,
					s.Visibility == ast.Public,
				))
				typeStr := ""
				if s.Type != nil {
					typeStr = s.Type.String()
				}
				valueStr := ""
				if s.Value != nil {
					valueStr = s.Value.String()
				}
				index.Consts[s.Name.Value] = ConstInfo{
					Name:  s.Name.Value,
					Type:  typeStr,
					Value: valueStr,
					Doc:   index.Docs[s.Name.Value],
				}
			}
		case *ast.ImplDecl:
			if s == nil || s.TypeName == nil {
				continue
			}
			for _, m := range s.Methods {
				if m == nil || m.Name == nil {
					continue
				}
				if !includePrivate && m.Visibility != ast.Public {
					continue
				}
				name := s.TypeName.Value + "." + m.Name.Value
				symbolWriter.addSymbol(
					symbolParamsFromTokenWithLength(
						name,
						"method",
						m.Name.Token,
						len(m.Name.Value),
						m.Visibility == ast.Public,
					),
				)

				if sig := buildFuncSignature(
					name,
					m.Parameters,
					m.ReturnType,
					index.Docs[name],
				); sig.Label != "" {
					index.Sigs[name] = sig
				}
			}
		}
	}
	return index
}

func buildFuncSignature(
	name string,
	params []*ast.Parameter,
	ret ast.TypeExpression,
	doc string,
) SignatureInfo {
	if name == "" {
		return SignatureInfo{}
	}
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

	label := strfmt.Named(
		"{name}({paramLabels}) -> ({retType})",
		"name", name,
		"paramLabels", strings.Join(paramLabels, ", "),
		"retType", retType,
	)

	return SignatureInfo{
		Label:  label,
		Params: paramLabels,
		Doc:    doc,
	}
}

func buildDocIndex(
	prog *ast.Program,
	comments []formatter.Comment,
	includePrivate bool,
) map[string]string {
	docByLine := make(map[int]string)
	lineComment := make(map[int]string)
	for _, c := range comments {
		if after, ok := strings.CutPrefix(c.Text, "//"); ok {
			lineComment[c.Line] = strings.TrimSpace(after)
		}
	}

	for _, stmt := range prog.Statements {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			if s != nil && s.Name != nil &&
				(includePrivate || s.Visibility == ast.Public) {

				docByLine[s.Name.Token.Line] = collectDoc(lineComment, s.Name.Token.Line)
			}

		case *ast.StructDecl:
			if s != nil && s.Name != nil &&
				(includePrivate || s.Visibility == ast.Public) {
				docByLine[s.Name.Token.Line] = collectDoc(lineComment, s.Name.Token.Line)
			}
		case *ast.EnumDecl:
			if s != nil && s.Name != nil && (includePrivate || s.Visibility == ast.Public) {
				docByLine[s.Name.Token.Line] = collectDoc(lineComment, s.Name.Token.Line)
			}
		case *ast.TypeDecl:
			if s != nil && s.Name != nil && (includePrivate || s.Visibility == ast.Public) {
				docByLine[s.Name.Token.Line] = collectDoc(lineComment, s.Name.Token.Line)
			}
		case *ast.AliasDecl:
			if s != nil && s.Name != nil && (includePrivate || s.Visibility == ast.Public) {
				docByLine[s.Name.Token.Line] = collectDoc(lineComment, s.Name.Token.Line)
			}
		case *ast.ImplDecl:
			if s == nil || s.TypeName == nil {
				continue
			}
			for _, m := range s.Methods {
				if m == nil || m.Name == nil {
					continue
				}
				docByLine[m.Name.Token.Line] = collectDoc(lineComment, m.Name.Token.Line)
			}
		}
	}

	docs := make(map[string]string)
	for _, stmt := range prog.Statements {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			if s != nil && s.Name != nil && (includePrivate || s.Visibility == ast.Public) {
				if doc := docByLine[s.Name.Token.Line]; doc != "" {
					docs[s.Name.Value] = doc
				}
			}
		case *ast.StructDecl:
			if s != nil && s.Name != nil && (includePrivate || s.Visibility == ast.Public) {
				if doc := docByLine[s.Name.Token.Line]; doc != "" {
					docs[s.Name.Value] = doc
				}
			}
		case *ast.EnumDecl:
			if s != nil && s.Name != nil && (includePrivate || s.Visibility == ast.Public) {
				if doc := docByLine[s.Name.Token.Line]; doc != "" {
					docs[s.Name.Value] = doc
				}
			}
		case *ast.TypeDecl:
			if s != nil && s.Name != nil && (includePrivate || s.Visibility == ast.Public) {
				if doc := docByLine[s.Name.Token.Line]; doc != "" {
					docs[s.Name.Value] = doc
				}
			}
		case *ast.AliasDecl:
			if s != nil && s.Name != nil && (includePrivate || s.Visibility == ast.Public) {
				if doc := docByLine[s.Name.Token.Line]; doc != "" {
					docs[s.Name.Value] = doc
				}
			}
		case *ast.ImplDecl:
			if s == nil || s.TypeName == nil {
				continue
			}
			for _, m := range s.Methods {
				if m == nil || m.Name == nil {
					continue
				}
				if doc := docByLine[m.Name.Token.Line]; doc != "" {
					key := s.TypeName.Value + "." + m.Name.Value
					docs[key] = doc
				}
			}
		}
	}

	return docs
}

func collectDoc(lineComment map[int]string, declLine int) string {
	if declLine <= 1 {
		return ""
	}
	lines := []string{}
	for line := declLine - 1; line >= 1; line-- {
		txt, ok := lineComment[line]
		if !ok {
			break
		}
		lines = append([]string{txt}, lines...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

type addSymbolParams struct {
	Name     string
	Kind     string
	Line     int
	Column   int
	Length   int
	Exported bool
}

func symbolParamsFromToken(
	name,
	kind string,
	tok token.Token,
	exported bool,
) addSymbolParams {
	return symbolParamsFromTokenWithLength(
		name,
		kind,
		tok,
		len(name),
		exported,
	)
}

func symbolParamsFromTokenWithLength(
	name,
	kind string,
	tok token.Token,
	length int,
	exported bool,
) addSymbolParams {
	return addSymbolParams{
		Name:     name,
		Kind:     kind,
		Line:     tok.Line,
		Column:   tok.Column,
		Length:   length,
		Exported: exported,
	}
}

type indexSymbolWriter struct {
	index *FileIndex
	uri   string
}

func newIndexSymbolWriter(index *FileIndex, uri string) indexSymbolWriter {
	return indexSymbolWriter{
		index: index,
		uri:   uri,
	}
}

func (w indexSymbolWriter) addSymbol(p addSymbolParams) {
	if p.Line <= 0 || p.Column <= 0 || w.index == nil {
		return
	}
	if p.Length <= 0 {
		p.Length = len(p.Name)
	}
	w.index.Symbols[p.Name] = SymbolInfo{
		Name:     p.Name,
		Kind:     p.Kind,
		Location: makeLocation(w.uri, p.Line, p.Column, p.Length),
		Exported: p.Exported,
	}
}

func sortedSymbols(index *FileIndex) []SymbolInfo {
	if index == nil {
		return nil
	}
	items := make([]SymbolInfo, 0, len(index.Symbols))
	for _, sym := range index.Symbols {
		items = append(items, sym)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

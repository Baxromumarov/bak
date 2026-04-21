package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/builtins"
	"github.com/baxromumarov/bak/pkg/formatter"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/linter"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/token"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

// builtinMethodInfo holds signature and documentation for a built-in method.
type builtinMethodInfo struct {
	Signature string
	Doc       string
}

// builtinMethods maps a type name to a map of method names to their info.
var builtinMethods = map[string]map[string]builtinMethodInfo{
	"string": {
		"len":           {Signature: "func len() -> (int)", Doc: "Returns the length of the string in bytes."},
		"bytes":         {Signature: "func bytes() -> (Vec<int, _>)", Doc: "Returns the byte representation of the string as a vector of integers."},
		"chars":         {Signature: "func chars() -> (Vec<char, _>)", Doc: "Returns the characters of the string as a vector of chars."},
		"lines":         {Signature: "func lines() -> (Vec<string, _>)", Doc: "Returns the lines of the string."},
		"split":         {Signature: "func split(sep: string) -> (Vec<string, _>)", Doc: "Splits the string by the given separator."},
		"replace":       {Signature: "func replace(old: string, new: string) -> (string)", Doc: "Replaces all occurrences of old with new."},
		"substring":     {Signature: "func substring(start: int, end: int) -> (string)", Doc: "Returns a substring from start (inclusive) to end (exclusive)."},
		"trim":          {Signature: "func trim() -> (string)", Doc: "Returns the string with leading and trailing whitespace removed."},
		"to_int":        {Signature: "func to_int() -> (Result<int, string>)", Doc: "Parses the string as an integer."},
		"contains":      {Signature: "func contains(substr: string) -> (bool)", Doc: "Returns true if the string contains the substring."},
		"hash":          {Signature: "func hash() -> (int)", Doc: "Returns the hash code of the string."},
		"index_of":      {Signature: "func index_of(substr: string) -> (Result<int, string>)", Doc: "Returns the index of the first occurrence of the substring, or Err if not found."},
		"last_index_of": {Signature: "func last_index_of(substr: string) -> (Result<int, string>)", Doc: "Returns the index of the last occurrence of the substring, or Err if not found."},
		"starts_with":   {Signature: "func starts_with(prefix: string) -> (bool)", Doc: "Returns true if the string starts with prefix."},
		"ends_with":     {Signature: "func ends_with(suffix: string) -> (bool)", Doc: "Returns true if the string ends with suffix."},
		"parse_int":     {Signature: "func parse_int() -> (Result<int, string>)", Doc: "Parses the string as an integer."},
		"parse_float":   {Signature: "func parse_float() -> (Result<float64, string>)", Doc: "Parses the string as a float."},
		"to_string":     {Signature: "func to_string() -> (string)", Doc: "Returns this string value."},
		"get":           {Signature: "func get(index: int) -> (Result<char, string>)", Doc: "Returns the character at index, or Err if out of bounds."},
		"indexOf":       {Signature: "func indexOf(substr: string) -> (Result<int, string>)", Doc: "Deprecated: use index_of."},
		"lastIndexOf":   {Signature: "func lastIndexOf(substr: string) -> (Result<int, string>)", Doc: "Deprecated: use last_index_of."},
		"startsWith":    {Signature: "func startsWith(prefix: string) -> (bool)", Doc: "Deprecated: use starts_with."},
		"endsWith":      {Signature: "func endsWith(suffix: string) -> (bool)", Doc: "Deprecated: use ends_with."},
		"parseInt":      {Signature: "func parseInt() -> (Result<int, string>)", Doc: "Deprecated: use parse_int."},
		"parseFloat":    {Signature: "func parseFloat() -> (Result<float64, string>)", Doc: "Deprecated: use parse_float."},
		"toString":      {Signature: "func toString() -> (string)", Doc: "Deprecated: use to_string."},
	},
	"Vec": {
		"push":     {Signature: "func push(value: T) -> (void)", Doc: "Pushes an element to the end of the vector."},
		"append":   {Signature: "func append(other: Vec<T, _>) -> (void)", Doc: "Appends all elements from another vector."},
		"pop":      {Signature: "func pop() -> (Result<T, string>)", Doc: "Removes and returns the last element of the vector, or Err if empty."},
		"remove":   {Signature: "func remove(index: int) -> (Result<T, string>)", Doc: "Removes and returns the element at the given index."},
		"first":    {Signature: "func first() -> (Result<T, string>)", Doc: "Returns the first element, or Err if empty."},
		"last":     {Signature: "func last() -> (Result<T, string>)", Doc: "Returns the last element, or Err if empty."},
		"get":      {Signature: "func get(index: int) -> (Result<T, string>)", Doc: "Returns the element at index, or Err if out of bounds."},
		"len":      {Signature: "func len() -> (int)", Doc: "Returns the number of elements in the vector."},
		"cap":      {Signature: "func cap() -> (int)", Doc: "Returns the vector capacity."},
		"is_empty": {Signature: "func is_empty() -> (bool)", Doc: "Returns true if the vector has no elements."},
		"isEmpty":  {Signature: "func isEmpty() -> (bool)", Doc: "Deprecated: use is_empty."},
		"clear":    {Signature: "func clear() -> (void)", Doc: "Removes all elements from the vector."},
		"reverse":  {Signature: "func reverse() -> (void)", Doc: "Reverses elements in place."},
		"contains": {Signature: "func contains(value: T) -> (bool)", Doc: "Returns true if the vector contains the value."},
		"join":     {Signature: "func join(separator: string) -> (string)", Doc: "Concatenates elements using a separator."},
		"slice":    {Signature: "func slice(start: int, end: int) -> (Vec<T, _>)", Doc: "Returns a sub-vector between start and end."},
		"to_vec":   {Signature: "func to_vec() -> (Vec<T, _>)", Doc: "Returns a dynamic Vec copy."},
		"set":      {Signature: "func set(index: int, value: T) -> (void)", Doc: "Sets the element at index."},
	},
	"HashMap": {
		"insert":   {Signature: "func insert(key: K, value: V) -> (void)", Doc: "Inserts or updates a key-value pair."},
		"get":      {Signature: "func get(key: &K) -> (Result<V, string>)", Doc: "Looks up a key and returns Result<V, string>."},
		"len":      {Signature: "func len() -> (int)", Doc: "Returns the number of key-value pairs in the map."},
		"is_empty": {Signature: "func is_empty() -> (bool)", Doc: "Returns true if the map has no entries."},
		"keys":     {Signature: "func keys() -> (Vec<K, _>)", Doc: "Returns a vector of all keys in the map."},
		"values":   {Signature: "func values() -> (Vec<V, _>)", Doc: "Returns a vector of all values in the map."},
		"clear":    {Signature: "func clear() -> (void)", Doc: "Removes all entries from the map."},
		"remove":   {Signature: "func remove(key: &K) -> (Result<V, string>)", Doc: "Removes and returns the value for the given key."},
		"contains": {Signature: "func contains(key: &K) -> (bool)", Doc: "Returns true if the map contains the given key."},
	},
	"Map": {
		"len":      {Signature: "func len() -> (int)", Doc: "Returns the number of key-value pairs in the map."},
		"keys":     {Signature: "func keys() -> (Vec<K, _>)", Doc: "Returns a vector of all keys in the map."},
		"values":   {Signature: "func values() -> (Vec<V, _>)", Doc: "Returns a vector of all values in the map."},
		"clear":    {Signature: "func clear() -> (void)", Doc: "Removes all entries from the map."},
		"remove":   {Signature: "func remove(key: K) -> (void)", Doc: "Removes the entry with the given key."},
		"contains": {Signature: "func contains(key: K) -> (bool)", Doc: "Returns true if the map contains the given key."},
	},
	"Result": {
		"is_ok":      {Signature: "func is_ok() -> (bool)", Doc: "Returns true if the result is Ok."},
		"is_err":     {Signature: "func is_err() -> (bool)", Doc: "Returns true if the result is Err."},
		"unwrap":     {Signature: "func unwrap() -> (T)", Doc: "Returns the Ok value. Panics if Err."},
		"unwrap_err": {Signature: "func unwrap_err() -> (E)", Doc: "Returns the Err value. Panics if Ok."},
		"to_string":  {Signature: "func to_string() -> (string)", Doc: "Returns a string representation of the Result."},
		"toString":   {Signature: "func toString() -> (string)", Doc: "Deprecated: use to_string."},
	},
	"Option": {
		"is_some":   {Signature: "func is_some() -> (bool)", Doc: "Returns true if the option is Some."},
		"is_none":   {Signature: "func is_none() -> (bool)", Doc: "Returns true if the option is None."},
		"unwrap":    {Signature: "func unwrap() -> (T)", Doc: "Returns the Some value. Panics if None."},
		"to_string": {Signature: "func to_string() -> (string)", Doc: "Returns a string representation of the Option."},
	},
}

var builtinStaticMethods = map[string]map[string]builtinMethodInfo{
	"Vec": {
		"new":      {Signature: "func new() -> (Vec<T, _>)", Doc: "Creates an empty vector."},
		"with_cap": {Signature: "func with_cap(cap: int) -> (Vec<T, _>)", Doc: "Creates an empty vector with reserved capacity."},
		"from":     {Signature: "func from<N>(arr: Vec<T, N>) -> (Vec<T, _>)", Doc: "Creates a dynamic vector from a fixed-size vector."},
	},
	"HashMap": {
		"new":      {Signature: "func new() -> (HashMap<K, V>)", Doc: "Creates an empty hash map."},
		"with_cap": {Signature: "func with_cap(c: int) -> (HashMap<K, V>)", Doc: "Creates an empty hash map with reserved capacity."},
	},
}

type AnalysisResult struct {
	AST      *ast.Program
	TC       *typechecker.TypeChecker
	Index    *FileIndex
	Imports  map[string]string
	RefIndex map[string][]Location
	RefByPos map[string]string
	Defs     map[string]Location
}

type SymbolInfo struct {
	Name     string
	Kind     string
	Location Location
	Exported bool
}

type FileIndex struct {
	Symbols map[string]SymbolInfo
	Docs    map[string]string
	Sigs    map[string]SignatureInfo
	Structs map[string]StructInfo
	Consts  map[string]ConstInfo
	Types   map[string]TypeDeclInfo
	Aliases map[string]AliasInfo
	Enums   map[string]EnumInfo
	Vars    map[string]VarInfo
}

type SignatureInfo struct {
	Label  string
	Params []string
	Doc    string
}

type StructInfo struct {
	Name   string
	Fields []string
	Doc    string
}

type ConstInfo struct {
	Name  string
	Type  string
	Value string
	Doc   string
}

type TypeDeclInfo struct {
	Name       string
	Underlying string
	Doc        string
}

type AliasInfo struct {
	Name       string
	Underlying string
	Doc        string
}

type EnumInfo struct {
	Name     string
	Variants []string
	Doc      string
}

type VarInfo struct {
	Name    string
	Type    string
	Mutable bool
	Doc     string
}
type Server struct {
	Documents      map[string]string
	Cache          map[string]*AnalysisResult
	Indexes        map[string]*FileIndex
	PublicIndexes  map[string]*FileIndex // Cache for external module indexes (public symbols only)
	RootPath       string
	stdImportPaths []string
	stdPackages    []string
	pendingLocks   map[string]*time.Timer
}

func NewServer() *Server {
	return &Server{
		Documents:     make(map[string]string),
		Cache:         make(map[string]*AnalysisResult),
		Indexes:       make(map[string]*FileIndex),
		PublicIndexes: make(map[string]*FileIndex),
		pendingLocks:  make(map[string]*time.Timer),
	}
}

func (s *Server) Handle(req Request) any {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "initialized":
		return nil
	case "textDocument/didOpen":
		s.handleDidOpen(req)
		return nil
	case "textDocument/didChange":
		s.handleDidChange(req)
		return nil
	case "textDocument/didSave":
		return nil
	case "textDocument/hover":
		return s.handleHover(req)
	case "textDocument/definition":
		return s.handleDefinition(req)
	case "textDocument/typeDefinition":
		return s.handleTypeDefinition(req)
	case "textDocument/implementation":
		return s.handleImplementation(req)
	case "textDocument/references":
		return s.handleReferences(req)
	case "textDocument/documentSymbol":
		return s.handleDocumentSymbol(req)
	case "workspace/symbol":
		return s.handleWorkspaceSymbol(req)
	case "textDocument/rename":
		return s.handleRename(req)
	case "textDocument/completion":
		return s.handleCompletion(req)
	case "textDocument/signatureHelp":
		return s.handleSignatureHelp(req)
	case "textDocument/semanticTokens/full":
		return s.handleSemanticTokensFull(req)
	case "textDocument/inlayHint":
		return s.handleInlayHint(req)
	case "textDocument/formatting":
		return s.handleFormatting(req)
	case "textDocument/documentHighlight":
		return s.handleDocumentHighlight(req)
	case "textDocument/codeAction":
		return s.handleCodeAction(req)
	case "shutdown":
		return nil
	case "exit":
		os.Exit(0)
		return nil
	}
	return nil
}

func (s *Server) handleInitialize(req Request) InitializeResult {
	var params InitializeParams
	if err := json.Unmarshal(req.Params, &params); err == nil {
		if params.RootURI != "" {
			s.RootPath = uriToPath(params.RootURI)
		} else if params.RootPath != "" {
			s.RootPath = params.RootPath
		}
	}
	// Set CWD to project root so typechecker import resolution works
	if s.RootPath != "" {
		_ = os.Chdir(s.RootPath)
	}
	return InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync:       1, // Full sync
			HoverProvider:          true,
			DefinitionProvider:     true,
			ImplementationProvider: true,
			ReferencesProvider:     true,
			RenameProvider:         true,
			CompletionProvider:     &CompletionOptions{ResolveProvider: false, TriggerCharacters: []string{".", "{", ",", ":"}},
			SignatureHelpProvider:  &SignatureHelpOptions{TriggerCharacters: []string{"(", ","}},
			// Do not provide semantic tokens from the server - rely on TextMate
			// grammar + theme for coloring. Semantic tokens from the server were
			// causing scope/color mismatches with some VS Code themes.
			SemanticTokensProvider:     nil,
			TypeDefinitionProvider:     true,
			InlayHintProvider:          true,
			DocumentFormattingProvider: true,
			DocumentHighlightProvider:  true,
			CodeActionProvider:         true,
			DocumentSymbolProvider:     true,
			WorkspaceSymbolProvider:    true,
		},
	}
}

func (s *Server) handleDidOpen(req Request) {
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Error unmarshalling didOpen: %v", err)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC handling method textDocument/didOpen: %v\nStack Trace:\n%s", r, debug.Stack())
		}
	}()
	s.Documents[params.TextDocument.URI] = params.TextDocument.Text
	s.analyzeAndPublish(params.TextDocument.URI, params.TextDocument.Text)
}

func (s *Server) handleDidChange(req Request) {
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Error unmarshalling didChange: %v", err)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC handling method textDocument/didChange: %v\nStack Trace:\n%s", r, debug.Stack())
		}
	}()
	if len(params.ContentChanges) > 0 {
		text := params.ContentChanges[0].Text
		s.Documents[params.TextDocument.URI] = text
		typechecker.InvalidatePackage(uriToPath(params.TextDocument.URI))

		// Debounce analysis
		if timer, ok := s.pendingLocks[params.TextDocument.URI]; ok {
			timer.Stop()
		}
		s.pendingLocks[params.TextDocument.URI] = time.AfterFunc(200*time.Millisecond, func() {
			s.analyzeAndPublish(params.TextDocument.URI, text)
		})
	}
}

func (s *Server) handleHover(req Request) *Hover {
	var params HoverParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	text, _ := s.Documents[params.TextDocument.URI]

	result, ok := s.Cache[params.TextDocument.URI]
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

	// Helper for checking built-ins
	if mce, ok := node.(*ast.MethodCallExpression); ok {
		// Primitive method support (e.g. string.bytes())
		// We try to guess the receiver type based on common variable names or explicit type info if available.
		// Since we have result.TC (TypeChecker), we might be able to get the type.
		if result.TC != nil {
			typeStr := result.TC.GetNodeType(mce.Object)
			if typeStr != "" {
				// Clean up type string (e.g. "string" -> "string")
				if info, ok := builtinMethods[typeStr][mce.Method.Value]; ok {
					return &Hover{
						Contents: MarkupContent{
							Kind:  "markdown",
							Value: fmt.Sprintf("```bak\n%s\n```\n%s", info.Signature, info.Doc),
						},
					}
				}
			}
		}
		// Fallback: Check if it matches a known builtin method name globally (less accurate but helpful)
		for _, methods := range builtinMethods {
			if info, ok := methods[mce.Method.Value]; ok {
				return &Hover{
					Contents: MarkupContent{
						Kind:  "markdown",
						Value: fmt.Sprintf("```bak\n%s\n```\n%s", info.Signature, info.Doc),
					},
				}
			}
		}
	}

	// Fallback for built-in methods when hovering over the identifier part (e.g. `bytes` in `msg.bytes()`)
	var builtinInfo *builtinMethodInfo
	if ident, ok := node.(*ast.Identifier); ok {
		// Heuristic: if it's not in the index, search builtins
		if result.Index == nil || result.Index.Symbols[ident.Value].Location.Range.Start.Line == 0 {
			for _, methods := range builtinMethods {
				if info, ok := methods[ident.Value]; ok {
					builtinInfo = &info
					break
				}
			}
		}
	}
	if builtinInfo != nil {
		return &Hover{
			Contents: MarkupContent{
				Kind:  "markdown",
				Value: fmt.Sprintf("```bak\n%s\n```\n%s", builtinInfo.Signature, builtinInfo.Doc),
			},
		}
	}

	typeStr := ""
	if result.TC != nil {
		typeStr = result.TC.GetNodeType(node)
	}
	doc := ""
	sig := ""
	structInfo := StructInfo{}
	hasStructInfo := false
	if result.Index != nil && result.Index.Docs != nil {
		switch n := node.(type) {
		case *ast.Identifier:
			doc = result.Index.Docs[n.Value]
			if result.Index.Sigs != nil {
				if s, ok := result.Index.Sigs[n.Value]; ok {
					sig = s.Label
					if doc == "" {
						doc = s.Doc
					}
				}
			}
			if result.Index.Structs != nil {
				if st, ok := result.Index.Structs[n.Value]; ok {
					structInfo = st
					hasStructInfo = true
					if doc == "" {
						doc = st.Doc
					}
				}
			}
			// Check for constants
			if sig == "" && result.Index.Consts != nil {
				if c, ok := result.Index.Consts[n.Value]; ok {
					sig = fmt.Sprintf("const %s: %s = %s", c.Name, c.Type, c.Value)
					if doc == "" {
						doc = c.Doc
					}
				}
			}
			// Check for type declarations
			if sig == "" && result.Index.Types != nil {
				if t, ok := result.Index.Types[n.Value]; ok {
					sig = fmt.Sprintf("type %s = %s", t.Name, t.Underlying)
					if doc == "" {
						doc = t.Doc
					}
				}
			}
			// Check for aliases
			if sig == "" && result.Index.Aliases != nil {
				if a, ok := result.Index.Aliases[n.Value]; ok {
					sig = fmt.Sprintf("alias %s = %s", a.Name, a.Underlying)
					if doc == "" {
						doc = a.Doc
					}
					if !hasStructInfo && a.Underlying != "" {
						if st, ok := s.findStructInfoByType(result, params.TextDocument.URI, a.Underlying); ok {
							structInfo = st
							hasStructInfo = true
						}
					}
				}
			}
			// Check for enums
			if sig == "" && !hasStructInfo && result.Index.Enums != nil {
				if e, ok := result.Index.Enums[n.Value]; ok {
					variants := strings.Join(e.Variants, ", ")
					sig = fmt.Sprintf("enum %s { %s }", e.Name, variants)
					if doc == "" {
						doc = e.Doc
					}
				}
			}
			// Check for import alias to show package documentation
			if result.Imports != nil {
				if importPath, ok := result.Imports[n.Value]; ok {
					path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
					if path != "" {
						pkgName, pkgDoc := s.packageDoc(path)
						if sig == "" && pkgName != "" {
							sig = fmt.Sprintf("package %s", pkgName)
						}
						if doc == "" && pkgDoc != "" {
							doc = pkgDoc
						}
					}
				}
			}
			// Check for variables
			if sig == "" && result.Index.Vars != nil {
				if v, ok := result.Index.Vars[n.Value]; ok {
					mutPrefix := ""
					if v.Mutable {
						mutPrefix = "mut "
					}
					sig = fmt.Sprintf("%svar %s: %s", mutPrefix, v.Name, v.Type)
					if doc == "" {
						doc = v.Doc
					}
				}
			}
			if doc == "" && sig == "" && !hasStructInfo && text != "" {
				lineText := lineAt(text, params.Position.Line)
				word, start := wordAt(lineText, params.Position.Character)
				if word == n.Value && start > 0 && lineText[start-1] == '.' {
					qualifier := qualifierBefore(lineText, start-1)
					if qualifier != "" {
						if importPath, ok := result.Imports[qualifier]; ok {
							path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
							if path != "" {
								modIndex := s.getOrIndexFile(path)
								if modIndex != nil && modIndex.Docs != nil {
									doc = modIndex.Docs[word]
									if modIndex.Sigs != nil {
										if s, ok := modIndex.Sigs[word]; ok {
											sig = s.Label
											if doc == "" {
												doc = s.Doc
											}
										}
									}
									if modIndex.Structs != nil {
										if st, ok := modIndex.Structs[word]; ok {
											structInfo = st
											hasStructInfo = true
											if doc == "" {
												doc = st.Doc
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
						if modIndex != nil && modIndex.Docs != nil {
							doc = modIndex.Docs[n.Field.Value]
							if modIndex.Sigs != nil {
								if s, ok := modIndex.Sigs[n.Field.Value]; ok {
									sig = s.Label
									if doc == "" {
										doc = s.Doc
									}
								}
							}
							if modIndex.Structs != nil {
								if st, ok := modIndex.Structs[n.Field.Value]; ok {
									structInfo = st
									hasStructInfo = true
									if doc == "" {
										doc = st.Doc
									}
								}
							}
							// Check for constants in imported module
							if sig == "" && modIndex.Consts != nil {
								if c, ok := modIndex.Consts[n.Field.Value]; ok {
									sig = fmt.Sprintf("const %s: %s = %s", c.Name, c.Type, c.Value)
									if doc == "" {
										doc = c.Doc
									}
								}
							}
							// Check for types in imported module
							if sig == "" && modIndex.Types != nil {
								if t, ok := modIndex.Types[n.Field.Value]; ok {
									sig = fmt.Sprintf("type %s = %s", t.Name, t.Underlying)
									if doc == "" {
										doc = t.Doc
									}
								}
							}
							// Check for aliases in imported module
							if sig == "" && modIndex.Aliases != nil {
								if a, ok := modIndex.Aliases[n.Field.Value]; ok {
									sig = fmt.Sprintf("alias %s = %s", a.Name, a.Underlying)
									if doc == "" {
										doc = a.Doc
									}
								}
							}
							// Check for enums in imported module
							if sig == "" && !hasStructInfo && modIndex.Enums != nil {
								if e, ok := modIndex.Enums[n.Field.Value]; ok {
									variants := strings.Join(e.Variants, ", ")
									sig = fmt.Sprintf("enum %s { %s }", e.Name, variants)
									if doc == "" {
										doc = e.Doc
									}
								}
							}
						}
					}
				}
				if doc == "" && sig == "" && !hasStructInfo {
					key := ident.Value + "." + n.Field.Value
					doc = result.Index.Docs[key]
					if result.Index.Sigs != nil {
						if s, ok := result.Index.Sigs[key]; ok {
							sig = s.Label
							if doc == "" {
								doc = s.Doc
							}
						}
					}
					if result.Index.Structs != nil {
						if st, ok := result.Index.Structs[key]; ok {
							structInfo = st
							hasStructInfo = true
							if doc == "" {
								doc = st.Doc
							}
						}
					}
				}
			}
		case *ast.MethodCallExpression:
			if ident, ok := n.Object.(*ast.Identifier); ok {
				if importPath, ok := result.Imports[ident.Value]; ok {
					path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
					if path != "" {
						modIndex := s.getOrIndexFile(path)
						if modIndex != nil && modIndex.Docs != nil {
							methodName := n.Method.Value
							doc = modIndex.Docs[methodName]
							if modIndex.Sigs != nil {
								if s, ok := modIndex.Sigs[methodName]; ok {
									sig = s.Label
									if doc == "" {
										doc = s.Doc
									}
								}
							}
							if doc == "" && modIndex.Structs != nil {
								if st, ok := modIndex.Structs[methodName]; ok {
									structInfo = st
									hasStructInfo = true
									doc = st.Doc
								}
							}
						}
					}
				}
			}
			if doc == "" && sig == "" && !hasStructInfo {
				if result.TC != nil {
					if t := result.TC.GetNodeType(n.Object); t != "" {
						baseType := baseTypeName(t)
						key := baseType + "." + n.Method.Value

						// Try local index first
						doc = result.Index.Docs[key]
						if result.Index.Sigs != nil {
							if s, ok := result.Index.Sigs[key]; ok {
								sig = s.Label
								if doc == "" {
									doc = s.Doc
								}
							}
						}

						// If not found locally, try imported modules
						if doc == "" && sig == "" {
							for _, importPath := range result.Imports {
								path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
								if path != "" {
									modIndex := s.getOrIndexFile(path)
									if modIndex != nil {
										// Try fully qualified key (Type.Method)
										if d, ok := modIndex.Docs[key]; ok {
											doc = d
										}
										if modIndex.Sigs != nil {
											if s, ok := modIndex.Sigs[key]; ok {
												sig = s.Label
												if doc == "" {
													doc = s.Doc
												}
											}
										}

										// If still not found, try just method name
										if doc == "" && sig == "" {
											if d, ok := modIndex.Docs[n.Method.Value]; ok {
												doc = d
											}
											if modIndex.Sigs != nil {
												if s, ok := modIndex.Sigs[n.Method.Value]; ok {
													sig = s.Label
													if doc == "" {
														doc = s.Doc
													}
												}
											}
										}

										// If found, break
										if doc != "" || sig != "" {
											break
										}
									}
								}
							}
						}
					}
				}

				// If not found locally or in imports, check prelude (Vec, HashMap)
				if doc == "" && sig == "" {
					var key string
					if result.TC != nil {
						if t := result.TC.GetNodeType(n.Object); t != "" {
							key = baseTypeName(t) + "." + n.Method.Value
						}
					}

					stdLibPath := getStdLibPath()
					preludeFiles := []string{
						filepath.Join(stdLibPath, "collections", "vec.bak"),
						filepath.Join(stdLibPath, "collections", "hashmap.bak"),
						filepath.Join(stdLibPath, "result.bak"),
					}

					for _, path := range preludeFiles {
						modIndex := s.getOrIndexFile(path)
						if modIndex != nil {
							// Try fully qualified key (Type.Method)
							if key != "" {
								if d, ok := modIndex.Docs[key]; ok {
									doc = d
								}
								if modIndex.Sigs != nil {
									if s, ok := modIndex.Sigs[key]; ok {
										sig = s.Label
										if doc == "" {
											doc = s.Doc
										}
									}
								}
							}

							// If still not found, try just method name
							if doc == "" && sig == "" {
								if d, ok := modIndex.Docs[n.Method.Value]; ok {
									doc = d
								}
								if modIndex.Sigs != nil {
									if s, ok := modIndex.Sigs[n.Method.Value]; ok {
										sig = s.Label
										if doc == "" {
											doc = s.Doc
										}
									}
								}
							}

							// If found, break
							if doc != "" || sig != "" {
								break
							}
						}
					}
				}
			}
		case *ast.CallExpression:
			// Sometimes module function calls are parsed as CallExpression with a FieldAccess function
			if fa, ok := n.Function.(*ast.FieldAccessExpression); ok {
				if ident, ok := fa.Object.(*ast.Identifier); ok {
					if importPath, ok := result.Imports[ident.Value]; ok {
						path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
						if path != "" {
							modIndex := s.getOrIndexFile(path)
							if modIndex != nil && modIndex.Sigs != nil {
								funcName := fa.Field.Value
								if s, ok := modIndex.Sigs[funcName]; ok {
									sig = s.Label
									doc = s.Doc
									if doc == "" {
										doc = modIndex.Docs[funcName]
									}
								}
							}
						}
					}
				}
			}
		case *ast.SimpleType:
			if !hasStructInfo {
				if st, ok := s.findStructInfoByType(result, params.TextDocument.URI, n.Name); ok {
					structInfo = st
					hasStructInfo = true
				}
			}
		}
	}
	if doc == "" && sig == "" && !hasStructInfo && text != "" {
		lineText := lineAt(text, params.Position.Line)
		word, start := wordAt(lineText, params.Position.Character)
		if word != "" && start > 0 && lineText[start-1] == '.' {
			qualifier := qualifierBefore(lineText, start-1)
			if qualifier != "" {
				if importPath, ok := result.Imports[qualifier]; ok {
					path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
					if path != "" {
						modIndex := s.getOrIndexFile(path)
						if modIndex != nil && modIndex.Docs != nil {
							doc = modIndex.Docs[word]
							if modIndex.Sigs != nil {
								if s, ok := modIndex.Sigs[word]; ok {
									sig = s.Label
									if doc == "" {
										doc = s.Doc
									}
								}
							}
							if modIndex.Structs != nil {
								if st, ok := modIndex.Structs[word]; ok {
									structInfo = st
									hasStructInfo = true
									if doc == "" {
										doc = st.Doc
									}
								}
							}
							// Check for enums in imported module
							if sig == "" && !hasStructInfo && modIndex.Enums != nil {
								if e, ok := modIndex.Enums[word]; ok {
									variants := strings.Join(e.Variants, ", ")
									sig = fmt.Sprintf("enum %s { %s }", e.Name, variants)
									if doc == "" {
										doc = e.Doc
									}
								}
							}
						}
					}
				}
			}
		}
	}
	hoverType := formatHoverType(typeStr)
	if ident, ok := node.(*ast.Identifier); ok && isDynamicVecType(typeStr) {
		vecLen, hasLen := inferDynamicVecLengthAtPosition(result.AST, ident.Value, line, char)
		hoverType = formatDynamicVecTypeWithLength(hoverType, vecLen, hasLen)
	}

	if typeStr != "" || doc != "" || sig != "" || hasStructInfo {
		var body string
		if sig != "" {
			body = fmt.Sprintf("```bak\n%s\n```", sig)
		} else if hasStructInfo {
			lines := []string{"struct " + structInfo.Name}
			for _, f := range structInfo.Fields {
				lines = append(lines, "  "+f)
			}
			body = "```bak\n" + strings.Join(lines, "\n") + "\n```"
		} else if typeStr != "" {
			body = fmt.Sprintf("```bak\n%s: %s\n```", node.TokenLiteral(), hoverType)
		}
		if doc != "" {
			if body != "" {
				body += "\n\n" + doc
			} else {
				body = doc
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

	return nil
}

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
				stdLibPath := getStdLibPath()
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
									stdLibPath := getStdLibPath()
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
				stdLibPath := getStdLibPath()
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
			importLine := fmt.Sprintf("import \"%s\" as %s\n", candidate.ImportPath, candidate.Alias)
			actions = append(actions, CodeAction{
				Title:       fmt.Sprintf("Import '%s' from %s", symbolName, candidate.Alias),
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

// extractUndefinedSymbol extracts the symbol name from "undefined: X" or "undefined type: X" messages.
func extractUndefinedSymbol(msg string) string {
	if strings.HasPrefix(msg, "undefined: ") {
		return strings.TrimSpace(msg[len("undefined: "):])
	}
	if strings.HasPrefix(msg, "undefined type: ") {
		return strings.TrimSpace(msg[len("undefined type: "):])
	}
	return ""
}

// findImportInsertPosition finds where to insert a new import statement.
func findImportInsertPosition(result *AnalysisResult) Position {
	if result == nil || result.AST == nil {
		return Position{Line: 1, Character: 0}
	}
	lastImportLine := 0
	for _, stmt := range result.AST.Statements {
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			if s.Token.Line > lastImportLine {
				lastImportLine = s.Token.Line
			}
		case *ast.PackageStatement:
			if s.Token.Line > lastImportLine {
				lastImportLine = s.Token.Line
			}
		}
	}
	return Position{Line: lastImportLine + 1, Character: 0}
}

func (s *Server) getOrganizeImportsEdit(uri string) *WorkspaceEdit {
	// Simple heuristic for organizing imports in Bak:
	// 1. Collect all import statements.
	// 2. Sort them.
	// 3. Remove duplicates.
	// This is a complex task to do perfectly without a full re-format,
	// but we can provide a basic implementation.

	result := s.Cache[uri]
	if result == nil || result.AST == nil {
		return nil
	}

	var firstImport *ast.ImportStatement
	var lastImport *ast.ImportStatement
	imports := []*ast.ImportStatement{}

	for _, stmt := range result.AST.Statements {
		if imp, ok := stmt.(*ast.ImportStatement); ok {
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

	// Generate new text for imports
	var sb strings.Builder
	seen := make(map[string]bool)
	for _, imp := range imports {
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

	start := Position{Line: firstImport.Token.Line - 1, Character: firstImport.Token.Column - 1}
	end := Position{Line: lastImport.Token.Line - 1, Character: 1000} // End of line

	return &WorkspaceEdit{
		Changes: map[string][]TextEdit{
			uri: {
				{
					Range:   Range{Start: start, End: end},
					NewText: sb.String(),
				},
			},
		},
	}
}

func (s *Server) handleCompletion(req Request) CompletionList {
	var params CompletionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return CompletionList{Items: []CompletionItem{}}
	}
	text, ok := s.Documents[params.TextDocument.URI]
	if !ok {
		return CompletionList{Items: []CompletionItem{}}
	}
	if isPositionInComment(text, params.Position) {
		return CompletionList{Items: []CompletionItem{}}
	}

	lineText := lineAt(text, params.Position.Line)
	if prefix, ok := importPathPrefix(lineText, params.Position.Character); ok {
		items := []CompletionItem{}
		for _, path := range s.getStdImportPaths() {
			if strings.HasPrefix(path, prefix) {
				items = append(items, CompletionItem{
					Label:  path,
					Kind:   9, // Module
					Detail: "std import",
				})
			}
		}
		return CompletionList{Items: items}
	}
	dotIdx := dotBefore(lineText, params.Position.Character)

	isDotCompletion := dotIdx != -1
	qualifier := ""
	if isDotCompletion {
		qualifier = qualifierBefore(lineText, dotIdx)
	} else {
		qualifier = qualifierAt(lineText, params.Position.Character)
	}

	items := []CompletionItem{}
	if !isDotCompletion {
		result := s.Cache[params.TextDocument.URI]
		if result != nil {
			tc := result.TC
			astRoot := result.AST
			if tc == nil || astRoot == nil {
				tc, astRoot = s.typecheckForCompletion(text, params.TextDocument.URI, params.Position)
			}
			if tc != nil && astRoot != nil {
				line := params.Position.Line + 1
				col := params.Position.Character + 1
				lit := findStructLiteralAt(astRoot, line, col)
				typeName := ""
				if lit != nil && lit.Name != nil {
					typeName = lit.Name.Value
				}
				if typeName == "" {
					typeName = structLiteralTypeAt(text, params.Position)
				}
				if typeName != "" {
					currentPkg := ""
					for _, stmt := range astRoot.Statements {
						if pkgStmt, ok := stmt.(*ast.PackageStatement); ok {
							currentPkg = pkgStmt.Name.Value
							break
						}
					}
					typeName = resolveAliasTypeString(result, typeName)
					structDef, ok := tc.GetStruct(typeName)
					if !ok {
						base := baseTypeName(typeName)
						if base != typeName {
							structDef, ok = tc.GetStruct(base)
						}
					}
					if ok {
						isSamePkg := structDef.Package == currentPkg
						existing := map[string]bool{}
						if lit != nil {
							for name := range lit.Fields {
								existing[name] = true
							}
						}
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
							return CompletionList{Items: items}
						}
					}
					if st, ok := s.findStructInfoByType(result, params.TextDocument.URI, typeName); ok {
						existing := map[string]bool{}
						if lit != nil {
							for name := range lit.Fields {
								existing[name] = true
							}
						}
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
						if len(items) > 0 {
							return CompletionList{Items: items}
						}
					}
				}
			}
		}
	}
	if qualifier != "" || isDotCompletion {
		result := s.Cache[params.TextDocument.URI]
		if result != nil {
			tc := result.TC
			astRoot := result.AST
			if tc == nil || astRoot == nil || dotIdx != -1 {
				tc, astRoot = s.typecheckForCompletion(text, params.TextDocument.URI, params.Position)
			}
			currentPkg := ""
			if astRoot != nil {
				for _, stmt := range astRoot.Statements {
					if pkgStmt, ok := stmt.(*ast.PackageStatement); ok {
						currentPkg = pkgStmt.Name.Value
						break
					}
				}
			}

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

					// Check for standard library types that might be injected via prelude (incorrectly appearing as same package)
					// If we are in 'main' but completion is for 'HashMap' (which belongs to 'collections'), force false.
					if isSamePkg {
						if (strings.HasSuffix(baseType, "HashMap") || strings.HasSuffix(baseType, "Vec")) && currentPkg != "collections" {
							isSamePkg = false
						}
					}

					for methodName, methodSig := range structDef.Methods {
						// Filter private methods if different package
						if methodSig.Visibility == ast.Private && !isSamePkg {
							continue
						}

						// Filter based on static/instance context
						if isStatic {
							// If accessing via Type (Static), hide instance methods
							if methodSig.IsInstance {
								continue
							}
						} else {
							// If accessing via Instance, hide static methods
							if !methodSig.IsInstance {
								continue
							}
						}

						insertText := methodName
						insertFormat := 1
						if methodSig != nil {
							insertFormat = 2 // Snippet
							if len(methodSig.Parameters) == 0 {
								insertText = methodName + "()"
							} else {
								insertText = methodName + "($0)"
							}
						}

						items = append(items, CompletionItem{
							Label:            methodName,
							Kind:             2, // Method
							Detail:           methodDetail(methodSig),
							InsertText:       insertText,
							InsertTextFormat: insertFormat,
						})
					}
					// Only show fields if NOT static access
					if !isStatic {
						for fieldName, fieldDef := range structDef.Fields {
							// Filter private fields if different package
							if fieldDef.Visibility == ast.Private && !isSamePkg {
								continue
							}
							items = append(items, CompletionItem{
								Label:  fieldName,
								Kind:   5, // Field
								Detail: fieldDef.Type.String(),
							})
						}
					}
				} else {
					if appendBuiltinTypeMethodCompletions(&items, baseType, isStatic) {
						return
					}
					if result.Index != nil && result.Index.Structs != nil {
						if st, ok := result.Index.Structs[baseType]; ok {
							if !isStatic {
								for _, f := range st.Fields {
									items = append(items, CompletionItem{
										Label:  f,
										Kind:   5, // Field
										Detail: "field",
									})
								}
							}
						}
					}
				}
			}
			// 1. Check for Imports
			if qualifier != "" && isDotCompletion {
				if importPath, ok := result.Imports[qualifier]; ok {
					path := s.resolveImportPath(uriToPath(params.TextDocument.URI), importPath)
					if path != "" {
						modIndex := s.getOrIndexFile(path)
						for _, sym := range sortedSymbols(modIndex) {
							if !sym.Exported {
								continue
							}
							if strings.Contains(sym.Name, ".") {
								continue
							}
							insertText := sym.Name
							insertFormat := 1
							detail := sym.Kind
							if sym.Kind == "func" {
								insertFormat = 2 // Snippet
								// We don't have param info in SymbolInfo easily here,
								// but usually functions have at least one or we can assume ()
								// For now, use ($0) if it's a func to be safe, or just () if we prefer.
								// Actually, let's try to find SignatureInfo if available.
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
					}
					return CompletionList{Items: items}
				}
			}

			// 2. Check for Variable/Struct Methods
			if tc != nil && astRoot != nil {
				typeStr := ""
				isStatic := false

				if isDotCompletion && (qualifier == "" || result.Imports[qualifier] == "") {
					node := findNode(astRoot, params.Position.Line+1, params.Position.Character+1)
					switch n := node.(type) {
					case *ast.FieldAccessExpression:
						typeStr = tc.GetNodeType(n.Object)
						// Check if the object is a Type Identifier
						if ident, ok := n.Object.(*ast.Identifier); ok {
							// Check if this identifier resolves to a variable locally
							locals := collectLocalSymbols(astRoot, params.Position.Line+1)
							_, isLocal := locals[ident.Value]

							// Check if it resolves to a global variable
							isGlobalVar := false
							if result.Index != nil && result.Index.Vars != nil {
								_, isGlobalVar = result.Index.Vars[ident.Value]
							}

							// If it's not a local or global var, and it IS a known struct, assume static
							if !isLocal && !isGlobalVar {
								base := typeStr
								if bracket := strings.Index(base, "<"); bracket != -1 {
									base = base[:bracket]
								}
								// Check against Structs
								if _, ok := tc.GetStruct(base); ok {
									isStatic = true
								} else if result.Index != nil && result.Index.Structs != nil {
									if _, ok := result.Index.Structs[base]; ok {
										isStatic = true
									}
								}
								// Special case: if typeStr == identifier value, it's likely the type itself
								// (assuming types shadow same-named vars or vice versa)
								if base == ident.Value {
									isStatic = true
								}
							}
						}
					case *ast.MethodCallExpression:
						typeStr = tc.GetNodeType(n.Object)
						// Method calls like Type.method() are handled similarly
					}
				}

				if typeStr == "" && qualifier != "" {
					if typeStr == "" {
						locals := collectLocalSymbols(astRoot, params.Position.Line+1)
						if local, ok := locals[qualifier]; ok && local.Node != nil {
							typeStr = tc.GetNodeType(local.Node)
							isStatic = false // It's a local variable
						}
					}
				}
				if typeStr == "" && qualifier != "" && hasBuiltinStaticCompletionType(qualifier) {
					typeStr = qualifier
					isStatic = true
				}
				addMembers(typeStr, isStatic)
			}
			if len(items) == 0 && qualifier != "" && hasBuiltinStaticCompletionType(qualifier) {
				appendBuiltinTypeMethodCompletions(&items, qualifier, true)
			}
		}
		return CompletionList{Items: items}
	}

	if result := s.Cache[params.TextDocument.URI]; result != nil && result.Index != nil {
		seen := make(map[string]bool)
		for _, sym := range sortedSymbols(result.Index) {
			insertText := sym.Name
			insertFormat := 1
			if sym.Kind == "func" || sym.Kind == "method" {
				insertFormat = 2 // Snippet
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
					Kind:   6, // Variable
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
				Kind:   9, // Module
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
				Kind:   9, // Module
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
				Kind:   25, // Type
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
				Kind:   3, // Function
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
				Kind:   9, // Module
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
		"box",
		"type",
		"alias",
		"true",
		"false",
		"nil",
		"void",
	}
	for _, kw := range keywords {
		items = append(items, CompletionItem{Label: kw, Kind: 14})
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
		"Box",
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

	return CompletionList{Items: items}
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
	return fmt.Sprintf("%sfunc(%s) -> (%s)", mut, strings.Join(params, ", "), ret)
}

func hasBuiltinStaticCompletionType(typeName string) bool {
	_, ok := builtinStaticMethods[typeName]
	return ok
}

func appendBuiltinTypeMethodCompletions(items *[]CompletionItem, typeName string, isStatic bool) bool {
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
	"Box":       "Box(value: any) -> (Box<any>)",
	"unbox":     "unbox(value: Box<any>) -> (any)",
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

func rangeFromToken(tok token.Token, name string) Range {
	if tok.Line <= 0 || tok.Column <= 0 {
		return Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}}
	}
	length := len(name)
	if length == 0 {
		length = 1
	}
	startLine := tok.Line - 1
	startCol := tok.Column - 1
	return Range{
		Start: Position{Line: startLine, Character: startCol},
		End:   Position{Line: startLine, Character: startCol + length},
	}
}

func rangeFromSpan(span ast.Span) (Range, bool) {
	if span.Start.Line == 0 || span.End.Line == 0 {
		return Range{}, false
	}
	return Range{
		Start: Position{Line: span.Start.Line - 1, Character: span.Start.Column - 1},
		End:   Position{Line: span.End.Line - 1, Character: span.End.Column - 1},
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
	return Location{URI: uri, Range: rangeFromToken(tok, name)}
}

func symbolID(kind string, loc Location) string {
	return fmt.Sprintf("%s:%s:%d:%d", kind, loc.URI, loc.Range.Start.Line, loc.Range.Start.Character)
}

func tokenKey(tok token.Token) string {
	return fmt.Sprintf("%d:%d", tok.Line, tok.Column)
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
	if before, ok := strings.CutSuffix(t, " box?"); ok {
		t = before
	} else if before, ok := strings.CutSuffix(t, " box"); ok {
		t = before
	}
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

	if before, ok := strings.CutSuffix(t, " box?"); ok {
		inner := strings.TrimSpace(before)
		formattedInner := formatHoverType(inner)
		if formattedInner == "" {
			formattedInner = inner
		}
		return fmt.Sprintf("Result<Box<%s>, string>", formattedInner)
	}

	if before, ok := strings.CutSuffix(t, " box"); ok {
		inner := strings.TrimSpace(before)
		formattedInner := formatHoverType(inner)
		if formattedInner == "" {
			formattedInner = inner
		}
		return fmt.Sprintf("Box<%s>", formattedInner)
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
	if !walkProgramVecLengthPath(prog, line, col, states) {
		return 0, false
	}

	st, ok := states[varName]
	if !ok || !st.known {
		return 0, false
	}
	return st.length, true
}

func walkProgramVecLengthPath(prog *ast.Program, line, col int, states map[string]vecLenState) bool {
	if prog == nil {
		return false
	}
	return walkStatementsVecLengthPath(prog.Statements, line, col, states)
}

func walkStatementsVecLengthPath(stmts []ast.Statement, line, col int, states map[string]vecLenState) bool {
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
					return walkStatementsVecLengthPath(s.Body.Statements, line, col, states)
				}
				return true
			case *ast.ImplDecl:
				if s == nil {
					return true
				}
				for _, m := range s.Methods {
					if m != nil && m.Body != nil && spanContainsPosition(m.Span, line, col) {
						return walkStatementsVecLengthPath(m.Body.Statements, line, col, states)
					}
				}
				return true
			case *ast.IfStatement:
				if s != nil {
					if s.Consequence != nil && spanContainsPosition(ast.SpanOf(s.Consequence), line, col) {
						return walkStatementsVecLengthPath(s.Consequence.Statements, line, col, states)
					}
					if s.Alternative != nil && spanContainsPosition(ast.SpanOf(s.Alternative), line, col) {
						return walkStatementsVecLengthPath(s.Alternative.Statements, line, col, states)
					}
				}
				return true
			case *ast.WhileStatement:
				if s != nil && s.Body != nil {
					return walkStatementsVecLengthPath(s.Body.Statements, line, col, states)
				}
				return true
			case *ast.ForStatement:
				if s != nil && s.Body != nil {
					return walkStatementsVecLengthPath(s.Body.Statements, line, col, states)
				}
				return true
			case *ast.SwitchStatement:
				if s != nil {
					for _, c := range s.Cases {
						if c != nil && c.Body != nil && spanContainsPosition(ast.SpanOf(c.Body), line, col) {
							return walkStatementsVecLengthPath(c.Body.Statements, line, col, states)
						}
					}
				}
				return true
			case *ast.UnsafeBlock:
				if s != nil && s.Body != nil {
					return walkStatementsVecLengthPath(s.Body.Statements, line, col, states)
				}
				return true
			case *ast.BlockStatement:
				if s != nil {
					return walkStatementsVecLengthPath(s.Statements, line, col, states)
				}
				return true
			default:
				applyVecLengthStatement(stmt, states)
				return true
			}
		}

		applyVecLengthStatement(stmt, states)
		if isLoopBoundary(stmt) {
			invalidateAllVecStates(states)
		}
	}

	return true
}

func applyVecLengthStatement(stmt ast.Statement, states map[string]vecLenState) {
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
		applyVecLengthExpr(s.Expression, states)
	case *ast.IfStatement:
		if s == nil {
			return
		}
		base := cloneVecLenStates(states)

		consequenceStates := cloneVecLenStates(base)
		if s.Consequence != nil {
			applyVecLengthStatements(s.Consequence.Statements, consequenceStates)
		}

		alternativeStates := cloneVecLenStates(base)
		if s.Alternative != nil {
			applyVecLengthStatements(s.Alternative.Statements, alternativeStates)
		}

		merged := mergeBranchStates(base, []map[string]vecLenState{consequenceStates, alternativeStates})
		for name, st := range merged {
			states[name] = st
		}
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
			applyVecLengthStatements(c.Body.Statements, caseStates)
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
		for name, st := range merged {
			states[name] = st
		}
	case *ast.UnsafeBlock:
		if s != nil && s.Body != nil {
			applyVecLengthStatements(s.Body.Statements, states)
		}
	case *ast.BlockStatement:
		if s != nil {
			applyVecLengthStatements(s.Statements, states)
		}
	}
}

func applyVecLengthStatements(stmts []ast.Statement, states map[string]vecLenState) {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		applyVecLengthStatement(stmt, states)
		if isLoopBoundary(stmt) {
			invalidateAllVecStates(states)
		}
	}
}

func cloneVecLenStates(states map[string]vecLenState) map[string]vecLenState {
	out := make(map[string]vecLenState, len(states))
	for name, st := range states {
		out[name] = st
	}
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
				case "new", "with_cap", "from":
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

func applyVecLengthExpr(expr ast.Expression, states map[string]vecLenState) {
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
			case "new", "with_cap":
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

func formatFuncDetail(params []*ast.Parameter, ret ast.TypeExpression, mutable bool) string {
	paramLabels := make([]string, 0, len(params))
	for i, p := range params {
		if p == nil {
			continue
		}
		paramName := fmt.Sprintf("arg%d", i+1)
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
		paramLabels = append(paramLabels, fmt.Sprintf("%s: %s", paramName, typ))
	}
	retType := "void"
	if ret != nil {
		retType = ret.String()
	}
	mut := ""
	if mutable {
		mut = "mut "
	}
	return fmt.Sprintf("%sfunc(%s) -> (%s)", mut, strings.Join(paramLabels, ", "), retType)
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

func collectReferences(prog *ast.Program, uri string, name string) []Location {
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
		case *ast.Identifier:
			if n.Value == name {
				add(n.Token.Line, n.Token.Column, len(n.Value))
			}
		case *ast.MethodCallExpression:
			if n.Method != nil && n.Method.Value == name {
				add(n.Method.Token.Line, n.Method.Token.Column, len(n.Method.Value))
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
	idx *FileIndex,
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
		structFields[structName][fieldName] = newDef(fieldName, "field", tok, structName)
	}

	defineStructMethod := func(structName, methodName string, tok token.Token) {
		if structMethods[structName] == nil {
			structMethods[structName] = make(map[string]*symbolDef)
		}
		structMethods[structName][methodName] = newDef(methodName, "method", tok, structName)
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
		case *ast.BoxType:
			if tt != nil {
				walkType(tt.Inner, scope)
			}
		case *ast.BoxOptionalType:
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
		case *ast.BoxExpression:
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

func collectInlayHints(text string, result *AnalysisResult, s *Server) []InlayHint {
	hints := []InlayHint{}
	lines := strings.Split(text, "\n")
	var walk func(node ast.Node)
	walk = func(node ast.Node) {
		if node == nil {
			return
		}
		// log.Printf("Visiting node: %T", node)
		switch n := node.(type) {
		case *ast.CallExpression:
			if n == nil {
				return
			}
			fnName := ""
			if ident, ok := n.Function.(*ast.Identifier); ok && ident != nil {
				fnName = ident.Value
			}
			var sig SignatureInfo
			if fnName != "" && result.Index != nil {
				if s, ok := result.Index.Sigs[fnName]; ok {
					sig = s
				}
			}
			if sig.Label != "" {
				for i, arg := range n.Arguments {
					if arg == nil {
						continue
					}
					if i >= len(sig.Params) {
						break
					}
					pos := exprStartPosition(arg)
					if pos.Line >= 0 {
						label := sig.Params[i]
						if idx := strings.Index(label, ":"); idx != -1 {
							label = strings.TrimSpace(label[:idx]) + ":"
						}
						hints = append(hints, InlayHint{
							Position:     Position{Line: pos.Line - 1, Character: pos.Col - 1},
							Label:        label,
							Kind:         1,
							PaddingRight: true,
						})
					}
				}
			}
			walk(n.Function)
			for _, arg := range n.Arguments {
				if arg == nil {
					continue
				}
				walk(arg)
			}
		case *ast.VarStatement:
			if n != nil {
				if n.Value != nil {
					walk(n.Value)
				}
				// Add type hint if type is implicit (check for presence of colon in source)
				if n.Name != nil && n.Value != nil && result.TC != nil {
					// Check if explicit type annotation exists by looking for colon after var name
					nameEndLine := n.Name.Token.Line - 1
					nameEndCol := n.Name.Token.Column - 1 + len(n.Name.Value)

					// Find start of value
					valPos := exprStartPosition(n.Value)
					if valPos.Line > 0 {
						valStartLine := valPos.Line - 1
						valStartCol := valPos.Col - 1

						isExplicit := false

						// Only support checking single-line declarations for now
						if nameEndLine == valStartLine && nameEndLine < len(lines) {
							line := lines[nameEndLine]
							// Ensure bounds
							if nameEndCol < len(line) && valStartCol <= len(line) && nameEndCol < valStartCol {
								segment := line[nameEndCol:valStartCol]
								if strings.Contains(segment, ":") {
									isExplicit = true
								}
							}
						} else {
							// If multi-line, assume explicit to be safe
							if nameEndLine != valStartLine {
								isExplicit = true
							}
						}

						if !isExplicit {
							if t := strings.TrimSpace(result.TC.GetNodeType(n.Name)); t != "" && t != "void" {
								hints = append(hints, InlayHint{
									Position:     Position{Line: n.Name.Token.Line - 1, Character: n.Name.Token.Column + len(n.Name.Value) - 1},
									Label:        ": " + t,
									Kind:         2,
									PaddingLeft:  false,
									PaddingRight: false,
								})
							}
						}
					}
				}
			}
		case *ast.MethodCallExpression:
			if n == nil {
				return
			}
			walk(n.Object)
			for _, arg := range n.Arguments {
				if arg == nil {
					continue
				}
				walk(arg)
			}
		case *ast.ExpressionStatement:
			if n == nil {
				return
			}
			walk(n.Expression)
		case *ast.Program:
			if n == nil {
				return
			}
			for _, stmt := range n.Statements {
				if stmt != nil {
					walk(stmt)
				}
			}
		case *ast.BlockStatement:
			if n == nil {
				return
			}
			for _, stmt := range n.Statements {
				if stmt != nil {
					walk(stmt)
				}
			}
		case *ast.IfStatement:
			if n == nil {
				return
			}
			walk(n.Condition)
			walk(n.Consequence)
			walk(n.Alternative)
		case *ast.WhileStatement:
			if n == nil {
				return
			}
			walk(n.Condition)
			walk(n.Body)
		case *ast.ForStatement:
			if n == nil {
				return
			}
			walk(n.Iterable)
			walk(n.Body)
		case *ast.SwitchStatement:
			if n == nil {
				return
			}
			walk(n.Value)
			for _, c := range n.Cases {
				if c == nil {
					continue
				}
				for _, expr := range c.Values {
					if expr == nil {
						continue
					}
					walk(expr)
				}
				walk(c.Body)
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
			for _, m := range n.Methods {
				if m == nil || m.Body == nil {
					continue
				}
				walk(m.Body)
			}
		}
	}
	walk(result.AST)
	return hints
}

type exprPos struct {
	Line int
	Col  int
}

func exprStartPosition(expr ast.Expression) exprPos {
	if expr == nil {
		return exprPos{}
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		if e == nil {
			return exprPos{}
		}
		return exprPos{Line: e.Token.Line, Col: e.Token.Column}
	case *ast.IntegerLiteral:
		if e == nil {
			return exprPos{}
		}
		return exprPos{Line: e.Token.Line, Col: e.Token.Column}
	case *ast.FloatLiteral:
		if e == nil {
			return exprPos{}
		}
		return exprPos{Line: e.Token.Line, Col: e.Token.Column}
	case *ast.StringLiteral:
		if e == nil {
			return exprPos{}
		}
		return exprPos{Line: e.Token.Line, Col: e.Token.Column}
	case *ast.BooleanLiteral:
		if e == nil {
			return exprPos{}
		}
		return exprPos{Line: e.Token.Line, Col: e.Token.Column}
	case *ast.PrefixExpression:
		if e == nil {
			return exprPos{}
		}
		return exprStartPosition(e.Right)
	case *ast.InfixExpression:
		if e == nil {
			return exprPos{}
		}
		return exprStartPosition(e.Left)
	case *ast.CallExpression:
		if e == nil {
			return exprPos{}
		}
		return exprStartPosition(e.Function)
	case *ast.MethodCallExpression:
		if e == nil {
			return exprPos{}
		}
		if e.Method != nil {
			return exprPos{Line: e.Method.Token.Line, Col: e.Method.Token.Column}
		}
	}
	return exprPos{}
}

func (s *Server) handleSignatureHelp(req Request) *SignatureHelp {
	var params SignatureHelpParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	text, ok := s.Documents[params.TextDocument.URI]
	if !ok {
		return nil
	}
	result, ok := s.Cache[params.TextDocument.URI]
	if !ok || result == nil {
		return nil
	}
	return buildSignatureHelp(params.TextDocument.URI, text, params.Position, result, s)
}

func (s *Server) handleFormatting(req Request) []TextEdit {
	var params DocumentFormattingParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	text, ok := s.Documents[params.TextDocument.URI]
	if !ok {
		return nil
	}
	formatted, errs := formatter.Format(text)
	if len(errs) > 0 {
		return nil
	}
	return []TextEdit{
		{
			Range:   fullDocumentRange(text),
			NewText: formatted,
		},
	}
}

func (s *Server) handleSemanticTokensFull(req Request) *SemanticTokens {
	// Disable server-side semantic tokens. Returning an empty token
	// set forces the client to fall back to TextMate grammar + theme
	// for syntax coloring and avoids theme mismatches.
	return &SemanticTokens{Data: []int{}}
}

func (s *Server) handleInlayHint(req Request) []InlayHint {
	var params InlayHintParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic during inlayHint(%s): %v\nStack Trace:\n%s", params.TextDocument.URI, r, debug.Stack())
		}
	}()
	text, ok := s.Documents[params.TextDocument.URI]
	if !ok {
		return nil
	}
	result, ok := s.Cache[params.TextDocument.URI]
	if !ok || result == nil || result.AST == nil || result.Index == nil {
		return nil
	}
	return collectInlayHints(text, result, s)
}

var lineColRegex = regexp.MustCompile(`line (\d+):(\d+): (.*)`)

func (s *Server) analyzeAndPublish(uri string, text string) {
	filePath := uriToPath(uri)

	// 0. Update Registry
	// Ideally we should know the package path relative to workspace.
	// For now we treat single file as "main" or parse package decl.

	// 1. Parse
	l := lexer.New(text)
	p := parser.New(l)
	p.SetFilename(filePath)
	prog := p.ParseProgram()

	// 2. Type Check with Registry
	// We need to clear/reload the package in registry?
	// For now, let's create a fresh registry or update it.
	// A robust LSP needs a persistent registry.
	// But `typechecker.New()` creates a fresh environment.

	// IMPORTANT: For imports to work, we need to load them.
	// The typechecker does this if we use Check(prog).
	// But it resolves relative to current dir?
	// We need to set up the type checker to know about the file path.

	// IMPORTANT: For imports to work, we need to load them.
	// We need to set up the type checker to know about the file path.

	typechecker.InvalidatePackage(filePath)
	// log.Printf("Analyzing file: %s from URI: %s", filePath, uri)

	var tc *typechecker.TypeChecker
	// Attempt type checking even if there are parser errors to support completion
	// on partial files (e.g. "map.").
	// if len(p.Errors()) == 0 {
	{ // Always try
		tc = typechecker.NewWithPath(filePath)
		if strings.HasSuffix(filePath, "_test.bak") {
			tc.SetSuppressUnused(true)
		}
		typecheckPanicked := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					typecheckPanicked = true
					log.Printf("panic during typecheck(%s): %v\nStack Trace:\n%s", uri, r, debug.Stack())
				}
			}()
			if len(p.Errors()) == 0 {
				withSiblingPackageFiles(prog, filePath, func() {
					withPreludeForTypecheck(prog, filePath, func() {
						tc.Check(prog)
					})
				})
			}
		}()
		if typecheckPanicked {
			tc = nil
		}
	}

	comments := formatter.ScanComments(text)
	index := indexProgram(prog, uri, comments, true)
	imports := collectImports(prog)
	s.Indexes[uri] = index
	refIndex, refByPos, defs := buildReferenceIndex(prog, tc, uri, imports, index, s)

	// Update Cache
	s.Cache[uri] = &AnalysisResult{
		AST:      prog,
		TC:       tc,
		Index:    index,
		Imports:  imports,
		RefIndex: refIndex,
		RefByPos: refByPos,
		Defs:     defs,
	}

	// Collect Diagnostics
	diagnostics := []Diagnostic{}

	// Parser Errors
	for _, msg := range p.Errors() {
		matches := lineColRegex.FindStringSubmatch(msg)
		if len(matches) == 4 {
			l, _ := strconv.Atoi(matches[1]) // Line
			c, _ := strconv.Atoi(matches[2]) // Col
			m := matches[3]

			diagnostics = append(diagnostics, Diagnostic{
				Range: Range{
					Start: Position{Line: l - 1, Character: c - 1},
					End:   Position{Line: l - 1, Character: c},
				},
				Severity: 1,
				Source:   "bak-parser",
				Message:  m,
			})
		}
	}

	// Type Errors
	if tc != nil {
		typeErrors := tc.GetErrors()
		for _, typeErr := range typeErrors {
			if typeErr.File != "" && !samePath(typeErr.File, filePath) {
				continue
			}
			severity := 1
			if typeErr.Tier == typechecker.TierWarning {
				severity = 2
			}
			diagnostics = append(diagnostics, Diagnostic{
				Range: Range{
					Start: Position{Line: typeErr.Line - 1, Character: typeErr.Column - 1},
					End:   Position{Line: typeErr.Line - 1, Character: typeErr.Column},
				},
				Severity: severity,
				Source:   "bak-typechecker",
				Message:  typeErr.Message,
			})
		}
	}

	for _, finding := range linter.LintSource(filePath, text, nil) {
		diagnostics = append(diagnostics, lintFindingToDiagnostic(finding))
	}

	sort.SliceStable(diagnostics, func(i, j int) bool {
		if diagnostics[i].Range.Start.Line != diagnostics[j].Range.Start.Line {
			return diagnostics[i].Range.Start.Line < diagnostics[j].Range.Start.Line
		}
		if diagnostics[i].Range.Start.Character != diagnostics[j].Range.Start.Character {
			return diagnostics[i].Range.Start.Character < diagnostics[j].Range.Start.Character
		}
		if diagnostics[i].Severity != diagnostics[j].Severity {
			return diagnostics[i].Severity < diagnostics[j].Severity
		}
		if diagnostics[i].Source != diagnostics[j].Source {
			return diagnostics[i].Source < diagnostics[j].Source
		}
		return diagnostics[i].Message < diagnostics[j].Message
	})

	// Publish
	notification := Notification{
		BaseMessage: BaseMessage{JSONRPC: "2.0"},
		Method:      "textDocument/publishDiagnostics",
	}

	params := PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	}

	paramsBytes, _ := json.Marshal(params)
	notification.Params = paramsBytes

	msg := EncodeMessage(notification)
	os.Stdout.Write(msg)
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	if aa, err := filepath.Abs(a); err == nil {
		a = aa
	}
	if bb, err := filepath.Abs(b); err == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func lintFindingToDiagnostic(finding linter.Finding) Diagnostic {
	severity := 4
	switch strings.ToLower(finding.Level) {
	case "error":
		severity = 1
	case "warning":
		severity = 2
	case "style":
		severity = 4
	}

	line := finding.Line - 1
	if line < 0 {
		line = 0
	}
	column := finding.Column - 1
	if column < 0 {
		column = 0
	}

	return Diagnostic{
		Range: Range{
			Start: Position{Line: line, Character: column},
			End:   Position{Line: line, Character: column + 1},
		},
		Severity: severity,
		Source:   "bak-linter",
		Message:  finding.Message,
	}
}

func uriToPath(uri string) string {
	if len(uri) > 7 && uri[:7] == "file://" {
		return uri[7:]
	}
	return uri
}

func pathToURI(path string) string {
	if strings.HasPrefix(path, "file://") {
		return path
	}
	return "file://" + path
}

func (s *Server) resolveImportPath(baseFile, importPath string) string {
	root := s.RootPath
	if root == "" {
		root = filepath.Dir(baseFile)
	}
	searchPath := importPath
	if strings.HasPrefix(importPath, "std/") {
		searchPath = filepath.Join(root, "src", importPath)
	} else if strings.HasPrefix(importPath, "src/std/") {
		searchPath = filepath.Join(root, importPath)
	} else if !filepath.IsAbs(importPath) {
		searchPath = filepath.Join(root, importPath)
	}

	candidates := []string{searchPath}
	if !strings.HasSuffix(searchPath, ".bak") {
		candidates = append(candidates, searchPath+".bak")
		base := filepath.Base(searchPath)
		candidates = append(candidates, filepath.Join(searchPath, base+".bak"))
	}

	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	// Check if searchPath is a directory (multi-file package like std/http)
	if info, err := os.Stat(searchPath); err == nil && info.IsDir() {
		return searchPath
	}
	return ""
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
		if !strings.HasSuffix(d.Name(), ".bak") || strings.HasSuffix(d.Name(), "_test.bak") {
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

func indexProgram(prog *ast.Program, uri string, comments []formatter.Comment, includePrivate bool) *FileIndex {
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
				addSymbol(index, s.Name.Value, "func", uri, s.Name.Token.Line, s.Name.Token.Column, len(s.Name.Value), s.Visibility == ast.Public)
				if sig := buildFuncSignature(s.Name.Value, s.Parameters, s.ReturnType, index.Docs[s.Name.Value]); sig.Label != "" {
					index.Sigs[s.Name.Value] = sig
				}
			}
		case *ast.StructDecl:
			if s == nil || s.Name == nil {
				continue
			}
			if includePrivate || s.Visibility == ast.Public {
				addSymbol(index, s.Name.Value, "struct", uri, s.Name.Token.Line, s.Name.Token.Column, len(s.Name.Value), s.Visibility == ast.Public)
				fields := make([]string, 0, len(s.Fields))
				for _, f := range s.Fields {
					if f == nil || f.Name == nil || f.Type == nil {
						continue
					}
					// Only add public fields to the display list when not including private
					if includePrivate || f.Visibility == ast.Public {
						fields = append(fields, fmt.Sprintf("%s: %s", f.Name.Value, f.Type.String()))
						fieldName := s.Name.Value + "." + f.Name.Value
						addSymbol(index, fieldName, "field", uri, f.Name.Token.Line, f.Name.Token.Column, len(f.Name.Value), f.Visibility == ast.Public)
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
				addSymbol(index, s.Name.Value, "enum", uri, s.Name.Token.Line, s.Name.Token.Column, len(s.Name.Value), s.Visibility == ast.Public)
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
				addSymbol(index, s.Name.Value, "type", uri, s.Name.Token.Line, s.Name.Token.Column, len(s.Name.Value), s.Visibility == ast.Public)
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
				addSymbol(index, s.Name.Value, "alias", uri, s.Name.Token.Line, s.Name.Token.Column, len(s.Name.Value), s.Visibility == ast.Public)
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
			addSymbol(index, s.Name.Value, "var", uri, s.Name.Token.Line, s.Name.Token.Column, len(s.Name.Value), false)
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
				addSymbol(index, s.Name.Value, "const", uri, s.Name.Token.Line, s.Name.Token.Column, len(s.Name.Value), s.Visibility == ast.Public)
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
				addSymbol(index, name, "method", uri, m.Name.Token.Line, m.Name.Token.Column, len(m.Name.Value), m.Visibility == ast.Public)
				if sig := buildFuncSignature(name, m.Parameters, m.ReturnType, index.Docs[name]); sig.Label != "" {
					index.Sigs[name] = sig
				}
			}
		}
	}
	return index
}

func buildFuncSignature(name string, params []*ast.Parameter, ret ast.TypeExpression, doc string) SignatureInfo {
	if name == "" {
		return SignatureInfo{}
	}
	paramLabels := make([]string, 0, len(params))
	for i, p := range params {
		if p == nil {
			continue
		}
		paramName := fmt.Sprintf("arg%d", i+1)
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
		paramLabels = append(paramLabels, fmt.Sprintf("%s: %s", paramName, typ))
	}
	retType := "void"
	if ret != nil {
		retType = ret.String()
	}
	label := fmt.Sprintf("%s(%s) -> (%s)", name, strings.Join(paramLabels, ", "), retType)
	return SignatureInfo{Label: label, Params: paramLabels, Doc: doc}
}

func buildDocIndex(prog *ast.Program, comments []formatter.Comment, includePrivate bool) map[string]string {
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
			if s != nil && s.Name != nil && (includePrivate || s.Visibility == ast.Public) {
				docByLine[s.Name.Token.Line] = collectDoc(lineComment, s.Name.Token.Line)
			}
		case *ast.StructDecl:
			if s != nil && s.Name != nil && (includePrivate || s.Visibility == ast.Public) {
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

// ... existing code ...

func addSymbol(index *FileIndex, name, kind, uri string, line, col, length int, exported bool) {
	if line <= 0 || col <= 0 {
		return
	}
	index.Symbols[name] = SymbolInfo{
		Name: name,
		Kind: kind,
		Location: Location{
			URI: uri,
			Range: Range{
				Start: Position{Line: line - 1, Character: col - 1},
				End:   Position{Line: line - 1, Character: col - 1 + length},
			},
		},
		Exported: exported,
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

func lineAt(text string, line int) string {
	if line < 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	if line >= len(lines) {
		return ""
	}
	return lines[line]
}

func isPositionInComment(text string, pos Position) bool {
	offset := offsetAt(text, pos.Line, pos.Character)
	if offset < 0 {
		return false
	}
	if offset > len(text) {
		offset = len(text)
	}

	inLineComment := false
	inBlockComment := false
	inString := false
	inChar := false
	inRawString := false
	escaped := false

	for i := 0; i < offset; i++ {
		ch := text[i]
		var next byte
		if i+1 < len(text) {
			next = text[i+1]
		}

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if inChar {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '\'' {
				inChar = false
			}
			continue
		}
		if inRawString {
			if ch == '`' {
				inRawString = false
			}
			continue
		}

		switch ch {
		case '/':
			if next == '/' {
				inLineComment = true
				i++
			} else if next == '*' {
				inBlockComment = true
				i++
			}
		case '"':
			inString = true
			escaped = false
		case '\'':
			inChar = true
			escaped = false
		case '`':
			inRawString = true
		}
	}

	return inLineComment || inBlockComment
}

func buildSignatureHelp(uri string, text string, pos Position, result *AnalysisResult, s *Server) *SignatureHelp {
	lineText := lineAt(text, pos.Line)
	if lineText == "" {
		return nil
	}
	char := min(pos.Character, len(lineText))
	left := lineText[:char]
	openIdx := strings.LastIndex(left, "(")
	if openIdx == -1 {
		return nil
	}
	prefix := strings.TrimSpace(left[:openIdx])
	if prefix == "" {
		return nil
	}
	token := scanCallableToken(prefix)
	if token == "" {
		return nil
	}

	qualifier := ""
	name := token
	if parts := strings.Split(token, "."); len(parts) == 2 {
		qualifier = parts[0]
		name = parts[1]
	}

	var sig SignatureInfo
	if qualifier != "" {
		if result.Imports != nil {
			if importPath, ok := result.Imports[qualifier]; ok {
				path := s.resolveImportPath(uriToPath(uri), importPath)
				if path != "" {
					if idx := s.getOrIndexFile(path); idx != nil {
						if found, ok := idx.Sigs[name]; ok {
							sig = found
						}
					}
				}
			}
		}
		if sig.Label == "" && result.Index != nil {
			if found, ok := result.Index.Sigs[qualifier+"."+name]; ok {
				sig = found
			}
		}
	} else if result.Index != nil {
		if found, ok := result.Index.Sigs[name]; ok {
			sig = found
		}
	}

	if sig.Label == "" {
		if qualifier != "" {
			if methods, ok := builtinMethods[qualifier]; ok {
				if info, ok := methods[name]; ok {
					sig = builtinMethodSignatureInfo(qualifier, name, info)
				}
			}
			if sig.Label == "" {
				if methods, ok := builtinStaticMethods[qualifier]; ok {
					if info, ok := methods[name]; ok {
						sig = builtinMethodSignatureInfo(qualifier, name, info)
					}
				}
			}
		}
	}

	if sig.Label == "" {
		typeName, info, ok := lookupUniqueBuiltinMethod(name)
		if ok {
			sig = builtinMethodSignatureInfo(typeName, name, info)
		}
	}

	if sig.Label == "" {
		return nil
	}

	activeParam := 0
	for _, ch := range left[openIdx+1:] {
		if ch == ',' {
			activeParam++
		}
	}
	if activeParam >= len(sig.Params) {
		activeParam = max(len(sig.Params)-1, 0)
	}

	params := make([]ParameterInformation, 0, len(sig.Params))
	for _, p := range sig.Params {
		params = append(params, ParameterInformation{Label: p})
	}
	var doc *MarkupContent
	if sig.Doc != "" {
		doc = &MarkupContent{Kind: "markdown", Value: sig.Doc}
	}
	return &SignatureHelp{
		Signatures: []SignatureInformation{
			{
				Label:         sig.Label,
				Documentation: doc,
				Parameters:    params,
			},
		},
		ActiveSignature: 0,
		ActiveParameter: activeParam,
	}
}

func lookupUniqueBuiltinMethod(name string) (string, builtinMethodInfo, bool) {
	var (
		foundType string
		foundInfo builtinMethodInfo
		matches   int
	)
	for typeName, methods := range builtinMethods {
		if info, ok := methods[name]; ok {
			if strings.HasPrefix(info.Doc, "Deprecated:") {
				continue
			}
			foundType = typeName
			foundInfo = info
			matches++
		}
	}
	if matches == 1 {
		return foundType, foundInfo, true
	}
	return "", builtinMethodInfo{}, false
}

func builtinMethodSignatureInfo(typeName, methodName string, info builtinMethodInfo) SignatureInfo {
	label := info.Signature
	if after, ok := strings.CutPrefix(label, "func "); ok {
		label = after
	}
	
	label = typeName + "." + methodName
	if open := strings.Index(label, "("); open != -1 {
		label = label[open:]
	}

	return SignatureInfo{
		Label:  label,
		Params: parseSignatureParams(label),
		Doc:    info.Doc,
	}
}

func parseSignatureParams(signature string) []string {
	open := strings.Index(signature, "(")
	if open == -1 {
		return nil
	}
	close := open
	depth := 0
	for i := open; i < len(signature); i++ {
		switch signature[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				close = i
				raw := strings.TrimSpace(signature[open+1 : close])
				if raw == "" {
					return nil
				}
				return splitTopLevelParams(raw)
			}
		}
	}
	return nil
}

func splitTopLevelParams(raw string) []string {
	params := []string{}
	start := 0
	angleDepth := 0
	parenDepth := 0
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ',':
			if angleDepth == 0 && parenDepth == 0 {
				part := strings.TrimSpace(raw[start:i])
				if part != "" {
					params = append(params, part)
				}
				start = i + 1
			}
		}
	}
	last := strings.TrimSpace(raw[start:])
	if last != "" {
		params = append(params, last)
	}
	return params
}

func scanCallableToken(prefix string) string {
	i := len(prefix) - 1
	for i >= 0 {
		ch := prefix[i]
		if isIdentChar(ch) || ch == '.' {
			i--
			continue
		}
		break
	}
	token := strings.TrimSpace(prefix[i+1:])
	if strings.HasSuffix(token, ".") {
		return ""
	}
	return token
}

func isIdentChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

func qualifierAt(line string, char int) string {
	if char <= 0 || char > len(line) {
		return ""
	}
	i := char - 1
	for i >= 0 && line[i] == ' ' {
		i--
	}
	if i < 1 || line[i] != '.' {
		return ""
	}
	j := i - 1
	for j >= 0 {
		ch := line[j]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			j--
			continue
		}
		break
	}
	return line[j+1 : i]
}

func dotBefore(line string, char int) int {
	if char <= 0 || char > len(line) {
		return -1
	}
	i := char - 1
	for i >= 0 && line[i] == ' ' {
		i--
	}
	if i >= 0 && line[i] == '.' {
		return i
	}
	return -1
}

func importPathPrefix(line string, char int) (string, bool) {
	if char < 0 {
		return "", false
	}
	if char > len(line) {
		char = len(line)
	}
	before := line[:char]
	if strings.Count(before, "\"")%2 == 0 {
		return "", false
	}
	quoteIdx := strings.LastIndex(before, "\"")
	if quoteIdx == -1 {
		return "", false
	}
	beforeQuote := before[:quoteIdx]
	impIdx := strings.LastIndex(beforeQuote, "import")
	if impIdx == -1 {
		return "", false
	}
	if !isWordBoundary(beforeQuote, impIdx, len("import")) {
		return "", false
	}
	if strings.TrimSpace(beforeQuote[impIdx+len("import"):]) != "" {
		return "", false
	}
	return before[quoteIdx+1:], true
}

func isWordBoundary(s string, start, length int) bool {
	if start > 0 {
		if isIdentChar(s[start-1]) {
			return false
		}
	}
	end := start + length
	if end < len(s) {
		if isIdentChar(s[end]) {
			return false
		}
	}
	return true
}

func (s *Server) typecheckForCompletion(text, uri string, pos Position) (*typechecker.TypeChecker, *ast.Program) {
	modText := text
	lineText := lineAt(text, pos.Line)
	placeholder := "_lsp"
	if pos.Character >= 0 && pos.Character <= len(lineText) {
		if pos.Character > 0 && pos.Character <= len(lineText) && lineText[pos.Character-1] == '.' {
			offset := offsetAt(text, pos.Line, pos.Character)
			if offset >= 0 {
				modText = text[:offset] + placeholder + text[offset:]
			}
		} else if pos.Character < len(lineText) && lineText[pos.Character] == '.' {
			offset := offsetAt(text, pos.Line, pos.Character+1)
			if offset >= 0 {
				modText = text[:offset] + placeholder + text[offset:]
			}
		}
	}

	l := lexer.New(modText)
	p := parser.New(l)
	p.SetFilename(uriToPath(uri))
	prog := p.ParseProgram()
	if prog == nil {
		return nil, nil
	}

	filePath := uriToPath(uri)
	tc := typechecker.NewWithPath(filePath)
	if strings.HasSuffix(filePath, "_test.bak") {
		tc.SetSuppressUnused(true)
	}
	typecheckPanicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				typecheckPanicked = true
				log.Printf("panic during typecheck(%s): %v\nStack Trace:\n%s", uri, r, debug.Stack())
			}
		}()
		withPreludeForTypecheck(prog, filePath, func() {
			// Run typechecker with fault tolerance or partial checks
			// Ideally we use the same Check(prog) but ensure it doesn't stop on errors
			tc.Check(prog)
		})
	}()
	if typecheckPanicked {
		return nil, prog
	}
	return tc, prog
}

func offsetAt(text string, line, char int) int {
	if line < 0 || char < 0 {
		return -1
	}
	lines := strings.Split(text, "\n")
	if line >= len(lines) {
		return -1
	}
	if char > len(lines[line]) {
		char = len(lines[line])
	}
	offset := 0
	for i := range line {
		offset += len(lines[i]) + 1
	}
	return offset + char
}

func structLiteralTypeAt(text string, pos Position) string {
	offset := offsetAt(text, pos.Line, pos.Character)
	if offset < 0 {
		return ""
	}
	if offset > len(text) {
		offset = len(text)
	}
	depth := 0
	for i := offset - 1; i >= 0; i-- {
		ch := text[i]
		if ch == '}' {
			depth++
			continue
		}
		if ch != '{' {
			continue
		}
		if depth > 0 {
			depth--
			continue
		}
		j := i - 1
		for j >= 0 && (text[j] == ' ' || text[j] == '\t' || text[j] == '\n' || text[j] == '\r') {
			j--
		}
		if j < 0 {
			return ""
		}
		end := j
		for j >= 0 && (isIdentChar(text[j]) || text[j] == '.') {
			j--
		}
		name := strings.TrimSpace(text[j+1 : end+1])
		if name == "" || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
			return ""
		}
		return name
	}
	return ""
}

type localSymbol struct {
	Node   ast.Node
	Detail string
}

func collectLocalSymbols(prog *ast.Program, line int) map[string]localSymbol {
	out := make(map[string]localSymbol)
	if prog == nil {
		return out
	}
	fn := findEnclosingFunction(prog, line)
	if fn == nil {
		return out
	}
	for _, p := range fn.Parameters {
		if p != nil && p.Name != nil {
			out[p.Name.Value] = localSymbol{Node: p.Name, Detail: "param"}
		}
	}
	collectLocalSymbolsFromBlock(fn.Body, out)
	return out
}

func findEnclosingFunction(prog *ast.Program, line int) *ast.FunctionDecl {
	var best *ast.FunctionDecl
	for _, stmt := range prog.Statements {
		fn, ok := stmt.(*ast.FunctionDecl)
		if !ok || fn == nil {
			continue
		}
		if fn.Token.Line <= line {
			if best == nil || fn.Token.Line > best.Token.Line {
				best = fn
			}
		}
	}
	return best
}

func collectLocalSymbolsFromBlock(block *ast.BlockStatement, out map[string]localSymbol) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			if s != nil && s.Name != nil {
				out[s.Name.Value] = localSymbol{Node: s.Name, Detail: "var"}
			}
		case *ast.ConstStatement:
			if s != nil && s.Name != nil {
				out[s.Name.Value] = localSymbol{Node: s.Name, Detail: "const"}
			}
		case *ast.MultiVarStatement:
			if s != nil {
				for _, n := range s.Names {
					if n != nil {
						out[n.Value] = localSymbol{Node: n, Detail: "var"}
					}
				}
			}
		case *ast.ForStatement:
			if s != nil && s.Variable != nil {
				out[s.Variable.Value] = localSymbol{Node: s.Variable, Detail: "var"}
			}
			collectLocalSymbolsFromBlock(s.Body, out)
		case *ast.WhileStatement:
			if s != nil {
				collectLocalSymbolsFromBlock(s.Body, out)
			}
		case *ast.IfStatement:
			if s != nil {
				collectLocalSymbolsFromBlock(s.Consequence, out)
				collectLocalSymbolsFromBlock(s.Alternative, out)
			}
		case *ast.SwitchStatement:
			if s != nil {
				for _, c := range s.Cases {
					if c != nil {
						collectLocalSymbolsFromBlock(c.Body, out)
					}
				}
			}
		case *ast.BlockStatement:
			collectLocalSymbolsFromBlock(s, out)
		}
	}
}

func qualifierBefore(line string, dotIndex int) string {
	if dotIndex <= 0 || dotIndex > len(line) || line[dotIndex] != '.' {
		return ""
	}
	j := dotIndex - 1
	for j >= 0 {
		ch := line[j]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			j--
			continue
		}
		break
	}
	return line[j+1 : dotIndex]
}

func wordAt(line string, char int) (string, int) {
	if char < 0 {
		return "", 0
	}
	if char > len(line) {
		char = len(line)
	}
	pos := char
	if pos > 0 {
		pos--
	}
	if pos < 0 || pos >= len(line) {
		return "", 0
	}
	if !isWordChar(line[pos]) {
		return "", 0
	}
	start := pos
	for start >= 0 && isWordChar(line[start]) {
		start--
	}
	end := pos + 1
	for end < len(line) && isWordChar(line[end]) {
		end++
	}
	return line[start+1 : end], start + 1
}

func isWordChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

func fullDocumentRange(text string) Range {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}}
	}
	lastLine := len(lines) - 1
	lastCol := len(lines[lastLine])
	return Range{
		Start: Position{Line: 0, Character: 0},
		End:   Position{Line: lastLine, Character: lastCol},
	}
}

// Simple recursive node finder
func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Map || v.Kind() == reflect.Slice || v.Kind() == reflect.Chan || v.Kind() == reflect.Func {
		return v.IsNil()
	}
	return false
}

func findNode(node ast.Node, line, col int) ast.Node {
	if node == nil || isNil(node) {
		return nil
	}

	// Check identifiers explicitly
	if ident, ok := node.(*ast.Identifier); ok {
		// Simple check: start line matches, col is within start-end
		if ident.Token.Line == line {
			startCol := ident.Token.Column
			endCol := startCol + len(ident.Value)
			// Check if cursor is on the identifier
			if col >= startCol && col <= endCol {
				return ident
			}
		}
	}

	// Traverse children
	// Using type switch to traverse relevant nodes
	switch n := node.(type) {
	case *ast.SimpleType:
		if n.Token.Line == line {
			startCol := n.Token.Column
			endCol := startCol + len(n.Name)
			if col >= startCol && col <= endCol {
				return n
			}
		}
	case *ast.GenericType:
		if n.Token.Line == line {
			startCol := n.Token.Column
			endCol := startCol + len(n.Name)
			if col >= startCol && col <= endCol {
				return n
			}
		}
		for _, p := range n.TypeParams {
			if f := findNode(p, line, col); f != nil {
				return f
			}
		}
	case *ast.BorrowType:
		return findNode(n.Inner, line, col)
	case *ast.BoxType:
		return findNode(n.Inner, line, col)
	case *ast.BoxOptionalType:
		return findNode(n.Inner, line, col)
	case *ast.ArrayType:
		if f := findNode(n.ElemType, line, col); f != nil {
			return f
		}
	case *ast.TupleType:
		for _, el := range n.Elements {
			if f := findNode(el, line, col); f != nil {
				return f
			}
		}
	case *ast.FunctionType:
		for _, p := range n.Params {
			if f := findNode(p, line, col); f != nil {
				return f
			}
		}
		if f := findNode(n.ReturnType, line, col); f != nil {
			return f
		}
	case *ast.NamedType:
		if f := findNode(n.Type, line, col); f != nil {
			return f
		}
	case *ast.Program:
		for _, stmt := range n.Statements {
			if found := findNode(stmt, line, col); found != nil {
				return found
			}
		}
	case *ast.ImportStatement:
		return nil
	case *ast.ExpressionStatement:
		return findNode(n.Expression, line, col)
	case *ast.VarStatement:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		if f := findNode(n.Type, line, col); f != nil {
			return f
		}
		if f := findNode(n.Value, line, col); f != nil {
			return f
		}
	case *ast.ConstStatement:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		if f := findNode(n.Type, line, col); f != nil {
			return f
		}
		if f := findNode(n.Value, line, col); f != nil {
			return f
		}
	case *ast.MultiVarStatement:
		for _, name := range n.Names {
			if f := findNode(name, line, col); f != nil {
				return f
			}
		}
		for _, typ := range n.Types {
			if f := findNode(typ, line, col); f != nil {
				return f
			}
		}
		if f := findNode(n.Value, line, col); f != nil {
			return f
		}
	case *ast.VarBlock:
		for _, v := range n.Variables {
			if f := findNode(v, line, col); f != nil {
				return f
			}
		}
	case *ast.ConstBlock:
		for _, c := range n.Constants {
			if f := findNode(c, line, col); f != nil {
				return f
			}
		}
	case *ast.InfixExpression:
		if f := findNode(n.Left, line, col); f != nil {
			return f
		}
		if f := findNode(n.Right, line, col); f != nil {
			return f
		}
	case *ast.PrefixExpression:
		return findNode(n.Right, line, col)
	case *ast.CallExpression:
		if f := findNode(n.Function, line, col); f != nil {
			return f
		}
		for _, arg := range n.Arguments {
			if f := findNode(arg, line, col); f != nil {
				return f
			}
		}
	case *ast.FunctionLiteral:
		if f := findNode(n.Body, line, col); f != nil {
			return f
		}
	case *ast.MethodCallExpression:
		if n.Method != nil && n.Method.Token.Line == line {
			startCol := n.Method.Token.Column
			endCol := startCol + len(n.Method.Value)
			if col >= startCol && col <= endCol {
				return n
			}
		}
		if f := findNode(n.Object, line, col); f != nil {
			return f
		}
		for _, arg := range n.Arguments {
			if f := findNode(arg, line, col); f != nil {
				return f
			}
		}
	case *ast.FieldAccessExpression:
		if n.Field != nil && n.Field.Token.Line == line {
			startCol := n.Field.Token.Column
			endCol := startCol + len(n.Field.Value)
			if col >= startCol && col <= endCol {
				return n
			}
		}
		if f := findNode(n.Object, line, col); f != nil {
			return f
		}
	case *ast.IndexExpression:
		if f := findNode(n.Left, line, col); f != nil {
			return f
		}
		if f := findNode(n.Index, line, col); f != nil {
			return f
		}
	case *ast.FunctionDecl:
		// Traverse name, params, body
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		for _, p := range n.TypeParams {
			if f := findNode(p, line, col); f != nil {
				return f
			}
		}
		for _, p := range n.Parameters {
			if f := findNode(p.Name, line, col); f != nil {
				return f
			}
			if f := findNode(p.Type, line, col); f != nil {
				return f
			}
		}
		if f := findNode(n.ReturnType, line, col); f != nil {
			return f
		}
		if f := findNode(n.Body, line, col); f != nil {
			return f
		}
	case *ast.ImplDecl:
		if f := findNode(n.TypeName, line, col); f != nil {
			return f
		}
		for _, p := range n.TypeParams {
			if f := findNode(p, line, col); f != nil {
				return f
			}
		}
		if f := findNode(n.Receiver, line, col); f != nil {
			return f
		}
		for _, m := range n.Methods {
			if m == nil {
				continue
			}
			if f := findNode(m.Name, line, col); f != nil {
				return f
			}
			for _, p := range m.TypeParams {
				if f := findNode(p, line, col); f != nil {
					return f
				}
			}
			for _, p := range m.Parameters {
				if p != nil && p.Name != nil {
					if f := findNode(p.Name, line, col); f != nil {
						return f
					}
				}
				if p != nil && p.Type != nil {
					if f := findNode(p.Type, line, col); f != nil {
						return f
					}
				}
			}
			if f := findNode(m.ReturnType, line, col); f != nil {
				return f
			}
			if f := findNode(m.Body, line, col); f != nil {
				return f
			}
		}
	case *ast.StructDecl:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		for _, p := range n.TypeParams {
			if f := findNode(p, line, col); f != nil {
				return f
			}
		}
		for _, f := range n.Fields {
			if f := findNode(f.Name, line, col); f != nil {
				return f
			}
			if f := findNode(f.Type, line, col); f != nil {
				return f
			}
		}
	case *ast.TypeDecl:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		if f := findNode(n.Underlying, line, col); f != nil {
			return f
		}
	case *ast.AliasDecl:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		if f := findNode(n.Underlying, line, col); f != nil {
			return f
		}
	case *ast.EnumDecl:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		for _, v := range n.Variants {
			if f := findNode(v.Name, line, col); f != nil {
				return f
			}
		}
	case *ast.BlockStatement:
		for _, stmt := range n.Statements {
			if f := findNode(stmt, line, col); f != nil {
				return f
			}
		}
	case *ast.IfStatement:
		if f := findNode(n.Condition, line, col); f != nil {
			return f
		}
		if f := findNode(n.Consequence, line, col); f != nil {
			return f
		}
		if f := findNode(n.Alternative, line, col); f != nil {
			return f
		}
	case *ast.ReturnStatement:
		return findNode(n.ReturnValue, line, col)
	case *ast.ForStatement:
		if f := findNode(n.Variable, line, col); f != nil {
			return f
		}
		if f := findNode(n.Iterable, line, col); f != nil {
			return f
		}
		if f := findNode(n.Body, line, col); f != nil {
			return f
		}
	case *ast.WhileStatement:
		if f := findNode(n.Condition, line, col); f != nil {
			return f
		}
		if f := findNode(n.Body, line, col); f != nil {
			return f
		}
	case *ast.SwitchStatement:
		if f := findNode(n.Value, line, col); f != nil {
			return f
		}
		for _, c := range n.Cases {
			for _, val := range c.Values {
				if f := findNode(val, line, col); f != nil {
					return f
				}
			}
			if f := findNode(c.Body, line, col); f != nil {
				return f
			}
		}
	case *ast.StructLiteral:
		if n.Name != nil && n.Name.Token.Line == line {
			startCol := n.Name.Token.Column
			endCol := startCol + len(n.Name.Value)
			if col >= startCol && col <= endCol {
				return n.Name
			}
		}
		for _, v := range n.Fields {
			if f := findNode(v, line, col); f != nil {
				return f
			}
		}
	case *ast.VecLiteral:
		for _, el := range n.Elements {
			if f := findNode(el, line, col); f != nil {
				return f
			}
		}
	}

	return nil
}

func findStructLiteralAt(node ast.Node, line, col int) *ast.StructLiteral {
	if node == nil || isNil(node) {
		return nil
	}
	switch n := node.(type) {
	case *ast.StructLiteral:
		if !positionInSpan(n.Span, line, col) {
			return nil
		}
		for _, v := range n.Fields {
			if f := findStructLiteralAt(v, line, col); f != nil {
				return f
			}
		}
		return n
	case *ast.Program:
		for _, stmt := range n.Statements {
			if f := findStructLiteralAt(stmt, line, col); f != nil {
				return f
			}
		}
	case *ast.ExpressionStatement:
		return findStructLiteralAt(n.Expression, line, col)
	case *ast.VarStatement:
		if f := findStructLiteralAt(n.Value, line, col); f != nil {
			return f
		}
	case *ast.ConstStatement:
		if f := findStructLiteralAt(n.Value, line, col); f != nil {
			return f
		}
	case *ast.MultiVarStatement:
		if f := findStructLiteralAt(n.Value, line, col); f != nil {
			return f
		}
	case *ast.VarBlock:
		for _, v := range n.Variables {
			if f := findStructLiteralAt(v, line, col); f != nil {
				return f
			}
		}
	case *ast.ConstBlock:
		for _, c := range n.Constants {
			if f := findStructLiteralAt(c, line, col); f != nil {
				return f
			}
		}
	case *ast.AssignmentStatement:
		if f := findStructLiteralAt(n.Left, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Value, line, col); f != nil {
			return f
		}
	case *ast.InfixExpression:
		if f := findStructLiteralAt(n.Left, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Right, line, col); f != nil {
			return f
		}
	case *ast.PrefixExpression:
		return findStructLiteralAt(n.Right, line, col)
	case *ast.CallExpression:
		if f := findStructLiteralAt(n.Function, line, col); f != nil {
			return f
		}
		for _, arg := range n.Arguments {
			if f := findStructLiteralAt(arg, line, col); f != nil {
				return f
			}
		}
	case *ast.TypeConversion:
		return findStructLiteralAt(n.Value, line, col)
	case *ast.MethodCallExpression:
		if f := findStructLiteralAt(n.Object, line, col); f != nil {
			return f
		}
		for _, arg := range n.Arguments {
			if f := findStructLiteralAt(arg, line, col); f != nil {
				return f
			}
		}
	case *ast.FieldAccessExpression:
		return findStructLiteralAt(n.Object, line, col)
	case *ast.IndexExpression:
		if f := findStructLiteralAt(n.Left, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Index, line, col); f != nil {
			return f
		}
	case *ast.TupleExpression:
		for _, el := range n.Elements {
			if f := findStructLiteralAt(el, line, col); f != nil {
				return f
			}
		}
	case *ast.RangeExpression:
		if f := findStructLiteralAt(n.Start, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.End, line, col); f != nil {
			return f
		}
	case *ast.EnumVariantExpression:
		for _, v := range n.Values {
			if f := findStructLiteralAt(v, line, col); f != nil {
				return f
			}
		}
	case *ast.BorrowExpression:
		return findStructLiteralAt(n.Value, line, col)
	case *ast.BoxExpression:
		return findStructLiteralAt(n.Value, line, col)
	case *ast.DerefExpression:
		return findStructLiteralAt(n.Value, line, col)
	case *ast.FunctionLiteral:
		if f := findStructLiteralAt(n.Body, line, col); f != nil {
			return f
		}
	case *ast.FunctionDecl:
		if f := findStructLiteralAt(n.Body, line, col); f != nil {
			return f
		}
	case *ast.ImplDecl:
		for _, m := range n.Methods {
			if m == nil {
				continue
			}
			if f := findStructLiteralAt(m.Body, line, col); f != nil {
				return f
			}
		}
	case *ast.BlockStatement:
		for _, stmt := range n.Statements {
			if f := findStructLiteralAt(stmt, line, col); f != nil {
				return f
			}
		}
	case *ast.IfStatement:
		if f := findStructLiteralAt(n.Condition, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Consequence, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Alternative, line, col); f != nil {
			return f
		}
	case *ast.ReturnStatement:
		return findStructLiteralAt(n.ReturnValue, line, col)
	case *ast.ForStatement:
		if f := findStructLiteralAt(n.Iterable, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Body, line, col); f != nil {
			return f
		}
	case *ast.WhileStatement:
		if f := findStructLiteralAt(n.Condition, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Body, line, col); f != nil {
			return f
		}
	case *ast.SwitchStatement:
		if f := findStructLiteralAt(n.Value, line, col); f != nil {
			return f
		}
		for _, c := range n.Cases {
			for _, val := range c.Values {
				if f := findStructLiteralAt(val, line, col); f != nil {
					return f
				}
			}
			if f := findStructLiteralAt(c.Body, line, col); f != nil {
				return f
			}
		}
	case *ast.DeferStatement:
		if f := findStructLiteralAt(n.Body, line, col); f != nil {
			return f
		}
	case *ast.PanicStatement:
		return findStructLiteralAt(n.Message, line, col)
	case *ast.UnsafeBlock:
		return findStructLiteralAt(n.Body, line, col)
	case *ast.VecLiteral:
		for _, el := range n.Elements {
			if f := findStructLiteralAt(el, line, col); f != nil {
				return f
			}
		}
	}
	return nil
}

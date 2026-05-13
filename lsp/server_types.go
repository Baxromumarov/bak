package main

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/linter"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

// builtinMethodInfo holds signature and documentation for a built-in method.
type builtinMethodInfo struct {
	Signature string
	Doc       string
}

//go:generate go run ./tools/gen_builtin_methods -in builtin_methods.json -out builtin_methods_gen.go

type AnalysisResult struct {
	AST      *ast.Program
	TC       *typechecker.TypeChecker
	Index    *FileIndex
	Imports  map[string]string
	RefIndex map[string][]Location
	RefByPos map[string]string
	Defs     map[string]Location
	Graph    []packages.GraphNode
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
	stateMu        sync.RWMutex
	Documents      map[string]string
	Cache          map[string]*AnalysisResult
	Indexes        map[string]*FileIndex
	PublicIndexes  map[string]*FileIndex // Cache for external module indexes (public symbols only)
	ReverseDeps    map[string]map[string]struct{}
	RootPath       string
	stdImportPaths []string
	stdPackages    []string
	pendingLocks   map[string]*time.Timer
	pendingCancel  map[string]context.CancelFunc
	canceled       map[string]struct{}
	activeRequests map[string]context.CancelFunc
	workspaceTimer *time.Timer
	watchedChanges map[string]struct{}
	lintConfig     *linter.Config
	outputMu       sync.Mutex
	output         io.Writer
}

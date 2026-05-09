package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

type TypeInfo struct {
	Type       ast.TypeExpression
	Mutable    bool
	Visibility ast.Visibility
	Line       int
	Column     int
}

// FunctionSig represents a function signature
type FunctionSig struct {
	TypeParams  []string
	Parameters  []ast.TypeExpression
	ReturnType  ast.TypeExpression
	Package     string // Package name
	PackagePath string // Package path
	Line        int
	Column      int
	Visibility  ast.Visibility
	Mutable     bool
	IsInstance  bool
}

// FieldDef represents a field in a struct for type checking
type StructDef struct {
	Fields      map[string]FieldDef
	Methods     map[string]*FunctionSig
	TypeParams  []string // Generic type parameter names
	Package     string   // The package name where this struct is defined
	PackagePath string   // The package path where this struct is defined
	Line        int
	Column      int
	Visibility  ast.Visibility
}

// EnumVariantDef represents a variant in an enum
type EnumVariantDef struct {
	HasPayload bool
	Fields     []ast.TypeExpression
}

// EnumDef represents an enum definition for type checking
type EnumDef struct {
	Variants    map[string]EnumVariantDef // map of variant name -> variant info
	Visibility  ast.Visibility
	Package     string
	PackagePath string
	Line        int
	Column      int
}

// AliasDef represents a type alias for type checking
type AliasDef struct {
	Type       ast.TypeExpression
	Visibility ast.Visibility
	Line       int
	Column     int
}

// TypeDef represents a type definition for type checking
type TypeDef struct {
	Type       ast.TypeExpression
	Visibility ast.Visibility
	Line       int
	Column     int
}

// ImportInfo represents an import declaration for unused import checks.
type ImportInfo struct {
	Path   string
	Alias  string
	File   string
	Line   int
	Column int
}

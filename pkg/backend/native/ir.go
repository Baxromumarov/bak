package native

import (
	"sort"
	"strings"

	"reflect"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// IRProgramSet is a minimal, typed intermediate representation scaffold.
// It intentionally starts small so native codegen can migrate incrementally.
type IRProgramSet struct {
	Modules []IRModule
}

type IRModule struct {
	Name      string
	Source    string
	Functions []IRFunction
}

type IRFunction struct {
	Name       string
	ParamCount int
	ReturnType string
}

// IRMemKind is a typed memory operation. Native code generation selects the
// machine instruction from this kind. It must not inspect the runtime word
// to guess whether that word is a pointer.
type IRMemKind string

const (
	IRLoadValue          IRMemKind = "LoadValue"
	IRLoadBorrow         IRMemKind = "LoadBorrow"
	IRDerefBorrow        IRMemKind = "DerefBorrow"
	IRStoreThroughBorrow IRMemKind = "StoreThroughBorrow"
)

// IRMemOp is one typed lowering of a place access.
// Place names the local or the register that already holds the place.
// Disp is the byte offset used by LoadValue when the place is a heap field.
type IRMemOp struct {
	Kind  IRMemKind
	Place string
	Disp  int
}

// BuildIRProgramSet lowers parsed modules into a stable metadata IR.
// The current native backend still emits code directly from AST; this IR
// provides a migration point for future SSA-like lowering.
func BuildIRProgramSet(programs []ProgramWithPath) *IRProgramSet {
	set := &IRProgramSet{Modules: make([]IRModule, 0, len(programs))}
	for _, programWithPath := range programs {
		module := IRModule{
			Name:      programWithPath.PathName,
			Source:    "",
			Functions: make([]IRFunction, 0),
		}
		if programWithPath.Program != nil {
			module.Source = programWithPath.Program.SourcePath
			for _, stmt := range programWithPath.Program.Statements {
				fn, ok := stmt.(*ast.FunctionDecl)
				if !ok {
					continue
				}
				module.Functions = append(module.Functions, IRFunction{
					Name:       fn.Name.Value,
					ParamCount: len(fn.Parameters),
					ReturnType: renderIRType(fn.ReturnType),
				})
			}
		}
		sort.Slice(module.Functions, func(i, j int) bool {
			return module.Functions[i].Name < module.Functions[j].Name
		})
		set.Modules = append(set.Modules, module)
	}
	sort.Slice(set.Modules, func(i, j int) bool {
		return set.Modules[i].Name < set.Modules[j].Name
	})
	return set
}

func renderIRType(t ast.TypeExpression) string {
	if t == nil {
		return "void"
	}
	switch tt := t.(type) {
	case *ast.VoidType:
		return "void"
	case *ast.SimpleType:
		return tt.Name
	case *ast.GenericType:
		if len(tt.TypeParams) == 0 {
			return tt.Name
		}
		parts := make([]string, 0, len(tt.TypeParams))
		for _, param := range tt.TypeParams {
			parts = append(parts, renderIRType(param))
		}
		return strfmt.Named(
			"{Name}<{Parts}>",
			"Name", tt.Name,
			"Parts", strings.Join(parts, ","),
		)
	default:
		return strfmt.Named(
			"{t}",
			"T", reflect.TypeOf(t).String(),
		)
	}
}

package native

import sharedir "github.com/baxromumarov/bak/pkg/ir"

// Compatibility aliases keep the native inspection API stable while making
// the representation shared by both execution backends.
type IRProgramSet = sharedir.Program
type IRModule = sharedir.Module
type IRFunction = sharedir.Function
type Parameter = sharedir.Parameter

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

func BuildIRProgramSet(programs []ProgramWithPath) *IRProgramSet {
	set := &sharedir.Program{Modules: make([]sharedir.Module, 0, len(programs))}
	for _, p := range programs {
		set.Modules = append(set.Modules, sharedir.LowerModule(p.PathName, p.Program, nil))
	}
	return set
}

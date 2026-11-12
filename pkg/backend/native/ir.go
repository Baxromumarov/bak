package native

import sharedir "github.com/baxromumarov/bak/pkg/ir"

// Compatibility aliases keep the native inspection API stable while making
// the representation shared by both execution backends.
type IRProgramSet = sharedir.Program
type IRModule = sharedir.Module
type IRFunction = sharedir.Function
type Parameter = sharedir.Parameter

func BuildIRProgramSet(programs []ProgramWithPath) *IRProgramSet {
	set := &sharedir.Program{Modules: make([]sharedir.Module, 0, len(programs))}
	for _, p := range programs {
		set.Modules = append(set.Modules, sharedir.LowerModule(p.PathName, p.Program, nil))
	}
	return set
}

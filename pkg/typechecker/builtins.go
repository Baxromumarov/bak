package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/compiler"
)

type builtinCallSpec struct {
	Signature     *ast.FunctionType
	MinArgs       int
	MaxArgs       int // -1 means unbounded
	CheckArgTypes bool
}

func (s builtinCallSpec) acceptsArgCount(got int) bool {
	if got < s.MinArgs {
		return false
	}
	if s.MaxArgs >= 0 && got > s.MaxArgs {
		return false
	}
	return true
}

func buildBuiltinCallSpec(sig *ast.FunctionType) builtinCallSpec {
	if sig == nil || sig.Params == nil {
		return builtinCallSpec{
			Signature:     sig,
			MinArgs:       0,
			MaxArgs:       -1,
			CheckArgTypes: false,
		}
	}

	n := len(sig.Params)

	return builtinCallSpec{
		Signature:     sig,
		MinArgs:       n,
		MaxArgs:       n,
		CheckArgTypes: true,
	}
}

func (tc *TypeChecker) isBuiltin(name string) bool {
	if name == "Vec" {
		return true
	}

	if _, ok := compiler.LookupBuiltinID(name); ok {
		return true
	}

	return strings.HasPrefix(name, "__builtin_")
}

func (tc *TypeChecker) getBuiltinCallSpec(name string) (builtinCallSpec, bool) {
	sig := tc.getBuiltinType(name)
	if sig == nil {
		return builtinCallSpec{}, false
	}

	spec := buildBuiltinCallSpec(sig)
	if contract, ok := compiler.BuiltinContractByName(name); ok {
		spec.MinArgs = contract.MinArgs
		spec.MaxArgs = contract.MaxArgs
		spec.CheckArgTypes = contract.CheckArgTypes && sig.Params != nil
	}

	return spec, true
}

func (tc *TypeChecker) getBuiltinType(name string) *ast.FunctionType {
	if sig := compiler.BuiltinSignatureByName(name); sig != nil {
		return sig
	}

	return &ast.FunctionType{
		Params:     nil,
		ReturnType: &ast.VoidType{},
	}
}

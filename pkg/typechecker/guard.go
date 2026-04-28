// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
)

type resultGuardState int

const (
	resultGuardUnknown resultGuardState = iota
	resultGuardIsOk
	resultGuardIsErr
)

func invertResultGuardState(state resultGuardState) resultGuardState {
	switch state {
	case resultGuardIsOk:
		return resultGuardIsErr
	case resultGuardIsErr:
		return resultGuardIsOk
	default:
		return resultGuardUnknown
	}
}

func (tc *TypeChecker) withResultGuardFact(name string, state resultGuardState, fn func()) {
	if name == "" || state == resultGuardUnknown {
		fn()
		return
	}
	prev, hadPrev := tc.resultGuardFacts[name]
	tc.resultGuardFacts[name] = state
	defer func() {
		if hadPrev {
			tc.resultGuardFacts[name] = prev
		} else {
			delete(tc.resultGuardFacts, name)
		}
	}()
	fn()
}

func (tc *TypeChecker) resultGuardStateFor(name string) resultGuardState {
	if name == "" {
		return resultGuardUnknown
	}
	if state, ok := tc.resultGuardFacts[name]; ok {
		return state
	}
	return resultGuardUnknown
}

func (tc *TypeChecker) resultGuardVariableFromExpr(expr ast.Expression) (string, bool) {
	switch e := expr.(type) {
	case *ast.Identifier:
		return e.Value, true
	case *ast.MutableIdentifier:
		return e.Value, true
	default:
		return "", false
	}
}

func (tc *TypeChecker) detectResultGuardCondition(expr ast.Expression) (string, resultGuardState, bool) {
	if expr == nil {
		return "", resultGuardUnknown, false
	}
	if pe, ok := expr.(*ast.PrefixExpression); ok && pe.Operator == "!" {
		name, state, ok := tc.detectResultGuardCondition(pe.Right)
		if !ok {
			return "", resultGuardUnknown, false
		}
		return name, invertResultGuardState(state), true
	}
	mc, ok := expr.(*ast.MethodCallExpression)
	if !ok {
		return "", resultGuardUnknown, false
	}
	if len(mc.Arguments) != 0 {
		return "", resultGuardUnknown, false
	}
	var state resultGuardState
	switch mc.Method.Value {
	case "isOk":
		state = resultGuardIsOk
	case "isErr":
		state = resultGuardIsErr
	default:
		return "", resultGuardUnknown, false
	}
	name, ok := tc.resultGuardVariableFromExpr(mc.Object)
	if !ok {
		return "", resultGuardUnknown, false
	}
	if info, ok := tc.env.LookupSymbol(name); ok && info.Type != nil {
		resolved := tc.resolveType(info.Type)
		if gt, ok := resolved.(*ast.GenericType); !ok || gt.Name != "Result" {
			return "", resultGuardUnknown, false
		}
	} else {
		return "", resultGuardUnknown, false
	}
	return name, state, true
}

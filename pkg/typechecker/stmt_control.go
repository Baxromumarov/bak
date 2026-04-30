package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (tc *TypeChecker) checkIfStatement(is *ast.IfStatement) {
	condType := tc.inferType(is.Condition)
	if condType != nil && !tc.isBoolType(condType) {
		tc.addError(is.Token.Line, is.Token.Column, strfmt.Named("if condition must be bool, got {typeToString}", "TypeToString", typeToString(condType)))
	}

	guardVar, guardState, hasGuard := tc.detectResultGuardCondition(is.Condition)

	// Check if branches terminate (contain unconditional return).
	// If a branch terminates, its moves shouldn't propagate to subsequent code.
	conseqTerminates := tc.blockTerminates(is.Consequence)
	checkWithGuard := func(name string, state resultGuardState, branch func()) {
		if !hasGuard {
			branch()
			return
		}
		tc.withResultGuardFact(name, state, branch)
	}

	if conseqTerminates {
		// Create a scoped environment for the consequence branch
		// so its moves don't leak to subsequent code
		conseqEnv := NewIsolatedTypeEnv(tc.env)
		oldEnv := tc.env
		tc.env = conseqEnv
		checkWithGuard(guardVar, guardState, func() {
			tc.checkBlockStatement(is.Consequence)
		})
		tc.env = oldEnv
	} else {
		checkWithGuard(guardVar, guardState, func() {
			tc.checkBlockStatement(is.Consequence)
		})
	}

	if is.Alternative != nil {
		altTerminates := tc.blockTerminates(is.Alternative)
		if altTerminates {
			altEnv := NewIsolatedTypeEnv(tc.env)
			oldEnv := tc.env
			tc.env = altEnv
			checkWithGuard(guardVar, invertResultGuardState(guardState), func() {
				tc.checkBlockStatement(is.Alternative)
			})
			tc.env = oldEnv
		} else {
			checkWithGuard(guardVar, invertResultGuardState(guardState), func() {
				tc.checkBlockStatement(is.Alternative)
			})
		}
	}
}

func (tc *TypeChecker) checkWhileStatement(ws *ast.WhileStatement) {
	condType := tc.inferType(ws.Condition)
	if condType != nil && !tc.isBoolType(condType) {
		tc.addError(ws.Token.Line, ws.Token.Column, strfmt.Named("while condition must be bool, got {typeToString}", "TypeToString", typeToString(condType)))
	}

	// Use an isolated environment for the while body.
	// Loop bodies may have returns, and moves inside the loop shouldn't
	// leak to code after the loop (since the loop may not execute at all
	// or may execute multiple times).
	loopEnv := NewIsolatedTypeEnv(tc.env)
	oldEnv := tc.env
	tc.env = loopEnv
	tc.checkBlockStatement(ws.Body)
	tc.env = oldEnv
}

func (tc *TypeChecker) checkForStatement(fs *ast.ForStatement) {
	// Check for common mistake: iterating over [start, end] Vec literal instead of range
	if vecLit, ok := fs.Iterable.(*ast.VecLiteral); ok && len(vecLit.Elements) == 2 {
		tc.emitter.Emit(diagnostics.Diagnostic{
			Code:    diagnostics.DiagnosticCode("AmbiguousRange"),
			Level:   diagnostics.LevelWarning,
			Message: "iterating over a 2-element vector; did you mean to use a range 'start..end'?",
			Line:    vecLit.Token.Line,
			Column:  vecLit.Token.Column,
			File:    tc.currentPkgPath,
		})
	}

	iterType := tc.inferType(fs.Iterable)
	elemType, ok := tc.iterableElementType(iterType)
	if iterType != nil && !ok {
		tc.addError(fs.Token.Line, fs.Token.Column, "for loop requires a vector, string, or range iterable")
	}

	loopEnv := NewEnclosedTypeEnv(tc.env)
	oldEnv := tc.env
	tc.env = loopEnv
	tc.env.DefineSymbolAt(fs.Variable.Value, elemType, false, ast.Private, tokenPos(fs.Variable.Token))
	if fs.Variable != nil {
		tc.nodeTypes[fs.Variable] = typeToString(elemType)
	}

	tc.checkBlockStatement(fs.Body)
	tc.env = oldEnv
}

func (tc *TypeChecker) checkUnsafeBlock(ub *ast.UnsafeBlock) {
	if !tc.experimentalFeatureEnabled(runtimecap.ExperimentalFeatureUnsafe) {
		tc.addExperimentalFeatureError(
			tokenPos(ub.Token),
			"`unsafe` blocks",
			runtimecap.ExperimentalFeatureUnsafe,
		)
	}
	if ub.Body != nil {
		tc.checkBlockStatement(ub.Body)
	}
}

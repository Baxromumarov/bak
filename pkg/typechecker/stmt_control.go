package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (tc *TypeChecker) checkIfStatement(is *ast.IfStatement) {
	condType := tc.inferType(is.Condition)
	if condType != nil && !tc.isBoolType(condType) {
		tc.addError(is.Token.Line, is.Token.Column, strfmt.Named("if condition must be bool, got {typeToString}", "TypeToString", typeToString(condType)))
	}

	guardVar, guardState, hasGuard := tc.detectResultGuardCondition(is.Condition)

	// Every arm starts from the same incoming ownership state. Facts from arms
	// that can reach the join are merged as "may move" facts.
	parentEnv := tc.env
	var reaching []ownershipBranch
	checkWithGuard := func(name string, state resultGuardState, branch func()) {
		if !hasGuard {
			branch()
			return
		}
		tc.withResultGuardFact(name, state, branch)
	}

	conseqEnv := NewIsolatedTypeEnv(parentEnv)
	tc.env = conseqEnv
	checkWithGuard(guardVar, guardState, func() {
		tc.checkBlockStatement(is.Consequence)
	})
	if !tc.blockTerminates(is.Consequence) {
		reaching = append(reaching, captureOwnershipBranch(conseqEnv, parentEnv))
	}
	tc.env = parentEnv

	if is.Alternative != nil {
		altEnv := NewIsolatedTypeEnv(parentEnv)
		tc.env = altEnv
		checkWithGuard(guardVar, invertResultGuardState(guardState), func() {
			tc.checkBlockStatement(is.Alternative)
		})
		if !tc.blockTerminates(is.Alternative) {
			reaching = append(reaching, captureOwnershipBranch(altEnv, parentEnv))
		}
		tc.env = parentEnv
	}
	mergeOwnershipBranches(parentEnv, reaching...)
}

func (tc *TypeChecker) checkWhileStatement(ws *ast.WhileStatement) {
	condType := tc.inferType(ws.Condition)
	if condType != nil && !tc.isBoolType(condType) {
		tc.addError(ws.Token.Line, ws.Token.Column, strfmt.Named("while condition must be bool, got {typeToString}", "TypeToString", typeToString(condType)))
	}

	oldEnv := tc.env
	loopEnv := NewIsolatedTypeEnv(oldEnv)
	tc.env = loopEnv
	tc.checkBlockStatement(ws.Body)
	tc.env = oldEnv
	if !tc.blockTerminates(ws.Body) {
		mergeOwnershipBranches(oldEnv, captureOwnershipBranch(loopEnv, oldEnv))
	}
}

func (tc *TypeChecker) checkForStatement(fs *ast.ForStatement) {
	// Check for common mistake: iterating over [start, end] Vec literal instead of range
	if vecLit, ok := fs.Iterable.(*ast.VecLiteral); ok && len(vecLit.Elements) == 2 {
		tc.emitter.Emit(diagnostics.Diagnostic{
			Code:    diagnostics.WarnAmbiguousRange,
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

	oldEnv := tc.env
	loopEnv := NewIsolatedTypeEnv(oldEnv)
	tc.env = loopEnv
	tc.env.DefineSymbolAt(fs.Variable.Value, elemType, false, ast.Private, fs.Variable.Pos())
	if fs.Variable != nil {
		tc.nodeTypes[fs.Variable] = typeToString(elemType)
	}

	tc.checkBlockStatement(fs.Body)
	tc.env = oldEnv
	if !tc.blockTerminates(fs.Body) {
		mergeOwnershipBranches(oldEnv, captureOwnershipBranch(loopEnv, oldEnv))
	}
}

func (tc *TypeChecker) checkUnsafeBlock(ub *ast.UnsafeBlock) {
	if ub.Body != nil {
		tc.checkBlockStatement(ub.Body)
	}
}

package compiler

import (
	"slices"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

// EscapeReason explains why a local value must outlive its lexical stack scope.
type EscapeReason string

const (
	EscapeReturnedValue     EscapeReason = "returned_value"
	EscapeReturnedReference EscapeReason = "returned_reference"
	EscapeAssignedToGlobal  EscapeReason = "assigned_to_global"
	EscapeStoredExternally  EscapeReason = "stored_externally"
	EscapeCapturedByClosure EscapeReason = "captured_by_closure"
	EscapePassedToCall      EscapeReason = "passed_to_call"
	EscapeStoredInAggregate EscapeReason = "stored_in_aggregate"
)

// LocalEscape records escape reasons for a single local binding.
type LocalEscape struct {
	Name    string
	Reasons map[EscapeReason]struct{}
}

func (le *LocalEscape) add(reason EscapeReason) {
	if le == nil {
		return
	}
	if le.Reasons == nil {
		le.Reasons = make(map[EscapeReason]struct{})
	}
	le.Reasons[reason] = struct{}{}
}

// Has reports whether this local has at least one escape reason.
func (le *LocalEscape) Has() bool {
	return le != nil && len(le.Reasons) > 0
}

// SortedReasons returns stable reason ordering for diagnostics/debugging.
func (le *LocalEscape) SortedReasons() []EscapeReason {
	if le == nil || len(le.Reasons) == 0 {
		return nil
	}
	out := make([]EscapeReason, 0, len(le.Reasons))
	for reason := range le.Reasons {
		out = append(out, reason)
	}
	slices.Sort(out)
	return out
}

// FunctionEscapeSummary contains escape findings for one function/method/literal.
type FunctionEscapeSummary struct {
	FunctionName string
	Locals       map[string]*LocalEscape
}

func (s *FunctionEscapeSummary) ensureLocal(name string) *LocalEscape {
	if s == nil {
		return nil
	}
	if s.Locals == nil {
		s.Locals = make(map[string]*LocalEscape)
	}
	if existing, ok := s.Locals[name]; ok {
		return existing
	}
	le := &LocalEscape{Name: name, Reasons: make(map[EscapeReason]struct{})}
	s.Locals[name] = le
	return le
}

func (s *FunctionEscapeSummary) mark(name string, reason EscapeReason) {
	if s == nil || name == "" {
		return
	}
	s.ensureLocal(name).add(reason)
}

// ReasonsFor returns sorted reasons for a local binding.
func (s *FunctionEscapeSummary) ReasonsFor(name string) []EscapeReason {
	if s == nil || s.Locals == nil {
		return nil
	}
	le := s.Locals[name]
	if le == nil {
		return nil
	}
	return le.SortedReasons()
}

// Escapes reports whether a local binding has any escape reason.
func (s *FunctionEscapeSummary) Escapes(name string) bool {
	if s == nil || s.Locals == nil {
		return false
	}
	le := s.Locals[name]
	return le != nil && le.Has()
}

type escapeScope struct {
	parent *escapeScope
	locals map[string]struct{}
}

func newEscapeScope(parent *escapeScope) *escapeScope {
	return &escapeScope{
		parent: parent,
		locals: make(map[string]struct{}),
	}
}

func (s *escapeScope) define(name string) {
	if s == nil || name == "" {
		return
	}
	s.locals[name] = struct{}{}
}

func (s *escapeScope) isLocal(name string) bool {
	for cur := s; cur != nil; cur = cur.parent {
		if _, ok := cur.locals[name]; ok {
			return true
		}
	}
	return false
}

func (s *escapeScope) visibleLocals() map[string]struct{} {
	out := make(map[string]struct{})
	for cur := s; cur != nil; cur = cur.parent {
		for name := range cur.locals {
			out[name] = struct{}{}
		}
	}
	return out
}

type escapeAnalyzer struct {
	summary *FunctionEscapeSummary
	scope   *escapeScope
}

func newEscapeAnalyzer(functionName string) *escapeAnalyzer {
	return &escapeAnalyzer{
		summary: &FunctionEscapeSummary{
			FunctionName: functionName,
			Locals:       make(map[string]*LocalEscape),
		},
		scope: newEscapeScope(nil),
	}
}

func AnalyzeFunctionDeclEscapes(fd *ast.FunctionDecl) *FunctionEscapeSummary {
	if fd == nil || fd.Name == nil {
		return &FunctionEscapeSummary{FunctionName: "", Locals: map[string]*LocalEscape{}}
	}
	an := newEscapeAnalyzer(fd.Name.Value)
	for _, p := range fd.Parameters {
		if p != nil && p.Name != nil {
			an.scope.define(p.Name.Value)
			an.summary.ensureLocal(p.Name.Value)
		}
	}
	an.walkBlock(fd.Body)
	return an.summary
}

func AnalyzeMethodDeclEscapes(typeName, receiverName string, md *ast.MethodDecl) *FunctionEscapeSummary {
	if md == nil || md.Name == nil {
		return &FunctionEscapeSummary{FunctionName: "", Locals: map[string]*LocalEscape{}}
	}
	fnName := typeName + "." + md.Name.Value
	an := newEscapeAnalyzer(fnName)
	if receiverName != "" {
		an.scope.define(receiverName)
		an.summary.ensureLocal(receiverName)
	}
	for _, p := range md.Parameters {
		if p != nil && p.Name != nil {
			an.scope.define(p.Name.Value)
			an.summary.ensureLocal(p.Name.Value)
		}
	}
	an.walkBlock(md.Body)
	return an.summary
}

func AnalyzeFunctionLiteralEscapes(name string, fl *ast.FunctionLiteral) *FunctionEscapeSummary {
	an := newEscapeAnalyzer(name)
	if fl == nil {
		return an.summary
	}
	for _, p := range fl.Parameters {
		if p != nil && p.Name != nil {
			an.scope.define(p.Name.Value)
			an.summary.ensureLocal(p.Name.Value)
		}
	}
	an.walkBlock(fl.Body)
	return an.summary
}

func (an *escapeAnalyzer) withScope(run func()) {
	old := an.scope
	an.scope = newEscapeScope(old)
	defer func() { an.scope = old }()
	run()
}

func (an *escapeAnalyzer) walkBlock(block *ast.BlockStatement) {
	if block == nil {
		return
	}
	an.withScope(func() {
		for _, stmt := range block.Statements {
			an.walkStmt(stmt)
		}
	})
}

func (an *escapeAnalyzer) walkStmt(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.BlockStatement:
		an.walkBlock(s)
	case *ast.VarStatement:
		an.walkExpr(s.Value)
		if s.Name != nil {
			an.scope.define(s.Name.Value)
			an.summary.ensureLocal(s.Name.Value)
		}
	case *ast.MultiVarStatement:
		an.walkExpr(s.Value)
		for _, n := range s.Names {
			if n == nil {
				continue
			}
			an.scope.define(n.Value)
			an.summary.ensureLocal(n.Value)
		}
	case *ast.ReturnStatement:
		an.markLocalsInExpr(s.ReturnValue, EscapeReturnedValue)
		if be, ok := s.ReturnValue.(*ast.BorrowExpression); ok {
			if id, ok := be.Value.(*ast.Identifier); ok && an.scope.isLocal(id.Value) {
				an.summary.mark(id.Value, EscapeReturnedReference)
			}
		}
		an.walkExpr(s.ReturnValue)
	case *ast.ExpressionStatement:
		an.walkExpr(s.Expression)
	case *ast.AssignmentStatement:
		an.handleAssignmentEscape(s)
		an.walkExpr(s.Left)
		an.walkExpr(s.Value)
	case *ast.IfStatement:
		an.walkExpr(s.Condition)
		an.walkBlock(s.Consequence)
		an.walkBlock(s.Alternative)
	case *ast.WhileStatement:
		an.walkExpr(s.Condition)
		an.walkBlock(s.Body)
	case *ast.ForStatement:
		an.walkExpr(s.Iterable)
		an.withScope(func() {
			if s.Variable != nil {
				an.scope.define(s.Variable.Value)
				an.summary.ensureLocal(s.Variable.Value)
			}
			an.walkBlock(s.Body)
		})
	case *ast.SwitchStatement:
		an.walkExpr(s.Value)
		for _, c := range s.Cases {
			if c == nil {
				continue
			}
			for _, v := range c.Values {
				an.walkExpr(v)
			}
			an.walkBlock(c.Body)
		}
	case *ast.DeferStatement:
		an.walkBlock(s.Body)
	case *ast.PanicStatement:
		an.walkExpr(s.Message)
	case *ast.UnsafeBlock:
		an.walkBlock(s.Body)
	}
}

func (an *escapeAnalyzer) handleAssignmentEscape(as *ast.AssignmentStatement) {
	if as == nil {
		return
	}
	switch left := as.Left.(type) {
	case *ast.Identifier:
		if !an.scope.isLocal(left.Value) {
			an.markLocalsInExpr(as.Value, EscapeAssignedToGlobal)
		}
	case *ast.FieldAccessExpression, *ast.IndexExpression, *ast.DerefExpression:
		an.markLocalsInExpr(as.Value, EscapeStoredExternally)
	}
}

func (an *escapeAnalyzer) walkExpr(expr ast.Expression) {
	switch e := expr.(type) {
	case *ast.PrefixExpression:
		an.walkExpr(e.Right)
	case *ast.InfixExpression:
		an.walkExpr(e.Left)
		an.walkExpr(e.Right)
	case *ast.CallExpression:
		an.walkExpr(e.Function)
		for _, arg := range e.Arguments {
			an.markLocalsInExpr(arg, EscapePassedToCall)
			an.walkExpr(arg)
		}
	case *ast.MethodCallExpression:
		an.walkExpr(e.Object)
		for _, arg := range e.Arguments {
			an.markLocalsInExpr(arg, EscapePassedToCall)
			an.walkExpr(arg)
		}
	case *ast.FieldAccessExpression:
		an.walkExpr(e.Object)
	case *ast.IndexExpression:
		an.walkExpr(e.Left)
		an.walkExpr(e.Index)
	case *ast.StructLiteral:
		for _, value := range e.Fields {
			an.markLocalsInExpr(value, EscapeStoredInAggregate)
			an.walkExpr(value)
		}
	case *ast.VecLiteral:
		for _, el := range e.Elements {
			an.markLocalsInExpr(el, EscapeStoredInAggregate)
			an.walkExpr(el)
		}
	case *ast.TupleExpression:
		for _, el := range e.Elements {
			an.markLocalsInExpr(el, EscapeStoredInAggregate)
			an.walkExpr(el)
		}
	case *ast.RangeExpression:
		an.walkExpr(e.Start)
		an.walkExpr(e.End)
	case *ast.EnumVariantExpression:
		for _, v := range e.Values {
			an.markLocalsInExpr(v, EscapeStoredInAggregate)
			an.walkExpr(v)
		}
	case *ast.BorrowExpression:
		an.walkExpr(e.Value)
	case *ast.DerefExpression:
		an.walkExpr(e.Value)
	case *ast.UnwrapExpression:
		an.walkExpr(e.Value)
	case *ast.FunctionLiteral:
		visible := an.scope.visibleLocals()
		for _, p := range e.Parameters {
			if p != nil && p.Name != nil {
				delete(visible, p.Name.Value)
			}
		}
		used := make(map[string]struct{})
		collectIdentifiersFromBlock(e.Body, used)
		for name := range used {
			if _, ok := visible[name]; ok {
				an.summary.mark(name, EscapeCapturedByClosure)
			}
		}
	}
}

func (an *escapeAnalyzer) markLocalsInExpr(expr ast.Expression, reason EscapeReason) {
	if expr == nil {
		return
	}
	used := make(map[string]struct{})
	collectIdentifiersFromExpr(expr, used)
	for name := range used {
		if an.scope.isLocal(name) {
			an.summary.mark(name, reason)
		}
	}
}

func collectIdentifiersFromBlock(block *ast.BlockStatement, out map[string]struct{}) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		collectIdentifiersFromStmt(stmt, out)
	}
}

func collectIdentifiersFromStmt(stmt ast.Statement, out map[string]struct{}) {
	switch s := stmt.(type) {
	case *ast.VarStatement:
		collectIdentifiersFromExpr(s.Value, out)
	case *ast.MultiVarStatement:
		collectIdentifiersFromExpr(s.Value, out)
	case *ast.ReturnStatement:
		collectIdentifiersFromExpr(s.ReturnValue, out)
	case *ast.ExpressionStatement:
		collectIdentifiersFromExpr(s.Expression, out)
	case *ast.BlockStatement:
		collectIdentifiersFromBlock(s, out)
	case *ast.IfStatement:
		collectIdentifiersFromExpr(s.Condition, out)
		collectIdentifiersFromBlock(s.Consequence, out)
		collectIdentifiersFromBlock(s.Alternative, out)
	case *ast.WhileStatement:
		collectIdentifiersFromExpr(s.Condition, out)
		collectIdentifiersFromBlock(s.Body, out)
	case *ast.ForStatement:
		collectIdentifiersFromExpr(s.Iterable, out)
		collectIdentifiersFromBlock(s.Body, out)
	case *ast.SwitchStatement:
		collectIdentifiersFromExpr(s.Value, out)
		for _, c := range s.Cases {
			if c == nil {
				continue
			}
			for _, v := range c.Values {
				collectIdentifiersFromExpr(v, out)
			}
			collectIdentifiersFromBlock(c.Body, out)
		}
	case *ast.AssignmentStatement:
		collectIdentifiersFromExpr(s.Left, out)
		collectIdentifiersFromExpr(s.Value, out)
	case *ast.DeferStatement:
		collectIdentifiersFromBlock(s.Body, out)
	case *ast.PanicStatement:
		collectIdentifiersFromExpr(s.Message, out)
	case *ast.UnsafeBlock:
		collectIdentifiersFromBlock(s.Body, out)
	}
}

func collectIdentifiersFromExpr(expr ast.Expression, out map[string]struct{}) {
	switch e := expr.(type) {
	case *ast.Identifier:
		if e.Value != "" {
			out[e.Value] = struct{}{}
		}
	case *ast.MutableIdentifier:
		if e.Value != "" {
			out[e.Value] = struct{}{}
		}
	case *ast.PrefixExpression:
		collectIdentifiersFromExpr(e.Right, out)
	case *ast.InfixExpression:
		collectIdentifiersFromExpr(e.Left, out)
		collectIdentifiersFromExpr(e.Right, out)
	case *ast.CallExpression:
		collectIdentifiersFromExpr(e.Function, out)
		for _, arg := range e.Arguments {
			collectIdentifiersFromExpr(arg, out)
		}
	case *ast.MethodCallExpression:
		collectIdentifiersFromExpr(e.Object, out)
		for _, arg := range e.Arguments {
			collectIdentifiersFromExpr(arg, out)
		}
	case *ast.FieldAccessExpression:
		collectIdentifiersFromExpr(e.Object, out)
	case *ast.IndexExpression:
		collectIdentifiersFromExpr(e.Left, out)
		collectIdentifiersFromExpr(e.Index, out)
	case *ast.StructLiteral:
		for _, v := range e.Fields {
			collectIdentifiersFromExpr(v, out)
		}
	case *ast.VecLiteral:
		for _, el := range e.Elements {
			collectIdentifiersFromExpr(el, out)
		}
	case *ast.TupleExpression:
		for _, el := range e.Elements {
			collectIdentifiersFromExpr(el, out)
		}
	case *ast.RangeExpression:
		collectIdentifiersFromExpr(e.Start, out)
		collectIdentifiersFromExpr(e.End, out)
	case *ast.EnumVariantExpression:
		for _, v := range e.Values {
			collectIdentifiersFromExpr(v, out)
		}
	case *ast.BorrowExpression:
		collectIdentifiersFromExpr(e.Value, out)
	case *ast.DerefExpression:
		collectIdentifiersFromExpr(e.Value, out)
	case *ast.UnwrapExpression:
		collectIdentifiersFromExpr(e.Value, out)
	case *ast.FunctionLiteral:
		collectIdentifiersFromBlock(e.Body, out)
	}
}

// FormatEscapeReports renders a stable textual dump of escape decisions.
func FormatEscapeReports(reports map[string]*FunctionEscapeSummary) string {
	if len(reports) == 0 {
		return "Escape analysis:\n  (no functions analyzed)"
	}

	var b strings.Builder
	b.WriteString("Escape analysis:\n")

	functionNames := make([]string, 0, len(reports))
	for name := range reports {
		functionNames = append(functionNames, name)
	}
	sort.Strings(functionNames)

	for _, fnName := range functionNames {
		summary := reports[fnName]
		b.WriteString("  ")
		b.WriteString(fnName)
		b.WriteString(":\n")
		if summary == nil || len(summary.Locals) == 0 {
			b.WriteString("    (no locals)\n")
			continue
		}

		localNames := make([]string, 0, len(summary.Locals))
		for localName := range summary.Locals {
			localNames = append(localNames, localName)
		}
		sort.Strings(localNames)

		for _, localName := range localNames {
			reasons := summary.ReasonsFor(localName)
			if len(reasons) == 0 {
				b.WriteString("    ")
				b.WriteString(localName)
				b.WriteString(": stack\n")
				continue
			}
			b.WriteString("    ")
			b.WriteString(localName)
			b.WriteString(": heap")
			b.WriteString(" [")
			for i, reason := range reasons {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(string(reason))
			}
			b.WriteString("]\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

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
	EscapeBorrowedForCall   EscapeReason = "borrow_of_frame"
)

// AllocDecision is the allocation chosen for a binding. It is a compiler
// detail: the language still has the same ownership rules.
type AllocDecision string

const (
	AllocFrameSlot      AllocDecision = "frame_slot"
	AllocFrameBorrow    AllocDecision = "frame_borrow"
	AllocClosureStorage AllocDecision = "closure_storage"
	AllocCallerReturn   AllocDecision = "caller_return"
	AllocHeapCell       AllocDecision = "heap_cell"
	AllocTemporary      AllocDecision = "temporary"
)

// TempAlloc records where a temporary aggregate is placed.
type TempAlloc struct {
	Kind   string
	Region string
}

// LocalEscape records escape reasons for a single local binding.
type LocalEscape struct {
	Name          string
	Reasons       map[EscapeReason]struct{}
	Borrow        bool
	Param         bool
	MutableBorrow bool
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
	Temps        []TempAlloc
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

	le := &LocalEscape{
		Name:    name,
		Reasons: make(map[EscapeReason]struct{}),
	}

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

// Decision reports the allocation chosen for a local.
// Heap cells are reserved for references that outlive the frame and for
// values moved into closure storage. Owned moves stay in frame or caller storage.
func (s *FunctionEscapeSummary) Decision(name string) (AllocDecision, string) {
	if s == nil || s.Locals == nil {
		return AllocFrameSlot, ""
	}
	le := s.Locals[name]
	if le == nil {
		return AllocFrameSlot, ""
	}
	has := func(reason EscapeReason) bool {
		_, ok := le.Reasons[reason]
		return ok
	}
	if has(EscapeReturnedReference) {
		return AllocHeapCell, string(EscapeReturnedReference)
	}
	if has(EscapeCapturedByClosure) && !le.Borrow {
		return AllocClosureStorage, string(EscapeCapturedByClosure)
	}
	switch {
	case has(EscapeStoredExternally):
		return AllocHeapCell, string(EscapeStoredExternally)
	case has(EscapeAssignedToGlobal):
		return AllocHeapCell, string(EscapeAssignedToGlobal)
	case has(EscapeStoredInAggregate):
		return AllocHeapCell, string(EscapeStoredInAggregate)
	}
	if has(EscapeReturnedValue) {
		return AllocCallerReturn, string(EscapeReturnedValue)
	}
	if le.Borrow {
		switch {
		case le.Param && le.MutableBorrow:
			return AllocFrameBorrow, "mutable_borrow_param"
		case le.Param && !le.MutableBorrow:
			return AllocFrameBorrow, "readonly_borrow_param"
		default:
			return AllocFrameBorrow, "borrow_of_frame"
		}
	}
	if has(EscapeBorrowedForCall) {
		return AllocFrameSlot, string(EscapeBorrowedForCall)
	}
	if has(EscapePassedToCall) {
		return AllocFrameSlot, string(EscapePassedToCall)
	}
	return AllocFrameSlot, ""
}

// Escapes reports whether codegen must keep the binding alive past the frame.
// Borrow parameters and owned values that are only moved do not escape.
func (s *FunctionEscapeSummary) Escapes(name string) bool {
	decision, _ := s.Decision(name)
	return decision == AllocHeapCell || decision == AllocClosureStorage
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
	summary        *FunctionEscapeSummary
	scope          *escapeScope
	borrowBindings map[string]bool
	region         string
}

func newEscapeAnalyzer(functionName string) *escapeAnalyzer {
	return &escapeAnalyzer{
		summary: &FunctionEscapeSummary{
			FunctionName: functionName,
			Locals:       make(map[string]*LocalEscape),
		},
		scope:          newEscapeScope(nil),
		borrowBindings: make(map[string]bool),
		region:         string(AllocTemporary),
	}
}

func AnalyzeFunctionDeclEscapes(fd *ast.FunctionDecl) *FunctionEscapeSummary {
	if fd == nil || fd.Name == nil {
		return &FunctionEscapeSummary{
			FunctionName: "",
			Locals:       map[string]*LocalEscape{},
		}
	}

	an := newEscapeAnalyzer(fd.Name.Value)
	for _, p := range fd.Parameters {
		if p != nil && p.Name != nil {
			an.noteLocal(p.Name.Value, true, p.Type, nil)
		}
	}
	an.walkBlock(fd.Body)
	return an.summary
}

func AnalyzeMethodDeclEscapes(
	typeName,
	receiverName string,
	md *ast.MethodDecl,
) *FunctionEscapeSummary {

	if md == nil || md.Name == nil {
		return &FunctionEscapeSummary{
			FunctionName: "",
			Locals:       map[string]*LocalEscape{},
		}
	}

	fnName := typeName + "." + md.Name.Value
	an := newEscapeAnalyzer(fnName)
	if receiverName != "" {
		an.noteLocal(receiverName, true, nil, nil)
		if le := an.summary.ensureLocal(receiverName); le != nil {
			le.Borrow = true
			le.MutableBorrow = md.Mutable
			an.borrowBindings[receiverName] = true
		}
	}

	for _, p := range md.Parameters {
		if p != nil && p.Name != nil {
			an.noteLocal(p.Name.Value, true, p.Type, nil)
		}
	}
	an.walkBlock(md.Body)
	return an.summary
}

func defineEnsureLocal(an *escapeAnalyzer, name string) {
	an.noteLocal(name, false, nil, nil)
}

func (an *escapeAnalyzer) noteLocal(name string, param bool, typ ast.TypeExpression, value ast.Expression) {
	if an == nil || name == "" {
		return
	}
	an.scope.define(name)
	le := an.summary.ensureLocal(name)
	if le == nil {
		return
	}
	if param {
		le.Param = true
	}
	borrow, mutable := borrowFlags(typ)
	if !borrow {
		if be, ok := value.(*ast.BorrowExpression); ok {
			borrow = true
			mutable = be.Mutable
		}
	}
	if borrow {
		an.borrowBindings[name] = true
		le.Borrow = true
		le.MutableBorrow = mutable
	}
}

func borrowFlags(t ast.TypeExpression) (bool, bool) {
	switch tt := t.(type) {
	case *ast.BorrowType:
		return true, tt.Mutable
	case *ast.NamedType:
		return borrowFlags(tt.Type)
	default:
		return false, false
	}
}

func (an *escapeAnalyzer) withRegion(region string, run func()) {
	old := an.region
	an.region = region
	defer func() { an.region = old }()
	run()
}

func (an *escapeAnalyzer) noteTemp(kind string) {
	region := an.region
	if region == "" {
		region = string(AllocTemporary)
	}
	an.summary.Temps = append(an.summary.Temps, TempAlloc{Kind: kind, Region: region})
}

func AnalyzeFunctionLiteralEscapes(
	name string,
	fl *ast.FunctionLiteral,
) *FunctionEscapeSummary {

	an := newEscapeAnalyzer(name)
	if fl == nil {
		return an.summary
	}

	for _, p := range fl.Parameters {
		if p != nil && p.Name != nil {
			an.noteLocal(p.Name.Value, true, p.Type, nil)
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
		an.withRegion(string(AllocFrameSlot), func() {
			an.walkExpr(s.Value)
		})
		if s.Name != nil {
			an.noteLocal(s.Name.Value, false, s.Type, s.Value)
		}
	case *ast.MultiVarStatement:
		an.withRegion(string(AllocFrameSlot), func() {
			an.walkExpr(s.Value)
		})
		for i, n := range s.Names {
			if n == nil {
				continue
			}
			var typ ast.TypeExpression
			if i < len(s.Types) {
				typ = s.Types[i]
			}
			an.noteLocal(n.Value, false, typ, nil)
		}
	case *ast.ReturnStatement:
		an.withRegion(string(AllocCallerReturn), func() {
			an.noteReturnedValue(s.ReturnValue)
			an.walkExpr(s.ReturnValue)
		})
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
	switch as.Left.(type) {
	case *ast.Identifier:
		if id, ok := as.Left.(*ast.Identifier); ok && !an.scope.isLocal(id.Value) {
			an.noteStoredBorrow(as.Value, EscapeAssignedToGlobal)
		}
	case *ast.FieldAccessExpression,
		*ast.IndexExpression,
		*ast.DerefExpression:
		an.noteStoredBorrow(as.Value, EscapeStoredExternally)
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
		an.withRegion("call_arg", func() {
			for _, arg := range e.Arguments {
				an.noteCallArg(arg)
				an.walkExpr(arg)
			}
		})
	case *ast.MethodCallExpression:
		an.walkExpr(e.Object)
		an.withRegion("call_arg", func() {
			an.noteCallArg(e.Object)
			for _, arg := range e.Arguments {
				an.noteCallArg(arg)
				an.walkExpr(arg)
			}
		})
	case *ast.FieldAccessExpression:
		an.walkExpr(e.Object)
	case *ast.IndexExpression:
		an.walkExpr(e.Left)
		an.walkExpr(e.Index)
	case *ast.StructLiteral:
		an.noteTemp("struct")
		for _, value := range e.Fields {
			an.noteStoredBorrow(value, EscapeStoredInAggregate)
			an.walkExpr(value)
		}
	case *ast.VecLiteral:
		an.noteTemp("vec")
		for _, el := range e.Elements {
			an.noteStoredBorrow(el, EscapeStoredInAggregate)
			an.walkExpr(el)
		}
	case *ast.TupleExpression:
		an.noteTemp("tuple")
		for _, el := range e.Elements {
			an.noteStoredBorrow(el, EscapeStoredInAggregate)
			an.walkExpr(el)
		}
	case *ast.RangeExpression:
		an.walkExpr(e.Start)
		an.walkExpr(e.End)
	case *ast.EnumVariantExpression:
		an.noteTemp("enum")
		for _, v := range e.Values {
			an.noteStoredBorrow(v, EscapeStoredInAggregate)
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
			if _, ok := visible[name]; ok && !an.borrowBindings[name] {
				an.summary.mark(name, EscapeCapturedByClosure)
			}
		}
	}
}

func (an *escapeAnalyzer) noteCallArg(arg ast.Expression) {
	if _, ok := arg.(*ast.BorrowExpression); ok {
		an.noteStoredBorrow(arg, EscapeBorrowedForCall)
		return
	}
	an.markLocalsInExpr(arg, EscapePassedToCall)
}

func (an *escapeAnalyzer) noteReturnedValue(expr ast.Expression) {
	switch e := expr.(type) {
	case *ast.Identifier:
		if an.scope.isLocal(e.Value) {
			an.summary.mark(e.Value, EscapeReturnedValue)
		}
	case *ast.MutableIdentifier:
		if an.scope.isLocal(e.Value) {
			an.summary.mark(e.Value, EscapeReturnedValue)
		}
	case *ast.BorrowExpression:
		an.noteReturnedValue(e.Value)
		an.noteReturnedReference(expr)
	}
}

func (an *escapeAnalyzer) noteReturnedReference(expr ast.Expression) {
	be, ok := expr.(*ast.BorrowExpression)
	if !ok {
		return
	}
	switch id := be.Value.(type) {
	case *ast.Identifier:
		if an.scope.isLocal(id.Value) {
			an.summary.mark(id.Value, EscapeReturnedReference)
		}
	case *ast.MutableIdentifier:
		if an.scope.isLocal(id.Value) {
			an.summary.mark(id.Value, EscapeReturnedReference)
		}
	}
}

func (an *escapeAnalyzer) noteStoredBorrow(expr ast.Expression, reason EscapeReason) {
	be, ok := expr.(*ast.BorrowExpression)
	if !ok {
		return
	}
	var name string
	switch id := be.Value.(type) {
	case *ast.Identifier:
		name = id.Value
	case *ast.MutableIdentifier:
		name = id.Value
	}
	if name == "" || !an.scope.isLocal(name) || an.borrowBindings[name] {
		return
	}
	an.summary.mark(name, reason)
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
			decision, because := summary.Decision(localName)
			b.WriteString("    ")
			b.WriteString(localName)
			b.WriteString(": ")
			b.WriteString(string(decision))
			if because != "" {
				b.WriteString(" because ")
				b.WriteString(because)
			}
			b.WriteByte('\n')
		}
		for _, temp := range summary.Temps {
			b.WriteString("    temporary ")
			b.WriteString(temp.Kind)
			b.WriteString(": ")
			b.WriteString(temp.Region)
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

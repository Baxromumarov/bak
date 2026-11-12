package ir

import (
	"reflect"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

type lowerer struct {
	module     Module
	resolver   TypeResolver
	fn         *Function
	block      int
	scopes     [][]Place
	params     map[string]Parameter
	loops      []loopContext
	terminated map[int]bool
}

type loopContext struct {
	scopeDepth     int
	breakTarget    int
	continueTarget int
}

func LowerModule(name string, program *ast.Program, resolver TypeResolver) Module {
	l := &lowerer{resolver: resolver, params: make(map[string]Parameter)}
	l.module = Module{Name: name, Functions: []Function{}}
	if program == nil {
		return l.module
	}
	l.module.Source = program.SourcePath
	for _, stmt := range program.Statements {
		if fn, ok := stmt.(*ast.FunctionDecl); ok {
			l.lowerFunction(fn)
		}
		if impl, ok := stmt.(*ast.ImplDecl); ok {
			for _, method := range impl.Methods {
				l.lowerMethod(impl, method)
			}
		}
	}
	sort.SliceStable(l.module.Functions, func(i, j int) bool {
		return l.module.Functions[i].Name < l.module.Functions[j].Name
	})
	return l.module
}

func (l *lowerer) lowerFunction(decl *ast.FunctionDecl) {
	params := make([]Parameter, 0, len(decl.Parameters))
	l.params = make(map[string]Parameter, len(decl.Parameters))
	for _, p := range decl.Parameters {
		param := Parameter{Name: p.Name.Value, Type: RenderType(p.Type), Mutable: p.Mutable}
		params = append(params, param)
		l.params[param.Name] = param
	}
	fn := Function{Name: decl.Name.Value, ParamCount: len(params), ReturnType: RenderType(decl.ReturnType), Parameters: params, Entry: 0}
	l.fn = &fn
	l.terminated = make(map[int]bool)
	l.block = l.newBlock()
	l.scopes = [][]Place{make([]Place, 0, len(params))}
	for _, p := range params {
		l.scopes[0] = append(l.scopes[0], Place{Kind: PlaceLocal, Name: p.Name, Type: p.Type})
	}
	l.lowerBlock(decl.Body, true)
	if !l.terminated[l.block] {
		l.emitScopeDrops(decl, 0)
		l.emit(decl, Instruction{Op: OpReturn, Type: fn.ReturnType, Synthetic: true})
		l.terminated[l.block] = true
	}
	l.module.Functions = append(l.module.Functions, fn)
}

func (l *lowerer) lowerMethod(impl *ast.ImplDecl, method *ast.MethodDecl) {
	params := make([]*ast.Parameter, 0, len(method.Parameters)+1)
	if impl.Receiver != nil {
		params = append(params, &ast.Parameter{Name: impl.Receiver, Type: &ast.SimpleType{Name: impl.TypeName.Value}, Mutable: method.Mutable})
	}
	params = append(params, method.Parameters...)
	decl := &ast.FunctionDecl{Name: &ast.Identifier{Value: impl.TypeName.Value + "." + method.Name.Value}, Parameters: params, ReturnType: method.ReturnType, Body: method.Body}
	l.lowerFunction(decl)
}

func (l *lowerer) newBlock() int {
	id := len(l.fn.Blocks)
	l.fn.Blocks = append(l.fn.Blocks, Block{ID: id})
	return id
}

func (l *lowerer) emit(node ast.Node, inst Instruction) {
	if node != nil {
		pos := node.Pos()
		inst.Location = SourceLocation{File: l.module.Source, Line: pos.Line, Column: pos.Column}
	}
	l.fn.Blocks[l.block].Instructions = append(l.fn.Blocks[l.block].Instructions, inst)
}

func (l *lowerer) edge(from, to int) {
	l.fn.Blocks[from].Successors = append(l.fn.Blocks[from].Successors, to)
}

func (l *lowerer) lowerBlock(block *ast.BlockStatement, scope bool) {
	if block == nil {
		return
	}
	if scope {
		l.scopes = append(l.scopes, nil)
	}
	for _, stmt := range block.Statements {
		l.lowerStatement(stmt)
		if l.terminated[l.block] {
			break
		}
	}
	if scope {
		if !l.terminated[l.block] {
			l.emitScopeDrops(block, len(l.scopes)-1)
		}
		l.scopes = l.scopes[:len(l.scopes)-1]
	}
}

func (l *lowerer) lowerStatement(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.VarStatement:
		l.lowerExpr(s.Value, true)
		place := Place{Kind: PlaceLocal, Name: s.Name.Value, Type: RenderType(s.Type)}
		l.emit(s, Instruction{Op: OpStore, Type: place.Type, Place: &place})
		l.scopes[len(l.scopes)-1] = append(l.scopes[len(l.scopes)-1], place)
	case *ast.MultiVarStatement:
		l.lowerExpr(s.Value, true)
		for _, name := range s.Names {
			place := Place{Kind: PlaceLocal, Name: name.Value, Type: l.nodeType(name)}
			l.emit(s, Instruction{Op: OpStore, Type: place.Type, Place: &place})
			l.scopes[len(l.scopes)-1] = append(l.scopes[len(l.scopes)-1], place)
		}
	case *ast.AssignmentStatement:
		l.lowerExpr(s.Value, true)
		if place, ok := l.place(s.Left); ok {
			l.emit(s.Left, Instruction{Op: OpDrop, Type: place.Type, Place: &place, Synthetic: true})
			l.emit(s, Instruction{Op: OpStore, Type: place.Type, Place: &place})
		}
	case *ast.ExpressionStatement:
		l.lowerExpr(s.Expression, false)
	case *ast.ReturnStatement:
		l.lowerExpr(s.ReturnValue, true)
		l.emitAllDrops(s)
		l.emit(s, Instruction{Op: OpReturn, Type: l.fn.ReturnType})
		l.terminated[l.block] = true
	case *ast.IfStatement:
		l.lowerIf(s)
	case *ast.WhileStatement:
		l.lowerWhile(s)
	case *ast.ForStatement:
		l.lowerFor(s)
	case *ast.SwitchStatement:
		l.lowerSwitch(s)
	case *ast.BreakStatement:
		depth := 0
		targets := []int(nil)
		if len(l.loops) > 0 {
			loop := l.loops[len(l.loops)-1]
			depth = loop.scopeDepth
			targets = []int{loop.breakTarget}
		}
		l.emitDropsFrom(stmt, depth)
		l.emit(stmt, Instruction{Op: OpJump, Targets: targets})
		if len(targets) > 0 {
			l.edge(l.block, targets[0])
		}
		l.terminated[l.block] = true
	case *ast.ContinueStatement:
		depth := 0
		targets := []int(nil)
		if len(l.loops) > 0 {
			loop := l.loops[len(l.loops)-1]
			depth = loop.scopeDepth
			targets = []int{loop.continueTarget}
		}
		l.emitDropsFrom(stmt, depth)
		l.emit(stmt, Instruction{Op: OpJump, Targets: targets})
		if len(targets) > 0 {
			l.edge(l.block, targets[0])
		}
		l.terminated[l.block] = true
	case *ast.PanicStatement:
		l.lowerExpr(s.Message, false)
		l.emitAllDrops(s)
		l.emit(s, Instruction{Op: OpPanic})
		l.terminated[l.block] = true
	case *ast.DeferStatement:
		l.emit(s, Instruction{Op: OpDefer})
		l.lowerBlock(s.Body, true)
	case *ast.BlockStatement:
		l.lowerBlock(s, true)
	}
}

func (l *lowerer) lowerIf(s *ast.IfStatement) {
	l.lowerExpr(s.Condition, false)
	from := l.block
	thenID, elseID, joinID := l.newBlock(), l.newBlock(), l.newBlock()
	l.emit(s, Instruction{Op: OpBranch, Type: "bool", Targets: []int{thenID, elseID}})
	l.edge(from, thenID)
	l.edge(from, elseID)
	l.block = thenID
	l.lowerBlock(s.Consequence, true)
	thenEnd := l.block
	if !l.terminated[thenEnd] {
		l.emit(s, Instruction{Op: OpJump, Targets: []int{joinID}})
		l.edge(thenEnd, joinID)
	}
	l.block = elseID
	if s.Alternative != nil {
		l.lowerBlock(s.Alternative, true)
	}
	elseEnd := l.block
	if !l.terminated[elseEnd] {
		l.emit(s, Instruction{Op: OpJump, Targets: []int{joinID}})
		l.edge(elseEnd, joinID)
	}
	l.block = joinID
	if s.Alternative != nil && l.terminated[thenEnd] && l.terminated[elseEnd] {
		l.terminated[joinID] = true
	}
}

func (l *lowerer) lowerWhile(s *ast.WhileStatement) {
	header, body, exit := l.newBlock(), l.newBlock(), l.newBlock()
	l.emit(s, Instruction{Op: OpJump, Targets: []int{header}})
	l.edge(l.block, header)
	l.block = header
	l.lowerExpr(s.Condition, false)
	l.emit(s, Instruction{Op: OpBranch, Type: "bool", Targets: []int{body, exit}})
	l.edge(header, body)
	l.edge(header, exit)
	l.block = body
	l.lowerLoopBody(s.Body, exit, header)
	bodyEnd := l.block
	if !l.terminated[bodyEnd] {
		l.emit(s, Instruction{Op: OpJump, Targets: []int{header}})
		l.edge(bodyEnd, header)
	}
	l.block = exit
}

func (l *lowerer) lowerLoopBody(body *ast.BlockStatement, breakTarget, continueTarget int) {
	l.loops = append(l.loops, loopContext{
		scopeDepth:     len(l.scopes),
		breakTarget:    breakTarget,
		continueTarget: continueTarget,
	})
	l.lowerBlock(body, true)
	l.loops = l.loops[:len(l.loops)-1]
}

func (l *lowerer) lowerFor(s *ast.ForStatement) {
	l.lowerExpr(s.Iterable, false)
	header, body, step, exit := l.newBlock(), l.newBlock(), l.newBlock(), l.newBlock()
	l.emit(s, Instruction{Op: OpJump, Targets: []int{header}})
	l.edge(l.block, header)

	l.block = header
	l.emit(s, Instruction{Op: OpBranch, Type: "bool", Targets: []int{body, exit}, Synthetic: true})
	l.edge(header, body)
	l.edge(header, exit)

	l.block = body
	l.scopes = append(l.scopes, nil)
	iterator := Place{Kind: PlaceLocal, Name: s.Variable.Value, Type: l.nodeType(s.Variable)}
	l.emit(s.Variable, Instruction{Op: OpStore, Type: iterator.Type, Place: &iterator, Synthetic: true})
	l.scopes[len(l.scopes)-1] = append(l.scopes[len(l.scopes)-1], iterator)
	l.loops = append(l.loops, loopContext{
		scopeDepth:     len(l.scopes) - 1,
		breakTarget:    exit,
		continueTarget: step,
	})
	for _, stmt := range s.Body.Statements {
		l.lowerStatement(stmt)
		if l.terminated[l.block] {
			break
		}
	}
	l.loops = l.loops[:len(l.loops)-1]
	if !l.terminated[l.block] {
		l.emitScopeDrops(s.Body, len(l.scopes)-1)
		l.emit(s, Instruction{Op: OpJump, Targets: []int{step}})
		l.edge(l.block, step)
	}
	l.scopes = l.scopes[:len(l.scopes)-1]

	l.block = step
	l.emit(s, Instruction{Op: OpJump, Targets: []int{header}, Synthetic: true})
	l.edge(step, header)
	l.block = exit
}

func (l *lowerer) lowerSwitch(s *ast.SwitchStatement) {
	l.lowerExpr(s.Value, false)
	from, join := l.block, l.newBlock()
	targets := make([]int, 0, len(s.Cases))
	hasDefault := false
	for range s.Cases {
		targets = append(targets, l.newBlock())
	}
	l.block = from
	branchTargets := append([]int(nil), targets...)
	for _, c := range s.Cases {
		hasDefault = hasDefault || c.Default
	}
	if !hasDefault {
		branchTargets = append(branchTargets, join)
	}
	l.emit(s, Instruction{Op: OpBranch, Targets: branchTargets})
	if !hasDefault {
		l.edge(from, join)
	}
	allTerminated := hasDefault
	for i, c := range s.Cases {
		l.edge(from, targets[i])
		l.block = targets[i]
		l.lowerBlock(c.Body, true)
		caseEnd := l.block
		if !l.terminated[caseEnd] {
			allTerminated = false
			l.emit(c, Instruction{Op: OpJump, Targets: []int{join}})
			l.edge(caseEnd, join)
		}
	}
	l.block = join
	if allTerminated {
		l.terminated[join] = true
	}
}

func (l *lowerer) lowerExpr(expr ast.Expression, consume bool) {
	if expr == nil {
		return
	}
	if place, ok := l.place(expr); ok {
		op, own := OpLoad, OwnershipCopy
		if l.shouldMove(expr, consume, place.Type) {
			op, own = OpMove, OwnershipMove
		}
		if _, ok := expr.(*ast.MutableIdentifier); ok {
			op, own = OpBorrowMut, OwnershipMutBorrow
		}
		l.emit(expr, Instruction{Op: op, Type: place.Type, Ownership: own, Place: &place})
		return
	}
	switch e := expr.(type) {
	case *ast.BorrowExpression:
		if place, ok := l.place(e.Value); ok {
			op, own := OpBorrowShared, OwnershipSharedBorrow
			if e.Mutable {
				op, own = OpBorrowMut, OwnershipMutBorrow
			}
			l.emit(e, Instruction{Op: op, Type: l.nodeType(e), Ownership: own, Place: &place})
		}
	case *ast.CallExpression:
		for _, arg := range e.Arguments {
			l.lowerExpr(arg, true)
		}
		callee := "<expression>"
		if id, ok := e.Function.(*ast.Identifier); ok {
			callee = id.Value
		}
		l.emit(e, Instruction{Op: OpCall, Type: l.nodeType(e), Callee: callee})
	case *ast.MethodCallExpression:
		l.lowerExpr(e.Object, false)
		for _, arg := range e.Arguments {
			l.lowerExpr(arg, true)
		}
		l.emit(e, Instruction{Op: OpCall, Type: l.nodeType(e), Callee: e.Method.Value})
	case *ast.TupleExpression:
		for _, item := range e.Elements {
			l.lowerExpr(item, consume)
		}
	case *ast.VecLiteral:
		for _, item := range e.Elements {
			l.lowerExpr(item, consume)
		}
	case *ast.StructLiteral:
		for _, name := range e.FieldOrder {
			l.lowerExpr(e.Fields[name], consume)
		}
	case *ast.EnumVariantExpression:
		for _, item := range e.Values {
			l.lowerExpr(item, consume)
		}
	case *ast.InfixExpression:
		l.lowerExpr(e.Left, false)
		l.lowerExpr(e.Right, false)
	case *ast.PrefixExpression:
		l.lowerExpr(e.Right, false)
	case *ast.TypeConversion:
		l.lowerExpr(e.Value, consume)
	case *ast.UnwrapExpression:
		l.lowerExpr(e.Value, consume)
	default:
		l.emit(expr, Instruction{Op: OpValue, Type: l.nodeType(expr), Ownership: OwnershipCopy})
	}
}

func (l *lowerer) shouldMove(expr ast.Expression, consume bool, typ string) bool {
	if !consume {
		return false
	}
	if resolver, ok := l.resolver.(OwnershipResolver); ok {
		return resolver.IsMoveExpression(expr)
	}
	return false
}

func (l *lowerer) place(expr ast.Expression) (Place, bool) {
	switch e := expr.(type) {
	case *ast.Identifier:
		kind := PlaceLocal
		typ := l.nodeType(e)
		if param, local := l.params[e.Value]; local {
			typ = param.Type
		} else if localType, local := l.localType(e.Value); local {
			typ = localType
		} else {
			kind = PlaceGlobal
		}
		return Place{Kind: kind, Name: e.Value, Type: typ}, true
	case *ast.MutableIdentifier:
		return Place{Kind: PlaceLocal, Name: e.Value, Type: l.nodeType(e)}, true
	case *ast.FieldAccessExpression:
		place, ok := l.place(e.Object)
		if !ok {
			return Place{}, false
		}
		place.Projections = append(place.Projections, Projection{Kind: ProjectionField, Name: e.Field.Value})
		place.Type = l.nodeType(e)
		return place, true
	case *ast.IndexExpression:
		place, ok := l.place(e.Left)
		if !ok {
			return Place{}, false
		}
		place.Projections = append(place.Projections, Projection{Kind: ProjectionIndex})
		place.Type = l.nodeType(e)
		return place, true
	default:
		return Place{}, false
	}
}

func (l *lowerer) localExists(name string) bool {
	_, ok := l.localType(name)
	return ok
}

func (l *lowerer) localType(name string) (string, bool) {
	for i := len(l.scopes) - 1; i >= 0; i-- {
		for _, p := range l.scopes[i] {
			if p.Name == name {
				return p.Type, true
			}
		}
	}
	return "", false
}

func (l *lowerer) emitScopeDrops(node ast.Node, scope int) {
	if scope < 0 || scope >= len(l.scopes) {
		return
	}
	for i := len(l.scopes[scope]) - 1; i >= 0; i-- {
		place := l.scopes[scope][i]
		l.emit(node, Instruction{Op: OpDrop, Type: place.Type, Place: &place, Synthetic: true})
	}
	l.emit(node, Instruction{Op: OpScopeExit, Synthetic: true})
}

func (l *lowerer) emitDropsFrom(node ast.Node, first int) {
	for scope := len(l.scopes) - 1; scope >= first; scope-- {
		l.emitScopeDrops(node, scope)
	}
}
func (l *lowerer) emitAllDrops(node ast.Node) { l.emitDropsFrom(node, 0) }

func (l *lowerer) nodeType(node ast.Node) string {
	if l.resolver != nil {
		if typ := l.resolver.GetNodeType(node); typ != "" {
			return typ
		}
	}
	switch n := node.(type) {
	case *ast.VarStatement:
		return RenderType(n.Type)
	case *ast.IntegerLiteral:
		return "int"
	case *ast.FloatLiteral:
		return "float64"
	case *ast.StringLiteral:
		return "string"
	case *ast.BooleanLiteral:
		return "bool"
	case *ast.CharLiteral:
		return "char"
	case *ast.VoidLiteral:
		return "void"
	}
	return "unknown"
}

func isCopyType(t string) bool {
	switch t {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float", "float32", "float64", "bool", "char", "byte":
		return true
	}
	return false
}

func RenderType(t ast.TypeExpression) string {
	if t == nil {
		return "void"
	}
	switch tt := t.(type) {
	case *ast.VoidType:
		return "void"
	case *ast.SimpleType:
		return tt.Name
	case *ast.BorrowType:
		prefix := "&"
		if tt.Mutable {
			prefix = "&mut "
		}
		return prefix + RenderType(tt.Inner)
	case *ast.GenericType:
		parts := make([]string, 0, len(tt.TypeParams))
		for _, p := range tt.TypeParams {
			parts = append(parts, RenderType(p))
		}
		if len(parts) == 0 {
			return tt.Name
		}
		return tt.Name + "<" + strings.Join(parts, ",") + ">"
	case *ast.TupleType:
		parts := make([]string, 0, len(tt.Elements))
		for _, p := range tt.Elements {
			parts = append(parts, RenderType(p))
		}
		return "(" + strings.Join(parts, ",") + ")"
	default:
		return reflect.TypeOf(t).String()
	}
}

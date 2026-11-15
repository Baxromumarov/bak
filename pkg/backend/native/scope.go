package native

import "slices"

import "github.com/baxromumarov/bak/pkg/ast"

// Variable and scope management for the native code generator.

// Local represents a local variable on the stack.
type Local struct {
	Name   string
	Offset int // negative offset from RBP, e.g. -8, -16, -24
	// Owned is true when this slot is the unique owner of a heap value.
	// Borrows, for-in bindings, and moved-from aliases stay false.
	Owned bool
	// Spec describes how to destroy the value stored in the slot.
	Spec DropSpec
}

// Scope represents a lexical scope with local variables.
type Scope struct {
	Locals []Local
}

// LoopContext tracks break/continue jump sites for loop compilation.
type LoopContext struct {
	BreakSites     []int // offsets of jmp rel32 imm fields to patch to after-loop
	ContinueSites  []int // offsets of jmp rel32 imm fields to patch to loop-start
	ContinueTarget int   // code offset of the loop condition re-evaluation
	// ScopeDepth is len(Scopes) when the loop was pushed, before the body scope.
	// Break and continue drop scopes at this depth and deeper.
	ScopeDepth int
}

// enterScope pushes a new empty scope.
func (s *EmitState) enterScope() {
	s.Scopes = append(s.Scopes, Scope{})
}

// leaveScope destroys owned locals in the current scope, then pops it.
func (s *EmitState) leaveScope() {
	s.emitDropScope(len(s.Scopes) - 1)
	s.popScope()
}

// popScope pops the current scope without emitting drops.
// Terminated paths already dropped, and code placed after ret would be dead.
func (s *EmitState) popScope() {
	if len(s.Scopes) > 0 {
		s.Scopes = s.Scopes[:len(s.Scopes)-1]
	}
}

// declareLocal allocates a stack slot for a named local variable.
// Returns the RBP-relative offset (negative).
func (s *EmitState) declareLocal(name string, size int) int {
	slots := size / 8
	if size%8 != 0 {
		slots++
	}
	if slots == 0 {
		slots = 1
	}

	s.LocalSlots += slots
	offset := -8 * s.LocalSlots
	if len(s.Scopes) > 0 {
		scope := &s.Scopes[len(s.Scopes)-1]
		scope.Locals = append(scope.Locals, Local{
			Name:   name,
			Offset: offset,
		},
		)
	}
	return offset
}

// resolveLocal searches all scopes (innermost first) for a variable by name.
// Returns the RBP offset and true if found.
func (s *EmitState) resolveLocal(name string) (int, bool) {
	for _, v := range slices.Backward(s.Scopes) {
		for _, l := range v.Locals {
			if l.Name == name {
				return l.Offset, true
			}
		}
	}
	return 0, false
}

// resolveConst searches top-level constants for a name.
// Returns the constant index and true if found.
func (s *EmitState) resolveConst(name string) (int, bool) {
	for i, c := range s.Constants {
		if c.Name.Value == name {
			return i, true
		}
	}
	return 0, false
}

// pushLoop pushes a new loop context for break/continue tracking.
func (s *EmitState) pushLoop(continueTarget int) {
	s.LoopStack = append(s.LoopStack, LoopContext{
		ContinueTarget: continueTarget,
		ScopeDepth:     len(s.Scopes),
	})
}

// popLoop pops the current loop context and returns it.
func (s *EmitState) popLoop() *LoopContext {
	if len(s.LoopStack) == 0 {
		return nil
	}

	ctx := s.LoopStack[len(s.LoopStack)-1]
	s.LoopStack = s.LoopStack[:len(s.LoopStack)-1]
	return &ctx
}

// currentLoop returns a pointer to the current loop context.
func (s *EmitState) currentLoop() *LoopContext {
	if len(s.LoopStack) == 0 {
		return nil
	}

	return &s.LoopStack[len(s.LoopStack)-1]
}

// resetFunctionState resets per-function state (scopes, local slots, loop stack).
func (s *EmitState) resetFunctionState() {
	s.Scopes = s.Scopes[:0]
	// Reset local slots counting
	s.LocalSlots = 0
	s.LoopStack = s.LoopStack[:0]
	// Reset type tracking
	s.IntVariables = make(map[string]bool)
	s.CharVariables = make(map[string]bool)
	s.StringVariables = make(map[string]bool)
	s.FloatVariables = make(map[string]bool)
	s.VecStringElements = make(map[string]bool)
	s.StructVariables = make(map[string]string)
	s.RefVariables = make(map[string]bool)
	s.InOutVariables = make(map[string]bool)
	s.VecElementTypes = make(map[string]string)
	s.GenericVariables = make(map[string]string)
	s.OptionPayloadTypes = make(map[string]ast.TypeExpression)
	s.ResultOkTypes = make(map[string]ast.TypeExpression)
	s.ResultErrTypes = make(map[string]ast.TypeExpression)
	s.DeferStack = s.DeferStack[:0]
	s.terminated = false
	s.consumeNext = false
	s.placeConsume = false
	s.emitDepth = 0
	s.rootMoved = false
}

// closeEnteredScope finishes a scope opened with enterScope.
// A terminated body already dropped that scope on the way out.
func (s *EmitState) closeEnteredScope(terminated bool) {
	if terminated {
		s.popScope()
		return
	}
	s.leaveScope()
}

// emitOpenBlock emits a block whose scope was already entered.
// The caller's terminated flag is preserved.
func (s *EmitState) emitOpenBlock(block *ast.BlockStatement) (bool, error) {
	prev := s.terminated
	s.terminated = false
	err := s.emitBlock(block)
	term := s.terminated
	s.terminated = prev
	return term, err
}

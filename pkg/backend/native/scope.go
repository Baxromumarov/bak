package native

import "github.com/baxromumarov/bak/pkg/ast"

// Variable and scope management for the native code generator.
// Ported from src/compiler/native/backend.bak scope management.

// Local represents a local variable on the stack.
type Local struct {
	Name   string
	Offset int // negative offset from RBP, e.g. -8, -16, -24
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
}

// enterScope pushes a new empty scope.
func (s *EmitState) enterScope() {
	s.Scopes = append(s.Scopes, Scope{})
}

// leaveScope pops the current scope.
func (s *EmitState) leaveScope() {
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
		scope.Locals = append(scope.Locals, Local{Name: name, Offset: offset})
	}
	return offset
}

// resolveLocal searches all scopes (innermost first) for a variable by name.
// Returns the RBP offset and true if found.
func (s *EmitState) resolveLocal(name string) (int, bool) {
	for i := len(s.Scopes) - 1; i >= 0; i-- {
		for _, l := range s.Scopes[i].Locals {
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
	s.VecElementTypes = make(map[string]string)
	s.OptionPayloadTypes = make(map[string]ast.TypeExpression)
	s.ResultOkTypes = make(map[string]ast.TypeExpression)
	s.ResultErrTypes = make(map[string]ast.TypeExpression)
	s.DeferStack = s.DeferStack[:0]
}

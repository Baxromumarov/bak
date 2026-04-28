package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

// DefineSymbol defines a symbol in the current scope
func (e *TypeEnv) DefineSymbol(
	name string,
	typeExpr ast.TypeExpression,
	mutable bool,
	vis ast.Visibility,
	line,
	col int,
) {
	e.symbols[name] = &TypeInfo{
		Type:       typeExpr,
		Mutable:    mutable,
		Visibility: vis,
		Line:       line,
		Column:     col,
	}
	// Clear any moved status when a variable is (re)defined
	delete(e.moved, name)
	delete(e.moveInfo, name)
	delete(e.poisoned, name)
}

// LookupSymbol looks up a symbol in the environment chain
func (e *TypeEnv) LookupSymbol(name string) (*TypeInfo, bool) {
	if info, ok := e.symbols[name]; ok {
		e.MarkUsed(name)
		return info, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			// Jump straight to the root parent if we are in a non-capturing environment.
			// This allows global constants/variables but prevents capturing locals.
			res, ok := e.root().LookupSymbol(name)
			return res, ok
		}
		return e.parent.LookupSymbol(name)
	}
	return nil, false
}

// root returns the top-most environment (package level)
func (e *TypeEnv) root() *TypeEnv {
	curr := e
	for curr.parent != nil {
		curr = curr.parent
	}
	return curr
}

// IsCapture checks if a name exists in a parent environment across a non-capturing boundary.
func (e *TypeEnv) IsCapture(name string) bool {
	curr := e
	var boundary *TypeEnv
	for curr != nil {
		if curr.nonCapturing {
			boundary = curr
			break
		}
		curr = curr.parent
	}

	if boundary == nil || boundary.parent == nil {
		return false
	}

	curr = boundary.parent
	for curr != nil {
		if _, ok := curr.symbols[name]; ok {
			return true
		}
		curr = curr.parent
	}
	return false
}

func (e *TypeEnv) MarkUsed(name string) {
	// Find the environment that defines this symbol and mark there
	env := e
	for env != nil {
		if _, ok := env.symbols[name]; ok {
			env.used[name] = true
			return
		}
		if _, ok := env.functions[name]; ok {
			env.used[name] = true
			return
		}
		if _, ok := env.structs[name]; ok {
			env.used[name] = true
			return
		}
		if _, ok := env.enums[name]; ok {
			env.used[name] = true
			return
		}
		if _, ok := env.typedefs[name]; ok {
			env.used[name] = true
			return
		}
		if _, ok := env.aliases[name]; ok {
			env.used[name] = true
			return
		}
		env = env.parent
	}
	// If not found, mark in current environment (for global tracking, e.g. builtins)
	e.used[name] = true
}

// MarkFieldUsed marks a struct field as used at the root environment level.
func (e *TypeEnv) MarkFieldUsed(fieldName string) {
	env := e
	for env.parent != nil {
		env = env.parent
	}
	env.used[fieldName] = true
}

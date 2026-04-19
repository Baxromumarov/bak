// Package object provides the Environment for variable storage in bak.
package object

import (
	"maps"
	"sync"
)

// VariableInfo holds information about a variable
type VariableInfo struct {
	Value   Object
	Mutable bool
	Moved   bool // For ownership tracking
}

// Environment stores variable bindings
type Environment struct {
	mu           sync.RWMutex
	store        map[string]*VariableInfo
	outer        *Environment
	types        map[string]*StructDef
	enums        map[string]*EnumDef
	impls        map[string]*ImplDef
	deferred     []Object // Deferred calls
	funcScope    bool
	namedReturns []string // Names of named return values
	sourcePath   string
}

// NewEnvironment creates a new environment
func NewEnvironment() *Environment {
	s := make(map[string]*VariableInfo)
	types := make(map[string]*StructDef)
	enums := make(map[string]*EnumDef)
	impls := make(map[string]*ImplDef)
	return &Environment{
		store: s,
		outer: nil,
		types: types,
		enums: enums,
		impls: impls,
	}
}

// NewEnclosedEnvironment creates a new environment with an outer scope
func NewEnclosedEnvironment(outer *Environment) *Environment {
	env := NewEnvironment()
	env.outer = outer
	if outer != nil {
		env.sourcePath = outer.SourcePath()
	}
	return env
}

// Snapshot creates a shallow copy of the current environment's variable bindings.
// The snapshot has the same outer scope but its own copy of the store,
// so mutations after the snapshot don't affect deferred closures.
func (e *Environment) Snapshot() *Environment {
	e.mu.RLock()
	defer e.mu.RUnlock()
	snap := NewEnvironment()
	snap.outer = e.outer
	snap.funcScope = e.funcScope
	snap.sourcePath = e.sourcePath
	for k, v := range e.store {
		snap.store[k] = &VariableInfo{Value: v.Value, Mutable: v.Mutable, Moved: v.Moved}
	}
	maps.Copy(snap.types, e.types)
	maps.Copy(snap.enums, e.enums)
	maps.Copy(snap.impls, e.impls)
	snap.deferred = append([]Object(nil), e.deferred...)
	snap.namedReturns = append([]string(nil), e.namedReturns...)
	return snap
}

func (e *Environment) FunctionScope() *Environment {
	curr := e
	for curr != nil {
		if curr.funcScope {
			return curr
		}
		curr = curr.outer
	}
	return e
}

func (e *Environment) MarkFunctionScope() {
	e.funcScope = true
}

// Root returns the top-most environment (package level)
func (e *Environment) Root() *Environment {
	curr := e
	for curr.outer != nil {
		curr = curr.outer
	}
	return curr
}

// Get retrieves a variable from the environment
func (e *Environment) Get(name string) (*VariableInfo, bool) {
	e.mu.RLock()
	info, ok := e.store[name]
	e.mu.RUnlock()
	if !ok && e.outer != nil {
		return e.outer.Get(name)
	}
	return info, ok
}

// Set stores a variable in the current environment
func (e *Environment) Set(name string, val Object, mutable bool) Object {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.store[name] = &VariableInfo{Value: val, Mutable: mutable, Moved: false}
	return val
}

// Update updates an existing variable's value
func (e *Environment) Update(name string, val Object) (Object, error) {
	e.mu.Lock()
	info, ok := e.store[name]
	if ok {
		if !info.Mutable {
			e.mu.Unlock()
			return nil, &ImmutableError{Name: name}
		}
		if info.Moved {
			e.mu.Unlock()
			return nil, &MovedError{Name: name}
		}
		info.Value = val
		e.mu.Unlock()
		return val, nil
	}
	e.mu.Unlock()
	if e.outer != nil {
		return e.outer.Update(name, val)
	}
	return nil, &UndefinedError{Name: name}
}

// MarkMoved marks a variable as moved
func (e *Environment) MarkMoved(name string) error {
	e.mu.Lock()
	info, ok := e.store[name]
	if ok {
		info.Moved = true
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()
	if e.outer != nil {
		return e.outer.MarkMoved(name)
	}
	return &UndefinedError{Name: name}
}

// IsMoved checks if a variable has been moved
func (e *Environment) IsMoved(name string) bool {
	e.mu.RLock()
	info, ok := e.store[name]
	if ok {
		moved := info.Moved
		e.mu.RUnlock()
		return moved
	}
	e.mu.RUnlock()
	if e.outer != nil {
		return e.outer.IsMoved(name)
	}
	return false
}

// DefineType stores a struct definition
func (e *Environment) DefineType(name string, def *StructDef) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.types[name] = def
}

// GetType retrieves a struct definition
func (e *Environment) GetType(name string) (*StructDef, bool) {
	e.mu.RLock()
	def, ok := e.types[name]
	e.mu.RUnlock()
	if ok {
		return def, true
	}
	if e.outer != nil {
		return e.outer.GetType(name)
	}
	return nil, false
}

// DefineEnum stores an enum definition
func (e *Environment) DefineEnum(name string, def *EnumDef) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enums[name] = def
}

// GetEnum retrieves an enum definition
func (e *Environment) GetEnum(name string) (*EnumDef, bool) {
	e.mu.RLock()
	def, ok := e.enums[name]
	e.mu.RUnlock()
	if !ok && e.outer != nil {
		return e.outer.GetEnum(name)
	}
	return def, ok
}

// DefineImpl stores an impl block definition
func (e *Environment) DefineImpl(typeName string, def *ImplDef) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.impls[typeName] = def
}

// GetImpl retrieves an impl block definition
func (e *Environment) GetImpl(typeName string) (*ImplDef, bool) {
	e.mu.RLock()
	def, ok := e.impls[typeName]
	e.mu.RUnlock()
	if !ok && e.outer != nil {
		return e.outer.GetImpl(typeName)
	}
	return def, ok
}

// GetModules returns all modules in the environment (including outer scopes)
func (e *Environment) GetModules() map[string]Object {
	modules := make(map[string]Object)

	// Get from outer first
	if e.outer != nil {
		outerModules := e.outer.GetModules()
		maps.Copy(modules, outerModules)
	}

	// Override/add from current
	e.mu.RLock()
	for name, info := range e.store {
		if info.Value.Type() == MODULE_OBJ {
			modules[name] = info.Value
		}
	}
	e.mu.RUnlock()

	return modules
}

// AddDeferred adds a deferred call
func (e *Environment) AddDeferred(obj Object) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.deferred = append(e.deferred, obj)
}

// GetDeferred returns all deferred calls (in reverse order)
func (e *Environment) GetDeferred() []Object {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Object, len(e.deferred))
	for i, d := range e.deferred {
		result[len(e.deferred)-1-i] = d
	}
	return result
}

// AddNamedReturn registers a named return variable
func (e *Environment) AddNamedReturn(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.namedReturns = append(e.namedReturns, name)
}

// GetNamedReturns returns the list of named return variable names
func (e *Environment) GetNamedReturns() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]string(nil), e.namedReturns...)
}

func (e *Environment) SetSourcePath(path string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sourcePath = path
}

func (e *Environment) SourcePath() string {
	e.mu.RLock()
	path := e.sourcePath
	e.mu.RUnlock()
	if path == "" && e.outer != nil {
		return e.outer.SourcePath()
	}
	return path
}

// Error types for environment operations

type ImmutableError struct {
	Name string
}

func (e *ImmutableError) Error() string {
	return "cannot assign to immutable variable: " + e.Name
}

type MovedError struct {
	Name string
}

func (e *MovedError) Error() string {
	return "use of moved value: " + e.Name
}

type UndefinedError struct {
	Name string
}

func (e *UndefinedError) Error() string {
	return "undefined variable: " + e.Name
}

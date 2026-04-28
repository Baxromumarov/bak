// Package builtins provides built-in modules for the bak language.
package builtins

import (
	"github.com/baxromumarov/bak/pkg/object"
)

// Modules contains all built-in modules
var Modules = map[string]*object.Module{
	"fs": FSModule,
	"os": OSModule,
}

// TypeConstructors contains type constructors with static methods (e.g., Vec.new())
var TypeConstructors = map[string]*object.TypeConstructor{
	"Vec": {
		Name: "Vec",
		Functions: map[string]*object.Builtin{
			"new":     {Fn: vecNew},
			"from":    {Fn: vecFrom},
			"withCap": {Fn: vecWithCap},
		},
	},
}

// vecNew creates a new empty dynamic Vec
func vecNew(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("Vec.new() takes no arguments")
	}
	return &object.Vec{
		Elements: []object.Object{},
		Mutable:  true,
		Size:     -1, // -1 indicates dynamic size
	}
}

// vecFrom creates a Vec from an array literal
// For dynamic Vec<T,_>: copies elements to a new heap-backed Vec
// For static Vec<T,N>: the type checker validates the size matches
func vecFrom(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("Vec.from() takes exactly one argument (an array literal)")
	}

	// The argument should be a Vec (array literal is parsed as VecLiteral which evaluates to Vec)
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return argTypeError("Vec.from()", args[0], "an array literal")
	}

	// Create a new Vec with copied elements
	newElements := make([]object.Object, len(vec.Elements))
	copy(newElements, vec.Elements)

	return &object.Vec{
		Elements: newElements,
		Mutable:  true,
		Size:     -1, // Size will be set by type checker for static arrays
	}
}

// vecWithCap creates a new dynamic Vec with pre-allocated capacity
func vecWithCap(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("Vec.withCap() takes exactly one argument (capacity)")
	}

	cap, ok := args[0].(*object.Integer)
	if !ok {
		return argTypeError("Vec.withCap()", args[0], "an integer")
	}

	if cap.Value < 0 {
		return newError("Vec.withCap() capacity cannot be negative")
	}

	// Pre-allocate the underlying slice with the given capacity
	elements := make([]object.Object, 0, cap.Value)

	return &object.Vec{
		Elements: elements,
		Mutable:  true,
		Size:     -1, // Dynamic Vec
	}
}

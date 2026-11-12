// Package semantics contains backend-independent validation for Bak's stable
// language contract. These checks deliberately run in code-generation entry
// points as well as in the typechecker so callers cannot bypass stable
// semantics by compiling an AST directly.
package semantics

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

// Violation describes a stable-language rule violation.
type Violation struct {
	Position ast.Position
	Message  string
}

func (v Violation) Error() string {
	if v.Position.Line > 0 {
		return fmt.Sprintf("%d:%d: %s", v.Position.Line, v.Position.Column, v.Message)
	}
	return v.Message
}

// ContainsBorrow reports whether a type contains a borrow at any nesting
// depth. Borrowed function parameters are valid; callers decide whether the
// position in which a type appears permits borrows.
func ContainsBorrow(t ast.TypeExpression) bool {
	return containsBorrow(t, nil, nil)
}

func containsBorrow(t ast.TypeExpression, aliases map[string]ast.TypeExpression, seen map[string]bool) bool {
	if ast.IsNilNode(t) {
		return false
	}
	switch tt := t.(type) {
	case *ast.BorrowType:
		return true
	case *ast.GenericType:
		for _, param := range tt.TypeParams {
			if containsBorrow(param, aliases, seen) {
				return true
			}
		}
	case *ast.TupleType:
		for _, element := range tt.Elements {
			if containsBorrow(element, aliases, seen) {
				return true
			}
		}
	case *ast.FunctionType:
		for _, param := range tt.Params {
			if containsBorrow(param, aliases, seen) {
				return true
			}
		}
		return containsBorrow(tt.ReturnType, aliases, seen)
	case *ast.SimpleType:
		underlying, ok := aliases[tt.Name]
		if !ok {
			return false
		}
		if seen == nil {
			seen = make(map[string]bool)
		}
		if seen[tt.Name] {
			return false
		}
		seen[tt.Name] = true
		contains := containsBorrow(underlying, aliases, seen)
		delete(seen, tt.Name)
		return contains
	}
	return false
}

// ValidateStable checks structural AST rules that every frontend and backend
// must agree on. Type-dependent checks remain in the typechecker.
func ValidateStable(program *ast.Program) []Violation {
	if program == nil {
		return nil
	}
	var violations []Violation
	aliases := make(map[string]ast.TypeExpression)
	for _, stmt := range program.Statements {
		switch decl := stmt.(type) {
		case *ast.TypeDecl:
			aliases[decl.Name.Value] = decl.Underlying
		case *ast.AliasDecl:
			aliases[decl.Name.Value] = decl.Underlying
		}
	}
	for _, stmt := range program.Statements {
		validateStatement(stmt, aliases, &violations)
	}
	ast.Walk(program, func(node ast.Node) {
		if literal, ok := node.(*ast.FunctionLiteral); ok {
			validateCallableReturn(literal.ReturnType, literal.Pos(), aliases, &violations)
		}
	})
	return violations
}

func validateStatement(stmt ast.Statement, aliases map[string]ast.TypeExpression, violations *[]Violation) {
	if ast.IsNilNode(stmt) {
		return
	}
	switch s := stmt.(type) {
	case *ast.FunctionDecl:
		validateCallableReturn(s.ReturnType, s.Name.Pos(), aliases, violations)
	case *ast.ImplDecl:
		for _, method := range s.Methods {
			if method == nil {
				continue
			}
			validateCallableReturn(method.ReturnType, method.Name.Pos(), aliases, violations)
		}
	case *ast.StructDecl:
		for _, field := range s.Fields {
			if field != nil && containsBorrow(field.Type, aliases, nil) {
				*violations = append(*violations, Violation{
					Position: field.Name.Pos(),
					Message:  "borrowed references cannot be stored in struct fields",
				})
			}
		}
	case *ast.EnumDecl:
		for _, variant := range s.Variants {
			if variant == nil {
				continue
			}
			for _, field := range variant.Fields {
				if containsBorrow(field, aliases, nil) {
					*violations = append(*violations, Violation{
						Position: variant.Name.Pos(),
						Message:  "borrowed references cannot be stored in enum payloads",
					})
				}
			}
		}
	}
}

func validateCallableReturn(returnType ast.TypeExpression, pos ast.Position, aliases map[string]ast.TypeExpression, violations *[]Violation) {
	if containsBorrow(returnType, aliases, nil) {
		*violations = append(*violations, Violation{
			Position: pos,
			Message:  "functions cannot return borrowed values in Bak v0.1",
		})
	}
}

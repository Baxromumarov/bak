package compiler

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

func (c *Compiler) compilePrefixExpression(pe *ast.PrefixExpression) error {
	if err := c.compileExpression(pe.Right); err != nil {
		return err
	}

	switch pe.Operator {
	case "-":
		c.emit(OP_NEG)
	case "!":
		c.emit(OP_NOT)
	default:
		return fmt.Errorf("unknown prefix operator: %s", pe.Operator)
	}
	return nil
}

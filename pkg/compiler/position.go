package compiler

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

// CompileError wraps an error with source position info.
type CompileError struct {
	Err    error
	Line   int
	Column int
}

func (e *CompileError) Error() string {
	msg := ""
	if e.Err != nil {
		msg = e.Err.Error()
	}
	if e.Line > 0 {
		return fmt.Sprintf("line %d:%d: %s", e.Line, e.Column, msg)
	}
	return msg
}

func (e *CompileError) Unwrap() error {
	return e.Err
}

func (c *Compiler) wrapError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*CompileError); ok {
		return err
	}
	if c.currentPos.Line > 0 {
		return &CompileError{
			Err:    err,
			Line:   c.currentPos.Line,
			Column: c.currentPos.Column,
		}
	}
	return err
}

func (c *Compiler) pushPos(node ast.Node) func() {
	prev := c.currentPos
	if node != nil {
		span := ast.SpanOf(node)
		if span.Start.Line > 0 {
			c.currentPos = SourcePos{
				Line:   span.Start.Line,
				Column: span.Start.Column,
			}
		}
	}
	return func() { c.currentPos = prev }
}

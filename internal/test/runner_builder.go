package test

import (
	"strconv"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/token"
)

func buildTestRunner(
	filename string,
	tests []testFunctionInfo,
	quiet bool,
) *ast.FunctionDecl {

	statements := make([]ast.Statement, 0, len(tests)*4+4)
	statements = append(statements,
		makeExpressionStatement(methodCall(
			identifier("test"),
			"setPrefix",
			stringLiteral(filename),
		)),

		makeExpressionStatement(methodCall(
			identifier("test"),
			"setQuiet",
			boolLiteral(quiet),
		)),

		makeVarStatementWithType(
			"results",
			true,
			makeVecOfTestResultType(),
			methodCall(
				identifier("Vec"),
				"new",
			),
		),
	)

	for i, testFn := range tests {
		index := i + 1
		label := filename + ":" + testFn.name
		prefix := filename
		if testFn.line > 0 {
			line := strconv.Itoa(testFn.line)
			label = filename + ":" + line + ":" + testFn.name
			prefix = filename + ":" + line
		}

		statements = append(statements,
			makeExpressionStatement(methodCall(
				identifier("test"),
				"setPrefix",
				stringLiteral(prefix),
			)),
		)

		if testFn.arity == 0 {
			lrName := strfmt.Named("lr{index}",
				"Index", index,
			)

			rName := strfmt.Named("r{index}",
				"Index", index,
			)

			statements = append(statements,
				makeExpressionStatement(callExpression(identifier(testFn.name))),

				makeVarStatement(lrName, false, methodCall(identifier("test"), "takeLastResult")),

				makeSwitchStatement(
					identifier(lrName),
					[]*ast.SwitchCase{
						{
							Values: []ast.Expression{
								enumVariant("Ok", identifier(rName)),
							},
							Body: makeBlockStatement([]ast.Statement{
								makeExpressionStatement(methodCall(
									identifier("results"),
									"push",
									identifier(rName),
								)),
							}),
						},
						{
							Values: []ast.Expression{enumVariant(
								"Err",
								identifier("_e")),
							},
							Body: makeBlockStatement([]ast.Statement{
								makeExpressionStatement(methodCall(
									identifier("results"),
									"push",
									methodCall(
										identifier("test"),
										"failResult",
										stringLiteral(label),
										stringLiteral("test did not call t.finish()"),
									),
								)),
							}),
						},
					}),
			)
			continue
		}

		contextName := strfmt.Named("t{index}",
			"Index", index,
		)

		resultName := strfmt.Named("r{index}",
			"Index", index,
		)

		statements = append(statements,
			makeVarStatement(
				contextName,
				true,
				methodCall(
					identifier("test"),
					"new",
					stringLiteral(label),
				),
			),

			makeExpressionStatement(callExpression(
				identifier(testFn.name),
				borrowExpression(true, identifier(contextName)),
			)),

			makeVarStatement(
				resultName,
				false,
				methodCall(
					identifier("test"),
					"finish",
					borrowExpression(false, identifier(contextName)),
				),
			),

			makeExpressionStatement(methodCall(
				identifier("results"),
				"push",
				identifier(resultName),
			)),
		)
	}

	statements = append(statements,
		makeExpressionStatement(methodCall(
			identifier("test"),
			"setPrefix",
			stringLiteral(filename),
		)),

		makeReturnStatement(methodCall(
			identifier("test"),
			"runTests",
			identifier("results"),
		)),
	)

	return &ast.FunctionDecl{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.FUNC,
				Literal: "func",
			},
		},
		Name: identifier("run_all_tests"),
		ReturnType: &ast.SimpleType{
			Token: token.Token{
				Type:    token.IDENT,
				Literal: "bool",
			},
			Name: "bool",
		},
		Body: makeBlockStatement(statements),
	}
}

func makeVecOfTestResultType() ast.TypeExpression {
	return &ast.GenericType{
		Token: token.Token{
			Type:    token.IDENT,
			Literal: "Vec",
		},
		Name: "Vec",
		TypeParams: []ast.TypeExpression{
			&ast.SimpleType{
				Token: token.Token{
					Type:    token.IDENT,
					Literal: "test.TestResult",
				},
				Name: "test.TestResult",
			},
		},
	}
}

func makeVarStatementWithType(
	name string,
	mutable bool,
	typ ast.TypeExpression,
	value ast.Expression,
) *ast.VarStatement {
	return &ast.VarStatement{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.VAR,
				Literal: "var",
			},
		},
		Mutable: mutable,
		Name:    identifier(name),
		Type:    typ,
		Value:   value,
	}
}

func cloneProgram(program *ast.Program) *ast.Program {
	if program == nil {
		return &ast.Program{
			Statements: []ast.Statement{},
		}
	}

	clone := &ast.Program{
		SourcePath: program.SourcePath,
	}

	clone.Statements = append(clone.Statements, program.Statements...)
	return clone
}

func makeVarStatement(
	name string,
	mutable bool,
	value ast.Expression,
) *ast.VarStatement {
	return &ast.VarStatement{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.VAR,
				Literal: "var",
			},
		},
		Mutable: mutable,
		Name:    identifier(name),
		Value:   value,
	}
}

func makeExpressionStatement(expr ast.Expression) *ast.ExpressionStatement {
	return &ast.ExpressionStatement{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.IDENT,
				Literal: expr.TokenLiteral(),
			},
		},
		Expression: expr,
	}
}

func makeReturnStatement(expr ast.Expression) *ast.ReturnStatement {
	return &ast.ReturnStatement{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.RETURN,
				Literal: "return",
			},
		},
		ReturnValue: expr,
	}
}

func makeSwitchStatement(value ast.Expression, cases []*ast.SwitchCase) *ast.SwitchStatement {
	return &ast.SwitchStatement{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.SWITCH,
				Literal: "switch",
			},
		},
		Value: value,
		Cases: cases,
	}
}

func makeBlockStatement(statements []ast.Statement) *ast.BlockStatement {
	return &ast.BlockStatement{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.LBRACE,
				Literal: "{",
			},
		},
		Statements: statements,
	}
}

func callExpression(
	function ast.Expression,
	args ...ast.Expression,
) ast.Expression {
	return &ast.CallExpression{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.LPAREN,
				Literal: "(",
			},
		},
		Function:  function,
		Arguments: args,
	}
}

func methodCall(
	object ast.Expression,
	method string,
	args ...ast.Expression,
) ast.Expression {
	return &ast.MethodCallExpression{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.DOT,
				Literal: ".",
			},
		},
		Object:    object,
		Method:    identifier(method),
		Arguments: args,
	}
}

func enumVariant(name string, values ...ast.Expression) ast.Expression {
	return &ast.EnumVariantExpression{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.IDENT,
				Literal: name,
			},
		},
		Variant: identifier(name),
		Values:  values,
	}
}

func borrowExpression(mutable bool, value ast.Expression) ast.Expression {
	literal := "&"
	if mutable {
		literal = "&mut"
	}
	return &ast.BorrowExpression{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.AND,
				Literal: literal,
			},
		},
		Mutable: mutable,
		Value:   value,
	}
}

func stringLiteral(value string) ast.Expression {
	return &ast.StringLiteral{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.STRING,
				Literal: value,
			},
		},
		Value: value,
	}
}

func boolLiteral(value bool) ast.Expression {
	literal := "false"
	tok := token.FALSE
	if value {
		literal = "true"
		tok = token.TRUE
	}
	return &ast.BooleanLiteral{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    tok,
				Literal: literal,
			},
		},
		Value: value,
	}
}

func identifier(name string) *ast.Identifier {
	return &ast.Identifier{
		NodeBase: ast.NodeBase{
			Token: token.Token{
				Type:    token.IDENT,
				Literal: name,
			},
		},
		Value: name,
	}
}

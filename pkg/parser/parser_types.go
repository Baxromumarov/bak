package parser

import (
	"strconv"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/token"
)

func (p *Parser) parseTypeExpression() ast.TypeExpression {
	if p.curTokenIs(token.AMPERSAND) {
		return p.parseBorrowType()
	}
	if p.curTokenIs(token.FUNC) {
		return p.parseFunctionType()
	}
	if p.curTokenIs(token.VOID) {
		return &ast.VoidType{Token: p.curToken}
	}
	if p.curTokenIs(token.LPAREN) {
		return p.parseReturnTypes()
	}
	if !p.curTokenIs(token.IDENT) && !token.IsType(p.curToken.Type) {
		p.errors = append(p.errors, p.formatMessage(
			tokenPos(p.curToken),
			"expected a type, got "+p.describeToken(p.curToken),
			"use a built-in type or a declared type name",
		))
		return nil
	}

	name, tok := p.curToken.Literal, p.curToken
	if p.peekTokenIs(token.DOT) {
		p.nextToken()
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		name = name + "." + p.curToken.Literal
		tok = p.curToken
	}
	if p.peekTokenIs(token.LT) {
		return p.parseGenericType(tok, name)
	}
	return &ast.SimpleType{Token: tok, Name: name}
}

func (p *Parser) parseFunctionType() *ast.FunctionType {
	ft := &ast.FunctionType{Token: p.curToken}
	p.pushContext("parsing function type")
	defer p.popContext()

	if !p.expectPeek(token.LPAREN) {
		return nil
	}
	if p.peekTokenIs(token.RPAREN) {
		p.nextToken()
		ft.Params = []ast.TypeExpression{}
	} else if !p.parseFunctionTypeParams(ft) {
		return nil
	}
	if !p.parseFunctionReturnType(ft) {
		return nil
	}
	return ft
}

func (p *Parser) parseFunctionTypeParams(ft *ast.FunctionType) bool {
	p.pushContext("function parameter list")
	p.pushIntent("function parameter list")
	defer func() {
		p.popIntent()
		p.popContext()
	}()

	p.nextToken()
	first := p.parseTypeExpression()
	if first == nil {
		return false
	}
	ft.Params = []ast.TypeExpression{first}
	for p.peekTokenIs(token.COMMA) {
		p.nextToken()
		p.nextToken()
		next := p.parseTypeExpression()
		if next == nil {
			return false
		}
		ft.Params = append(ft.Params, next)
	}
	return p.expectPeek(token.RPAREN)
}

func (p *Parser) parseFunctionReturnType(ft *ast.FunctionType) bool {
	p.pushContext("function return type")
	p.pushIntent("function return type")
	defer func() {
		p.popIntent()
		p.popContext()
	}()

	if !p.expectPeek(token.ARROW) || !p.expectPeek(token.LPAREN) {
		return false
	}
	ft.ReturnType = p.parseReturnTypes()
	return true
}

func (p *Parser) parseBorrowType() *ast.BorrowType {
	borrow := &ast.BorrowType{Token: p.curToken}
	p.nextToken()
	if p.curTokenIs(token.MUT) {
		borrow.Mutable = true
		p.nextToken()
	}
	borrow.Inner = p.parseTypeExpression()
	return borrow
}

func (p *Parser) parseGenericType(tok token.Token, name string) ast.TypeExpression {
	generic := &ast.GenericType{Token: tok, Name: name, TypeParams: []ast.TypeExpression{}}
	p.nextToken() // consume <
	p.nextToken()
	if p.curTokenIs(token.GT) {
		p.errors = append(p.errors, p.formatMessage(
			tokenPos(p.curToken),
			"expected type parameter after '<' in generic type",
			"",
		))
		return generic
	}

	generic.TypeParams = append(generic.TypeParams, p.parseGenericTypeArgument())
	for p.peekTokenIs(token.COMMA) {
		p.nextToken()
		p.nextToken()
		if p.curTokenIs(token.GT) {
			p.errors = append(p.errors, p.formatMessage(
				tokenPos(p.curToken),
				"expected type parameter after ',' in generic type",
				"",
			))
			return generic
		}
		generic.TypeParams = append(generic.TypeParams, p.parseGenericTypeArgument())
	}

	if p.peekTokenIs(token.GT_EQ) {
		p.splitCompoundToken(token.GT, ">", token.ASSIGN, "=")
	} else if p.peekTokenIs(token.RSHIFT) {
		p.splitCompoundToken(token.GT, ">", token.GT, ">")
	} else if !p.expectPeek(token.GT) {
		return nil
	}

	if name != "Vec" && tok.Type != token.VEC {
		return generic
	}
	if len(generic.TypeParams) == 2 {
		if size, ok := generic.TypeParams[1].(*ast.SizeExpression); ok {
			if size.IsDynamic {
				return generic
			}
			return &ast.ArrayType{
				Span: generic.Span, Token: tok,
				ElemType: generic.TypeParams[0], Size: size.Value,
			}
		}
		if simple, ok := generic.TypeParams[1].(*ast.SimpleType); ok && simple.Name == "_" {
			return generic
		}
	}
	if len(generic.TypeParams) == 1 {
		generic.TypeParams = append(generic.TypeParams, &ast.SizeExpression{IsDynamic: true})
	}
	return generic
}

func (p *Parser) parseGenericTypeArgument() ast.TypeExpression {
	switch {
	case p.curTokenIs(token.UNDERSCORE):
		return &ast.SizeExpression{Token: p.curToken, IsDynamic: true}
	case p.curTokenIs(token.INT):
		value, _ := strconv.ParseInt(p.curToken.Literal, 10, 64)
		return &ast.SizeExpression{Token: p.curToken, Value: value}
	default:
		return p.parseTypeExpression()
	}
}

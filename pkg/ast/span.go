package ast

import (
	"reflect"

	"github.com/baxromumarov/bak/pkg/token"
)

// PopulateSpans fills Span for all nodes in the AST.
func PopulateSpans(prog *Program) {
	if prog == nil {
		return
	}
	spanNode(prog)
}

// SpanOf returns the span for any AST node if present.
func SpanOf(node Node) Span {
	if node == nil {
		return Span{}
	}
	v := reflect.ValueOf(node)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return Span{}
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return Span{}
	}
	f := v.FieldByName("Span")
	if !f.IsValid() || !f.CanInterface() {
		return Span{}
	}
	span, ok := f.Interface().(Span)
	if !ok {
		return Span{}
	}
	return span
}

func spanNode(node Node) Span {
	if isNilNode(node) {
		return Span{}
	}
	switch n := node.(type) {
	case *Program:
		n.Span = spanFromStatements(n.Statements)
		return n.Span
	case *PackageStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Name))
		return n.Span
	case *ImportStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), tokenSpan(n.PathToken), tokenSpan(n.AliasToken))
		return n.Span
	case *ImportBlock:
		n.Span = mergeSpans(tokenSpan(n.Token), spanFromStatements(importStatementsToStatements(n.Imports)))
		return n.Span
	case *VarStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Name), spanNode(n.Type), spanNode(n.Value))
		return n.Span
	case *MultiVarStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanFromIdentifiers(n.Names), spanNode(n.Value), spanFromTypes(n.Types))
		return n.Span
	case *ConstStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Name), spanNode(n.Type), spanNode(n.Value))
		return n.Span
	case *ConstBlock:
		n.Span = mergeSpans(tokenSpan(n.Token), spanFromStatements(constStatementsToStatements(n.Constants)))
		return n.Span
	case *VarBlock:
		n.Span = mergeSpans(tokenSpan(n.Token), spanFromStatements(varStatementsToStatements(n.Variables)))
		return n.Span
	case *ReturnStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.ReturnValue))
		return n.Span
	case *ExpressionStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Expression))
		return n.Span
	case *BlockStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanFromStatements(n.Statements))
		return n.Span
	case *IfStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Condition), spanNode(n.Consequence), spanNode(n.Alternative))
		return n.Span
	case *WhileStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Condition), spanNode(n.Body))
		return n.Span
	case *ForStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Variable), spanNode(n.Iterable), spanNode(n.Body))
		return n.Span
	case *SwitchStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Value), spanFromSwitchCases(n.Cases))
		return n.Span
	case *DeferStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Body))
		return n.Span
	case *PanicStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Message))
		return n.Span
	case *UnsafeBlock:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Body))
		return n.Span
	case *BreakStatement:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *ContinueStatement:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *AssignmentStatement:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Left), spanNode(n.Value))
		return n.Span
	case *FunctionDecl:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Name), spanFromTypeParameters(n.TypeParams), spanFromParameters(n.Parameters), spanNode(n.ReturnType), spanNode(n.Body))
		return n.Span
	case *StructDecl:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Name), spanFromTypeParameters(n.TypeParams), spanFromStructFields(n.Fields))
		return n.Span
	case *TypeDecl:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Name), spanNode(n.Underlying))
		return n.Span
	case *AliasDecl:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Name), spanNode(n.Underlying))
		return n.Span
	case *EnumDecl:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Name), spanFromTypeParameters(n.TypeParams), spanFromEnumVariants(n.Variants))
		return n.Span
	case *ImplDecl:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.TypeName), spanNode(n.Receiver), spanFromMethodDecls(n.Methods))
		return n.Span
	case *Identifier:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *MutableIdentifier:
		n.Span = mergeSpans(tokenSpan(n.Token), tokenSpan(n.NameToken))
		return n.Span
	case *IntegerLiteral:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *FloatLiteral:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *StringLiteral:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *CharLiteral:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *BooleanLiteral:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *VoidLiteral:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *PrefixExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Right))
		return n.Span
	case *InfixExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Left), spanNode(n.Right))
		return n.Span
	case *CallExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Function), spanFromExpressions(n.Arguments))
		return n.Span
	case *TypeConversion:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Value))
		return n.Span
	case *MethodCallExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Object), spanNode(n.Method), spanFromExpressions(n.Arguments))
		return n.Span
	case *FieldAccessExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Object), spanNode(n.Field))
		return n.Span
	case *IndexExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Left), spanNode(n.Index))
		return n.Span
	case *StructLiteral:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Name), spanFromMapValues(n.Fields))
		return n.Span
	case *VecLiteral:
		n.Span = mergeSpans(tokenSpan(n.Token), spanFromExpressions(n.Elements))
		return n.Span
	case *TupleExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanFromExpressions(n.Elements))
		return n.Span
	case *RangeExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Start), spanNode(n.End))
		return n.Span
	case *EnumVariantExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Variant), spanFromExpressions(n.Values))
		return n.Span
	case *BorrowExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Value))
		return n.Span
	case *BoxExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Value))
		return n.Span
	case *DerefExpression:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Value))
		return n.Span
	case *UnwrapExpression:
		n.Span = mergeSpans(spanNode(n.Value), tokenSpan(n.Token))
		return n.Span
	case *FunctionLiteral:
		n.Span = mergeSpans(tokenSpan(n.Token), spanFromParameters(n.Parameters), spanNode(n.ReturnType), spanNode(n.Body))
		return n.Span
	case *SimpleType:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *GenericType:
		n.Span = mergeSpans(tokenSpan(n.Token), spanFromTypes(n.TypeParams))
		return n.Span
	case *BorrowType:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Inner))
		return n.Span
	case *BoxType:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Inner))
		return n.Span
	case *BoxOptionalType:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Inner))
		return n.Span
	case *SizeExpression:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *VoidType:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *ErrorType:
		n.Span = tokenSpan(n.Token)
		return n.Span
	case *TupleType:
		n.Span = mergeSpans(tokenSpan(n.Token), spanFromTypes(n.Elements))
		return n.Span
	case *FunctionType:
		n.Span = mergeSpans(tokenSpan(n.Token), spanFromTypes(n.Params), spanNode(n.ReturnType))
		return n.Span
	case *NamedType:
		n.Span = mergeSpans(tokenSpan(n.Token), spanNode(n.Type))
		return n.Span
	}
	return Span{}
}

func isNilNode(node Node) bool {
	if node == nil {
		return true
	}
	v := reflect.ValueOf(node)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

func spanFromStatements(stmts []Statement) Span {
	spans := make([]Span, 0, len(stmts))
	for _, s := range stmts {
		spans = append(spans, spanNode(s))
	}
	return mergeSpans(spans...)
}

func spanFromExpressions(exprs []Expression) Span {
	spans := make([]Span, 0, len(exprs))
	for _, e := range exprs {
		spans = append(spans, spanNode(e))
	}
	return mergeSpans(spans...)
}

func spanFromMapValues(values map[string]Expression) Span {
	spans := make([]Span, 0, len(values))
	for _, v := range values {
		spans = append(spans, spanNode(v))
	}
	return mergeSpans(spans...)
}

func spanFromTypes(types []TypeExpression) Span {
	spans := make([]Span, 0, len(types))
	for _, t := range types {
		spans = append(spans, spanNode(t))
	}
	return mergeSpans(spans...)
}

func spanFromTypeParameters(params []*TypeParameter) Span {
	spans := make([]Span, 0, len(params))
	for _, p := range params {
		spans = append(spans, spanNode(p))
	}
	return mergeSpans(spans...)
}

func spanFromIdentifiers(ids []*Identifier) Span {
	spans := make([]Span, 0, len(ids))
	for _, id := range ids {
		spans = append(spans, spanNode(id))
	}
	return mergeSpans(spans...)
}

func spanFromParameters(params []*Parameter) Span {
	spans := make([]Span, 0, len(params))
	for _, p := range params {
		spans = append(spans, spanParameter(p))
	}
	return mergeSpans(spans...)
}

func spanFromStructFields(fields []*StructField) Span {
	spans := make([]Span, 0, len(fields))
	for _, f := range fields {
		spans = append(spans, spanStructField(f))
	}
	return mergeSpans(spans...)
}

func spanFromEnumVariants(vars []*EnumVariant) Span {
	spans := make([]Span, 0, len(vars))
	for _, v := range vars {
		spans = append(spans, spanEnumVariant(v))
	}
	return mergeSpans(spans...)
}

func spanFromSwitchCases(cases []*SwitchCase) Span {
	spans := make([]Span, 0, len(cases))
	for _, c := range cases {
		spans = append(spans, spanSwitchCase(c))
	}
	return mergeSpans(spans...)
}

func spanFromMethodDecls(methods []*MethodDecl) Span {
	spans := make([]Span, 0, len(methods))
	for _, m := range methods {
		spans = append(spans, spanMethodDecl(m))
	}
	return mergeSpans(spans...)
}

func spanParameter(p *Parameter) Span {
	if p == nil {
		return Span{}
	}
	p.Span = mergeSpans(tokenSpan(p.Token), spanNode(p.Name), spanNode(p.Type))
	return p.Span
}

func spanStructField(f *StructField) Span {
	if f == nil {
		return Span{}
	}
	f.Span = mergeSpans(tokenSpan(f.Token), spanNode(f.Name), spanNode(f.Type))
	return f.Span
}

func spanEnumVariant(v *EnumVariant) Span {
	if v == nil {
		return Span{}
	}
	v.Span = mergeSpans(tokenSpan(v.Token), spanNode(v.Name), spanFromTypes(v.Fields))
	return v.Span
}

func spanSwitchCase(c *SwitchCase) Span {
	if c == nil {
		return Span{}
	}
	c.Span = mergeSpans(tokenSpan(c.Token), spanFromExpressions(c.Values), spanNode(c.Body))
	return c.Span
}

func spanMethodDecl(m *MethodDecl) Span {
	if m == nil {
		return Span{}
	}
	m.Span = mergeSpans(tokenSpan(m.Token), spanNode(m.Name), spanFromTypeParameters(m.TypeParams), spanFromParameters(m.Parameters), spanNode(m.ReturnType), spanNode(m.Body))
	return m.Span
}

func importStatementsToStatements(items []*ImportStatement) []Statement {
	out := make([]Statement, 0, len(items))
	for _, it := range items {
		out = append(out, it)
	}
	return out
}

func constStatementsToStatements(items []*ConstStatement) []Statement {
	out := make([]Statement, 0, len(items))
	for _, it := range items {
		out = append(out, it)
	}
	return out
}

func varStatementsToStatements(items []*VarStatement) []Statement {
	out := make([]Statement, 0, len(items))
	for _, it := range items {
		out = append(out, it)
	}
	return out
}

func tokenSpan(tok token.Token) Span {
	if tok.Line <= 0 || tok.Column <= 0 {
		return Span{}
	}
	endLine := tok.EndLine
	endColumn := tok.EndColumn
	if endLine == 0 || endColumn == 0 {
		endLine = tok.Line
		endColumn = tok.Column + len(tok.Literal)
	}
	return Span{
		Start: Position{Line: tok.Line, Column: tok.Column},
		End:   Position{Line: endLine, Column: endColumn},
	}
}

func mergeSpans(spans ...Span) Span {
	merged := Span{}
	for _, span := range spans {
		merged = mergeSpan(merged, span)
	}
	return merged
}

func mergeSpan(a, b Span) Span {
	if isZeroSpan(a) {
		return b
	}
	if isZeroSpan(b) {
		return a
	}
	start := a.Start
	if posLess(b.Start, start) {
		start = b.Start
	}
	end := a.End
	if posLess(end, b.End) {
		end = b.End
	}
	return Span{Start: start, End: end}
}

func isZeroSpan(s Span) bool {
	return s.Start.Line == 0 || s.End.Line == 0
}

func posLess(a, b Position) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Column < b.Column
}

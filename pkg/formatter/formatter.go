// Package formatter provides formatting for bak source code.
package formatter

import (
	"bytes"
	"reflect"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/token"
	"github.com/baxromumarov/bak/pkg/typestr"
)

const indentUnit = "    "
const maxLineLength = 80

// Comment represents a source comment with position.
type Comment struct {
	Line   int
	Column int
	Text   string
}

// Format parses and formats bak source code.
func Format(input string) (string, []string) {
	l := lexer.New(input)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return "", p.Errors()
	}

	comments := ScanComments(input)
	return FormatProgram(program, comments), nil
}

// FormatProgram formats a parsed program.
func FormatProgram(program *ast.Program, comments []Comment) string {
	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].Line != comments[j].Line {
			return comments[i].Line < comments[j].Line
		}
		return comments[i].Column < comments[j].Column
	})

	p := &printer{
		comments:   comments,
		needIndent: true,
	}
	p.printProgram(program)
	out := p.buf.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

// ScanComments extracts line and block comments from source.
func ScanComments(input string) []Comment {
	var comments []Comment
	line, col := 1, 0
	i := 0

	for i < len(input) {
		ch := input[i]

		if ch == '\n' {
			line++
			col = 0
			i++
			continue
		}

		col++

		if ch == '"' {
			i++
			for i < len(input) {
				ch = input[i]
				if ch == '\n' {
					line++
					col = 0
					i++
					continue
				}
				col++
				if ch == '\\' {
					i++
					if i < len(input) {
						if input[i] == '\n' {
							line++
							col = 0
						} else {
							col++
						}
						i++
					}
					continue
				}
				if ch == '"' {
					i++
					break
				}
				i++
			}
			continue
		}

		if ch == '\'' {
			i++
			for i < len(input) {
				ch = input[i]
				if ch == '\n' {
					line++
					col = 0
					i++
					continue
				}
				col++
				if ch == '\\' {
					i++
					if i < len(input) {
						if input[i] == '\n' {
							line++
							col = 0
						} else {
							col++
						}
						i++
					}
					continue
				}
				if ch == '\'' {
					i++
					break
				}
				i++
			}
			continue
		}

		if ch == '/' && i+1 < len(input) {
			next := input[i+1]
			if next == '/' {
				startLine, startCol := line, col
				start := i
				i += 2
				col++
				for i < len(input) && input[i] != '\n' {
					i++
					col++
				}
				comments = append(comments, Comment{
					Line:   startLine,
					Column: startCol,
					Text:   input[start:i],
				})
				continue
			}
			if next == '*' {
				startLine, startCol := line, col
				start := i
				i += 2
				col++
				for i < len(input) {
					ch = input[i]
					if ch == '\n' {
						line++
						col = 0
						i++
						continue
					}
					col++
					if ch == '*' && i+1 < len(input) && input[i+1] == '/' {
						i += 2
						col++
						break
					}
					i++
				}
				comments = append(comments, Comment{
					Line:   startLine,
					Column: startCol,
					Text:   input[start:i],
				})
				continue
			}
		}

		i++
	}

	return comments
}

type printer struct {
	buf          bytes.Buffer
	indent       int
	needIndent   bool
	comments     []Comment
	commentIndex int
	column       int
}

func (p *printer) printProgram(program *ast.Program) {
	for i, stmt := range program.Statements {
		line, col := statementPosition(stmt)
		p.flushCommentsBefore(line, col)
		p.writeIndent()
		p.printStatement(stmt)
		if i < len(program.Statements)-1 {
			p.newline()
			if needsBlankLine(stmt, program.Statements[i+1]) {
				p.newline()
			}
		}
	}
	p.flushCommentsBefore(maxInt, maxInt)
}

func isImportStatement(stmt ast.Statement) bool {
	switch stmt.(type) {
	case *ast.ImportStatement, *ast.ImportBlock:
		return true
	default:
		return false
	}
}

func isTopLevelDecl(stmt ast.Statement) bool {
	switch stmt.(type) {
	case *ast.FunctionDecl, *ast.StructDecl, *ast.EnumDecl, *ast.TypeDecl, *ast.AliasDecl, *ast.ImplDecl:
		return true
	default:
		return false
	}
}

func isTypeAliasDecl(stmt ast.Statement) bool {
	switch stmt.(type) {
	case *ast.TypeDecl, *ast.AliasDecl:
		return true
	default:
		return false
	}
}

func isVarConstDecl(stmt ast.Statement) bool {
	switch stmt.(type) {
	case *ast.VarStatement, *ast.VarBlock, *ast.MultiVarStatement, *ast.ConstStatement, *ast.ConstBlock:
		return true
	default:
		return false
	}
}

func needsBlankLine(curr, next ast.Statement) bool {
	if _, ok := curr.(*ast.PackageStatement); ok {
		return true
	}
	if isImportStatement(curr) && !isImportStatement(next) {
		return true
	}
	if isTypeAliasDecl(curr) && isTypeAliasDecl(next) {
		return false
	}
	if isTopLevelDecl(curr) && isTopLevelDecl(next) {
		return true
	}
	if isTopLevelDecl(curr) && isVarConstDecl(next) {
		return true
	}
	if isVarConstDecl(curr) && isTopLevelDecl(next) {
		return true
	}
	return false
}

func needsInnerBlankLine(curr, next ast.Statement) bool {
	// 1. Separate variable declarations from other statements
	if isVarConstDecl(curr) != isVarConstDecl(next) {
		return true
	}

	// 2. Separate control structures
	if isControlStructure(curr) || isControlStructure(next) {
		return true
	}

	// 3. Separate returns
	if _, ok := next.(*ast.ReturnStatement); ok {
		return true
	}
	// Also separate panic?
	if _, ok := next.(*ast.PanicStatement); ok {
		return true
	}

	return false
}

func isControlStructure(stmt ast.Statement) bool {
	switch stmt.(type) {
	case *ast.IfStatement, *ast.ForStatement, *ast.WhileStatement, *ast.SwitchStatement, *ast.UnsafeBlock:
		return true
	default:
		return false
	}
}

func (p *printer) printStatements(stmts []ast.Statement) {
	for i, stmt := range stmts {
		line, col := statementPosition(stmt)
		p.flushCommentsBefore(line, col)
		p.writeIndent()
		p.printStatement(stmt)
		p.newline()

		if i < len(stmts)-1 {
			next := stmts[i+1]
			if needsInnerBlankLine(stmt, next) {
				// Avoid double newlines if comments already caused spacing or if we just printed one
				// But printer handles newlines cleanly. Just add one more.
				p.newline()
			}
		}
	}
}

func (p *printer) printStatement(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.PackageStatement:
		p.write("package ")
		p.write(s.Name.Value)
	case *ast.ImportStatement:
		p.write("import ")
		p.write("\"")
		p.write(s.Path)
		p.write("\"")
		if s.Alias != "" {
			p.write(" as ")
			p.write(s.Alias)
		}
	case *ast.ImportBlock:
		p.write("import ")
		p.write("(")
		p.newline()
		p.indent++
		for _, imp := range s.Imports {
			line, col := statementPosition(imp)
			p.flushCommentsBefore(line, col)
			p.writeIndent()
			p.write("\"")
			p.write(imp.Path)
			p.write("\"")
			if imp.Alias != "" {
				p.write(" as ")
				p.write(imp.Alias)
			}
			p.newline()
		}
		p.indent--
		p.writeIndent()
		p.write(")")
	case *ast.VarStatement:
		if s.Mutable {
			p.write("mut ")
		}
		p.write("var ")
		p.write(s.Name.Value)
		if s.Type != nil {
			p.write(": ")
			p.write(formatType(s.Type))
		}
		if s.Value != nil {
			p.write(" = ")
			p.printExpression(s.Value, precLowest)
		}
	case *ast.VarBlock:
		p.printVarBlock(s)
	case *ast.MultiVarStatement:
		if s.Mutable {
			p.write("mut ")
		}
		p.write("var (")
		for i, name := range s.Names {
			if i > 0 {
				p.write(", ")
			}
			p.write(name.Value)
		}
		p.write(") = ")
		p.printExpression(s.Value, precLowest)
	case *ast.ConstStatement:
		p.write(s.Visibility.String())
		p.write("const ")
		p.write(s.Name.Value)
		if s.Type != nil {
			p.write(": ")
			p.write(formatType(s.Type))
		}
		p.write(" = ")
		p.printExpression(s.Value, precLowest)
	case *ast.ConstBlock:
		p.write("const ")
		p.write("(")
		p.newline()
		p.indent++
		for _, c := range s.Constants {
			line, col := statementPosition(c)
			p.flushCommentsBefore(line, col)
			p.writeIndent()
			p.write(c.Name.Value)
			if c.Type != nil {
				p.write(": ")
				p.write(formatType(c.Type))
			}
			p.write(" = ")
			p.printExpression(c.Value, precLowest)
			p.newline()
		}
		p.indent--
		p.writeIndent()
		p.write(")")
	case *ast.ReturnStatement:
		p.write("return")
		if s.ReturnValue != nil {
			p.write(" ")
			if tuple, ok := s.ReturnValue.(*ast.TupleExpression); ok {
				p.printExpressionList(tuple.Elements, false)
			} else {
				p.printExpression(s.ReturnValue, precLowest)
			}
		}
	case *ast.ExpressionStatement:
		p.printExpression(s.Expression, precLowest)
	case *ast.AssignmentStatement:
		p.printExpression(s.Left, precLowest)
		p.write(" = ")
		p.printExpression(s.Value, precLowest)
	case *ast.BlockStatement:
		p.printBlock(s)
	case *ast.IfStatement:
		p.printIfStatement(s)
	case *ast.WhileStatement:
		p.write("while ")
		p.printExpression(s.Condition, precLowest)
		p.write(" ")
		p.printBlock(s.Body)
	case *ast.ForStatement:
		p.write("for ")
		p.write(s.Variable.Value)
		p.write(" in ")
		p.printExpression(s.Iterable, precLowest)
		p.write(" ")
		p.printBlock(s.Body)
	case *ast.SwitchStatement:
		p.printSwitchStatement(s)
	case *ast.DeferStatement:
		p.write("defer ")
		if s.Body != nil {
			p.printBlock(s.Body)
		} else {
			p.write("{}")
		}
	case *ast.PanicStatement:
		p.write("panic ")
		p.printExpression(s.Message, precLowest)
	case *ast.UnsafeBlock:
		p.write("unsafe ")
		p.printBlock(s.Body)
	case *ast.BreakStatement:
		p.write("break")
	case *ast.ContinueStatement:
		p.write("continue")
	case *ast.FunctionDecl:
		p.printFunctionDecl(s)
	case *ast.StructDecl:
		p.printStructDecl(s)
	case *ast.TypeDecl:
		p.write(s.Visibility.String())
		p.write("type ")
		p.write(s.Name.Value)
		p.write(" = ")
		p.write(formatType(s.Underlying))
	case *ast.AliasDecl:
		p.write(s.Visibility.String())
		p.write("alias ")
		p.write(s.Name.Value)
		p.write(" = ")
		p.write(formatType(s.Underlying))
	case *ast.EnumDecl:
		p.printEnumDecl(s)
	case *ast.ImplDecl:
		p.printImplDecl(s)
	}
}

func (p *printer) printVarBlock(vb *ast.VarBlock) {
	p.write("var ")
	p.write("(")
	p.newline()
	p.indent++
	for _, v := range vb.Variables {
		line, col := statementPosition(v)
		p.flushCommentsBefore(line, col)
		p.writeIndent()
		p.write(v.Name.Value)
		if v.Type != nil {
			p.write(": ")
			p.write(formatType(v.Type))
		}
		if v.Value != nil {
			p.write(" = ")
			p.printExpression(v.Value, precLowest)
		}
		p.newline()
	}
	p.indent--
	p.writeIndent()
	p.write(")")
}

func (p *printer) printIfStatement(stmt *ast.IfStatement) {
	p.write("if ")
	p.printExpression(stmt.Condition, precLowest)
	p.write(" ")
	p.printBlock(stmt.Consequence)

	if stmt.Alternative != nil {
		if elseIf := unwrapElseIf(stmt.Alternative); elseIf != nil {
			p.write(" else ")
			p.printIfStatement(elseIf)
			return
		}
		p.write(" else ")
		p.printBlock(stmt.Alternative)
	}
}

func (p *printer) printSwitchStatement(stmt *ast.SwitchStatement) {
	p.write("switch ")
	p.printExpression(stmt.Value, precLowest)
	p.write(" ")
	p.write("{")
	p.newline()
	p.indent++
	for _, c := range stmt.Cases {
		if c.Token.Line != 0 {
			p.flushCommentsBefore(c.Token.Line, c.Token.Column)
		}
		p.writeIndent()
		if c.Default {
			p.write("default")
		} else {
			p.write("case ")
			p.printExpressionList(c.Values, false)
		}
		p.write(" ")
		p.printBlock(c.Body)
		p.newline()
	}
	p.indent--
	p.writeIndent()
	p.write("}")
}

func (p *printer) printFunctionDecl(fn *ast.FunctionDecl) {
	p.write(fn.Visibility.String())
	p.write("func ")
	p.write(fn.Name.Value)
	if len(fn.TypeParams) > 0 {
		p.write("<")
		for i, param := range fn.TypeParams {
			if i > 0 {
				p.write(", ")
			}
			p.write(param.String())
		}
		p.write(">")
	}
	p.write("(")
	returnType := formatType(fn.ReturnType)
	suffixLen := len(") -> (") + len(returnType) + len(") {")
	multiline := p.shouldMultilineParams(fn.Parameters, suffixLen)
	p.printParameters(fn.Parameters, multiline)
	p.write(") -> (")
	p.write(returnType)
	p.write(") ")
	p.printBlock(fn.Body)
}

func (p *printer) printStructDecl(sd *ast.StructDecl) {
	p.write(sd.Visibility.String())
	p.write("struct ")
	p.write(sd.Name.Value)
	if len(sd.TypeParams) > 0 {
		p.write("<")
		for i, param := range sd.TypeParams {
			if i > 0 {
				p.write(", ")
			}
			p.write(param.String())
		}
		p.write(">")
	}
	p.write(" ")
	p.write("{")
	p.newline()
	p.indent++
	maxFieldLen := 0
	for _, field := range sd.Fields {
		fieldPrefix := field.Visibility.String() + field.Name.Value
		if len(fieldPrefix) > maxFieldLen {
			maxFieldLen = len(fieldPrefix)
		}
	}
	for _, field := range sd.Fields {
		p.writeIndent()
		fieldPrefix := field.Visibility.String() + field.Name.Value
		p.write(fieldPrefix)
		padding := max(maxFieldLen-len(fieldPrefix), 0)
		p.write(":")
		p.write(strings.Repeat(" ", padding+1))
		p.write(formatType(field.Type))
		p.newline()
	}
	p.indent--
	p.writeIndent()
	p.write("}")
}

func (p *printer) printEnumDecl(ed *ast.EnumDecl) {
	p.write(ed.Visibility.String())
	p.write("enum ")
	p.write(ed.Name.Value)
	if len(ed.TypeParams) > 0 {
		p.write("<")
		for i, param := range ed.TypeParams {
			if i > 0 {
				p.write(", ")
			}
			p.write(param.String())
		}
		p.write(">")
	}
	p.write(" ")
	p.write("{")
	p.newline()
	p.indent++
	for _, variant := range ed.Variants {
		p.writeIndent()
		p.write(variant.Name.Value)
		if len(variant.Fields) > 0 {
			p.write("(")
			for i, field := range variant.Fields {
				if i > 0 {
					p.write(", ")
				}
				p.write(formatType(field))
			}
			p.write(")")
		}
		p.newline()
	}
	p.indent--
	p.writeIndent()
	p.write("}")
}

func (p *printer) printImplDecl(id *ast.ImplDecl) {
	p.write("impl ")
	p.write(id.TypeName.Value)
	p.write(" as ")
	p.write(id.Receiver.Value)
	p.write(" ")
	p.write("{")
	p.newline()
	p.indent++
	for i, method := range id.Methods {
		if method.Token.Line != 0 {
			p.flushCommentsBefore(method.Token.Line, method.Token.Column)
		}
		p.writeIndent()
		p.printMethodDecl(method)
		p.newline()
		if i < len(id.Methods)-1 {
			nextMethod := id.Methods[i+1]
			if !p.hasPendingCommentBefore(nextMethod.Token.Line, nextMethod.Token.Column) {
				p.newline()
			}
		}
	}
	p.indent--
	p.writeIndent()
	p.write("}")
}

func (p *printer) printMethodDecl(md *ast.MethodDecl) {
	p.write(md.Visibility.String())
	if md.Mutable {
		p.write("mut ")
	}
	p.write("func ")
	p.write(md.Name.Value)
	p.write("(")
	returnType := formatType(md.ReturnType)
	suffixLen := len(") -> (") + len(returnType) + len(") {")
	multiline := p.shouldMultilineParams(md.Parameters, suffixLen)
	p.printParameters(md.Parameters, multiline)
	p.write(") -> (")
	p.write(returnType)
	p.write(") ")
	p.printBlock(md.Body)
}

func (p *printer) printParameters(params []*ast.Parameter, multiline bool) {
	if multiline {
		p.newline()
		p.indent++
		for i, param := range params {
			p.writeIndent()
			if param.Mutable {
				p.write("mut ")
			}
			p.write(param.Name.Value)
			p.write(": ")
			p.write(formatType(param.Type))
			if i < len(params)-1 || true {
				p.write(",")
			}
			p.newline()
		}
		p.indent--
		p.writeIndent()
		return
	}

	for i, param := range params {
		if i > 0 {
			p.write(", ")
		}
		if param.Mutable {
			p.write("mut ")
		}
		p.write(param.Name.Value)
		p.write(": ")
		p.write(formatType(param.Type))
	}
}

func (p *printer) shouldMultilineParams(params []*ast.Parameter, suffixLen int) bool {
	if len(params) > 4 {
		return true
	}
	totalLen := p.column + p.parametersInlineLength(params) + suffixLen
	return totalLen > maxLineLength
}

func (p *printer) parametersInlineLength(params []*ast.Parameter) int {
	total := 0
	for i, param := range params {
		if i > 0 {
			total += len(", ")
		}
		total += p.parameterInlineLength(param)
	}
	return total
}

func (p *printer) parameterInlineLength(param *ast.Parameter) int {
	length := len(param.Name.Value) + len(": ") + len(formatType(param.Type))
	if param.Mutable {
		length += len("mut ")
	}
	return length
}

func (p *printer) expressionInlineLength(expr ast.Expression) int {
	if expr == nil {
		return 0
	}
	return len(expr.String())
}

func (p *printer) printBlock(block *ast.BlockStatement) {
	p.write("{")
	p.newline()
	p.indent++
	p.printStatements(block.Statements)
	p.indent--
	p.writeIndent()
	p.write("}")
}

func (p *printer) printExpression(expr ast.Expression, parentPrec int) {
	if expr == nil {
		return
	}

	exprPrec := precedence(expr)
	needsParens := exprPrec != precLowest && exprPrec < parentPrec
	if needsParens {
		p.write("(")
	}

	switch e := expr.(type) {
	case *ast.Identifier:
		p.write(e.Value)
	case *ast.MutableIdentifier:
		p.write("mut ")
		p.write(e.Value)
	case *ast.IntegerLiteral:
		p.write(e.Token.Literal)
	case *ast.FloatLiteral:
		p.write(e.Token.Literal)
	case *ast.StringLiteral:
		p.write("\"")
		p.write(e.Value)
		p.write("\"")
	case *ast.CharLiteral:
		p.write("'")
		p.write(string(e.Value))
		p.write("'")
	case *ast.BooleanLiteral:
		if e.Value {
			p.write("true")
		} else {
			p.write("false")
		}
	case *ast.VoidLiteral:
		p.write("void")
	case *ast.PrefixExpression:
		p.write(e.Operator)
		p.printExpression(e.Right, precPrefix)
	case *ast.InfixExpression:
		prec := infixPrecedence(e.Operator)
		p.printExpression(e.Left, prec)
		p.write(" ")
		p.write(e.Operator)
		p.write(" ")
		p.printExpression(e.Right, prec+1)
	case *ast.CallExpression:
		p.printExpression(e.Function, precCall)
		p.write("(")
		multiline := isSourceMultiline(e) || p.isComplex(e)
		p.printExpressionList(e.Arguments, multiline)
		p.write(")")
	case *ast.MethodCallExpression:
		p.printExpression(e.Object, precDot)
		p.write(".")
		p.write(e.Method.Value)
		p.write("(")
		multiline := isSourceMultiline(e) || p.isComplex(e)
		p.printExpressionList(e.Arguments, multiline)
		p.write(")")
	case *ast.FieldAccessExpression:
		p.printExpression(e.Object, precDot)
		p.write(".")
		p.write(e.Field.Value)
	case *ast.IndexExpression:
		p.printExpression(e.Left, precIndex)
		p.write("[")
		p.printExpression(e.Index, precLowest)
		p.write("]")
	case *ast.StructLiteral:
		p.printStructLiteral(e)
	case *ast.VecLiteral:
		p.printVecLiteral(e)
	case *ast.TupleExpression:
		p.write("(")
		multiline := isSourceMultiline(e)
		p.printExpressionList(e.Elements, multiline)
		p.write(")")
	case *ast.RangeExpression:
		p.printRangeExpression(e)
	case *ast.EnumVariantExpression:
		p.write(e.Variant.Value)
		if len(e.Values) > 0 {
			p.write("(")
			multiline := isSourceMultiline(e)
			p.printExpressionList(e.Values, multiline)
			p.write(")")
		}
	case *ast.BorrowExpression:
		if e.Mutable {
			p.write("&mut ")
		} else {
			p.write("&")
		}
		p.printExpression(e.Value, precPrefix)
	case *ast.BoxExpression:
		p.write("box ")
		p.printExpression(e.Value, precPrefix)
	case *ast.DerefExpression:
		p.write("*")
		p.printExpression(e.Value, precPrefix)
	case *ast.FunctionLiteral:
		p.write("func(")
		returnType := formatType(e.ReturnType)
		suffixLen := len(") -> (") + len(returnType) + len(") {")
		multiline := p.shouldMultilineParams(e.Parameters, suffixLen)
		p.printParameters(e.Parameters, multiline)
		p.write(") -> (")
		p.write(returnType)
		p.write(") ")
		p.printBlock(e.Body)
	case *ast.TypeConversion:
		p.write(e.TypeName)
		p.write("(")
		p.printExpression(e.Value, precLowest)
		p.write(")")
	}

	if needsParens {
		p.write(")")
	}
}

func (p *printer) printRangeExpression(re *ast.RangeExpression) {
	if re.Token.Type == token.DOTDOT {
		p.printExpression(re.Start, precLessGreater)
		p.write("..")
		p.printExpression(re.End, precLessGreater+1)
		return
	}

	if re.StartInclusive {
		p.write("[")
	} else {
		p.write("(")
	}
	p.printExpression(re.Start, precLowest)
	p.write(", ")
	p.printExpression(re.End, precLowest)
	if re.EndInclusive {
		p.write("]")
	} else {
		p.write(")")
	}
}

func isSourceMultiline(node any) bool {
	if node == nil {
		return false
	}
	val := reflect.ValueOf(node)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return false
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return false
	}

	field := val.FieldByName("Span")
	if !field.IsValid() {
		return false
	}

	// We assume it is ast.Span but we can't assert cleanly without knowing it matches
	// But since we only pass AST nodes...
	// Let's rely on the fact that Span struct from AST is what we have.
	// Actually, careful with type assertion if not ast.Span?
	// Using interface{} assertion for safety
	if span, ok := field.Interface().(ast.Span); ok {
		return span.End.Line > span.Start.Line
	}
	return false
}

func (p *printer) printStructLiteral(sl *ast.StructLiteral) {
	name := ""
	if sl.Name != nil {
		name = sl.Name.Value
	}
	if name != "" {
		p.write(name)
	}
	p.write("{")
	if len(sl.Fields) == 0 {
		p.write("}")
		return
	}

	fieldNames := make([]string, 0, len(sl.Fields))
	for fname := range sl.Fields {
		fieldNames = append(fieldNames, fname)
	}
	sort.Strings(fieldNames)

	multiline := isSourceMultiline(sl) || len(fieldNames) > 2
	if !multiline {
		totalLen := p.column
		for i, fname := range fieldNames {
			value := sl.Fields[fname]
			if p.isComplex(value) {
				multiline = true
				break
			}
			totalLen += len(fname) + len(": ") + p.expressionInlineLength(value)
			if i < len(fieldNames)-1 {
				totalLen += len(", ")
			}
		}
		totalLen += len("}")
		if totalLen > maxLineLength {
			multiline = true
		}
	}

	if !multiline {
		for i, fname := range fieldNames {
			if i > 0 {
				p.write(", ")
			}
			p.write(fname)
			p.write(": ")
			p.printExpression(sl.Fields[fname], precLowest)
		}
		p.write("}")
		return
	}

	p.newline()
	p.indent++
	maxFieldLen := 0
	for _, fname := range fieldNames {
		if len(fname) > maxFieldLen {
			maxFieldLen = len(fname)
		}
	}
	for _, fname := range fieldNames {
		p.writeIndent()
		p.write(fname)
		padding := max(maxFieldLen-len(fname), 0)
		p.write(":")
		p.write(strings.Repeat(" ", padding+1))
		p.printExpression(sl.Fields[fname], precLowest)
		p.write(",")
		p.newline()
	}
	p.indent--
	p.writeIndent()
	p.write("}")
}

func (p *printer) printVecLiteral(vl *ast.VecLiteral) {
	p.write("[")
	if len(vl.Elements) == 0 {
		p.write("]")
		return
	}

	multiline := isSourceMultiline(vl) || len(vl.Elements) > 3
	if !multiline {
		totalLen := p.column
		for i, el := range vl.Elements {
			if p.isComplex(el) {
				multiline = true
				break
			}
			totalLen += p.expressionInlineLength(el)
			if i < len(vl.Elements)-1 {
				totalLen += len(", ")
			}
		}
		totalLen += len("]")
		if totalLen > maxLineLength {
			multiline = true
		}
	}

	p.printExpressionList(vl.Elements, multiline)
	p.write("]")
}

func (p *printer) isComplex(expr ast.Expression) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.StructLiteral, *ast.VecLiteral, *ast.FunctionLiteral:
		return true
	case *ast.CallExpression:
		return len(e.Arguments) > 1
	case *ast.MethodCallExpression:
		return len(e.Arguments) > 1
	case *ast.InfixExpression:
		// Many-level nested infix expressions should be considered complex
		return p.infixComplexity(e) > 2
	default:
		return false
	}
}

func (p *printer) infixComplexity(expr ast.Expression) int {
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		return 0
	}
	return 1 + p.infixComplexity(infix.Left) + p.infixComplexity(infix.Right)
}

func (p *printer) printExpressionList(exprs []ast.Expression, multiline bool) {
	if !multiline && len(exprs) > 1 {
		// Quick check for total length if small number of elements
		totalLen := p.column
		for _, expr := range exprs {
			// Very rough estimate of expression length: just its literal string length if possible
			// or a constant for complex ones
			if p.isComplex(expr) {
				multiline = true
				break
			}
			totalLen += 10 // rough average per simple expression
		}
		if totalLen > maxLineLength {
			multiline = true
		}
	}

	if multiline {
		p.newline()
		p.indent++
		for i, expr := range exprs {
			p.writeIndent()
			p.printExpression(expr, precLowest)
			if i < len(exprs)-1 || true { // Always add comma for multiline
				p.write(",")
			}
			p.newline()
		}
		p.indent--
		p.writeIndent()
		return
	}

	for i, expr := range exprs {
		if i > 0 {
			p.write(", ")
		}
		p.printExpression(expr, precLowest)
	}
}

func (p *printer) writeIndent() {
	if p.needIndent {
		indent := strings.Repeat(indentUnit, p.indent)
		p.buf.WriteString(indent)
		p.column += len(indent)
		p.needIndent = false
	}
}

func (p *printer) write(s string) {
	p.writeIndent()
	p.buf.WriteString(s)
	p.column += len(s)
}

func (p *printer) newline() {
	p.buf.WriteByte('\n')
	p.column = 0
	p.needIndent = true
}

func (p *printer) writeComment(c Comment) {
	lines := strings.Split(c.Text, "\n")
	inlineHandled := false
	for i, line := range lines {
		if i > 0 {
			p.newline()
		}
		trimmed := strings.TrimSpace(line)
		if i == len(lines)-1 && strings.HasPrefix(trimmed, "//") && !p.needIndent && !strings.Contains(line, "\n") {
			p.buf.WriteString(" ")
			p.buf.WriteString(strings.TrimSpace(line))
			p.newline()
			inlineHandled = true
			continue
		}
		p.write(line)
	}
	if !inlineHandled {
		p.newline()
	}
}

func (p *printer) flushCommentsBefore(line, col int) {
	for p.commentIndex < len(p.comments) {
		c := p.comments[p.commentIndex]
		if c.Line < line || (c.Line == line && c.Column <= col) {
			p.writeComment(c)
			p.commentIndex++
			continue
		}
		break
	}
}

func (p *printer) hasPendingCommentBefore(line, col int) bool {
	if p.commentIndex >= len(p.comments) {
		return false
	}
	c := p.comments[p.commentIndex]
	return c.Line < line || (c.Line == line && c.Column <= col)
}

func statementPosition(stmt ast.Statement) (int, int) {
	switch s := stmt.(type) {
	case *ast.PackageStatement:
		return s.Token.Line, s.Token.Column
	case *ast.ImportStatement:
		return s.Token.Line, s.Token.Column
	case *ast.ImportBlock:
		return s.Token.Line, s.Token.Column
	case *ast.VarStatement:
		return s.Token.Line, s.Token.Column
	case *ast.MultiVarStatement:
		return s.Token.Line, s.Token.Column
	case *ast.ConstStatement:
		return s.Token.Line, s.Token.Column
	case *ast.ConstBlock:
		return s.Token.Line, s.Token.Column
	case *ast.ReturnStatement:
		return s.Token.Line, s.Token.Column
	case *ast.ExpressionStatement:
		return s.Token.Line, s.Token.Column
	case *ast.BlockStatement:
		return s.Token.Line, s.Token.Column
	case *ast.IfStatement:
		return s.Token.Line, s.Token.Column
	case *ast.WhileStatement:
		return s.Token.Line, s.Token.Column
	case *ast.ForStatement:
		return s.Token.Line, s.Token.Column
	case *ast.SwitchStatement:
		return s.Token.Line, s.Token.Column
	case *ast.DeferStatement:
		return s.Token.Line, s.Token.Column
	case *ast.UnsafeBlock:
		return s.Token.Line, s.Token.Column
	case *ast.BreakStatement:
		return s.Token.Line, s.Token.Column
	case *ast.ContinueStatement:
		return s.Token.Line, s.Token.Column
	case *ast.AssignmentStatement:
		return s.Token.Line, s.Token.Column
	case *ast.FunctionDecl:
		return s.Token.Line, s.Token.Column
	case *ast.StructDecl:
		return s.Token.Line, s.Token.Column
	case *ast.TypeDecl:
		return s.Token.Line, s.Token.Column
	case *ast.AliasDecl:
		return s.Token.Line, s.Token.Column
	case *ast.EnumDecl:
		return s.Token.Line, s.Token.Column
	case *ast.ImplDecl:
		return s.Token.Line, s.Token.Column
	default:
		return 0, 0
	}
}

func unwrapElseIf(block *ast.BlockStatement) *ast.IfStatement {
	if block == nil || len(block.Statements) != 1 {
		return nil
	}
	if stmt, ok := block.Statements[0].(*ast.IfStatement); ok {
		return stmt
	}
	return nil
}

func formatType(expr ast.TypeExpression) string {
	return typestr.RenderTypeForSyntax(expr)
}

const (
	maxInt     = int(^uint(0) >> 1)
	precLowest = iota
	precOr
	precAnd
	precEquals
	precLessGreater
	precBitOr
	precBitXor
	precBitAnd
	precShift
	precSum
	precProduct
	precPrefix
	precCall
	precIndex
	precDot
)

func precedence(expr ast.Expression) int {
	switch e := expr.(type) {
	case *ast.InfixExpression:
		return infixPrecedence(e.Operator)
	case *ast.PrefixExpression, *ast.BorrowExpression, *ast.BoxExpression, *ast.DerefExpression:
		return precPrefix
	case *ast.CallExpression:
		return precCall
	case *ast.IndexExpression:
		return precIndex
	case *ast.FieldAccessExpression, *ast.MethodCallExpression:
		return precDot
	default:
		return precLowest
	}
}

func infixPrecedence(op string) int {
	switch op {
	case "||":
		return precOr
	case "&&":
		return precAnd
	case "==", "!=":
		return precEquals
	case "<", ">", "<=", ">=":
		return precLessGreater
	case "|":
		return precBitOr
	case "^":
		return precBitXor
	case "&":
		return precBitAnd
	case "<<", ">>":
		return precShift
	case "+", "-":
		return precSum
	case "*", "/", "%":
		return precProduct
	default:
		return precLowest
	}
}

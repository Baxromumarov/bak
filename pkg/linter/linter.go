package linter

import (
	"os"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/token"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

type NamedNode interface {
	NodeName() string
	NodeToken() token.Token
	Kind() string
}

type namedNodeAdapter struct {
	name string
	tok  token.Token
	kind string
}

func (n namedNodeAdapter) NodeName() string       { return n.name }
func (n namedNodeAdapter) NodeToken() token.Token { return n.tok }
func (n namedNodeAdapter) Kind() string           { return n.kind }

// Finding represents a lint finding at a specific location.
type Finding struct {
	Rule    string
	Level   string // "error", "warning", "style"
	Message string
	File    string
	Line    int
	Column  int
}

// Config holds linter configuration.
type Config struct {
	MaxLineLength   int
	MaxFuncParams   int
	MaxNestingDepth int
	DisabledRules   map[string]bool
}

// DefaultConfig returns the default linter configuration.
func DefaultConfig() *Config {
	return &Config{
		MaxLineLength:   120,
		MaxFuncParams:   7,
		MaxNestingDepth: 5,
		DisabledRules:   make(map[string]bool),
	}
}

func normalizeConfig(config *Config) *Config {
	if config == nil {
		config = DefaultConfig()
	}
	if config.DisabledRules == nil {
		config.DisabledRules = make(map[string]bool)
	}
	return config
}

// Rule is the interface all lint rules implement.
type Rule interface {
	Name() string
	Check(prog *ast.Program, source string, config *Config) []Finding
}

var defaultRules = []Rule{
	&NamingConventionRule{},
	&ImportStyleRule{},
	&StyleRule{},
	&ComplexityRule{},
	&EmptyBlockRule{},
}

// AvailableRules returns all built-in lint rules in sorted order.
func AvailableRules() []string {
	names := make([]string, 0, len(defaultRules))
	for _, rule := range defaultRules {
		names = append(names, rule.Name())
	}
	sort.Strings(names)
	return names
}

// ApplyDisabledRulesCSV parses a comma-separated list and disables those rules.
func ApplyDisabledRulesCSV(config *Config, csv string) {
	config = normalizeConfig(config)
	for rule := range strings.SplitSeq(csv, ",") {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		config.DisabledRules[rule] = true
	}
}

// LintFile parses and lints a single file.
func LintFile(path string, config *Config) []Finding {
	data, err := os.ReadFile(path)
	if err != nil {
		return []Finding{{Rule: "io", Level: "error", Message: err.Error(), File: path}}
	}
	return LintSource(path, string(data), config)
}

// LintSource parses and lints an in-memory source string.
func LintSource(path, source string, config *Config) []Finding {
	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	return LintProgram(path, source, program, config)
}

// LintProgram lints a pre-parsed AST and avoids reparsing.
func LintProgram(path, source string, program *ast.Program, config *Config) []Finding {
	config = normalizeConfig(config)
	if program == nil {
		return nil
	}
	return lintProgram(path, source, program, config)
}

func lintProgram(path, source string, program *ast.Program, config *Config) []Finding {
	var findings []Finding
	for _, rule := range defaultRules {
		if config.DisabledRules[rule.Name()] {
			continue
		}
		for _, f := range rule.Check(program, source, config) {
			f.File = path
			findings = append(findings, f)
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Column < findings[j].Column
	})
	return findings
}

// isCamelCase checks if a string is camelCase (letters + digits, lowercase first letter).
func isCamelCase(s string) bool {
	if s == "" {
		return true
	}

	if !(s[0] >= 'a' && s[0] <= 'z') {
		return false
	}

	for _, c := range s {
		if !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9')) {
			return false
		}
	}

	return true
}

// isPascalCase checks if a string starts with an uppercase letter.
func isPascalCase(s string) bool {
	if len(s) == 0 {
		return false
	}

	return s[0] >= 'A' && s[0] <= 'Z'
}

// isUpperSnakeCase checks if a string is UPPER_SNAKE_CASE.
func isUpperSnakeCase(s string) bool {
	if s == "" {
		return true
	}

	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_') {
			return false
		}
	}

	return true
}

// isPascalOrCamel: Used for Structs
func isPascalOrCamel(s string) bool {
	return isPascalCase(s) || isCamelCase(s)
}

// isUpperSnakeOrPascal: Used for Constants
func isUpperSnakeOrPascal(s string) bool {
	return isUpperSnakeCase(s) || isPascalCase(s)
}

// --- Rules ---

// ImportStyleRule nudges source toward the canonical Go-like package import style.
type ImportStyleRule struct{}

func (r *ImportStyleRule) Name() string { return string(diagnostics.LintImportStyle) }

func (r *ImportStyleRule) Check(
	prog *ast.Program,
	source string,
	config *Config,
) []Finding {
	var findings []Finding
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			findings = append(findings, importStyleFinding(s)...)
		case *ast.ImportBlock:
			for _, imp := range s.Imports {
				findings = append(findings, importStyleFinding(imp)...)
			}
		}
	}
	return findings
}

func importStyleFinding(imp *ast.ImportStatement) []Finding {
	if imp == nil {
		return nil
	}
	findings := []Finding{}
	canonical, ok := canonicalStdImportPath(imp.Path)
	if !ok || canonical == imp.Path {
		return findings
	}
	findings = append(findings, Finding{
		Rule:    string(diagnostics.LintImportStyle),
		Level:   "style",
		Message: strfmt.Named("prefer Go-like import path '{path}'", "Path", canonical),
		Line:    imp.PathToken.Line,
		Column:  imp.PathToken.Column,
	})
	return findings
}

func canonicalStdImportPath(path string) (string, bool) {
	var rel string
	switch {
	case strings.HasPrefix(path, "src/std/"):
		rel = strings.TrimPrefix(path, "src/")
	case strings.HasPrefix(path, "std/"):
		rel = path
	default:
		return "", false
	}
	rel = strings.TrimSuffix(rel, ".bak")
	parts := strings.Split(rel, "/")
	if len(parts) >= 3 && parts[len(parts)-1] == parts[len(parts)-2] {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, "/"), true
}

// NamingConventionRule checks that identifiers follow naming conventions.
type NamingConventionRule struct{}

func (r *NamingConventionRule) Name() string { return "naming-convention" }

func (r *NamingConventionRule) Check(
	prog *ast.Program,
	source string,
	config *Config,
) []Finding {
	var findings []Finding

	for _, stmt := range prog.Statements {
		findings = append(findings, r.checkStatement(stmt)...)
	}

	return findings
}

func (r *NamingConventionRule) extractNamedNodes(stmt ast.Statement) []NamedNode {
	switch s := stmt.(type) {
	case *ast.FunctionDecl:
		// We return the function itself AND its parameters
		list := []NamedNode{s}
		for _, p := range s.Parameters {
			list = append(list, p)
		}
		return list

	case *ast.StructDecl:
		if s == nil {
			return nil
		}
		return r.extractIdentifierAsNode(s.Name, "struct")

	case *ast.EnumDecl:
		if s == nil {
			return nil
		}
		return r.extractIdentifierAsNode(s.Name, "enum")

	case *ast.ConstStatement:
		if s == nil {
			return nil
		}
		return r.extractIdentifierAsNode(s.Name, "constant")

	default:
		return nil
	}
}

func (r *NamingConventionRule) extractIdentifierAsNode(name *ast.Identifier, kind string) []NamedNode {
	if name == nil {
		return nil
	}
	return []NamedNode{namedNodeAdapter{
		name: name.Value,
		tok:  name.Token,
		kind: kind,
	}}
}

type Validator func(string) bool

var namingRegistry = map[string]struct {
	validate Validator
	expected string
}{
	"function":  {isCamelCase, "camelCase"},
	"parameter": {isCamelCase, "camelCase"},
	"struct":    {isPascalOrCamel, "PascalCase or camelCase"},
	"enum":      {isPascalCase, "PascalCase"},
	"constant":  {isUpperSnakeOrPascal, "UPPER_SNAKE_CASE or PascalCase"},
}

func (r *NamingConventionRule) checkStatement(stmt ast.Statement) []Finding {
	var findings []Finding

	// 1. Extract all potential nodes to check from this statement
	nodes := r.extractNamedNodes(stmt)

	// 2. Run the unified validation pipeline
	for _, node := range nodes {
		name := node.NodeName()

		// Skip ignored names (standard in production linters)
		if name == "" ||
			name == "_" ||
			name == "main" ||
			strings.HasPrefix(name, "_") {
			continue
		}

		// Look up the rule in our registry
		rule, exists := namingRegistry[node.Kind()]
		if !exists {
			continue
		}

		// Validate
		if !rule.validate(name) {
			findings = append(findings, Finding{
				Rule:  "naming-convention",
				Level: "warning",
				Message: strfmt.Named("{Kind} '{Value}' should be {Expected}",
					"Kind", node.Kind(),
					"Value", name,
					"Expected", rule.expected,
				),
				Line:   node.NodeToken().Line,
				Column: node.NodeToken().Column,
			})
		}
	}

	return findings
}

// StyleRule checks for style issues.
type StyleRule struct{}

func (r *StyleRule) Name() string { return "style" }

func (r *StyleRule) Check(
	prog *ast.Program,
	source string,
	config *Config,
) []Finding {
	var findings []Finding
	lines := strings.Split(source, "\n")
	inBlockComment := false
	for i, line := range lines {
		if isCommentOnlyLine(line, &inBlockComment) {
			continue
		}

		// Line length check
		if len(line) > config.MaxLineLength {
			findings = append(findings, Finding{
				Rule:  "style/line-length",
				Level: "style",
				Message: strfmt.Named("line exceeds {MaxLineLength} characters ({lineCount})",
					"MaxLineLength", config.MaxLineLength,
					"LineCount", len(line),
				),
				Line:   i + 1,
				Column: config.MaxLineLength + 1,
			})
		}
		// Trailing whitespace
		trimmed := strings.TrimRight(line, " \t")
		if len(trimmed) < len(line) && len(trimmed) > 0 {
			findings = append(findings, Finding{
				Rule:    "style/trailing-whitespace",
				Level:   "style",
				Message: "trailing whitespace",
				Line:    i + 1,
				Column:  len(trimmed) + 1,
			})
		}
	}
	return findings
}

func isCommentOnlyLine(line string, inBlockComment *bool) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return false
	}

	if *inBlockComment {
		_, after, ok := strings.Cut(trimmed, "*/")
		if !ok {
			return true
		}
		*inBlockComment = false
		rest := strings.TrimSpace(after)
		if rest == "" || strings.HasPrefix(rest, "//") || strings.HasPrefix(rest, "/*") {
			if strings.HasPrefix(rest, "/*") && !strings.Contains(rest, "*/") {
				*inBlockComment = true
			}
			return true
		}
		return false
	}

	if strings.HasPrefix(trimmed, "//") {
		return true
	}
	if strings.HasPrefix(trimmed, "/*") {
		if !strings.Contains(trimmed, "*/") {
			*inBlockComment = true
		}
		rest := ""
		if _, after, ok := strings.Cut(trimmed, "*/"); ok {
			rest = strings.TrimSpace(after)
		}
		return rest == "" || strings.HasPrefix(rest, "//")
	}
	return false
}

// EmptyBlockRule checks for empty blocks.
type EmptyBlockRule struct{}

func (r *EmptyBlockRule) Name() string { return "empty-block" }

func (r *EmptyBlockRule) Check(
	prog *ast.Program,
	source string,
	config *Config,
) []Finding {
	var findings []Finding
	for _, stmt := range prog.Statements {
		r.checkStmt(stmt, &findings)
	}
	return findings
}

type BlockOwner interface {
	GetBlock() []ast.Statement
	GetLocation() (line int, col int)
	BlockName() string // "if", "while", or the function name
}

func (r *EmptyBlockRule) checkStmt(stmt ast.Statement, findings *[]Finding) {

	blockOwner, ok := stmt.(BlockOwner)
	if !ok {
		return
	}

	// Single logic path for all block types
	if len(blockOwner.GetBlock()) == 0 {
		line, col := blockOwner.GetLocation()

		*findings = append(*findings, Finding{
			Rule:  "empty-block",
			Level: "warning",
			Message: strfmt.Named("{BlockName} block is empty",
				"BlockName", blockOwner.BlockName(),
			),
			Line:   line,
			Column: col,
		})
	}
}

// ComplexityRule checks for overly complex code.
type ComplexityRule struct{}

func (r *ComplexityRule) Name() string { return "complexity" }

func (r *ComplexityRule) Check(
	prog *ast.Program,
	source string,
	config *Config,
) []Finding {
	var findings []Finding
	for _, stmt := range prog.Statements {
		if fd, ok := stmt.(*ast.FunctionDecl); ok {
			if fd.Name == nil {
				continue
			}
			// Too many parameters
			if len(fd.Parameters) > config.MaxFuncParams {
				findings = append(findings, Finding{
					Rule:  "complexity/too-many-params",
					Level: "warning",
					Message: strfmt.Named("function '{Value}' has {ParametersCount} parameters (max {MaxFuncParams})",
						"Value", fd.Name.Value,
						"ParametersCount", len(fd.Parameters),
						"MaxFuncParams", config.MaxFuncParams,
					),
					Line:   fd.Name.Token.Line,
					Column: fd.Name.Token.Column,
				})
			}
			// Deep nesting
			if fd.Body != nil {
				depth := measureNestingDepth(fd.Body, 0)
				if depth > config.MaxNestingDepth {
					findings = append(findings, Finding{
						Rule:  "complexity/deep-nesting",
						Level: "warning",
						Message: strfmt.Named("function '{Value}' has nesting depth {depth} (max {MaxNestingDepth})",
							"Value", fd.Name.Value,
							"Depth", depth,
							"MaxNestingDepth", config.MaxNestingDepth,
						),
						Line:   fd.Name.Token.Line,
						Column: fd.Name.Token.Column,
					})
				}
			}
		}
	}
	return findings
}

type Nester interface {
	GetNestedBlocks() []*ast.BlockStatement
}

func measureNestingDepth(block *ast.BlockStatement, current int) int {
	if block == nil {
		return current
	}

	maxDepth := current
	for _, stmt := range block.Statements {
		// Advanced: Check if the statement is a "Nester"
		if nester, ok := stmt.(Nester); ok {
			for _, nestedBlock := range nester.GetNestedBlocks() {
				d := measureNestingDepth(nestedBlock, current+1)
				if d > maxDepth {
					maxDepth = d
				}
			}
		}
	}

	return maxDepth
}

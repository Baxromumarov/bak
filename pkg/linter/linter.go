package linter

import (
	"os"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

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
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
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
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// --- Rules ---

// NamingConventionRule checks that identifiers follow naming conventions.
type NamingConventionRule struct{}

func (r *NamingConventionRule) Name() string { return "naming-convention" }

func (r *NamingConventionRule) Check(prog *ast.Program, source string, config *Config) []Finding {
	var findings []Finding
	for _, stmt := range prog.Statements {
		findings = append(findings, r.checkStatement(stmt)...)
	}
	return findings
}

func (r *NamingConventionRule) checkStatement(stmt ast.Statement) []Finding {
	var findings []Finding
	switch s := stmt.(type) {
	case *ast.FunctionDecl:
		if s.Name != nil && !isCamelCase(s.Name.Value) && s.Name.Value != "main" {
			findings = append(findings, Finding{
				Rule:    "naming-convention",
				Level:   "warning",
				Message: "function '" + s.Name.Value + "' should be camelCase",
				Line:    s.Name.Token.Line,
				Column:  s.Name.Token.Column,
			})
		}
		for _, p := range s.Parameters {
			if p.Name != nil && p.Name.Value != "_" && !strings.HasPrefix(p.Name.Value, "_") && !isCamelCase(p.Name.Value) {
				findings = append(findings, Finding{
					Rule:    "naming-convention",
					Level:   "warning",
					Message: "parameter '" + p.Name.Value + "' should be camelCase",
					Line:    p.Name.Token.Line,
					Column:  p.Name.Token.Column,
				})
			}
		}
	case *ast.StructDecl:
		if s.Name != nil && !isPascalCase(s.Name.Value) && !isCamelCase(s.Name.Value) {
			findings = append(findings, Finding{
				Rule:    "naming-convention",
				Level:   "warning",
				Message: "struct '" + s.Name.Value + "' should be PascalCase or camelCase",
				Line:    s.Name.Token.Line,
				Column:  s.Name.Token.Column,
			})
		}
	case *ast.EnumDecl:
		if s.Name != nil && !isPascalCase(s.Name.Value) {
			findings = append(findings, Finding{
				Rule:    "naming-convention",
				Level:   "warning",
				Message: "enum '" + s.Name.Value + "' should be PascalCase",
				Line:    s.Name.Token.Line,
				Column:  s.Name.Token.Column,
			})
		}
	case *ast.ConstStatement:
		if s.Name != nil && !isUpperSnakeCase(s.Name.Value) && !isPascalCase(s.Name.Value) {
			findings = append(findings, Finding{
				Rule:    "naming-convention",
				Level:   "warning",
				Message: "constant '" + s.Name.Value + "' should be UPPER_SNAKE_CASE or PascalCase",
				Line:    s.Name.Token.Line,
				Column:  s.Name.Token.Column,
			})
		}
	}
	return findings
}

// StyleRule checks for style issues.
type StyleRule struct{}

func (r *StyleRule) Name() string { return "style" }

func (r *StyleRule) Check(prog *ast.Program, source string, config *Config) []Finding {
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
				Message: strfmt.Named("line exceeds {MaxLineLength} characters ({lineCount})", "MaxLineLength", config.MaxLineLength, "LineCount", len(line)),
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

func (r *EmptyBlockRule) Check(prog *ast.Program, source string, config *Config) []Finding {
	var findings []Finding
	for _, stmt := range prog.Statements {
		r.checkStmt(stmt, &findings)
	}
	return findings
}

func (r *EmptyBlockRule) checkStmt(stmt ast.Statement, findings *[]Finding) {
	switch s := stmt.(type) {
	case *ast.FunctionDecl:
		if s.Body != nil && len(s.Body.Statements) == 0 && s.Name != nil {
			*findings = append(*findings, Finding{
				Rule:    "empty-block",
				Level:   "warning",
				Message: "empty function body '" + s.Name.Value + "'",
				Line:    s.Name.Token.Line,
				Column:  s.Name.Token.Column,
			})
		}
	case *ast.IfStatement:
		if s.Consequence != nil && len(s.Consequence.Statements) == 0 {
			*findings = append(*findings, Finding{
				Rule:    "empty-block",
				Level:   "warning",
				Message: "empty if block",
				Line:    s.Token.Line,
				Column:  s.Token.Column,
			})
		}
	case *ast.WhileStatement:
		if s.Body != nil && len(s.Body.Statements) == 0 {
			*findings = append(*findings, Finding{
				Rule:    "empty-block",
				Level:   "warning",
				Message: "empty while block",
				Line:    s.Token.Line,
				Column:  s.Token.Column,
			})
		}
	case *ast.ForStatement:
		if s.Body != nil && len(s.Body.Statements) == 0 {
			*findings = append(*findings, Finding{
				Rule:    "empty-block",
				Level:   "warning",
				Message: "empty for block",
				Line:    s.Token.Line,
				Column:  s.Token.Column,
			})
		}
	}
}

// ComplexityRule checks for overly complex code.
type ComplexityRule struct{}

func (r *ComplexityRule) Name() string { return "complexity" }

func (r *ComplexityRule) Check(prog *ast.Program, source string, config *Config) []Finding {
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
					Message: strfmt.Named("function '{Value}' has {ParametersCount} parameters (max {MaxFuncParams})", "Value", fd.Name.Value, "ParametersCount", len(fd.Parameters), "MaxFuncParams", config.MaxFuncParams),
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
						Message: strfmt.Named("function '{Value}' has nesting depth {depth} (max {MaxNestingDepth})", "Value", fd.Name.Value, "Depth", depth, "MaxNestingDepth", config.MaxNestingDepth),
						Line:   fd.Name.Token.Line,
						Column: fd.Name.Token.Column,
					})
				}
			}
		}
	}
	return findings
}

func measureNestingDepth(block *ast.BlockStatement, current int) int {
	max := current
	for _, stmt := range block.Statements {
		switch s := stmt.(type) {
		case *ast.IfStatement:
			if s.Consequence != nil {
				d := measureNestingDepth(s.Consequence, current+1)
				if d > max {
					max = d
				}
			}
			if s.Alternative != nil {
				d := measureNestingDepth(s.Alternative, current+1)
				if d > max {
					max = d
				}
			}
		case *ast.ForStatement:
			if s.Body != nil {
				d := measureNestingDepth(s.Body, current+1)
				if d > max {
					max = d
				}
			}
		case *ast.WhileStatement:
			if s.Body != nil {
				d := measureNestingDepth(s.Body, current+1)
				if d > max {
					max = d
				}
			}
		}
	}
	return max
}

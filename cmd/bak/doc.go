package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

// generateDocs Parses a Bak source file and outputs Markdown documentation
// for its public symbols (functions, structs, enums).
func generateDocs(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	l := lexer.New(string(data))
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		fmt.Fprintf(os.Stderr, "Parse errors in %s:\n", filename)
		for _, msg := range p.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", msg)
		}
		os.Exit(1)
	}

	fmt.Printf("# Documentation for `%s`\n\n", filename)

	// Collect public declarations
	var funcs []*ast.FunctionDecl
	var structs []*ast.StructDecl
	var enums []*ast.EnumDecl

	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			if s.Visibility == ast.Public {
				funcs = append(funcs, s)
			}
		case *ast.StructDecl:
			if s.Visibility == ast.Public {
				structs = append(structs, s)
			}
		case *ast.EnumDecl:
			if s.Visibility == ast.Public {
				enums = append(enums, s)
			}
		}
	}

	if len(funcs) > 0 {
		fmt.Println("## Functions")
		fmt.Println("")
		for _, fn := range funcs {
			fmt.Printf("### `%s`\n", fn.Name.Value)
			fmt.Printf("```bak\n%s\n```\n\n", formatFunctionSignature(fn))
		}
	}

	if len(structs) > 0 {
		fmt.Println("## Structs")
		fmt.Println("")
		for _, s := range structs {
			fmt.Printf("### `%s`\n", s.Name.Value)
			fmt.Printf("```bak\n%s\n```\n\n", formatStructSignature(s))
		}
	}

	if len(enums) > 0 {
		fmt.Println("## Enums")
		fmt.Println("")
		for _, e := range enums {
			fmt.Printf("### `%s`\n", e.Name.Value)
			fmt.Printf("```bak\n%s\n```\n\n", formatEnumSignature(e))
		}
	}
}

func formatFunctionSignature(fn *ast.FunctionDecl) string {
	var params []string
	for _, p := range fn.Parameters {
		params = append(params, fmt.Sprintf("%s: %s", p.Name.Value, p.Type.String()))
	}
	ret := "void"
	if fn.ReturnType != nil {
		ret = fn.ReturnType.String()
	}
	return fmt.Sprintf("pub func %s(%s) -> (%s)", fn.Name.Value, strings.Join(params, ", "), ret)
}

func formatStructSignature(s *ast.StructDecl) string {
	var res strings.Builder
	fmt.Fprintf(&res, "pub struct %s {\n", s.Name.Value)
	for _, f := range s.Fields {
		pub := ""
		if f.Visibility == ast.Public {
			pub = "pub "
		}
		fmt.Fprintf(&res, "    %s%s: %s,\n", pub, f.Name.Value, f.Type.String())
	}
	res.WriteString("}")
	return res.String()
}

func formatEnumSignature(e *ast.EnumDecl) string {
	var res strings.Builder
	fmt.Fprintf(&res, "pub enum %s {\n", e.Name.Value)
	for _, v := range e.Variants {
		fmt.Fprintf(&res, "    %s,\n", v.Name.Value)
	}
	res.WriteString("}")
	return res.String()
}

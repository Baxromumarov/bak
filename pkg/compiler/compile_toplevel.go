package compiler

import "github.com/baxromumarov/bak/pkg/ast"

func (c *Compiler) registerTopLevelDeclarations(program *ast.Program) error {
	return walkTopLevelStatements(
		program,
		func(stmt ast.Statement) error {
			switch s := stmt.(type) {
			case *ast.ImportStatement:
				return c.processImport(s)
			case *ast.StructDecl:
				c.compileStructDef(s)
			case *ast.EnumDecl:
				c.compileEnumDef(s)
			case *ast.FunctionDecl:
				c.registerTopLevelGlobal(s.Name.Value)
			case *ast.VarStatement:
				c.registerTopLevelGlobal(s.Name.Value)
			case *ast.MultiVarStatement:
				for _, name := range s.Names {
					c.registerTopLevelGlobal(name.Value)
				}
			case *ast.ConstStatement:
				c.registerTopLevelGlobal(s.Name.Value)
			case *ast.AliasDecl:
				c.aliases[s.Name.Value] = s.Underlying
			case *ast.ImplDecl:
				// Methods are compiled in a later pass.
			}
			return nil
		},
	)
}

func (c *Compiler) registerFunctionStubs(program *ast.Program) {
	if program == nil {
		return
	}
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			c.registerFunctionStub(s.Name.Value)
		case *ast.ImplDecl:
			for _, method := range s.Methods {
				c.registerFunctionStub(s.TypeName.Value + "." + method.Name.Value)
			}
		}
	}
}

func (c *Compiler) compileTopLevelStatements(program *ast.Program) error {
	return walkTopLevelStatements(program, func(stmt ast.Statement) error {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			return c.compileFunction(s)
		case *ast.ImplDecl:
			return c.compileImpl(s)
		case *ast.VarStatement:
			c.registerTopLevelGlobal(s.Name.Value)
		case *ast.MultiVarStatement:
			for _, name := range s.Names {
				c.registerTopLevelGlobal(name.Value)
			}
		case *ast.ConstStatement:
			c.registerTopLevelGlobal(s.Name.Value)
		}
		return nil
	})
}

func walkTopLevelStatements(program *ast.Program, visit func(ast.Statement) error) error {
	if program == nil {
		return nil
	}
	for _, stmt := range program.Statements {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *ast.ImportBlock:
			for _, imp := range s.Imports {
				if err := visit(imp); err != nil {
					return err
				}
			}
		case *ast.VarBlock:
			for _, v := range s.Variables {
				if err := visit(v); err != nil {
					return err
				}
			}
		case *ast.ConstBlock:
			for _, cst := range s.Constants {
				if err := visit(cst); err != nil {
					return err
				}
			}
		default:
			if err := visit(stmt); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Compiler) registerTopLevelGlobal(name string) {
	if name == "" {
		return
	}
	c.module.AddGlobal(name)
}

func (c *Compiler) registerFunctionStub(name string) {
	if name == "" {
		return
	}
	stub := &FunctionObj{Name: name}
	idx := c.module.AddFunction(stub)
	c.module.FunctionIndices[name] = idx
}

func (c *Compiler) findMainFunction() int {
	for i, fn := range c.module.Functions {
		if fn.Name == "main" {
			c.module.EntryPoint = i
			return i
		}
	}
	return -1
}

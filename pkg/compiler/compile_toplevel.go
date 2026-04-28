package compiler

import "github.com/baxromumarov/bak/pkg/ast"

func (c *Compiler) registerTopLevelDeclarations(program *ast.Program) error {
	if err := walkTopLevelStatements(program, func(stmt ast.Statement) error {
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			return c.processImport(s)
		case *ast.StructDecl:
			c.compileStructDef(s)
		case *ast.EnumDecl:
			c.compileEnumDef(s)
		case *ast.FunctionDecl:
			c.module.AddGlobal(s.Name.Value)
		case *ast.VarStatement:
			c.module.AddGlobal(s.Name.Value)
		case *ast.MultiVarStatement:
			for _, name := range s.Names {
				c.module.AddGlobal(name.Value)
			}
		case *ast.ConstStatement:
			c.module.AddGlobal(s.Name.Value)
		case *ast.AliasDecl:
			c.aliases[s.Name.Value] = s.Underlying
		case *ast.ImplDecl:
			// Methods are compiled in a later pass.
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (c *Compiler) registerFunctionStubs(program *ast.Program) {
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			stub := &FunctionObj{Name: s.Name.Value}
			idx := c.module.AddFunction(stub)
			c.module.FunctionIndices[s.Name.Value] = idx
		case *ast.ImplDecl:
			for _, method := range s.Methods {
				methodName := s.TypeName.Value + "." + method.Name.Value
				stub := &FunctionObj{Name: methodName}
				idx := c.module.AddFunction(stub)
				c.module.FunctionIndices[methodName] = idx
			}
		}
	}
}

func (c *Compiler) compileTopLevelStatements(program *ast.Program) error {
	if err := walkTopLevelStatements(program, func(stmt ast.Statement) error {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			return c.compileFunction(s)
		case *ast.ImplDecl:
			return c.compileImpl(s)
		case *ast.VarStatement:
			c.module.AddGlobal(s.Name.Value)
		case *ast.MultiVarStatement:
			for _, name := range s.Names {
				c.module.AddGlobal(name.Value)
			}
		case *ast.ConstStatement:
			c.module.AddGlobal(s.Name.Value)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func walkTopLevelStatements(
	program *ast.Program,
	visit func(ast.Statement) error,
) error {
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

func (c *Compiler) findMainFunction() int {
	for i, fn := range c.module.Functions {
		if fn.Name == "main" {
			c.module.EntryPoint = i
			return i
		}
	}
	return -1
}

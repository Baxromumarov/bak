// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"path/filepath"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

func (tc *TypeChecker) collectDefinitions(program *ast.Program) {
	if program == nil {
		return
	}

	// Pass 1: Register all types, functions, and imports
	for _, stmt := range program.Statements {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			tc.checkImportStatement(s)
		case *ast.ImportBlock:
			tc.checkImportBlock(s)
		case *ast.StructDecl:
			tc.registerStructDecl(s)
		case *ast.EnumDecl:
			tc.registerEnumDecl(s)
		case *ast.FunctionDecl:
			tc.registerFunctionDecl(s)
		case *ast.TypeDecl:
			if s.Name == nil {
				continue
			}
			tc.env.DefineTypeDefAt(s.Name.Value, s.Underlying, s.Visibility, tokenPos(s.Name.Token))
		case *ast.AliasDecl:
			if s.Name == nil {
				continue
			}
			tc.env.DefineAliasAt(s.Name.Value, s.Underlying, s.Visibility, tokenPos(s.Name.Token))
		}
	}

	// Pass 2: Register methods and validate all type usages
	for _, stmt := range program.Statements {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *ast.ImplDecl:
			tc.registerImplMethods(s)
		case *ast.StructDecl:
			tc.validateDeclTypeUsage(s)
		case *ast.FunctionDecl:
			tc.validateDeclTypeUsage(s)
		case *ast.EnumDecl:
			tc.validateDeclTypeUsage(s)
		case *ast.TypeDecl:
			tc.validateDeclTypeUsage(s)
		case *ast.AliasDecl:
			tc.validateDeclTypeUsage(s)
		}
	}
}

func (tc *TypeChecker) registerStructDecl(s *ast.StructDecl) {
	if s == nil || s.Name == nil {
		return
	}
	tc.reportUserGenericDeclIfDisabled(len(s.TypeParams), tokenPos(s.Name.Token), s.Name.Token.Filename, "generic struct declarations")
	typeParams := typeParamNames(s.TypeParams)
	fields := make(map[string]FieldDef)
	for _, f := range s.Fields {
		if f == nil || f.Name == nil {
			continue
		}
		fields[f.Name.Value] = FieldDef{
			Type:       f.Type,
			Visibility: f.Visibility,
		}
	}
	tc.env.DefineStruct(s.Name.Value, &StructDef{
		Fields:      fields,
		Methods:     make(map[string]*FunctionSig),
		TypeParams:  typeParams,
		Package:     tc.currentPkgName,
		PackagePath: tc.currentPkgPath,
		Line:        s.Name.Token.Line,
		Column:      s.Name.Token.Column,
		Visibility:  s.Visibility,
	})
}

func (tc *TypeChecker) registerEnumDecl(s *ast.EnumDecl) {
	if s == nil || s.Name == nil {
		return
	}
	tc.reportUserGenericDeclIfDisabled(len(s.TypeParams), tokenPos(s.Name.Token), s.Name.Token.Filename, "generic enum declarations")
	variants := make(map[string]EnumVariantDef)
	for _, v := range s.Variants {
		if v == nil || v.Name == nil {
			continue
		}
		variants[v.Name.Value] = EnumVariantDef{
			HasPayload: len(v.Fields) > 0,
			Fields:     v.Fields,
		}
	}
	tc.env.DefineEnum(s.Name.Value, &EnumDef{
		Variants:    variants,
		Visibility:  s.Visibility,
		Package:     tc.currentPkgName,
		PackagePath: tc.currentPkgPath,
		Line:        s.Name.Token.Line,
		Column:      s.Name.Token.Column,
	})
}

func (tc *TypeChecker) registerFunctionDecl(s *ast.FunctionDecl) {
	if s == nil || s.Name == nil {
		return
	}
	tc.reportUserGenericDeclIfDisabled(len(s.TypeParams), tokenPos(s.Name.Token), s.Name.Token.Filename, "generic function declarations")
	typeParams := typeParamNames(s.TypeParams)
	params := parameterTypes(s.Parameters)
	tc.env.DefineFunction(s.Name.Value, &FunctionSig{
		TypeParams:  typeParams,
		Parameters:  params,
		ReturnType:  s.ReturnType,
		Package:     tc.currentPkgName,
		PackagePath: tc.currentPkgPath,
		Line:        s.Name.Token.Line,
		Column:      s.Name.Token.Column,
		Visibility:  s.Visibility,
	})
}

func (tc *TypeChecker) registerImplMethods(s *ast.ImplDecl) {
	if s == nil || s.TypeName == nil {
		return
	}
	tc.reportUserGenericDeclIfDisabled(len(s.TypeParams), tokenPos(s.TypeName.Token), s.TypeName.Token.Filename, "generic impl declarations")
	structDef, ok := tc.env.LookupStruct(s.TypeName.Value)
	if !ok {
		return
	}
	for _, method := range s.Methods {
		if method == nil || method.Name == nil {
			continue
		}
		tc.reportUserGenericDeclIfDisabled(len(method.TypeParams), tokenPos(method.Name.Token), method.Name.Token.Filename, "generic method declarations")
		if _, exists := structDef.Methods[method.Name.Value]; exists {
			tc.addError(
				method.Name.Token.Line,
				method.Name.Token.Column,
				"duplicate method '%s' for type '%s'",
				method.Name.Value,
				s.TypeName.Value,
			)
			continue
		}
		structDef.Methods[method.Name.Value] = &FunctionSig{
			Parameters:  parameterTypes(method.Parameters),
			ReturnType:  method.ReturnType,
			Package:     tc.currentPkgName,
			PackagePath: tc.currentPkgPath,
			Line:        method.Name.Token.Line,
			Column:      method.Name.Token.Column,
			Visibility:  method.Visibility,
			Mutable:     method.Mutable,
			IsInstance:  s.Receiver != nil,
		}
	}
}

func (tc *TypeChecker) validateDeclTypeUsage(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.StructDecl:
		restore := tc.setTypeParams(typeParamNames(s.TypeParams))
		for _, f := range s.Fields {
			if f != nil && f.Type != nil && f.Name != nil {
				tc.validateTypeUsage(f.Type, tokenPos(f.Name.Token))
			}
		}
		restore()
	case *ast.FunctionDecl:
		restore := tc.setTypeParams(typeParamNames(s.TypeParams))
		for _, p := range s.Parameters {
			if p != nil && p.Name != nil {
				tc.validateTypeUsage(p.Type, tokenPos(p.Name.Token))
			}
		}
		if s.Name != nil {
			tc.validateTypeUsage(s.ReturnType, tokenPos(s.Name.Token))
		}
		restore()
	case *ast.EnumDecl:
		restore := tc.setTypeParams(typeParamNames(s.TypeParams))
		for _, v := range s.Variants {
			if v == nil || v.Name == nil {
				continue
			}
			for _, f := range v.Fields {
				tc.validateTypeUsage(f, tokenPos(v.Name.Token))
			}
		}
		restore()
	case *ast.TypeDecl:
		tc.validateTypeUsage(s.Underlying, tokenPos(s.Token))
	case *ast.AliasDecl:
		tc.validateTypeUsage(s.Underlying, tokenPos(s.Token))
	}
}

func (tc *TypeChecker) isTestFile() bool {
	if tc.currentPkgPath == "" {
		return false
	}
	name := filepath.Base(tc.currentPkgPath)
	return strings.HasSuffix(name, "_test.bak")
}

package ast

// Walk traverses the AST starting at node, calling fn for every Node encountered
// (including node itself). It descends into all child fields automatically.
// Non-Node children (e.g. Parameter, StructField) are transparently unwrapped
// so that their Node-valued children are still visited.
func Walk(node Node, fn func(Node)) {
	if node == nil {
		return
	}
	fn(node)

	switch n := node.(type) {
	case *Program:
		// walkStmtList(n.Statements, fn)
		walkNodeList(n.Statements, fn)

	// Statements
	case *PackageStatement:
		Walk(n.Name, fn)
	case *ImportStatement:
		// leaf
	case *ImportBlock:
		for _, imp := range n.Imports {
			Walk(imp, fn)
		}
	case *VarStatement:
		Walk(n.Name, fn)
		Walk(n.Type, fn)
		Walk(n.Value, fn)
	case *MultiVarStatement:
		walkNodeList(n.Names, fn)
		walkNodeList(n.Types, fn)
		Walk(n.Value, fn)
	case *ConstStatement:
		Walk(n.Name, fn)
		Walk(n.Type, fn)
		Walk(n.Value, fn)
	case *ConstBlock:
		for _, c := range n.Constants {
			Walk(c, fn)
		}
	case *VarBlock:
		for _, v := range n.Variables {
			Walk(v, fn)
		}
	case *ReturnStatement:
		Walk(n.ReturnValue, fn)
	case *ExpressionStatement:
		Walk(n.Expression, fn)
	case *BlockStatement:
		walkNodeList(n.Statements, fn)
	case *IfStatement:
		Walk(n.Condition, fn)
		Walk(n.Consequence, fn)
		Walk(n.Alternative, fn)
	case *WhileStatement:
		Walk(n.Condition, fn)
		Walk(n.Body, fn)
	case *ForStatement:
		Walk(n.Variable, fn)
		Walk(n.Iterable, fn)
		Walk(n.Body, fn)
	case *SwitchStatement:
		Walk(n.Value, fn)
		for _, c := range n.Cases {
			if c == nil {
				continue
			}
			walkNodeList(c.Values, fn)
			Walk(c.Body, fn)
		}
	case *DeferStatement:
		Walk(n.Body, fn)
	case *PanicStatement:
		Walk(n.Message, fn)
	case *UnsafeBlock:
		Walk(n.Body, fn)
	case *BreakStatement, *ContinueStatement:
		// leaf
	case *AssignmentStatement:
		Walk(n.Left, fn)
		Walk(n.Value, fn)

	// Declarations (also Statements)
	case *FunctionDecl:
		Walk(n.Name, fn)
		walkNodeList(n.TypeParams, fn)
		walkParamList(n.Parameters, fn)
		Walk(n.ReturnType, fn)
		Walk(n.Body, fn)
	case *StructDecl:
		Walk(n.Name, fn)
		walkNodeList(n.TypeParams, fn)
		walkStructFieldList(n.Fields, fn)
	case *TypeDecl:
		Walk(n.Name, fn)
		Walk(n.Underlying, fn)
	case *AliasDecl:
		Walk(n.Name, fn)
		Walk(n.Underlying, fn)
	case *EnumDecl:
		Walk(n.Name, fn)
		walkNodeList(n.TypeParams, fn)
		walkEnumVariantList(n.Variants, fn)
	case *ImplDecl:
		Walk(n.TypeName, fn)
		Walk(n.Receiver, fn)
		walkMethodDeclList(n.Methods, fn)

	// Expressions
	case *Identifier,
		*MutableIdentifier:
		// leaf
	case *IntegerLiteral,
		*FloatLiteral,
		*StringLiteral,
		*CharLiteral,
		*BooleanLiteral,
		*VoidLiteral:
		// leaf
	case *FStringLiteral:
		walkNodeList(n.Elements, fn)
	case *PrefixExpression:
		Walk(n.Right, fn)
	case *InfixExpression:
		Walk(n.Left, fn)
		Walk(n.Right, fn)
	case *CallExpression:
		Walk(n.Function, fn)
		walkNodeList(n.Arguments, fn)
	case *TypeConversion:
		Walk(n.Value, fn)
	case *MethodCallExpression:
		Walk(n.Object, fn)
		Walk(n.Method, fn)
		walkNodeList(n.Arguments, fn)
	case *FieldAccessExpression:
		Walk(n.Object, fn)
		Walk(n.Field, fn)
	case *IndexExpression:
		Walk(n.Left, fn)
		Walk(n.Index, fn)
	case *StructLiteral:
		Walk(n.Name, fn)
		for _, v := range n.Fields {
			Walk(v, fn)
		}
	case *VecLiteral:
		walkNodeList(n.Elements, fn)
	case *TupleExpression:
		walkNodeList(n.Elements, fn)
	case *RangeExpression:
		Walk(n.Start, fn)
		Walk(n.End, fn)
	case *EnumVariantExpression:
		Walk(n.Variant, fn)
		walkNodeList(n.Values, fn)
	case *BorrowExpression:
		Walk(n.Value, fn)
	case *DerefExpression:
		Walk(n.Value, fn)
	case *UnwrapExpression:
		Walk(n.Value, fn)
	case *FunctionLiteral:
		walkNodeList(n.Parameters, fn)
		Walk(n.ReturnType, fn)
		Walk(n.Body, fn)

	// Type expressions
	case *SimpleType,
		*VoidType,
		*ErrorType:
		// leaf
	case *GenericType:
		walkNodeList(n.TypeParams, fn)
	case *TypeParameter:
		Walk(n.Name, fn)
	case *BorrowType:
		Walk(n.Inner, fn)
	case *ArrayType:
		Walk(n.ElemType, fn)
	case *SizeExpression:
		// leaf
	case *TupleType:
		walkNodeList(n.Elements, fn)
	case *FunctionType:
		walkNodeList(n.Params, fn)
		Walk(n.ReturnType, fn)
	case *NamedType:
		Walk(n.Type, fn)
	}
}

func walkNodeList[T Node](list []T, fn func(Node)) {
	for _, item := range list {
		Walk(item, fn)
	}
}

func walkParamList(list []*Parameter, fn func(Node)) {
	for _, p := range list {
		if p == nil {
			continue
		}
		Walk(p.Name, fn)
		Walk(p.Type, fn)
	}
}

func walkStructFieldList(list []*StructField, fn func(Node)) {
	for _, f := range list {
		if f == nil {
			continue
		}
		Walk(f.Name, fn)
		Walk(f.Type, fn)
	}
}

func walkEnumVariantList(list []*EnumVariant, fn func(Node)) {
	for _, v := range list {
		if v == nil {
			continue
		}
		Walk(v.Name, fn)
		walkNodeList(v.Fields, fn)
	}
}

func walkMethodDeclList(list []*MethodDecl, fn func(Node)) {
	for _, m := range list {
		if m == nil {
			continue
		}
		Walk(m.Name, fn)
		walkNodeList(m.TypeParams, fn)
		walkNodeList(m.Parameters, fn)
		Walk(m.ReturnType, fn)
		Walk(m.Body, fn)
	}
}

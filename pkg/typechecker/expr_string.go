package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

func stringStdlibReplacementHint(method string) string {
	switch method {
	case "split":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.split(&value, &sep)"
	case "trim":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.trim(&value)"
	case "trimLeft":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.trimLeft(&value)"
	case "trimRight":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.trimRight(&value)"
	case "trimPrefix":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.trimPrefix(&value, &prefix)"
	case "trimSuffix":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.trimSuffix(&value, &suffix)"
	case "toUpper":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.toUpper(&value)"
	case "toLower":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.toLower(&value)"
	case "replaceFirst":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.replaceFirst(&value, &old, &new)"
	case "count":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.count(&value, &sub)"
	case "compare":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.compare(&a, &b)"
	case "equalIgnoreCase":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.equalIgnoreCase(&a, &b)"
	default:
		return ""
	}
}

// checkStringMethodCall type checks String method calls
func (tc *TypeChecker) checkStringMethodCall(mc *ast.MethodCallExpression) ast.TypeExpression {
	method := mc.Method.Value
	switch method {
	case "len":
		return &ast.SimpleType{Name: "int"}
	case "bytes":
		return &ast.GenericType{
			Name: "Vec",
			TypeParams: []ast.TypeExpression{
				&ast.SimpleType{Name: "int"},
				&ast.SizeExpression{IsDynamic: true},
			},
		}
	case "chars":
		return &ast.GenericType{
			Name: "Vec",
			TypeParams: []ast.TypeExpression{
				&ast.SimpleType{Name: "char"},
				&ast.SizeExpression{IsDynamic: true},
			},
		}
	case "hash":
		return &ast.SimpleType{Name: "int"}
	case "substring":
		return &ast.SimpleType{Name: "string"}
	case "indexOf", "lastIndexOf":
		return &ast.GenericType{Name: "Result", TypeParams: []ast.TypeExpression{
			&ast.SimpleType{Name: "int"},
			&ast.SimpleType{Name: "string"},
		}}
	case "contains", "startsWith", "endsWith":
		return &ast.SimpleType{Name: "bool"}
	case "parseInt":
		return &ast.GenericType{Name: "Result", TypeParams: []ast.TypeExpression{
			&ast.SimpleType{Name: "int"},
			&ast.SimpleType{Name: "string"},
		}}
	case "parseFloat":
		return &ast.GenericType{Name: "Result", TypeParams: []ast.TypeExpression{
			&ast.SimpleType{Name: "float64"},
			&ast.SimpleType{Name: "string"},
		}}
	case "toString":
		return &ast.SimpleType{Name: "string"}
	case "get":
		return &ast.GenericType{Name: "Result", TypeParams: []ast.TypeExpression{
			&ast.SimpleType{Name: "char"},
			&ast.SimpleType{Name: "string"},
		}}
	default:
		callPos := tokenPos(mc.Token)
		if hint := stringStdlibReplacementHint(method); hint != "" {
			tc.errorUndefinedMethodWithHelpAt("string", method, callPos, stringMethodCandidates, hint)
			return nil
		}
		tc.errorUndefinedMethodAt("string", method, callPos, stringMethodCandidates)
		return nil
	}
}

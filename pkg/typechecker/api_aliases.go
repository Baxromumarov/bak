package typechecker

import "strings"

var vecMethodAliasCanonical = map[string]string{
	"isEmpty": "is_empty",
}

var stringMethodAliasCanonical = map[string]string{
	"indexOf":     "index_of",
	"lastIndexOf": "last_index_of",
	"startsWith":  "starts_with",
	"endsWith":    "ends_with",
	"parseInt":    "parse_int",
	"parseFloat":  "parse_float",
	"toString":    "to_string",
}

var primitiveMethodAliasCanonical = map[string]string{
	"toString": "to_string",
}

var resultMethodAliasCanonical = map[string]string{
	"toString": "to_string",
}

var stringsFunctionAliasCanonical = map[string]string{
	"charAt":          "char_at",
	"indexOf":         "index_of",
	"lastIndexOf":     "last_index_of",
	"startsWith":      "starts_with",
	"endsWith":        "ends_with",
	"trimLeft":        "trim_left",
	"trimRight":       "trim_right",
	"trimPrefix":      "trim_prefix",
	"trimSuffix":      "trim_suffix",
	"toUpper":         "to_upper",
	"toLower":         "to_lower",
	"replaceFirst":    "replace_first",
	"padLeft":         "pad_left",
	"padRight":        "pad_right",
	"isLetter":        "is_letter",
	"isDigit":         "is_digit",
	"isAlphanumeric":  "is_alphanumeric",
	"isUpper":         "is_upper",
	"isLower":         "is_lower",
	"equalIgnoreCase": "equal_ignore_case",
	"fromChars":       "from_chars",
	"fromBytes":       "from_bytes",
	"fromBytesSlice":  "from_bytes_slice",
	"toChars":         "to_chars",
}

var strconvFunctionAliasCanonical = map[string]string{
	"intToString":  "int_to_string",
	"formatInt":    "format_int",
	"formatBinary": "format_binary",
	"formatOctal":  "format_octal",
	"formatHex":    "format_hex",
	"isDigit":      "is_digit",
	"digitValue":   "digit_value",
	"parseInt":     "parse_int",
	"parseBinary":  "parse_binary",
	"parseOctal":   "parse_octal",
	"parseHex":     "parse_hex",
	"formatFloat":  "format_float",
	"parseBool":    "parse_bool",
	"formatBool":   "format_bool",
}

func normalizeAlias(aliasMap map[string]string, name string) (string, bool) {
	canonical, ok := aliasMap[name]
	if !ok {
		return name, false
	}
	return canonical, true
}

func (tc *TypeChecker) canonicalizeVecMethod(method string, line, col int) string {
	if canonical, deprecated := normalizeAlias(vecMethodAliasCanonical, method); deprecated {
		tc.warnDeprecatedAlias("method", method, canonical, line, col)
		return canonical
	}
	return method
}

func (tc *TypeChecker) canonicalizeStringMethod(method string, line, col int) string {
	if canonical, deprecated := normalizeAlias(stringMethodAliasCanonical, method); deprecated {
		tc.warnDeprecatedAlias("method", method, canonical, line, col)
		return canonical
	}
	return method
}

func (tc *TypeChecker) canonicalizePrimitiveMethod(method string, line, col int) string {
	if canonical, deprecated := normalizeAlias(primitiveMethodAliasCanonical, method); deprecated {
		tc.warnDeprecatedAlias("method", method, canonical, line, col)
		return canonical
	}
	return method
}

func (tc *TypeChecker) canonicalizeResultMethod(method string, line, col int) string {
	if canonical, deprecated := normalizeAlias(resultMethodAliasCanonical, method); deprecated {
		tc.warnDeprecatedAlias("method", method, canonical, line, col)
		return canonical
	}
	return method
}

func isStdStringsImportPath(path string) bool {
	normalized := strings.ReplaceAll(path, "\\", "/")
	return strings.Contains(normalized, "std/strings/strings")
}

func isStdStrconvImportPath(path string) bool {
	normalized := strings.ReplaceAll(path, "\\", "/")
	return strings.Contains(normalized, "std/strconv/strconv")
}

func (tc *TypeChecker) canonicalizeStringsModuleFunction(moduleAlias, name string, line, col int) string {
	path := tc.importedPkgPaths[moduleAlias]
	if isStdStringsImportPath(path) {
		if canonical, deprecated := normalizeAlias(stringsFunctionAliasCanonical, name); deprecated {
			tc.warnDeprecatedAlias("function", moduleAlias+"."+name, moduleAlias+"."+canonical, line, col)
			return canonical
		}
	}
	if isStdStrconvImportPath(path) {
		if canonical, deprecated := normalizeAlias(strconvFunctionAliasCanonical, name); deprecated {
			tc.warnDeprecatedAlias("function", moduleAlias+"."+name, moduleAlias+"."+canonical, line, col)
			return canonical
		}
	}
	return name
}

package typechecker

// This file provides additional suggestion helpers for better error messages.
// The main levenshtein distance implementation is in typechecker.go.

func (tc *TypeChecker) suggestFields(name string, structFields []string, limit int) []string {
	return bestSuggestions(name, structFields, limit)
}

// getStructFieldNames returns a list of field names for a given struct type.
// It searches through the environment chain to find the struct definition.
func (tc *TypeChecker) getStructFieldNames(structName string) []string {
	var fields []string

	// Walk up the environment chain
	for env := tc.env; env != nil; env = env.parent {
		if structDef, ok := env.structs[structName]; ok {
			for fieldName := range structDef.Fields {
				fields = append(fields, fieldName)
			}
			return fields // Found struct, return its fields
		}
	}

	return fields
}

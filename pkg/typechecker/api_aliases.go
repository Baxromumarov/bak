package typechecker

func (tc *TypeChecker) canonicalizeVecMethod(method string, line, col int) string {
	return method
}

func (tc *TypeChecker) canonicalizeStringMethod(method string, line, col int) string {
	return method
}

func (tc *TypeChecker) canonicalizePrimitiveMethod(method string, line, col int) string {
	return method
}

func (tc *TypeChecker) canonicalizeResultMethod(method string, line, col int) string {
	return method
}

func (tc *TypeChecker) canonicalizeStdModuleFunction(moduleAlias, name string, line, col int) string {
	return name
}

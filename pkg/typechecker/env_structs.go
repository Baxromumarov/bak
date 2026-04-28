package typechecker

// DefineStruct defines a struct type
func (e *TypeEnv) DefineStruct(name string, structDef *StructDef) {
	e.structs[name] = structDef
}

// LookupStruct looks up a struct definition
func (e *TypeEnv) LookupStruct(name string) (*StructDef, bool) {
	if s, ok := e.structs[name]; ok {
		e.MarkUsed(name)
		return s, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			return e.root().LookupStruct(name)
		}
		return e.parent.LookupStruct(name)
	}
	return nil, false
}

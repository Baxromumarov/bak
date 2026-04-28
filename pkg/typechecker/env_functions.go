package typechecker

// DefineFunction defines a function signature
func (e *TypeEnv) DefineFunction(name string, sig *FunctionSig) {
	e.functions[name] = sig
}

// LookupFunction looks up a function signature
func (e *TypeEnv) LookupFunction(name string) (*FunctionSig, bool) {
	if sig, ok := e.functions[name]; ok {
		e.MarkUsed(name)
		return sig, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			return e.root().LookupFunction(name)
		}
		return e.parent.LookupFunction(name)
	}
	return nil, false
}

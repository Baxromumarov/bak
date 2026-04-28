package typechecker

import "testing"

func TestLookupEnum_NonCapturingScopeSeesRootNotParentLocals(t *testing.T) {
	root := NewTypeEnv()
	root.DefineEnum("RootEnum", &EnumDef{})

	parentLocal := NewEnclosedTypeEnv(root)
	parentLocal.DefineEnum("ParentEnum", &EnumDef{})

	nonCapt := NewEnclosedTypeEnv(parentLocal)
	nonCapt.nonCapturing = true

	if _, ok := nonCapt.LookupEnum("RootEnum"); !ok {
		t.Fatalf("expected non-capturing scope to resolve root enum")
	}
	if _, ok := nonCapt.LookupEnum("ParentEnum"); ok {
		t.Fatalf("expected non-capturing scope to not capture parent-local enum")
	}
}

func TestLookupEnum_NonCapturingScopeStillSeesOwnLocalEnum(t *testing.T) {
	root := NewTypeEnv()
	scope := NewEnclosedTypeEnv(root)
	scope.nonCapturing = true
	scope.DefineEnum("LocalEnum", &EnumDef{})

	if _, ok := scope.LookupEnum("LocalEnum"); !ok {
		t.Fatalf("expected lookup to resolve enum defined in current non-capturing scope")
	}
}

func TestLookupEnum_CapturingScopeSeesParentEnums(t *testing.T) {
	root := NewTypeEnv()
	parent := NewEnclosedTypeEnv(root)
	parent.DefineEnum("ParentEnum", &EnumDef{})

	child := NewEnclosedTypeEnv(parent)
	if _, ok := child.LookupEnum("ParentEnum"); !ok {
		t.Fatalf("expected capturing scope to resolve parent enum")
	}
}

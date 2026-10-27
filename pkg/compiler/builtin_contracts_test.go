package compiler

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/runtimecap"
)

func TestBuiltinContract_Exec(t *testing.T) {
	contract, ok := BuiltinContractByName("__builtin_exec")
	if !ok {
		t.Fatalf("expected __builtin_exec contract")
	}
	if contract.MinArgs != 2 || contract.MaxArgs != 2 {
		t.Fatalf("unexpected exec arity: min=%d max=%d", contract.MinArgs, contract.MaxArgs)
	}
	if contract.PermissionFlag != runtimecap.FlagAllowExec {
		t.Fatalf("unexpected exec permission flag: %q", contract.PermissionFlag)
	}
	if contract.PermissionOp != "os.exec" {
		t.Fatalf("unexpected exec permission op: %q", contract.PermissionOp)
	}
}

func TestBuiltinContract_PgQueryOptionalParams(t *testing.T) {
	contract, ok := BuiltinContractByName("__builtin_pg_query")
	if !ok {
		t.Fatalf("expected __builtin_pg_query contract")
	}
	if contract.MinArgs != 2 || contract.MaxArgs != 3 {
		t.Fatalf("unexpected pg_query arity: min=%d max=%d", contract.MinArgs, contract.MaxArgs)
	}
	if !contract.AcceptsArity(2) || !contract.AcceptsArity(3) || contract.AcceptsArity(1) || contract.AcceptsArity(4) {
		t.Fatalf("pg_query arity acceptance is incorrect")
	}
}

func TestBuiltinContract_MysqlQueryOptionalParams(t *testing.T) {
	contract, ok := BuiltinContractByName("__builtin_mysql_query")
	if !ok {
		t.Fatalf("expected __builtin_mysql_query contract")
	}
	if contract.MinArgs != 2 || contract.MaxArgs != 3 {
		t.Fatalf("unexpected mysql_query arity: min=%d max=%d", contract.MinArgs, contract.MaxArgs)
	}
	if !contract.AcceptsArity(2) || !contract.AcceptsArity(3) || contract.AcceptsArity(1) || contract.AcceptsArity(4) {
		t.Fatalf("mysql_query arity acceptance is incorrect")
	}
}

func TestLookupBuiltinID_TypeofAlias(t *testing.T) {
	idType, ok := LookupBuiltinID("type")
	if !ok {
		t.Fatalf("expected builtin id for type")
	}
	idTypeof, ok := LookupBuiltinID("typeof")
	if !ok {
		t.Fatalf("expected builtin id for typeof")
	}
	if idType != idTypeof || idType != BUILTIN_TYPE {
		t.Fatalf("type/typeof alias mismatch: type=%d typeof=%d", idType, idTypeof)
	}
}

func TestLookupBuiltinNameForModuleMethod(t *testing.T) {
	builtinName, ok := LookupBuiltinNameForModuleMethod("os", "exec")
	if !ok {
		t.Fatalf("expected alias for os.exec")
	}
	if builtinName != "__builtin_exec" {
		t.Fatalf("unexpected builtin alias for os.exec: %q", builtinName)
	}

	builtinName, ok = LookupBuiltinNameForModuleMethod("fs", "writeFile")
	if !ok {
		t.Fatalf("expected alias for fs.writeFile")
	}
	if builtinName != "__builtin_write_file" {
		t.Fatalf("unexpected builtin alias for fs.writeFile: %q", builtinName)
	}
}

func TestBuiltinCatalogIsInternallyConsistent(t *testing.T) {
	names := make(map[string]struct{})
	ids := make(map[BuiltinID]string)
	modules := make(map[string]struct{})

	for _, spec := range builtinCatalog {
		if len(spec.names) == 0 || spec.names[0] == "" {
			t.Fatal("builtin catalog entry has no canonical name")
		}
		if spec.contract.Name != spec.names[0] {
			t.Fatalf("builtin %q contract name is %q", spec.names[0], spec.contract.Name)
		}
		if spec.contract.MinArgs < 0 || (spec.contract.MaxArgs >= 0 && spec.contract.MaxArgs < spec.contract.MinArgs) {
			t.Fatalf("builtin %q has invalid arity range", spec.names[0])
		}
		for _, name := range spec.names {
			if _, duplicate := names[name]; duplicate {
				t.Fatalf("duplicate builtin name %q", name)
			}
			names[name] = struct{}{}
		}
		for _, module := range spec.moduleMethods {
			if _, duplicate := modules[module]; duplicate {
				t.Fatalf("duplicate builtin module method %q", module)
			}
			modules[module] = struct{}{}
		}
		if !spec.bytecode {
			continue
		}
		if previous, duplicate := ids[spec.id]; duplicate {
			t.Fatalf("builtin id %d is shared by %q and %q", spec.id, previous, spec.names[0])
		}
		ids[spec.id] = spec.names[0]
	}
}

func TestBuiltinNamesReturnsDefensiveCopy(t *testing.T) {
	names := BuiltinNames()
	delete(names, "print")

	if id, ok := LookupBuiltinID("print"); !ok || id != BUILTIN_PRINT {
		t.Fatalf("caller mutated builtin registry: id=%d ok=%v", id, ok)
	}
}

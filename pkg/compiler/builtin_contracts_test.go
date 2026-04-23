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

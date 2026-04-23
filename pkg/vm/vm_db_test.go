package vm

import (
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/compiler"
)

func TestDBPermissionOpNames(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{name: "pg_connect", want: "db.postgres.connect"},
		{name: "pg_query", want: "db.postgres.query"},
		{name: "pg_close", want: "db.postgres.close"},
		{name: "mysql_connect", want: "db.mysql.connect"},
		{name: "mysql_query", want: "db.mysql.query"},
		{name: "mysql_close", want: "db.mysql.close"},
		{name: "db_config", want: "db.config"},
	}

	for _, tc := range cases {
		if got := dbPermissionOp(tc.name); got != tc.want {
			t.Fatalf("unexpected permission op for %q: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestDBQueryArgs_WithoutParams(t *testing.T) {
	args := []compiler.Value{
		compiler.NewInt(1),
		compiler.NewString("select 1"),
	}
	got, err := dbQueryArgs(args, "__builtin_pg_query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil query args, got %#v", got)
	}
}

func TestDBQueryArgs_FromArray(t *testing.T) {
	params := &compiler.ArrayInstance{
		Elements: []compiler.Value{
			compiler.NewString("a"),
			compiler.NewString("b"),
		},
	}
	args := []compiler.Value{
		compiler.NewInt(1),
		compiler.NewString("select ?, ?"),
		{Type: compiler.VAL_ARRAY, AsObject: params},
	}

	got, err := dbQueryArgs(args, "__builtin_mysql_query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected query args: %#v", got)
	}
}

func TestDBQueryArgs_FromVecStruct(t *testing.T) {
	params := &compiler.ArrayInstance{
		Elements: []compiler.Value{
			compiler.NewString("x"),
		},
	}
	vecInst := &compiler.StructInstance{
		TypeName: "db.Vec",
		Fields: []compiler.Value{
			{Type: compiler.VAL_ARRAY, AsObject: params},
			compiler.NewInt(1),
			compiler.NewInt(1),
		},
	}
	args := []compiler.Value{
		compiler.NewInt(1),
		compiler.NewString("select ?"),
		{Type: compiler.VAL_STRUCT, AsObject: vecInst},
	}

	got, err := dbQueryArgs(args, "__builtin_mysql_query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "x" {
		t.Fatalf("unexpected query args: %#v", got)
	}
}

func TestDBQueryArgs_RejectsNonStringElement(t *testing.T) {
	params := &compiler.ArrayInstance{
		Elements: []compiler.Value{
			compiler.NewString("ok"),
			compiler.NewInt(1),
		},
	}
	args := []compiler.Value{
		compiler.NewInt(1),
		compiler.NewString("select ?, ?"),
		{Type: compiler.VAL_ARRAY, AsObject: params},
	}

	_, err := dbQueryArgs(args, "__builtin_pg_query")
	if err == nil {
		t.Fatalf("expected error for non-string param")
	}
	if !strings.Contains(err.Error(), "param at index 1 must be string") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVecDataAndLengthFromStruct(t *testing.T) {
	vecData := &compiler.ArrayInstance{
		Elements: []compiler.Value{
			compiler.NewInt(1),
			compiler.NewInt(2),
			compiler.NewInt(3),
		},
	}
	inst := &compiler.StructInstance{
		TypeName: "db.Vec",
		Fields: []compiler.Value{
			{Type: compiler.VAL_ARRAY, AsObject: vecData},
			compiler.NewInt(2),
			compiler.NewInt(3),
		},
	}

	arr, vecLen, ok := vecDataAndLengthFromStruct(inst)
	if !ok {
		t.Fatalf("expected Vec struct to be recognized")
	}
	if arr != vecData {
		t.Fatalf("unexpected vec backing array: got %#v want %#v", arr, vecData)
	}
	if vecLen != 2 {
		t.Fatalf("unexpected vec length: got %d want 2", vecLen)
	}
}

func TestVecDataAndLengthFromStruct_ClampsLength(t *testing.T) {
	inst := &compiler.StructInstance{
		TypeName: "Vec",
		Fields: []compiler.Value{
			{
				Type: compiler.VAL_ARRAY,
				AsObject: &compiler.ArrayInstance{
					Elements: []compiler.Value{compiler.NewInt(1), compiler.NewInt(2)},
				},
			},
			compiler.NewInt(5),
			compiler.NewInt(8),
		},
	}

	_, vecLen, ok := vecDataAndLengthFromStruct(inst)
	if !ok {
		t.Fatalf("expected Vec struct to be recognized")
	}
	if vecLen != 2 {
		t.Fatalf("expected clamped vec length to equal backing array length, got %d", vecLen)
	}
}

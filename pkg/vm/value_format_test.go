package vm

import (
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/compiler"
)

func structValue(typeName string, typeID int, fields ...compiler.Value) compiler.Value {
	return compiler.Value{
		Type: compiler.VAL_STRUCT,
		AsObject: &compiler.StructInstance{
			TypeName: typeName,
			TypeID:   typeID,
			Fields:   fields,
		},
	}
}

func TestFormatValue_ArrayPrintsElements(t *testing.T) {
	v := New(compiler.NewBytecodeModule())

	arr := compiler.Value{
		Type: compiler.VAL_ARRAY,
		AsObject: &compiler.ArrayInstance{
			Elements: []compiler.Value{
				compiler.NewInt(1),
				compiler.NewString("x"),
				compiler.NewBool(true),
			},
		},
	}

	got := v.formatValue(arr)
	want := "[1, x, true]"
	if got != want {
		t.Fatalf("unexpected formatted array: got %q want %q", got, want)
	}
}

func TestFormatValue_HashMapPrintsEntries(t *testing.T) {
	module := compiler.NewBytecodeModule()
	vecTypeID := module.AddStruct("Vec", []compiler.FieldDef{
		{Name: "data"},
		{Name: "length"},
		{Name: "capacity"},
		{Name: "fixed"},
		{Name: "growth"},
	})
	bucketTypeID := module.AddStruct("HashBucket", []compiler.FieldDef{
		{Name: "key"},
		{Name: "value"},
		{Name: "filled"},
	})
	hashMapTypeID := module.AddStruct("HashMap", []compiler.FieldDef{
		{Name: "ctrl"},
		{Name: "buckets"},
		{Name: "len"},
		{Name: "cap"},
		{Name: "grow_left"},
	})
	v := New(module)

	bucketA := structValue(
		"HashBucket",
		bucketTypeID,
		compiler.NewString("a"),
		compiler.NewInt(1),
		compiler.NewBool(true),
	)
	bucketB := structValue(
		"HashBucket",
		bucketTypeID,
		compiler.NewString("b"),
		compiler.NewInt(2),
		compiler.NewBool(true),
	)
	bucketEmpty := structValue(
		"HashBucket",
		bucketTypeID,
		compiler.NewString(""),
		compiler.NewInt(0),
		compiler.NewBool(false),
	)

	bucketsArray := compiler.Value{
		Type: compiler.VAL_ARRAY,
		AsObject: &compiler.ArrayInstance{
			Elements: []compiler.Value{bucketA, bucketB, bucketEmpty},
		},
	}
	bucketsVec := structValue(
		"Vec",
		vecTypeID,
		bucketsArray,
		compiler.NewInt(3),
		compiler.NewInt(3),
		compiler.NewBool(false),
		compiler.NewInt(2),
	)
	mapVal := structValue(
		"HashMap",
		hashMapTypeID,
		compiler.Value{Type: compiler.VAL_ARRAY, AsObject: &compiler.ArrayInstance{}},
		bucketsVec,
		compiler.NewInt(2),
		compiler.NewInt(16),
		compiler.NewInt(10),
	)

	got := v.formatValue(mapVal)
	if !strings.HasPrefix(got, "HashMap{") || !strings.HasSuffix(got, "}") {
		t.Fatalf("unexpected HashMap format: %q", got)
	}
	if strings.Contains(got, "<HashMap instance>") {
		t.Fatalf("expected expanded map value, got placeholder: %q", got)
	}
	if !strings.Contains(got, "a: 1") || !strings.Contains(got, "b: 2") {
		t.Fatalf("expected map entries in output, got %q", got)
	}
}

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

func TestFormatValue_StructPrintsFieldData(t *testing.T) {
	module := compiler.NewBytecodeModule()
	typeID := module.AddStruct("Data", []compiler.FieldDef{
		{Name: "age"},
		{Name: "name"},
		{Name: "fl"},
	})
	v := New(module)

	data := structValue(
		"Data",
		typeID,
		compiler.NewInt(30),
		compiler.NewString("Alice"),
		compiler.NewFloat(3.14),
	)

	got := v.formatValue(data)
	want := "Data{age: 30, name: Alice, fl: 3.14}"
	if got != want {
		t.Fatalf("unexpected struct format: got %q want %q", got, want)
	}
}

func TestFormatValue_VecOfStructsPrintsElementTypeOnce(t *testing.T) {
	module := compiler.NewBytecodeModule()
	vecTypeID := module.AddStruct("Vec", []compiler.FieldDef{
		{Name: "data"},
		{Name: "length"},
		{Name: "capacity"},
	})
	dataTypeID := module.AddStruct("Data", []compiler.FieldDef{
		{Name: "age"},
		{Name: "name"},
		{Name: "fl"},
	})
	v := New(module)

	first := structValue(
		"Data",
		dataTypeID,
		compiler.NewInt(12),
		compiler.NewString("Hello"),
		compiler.NewFloat(12.2),
	)
	second := structValue(
		"Data",
		dataTypeID,
		compiler.NewInt(63),
		compiler.NewString("Hello2"),
		compiler.NewFloat(1543.32),
	)
	dataArray := compiler.Value{
		Type: compiler.VAL_ARRAY,
		AsObject: &compiler.ArrayInstance{
			Elements: []compiler.Value{first, second},
		},
	}
	vec := structValue(
		"Vec",
		vecTypeID,
		dataArray,
		compiler.NewInt(2),
		compiler.NewInt(2),
	)

	got := v.formatValue(vec)
	want := "Vec<Data>[\n  {age: 12, name: Hello, fl: 12.2},\n  {age: 63, name: Hello2, fl: 1543.32}\n]"
	if got != want {
		t.Fatalf("unexpected Vec<Data> format: got %q want %q", got, want)
	}
	if strings.Count(got, "Data") != 1 {
		t.Fatalf("expected element type to appear once, got %q", got)
	}
}

func TestBuiltinStringFormatsStructFields(t *testing.T) {
	module := compiler.NewBytecodeModule()
	typeID := module.AddStruct("Data", []compiler.FieldDef{
		{Name: "age"},
		{Name: "name"},
	})
	v := New(module)

	data := structValue(
		"Data",
		typeID,
		compiler.NewInt(30),
		compiler.NewString("Alice"),
	)

	got, err := v.callBuiltin(compiler.BUILTIN_STRING, []compiler.Value{data})
	if err != nil {
		t.Fatalf("string builtin failed: %v", err)
	}

	want := "Data{age: 30, name: Alice}"
	if got.AsString != want {
		t.Fatalf("unexpected string conversion: got %q want %q", got.AsString, want)
	}
}

func TestDebugValueStructIncludesFieldsAndMethods(t *testing.T) {
	module := compiler.NewBytecodeModule()
	typeID := module.AddStruct("Data", []compiler.FieldDef{
		{Name: "age", Type: "int"},
		{Name: "name", Type: "string"},
	})
	methodIdx := module.AddFunction(&compiler.FunctionObj{
		Name:      "Data.setAge",
		Arity:     2,
		NumLocals: 2,
	})
	module.AddMethod("Data", "setAge", methodIdx)
	v := New(module)

	data := structValue(
		"Data",
		typeID,
		compiler.NewInt(30),
		compiler.NewString("Alice"),
	)

	got := v.debugValue(data)
	for _, want := range []string{
		"dbg Struct Data",
		"  value: Data{age: 30, name: Alice}",
		"  fields:",
		"    age: int = 30",
		"    name: string = Alice",
		"  methods:",
		"    setAge(arity=2)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected debug output to contain %q, got:\n%s", want, got)
		}
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

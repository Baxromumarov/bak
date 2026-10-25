package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestGeneratedBuiltinMethodMetadataMatchesJSON(t *testing.T) {
	raw, err := os.ReadFile("builtin_methods.json")
	if err != nil {
		t.Fatalf("read builtin method metadata: %v", err)
	}

	var source struct {
		Instance map[string]map[string]builtinMethodInfo `json:"instance"`
		Static   map[string]map[string]builtinMethodInfo `json:"static"`
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatalf("parse builtin method metadata: %v", err)
	}

	if !reflect.DeepEqual(builtinMethods, source.Instance) {
		t.Fatal("generated builtinMethods is stale; run go generate ./lsp")
	}
	if !reflect.DeepEqual(builtinStaticMethods, source.Static) {
		t.Fatal("generated builtinStaticMethods is stale; run go generate ./lsp")
	}
}

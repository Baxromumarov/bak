package prelude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedPreludesMatchCanonicalStdlibSources(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		path     string
		embedded string
	}{
		{name: "HashMap", path: "src/std/collections/hashmap.bak", embedded: hashmapPrelude},
		{name: "Vec", path: "src/std/collections/vec.bak", embedded: vecPrelude},
		{name: "Result", path: "src/std/result.bak", embedded: resultPrelude},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join("..", "..", tc.path))
			if err != nil {
				t.Fatalf("read canonical source: %v", err)
			}
			if tc.embedded != string(contents) {
				t.Fatalf("embedded prelude is stale; run go generate ./pkg/prelude")
			}
		})
	}
}

package vm

import (
	"testing"
)

func TestVMTryExpressionUnwrapsOk(t *testing.T) {
	src := `package main

func load() -> (Result<int, string>) {
	return Ok(21)
}

func calc() -> (Result<int, string>) {
	var out: int = try load()
	return Ok(out + 1)
}

func main() -> (void) {
	var result: Result<int, string> = calc()
	switch result {
		case Ok(value) {
			if value != 22 {
				panic "wrong value"
			}
		}
		case Err(err) { panic err }
	}
	return void
}
`

	module := compileModule(t, src)
	if _, err := New(module).Run(); err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}
}

func TestVMTryExpressionPropagatesErr(t *testing.T) {
	src := `package main

func load() -> (Result<int, string>) {
	return Err("bad")
}

func calc() -> (Result<int, string>) {
	var out: int = try load()
	return Ok(out + 1)
}

func main() -> (void) {
	var result: Result<int, string> = calc()
	switch result {
		case Ok(_) { panic "expected Err" }
		case Err(err) {
			if err != "bad" {
				panic err
			}
		}
	}
	return void
}
`

	module := compileModule(t, src)
	if _, err := New(module).Run(); err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}
}

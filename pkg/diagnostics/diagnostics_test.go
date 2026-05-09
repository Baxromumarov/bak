package diagnostics

import "testing"

func TestEmitterNormalizesMissingCodeAndFile(t *testing.T) {
	emitter := NewEmitter("demo.bak")
	emitter.Emit(Diagnostic{
		Level:   LevelError,
		Message: "failure",
		Line:    1,
		Column:  1,
	})

	diags := emitter.Diagnostics()
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Code != ErrGeneric {
		t.Fatalf("expected generic error code %q, got %q", ErrGeneric, diags[0].Code)
	}
	if diags[0].File != "demo.bak" {
		t.Fatalf("expected emitter file context, got %q", diags[0].File)
	}
}

func TestEmitterNormalizesWarningAndEmptyMessage(t *testing.T) {
	emitter := NewEmitter("")
	emitter.Emit(Diagnostic{
		Level: LevelWarning,
	})

	diags := emitter.Diagnostics()
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Code != WarnGeneric {
		t.Fatalf("expected warning default code, got %q", diags[0].Code)
	}
	if diags[0].Message != "unknown diagnostic" {
		t.Fatalf("expected fallback message, got %q", diags[0].Message)
	}
}

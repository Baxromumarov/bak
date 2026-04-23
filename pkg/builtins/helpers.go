package builtins

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/object"
)

func newError(format string, a ...any) *object.Error {
	return &object.Error{
		Message: fmt.Sprintf(format, a...),
	}
}

// Helpers
func resultErrString(errStr string) *object.Result {
	return &object.Result{
		IsOk:  false,
		Value: object.NewString(errStr),
	}
}

func resultOkVoid() *object.Result {
	return &object.Result{
		IsOk:  true,
		Value: object.NewVoid(),
	}
}

func resultOk(value object.Object) *object.Result {
	return &object.Result{
		IsOk:  true,
		Value: value,
	}
}

func resultErr(err error) *object.Result {
	return &object.Result{
		IsOk:  false,
		Value: object.NewString(err.Error()),
	}
}

func resultOkString(s string) *object.Result {
	return &object.Result{
		IsOk:  true,
		Value: object.NewString(s),
	}
}

func resultOkInt(n int64) *object.Result {
	return &object.Result{
		IsOk:  true,
		Value: object.NewInteger(n),
	}
}

func argCountError(fn string, got int, want string) object.Object {
	return newError("%s: wrong number of arguments. got=%d, want=%s", fn, got, want)
}

func argTypeError(fn string, got object.Object, wantType string) object.Object {
	return newError("%s: argument must be %s, got %s", fn, wantType, got.Type())
}

func firstArgTypeError(fn string, got object.Object, wantType string) object.Object {
	return newError("%s: first argument must be %s, got %s", fn, wantType, got.Type())
}

func secondArgTypeError(fn string, got object.Object, wantType string) object.Object {
	return newError("%s: second argument must be %s, got %s", fn, wantType, got.Type())
}

func nthArgTypeError(fn string, n int, got object.Object, wantType string) object.Object {
	return newError("%s: argument %d must be %s, got %s", fn, n, wantType, got.Type())
}

// Package builtins provides built-in modules for the bak language.
// This file implements the 'os' module for OS operations.
package builtins

import (
	"os"

	"github.com/baxromumarov/bak/pkg/object"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// OSModule is the built-in OS module
var OSModule = &object.Module{
	Name: "os",
	Functions: map[string]*object.Builtin{
		"args":        {Fn: osArgs},
		"exit":        {Fn: osExit},
		"getenv":      {Fn: osGetenv},
		"setenv":      {Fn: osSetenv},
		"hasenv":      {Fn: osHasenv},
		"cwd":         {Fn: osCwd},
		"chdir":       {Fn: osChdir},
		"executable":  {Fn: osExecutable},
		"exec":        {Fn: osExec},
		"hostname":    {Fn: osHostname},
		"tempDir":     {Fn: osTempDir},
		"userHomeDir": {Fn: osUserHomeDir},
	},
}

// osArgs returns command-line arguments as Vec<string>
func osArgs(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("os.args: wrong number of arguments. got=%d, want=0", len(args))
	}

	osArgs := os.Args
	elements := make([]object.Object, len(osArgs))
	for i, arg := range osArgs {
		elements[i] = &object.String{Value: arg}
	}

	return &object.Vec{
		Elements: elements,
		ElemType: "string",
		Size:     -1,
		Mutable:  false,
	}
}

// osExit exits the program with a status code
func osExit(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("os.exit: wrong number of arguments. got=%d, want=1", len(args))
	}

	code, ok := args[0].(*object.Integer)
	if !ok {
		return newError("os.exit: argument must be INTEGER, got %s", args[0].Type())
	}

	os.Exit(int(code.Value))
	return &object.Void{} // Never reached
}

// osGetenv gets an environment variable, returns Option<string>
func osGetenv(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("os.getenv: wrong number of arguments. got=%d, want=1", len(args))
	}

	name, ok := args[0].(*object.String)
	if !ok {
		return newError("os.getenv: argument must be STRING, got %s", args[0].Type())
	}

	value, exists := os.LookupEnv(name.Value)
	if !exists {
		return &object.Option{
			IsSome: false,
			Value:  nil,
		}
	}

	return &object.Option{
		IsSome: true,
		Value:  &object.String{Value: value},
	}
}

// osSetenv sets an environment variable
func osSetenv(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("os.setenv: wrong number of arguments. got=%d, want=2", len(args))
	}

	name, ok := args[0].(*object.String)
	if !ok {
		return newError("os.setenv: first argument must be STRING, got %s", args[0].Type())
	}

	value, ok := args[1].(*object.String)
	if !ok {
		return newError("os.setenv: second argument must be STRING, got %s", args[1].Type())
	}

	err := os.Setenv(name.Value, value.Value)
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	return &object.Result{
		IsOk:  true,
		Value: &object.Void{},
	}
}

// osHasenv checks if an environment variable exists
func osHasenv(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("os.hasenv: wrong number of arguments. got=%d, want=1", len(args))
	}

	name, ok := args[0].(*object.String)
	if !ok {
		return newError("os.hasenv: argument must be STRING, got %s", args[0].Type())
	}

	_, exists := os.LookupEnv(name.Value)
	return &object.Boolean{Value: exists}
}

// osCwd returns the current working directory
func osCwd(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("os.cwd: wrong number of arguments. got=%d, want=0", len(args))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	return &object.Result{
		IsOk:  true,
		Value: &object.String{Value: cwd},
	}
}

// osChdir changes the current working directory
func osChdir(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("os.chdir: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("os.chdir: argument must be STRING, got %s", args[0].Type())
	}

	err := os.Chdir(path.Value)
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	return &object.Result{
		IsOk:  true,
		Value: &object.Void{},
	}
}

// osExecutable returns the path of the current executable
func osExecutable(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("os.executable: wrong number of arguments. got=%d, want=0", len(args))
	}

	path, err := os.Executable()
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	return &object.Result{
		IsOk:  true,
		Value: &object.String{Value: path},
	}
}

// osHostname returns the system hostname
func osHostname(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("os.hostname: wrong number of arguments. got=%d, want=0", len(args))
	}
	name, err := os.Hostname()
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}
	return &object.Result{IsOk: true, Value: &object.String{Value: name}}
}

// osTempDir returns the temp directory path (string)
func osTempDir(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("os.tempDir: wrong number of arguments. got=%d, want=0", len(args))
	}
	return &object.String{Value: os.TempDir()}
}

// osUserHomeDir returns the user home directory path
func osUserHomeDir(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("os.userHomeDir: wrong number of arguments. got=%d, want=0", len(args))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}
	return &object.Result{IsOk: true, Value: &object.String{Value: home}}
}

// osChmod changes file mode bits
// Takes (path: string, mode: int) -> Result<void, string>
func osChmod(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("os.chmod: wrong number of arguments. got=%d, want=2", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("os.chmod: first argument must be STRING, got %s", args[0].Type())
	}

	modeInt, ok := args[1].(*object.Integer)
	if !ok {
		return newError("os.chmod: second argument must be INTEGER, got %s", args[1].Type())
	}

	err := os.Chmod(path.Value, os.FileMode(modeInt.Value))
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	return &object.Result{
		IsOk:  true,
		Value: &object.Void{},
	}
}

// osExec executes an external command without invoking a shell.
// Returns Result<ExecResult, string>.
func osExec(args ...object.Object) object.Object {
	if len(args) < 1 {
		return newError("os.exec: wrong number of arguments. got=%d, want at least 1", len(args))
	}

	cmdName, ok := args[0].(*object.String)
	if !ok {
		return newError("os.exec: first argument must be STRING, got %s", args[0].Type())
	}

	// Extract command arguments
	var cmdArgs []string
	if len(args) > 1 {
		argsVec, ok := args[1].(*object.Vec)
		if !ok {
			return newError("os.exec: second argument must be VEC, got %s", args[1].Type())
		}
		for _, elem := range argsVec.Elements {
			str, ok := elem.(*object.String)
			if !ok {
				return newError("os.exec: all command arguments must be STRING")
			}
			cmdArgs = append(cmdArgs, str.Value)
		}
	}

	if !runtimecap.Current().AllowExec {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("os.exec", runtimecap.FlagAllowExec)},
		}
	}

	execResult, err := runtimecap.ExecuteCommand(cmdName.Value, cmdArgs, runtimecap.Current())
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	return &object.Result{
		IsOk: true,
		Value: &object.Struct{
			Name: "ExecResult",
			Fields: map[string]object.Object{
				"Output":    &object.String{Value: execResult.Output},
				"Stdout":    &object.String{Value: execResult.Stdout},
				"Stderr":    &object.String{Value: execResult.Stderr},
				"ExitCode":  &object.Integer{Value: execResult.ExitCode},
				"TimedOut":  &object.Boolean{Value: execResult.TimedOut},
				"Truncated": &object.Boolean{Value: execResult.Truncated},
			},
		},
	}
}

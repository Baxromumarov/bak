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

// osArgs returns command-line arguments as Vec<string, _>
func osArgs(args ...object.Object) object.Object {
	if len(args) != 0 {
		return argCountError("os.args", len(args), "0")
	}

	osArgs := os.Args
	elements := make([]object.Object, len(osArgs))
	for i, arg := range osArgs {
		elements[i] = object.NewString(arg)
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
		return argCountError("os.exit", len(args), "1")
	}

	code, ok := args[0].(*object.Integer)
	if !ok {
		return argTypeError("os.exit", args[0], "INTEGER")
	}

	os.Exit(int(code.Value))
	return object.NewVoid() // Never reached
}

// osGetenv gets an environment variable, returns Result<string, string>
func osGetenv(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("os.getenv", len(args), "1")
	}

	name, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("os.getenv", args[0], "STRING")
	}

	value, exists := os.LookupEnv(name.Value)
	if !exists {
		return resultErrString("environment variable '" + name.Value + "' is not set")
	}

	return resultOk(object.NewString(value))
}

// osSetenv sets an environment variable
func osSetenv(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("os.setenv", len(args), "2")
	}

	name, ok := args[0].(*object.String)
	if !ok {
		return firstArgTypeError("os.setenv", args[0], "STRING")
	}

	value, ok := args[1].(*object.String)
	if !ok {
		return secondArgTypeError("os.setenv", args[1], "STRING")
	}

	err := os.Setenv(name.Value, value.Value)
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

// osHasenv checks if an environment variable exists
func osHasenv(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("os.hasenv", len(args), "1")
	}

	name, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("os.hasenv", args[0], "STRING")
	}

	_, exists := os.LookupEnv(name.Value)
	return object.NewBool(exists)
}

// osCwd returns the current working directory
func osCwd(args ...object.Object) object.Object {
	if len(args) != 0 {
		return argCountError("os.cwd", len(args), "0")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return resultErr(err)
	}

	return resultOk(object.NewString(cwd))
}

// osChdir changes the current working directory
func osChdir(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("os.chdir", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("os.chdir", args[0], "STRING")
	}

	err := os.Chdir(path.Value)
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

// osExecutable returns the path of the current executable
func osExecutable(args ...object.Object) object.Object {
	if len(args) != 0 {
		return argCountError("os.executable", len(args), "0")
	}

	path, err := os.Executable()
	if err != nil {
		return resultErr(err)
	}

	return resultOk(object.NewString(path))
}

// osHostname returns the system hostname
func osHostname(args ...object.Object) object.Object {
	if len(args) != 0 {
		return argCountError("os.hostname", len(args), "0")
	}
	name, err := os.Hostname()
	if err != nil {
		return resultErr(err)
	}
	return resultOkString(name)
}

// osTempDir returns the temp directory path (string)
func osTempDir(args ...object.Object) object.Object {
	if len(args) != 0 {
		return argCountError("os.tempDir", len(args), "0")
	}
	return object.NewString(os.TempDir())
}

// osUserHomeDir returns the user home directory path
func osUserHomeDir(args ...object.Object) object.Object {
	if len(args) != 0 {
		return argCountError("os.userHomeDir", len(args), "0")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return resultErr(err)
	}
	return resultOkString(home)
}

// osChmod changes file mode bits
// Takes (path: string, mode: int) -> Result<void, string>
func osChmod(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("os.chmod", len(args), "2")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return firstArgTypeError("os.chmod", args[0], "STRING")
	}

	modeInt, ok := args[1].(*object.Integer)
	if !ok {
		return secondArgTypeError("os.chmod", args[1], "INTEGER")
	}
	if denied := requireFSMutatePermission("os.chmod"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "os.chmod"); err != nil {
		return resultErr(err)
	}

	err := os.Chmod(path.Value, os.FileMode(modeInt.Value))
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

// osExec executes an external command without invoking a shell.
// Returns Result<ExecResult, string>.
func osExec(args ...object.Object) object.Object {
	if len(args) < 1 {
		return argCountError("os.exec", len(args), "at least 1")
	}

	cmdName, ok := args[0].(*object.String)
	if !ok {
		return firstArgTypeError("os.exec", args[0], "STRING")
	}

	// Extract command arguments
	var cmdArgs []string
	if len(args) > 1 {
		argsVec, ok := args[1].(*object.Vec)
		if !ok {
			return secondArgTypeError("os.exec", args[1], "VEC")
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
		return resultErrString(runtimecap.PermissionError("os.exec", runtimecap.FlagAllowExec))
	}

	execResult, err := runtimecap.ExecuteCommand(cmdName.Value, cmdArgs, runtimecap.Current())
	if err != nil {
		return resultErr(err)
	}

	return resultOk(&object.Struct{
		Name: "ExecResult",
		Fields: map[string]object.Object{
			"Output":    object.NewString(execResult.Output),
			"Stdout":    object.NewString(execResult.Stdout),
			"Stderr":    object.NewString(execResult.Stderr),
			"ExitCode":  object.NewInteger(execResult.ExitCode),
			"TimedOut":  object.NewBool(execResult.TimedOut),
			"Truncated": object.NewBool(execResult.Truncated),
		},
	})
}

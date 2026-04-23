// Package builtins provides built-in modules for the bak language.
// This file implements the 'fs' module for file system operations.
package builtins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/baxromumarov/bak/pkg/object"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// FSModule is the built-in file system module
var FSModule = &object.Module{
	Name: "fs",
	Functions: map[string]*object.Builtin{
		"readFile":  {Fn: fsReadFile},
		"readLines": {Fn: fsReadLines},
		"exists":    {Fn: fsExists},
		"isDir":     {Fn: fsIsDir},
		"isFile":    {Fn: fsIsFile},
		"readDir":   {Fn: fsReadDir},
		"stat":      {Fn: fsStat},
		"join":      {Fn: fsJoin},
		"ext":       {Fn: fsExt},
		"base":      {Fn: fsBase},
		"dir":       {Fn: fsDir},
		"abs":       {Fn: fsAbs},
		// Write operations
		"writeFile":  {Fn: fsWriteFile},
		"appendFile": {Fn: fsAppendFile},
		"mkdir":      {Fn: fsMkdir},
		"mkdirAll":   {Fn: fsMkdirAll},
		"remove":     {Fn: fsRemove},
		"removeAll":  {Fn: fsRemoveAll},
		"rename":     {Fn: fsRename},
		"copy":       {Fn: fsCopy},
	},
}

func requireFSMutatePermission(op string) *object.Result {
	if runtimecap.Current().AllowFSMutate {
		return nil
	}
	return resultErrString(runtimecap.PermissionError(op, runtimecap.FlagAllowFSMutate))
}

// fsReadFile reads a file and returns Result<string, string>
func fsReadFile(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.readFile", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.readFile", args[0], "STRING")
	}

	content, err := os.ReadFile(path.Value)
	if err != nil {
		return resultErr(err)
	}

	return resultOk(object.NewString(string(content)))
}

// fsReadFileBytes reads a file and returns Result<Vec<int, _>, string>
func fsReadFileBytes(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.readFileBytes", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.readFileBytes", args[0], "STRING")
	}

	content, err := os.ReadFile(path.Value)
	if err != nil {
		return resultErr(err)
	}

	elements := make([]object.Object, len(content))
	for i, b := range content {
		elements[i] = object.NewInteger(int64(b))
	}

	return resultOk(&object.Vec{
			Elements: elements,
			ElemType: "int",
			Size:     -1,
			Mutable:  false,
		})
}

// fsWriteFileBytes writes raw bytes (Vec<int, _>) to a file
func fsWriteFileBytes(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("fs.writeFileBytes", len(args), "2")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return firstArgTypeError("fs.writeFileBytes", args[0], "STRING")
	}

	dataObj, ok := args[1].(*object.Vec)
	if !ok {
		return secondArgTypeError("fs.writeFileBytes", args[1], "Vec<int, _>")
	}
	if denied := requireFSMutatePermission("fs.writeFileBytes"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.writeFileBytes"); err != nil {
		return resultErr(err)
	}

	bytes := make([]byte, len(dataObj.Elements))
	for i, el := range dataObj.Elements {
		intVal, ok := el.(*object.Integer)
		if !ok {
			return newError("fs.writeFileBytes: element at %d is not int", i)
		}
		bytes[i] = byte(intVal.Value)
	}

	err := os.WriteFile(path.Value, bytes, 0644)
	if err != nil {
		return resultErr(err)
	}
	return resultOkVoid()
}

// fsReadLines reads a file and returns Result<Vec<string, _>, string>
func fsReadLines(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.readLines", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.readLines", args[0], "STRING")
	}

	content, err := os.ReadFile(path.Value)
	if err != nil {
		return resultErr(err)
	}

	lines := strings.Split(string(content), "\n")
	elements := make([]object.Object, len(lines))
	for i, line := range lines {
		elements[i] = object.NewString(line)
	}

	return resultOk(&object.Vec{
			Elements: elements,
			ElemType: "string",
			Size:     -1,
			Mutable:  false,
		})
}

// fsExists checks if a file or directory exists
func fsExists(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.exists", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.exists", args[0], "STRING")
	}

	_, err := os.Stat(path.Value)
	return object.NewBool(err == nil)
}

// fsIsDir checks if path is a directory
func fsIsDir(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.isDir", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.isDir", args[0], "STRING")
	}

	info, err := os.Stat(path.Value)
	if err != nil {
		return object.NewBool(false)
	}
	return object.NewBool(info.IsDir())
}

// fsIsFile checks if path is a regular file
func fsIsFile(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.isFile", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.isFile", args[0], "STRING")
	}

	info, err := os.Stat(path.Value)
	if err != nil {
		return object.NewBool(false)
	}
	return object.NewBool(!info.IsDir())
}

// fsReadDir lists directory contents, returns Result<Vec<DirEntry, _>, string>
func fsReadDir(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.readDir", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.readDir", args[0], "STRING")
	}

	entries, err := os.ReadDir(path.Value)
	if err != nil {
		return resultErr(err)
	}

	elements := make([]object.Object, len(entries))
	for i, entry := range entries {
		fullPath := filepath.Join(path.Value, entry.Name())
		elements[i] = &object.DirEntry{
			FileName: entry.Name(),
			IsDir:    entry.IsDir(),
			FullPath: fullPath,
		}
	}

	return resultOk(&object.Vec{
			Elements: elements,
			ElemType: "DirEntry",
			Size:     -1,
			Mutable:  false,
		})
}

// fsStat returns file info, returns Result<FileInfo, string>
func fsStat(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.stat", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.stat", args[0], "STRING")
	}

	info, err := os.Stat(path.Value)
	if err != nil {
		return resultErr(err)
	}

	return resultOk(&object.FileInfo{
			FileName: info.Name(),
			FileSize: info.Size(),
			ModTime:  info.ModTime().Unix(),
			IsDir:    info.IsDir(),
		})
}

// fsJoin joins path elements
func fsJoin(args ...object.Object) object.Object {
	if len(args) == 0 {
		return newError("fs.join: requires at least one argument")
	}

	parts := make([]string, len(args))
	for i, arg := range args {
		str, ok := arg.(*object.String)
		if !ok {
			return newError("fs.join: argument %d must be STRING, got %s", i+1, arg.Type())
		}
		parts[i] = str.Value
	}

	return object.NewString(filepath.Join(parts...))
}

// fsExt returns the file extension
func fsExt(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.ext", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.ext", args[0], "STRING")
	}

	return object.NewString(filepath.Ext(path.Value))
}

// fsBase returns the last element of path (file name)
func fsBase(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.base", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.base", args[0], "STRING")
	}

	return object.NewString(filepath.Base(path.Value))
}

// fsDir returns all but the last element of path (directory)
func fsDir(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.dir", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.dir", args[0], "STRING")
	}

	return object.NewString(filepath.Dir(path.Value))
}

// fsAbs returns the absolute path
func fsAbs(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.abs", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.abs", args[0], "STRING")
	}

	absPath, err := filepath.Abs(path.Value)
	if err != nil {
		return resultErr(err)
	}

	return resultOk(object.NewString(absPath))
}

// fsWriteFile writes content to a file (creates or overwrites)
func fsWriteFile(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("fs.writeFile", len(args), "2")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return firstArgTypeError("fs.writeFile", args[0], "STRING")
	}

	content, ok := args[1].(*object.String)
	if !ok {
		return secondArgTypeError("fs.writeFile", args[1], "STRING")
	}
	if denied := requireFSMutatePermission("fs.writeFile"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.writeFile"); err != nil {
		return resultErr(err)
	}

	err := os.WriteFile(path.Value, []byte(content.Value), 0644)
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

// fsAppendFile appends content to a file
func fsAppendFile(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("fs.appendFile", len(args), "2")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return firstArgTypeError("fs.appendFile", args[0], "STRING")
	}

	content, ok := args[1].(*object.String)
	if !ok {
		return secondArgTypeError("fs.appendFile", args[1], "STRING")
	}
	if denied := requireFSMutatePermission("fs.appendFile"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.appendFile"); err != nil {
		return resultErr(err)
	}

	f, err := os.OpenFile(path.Value, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return resultErr(err)
	}
	defer f.Close()

	_, err = f.WriteString(content.Value)
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

// fsMkdir creates a directory
func fsMkdir(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.mkdir", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.mkdir", args[0], "STRING")
	}
	if denied := requireFSMutatePermission("fs.mkdir"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.mkdir"); err != nil {
		return resultErr(err)
	}

	err := os.Mkdir(path.Value, 0755)
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

// fsMkdirAll creates a directory and all parent directories
func fsMkdirAll(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.mkdirAll", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.mkdirAll", args[0], "STRING")
	}
	if denied := requireFSMutatePermission("fs.mkdirAll"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.mkdirAll"); err != nil {
		return resultErr(err)
	}

	err := os.MkdirAll(path.Value, 0755)
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

// fsRemove removes a file or empty directory
func fsRemove(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.remove", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.remove", args[0], "STRING")
	}

	if denied := requireFSMutatePermission("fs.remove"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.remove"); err != nil {
		return resultErr(err)
	}

	err := os.Remove(path.Value)
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

// fsRemoveAll removes a file or directory recursively
func fsRemoveAll(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fs.removeAll", len(args), "1")
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("fs.removeAll", args[0], "STRING")
	}

	if denied := requireFSMutatePermission("fs.removeAll"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.removeAll"); err != nil {
		return resultErr(err)
	}

	err := os.RemoveAll(path.Value)
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

func validateDestructivePath(pathValue, op string) error {
	cleaned := filepath.Clean(strings.TrimSpace(pathValue))
	if cleaned == "" || cleaned == "." {
		return fmt.Errorf("%s: refusing to operate on current directory or empty path", op)
	}
	if cleaned == string(filepath.Separator) {
		return fmt.Errorf("%s: refusing to operate on root directory", op)
	}
	// Reject directory traversal attempts
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("%s: refusing path containing directory traversal (..)", op)
		}
	}
	return nil
}

// fsRename renames (moves) a file or directory
func fsRename(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("fs.rename", len(args), "2")
	}

	oldPath, ok := args[0].(*object.String)
	if !ok {
		return firstArgTypeError("fs.rename", args[0], "STRING")
	}

	newPath, ok := args[1].(*object.String)
	if !ok {
		return secondArgTypeError("fs.rename", args[1], "STRING")
	}
	if denied := requireFSMutatePermission("fs.rename"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(oldPath.Value, "fs.rename"); err != nil {
		return resultErr(err)
	}
	if err := validateDestructivePath(newPath.Value, "fs.rename"); err != nil {
		return resultErr(err)
	}

	err := os.Rename(oldPath.Value, newPath.Value)
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

// fsCopy copies a file
func fsCopy(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("fs.copy", len(args), "2")
	}

	srcPath, ok := args[0].(*object.String)
	if !ok {
		return firstArgTypeError("fs.copy", args[0], "STRING")
	}

	dstPath, ok := args[1].(*object.String)
	if !ok {
		return secondArgTypeError("fs.copy", args[1], "STRING")
	}
	if denied := requireFSMutatePermission("fs.copy"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(srcPath.Value, "fs.copy"); err != nil {
		return resultErr(err)
	}
	if err := validateDestructivePath(dstPath.Value, "fs.copy"); err != nil {
		return resultErr(err)
	}

	// Read source file
	content, err := os.ReadFile(srcPath.Value)
	if err != nil {
		return resultErr(err)
	}

	// Write to destination
	err = os.WriteFile(dstPath.Value, content, 0644)
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

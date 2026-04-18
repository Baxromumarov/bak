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
	return &object.Result{
		IsOk:  false,
		Value: &object.String{Value: runtimecap.PermissionError(op, runtimecap.FlagAllowFSMutate)},
	}
}

// fsReadFile reads a file and returns Result<string, string>
func fsReadFile(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.readFile: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.readFile: argument must be STRING, got %s", args[0].Type())
	}

	content, err := os.ReadFile(path.Value)
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	return &object.Result{
		IsOk:  true,
		Value: &object.String{Value: string(content)},
	}
}

// fsReadFileBytes reads a file and returns Result<Vec<int>, string>
func fsReadFileBytes(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.readFileBytes: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.readFileBytes: argument must be STRING, got %s", args[0].Type())
	}

	content, err := os.ReadFile(path.Value)
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	elements := make([]object.Object, len(content))
	for i, b := range content {
		elements[i] = &object.Integer{Value: int64(b)}
	}

	return &object.Result{
		IsOk: true,
		Value: &object.Vec{
			Elements: elements,
			ElemType: "int",
			Size:     -1,
			Mutable:  false,
		},
	}
}

// fsWriteFileBytes writes raw bytes (Vec<int>) to a file
func fsWriteFileBytes(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("fs.writeFileBytes: wrong number of arguments. got=%d, want=2", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.writeFileBytes: first argument must be STRING, got %s", args[0].Type())
	}

	dataObj, ok := args[1].(*object.Vec)
	if !ok {
		return newError("fs.writeFileBytes: second argument must be Vec<int>, got %s", args[1].Type())
	}
	if denied := requireFSMutatePermission("fs.writeFileBytes"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.writeFileBytes"); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
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
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}
	return &object.Result{IsOk: true, Value: &object.Void{}}
}

// fsReadLines reads a file and returns Result<Vec<string>, string>
func fsReadLines(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.readLines: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.readLines: argument must be STRING, got %s", args[0].Type())
	}

	content, err := os.ReadFile(path.Value)
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	lines := strings.Split(string(content), "\n")
	elements := make([]object.Object, len(lines))
	for i, line := range lines {
		elements[i] = &object.String{Value: line}
	}

	return &object.Result{
		IsOk: true,
		Value: &object.Vec{
			Elements: elements,
			ElemType: "string",
			Size:     -1,
			Mutable:  false,
		},
	}
}

// fsExists checks if a file or directory exists
func fsExists(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.exists: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.exists: argument must be STRING, got %s", args[0].Type())
	}

	_, err := os.Stat(path.Value)
	return &object.Boolean{Value: err == nil}
}

// fsIsDir checks if path is a directory
func fsIsDir(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.isDir: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.isDir: argument must be STRING, got %s", args[0].Type())
	}

	info, err := os.Stat(path.Value)
	if err != nil {
		return &object.Boolean{Value: false}
	}
	return &object.Boolean{Value: info.IsDir()}
}

// fsIsFile checks if path is a regular file
func fsIsFile(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.isFile: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.isFile: argument must be STRING, got %s", args[0].Type())
	}

	info, err := os.Stat(path.Value)
	if err != nil {
		return &object.Boolean{Value: false}
	}
	return &object.Boolean{Value: !info.IsDir()}
}

// fsReadDir lists directory contents, returns Result<Vec<DirEntry>, string>
func fsReadDir(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.readDir: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.readDir: argument must be STRING, got %s", args[0].Type())
	}

	entries, err := os.ReadDir(path.Value)
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
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

	return &object.Result{
		IsOk: true,
		Value: &object.Vec{
			Elements: elements,
			ElemType: "DirEntry",
			Size:     -1,
			Mutable:  false,
		},
	}
}

// fsStat returns file info, returns Result<FileInfo, string>
func fsStat(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.stat: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.stat: argument must be STRING, got %s", args[0].Type())
	}

	info, err := os.Stat(path.Value)
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	return &object.Result{
		IsOk: true,
		Value: &object.FileInfo{
			FileName: info.Name(),
			FileSize: info.Size(),
			ModTime:  info.ModTime().Unix(),
			IsDir:    info.IsDir(),
		},
	}
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

	return &object.String{Value: filepath.Join(parts...)}
}

// fsExt returns the file extension
func fsExt(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.ext: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.ext: argument must be STRING, got %s", args[0].Type())
	}

	return &object.String{Value: filepath.Ext(path.Value)}
}

// fsBase returns the last element of path (file name)
func fsBase(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.base: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.base: argument must be STRING, got %s", args[0].Type())
	}

	return &object.String{Value: filepath.Base(path.Value)}
}

// fsDir returns all but the last element of path (directory)
func fsDir(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.dir: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.dir: argument must be STRING, got %s", args[0].Type())
	}

	return &object.String{Value: filepath.Dir(path.Value)}
}

// fsAbs returns the absolute path
func fsAbs(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.abs: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.abs: argument must be STRING, got %s", args[0].Type())
	}

	absPath, err := filepath.Abs(path.Value)
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	return &object.Result{
		IsOk:  true,
		Value: &object.String{Value: absPath},
	}
}

// fsWriteFile writes content to a file (creates or overwrites)
func fsWriteFile(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("fs.writeFile: wrong number of arguments. got=%d, want=2", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.writeFile: first argument must be STRING, got %s", args[0].Type())
	}

	content, ok := args[1].(*object.String)
	if !ok {
		return newError("fs.writeFile: second argument must be STRING, got %s", args[1].Type())
	}
	if denied := requireFSMutatePermission("fs.writeFile"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.writeFile"); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	err := os.WriteFile(path.Value, []byte(content.Value), 0644)
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

// fsAppendFile appends content to a file
func fsAppendFile(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("fs.appendFile: wrong number of arguments. got=%d, want=2", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.appendFile: first argument must be STRING, got %s", args[0].Type())
	}

	content, ok := args[1].(*object.String)
	if !ok {
		return newError("fs.appendFile: second argument must be STRING, got %s", args[1].Type())
	}
	if denied := requireFSMutatePermission("fs.appendFile"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.appendFile"); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	f, err := os.OpenFile(path.Value, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}
	defer f.Close()

	_, err = f.WriteString(content.Value)
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

// fsMkdir creates a directory
func fsMkdir(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.mkdir: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.mkdir: argument must be STRING, got %s", args[0].Type())
	}
	if denied := requireFSMutatePermission("fs.mkdir"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.mkdir"); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	err := os.Mkdir(path.Value, 0755)
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

// fsMkdirAll creates a directory and all parent directories
func fsMkdirAll(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.mkdirAll: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.mkdirAll: argument must be STRING, got %s", args[0].Type())
	}
	if denied := requireFSMutatePermission("fs.mkdirAll"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.mkdirAll"); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	err := os.MkdirAll(path.Value, 0755)
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

// fsRemove removes a file or empty directory
func fsRemove(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.remove: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.remove: argument must be STRING, got %s", args[0].Type())
	}

	if denied := requireFSMutatePermission("fs.remove"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.remove"); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	err := os.Remove(path.Value)
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

// fsRemoveAll removes a file or directory recursively
func fsRemoveAll(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fs.removeAll: wrong number of arguments. got=%d, want=1", len(args))
	}

	path, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.removeAll: argument must be STRING, got %s", args[0].Type())
	}

	if denied := requireFSMutatePermission("fs.removeAll"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(path.Value, "fs.removeAll"); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	err := os.RemoveAll(path.Value)
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
		return newError("fs.rename: wrong number of arguments. got=%d, want=2", len(args))
	}

	oldPath, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.rename: first argument must be STRING, got %s", args[0].Type())
	}

	newPath, ok := args[1].(*object.String)
	if !ok {
		return newError("fs.rename: second argument must be STRING, got %s", args[1].Type())
	}
	if denied := requireFSMutatePermission("fs.rename"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(oldPath.Value, "fs.rename"); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}
	if err := validateDestructivePath(newPath.Value, "fs.rename"); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	err := os.Rename(oldPath.Value, newPath.Value)
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

// fsCopy copies a file
func fsCopy(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("fs.copy: wrong number of arguments. got=%d, want=2", len(args))
	}

	srcPath, ok := args[0].(*object.String)
	if !ok {
		return newError("fs.copy: first argument must be STRING, got %s", args[0].Type())
	}

	dstPath, ok := args[1].(*object.String)
	if !ok {
		return newError("fs.copy: second argument must be STRING, got %s", args[1].Type())
	}
	if denied := requireFSMutatePermission("fs.copy"); denied != nil {
		return denied
	}
	if err := validateDestructivePath(srcPath.Value, "fs.copy"); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}
	if err := validateDestructivePath(dstPath.Value, "fs.copy"); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	// Read source file
	content, err := os.ReadFile(srcPath.Value)
	if err != nil {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: err.Error()},
		}
	}

	// Write to destination
	err = os.WriteFile(dstPath.Value, content, 0644)
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

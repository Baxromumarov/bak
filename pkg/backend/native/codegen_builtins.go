package native

import (
	"fmt"
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"os"
	"strings"
)

func (s *EmitState) emitBuiltinCall(funcName string, e *ast.CallExpression) (bool, error) {
	// User-facing builtins
	switch funcName {
	case "println":
		return true, s.emitBuiltinPrintln(e)
	case "print":
		return true, s.emitBuiltinPrint(e)
	case "len":
		return true, s.emitBuiltinLen(e)
	case "__builtin_string_from_bytes":
		return true, s.emitBuiltinStringFromBytes(e)
	case "__builtin_args":
		return true, s.emitOsArgs()
	case "__builtin_exit":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_exit expects 1 argument")
		}
		return true, s.emitOsExit(e.Arguments[0])
	case "__builtin_getenv":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_getenv expects 1 argument")
		}
		return true, s.emitOsGetenv(e.Arguments[0])
	case "__builtin_setenv":
		if len(e.Arguments) != 2 {
			return true, fmt.Errorf("native: __builtin_setenv expects 2 arguments")
		}
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return true, err
			}
		}
		return true, s.emitResultErrStr("setenv is not supported in native backend")
	case "__builtin_cwd":
		return true, s.emitOsCwd()
	case "__builtin_chdir":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_chdir expects 1 argument")
		}
		return true, s.emitOsChdir(e.Arguments[0])
	case "__builtin_chmod":
		if len(e.Arguments) != 2 {
			return true, fmt.Errorf("native: __builtin_chmod expects 2 arguments")
		}
		return true, s.emitOsChmod(e.Arguments[0], e.Arguments[1])
	case "__builtin_mkdir":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_mkdir expects 1 argument")
		}
		if err := s.requirePermission(
			s.Permissions.AllowFSMutate,
			"__builtin_mkdir",
			runtimecap.FlagAllowFSMutate,
		); err != nil {
			return true, err
		}
		s.emitRuntimePermissionCheck(0x04)
		return true, s.emitOsMkdir(e.Arguments[0])
	case "__builtin_string_ptr":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_string_ptr expects 1 argument")
		}
		if err := s.emitExpression(e.Arguments[0]); err != nil {
			return true, err
		}
		if s.isRefExpression(e.Arguments[0]) {
			s.emitSafeRefDeref()
		}
		s.emitSafeLoadRaxFromRaxDisp(0)
		return true, nil
	}

	// FS builtins
	switch funcName {
	case "__builtin_read_file", "__builtin_read_all":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_read_file expects 1 argument")
		}
		return true, s.emitBuiltinReadFile(e.Arguments[0])
	case "__builtin_write_file":
		if len(e.Arguments) != 2 {
			return true, fmt.Errorf("native: __builtin_write_file expects 2 arguments")
		}
		if err := s.requirePermission(
			s.Permissions.AllowFSMutate,
			"__builtin_write_file",
			runtimecap.FlagAllowFSMutate,
		); err != nil {
			return true, err
		}
		s.emitRuntimePermissionCheck(0x04)
		return true, s.emitFsWriteFile(e.Arguments[0], e.Arguments[1])
	case "__builtin_write_file_bytes":
		if len(e.Arguments) != 2 {
			return true, fmt.Errorf("native: __builtin_write_file_bytes expects 2 arguments")
		}
		if err := s.requirePermission(
			s.Permissions.AllowFSMutate,
			"__builtin_write_file_bytes",
			runtimecap.FlagAllowFSMutate,
		); err != nil {
			return true, err
		}
		s.emitRuntimePermissionCheck(0x04)
		return true, s.emitFsWriteFileBytes(e.Arguments[0], e.Arguments[1])
	case "__builtin_append_file":
		if len(e.Arguments) != 2 {
			return true, fmt.Errorf("native: __builtin_append_file expects 2 arguments")
		}
		if err := s.requirePermission(
			s.Permissions.AllowFSMutate,
			"__builtin_append_file",
			runtimecap.FlagAllowFSMutate,
		); err != nil {
			return true, err
		}
		s.emitRuntimePermissionCheck(0x04)
		return true, s.emitFsAppendFile(e.Arguments[0], e.Arguments[1])
	case "__builtin_remove":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_remove expects 1 argument")
		}
		if err := s.requirePermission(
			s.Permissions.AllowFSMutate,
			"__builtin_remove",
			runtimecap.FlagAllowFSMutate,
		); err != nil {
			return true, err
		}
		s.emitRuntimePermissionCheck(0x04)
		return true, s.emitFsRemove(e.Arguments[0])
	case "__builtin_read_dir":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_read_dir expects 1 argument")
		}
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return true, err
			}
		}
		return true, s.emitResultErrStr("readDir is not supported in native backend")
	case "__builtin_file_exists":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_file_exists expects 1 argument")
		}
		return true, s.emitFsExists(e.Arguments[0])
	case "__builtin_is_file":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_is_file expects 1 argument")
		}
		return true, s.emitFsIsFile(e.Arguments[0])
	case "__builtin_is_dir":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_is_dir expects 1 argument")
		}
		return true, s.emitFsIsDir(e.Arguments[0])
	case "__builtin_temp_dir":
		tmpIdx := s.addStringLiteral("/tmp")
		s.emitDataAddr(tmpIdx)
		return true, nil
	case "__builtin_user_home_dir", "__builtin_hostname", "__builtin_executable":
		return true, s.emitResultErrStr(strfmt.Named("{funcName} is not supported in native backend", "FuncName", funcName))
	case "__builtin_join":
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return true, err
			}
		}
		return true, s.emitEmptyString()
	}

	// IO builtins
	switch funcName {
	case "__builtin_print":
		return true, s.emitBuiltinPrint(e)
	case "__builtin_println":
		return true, s.emitBuiltinPrintln(e)
	case "__builtin_eprint", "__builtin_eprintln":
		return true, s.emitBuiltinPrint(e)
	case "__builtin_read_line":
		return true, s.emitResultErrStr("readLine is not supported in native backend")
	}

	// Time builtins
	switch funcName {
	case "__builtin_time_now":
		return true, s.emitTimeNow(0)
	case "__builtin_monotonic_now":
		return true, s.emitTimeNow(1)
	case "__builtin_time_parts":
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return true, err
			}
		}
		emitXorRegReg(&s.Code, RAX, RAX)
		return true, nil
	case "__builtin_sleep":
		if len(e.Arguments) != 1 {
			return true, fmt.Errorf("native: __builtin_sleep expects 1 argument")
		}
		return true, s.emitSleepMs(e.Arguments[0])
	}

	// Threading / sync stubs
	switch funcName {
	case "__builtin_spawn", "__builtin_thread_id", "__builtin_mutex_new",
		"__builtin_mutex_lock", "__builtin_mutex_unlock":
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return true, err
			}
		}
		emitXorRegReg(&s.Code, RAX, RAX)
		return true, nil
	}

	// Cancel token stubs
	switch funcName {
	case "__builtin_cancel_new", "__builtin_cancel", "__builtin_is_cancelled":
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return true, err
			}
		}
		emitXorRegReg(&s.Code, RAX, RAX)
		return true, nil
	}

	// Database builtins (not supported in native backend)
	switch funcName {
	case "__builtin_pg_connect",
		"__builtin_pg_close",
		"__builtin_pg_query",
		"__builtin_mysql_connect",
		"__builtin_mysql_close",
		"__builtin_mysql_query",
		"__builtin_db_config":
		return true, fmt.Errorf("native backend does not support %s", funcName)
	}

	// Socket builtins (not supported in native backend)
	switch funcName {
	case "__builtin_socket_bind",
		"__builtin_socket_accept",
		"__builtin_socket_connect",
		"__builtin_socket_connect_tls",
		"__builtin_socket_read",
		"__builtin_socket_write",
		"__builtin_socket_close",
		"__builtin_socket_set_timeout":
		return true, fmt.Errorf("native backend does not support %s", funcName)
	}

	// Exec builtin
	if funcName == "__builtin_exec" {
		if len(e.Arguments) != 2 {
			return true, fmt.Errorf("native: __builtin_exec expects 2 arguments")
		}

		if err := s.requirePermission(
			s.Permissions.AllowExec,
			"__builtin_exec",
			runtimecap.FlagAllowExec,
		); err != nil {
			return true, err
		}

		s.emitRuntimePermissionCheck(0x01)
		return true, s.emitOsExec(e.Arguments[0], e.Arguments[1])
	}

	return false, nil
}

func (s *EmitState) emitBuiltinPrintln(e *ast.CallExpression) error {
	if len(e.Arguments) == 0 {
		// println() with no args: just print newline
		// Store '\n' on stack and write it
		emitMovRegImm32(&s.Code, RAX, 0x0A)
		emitPushReg(&s.Code, RAX)
		emitMovRegReg(&s.Code, RSI, RSP) // buf
		emitMovRegImm32(&s.Code, RDX, 1) // len
		emitMovRegImm32(&s.Code, RDI, 1) // stdout
		emitMovRegImm32(&s.Code, RAX, 1) // sys_write
		emitSyscall(&s.Code)
		emitPopReg(&s.Code, RAX) // cleanup
		return nil
	}

	if len(e.Arguments) == 1 {
		arg := e.Arguments[0]
		// Check if it's an integer type
		// println(expr): evaluate expr
		if err := s.emitExpression(arg); err != nil {
			return err
		}
		// If argument is a reference (&string, etc.), dereference first
		if s.isRefExpression(arg) {
			s.emitSafeRefDeref()
		}
		emitMovRegReg(&s.Code, RDI, RAX)

		var printTarget string
		if s.isStringExpression(arg) {
			printTarget = "__rt_print_str"
		} else if s.isFloatExpression(arg) {
			printTarget = "__rt_print_float"
		} else if s.isIntExpression(arg) {
			printTarget = "__rt_print_int"
		} else {
			// Fallback (e.g. for unknown types or pointers) - treat as int/pointer
			printTarget = "__rt_print_int"
		}
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{
			ImmOffset: callSite,
			Target:    printTarget,
		})

		// Now print newline
		emitMovRegImm32(&s.Code, RAX, 0x0A)
		emitPushReg(&s.Code, RAX)
		emitMovRegReg(&s.Code, RSI, RSP)
		emitMovRegImm32(&s.Code, RDX, 1)
		emitMovRegImm32(&s.Code, RDI, 1)
		emitMovRegImm32(&s.Code, RAX, 1)
		emitSyscall(&s.Code)
		emitPopReg(&s.Code, RAX)
		return nil
	}

	// Multiple args: print each with space separator, then newline
	for i, arg := range e.Arguments {
		if i > 0 {
			// Print space
			emitMovRegImm32(&s.Code, RAX, 0x20) // ' '
			emitPushReg(&s.Code, RAX)
			emitMovRegReg(&s.Code, RSI, RSP)
			emitMovRegImm32(&s.Code, RDX, 1)
			emitMovRegImm32(&s.Code, RDI, 1)
			emitMovRegImm32(&s.Code, RAX, 1)
			emitSyscall(&s.Code)
			emitPopReg(&s.Code, RAX)
		}
		if err := s.emitExpression(arg); err != nil {
			return err
		}
		// If argument is a reference, dereference first
		if s.isRefExpression(arg) {
			s.emitSafeRefDeref()
		}
		emitMovRegReg(&s.Code, RDI, RAX)
		var target string
		if s.isStringExpression(arg) {
			target = "__rt_print_str"
		} else if s.isFloatExpression(arg) {
			target = "__rt_print_float"
		} else if s.isIntExpression(arg) {
			target = "__rt_print_int"
		} else {
			// Fallback for unknown types: print as int/pointer
			target = "__rt_print_int"
		}
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{
			ImmOffset: callSite,
			Target:    target,
		})
	}
	// Final newline
	emitMovRegImm32(&s.Code, RAX, 0x0A)
	emitPushReg(&s.Code, RAX)
	emitMovRegReg(&s.Code, RSI, RSP)
	emitMovRegImm32(&s.Code, RDX, 1)
	emitMovRegImm32(&s.Code, RDI, 1)
	emitMovRegImm32(&s.Code, RAX, 1)
	emitSyscall(&s.Code)
	emitPopReg(&s.Code, RAX)
	return nil
}

func (s *EmitState) emitBuiltinPrint(e *ast.CallExpression) error {
	if len(e.Arguments) != 1 {
		return fmt.Errorf("native: print expects 1 argument")
	}
	arg := e.Arguments[0]

	if err := s.emitExpression(arg); err != nil {
		return err
	}
	// If argument is a reference, dereference first
	if s.isRefExpression(arg) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, RDI, RAX)

	var target string
	if s.isStringExpression(arg) {
		target = "__rt_print_str"
	} else if s.isFloatExpression(arg) {
		target = "__rt_print_float"
	} else if s.isIntExpression(arg) {
		target = "__rt_print_int"
	} else {
		target = "__rt_print_int"
	}
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{
		ImmOffset: callSite,
		Target:    target,
	})
	return nil
}

func (s *EmitState) emitBuiltinLen(e *ast.CallExpression) error {
	if len(e.Arguments) != 1 {
		return fmt.Errorf("native: len expects 1 argument")
	}
	if err := s.emitExpression(e.Arguments[0]); err != nil {
		return err
	}
	// If argument is a reference (&Vec, &string, etc.), dereference first
	if s.isRefExpression(e.Arguments[0]) {
		s.emitSafeRefDeref()
	}
	// For strings: header is {ptr:8, len:8}, len is at offset 8
	// For vecs: header is {ptr:8, len:8, cap:8}, len is at offset 8
	s.emitSafeLoadRaxFromRaxDisp(8)
	return nil
}

// emitBuiltinStringFromBytes creates a string from a Vec<int, _> slice
// __builtin_string_from_bytes(vec, start, end) -> string
// Args: RDI = vec ptr, RSI = start, RDX = end
// The vec is Vec<int, _> where each int is a byte value
// Returns string header pointer in RAX
func (s *EmitState) emitBuiltinStringFromBytes(e *ast.CallExpression) error {
	if len(e.Arguments) != 3 {
		return fmt.Errorf("native: __builtin_string_from_bytes expects 3 arguments (vec, start, end)")
	}

	// Evaluate arguments: vec, start, end
	// Push them right-to-left
	if err := s.emitExpression(e.Arguments[2]); err != nil { // end
		return err
	}
	emitPushReg(&s.Code, RAX)

	if err := s.emitExpression(e.Arguments[1]); err != nil { // start
		return err
	}
	emitPushReg(&s.Code, RAX)

	if err := s.emitExpression(e.Arguments[0]); err != nil { // vec
		return err
	}
	// If vec argument is a reference (&Vec), dereference first
	if s.isRefExpression(e.Arguments[0]) {
		s.emitSafeRefDeref()
	}
	// RAX = vec ptr, pop start and end
	emitMovRegReg(&s.Code, RDI, RAX) // RDI = vec ptr
	emitPopReg(&s.Code, RSI)         // RSI = start
	emitPopReg(&s.Code, RDX)         // RDX = end

	// Call runtime function: __rt_string_from_bytes(vec, start, end) -> string ptr
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{
		ImmOffset: callSite,
		Target:    "__rt_string_from_bytes",
	})

	return nil
}

func (s *EmitState) emitMethodCall(e *ast.MethodCallExpression) error {
	// Module-qualified calls: module.func(args)
	if id, ok := e.Object.(*ast.Identifier); ok {
		if err := s.enforceModuleMethodBuiltinContract(
			id.Value,
			e.Method.Value,
			len(e.Arguments),
		); err != nil {
			return err
		}

		// Handle os.* functions
		if id.Value == "os" {
			switch e.Method.Value {
			case "args":
				return s.emitOsArgs()
			case "exit":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: os.exit expects 1 argument")
				}
				return s.emitOsExit(e.Arguments[0])
			case "cwd":
				if len(e.Arguments) != 0 {
					return fmt.Errorf("native: os.cwd expects 0 arguments")
				}
				return s.emitOsCwd()
			case "getenv":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: os.getenv expects 1 argument")
				}
				return s.emitOsGetenv(e.Arguments[0])
			case "exec":
				if len(e.Arguments) != 2 {
					return fmt.Errorf("native: os.exec expects 2 arguments (cmd, args)")
				}
				return s.emitOsExec(e.Arguments[0], e.Arguments[1])
			}
		}

		// Handle strconv.* functions
		if id.Value == "strconv" {
			switch e.Method.Value {
			case "intToString", "itoa":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: strconv.intToString expects 1 argument")
				}
				if err := s.emitExpression(e.Arguments[0]); err != nil {
					return err
				}
				emitMovRegReg(&s.Code, RDI, RAX)
				callSite := emitCallRel32(&s.Code, 0)
				s.CallPatches = append(s.CallPatches, CallPatch{
					ImmOffset: callSite,
					Target:    "__rt_itoa",
				})
				return nil
			case "parseInt", "atoi":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: strconv.parseInt expects 1 argument")
				}
				if err := s.emitExpression(e.Arguments[0]); err != nil {
					return err
				}
				// If argument is a reference (&string), dereference first
				if s.isRefExpression(e.Arguments[0]) {
					s.emitSafeRefDeref()
				}
				emitMovRegReg(&s.Code, RDI, RAX)
				callSite := emitCallRel32(&s.Code, 0)
				s.CallPatches = append(s.CallPatches, CallPatch{
					ImmOffset: callSite,
					Target:    "__rt_atoi",
				})
				return nil
			case "formatInt", "formatHex", "formatOctal", "formatBinary":
				// For now, redirect these to itoa (base 10 only)
				if len(e.Arguments) >= 1 {
					if err := s.emitExpression(e.Arguments[0]); err != nil {
						return err
					}
					emitMovRegReg(&s.Code, RDI, RAX)
					callSite := emitCallRel32(&s.Code, 0)
					s.CallPatches = append(s.CallPatches, CallPatch{
						ImmOffset: callSite,
						Target:    "__rt_itoa",
					})

					return nil
				}
				return fmt.Errorf("native: strconv.%s expects at least 1 argument", e.Method.Value)
			}
		}

		// Handle fs.* functions
		if id.Value == "fs" {
			switch e.Method.Value {
			case "exists":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: fs.exists expects 1 argument")
				}
				return s.emitFsExists(e.Arguments[0])
			case "isFile":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: fs.isFile expects 1 argument")
				}
				return s.emitFsIsFile(e.Arguments[0])
			case "isDir":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: fs.isDir expects 1 argument")
				}
				return s.emitFsIsDir(e.Arguments[0])
			case "readFile":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: fs.readFile expects 1 argument")
				}
				return s.emitFsReadFile(e.Arguments[0])
			case "writeFile":
				if len(e.Arguments) != 2 {
					return fmt.Errorf("native: fs.writeFile expects 2 arguments")
				}
				return s.emitFsWriteFile(e.Arguments[0], e.Arguments[1])
			}
		}

		// Handle enum type constructors in the same module: EnumType.Variant(...)
		// Avoid intercepting instance method calls on local variables with enum-like names.
		if _, isLocal := s.resolveLocal(id.Value); !isLocal {
			if ed, variantIdx, ok := s.findEnumDeclByTypeAndVariant(id.Value, e.Method.Value); ok {
				return s.emitEnumVariantConstruction(ed, variantIdx, e.Arguments)
			}
		}

		// Try to resolve module-qualified call
		alias := id.Value
		// Check if this is an import alias and resolve to actual package name
		if pkgName, ok := s.ImportAliases[alias]; ok {
			alias = pkgName
		}
		qualName := alias + "." + e.Method.Value
		// Check if it's a known function
		if _, found := s.findFunctionParamCount(qualName); found {
			return s.emitQualifiedCall(qualName, e.Arguments)
		}
		// Also try with original alias name
		qualName2 := id.Value + "." + e.Method.Value
		if _, found := s.findFunctionParamCount(qualName2); found {
			return s.emitQualifiedCall(qualName2, e.Arguments)
		}
		// NOTE: Do NOT check as unqualified here - that would incorrectly treat
		// method calls like vec.len() as function calls to a global "len" function.
		// Method calls should fall through to the method handling code below.
	}

	// Handle qualified enum variant constructor: module.EnumType.Variant(payload)
	// e.g., ast.Expression.BooleanLiteral(bl)
	if fa, ok := e.Object.(*ast.FieldAccessExpression); ok {
		// Check if the Object of the FieldAccess is a module alias
		if id, ok := fa.Object.(*ast.Identifier); ok {
			// Check if this is a known import alias (module.EnumType.Variant pattern)
			if _, isImportAlias := s.ImportAliases[id.Value]; isImportAlias {
				enumTypeName := fa.Field.Value // e.g., "TypeExpression"
				variantName := e.Method.Value  // e.g., "Void"

				// Find enum declaration with this type name
				for _, ed := range s.Enums {
					if ed.Name.Value == enumTypeName {
						// Find the variant
						for i, v := range ed.Variants {
							if v.Name.Value == variantName {
								// Found matching enum variant - construct it
								return s.emitEnumVariantConstruction(ed, i, e.Arguments)
							}
						}
						// Variant not found
						return fmt.Errorf(
							"native: enum %s has no variant %s (in function %s)",
							enumTypeName,
							variantName,
							s.CurrentFunc,
						)
					}
				}
				// Enum type not found - collect all enum names for debugging
				var enumNames []string
				for _, ed := range s.Enums {
					enumNames = append(enumNames, ed.Name.Value)
				}
				return fmt.Errorf(
					"native: could not find enum %s (module alias: %s, variant: %s) in %d enums: %v (in function %s)",
					enumTypeName,
					id.Value,
					variantName,
					len(s.Enums),
					enumNames,
					s.CurrentFunc,
				)
			}
		}
	}

	// Vec static methods
	if id, ok := e.Object.(*ast.Identifier); ok && id.Value == "Vec" {
		switch e.Method.Value {
		case "new":
			return s.emitVecNew()
		case "withCap":
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: Vec.withCap expects 1 argument")
			}
			return s.emitVecWithCap(e.Arguments[0])
		case "from":
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: Vec.from expects 1 argument")
			}
			return s.emitVecFrom(e.Arguments[0])
		}
	}

	// Instance methods (including field-access receivers like obj.field.method())
	obj := e.Object
	if _, ok := obj.(*ast.FieldAccessExpression); ok {
		// field-access receivers use the same dispatch as direct instance methods
	}
	method := e.Method.Value
	switch method {
	case "push":
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: push expects 1 argument")
		}
		return s.emitVecPush(obj, e.Arguments[0])
	case "len":
		return s.emitLen(obj)
	case "pop":
		return s.emitVecPop(obj)
	case "first":
		return s.emitVecFirst(obj)
	case "last":
		return s.emitVecLast(obj)
	case "remove":
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: remove expects 1 argument")
		}
		return s.emitVecRemove(obj, e.Arguments[0])
	case "get":
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: get expects 1 argument")
		}
		if s.isStringExpression(obj) {
			return s.emitStringGet(obj, e.Arguments[0])
		}
		return s.emitVecGet(obj, e.Arguments[0])
	case "bytes", "chars":
		return s.emitStringBytes(obj)
	case "isEmpty":
		return s.emitIsEmpty(obj)
	case "contains":
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: contains expects 1 argument")
		}
		return s.emitStringContains(obj, e.Arguments[0])
	case "startsWith":
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: startsWith expects 1 argument")
		}
		return s.emitStringStartsWith(obj, e.Arguments[0])
	case "endsWith":
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: endsWith expects 1 argument")
		}
		return s.emitStringEndsWith(obj, e.Arguments[0])
	case "trim":
		return s.emitStringTrim(obj)
	case "split":
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: split expects 1 argument")
		}
		return s.emitStringSplit(obj, e.Arguments[0])
	case "isOk":
		return s.emitResultIsOk(obj)
	case "isErr":
		return s.emitResultIsErr(obj)
	case "unwrap":
		return s.emitResultUnwrap(obj)
	case "unwrapErr":
		return s.emitResultUnwrapErr(obj)
	case "isSome":
		return s.emitResultIsErr(obj) // tag==1
	case "isNone":
		return s.emitResultIsOk(obj) // tag==0
	case "substring":
		if len(e.Arguments) != 2 {
			return fmt.Errorf("native: substring expects 2 arguments (start, end)")
		}
		return s.emitStringSubstring(obj, e.Arguments[0], e.Arguments[1])
	case "indexOf":
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: indexOf expects 1 argument")
		}
		return s.emitStringIndexOf(obj, e.Arguments[0])
	case "hash":
		return s.emitStringHash(obj)
	case "parseFloat":
		if err := s.emitExpression(obj); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
		emitMovRegReg(&s.Code, RDI, RSP)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{
			ImmOffset: callSite,
			Target:    "strconv.atof",
		})

		emitAddRspImm8(&s.Code, 8)
		return nil
	case "parseInt":
		if err := s.emitExpression(obj); err != nil {
			return err
		}
		if s.isRefExpression(obj) {
			s.emitSafeRefDeref()
		}
		emitMovRegReg(&s.Code, RDI, RAX)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_atoi",
		})
		return nil
	case "toString":
		return s.emitToString(obj)
	}

	// Try to resolve based on receiver type first (avoids ambiguous suffix matches)
	methodName := method
	if recvType := s.getExpressionStructType(e.Object); recvType != "" {
		if fnName, ok := s.resolveMethodFuncName(recvType, methodName); ok {
			receiver := e.Object
			if s.isRefExpression(receiver) {
				receiver = &ast.DerefExpression{Value: receiver}
			}
			allArgs := make([]ast.Expression, 0, len(e.Arguments)+1)
			allArgs = append(allArgs, receiver)
			allArgs = append(allArgs, e.Arguments...)
			return s.emitQualifiedCall(fnName, allArgs)
		}
	}

	// Generic method call: try to find a function with matching name
	// This handles cases like structInstance.method(args) -> Type.method(self, args)
	// or fieldAccess.method(args) -> method called with receiver as first arg
	// Try to find a function that matches the method name (with various type prefixes)
	// For methods like p.l.nextToken(), we need to call a function like Lexer.nextToken(l)
	// Search all functions for one ending with ".methodName"
	for _, fn := range s.Functions {
		if strings.HasSuffix(fn.Name, "."+methodName) {
			// Found a matching method, call it with receiver as first arg
			receiver := e.Object
			if s.isRefExpression(receiver) {
				receiver = &ast.DerefExpression{Value: receiver}
			}
			allArgs := make([]ast.Expression, 0, len(e.Arguments)+1)
			allArgs = append(allArgs, receiver)
			allArgs = append(allArgs, e.Arguments...)
			return s.emitQualifiedCall(fn.Name, allArgs)
		}
	}

	// Fallback: try treating object.method as a qualified function call (e.g., native.ShouldSkipBootstrapFunction)
	if ident, ok := e.Object.(*ast.Identifier); ok {
		qualifiedName := ident.Value + "." + method
		return s.emitQualifiedCall(qualifiedName, e.Arguments)
	}

	return fmt.Errorf("native: unsupported method call %s.%s", e.Object.String(), e.Method.Value)
}

func (s *EmitState) emitQualifiedCall(name string, args []ast.Expression) error {
	argCount := len(args)

	// Check return type size (ABI)
	retType, ok := s.FunctionReturnTypes[name]
	if !ok {
		if _, after, ok0 := strings.Cut(name, "."); ok0 {
			if t, ok2 := s.FunctionReturnTypes[after]; ok2 {
				retType = t
			}
		}
	}
	retSize := s.getTypeSize(retType)
	hasRetPtr := retSize > 8

	regArgLimit := 6
	if hasRetPtr {
		regArgLimit = 5 // first reg taken by ret ptr
	}

	// Calculate how many args go to stack vs regs
	stackArgCount := 0
	if argCount > regArgLimit {
		stackArgCount = argCount - regArgLimit
	}
	regArgCount := min(argCount, regArgLimit)

	// 1. Push stack arguments (reverse order)
	for i := argCount - 1; i >= regArgCount; i-- {
		if err := s.emitExpression(args[i]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
	}

	// 2. Evaluate and push register arguments (reverse order)
	for i := regArgCount - 1; i >= 0; i-- {
		if err := s.emitExpression(args[i]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
	}

	if hasRetPtr {
		// Allocate return slot in caller frame
		retName := "__call_ret_" + name
		offset := s.declareLocal(retName, retSize)
		emitLeaReg(&s.Code, RAX, RBP, offset)
		emitPushReg(&s.Code, RAX)
	}

	// 3. Pop into argument registers left-to-right
	allRegs := []int{RDI, RSI, RDX, RCX, R8, R9}
	popCount := regArgCount
	if hasRetPtr {
		popCount++
	}
	for i := 0; i < popCount; i++ {
		emitPopReg(&s.Code, allRegs[i])
	}

	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: name, Module: s.CurrentModule})

	// 4. Cleanup stack args
	if stackArgCount > 0 {
		emitAddRspImm32(&s.Code, stackArgCount*8)
	}

	return nil
}

func (s *EmitState) emitTuple(e *ast.TupleExpression) error {
	if len(e.Elements) == 2 {
		// Two-element tuple: first in RAX, second in RDX
		if err := s.emitExpression(e.Elements[0]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
		if err := s.emitExpression(e.Elements[1]); err != nil {
			return err
		}
		emitMovRegReg(&s.Code, RDX, RAX)
		emitPopReg(&s.Code, RAX)
		return nil
	}
	return fmt.Errorf("native: only 2-element tuples supported")
}

// ============================================================
//  Enums
// ============================================================

// getEnumVariantTag returns the tag value for a given enum variant name.
// For now, we use a simple hash-based approach since we don't have full enum tracking.
// Standard variants: Ok=0, Err=1, Some=1, None=0
// For ast.Statement variants, we assign sequential tags based on variant name.
func (s *EmitState) getEnumVariantTag(variantName string) int {
	// Standard Result/Option variants
	switch variantName {
	case "Ok":
		return 0
	case "Err":
		return 1
	case "Some":
		return 1
	case "None":
		return 0
	}

	// For custom enums (like ast.Statement), search our enum declarations
	for _, ed := range s.Enums {
		for i, v := range ed.Variants {
			if v.Name.Value == variantName {
				return i
			}
		}
	}

	// Fallback: use simple hash for unknown variants
	// This ensures deterministic behavior
	h := 0
	for _, c := range variantName {
		h = h*31 + int(c)
	}
	return h & 0x7FFFFFFF
}

// findEnumVariantTagByType returns the tag for a variant within a specific enum type.
// Returns -1 if the enum or variant is not found.
func (s *EmitState) findEnumVariantTagByType(enumTypeName, variantName string) int {
	if enumTypeName == "" {
		return -1
	}
	// If qualified, use the simple name
	if strings.Contains(enumTypeName, ".") {
		parts := strings.Split(enumTypeName, ".")
		enumTypeName = parts[len(parts)-1]
	}
	for _, ed := range s.Enums {
		if ed.Name.Value != enumTypeName {
			continue
		}
		for i, v := range ed.Variants {
			if v.Name.Value == variantName {
				return i
			}
		}
		return -1
	}
	return -1
}

// findEnumVariantTagByName returns the tag value for a variant name, or -1 if not found
func (s *EmitState) findEnumVariantTagByName(variantName string) int {
	// Standard Result/Option variants
	switch variantName {
	case "Ok":
		return 0
	case "Err":
		return 1
	case "Some":
		return 1
	case "None":
		return 0
	}

	// Search registered enum declarations
	for _, ed := range s.Enums {
		for i, v := range ed.Variants {
			if v.Name.Value == variantName {
				return i
			}
		}
	}

	return -1 // Not found
}

// findEnumDeclByVariantName finds the enum declaration and variant index for a variant name.
func (s *EmitState) findEnumDeclByVariantName(variantName string) (*ast.EnumDecl, int, bool) {
	for _, ed := range s.Enums {
		for i, v := range ed.Variants {
			if v.Name.Value == variantName {
				return ed, i, true
			}
		}
	}
	return nil, -1, false
}

// findEnumDeclByTypeAndVariant finds a specific enum declaration and variant index.
// enumTypeName may be qualified (module.EnumType) or simple (EnumType).
func (s *EmitState) findEnumDeclByTypeAndVariant(enumTypeName, variantName string) (*ast.EnumDecl, int, bool) {
	if enumTypeName == "" || variantName == "" {
		return nil, -1, false
	}

	enumTypeName = simpleStructName(enumTypeName)
	for _, ed := range s.Enums {
		if simpleStructName(ed.Name.Value) != enumTypeName {
			continue
		}
		for i, v := range ed.Variants {
			if v.Name.Value == variantName {
				return ed, i, true
			}
		}
		return nil, -1, false
	}

	return nil, -1, false
}

func enumDeclIsTagged(ed *ast.EnumDecl) bool {
	for _, v := range ed.Variants {
		if len(v.Fields) > 0 {
			return true
		}
	}
	return false
}

// isTaggedEnumVariant checks if a variant name belongs to an enum that has at least
// one variant with payload fields. Such enums use heap-allocated {tag, payload} structs,
// so even fieldless variants must be heap-allocated to be dereferenceable by switch code.
func (s *EmitState) isTaggedEnumVariant(variantName string) bool {
	// Built-in tagged enums: Option and Result always use heap-allocated representation
	switch variantName {
	case "None", "Some", "Ok", "Err":
		return true
	}

	// Search registered enum declarations
	for _, ed := range s.Enums {
		foundVariant := false
		for _, v := range ed.Variants {
			if v.Name.Value == variantName {
				foundVariant = true
				break
			}
		}
		if !foundVariant {
			continue
		}
		// Check if ANY variant in this enum has payload fields
		for _, v := range ed.Variants {
			if len(v.Fields) > 0 {
				return true // This is a tagged enum
			}
		}
		return false // All variants are fieldless — simple enum
	}
	return false
}

// ============================================================
//  Structs
// ============================================================

func simpleStructName(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx+1 < len(name) {
		return name[idx+1:]
	}
	return name
}

// findStructDecl finds a struct declaration by name
// Supports both qualified names (e.g., "token.Token") and simple names (e.g., "Token")
func (s *EmitState) findStructDecl(name string) *ast.StructDecl {
	if name == "" {
		return nil
	}

	// Exact declaration name match first.
	for _, sd := range s.Structs {
		if sd.Name.Value == name {
			return sd
		}
	}

	queryModule := ""
	querySimple := name
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		queryModule = name[:idx]
		querySimple = name[idx+1:]
	}

	var candidates []*ast.StructDecl
	for _, sd := range s.Structs {
		if simpleStructName(sd.Name.Value) != querySimple {
			continue
		}
		declModule := s.StructDeclModule[sd]
		if queryModule != "" && declModule == queryModule {
			return sd
		}
		candidates = append(candidates, sd)
	}

	// Unqualified lookup: prefer same-module declarations to avoid cross-module collisions.
	if queryModule == "" {
		if s.CurrentModule != "" {
			for _, sd := range candidates {
				if s.StructDeclModule[sd] == s.CurrentModule {
					return sd
				}
			}
		}
	}

	// Deterministic fallback: prefer main-module decls, then first match.
	for _, sd := range candidates {
		mod := s.StructDeclModule[sd]
		if mod == "" || mod == "main" {
			return sd
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return nil
}

// extractStructNameFromType unwraps type wrappers (Borrow, Generic)
// to extract the underlying struct name. Returns empty string if not a struct type.
func (s *EmitState) extractStructNameFromType(t ast.TypeExpression) string {
	switch typ := t.(type) {
	case *ast.SimpleType:
		// Direct struct type
		return typ.Name
	case *ast.BorrowType:
		// &StructName or &mut StructName - unwrap the inner type
		return s.extractStructNameFromType(typ.Inner)
	case *ast.GenericType:
		// Vec<StructName, _>, Option<StructName>, etc. - the base type is the generic name
		// For these, we don't care about the generic - we want the type parameters
		if len(typ.TypeParams) > 0 {
			// Return the first type parameter (e.g., StructName in Vec<StructName, _>)
			return s.extractStructNameFromType(typ.TypeParams[0])
		}
		return ""
	default:
		// Not a struct type (void, int, bool, etc.)
		return ""
	}
}

// getFieldTypeSize returns the byte size of a field type for offset calculation
// Most types are 8 bytes (pointers), but some inline types need their full size
func (s *EmitState) getFieldTypeSize(fieldType ast.TypeExpression) int {
	return s.getFieldStorageSize(fieldType)
}

// getFieldStorageSize returns the actual storage size of a field type
// Vec is 8 bytes (pointer to 24-byte header), string is 8 bytes (pointer to header), etc.
// Structs with Vec/string fields store POINTERS to heap-allocated headers, not inline data.
func (s *EmitState) getFieldStorageSize(fieldType ast.TypeExpression) int {
	_ = fieldType
	// All types are stored as 8-byte values (pointers or primitives)
	// Vec header, string header, and other complex types are heap-allocated
	// and the struct stores a pointer to them
	return 8
}

// getFieldOffset calculates the byte offset of a field in a struct
// Uses proper field sizes based on type
func (s *EmitState) getFieldOffset(sd *ast.StructDecl, fieldName string) (int, int) {
	offset := 0
	for i, f := range sd.Fields {
		if f.Name.Value == fieldName {
			return offset, i
		}
		offset += s.getFieldTypeSize(f.Type)
	}
	return -1, -1
}

// getStructSize returns the total size of a struct in bytes
func (s *EmitState) getStructSize(sd *ast.StructDecl) int {
	size := 0
	for _, f := range sd.Fields {
		size += s.getFieldTypeSize(f.Type)
	}
	return size
}

func (s *EmitState) emitStructLiteral(e *ast.StructLiteral) error {
	// Find struct declaration
	sd := s.findStructDecl(e.Name.Value)
	if sd == nil {
		return fmt.Errorf("native: unknown struct type '%s'", e.Name.Value)
	}

	// Calculate struct size
	size := s.getStructSize(sd)
	if size == 0 {
		size = 8 // Minimum allocation
	}

	// Allocate memory for struct
	emitMovRegImm32(&s.Code, RDI, int32(size))
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	// RAX = pointer to allocated struct

	// Initialize each field in order
	for _, field := range sd.Fields {
		fieldName := field.Name.Value
		offset, _ := s.getFieldOffset(sd, fieldName)
		if offset < 0 {
			continue
		}

		// Get the value expression for this field
		value, exists := e.Fields[fieldName]
		if !exists {
			// Field not specified, initialize to 0
			emitMovRegImm32(&s.Code, RDX, 0)
			emitMovMemRegBaseDisp(&s.Code, RAX, offset, RDX)
			continue
		}

		// Preserve struct pointer across field evaluation
		emitPushReg(&s.Code, RAX)

		// Evaluate field value
		if err := s.emitExpression(value); err != nil {
			return err
		}
		// RAX = field value (8 bytes - pointer for Vec/string, value for int)

		// Restore struct pointer and store the 8-byte value
		emitPopReg(&s.Code, RCX)
		emitMovMemRegBaseDisp(&s.Code, RCX, offset, RAX)
		emitMovRegReg(&s.Code, RAX, RCX)
	}

	return nil
}

func (s *EmitState) emitFieldAccess(e *ast.FieldAccessExpression) error {
	// Handle nested field access for module.EnumType.Variant pattern (e.g., ast.Visibility.Public)
	// In this case, e.Object is FieldAccess{Object: module, Field: EnumType}
	if innerFA, ok := e.Object.(*ast.FieldAccessExpression); ok {
		if moduleId, ok := innerFA.Object.(*ast.Identifier); ok {
			if _, isImportAlias := s.ImportAliases[moduleId.Value]; isImportAlias {
				enumTypeName := innerFA.Field.Value // e.g., "Visibility"
				variantName := e.Field.Value        // e.g., "Public"

				// Find enum declaration with this type name
				for _, ed := range s.Enums {
					if ed.Name.Value == enumTypeName {
						// Find the variant
						for i, v := range ed.Variants {
							if v.Name.Value == variantName {
								// Return the tag value for simple enum variants
								emitMovRegImm32(&s.Code, RAX, int32(i))
								return nil
							}
						}
						return fmt.Errorf("native: enum %s has no variant %s", enumTypeName, variantName)
					}
				}
			}
		}
	}

	// Check if this is a module-qualified constant (e.g., x86.RBP)
	if id, ok := e.Object.(*ast.Identifier); ok {
		// First, check if the object is an enum type name (e.g., TokenType.ILLEGAL)
		for _, ed := range s.Enums {
			if ed.Name.Value == id.Value {
				// This is enum variant access: EnumType.Variant
				for i, v := range ed.Variants {
					if v.Name.Value == e.Field.Value {
						// Return the tag value for simple enum variants
						emitMovRegImm32(&s.Code, RAX, int32(i))
						return nil
					}
				}
				return fmt.Errorf("native: enum %s has no variant %s", id.Value, e.Field.Value)
			}
		}

		// Check if it's an import alias
		if pkgName, isAlias := s.ImportAliases[id.Value]; isAlias {
			// Try to find a constant with this qualified name
			qualName := pkgName + "." + e.Field.Value
			for _, c := range s.Constants {
				if c.Name.Value == e.Field.Value {
					// Check if this constant comes from the right module
					// For now, just emit the constant value
					return s.emitExpression(c.Value)
				}
			}
			// Also try the qualified constant name
			for _, c := range s.Constants {
				if c.Name.Value == qualName {
					return s.emitExpression(c.Value)
				}
			}
			// Could also be an enum variant from a module
			tag := s.findEnumVariantTagByName(e.Field.Value)
			if tag >= 0 {
				emitMovRegImm32(&s.Code, RAX, int32(tag))
				return nil
			}
		}
	}

	// Evaluate object to get struct pointer
	if err := s.emitExpression(e.Object); err != nil {
		return err
	}
	// RAX = struct pointer
	// If object is a reference (&Struct), dereference once
	if s.isRefExpression(e.Object) {
		s.emitSafeRefDeref()
	}

	// Get struct type name using type tracking
	structName := s.getExpressionStructType(e.Object)

	// Fallback to simple cases
	if structName == "" {
		switch obj := e.Object.(type) {
		case *ast.Identifier:
			// Look up the variable type in scope
			structName = obj.Value
		case *ast.StructLiteral:
			structName = obj.Name.Value
		}
	}

	fieldName := e.Field.Value

	// If we know the struct type, look up in that specific struct first.
	// If the direct lookup misses (e.g., due same-name structs across modules),
	// retry only same-name candidates that actually define the field.
	if structName != "" {
		if sd := s.findStructDecl(structName); sd != nil {
			offset, _ := s.getFieldOffset(sd, fieldName)
			if offset >= 0 {
				s.emitSafeLoadRaxFromRaxDisp(offset)
				return nil
			}
		}
		// Try all declarations with matching simple/qualified type name.
		for _, sd := range s.Structs {
			sdName := sd.Name.Value
			nameMatch := sdName == structName || strings.HasSuffix(sdName, "."+structName)
			if strings.Contains(structName, ".") {
				parts := strings.Split(structName, ".")
				simple := parts[len(parts)-1]
				nameMatch = nameMatch || sdName == simple || strings.HasSuffix(sdName, "."+simple)
			}
			if !nameMatch {
				continue
			}
			offset, _ := s.getFieldOffset(sd, fieldName)
			if offset >= 0 {
				s.emitSafeLoadRaxFromRaxDisp(offset)
				return nil
			}
		}
	}

	// Fallback: try all known structs to find the field
	for _, sd := range s.Structs {
		offset, _ := s.getFieldOffset(sd, fieldName)
		if offset >= 0 {
			// Found the field, load it
			_, _ = strfmt.Fprintln(
				os.Stderr,
				"[WARN] field '", fieldName,
				"' resolved via fallback to struct '", sd.Name.Value,
				"' (offset=", offset, ") in module=", s.CurrentModule,
				", wanted struct=", structName,
			)
			s.emitSafeLoadRaxFromRaxDisp(offset)
			return nil
		}
	}

	return fmt.Errorf("native: cannot resolve field '%s'", fieldName)
}

func (s *EmitState) emitIndex(e *ast.IndexExpression) error {
	// Evaluate index first
	if err := s.emitExpression(e.Index); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save index

	// Evaluate left (container)
	if err := s.emitExpression(e.Left); err != nil {
		return err
	}
	// rax = container ptr (Vec or string header)
	// If left is a reference (&Vec or &string), dereference first
	if s.isRefExpression(e.Left) {
		s.emitSafeRefDeref()
	}
	// Load data ptr from container[0]
	emitMovRegMemBaseDisp(&s.Code, R11, RAX, 0) // r11 = data ptr

	emitPopReg(&s.Code, RDX) // rdx = index

	// String indexing: load a single byte (no *8 scaling)
	if s.isStringExpression(e.Left) {
		emitMovzxRaxMem8BaseIdx(&s.Code, R11, RDX)
		return nil
	}

	// Calculate data + index * 8 (Vec stores all elements as 8-byte slots)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDX), 0x03) // shl rdx, 3
	emitAddRegReg(&s.Code, RDX, R11)                                           // rdx = &data[index]
	emitMovRegMemBaseDisp(&s.Code, RAX, RDX, 0)                                // rax = data[index]
	return nil
}

func (s *EmitState) emitEnumVariantExpression(e *ast.EnumVariantExpression) error {
	// Handle None (Option::None) - no payload, just tag=0
	if e.Variant.Value == "None" && len(e.Values) == 0 {
		// Allocate tag+payload (payload will be 0)
		emitMovRegImm32(&s.Code, RDI, 16)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
		// Store tag=0 (None)
		emitMovRegImm32(&s.Code, RCX, 0)
		emitMovMemReg(&s.Code, RAX, RCX)
		// Store payload=0
		emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
		return nil
	}
	if e.Variant.Value == "Some" && len(e.Values) == 1 {
		// Evaluate the inner value
		if err := s.emitExpression(e.Values[0]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX) // save value
		// Allocate tag+payload
		emitMovRegImm32(&s.Code, RDI, 16)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
		// Store tag=1 (Some)
		emitMovRegImm32(&s.Code, RCX, 1)
		emitMovMemReg(&s.Code, RAX, RCX)
		// Store payload
		emitPopReg(&s.Code, RCX)
		emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
		return nil
	}
	if e.Variant.Value == "Ok" && len(e.Values) == 1 {
		if err := s.emitExpression(e.Values[0]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
		emitMovRegImm32(&s.Code, RDI, 16)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
		emitMovRegImm32(&s.Code, RCX, 0) // tag=0 (Ok)
		emitMovMemReg(&s.Code, RAX, RCX)
		emitPopReg(&s.Code, RCX)
		emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
		return nil
	}
	if e.Variant.Value == "Err" && len(e.Values) == 1 {
		if err := s.emitExpression(e.Values[0]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
		emitMovRegImm32(&s.Code, RDI, 16)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
		emitMovRegImm32(&s.Code, RCX, 1) // tag=1 (Err)
		emitMovMemReg(&s.Code, RAX, RCX)
		emitPopReg(&s.Code, RCX)
		emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
		return nil
	}
	return fmt.Errorf("native: unsupported enum variant %s", e.Variant.Value)
}

// emitUnwrapExpression implements the ? operator for Result<T,E> and Option<T>.
//
// For Result<T,E> (tag layout: Ok=0, Err=1):
//   - If tag == 0 (Ok): unwrap to payload at [RAX+8]
//   - If tag != 0 (Err): propagate by returning the Result as-is
//
// For Option<T> (tag layout: None=0, Some=1):
//   - If tag == 1 (Some): unwrap to payload at [RAX+8]
//   - If tag == 0 (None): propagate by returning the Option as-is
func (s *EmitState) emitUnwrapExpression(e *ast.UnwrapExpression) error {
	// Evaluate the inner expression -> RAX (pointer to {tag, payload})
	if err := s.emitExpression(e.Value); err != nil {
		return err
	}

	// Determine if this is Result or Option based on enclosing function's return type
	isOption := false
	if retType := s.FunctionReturnTypes[s.CurrentFunc]; retType != nil {
		if gt, ok := retType.(*ast.GenericType); ok && gt.Name == "Option" {
			isOption = true
		}
	}

	// Save the struct pointer on the stack (emitSafeLoadRaxFromRaxDisp clobbers RCX)
	emitPushReg(&s.Code, RAX)

	// Load tag from [RAX+0]
	s.emitSafeLoadRaxFromRaxDisp(0)

	if isOption {
		// Option: tag 0 = None (propagate), tag 1 = Some (unwrap)
		emitCmpRegImm32(&s.Code, RAX, 0)
		// jne -> ok_path (tag != 0 means Some, continue)
		jneOk := len(s.Code)
		s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jne rel32

		// None path: propagate by returning the Option struct
		emitPopReg(&s.Code, RAX) // RAX = original struct pointer
		if err := s.emitDeferredBodies(); err != nil {
			return err
		}
		emitMovRegReg(&s.Code, RSP, RBP)
		emitPopReg(&s.Code, RBP)
		emitRet(&s.Code)

		// ok_path: unwrap Some payload
		patchU32(s.Code, jneOk+2, uint32(len(s.Code)-(jneOk+6)))
	} else {
		// Result: tag 0 = Ok (unwrap), tag 1 = Err (propagate)
		emitCmpRegImm32(&s.Code, RAX, 0)
		// je -> ok_path (tag == 0 means Ok, continue)
		jeOk := len(s.Code)
		s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // je rel32

		// Err path: propagate by returning the Result struct
		emitPopReg(&s.Code, RAX) // RAX = original struct pointer
		if err := s.emitDeferredBodies(); err != nil {
			return err
		}
		emitMovRegReg(&s.Code, RSP, RBP)
		emitPopReg(&s.Code, RBP)
		emitRet(&s.Code)

		// ok_path: unwrap Ok payload
		patchU32(s.Code, jeOk+2, uint32(len(s.Code)-(jeOk+6)))
	}

	// Unwrap: restore struct pointer from stack, load payload from [RAX+8]
	emitPopReg(&s.Code, RAX)
	s.emitSafeLoadRaxFromRaxDisp(8)

	return nil
}

// emitEnumVariantConstruction constructs a tagged enum variant from arguments
// Used for qualified enum construction like ast.Expression.BooleanLiteral(bl)
func (s *EmitState) emitEnumVariantConstruction(ed *ast.EnumDecl, variantIdx int, args []ast.Expression) error {
	variant := ed.Variants[variantIdx]
	tag := int32(variantIdx)
	expectedArgs := len(variant.Fields)
	if len(args) != expectedArgs {
		return fmt.Errorf("native: enum variant %s.%s expects %d arguments, got %d",
			ed.Name.Value, variant.Name.Value, expectedArgs, len(args))
	}

	isTagged := enumDeclIsTagged(ed)
	if expectedArgs == 0 && !isTagged {
		// Fieldless enums without payload variants are represented as raw integer tags.
		emitMovRegImm32(&s.Code, RAX, tag)
		return nil
	}

	// Evaluate payload arguments and keep them on stack.
	for i := range args {
		if err := s.emitExpression(args[i]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
	}

	allocSize := 16
	if expectedArgs > 0 {
		allocSize = 8 + expectedArgs*8
	}
	emitMovRegImm32(&s.Code, RDI, int32(allocSize))
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})

	// Store tag at offset 0.
	emitMovRegImm32(&s.Code, RCX, tag)
	emitMovMemReg(&s.Code, RAX, RCX)

	// For unit variants in tagged enums, store zero payload at offset 8.
	if expectedArgs == 0 {
		emitMovRegImm32(&s.Code, RCX, 0)
		emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
		return nil
	}

	// Pop payload values in reverse and store in slot order.
	for i := len(args) - 1; i >= 0; i-- {
		emitPopReg(&s.Code, RCX)
		payloadOffset := 8 + i*8
		emitMovMemBaseDispReg(&s.Code, RAX, payloadOffset, RCX)
	}

	return nil
}

func (s *EmitState) emitVecNew() error {
	// Allocate Vec header {ptr:8, len:8, cap:8} = 24 bytes
	emitMovRegImm32(&s.Code, RDI, 24)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	// Zero out the header
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX)             // ptr = 0
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)  // len = 0
	emitMovMemBaseDispReg(&s.Code, RAX, 16, RCX) // cap = 0
	return nil
}

func (s *EmitState) emitVecWithCap(capExpr ast.Expression) error {
	// Evaluate capacity
	if err := s.emitExpression(capExpr); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save cap

	// Allocate header
	emitMovRegImm32(&s.Code, RDI, 24)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	emitPushReg(&s.Code, RAX) // save header ptr

	// Allocate data: cap * 8 bytes
	emitMovRegMemBaseDisp(&s.Code, RDI, RSP, 8) // cap from earlier push
	// Multiply by 8: shl rdi, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDI), 0x03)
	callSite2 := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite2, Target: "__rt_alloc"})

	// Set up header: ptr = data, len = 0, cap = cap
	emitPopReg(&s.Code, RCX)         // header ptr
	emitMovMemReg(&s.Code, RCX, RAX) // ptr = data
	emitMovRegImm32(&s.Code, RAX, 0)
	emitMovMemBaseDispReg(&s.Code, RCX, 8, RAX)  // len = 0
	emitPopReg(&s.Code, RAX)                     // cap
	emitMovMemBaseDispReg(&s.Code, RCX, 16, RAX) // cap = cap
	emitMovRegReg(&s.Code, RAX, RCX)             // return header ptr
	return nil
}

func (s *EmitState) emitVecFrom(elemExpr ast.Expression) error {
	// Vec.from expects a VecLiteral (array literal like [a, b, c])
	vecLit, ok := elemExpr.(*ast.VecLiteral)
	if !ok {
		return fmt.Errorf("native: Vec.from expects a vec literal, got %T", elemExpr)
	}

	numElems := len(vecLit.Elements)

	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)

	// Allocate Vec header (24 bytes: ptr, len, cap)
	emitMovRegImm32(&s.Code, RDI, 24)
	callHeader := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callHeader, Target: "__rt_alloc"})
	emitMovRegReg(&s.Code, R12, RAX) // R12 = vec header ptr

	if numElems > 0 {
		// Allocate data buffer (numElems * 8 bytes)
		emitMovRegImm32(&s.Code, RDI, int32(numElems*8))
		callData := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callData, Target: "__rt_alloc"})
		emitMovRegReg(&s.Code, R13, RAX) // R13 = data ptr

		// Store header fields
		emitMovMemReg(&s.Code, R12, R13) // vec.ptr = data
		emitMovRegImm32(&s.Code, RAX, int32(numElems))
		emitMovMemBaseDispReg(&s.Code, R12, 8, RAX)  // vec.len = numElems
		emitMovMemBaseDispReg(&s.Code, R12, 16, RAX) // vec.cap = numElems

		// Evaluate and store each element
		for i, elem := range vecLit.Elements {
			if err := s.emitExpression(elem); err != nil {
				return err
			}
			// Store at data[i*8]
			offset := i * 8
			emitMovMemBaseDispReg(&s.Code, R13, offset, RAX)
		}
	} else {
		// Empty vec
		emitXorRegReg(&s.Code, RAX, RAX)
		emitMovMemReg(&s.Code, R12, RAX)             // vec.ptr = 0
		emitMovMemBaseDispReg(&s.Code, R12, 8, RAX)  // vec.len = 0
		emitMovMemBaseDispReg(&s.Code, R12, 16, RAX) // vec.cap = 0
	}

	// Return vec header
	emitMovRegReg(&s.Code, RAX, R12)

	// Restore callee-saved registers
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	return nil
}

// emitVecLiteral handles Vec literal expressions like [a, b, c]
func (s *EmitState) emitVecLiteral(vecLit *ast.VecLiteral) error {
	return s.emitVecFrom(vecLit)
}

// ============================================================
//  Runtime Builtins
// ============================================================

func (s *EmitState) emitOsArgs() error {
	// Load args from cached location or build from initial stack pointer.
	// For now, call the __rt_args runtime function if registered, or emit inline.
	// Check if we have the args cache data slot
	if s.ArgsCacheDataIndex < 0 {
		// Allocate 24 bytes for Vec header cache
		data := make([]byte, 24)
		s.ArgsCacheDataIndex = s.addDataItem(data, 8)
	}

	// Emit code to check if cache is populated, if not build args
	// Load &cache
	s.emitDataAddr(s.ArgsCacheDataIndex)
	// rax = &cache
	// Load cache.ptr to check if populated
	emitMovRegMemBaseDisp(&s.Code, R8, RAX, 0) // r8 = cache.ptr
	// test r8, r8
	s.Code = append(s.Code, rexByte(1, regHi(R8), 0, regHi(R8)), 0x85, modRM(3, R8, R8))
	// jnz skip_build (cache already populated, return it)
	jnzPos := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz rel32

	// --- Build args from initial RSP ---
	// Load initial rsp
	if s.InitRspDataIndex < 0 {
		data := make([]byte, 8)
		s.InitRspDataIndex = s.addDataItem(data, 8)
	}
	s.emitDataAddr(s.InitRspDataIndex)
	emitMovRegMemBaseDisp(&s.Code, R10, RAX, 0) // r10 = initial rsp
	emitMovRegMemBaseDisp(&s.Code, R8, R10, 0)  // r8 = argc
	emitMovRegReg(&s.Code, R11, R10)
	emitMovRegImm32(&s.Code, RAX, 8)
	emitAddRegReg(&s.Code, R11, RAX) // r11 = argv base

	// Allocate data array (argc * 8 bytes for pointers to string headers)
	emitMovRegReg(&s.Code, RDX, R8)
	// Multiply by 8: shl rdx, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDX), 0x03)
	emitPushReg(&s.Code, R8)
	emitPushReg(&s.Code, R11)
	emitMovRegReg(&s.Code, RDI, RDX)
	callSiteData := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteData, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R11)
	emitPopReg(&s.Code, R8)
	emitMovRegReg(&s.Code, R9, RAX) // r9 = data ptr (for pointers)

	// Allocate Vec header (24 bytes)
	emitPushReg(&s.Code, R8)
	emitPushReg(&s.Code, R9)
	emitPushReg(&s.Code, R11)
	emitSubRspImm8(&s.Code, 8)
	emitMovRegImm32(&s.Code, RDI, 24)
	callSiteHeader := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteHeader, Target: "__rt_alloc"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R11)
	emitPopReg(&s.Code, R9)
	emitPopReg(&s.Code, R8)
	emitMovRegReg(&s.Code, RCX, RAX) // rcx = header ptr

	// Setup header: ptr, len, cap
	emitMovMemReg(&s.Code, RCX, R9)             // header.ptr = data
	emitMovMemBaseDispReg(&s.Code, RCX, 8, R8)  // header.len = argc
	emitMovMemBaseDispReg(&s.Code, RCX, 16, R8) // header.cap = argc

	// Loop to copy argv strings to data array
	// R8 = argc (loop counter), R9 = data ptr, R11 = argv ptr, RCX = header
	emitPushReg(&s.Code, RCX)        // save header
	emitMovRegImm32(&s.Code, R12, 0) // i = 0
	loopStart := len(s.Code)
	// Compare i < argc
	s.Code = append(s.Code, rexByte(1, regHi(R8), 0, regHi(R12)), 0x39, modRM(3, R8, R12)) // cmp r12, r8
	jgeEnd := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge end

	// Load argv[i] (C string pointer)
	// r11 + i*8 = argv[i]
	emitMovRegReg(&s.Code, RAX, R12)
	// Multiply by 8: shl rax, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RAX), 0x03)
	emitAddRegReg(&s.Code, RAX, R11)            // rax = &argv[i]
	emitMovRegMemBaseDisp(&s.Code, R13, RAX, 0) // r13 = argv[i] (char*)

	// Calculate string length (strlen)
	emitMovRegReg(&s.Code, RDI, R13)
	emitMovRegImm32(&s.Code, R14, 0) // len = 0
	strlenLoop := len(s.Code)
	// Load byte at [rdi]
	s.Code = append(s.Code, 0x40, 0x8A, 0x07) // mov al, [rdi] (8-bit)
	// test al, al
	s.Code = append(s.Code, 0x84, 0xC0)
	// jz strlen_done
	jzStrlenDone := len(s.Code)
	s.Code = append(s.Code, 0x74, 0) // jz rel8
	// inc r14
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R14)), 0xFF, modRM(3, 0, R14))
	// inc rdi
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RDI)), 0xFF, modRM(3, 0, RDI))
	// jmp strlen_loop
	rel := strlenLoop - (len(s.Code) + 2)
	s.Code = append(s.Code, 0xEB, byte(rel))
	// strlen_done:
	s.Code[jzStrlenDone+1] = byte(len(s.Code) - jzStrlenDone - 2)

	// Now r13 = ptr (stack), r14 = len
	// Need to allocate heap memory and copy the string content
	// Allocate string content buffer (r14 bytes)
	emitPushReg(&s.Code, R8)
	emitPushReg(&s.Code, R9)
	emitPushReg(&s.Code, R11)
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitSubRspImm8(&s.Code, 8)       // align stack
	emitMovRegReg(&s.Code, RDI, R14) // allocate len bytes for content
	callSiteContent := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteContent, Target: "__rt_alloc"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitPopReg(&s.Code, R11)
	emitPopReg(&s.Code, R9)
	emitPopReg(&s.Code, R8)
	// RAX = content buffer ptr (on heap)
	// Copy string content from r13 (stack) to rax (heap)
	// Use rep movsb: RDI=dest, RSI=src, RCX=count
	emitPushReg(&s.Code, RAX)           // save content ptr
	emitMovRegReg(&s.Code, RDI, RAX)    // dest = heap buffer
	emitMovRegReg(&s.Code, RSI, R13)    // src = stack string
	emitMovRegReg(&s.Code, RCX, R14)    // count = len
	s.Code = append(s.Code, 0xF3, 0xA4) // rep movsb
	emitPopReg(&s.Code, R13)            // R13 now = heap content ptr

	// Allocate string header (16 bytes)
	emitPushReg(&s.Code, R8)
	emitPushReg(&s.Code, R9)
	emitPushReg(&s.Code, R11)
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitSubRspImm8(&s.Code, 8) // align stack
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteStr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteStr, Target: "__rt_alloc"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitPopReg(&s.Code, R11)
	emitPopReg(&s.Code, R9)
	emitPopReg(&s.Code, R8)
	// RAX = string header ptr
	// Store ptr (heap) and len in the header
	emitMovMemReg(&s.Code, RAX, R13)            // header.ptr = r13 (now heap ptr)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R14) // header.len = r14
	emitMovRegReg(&s.Code, R15, RAX)            // R15 = string header ptr

	// Store string header pointer at data[i]
	// data + i*8 = &data[i]
	emitMovRegReg(&s.Code, RAX, R12)
	// Multiply by 8: shl rax, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RAX), 0x03)
	emitAddRegReg(&s.Code, RAX, R9)  // rax = &data[i]
	emitMovMemReg(&s.Code, RAX, R15) // data[i] = string header ptr

	// i++
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R12)), 0xFF, modRM(3, 0, R12))
	// jmp loop_start (use rel32 since loop body is large)
	jmpLoopStart := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp rel32 placeholder
	rel32 := int32(loopStart - (len(s.Code)))
	patchU32(s.Code, jmpLoopStart+1, uint32(rel32))

	// end:
	endPos := len(s.Code)
	patchRel32(&s.Code, jgeEnd+2, endPos)

	emitPopReg(&s.Code, RCX) // restore header

	// Store header in cache
	s.emitDataAddr(s.ArgsCacheDataIndex)
	emitMovMemReg(&s.Code, RAX, RCX)
	emitMovRegReg(&s.Code, RAX, RCX) // return header ptr

	// jmp to end (skip cached return path)
	jmpEnd := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp rel8

	// Patch jnz to here (return cached)
	returnCached := len(s.Code)
	patchRel32(&s.Code, jnzPos+2, returnCached)

	// Return cached: load header ptr from cache
	s.emitDataAddr(s.ArgsCacheDataIndex)
	s.emitSafeLoadRaxFromRaxDisp(0)

	// Patch jmp end
	s.Code[jmpEnd+1] = byte(len(s.Code) - jmpEnd - 2)

	return nil
}

// ============================================================
//  Vector Operations
// ============================================================

// emitVecPush implements vec.push(value)
// Vec layout: [ptr:8][len:8][cap:8]
func (s *EmitState) emitVecPush(obj ast.Expression, value ast.Expression) error {
	// Evaluate vec first and save in callee-saved register R12
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If obj is a reference (&mut Vec), dereference once to get the Vec header pointer.
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitPushReg(&s.Code, R12)        // save old R12
	emitMovRegReg(&s.Code, R12, RAX) // R12 = vec ptr

	// Treat misaligned/tagged immediates as null vec pointers.
	emitMovRegReg(&s.Code, RAX, R12)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xA9, 0x07, 0x00, 0x00, 0x00) // test rax, 7
	jzAlignedVec := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz aligned
	emitMovRegImm32(&s.Code, R12, 0)
	alignedVecPos := len(s.Code)
	patchRel32(&s.Code, jzAlignedVec+2, alignedVecPos)

	// Defensive init: if vec header pointer is null, allocate an empty header.
	// This avoids null deref when pushing into vectors that were not materialized.
	emitTestRegReg(&s.Code, R12, R12)
	jnzVecReady := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz rel32

	emitMovRegImm32(&s.Code, RDI, 24) // Vec header size
	callSiteHeader := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteHeader, Target: "__rt_alloc"})
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX)             // ptr = 0
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)  // len = 0
	emitMovMemBaseDispReg(&s.Code, RAX, 16, RCX) // cap = 0
	emitMovRegReg(&s.Code, R12, RAX)             // use newly allocated header

	// Persist the initialized Vec header back when the receiver address is valid.
	// Field/index receivers are common in backend state updates.
	switch obj.(type) {
	case *ast.Identifier, *ast.MutableIdentifier, *ast.FieldAccessExpression, *ast.IndexExpression:
		emitPushReg(&s.Code, R12)
		if err := s.emitAddressOf(obj); err != nil {
			emitPopReg(&s.Code, R12)
			return err
		}
		emitPopReg(&s.Code, R12)
		emitTestRegReg(&s.Code, RAX, RAX)
		jzSkipStore := len(s.Code)
		s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0)                            // jz skip
		s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xA9, 0x07, 0x00, 0x00, 0x00) // test rax, 7
		jnzSkipStore := len(s.Code)
		s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz skip
		emitMovMemReg(&s.Code, RAX, R12)
		skipStorePos := len(s.Code)
		patchRel32(&s.Code, jzSkipStore+2, skipStorePos)
		patchRel32(&s.Code, jnzSkipStore+2, skipStorePos)
	}

	vecReadyPos := len(s.Code)
	patchRel32(&s.Code, jnzVecReady+2, vecReadyPos)

	// Evaluate value and save in R13
	if err := s.emitExpression(value); err != nil {
		return err
	}
	emitPushReg(&s.Code, R13)        // save old R13
	emitMovRegReg(&s.Code, R13, RAX) // R13 = value

	// Load len and cap from vec header
	emitMovRegMemBaseDisp(&s.Code, R8, R12, 8)  // r8 = len
	emitMovRegMemBaseDisp(&s.Code, R9, R12, 16) // r9 = cap

	// Check if len < cap, if not, need to grow
	s.Code = append(s.Code, rexByte(1, regHi(R9), 0, regHi(R8)), 0x39, modRM(3, R9, R8)) // cmp r8, r9
	jlNoGrow := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl rel32 (skip grow)

	// Need to grow: new_cap = cap * 2, if cap == 0 then new_cap = 8
	emitMovRegReg(&s.Code, R10, R9) // r10 = old cap
	// test r9, r9
	s.Code = append(s.Code, rexByte(1, regHi(R9), 0, regHi(R9)), 0x85, modRM(3, R9, R9))
	jnzDouble := len(s.Code)
	s.Code = append(s.Code, 0x75, 0) // jnz rel8
	// cap == 0, set to 8
	emitMovRegImm32(&s.Code, R9, 8)
	jmpAfterDouble := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp rel8
	// double cap
	s.Code[jnzDouble+1] = byte(len(s.Code) - jnzDouble - 2)
	// shl r9, 1
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R9)), 0xD1, modRM(3, 4, R9))
	s.Code[jmpAfterDouble+1] = byte(len(s.Code) - jmpAfterDouble - 2)

	// r9 = new cap, allocate new_cap * 8 bytes (assuming 8-byte elements)
	emitPushReg(&s.Code, R8)  // save len
	emitPushReg(&s.Code, R9)  // save new cap
	emitPushReg(&s.Code, R10) // save old cap
	emitMovRegReg(&s.Code, RDI, R9)
	// shl rdi, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDI), 0x03)
	callSiteAlloc := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteAlloc, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R10)
	emitPopReg(&s.Code, R9)
	emitPopReg(&s.Code, R8)
	emitMovRegReg(&s.Code, R11, RAX) // r11 = new data ptr

	// Copy old data if old_cap > 0
	// test r10, r10
	s.Code = append(s.Code, rexByte(1, regHi(R10), 0, regHi(R10)), 0x85, modRM(3, R10, R10))
	jzSkipCopy := len(s.Code)
	s.Code = append(s.Code, 0x74, 0) // jz rel8 (skip copy if old_cap == 0)

	// memcpy: copy old_cap * 8 bytes from old_ptr to new_ptr
	emitMovRegMemBaseDisp(&s.Code, RSI, R12, 0) // rsi = old data ptr
	emitMovRegReg(&s.Code, RDI, R11)            // rdi = new data ptr
	emitMovRegReg(&s.Code, RCX, R10)            // rcx = old_cap
	// shl rcx, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RCX), 0x03)
	// rep movsb
	s.Code = append(s.Code, 0xF3, 0xA4)

	s.Code[jzSkipCopy+1] = byte(len(s.Code) - jzSkipCopy - 2)

	// Update vec header with new ptr and cap
	emitMovMemReg(&s.Code, R12, R11)            // vec.ptr = new data ptr
	emitMovMemBaseDispReg(&s.Code, R12, 16, R9) // vec.cap = new cap

	noGrowPos := len(s.Code)
	patchRel32(&s.Code, jlNoGrow+2, noGrowPos)

	// Now we can push: data[len] = value
	emitMovRegMemBaseDisp(&s.Code, R8, R12, 8)  // r8 = len (reload in case we grew)
	emitMovRegMemBaseDisp(&s.Code, R11, R12, 0) // r11 = data ptr

	// data + len*8
	emitMovRegReg(&s.Code, RDX, R8)
	// shl rdx, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDX), 0x03)
	emitAddRegReg(&s.Code, RDX, R11) // rdx = &data[len]
	emitMovMemReg(&s.Code, RDX, R13) // data[len] = value (R13 holds value)

	// len++ (inc r8)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R8)), 0xFF, modRM(3, 0, R8))
	emitMovMemBaseDispReg(&s.Code, R12, 8, R8) // vec.len = len + 1

	// Restore callee-saved registers
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	return nil
}

// ============================================================
//  Reference Safety
// ============================================================

// isRefExpression checks if an expression is a reference variable that needs extra dereference
func (s *EmitState) isRefExpression(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.Identifier:
		isRef := s.RefVariables[e.Value]
		return isRef
	case *ast.MutableIdentifier:
		return s.RefVariables[e.Value]
	case *ast.FieldAccessExpression:
		objType := s.getExpressionStructType(e.Object)
		if objType == "" {
			return false
		}
		if sd := s.findStructDecl(objType); sd != nil {
			for _, f := range sd.Fields {
				if f.Name.Value == e.Field.Value {
					return false
				}
			}
		}
	}
	return false
}

// emitSafeRefDeref dereferences RAX only when it looks like a real pointer.
// This avoids crashing on tagged immediates that may be misclassified as refs.
func (s *EmitState) emitSafeRefDeref() {
	emitTestRegReg(&s.Code, RAX, RAX)
	jzSkip := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz skip

	// Reject non-canonical low-user pointers (e.g. tagged immediates with high bits set).
	emitMovRegReg(&s.Code, RCX, RAX)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RCX)), 0xC1, modRM(3, 5, RCX), 48) // shr rcx, 48
	emitTestRegReg(&s.Code, RCX, RCX)
	jnzSkipHigh := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz skip

	// test rax, 7  (8-byte alignment check)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xA9, 0x07, 0x00, 0x00, 0x00)
	jnzSkip := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz skip

	// Reject very low canonical values that are unlikely to be mapped pointers.
	emitCmpRegImm32(&s.Code, RAX, 0x00400000)
	jbSkipLow := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x82, 0, 0, 0, 0) // jb skip

	emitMovRegMem(&s.Code, RAX, RAX)
	skipPos := len(s.Code)
	patchRel32(&s.Code, jzSkip+2, skipPos)
	patchRel32(&s.Code, jnzSkipHigh+2, skipPos)
	patchRel32(&s.Code, jnzSkip+2, skipPos)
	patchRel32(&s.Code, jbSkipLow+2, skipPos)
}

// emitSafeLoadRaxFromRaxDisp loads [RAX+disp] into RAX when base looks like a pointer.
// On invalid/non-pointer bases, it yields 0 instead of crashing.
func (s *EmitState) emitSafeLoadRaxFromRaxDisp(disp int) {
	emitTestRegReg(&s.Code, RAX, RAX)
	jzZero := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz zero

	// Reject non-canonical low-user pointers (e.g. tagged immediates with high bits set).
	emitMovRegReg(&s.Code, RCX, RAX)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RCX)), 0xC1, modRM(3, 5, RCX), 48) // shr rcx, 48
	emitTestRegReg(&s.Code, RCX, RCX)
	jnzZeroHigh := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz zero

	// test rax, 7  (8-byte alignment check)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xA9, 0x07, 0x00, 0x00, 0x00)
	jnzZero := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz zero

	// Reject very low canonical values that are unlikely to be mapped pointers.
	emitCmpRegImm32(&s.Code, RAX, 0x00400000)
	jbZeroLow := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x82, 0, 0, 0, 0) // jb zero

	if disp == 0 {
		emitMovRegMem(&s.Code, RAX, RAX)
	} else {
		emitMovRegMemBaseDisp(&s.Code, RAX, RAX, disp)
	}
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp done

	zeroPos := len(s.Code)
	emitMovRegImm32(&s.Code, RAX, 0)
	donePos := len(s.Code)

	patchRel32(&s.Code, jzZero+2, zeroPos)
	patchRel32(&s.Code, jnzZeroHigh+2, zeroPos)
	patchRel32(&s.Code, jnzZero+2, zeroPos)
	patchRel32(&s.Code, jbZeroLow+2, zeroPos)
	patchRel32(&s.Code, jmpDone+1, donePos)
}

// emitLen implements obj.len() for Vec or string
func (s *EmitState) emitLen(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If obj is a reference (&Vec or &string), dereference first
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	// For both Vec and string, len is at offset 8
	s.emitSafeLoadRaxFromRaxDisp(8)
	return nil
}

// emitVecPop implements vec.pop() -> Result<T, string>
func (s *EmitState) emitVecPop(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If obj is a reference (&mut Vec), dereference once to get the Vec header pointer.
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}

	// Preserve vec header pointer.
	emitMovRegReg(&s.Code, R11, RAX)
	// len = vec.len (safe load: invalid pointers become 0)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	// If len == 0 => Err("vec is empty")
	emitTestRegReg(&s.Code, R10, R10)
	jnzHasItems := emitJnzRel32(&s.Code, 0)
	if err := s.emitResultErrStr("vec is empty"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	hasItemsPos := len(s.Code)
	patchRel32(&s.Code, jnzHasItems, hasItemsPos)

	// new_len = len - 1; vec.len = new_len
	emitMovRegImm32(&s.Code, RCX, 1)
	emitSubRegReg(&s.Code, R10, RCX)
	emitMovMemBaseDispReg(&s.Code, R11, 8, R10)

	// data = vec.ptr (safe load)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0)
	emitMovRegReg(&s.Code, R12, RAX)

	// elem = data[new_len]
	emitMovRegReg(&s.Code, R13, R10)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R13)), 0xC1, modRM(3, 4, R13), 0x03) // shl r13, 3
	emitAddRegReg(&s.Code, R13, R12)
	emitMovRegMemBaseDisp(&s.Code, RAX, R13, 0)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitVecFirst implements vec.first() -> Result<T, string>
func (s *EmitState) emitVecFirst(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}

	emitMovRegReg(&s.Code, R11, RAX)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	emitTestRegReg(&s.Code, R10, R10)
	jnzHasItems := emitJnzRel32(&s.Code, 0)
	if err := s.emitResultErrStr("vec is empty"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	hasItemsPos := len(s.Code)
	patchRel32(&s.Code, jnzHasItems, hasItemsPos)

	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0)
	emitMovRegReg(&s.Code, R12, RAX)
	emitMovRegMemBaseDisp(&s.Code, RAX, R12, 0)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitVecLast implements vec.last() -> Result<T, string>
func (s *EmitState) emitVecLast(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}

	emitMovRegReg(&s.Code, R11, RAX)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	emitTestRegReg(&s.Code, R10, R10)
	jnzHasItems := emitJnzRel32(&s.Code, 0)
	if err := s.emitResultErrStr("vec is empty"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	hasItemsPos := len(s.Code)
	patchRel32(&s.Code, jnzHasItems, hasItemsPos)

	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0)
	emitMovRegReg(&s.Code, R12, RAX)
	emitMovRegReg(&s.Code, R13, R10)
	emitSubRegImm32(&s.Code, R13, 1)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R13)), 0xC1, modRM(3, 4, R13), 0x03) // shl r13, 3
	emitAddRegReg(&s.Code, R13, R12)
	emitMovRegMemBaseDisp(&s.Code, RAX, R13, 0)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitVecGet implements vec.get(index) -> Result<T, string>
func (s *EmitState) emitVecGet(obj ast.Expression, index ast.Expression) error {
	if err := s.emitExpression(index); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save index

	if err := s.emitExpression(obj); err != nil {
		return err
	}
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R11, RAX)
	emitPopReg(&s.Code, R13) // index

	// len = vec.len (safe load: invalid pointers become 0)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	// if index < 0 => Err
	emitCmpRegImm32(&s.Code, R13, 0)
	jlErrNeg := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl err

	// if index < len => inBounds, else Err
	emitCmpRegReg(&s.Code, R13, R10)
	jlInBounds := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl inBounds

	errPos := len(s.Code)
	if err := s.emitResultErrStr("index out of bounds"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	inBoundsPos := len(s.Code)
	patchRel32(&s.Code, jlErrNeg+2, errPos)
	patchRel32(&s.Code, jlInBounds+2, inBoundsPos)

	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0)
	emitMovRegReg(&s.Code, R12, RAX)
	emitMovRegReg(&s.Code, R14, R13)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R14)), 0xC1, modRM(3, 4, R14), 0x03) // shl r14, 3
	emitAddRegReg(&s.Code, R14, R12)
	emitMovRegMemBaseDisp(&s.Code, RAX, R14, 0)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitVecRemove implements vec.remove(index) -> Result<T, string>
func (s *EmitState) emitVecRemove(obj ast.Expression, index ast.Expression) error {
	if err := s.emitExpression(index); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save index

	if err := s.emitExpression(obj); err != nil {
		return err
	}
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R11, RAX)
	emitPopReg(&s.Code, R13) // index

	// len = vec.len (safe load: invalid pointers become 0)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	// Bounds check: index >= 0 && index < len
	emitCmpRegImm32(&s.Code, R13, 0)
	jlErrNeg := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl err
	emitCmpRegReg(&s.Code, R13, R10)
	jlInBounds := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl inBounds

	errPos := len(s.Code)
	if err := s.emitResultErrStr("index out of bounds"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	inBoundsPos := len(s.Code)
	patchRel32(&s.Code, jlErrNeg+2, errPos)
	patchRel32(&s.Code, jlInBounds+2, inBoundsPos)

	// data = vec.ptr (safe load)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0)
	emitMovRegReg(&s.Code, R12, RAX)

	// removed = data[index]
	emitMovRegReg(&s.Code, R14, R13)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R14)), 0xC1, modRM(3, 4, R14), 0x03) // shl r14, 3
	emitAddRegReg(&s.Code, R14, R12)
	emitMovRegMemBaseDisp(&s.Code, R9, R14, 0) // removed value

	// Shift elements left: for i=index; i < len-1; i++ { data[i] = data[i+1] }
	emitMovRegReg(&s.Code, R8, R13)  // i = index
	emitMovRegReg(&s.Code, RDX, R10) // len
	emitSubRegImm32(&s.Code, RDX, 1) // len-1
	loopStart := len(s.Code)
	emitCmpRegReg(&s.Code, R8, RDX)
	jgeDone := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge done

	// nextVal = data[i+1]
	emitMovRegReg(&s.Code, RCX, R8)
	emitAddRegImm32(&s.Code, RCX, 1)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RCX)), 0xC1, modRM(3, 4, RCX), 0x03) // shl rcx, 3
	emitAddRegReg(&s.Code, RCX, R12)
	emitMovRegMemBaseDisp(&s.Code, RAX, RCX, 0)

	// data[i] = nextVal
	emitMovRegReg(&s.Code, R14, R8)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R14)), 0xC1, modRM(3, 4, R14), 0x03) // shl r14, 3
	emitAddRegReg(&s.Code, R14, R12)
	emitMovMemBaseDispReg(&s.Code, R14, 0, RAX)

	emitAddRegImm32(&s.Code, R8, 1)
	jmpLoop := emitJmpRel32(&s.Code, 0)

	loopDonePos := len(s.Code)
	patchRel32(&s.Code, jgeDone+2, loopDonePos)
	patchRel32(&s.Code, jmpLoop, loopStart)

	// len--
	emitSubRegImm32(&s.Code, R10, 1)
	emitMovMemBaseDispReg(&s.Code, R11, 8, R10)

	// return Ok(removed)
	emitMovRegReg(&s.Code, RAX, R9)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitStringGet implements string.get(index) -> Result<char, string>
func (s *EmitState) emitStringGet(obj ast.Expression, index ast.Expression) error {
	if err := s.emitExpression(index); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save index

	if err := s.emitExpression(obj); err != nil {
		return err
	}
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R11, RAX) // string header
	emitPopReg(&s.Code, R13)         // index

	// len = string.len (safe load: invalid pointers become 0)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	emitCmpRegImm32(&s.Code, R13, 0)
	jlErrNeg := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl err

	emitCmpRegReg(&s.Code, R13, R10)
	jlInBounds := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl inBounds

	errPos := len(s.Code)
	if err := s.emitResultErrStr("index out of bounds"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	inBoundsPos := len(s.Code)
	patchRel32(&s.Code, jlErrNeg+2, errPos)
	patchRel32(&s.Code, jlInBounds+2, inBoundsPos)

	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0) // string data ptr
	emitMovRegReg(&s.Code, R12, RAX)
	emitMovzxRaxMem8BaseIdx(&s.Code, R12, R13)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitIsEmpty implements obj.isEmpty()
func (s *EmitState) emitIsEmpty(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If obj is a reference (&Vec or &string), dereference first.
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	// len is at offset 8
	s.emitSafeLoadRaxFromRaxDisp(8)
	// test rax, rax
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x85, modRM(3, RAX, RAX))
	// sete al
	s.Code = append(s.Code, 0x0F, 0x94, 0xC0)
	// movzx rax, al
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)
	return nil
}

// emitStringContains implements s.contains(other)
// Returns 1 if string contains other, 0 otherwise
// ============================================================
//  Result / Option Helpers
// ============================================================

// emitNone emits code for Option::None (tag = 0)

// emitEmptyString allocates and returns a pointer to an empty string header
// String layout: [ptr: 8 bytes][len: 8 bytes]
func (s *EmitState) emitEmptyString() error {
	// Allocate string header (16 bytes)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	// Store ptr = 0, len = 0
	emitXorRegReg(&s.Code, RCX, RCX)
	emitMovMemReg(&s.Code, RAX, RCX)            // ptr = 0
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX) // len = 0
	return nil
}

// emitResultErr creates a Result::Err(msg) struct where msg is a string.
// Result layout: [tag: 8 bytes][value: 8 bytes]
// Err has tag = 1.
func (s *EmitState) emitResultErrStr(msg string) error {
	// First create the error message string
	msgIdx := s.addStringLiteral(msg)

	// Allocate Result struct (16 bytes: tag + value)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	emitPushReg(&s.Code, RAX) // save result ptr

	// Load error string address
	s.emitDataAddr(msgIdx)
	emitMovRegReg(&s.Code, RCX, RAX) // RCX = error string ptr

	// Pop result struct ptr
	emitPopReg(&s.Code, RAX)

	// Store tag = 1 (Err)
	emitMovRegImm32(&s.Code, RDX, 1)
	emitMovMemReg(&s.Code, RAX, RDX)
	// Store error string at offset 8
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)

	return nil
}

// emitResultOkFromRax creates a Result::Ok(value) from the value currently in RAX.
// Assumes the value to wrap is already in RAX
// Result layout: [tag: 8 bytes][value: 8 bytes]
// Ok has tag = 0
func (s *EmitState) emitResultOkFromRax() error {
	emitPushReg(&s.Code, RAX) // save value

	// Allocate Result struct (16 bytes: tag + value)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	emitMovRegReg(&s.Code, RCX, RAX) // RCX = result ptr

	// Pop value
	emitPopReg(&s.Code, RDX)

	// Store tag = 0 (Ok)
	emitMovRegImm32(&s.Code, RAX, 0)
	emitMovMemReg(&s.Code, RCX, RAX)
	// Store value at offset 8
	emitMovMemBaseDispReg(&s.Code, RCX, 8, RDX)

	// Return result ptr in RAX
	emitMovRegReg(&s.Code, RAX, RCX)
	return nil
}

// emitResultOkVoid returns Result<void, string> with Ok tag and void value.
func (s *EmitState) emitResultOkVoid() error {
	emitMovRegImm32(&s.Code, RDI, 16)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX) // tag = 0 (Ok)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX) // value = 0 (void)
	return nil
}

// emitBuiltinReadFile reads a file and returns Result<string, string>
// Uses open, fstat, read, close syscalls
func (s *EmitState) emitBuiltinReadFile(pathExpr ast.Expression) error {
	// Evaluate path argument (string header ptr)
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}

	// Call runtime function to read file
	// __rt_read_file(path_str_ptr) -> Result<string, string> ptr
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_read_file"})

	return nil
}

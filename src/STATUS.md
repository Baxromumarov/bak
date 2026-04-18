Bak Self-Hosted Compiler Status (src/)

Overview
- The bak compiler is largely implemented in Bak under src/compiler/.
- The current self-hosted toolchain (bakc) parses, type-checks, and compiles to a custom bytecode format.
- Execution is still handled by the existing Go VM; native machine-code generation is not implemented in Bak yet.

What is implemented (Bak)

1) CLI and driver (src/compiler/cmd, src/compiler/driver)
- bakc CLI entrypoint: src/compiler/cmd/bakc/main.bak
- Driver handles subcommands: run/build/check/test, flags, output paths.
- Module graph builder resolves imports, detects cycles, combines package files.
- Test-mode runner injection for test functions in the entry module.
- Emits bundled bytecode JSON packages for execution in the Go VM.

2) Frontend: tokens, lexer, parser, AST
- Token definitions: src/compiler/token/token.bak
- Lexer: src/compiler/lexer/lexer.bak
- Parser: src/compiler/parser/parser.bak
- AST definitions: src/compiler/ast/ast.bak
- Language coverage in AST includes:
  - statements: package/import, var/const/blocks, return, if/while/for, switch, break/continue,
    assignment, defer, panic, unsafe, function/struct/enum/impl/type/alias declarations.
  - expressions: literals, identifiers, prefix/infix, calls, method calls, field access, indexing,
    tuples, ranges, borrow expressions, vec literals, struct literals.
  - types: simple, generic, borrow, box, box-optional, tuple, function, void.

3) Diagnostics
- Diagnostic types and formatting: src/compiler/diagnostics/diagnostics.bak
- Used across parser/typechecker/codegen with colored output support.

4) Type checker
- Core type checking implemented in src/compiler/typecheck/.
- Environment and symbol tracking: environment.bak
- Type info and helpers: types.bak
- Typechecker supports structs, enums, functions, generics (Vec/Option/Result), and borrowing rules.
- Builtin function signatures are registered in the typechecker.

5) Bytecode backend (compiler -> bytecode)
- Bytecode opcodes: src/compiler/bytecode/opcode.bak
- Bytecode module/value definitions: src/compiler/bytecode/module.bak, value.bak
- JSON emitter for bytecode modules/packages: src/compiler/bytecode/emit.bak
- Code generator: src/compiler/codegen/compiler.bak
  - Emits bytecode instructions for most language constructs.
  - Handles functions, structs/enums, methods, control flow, and builtins.

6) Interpreter (tree-walking)
- src/compiler/evaluator/evaluator.bak implements an AST interpreter.
- Runtime object model in src/compiler/object/ (Object + Environment).
- Useful for bootstrap/testing, but not the primary execution path for production.

7) IR scaffold
- src/compiler/ir/ir.bak defines an intermediate representation (types, ops, blocks).
- Currently a definition-only scaffold (not wired into compilation pipeline).

8) Standard library (Bak)
- src/std contains standard library modules used by the compiler and tests.
- Compiler uses std/os, std/fs, std/strings, std/strconv, etc.

What is NOT implemented / missing

1) Native machine-code generation (Bak)
- src/compiler/native/ is currently empty.
- There is no Bak-native ELF emitter or assembler implementation.
- No native runtime ABI, syscalls, or linker step in Bak.

2) IR usage and optimization
- The IR in src/compiler/ir/ is not integrated with codegen.
- No optimization pipeline in Bak.

3) Native runtime support
- No native memory allocator, string/vec runtime, or syscall bridge in Bak.
- Any native backend will need a minimal runtime for IO and memory.

4) Self-hosted native executable
- bakc currently emits bytecode JSON for the Go VM.
- A full self-hosted native compiler (Bak -> ELF) does not exist yet.

How to run the self-hosted compiler (bakc)
- From repo root:
  ./bak src/compiler/cmd/bakc/main.bak
- Typical commands:
  ./bak src/compiler/cmd/bakc/main.bak build path/to/file.bak
  ./bak src/compiler/cmd/bakc/main.bak run path/to/file.bak
  ./bak src/compiler/cmd/bakc/main.bak test path/to/tests

Current execution pipeline (as of this file)
- Bak source -> Lexer -> Parser -> Typecheck -> Bytecode Codegen -> JSON package
- Execution is performed by the Go VM (not by Bak-native runtime).

Next milestones (native backend focus)
- Design: ABI conventions, object layout, calling convention, ELF writer in Bak.
- Implement native code emitter in src/compiler/native/.
- Introduce minimal runtime for exit/write/alloc.
- Wire native backend into driver.bak as a build mode.
- Gradually port std features needed by native compiler.

File ownership and pointers
- src/compiler/README.md: current bootstrap notes and how to run bakc.
- src/compiler/native/: reserved for native backend implementation (empty as of now).

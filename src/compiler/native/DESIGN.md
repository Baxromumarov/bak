Native Backend Phase 0 Design (Bak compiler)

Scope and goals
- Target: Linux x86_64, SysV ABI, ELF64 output.
- Compiler: written in Bak (src/compiler/*), native backend in src/compiler/native/.
- Pipeline: AST -> native machine code (no Go backend).
- Minimal runtime for syscalls, panic, and allocation.

Non-goals (Phase 0)
- Cross-platform support.
- Optimizations beyond correctness.
- Full stdlib support.

Architecture decisions
1) ABI / calling convention (SysV x86_64)
- Integer/pointer args in: rdi, rsi, rdx, rcx, r8, r9.
- Return value in rax.
- Caller-saved: rax, rcx, rdx, rsi, rdi, r8-r11.
- Callee-saved: rbx, rbp, r12-r15.
- Stack alignment: 16-byte aligned at call sites.

2) Code generation model
- Direct AST lowering to machine code (Phase 1-3).
- Optional later: AST -> IR (src/compiler/ir) -> native.

3) Runtime data layout (initial)
- bool: 1 byte (zero/non-zero), aligned to 1.
- int: 8 bytes (signed 64-bit), aligned to 8.
- float64: 8 bytes, aligned to 8.
- char: 4 bytes (Unicode codepoint), aligned to 4.
- string: struct { ptr: *u8, len: int } (16 bytes).
- Vec<T, _>: struct { ptr: *T, len: int, cap: int } (24 bytes).
- Box<T>: ptr to heap-allocated T.
- Option<T>: tagged union, layout decided by "size + tag" for simplicity:
  { tag: int, payload: T } where tag=0 None, tag=1 Some.
- Result<T,E>: { tag: int, ok: T, err: E } (tag=0 Err, tag=1 Ok).
- Structs: field order as declared; align fields to their natural alignment; struct alignment = max field alignment.
- Enums (non-Option/Result): tag + payload union; tag is int.

4) Minimal runtime ABI (native)
- exit(code:int) -> no return (syscall 60).
- write(fd:int, buf:*u8, len:int) -> int (syscall 1).
- panic(msg:string) -> prints and exits non-zero.
- bump_alloc(size:int) -> *u8 (simple heap allocator).

5) ELF layout (Phase 1)
- Single PT_LOAD segment, RX for .text; RW for .data/.bss later.
- Entry: _start that calls main and exit.

File layout (planned)
- src/compiler/native/
  - DESIGN.md (this file)
  - elf.bak        : ELF writer utilities (headers, segments).
  - x86_64.bak      : instruction encoder helpers.
  - backend.bak     : AST -> machine code emitter.
  - runtime.bak     : minimal runtime syscall stubs.
  - layout.bak      : type sizes/alignments, struct layout.
  - symbols.bak     : symbol table and relocation model (if needed).

Phase 0 deliverables
- This design file.
- Agreed ABI, data layout, and file structure.

Test plan (Phase 1+)
- T0: empty main -> exit 0.
- T1: return int from main -> exit code matches.
- T2: println("hello") -> writes to stdout.
- T3: arithmetic + locals -> correct output.
- T4: if/while -> correct output.
- T5: struct create/access -> correct output.
- T6: Vec push/get -> correct output.

Open questions
- Should char be 1 byte (ASCII) or 4 bytes (Unicode)?
- Should int be fixed 64-bit or target-sized? (assume 64-bit for now)
- Should Option/Result use niche optimization later?
- Will we emit ELF sections or only PT_LOAD segments?

Status
- Phase 0: started and documented.

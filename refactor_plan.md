# Compiler + Typechecker Refactor Plan

## Goal
Improve structure, readability, and maintainability without changing behavior.

## Strategy
Execute as small, behavior-preserving PRs. Prefer move/extract-first changes, then targeted logic cleanups.

## Task Breakdown

### 1) Split compiler expression handlers out of `compiler.go`
- Create:
  - `pkg/compiler/compile_expr_infix.go`
  - `pkg/compiler/compile_expr_call.go`
  - `pkg/compiler/compile_expr_method.go`
  - `pkg/compiler/compile_expr_field.go`
  - `pkg/compiler/compile_expr_prefix.go`
- Keep `compileExpression` dispatch centralized.
- First step is pure extraction of existing function bodies.

### 2) Move builtin registry into a dedicated file
- Create `pkg/compiler/builtins_registry.go`.
- Move:
  - `BuiltinID` type
  - builtin constants
  - `builtinNames` map
  - `BuiltinNames()` accessor
- Keep API/behavior unchanged.

### 3) Extract top-level compilation pipeline helpers
- Create `pkg/compiler/compile_toplevel.go`.
- Move:
  - `registerTopLevelDeclarations`
  - `registerFunctionStubs`
  - `compileTopLevelStatements`
  - `walkTopLevelStatements`
  - `findMainFunction`
- Keep `Compile()` focused on pipeline orchestration.

### 4) Normalize `TypeEnv` lookup semantics for non-capturing scopes
- Update `LookupEnum` in `pkg/typechecker/env.go` to match behavior style of:
  - `LookupSymbol`
  - `LookupFunction`
  - `LookupStruct`
  - `LookupAlias`
  - `LookupTypeDef`
- Add tests covering capture boundary consistency.

### 5) Extract builtin/core type bootstrap from `NewTypeEnv`
- Create `pkg/typechecker/env_builtins.go`.
- Add helper: `registerBuiltinTypes(env *TypeEnv)`.
- Move current builtin/mock enum registration there.
- Keep `NewTypeEnv()` initialization flow explicit and minimal.

### 6) Unify enum variant resolution paths
- Refactor `pkg/typechecker/resolve.go`:
  - Consolidate duplicated logic in:
    - `resolveEnumVariantFromFieldAccess`
    - `resolveTwoPartEnumVariant`
    - `resolveThreePartEnumVariant`
- Implement one internal strategy-based resolver.
- Preserve current diagnostics and resolution behavior.

### 7) Extract declaration registration/validation helpers in collect pass
- Refactor `pkg/typechecker/collect.go`.
- Extract helpers:
  - `registerStructDecl`
  - `registerEnumDecl`
  - `registerFunctionDecl`
  - `registerImplMethods`
  - `validateDeclTypeUsage`
- Keep two-pass architecture intact.

## Recommended Execution Order
1. Compiler file splits (`compile_expr_*`).
2. Builtin registry extraction.
3. Top-level compiler pass extraction.
4. `TypeEnv` enum lookup consistency + tests.
5. `NewTypeEnv` builtin bootstrap extraction.
6. `resolve.go` enum resolver unification.
7. `collect.go` helper extraction.

## Guardrails
1. Run tests after each task.
2. Keep PRs small and scoped.
3. Separate pure-move PRs from behavior-changing PRs.
4. Add/extend snapshot tests for method calls and enum constructors before touching method-call resolution logic.

## Definition of Done
- No behavior regressions in tests.
- `pkg/compiler/compiler.go` and `pkg/typechecker/env.go` are materially smaller and easier to scan.
- Resolution and scope rules are consistent and test-covered.

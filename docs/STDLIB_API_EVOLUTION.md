# Evolving Stdlib APIs

Bak stdlib APIs are part of the language contract. Treat public names as stable once tests or `test_project` examples depend on them.

## Naming

- Public functions, methods, fields, constants, variables, and parameters use `camelCase`.
- Public structs, types, aliases, enums, and enum variants use `UpperCamelCase`.
- Runtime or ABI hooks that intentionally start with `__` are not user-facing APIs and are exempt.
- Do not add snake_case public aliases for compatibility unless a release plan explicitly requires them.

## Change Checklist

Before changing public stdlib APIs:

1. Update the stdlib source and all call sites in `src/std`, `tests`, and `test_project`.
2. Update `test_project/camelcase_stdlib` when the changed API is broad or commonly used.
3. Update docs that mention the old name.
4. Run `make api-style-check`.
5. Run `make stability-fast`.

For risky runtime APIs, also run `make test-runtime`.

## Review Rule

Prefer removing drift over adding wrappers. A compatibility wrapper is only useful when users already rely on the old name and there is a documented migration path.

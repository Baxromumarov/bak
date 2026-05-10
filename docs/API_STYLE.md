# Bak API Style

Bak's stable public API style is camelCase.

- Types, enum variants, and exported data constructors use UpperCamelCase.
- Functions, methods, fields, variables, and parameters use lowerCamelCase.
- Acronyms follow normal camelCase readability: `httpServer`, `dbRow`, `parseUrl`.
- Public APIs must not use snake_case.
- Runtime/ABI hooks that intentionally start with `__` are exempt because they are not user-facing APIs.

Run `make api-style-check` before changing public stdlib or example APIs.
For stdlib API changes, follow `docs/STDLIB_API_EVOLUTION.md`.

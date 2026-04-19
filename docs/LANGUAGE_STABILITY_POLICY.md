# Language Stability Policy

Last updated: 2026-04-19

Bak is now operating under a language freeze for the `v0.1` line.

The goal of the freeze is simple:

- stop widening the language surface casually,
- harden the existing compiler/runtime/tooling behavior,
- give contributors and users one stable contract to target.

## Canonical documents

The language contract is defined by:

- `docs/CORE_LANGUAGE_SPEC.md`
- this policy file

Document roles:

- `docs/CORE_LANGUAGE_SPEC.md`: normative compatibility contract
- `README.md`: overview for users and contributors
- design-note docs: useful, but non-normative unless promoted into the core spec

If any document conflicts with the core spec, the core spec wins.

## Stability tiers

### Stable

A feature is stable only when it is explicitly included in `docs/CORE_LANGUAGE_SPEC.md`.

Stable means:

- user code may depend on it,
- incompatible changes require a version bump,
- examples, tests, and tooling should treat it as supported surface.

### Experimental

A feature is experimental when it exists in code or docs but is not listed in the core spec.

Experimental means:

- it may change or be removed without a language version bump,
- parser support alone does not make it stable,
- examples using it should say so clearly.

### Internal

Anything used only for compiler implementation, bootstrap work, native runtime plumbing, or research-track docs is internal and outside the language contract.

## Freeze rules

During the `v0.1` freeze:

- no new syntax should land unless it closes a release-blocking product gap,
- no incompatible change to stable syntax or semantics should land without a versioned migration plan,
- implementation work should prefer correctness, diagnostics, determinism, native/VM parity, package safety, and tooling polish.

Accepted parser support is not a release decision. A feature is part of the language only when the spec says it is.

## Change admission rules

A proposed language change must answer all of the following before it lands as stable:

1. What real user problem does it solve?
2. Why can that problem not be solved with the current stable surface?
3. What exact syntax and semantics are being promised?
4. What diagnostics, formatter behavior, linter behavior, and LSP behavior are required?
5. What tests, docs, and examples will lock the behavior down?

If those answers are not ready, the change should stay experimental or not land yet.

## Promotion checklist

Before an experimental feature becomes stable, the repo should have all of the following:

- a spec update in `docs/CORE_LANGUAGE_SPEC.md`,
- parser/typechecker/compiler/runtime tests as applicable,
- formatter/linter/LSP handling if the feature affects editing workflows,
- at least one user-facing example,
- migration notes if the feature changes existing behavior,
- an explicit statement that the feature is now part of the stable contract.

## What the freeze does not block

The freeze does not block:

- better diagnostics,
- faster or more deterministic compilation,
- native backend bug fixes,
- runtime safety improvements,
- stricter package-management integrity,
- test coverage expansion,
- doc cleanup,
- additive stdlib improvements that do not force incompatible language changes.

## Current implementation guidance

For the next phase of Bak, maintainers should prefer work in this order:

1. Compiler correctness and determinism.
2. Native/VM behavioral parity on the frozen surface.
3. Tooling quality: `bak`, `bakfmt`, `baklint`, LSP.
4. Runtime safety and package-management hardening.
5. Only then, carefully scoped new language or FFI work.

## Current status

As of 2026-04-19:

- the frozen language line is `Bak v0.1`,
- the Go implementation in `pkg/` and `cmd/` is the compiler of record,
- self-hosting work in `src/` remains valuable but is not the release gate,
- features not named in `docs/CORE_LANGUAGE_SPEC.md` should be treated as experimental or internal.

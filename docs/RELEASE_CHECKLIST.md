# Release Checklist

Last updated: 2026-04-19

This checklist is required for any Bak release that changes the stable language surface, user-visible tooling behavior, or runtime/package policy.

Use this together with:

- `docs/CORE_LANGUAGE_SPEC.md`
- `docs/LANGUAGE_STABILITY_POLICY.md`
- `docs/PRODUCTION_READINESS_TRACKER.md`
- `docs/RESULT_MIGRATION_NOTES.md` (when Result-oriented API behavior changes)

## Language and Compatibility

- [ ] The change is classified correctly: stable, experimental, or internal.
- [ ] `docs/CORE_LANGUAGE_SPEC.md` was updated if the stable language contract changed.
- [ ] Migration notes were added if any existing valid program changes meaning or stops compiling.
- [ ] If Result-oriented stdlib/runtime API behavior changed, `docs/RESULT_MIGRATION_NOTES.md` was updated.
- [ ] Experimental features remain out of the frozen contract unless explicitly promoted.
- [ ] If `language_mode` defaults or experimental opt-in behavior changed, release notes include a concrete `bak.toml` migration snippet.

## Tests

- [ ] Parser/typechecker/compiler/runtime tests were updated for the changed behavior.
- [ ] Spec-conformance tests were updated when the frozen surface changed.
- [ ] Negative tests exist for newly rejected or gated behavior.
- [ ] `go test ./...` passes.

## Tooling

- [ ] `bak` CLI behavior and help text were updated if flags or workflows changed.
- [ ] `bakfmt` behavior was reviewed and updated if syntax or formatting rules changed.
- [ ] `baklint` behavior was reviewed and updated if syntax or diagnostics changed.
- [ ] LSP behavior was reviewed and updated if syntax, diagnostics, formatting, completion, or code actions changed.

## Docs and Examples

- [ ] User-facing docs were updated.
- [ ] Examples were updated or labeled experimental when they use non-frozen features.
- [ ] Contradictory or stale docs were removed, rewritten, or demoted.

## Runtime and Package Safety

- [ ] Runtime permission implications were reviewed.
- [ ] `docs/TRUST_MODEL.md` was updated if runtime or package trust assumptions changed.
- [ ] Package-manager integrity or lockfile behavior changes were covered by tests and docs.

## Release Notes

- [ ] Release notes summarize what changed.
- [ ] Known limitations and experimental areas are called out explicitly.
- [ ] Any required user action is stated plainly.

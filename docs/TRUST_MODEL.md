# Bak Trust Model

Last updated: 2026-04-18

## Status

Bak does not currently provide a sandbox for untrusted code.

`bak`-managed execution now has a minimal permission gate for dangerous runtime capabilities.

Current behavior:

- `bak run`, `bak --vm`, `bak --bc`, `bak test`, and the REPL deny subprocess execution, network/database access, and destructive filesystem mutation by default,
- those capabilities can be enabled explicitly with `--allow-exec`, `--allow-net`, `--allow-fs-mutate`, or `--allow-all`,
- `bak.toml` can request the same runtime capabilities under a `[permissions]` table, and CLI flags still take precedence,
- `os.exec` in interpreter/VM paths is direct-exec only, uses a default timeout, and caps captured output unless overridden with CLI flags,
- native executables produced by `bak build` now carry the same project permission policy at compile time and refuse dangerous builtin usage unless the matching permission is granted.

Bak still does not provide a sandbox. If you run native output or enable dangerous capabilities, you should assume the program has the same effective trust level as any other local program you choose to execute yourself.

That means a Bak program may be able to:

- read and write files your current user can access,
- change the current working directory,
- inspect and modify environment variables,
- start subprocesses through `os.exec`,
- access network features exposed by the standard library or runtime,
- delete files or directories through filesystem APIs such as recursive removal.

## Current Boundaries

### Compiler and CLI

The Go compiler and tools in `cmd/` are trusted developer tools. They are not designed to safely execute adversarial input in a sandboxed environment.

### Runtime builtins

Several builtins expose host capabilities directly:

- `os.exec`
- filesystem mutation APIs including recursive deletion
- network/database builtins
- environment access and mutation
- process exit

These are intentional features, but they are privileged operations.

Current CLI/runtime guardrails:

- `os.exec` requires `--allow-exec`,
- `os.exec` is direct-exec only; it does not invoke a shell for pipes, redirection, or expansion,
- `os.exec` uses `--exec-timeout` and `--exec-max-output-bytes` to control runtime and captured output size,
- socket and database builtins require `--allow-net`,
- destructive filesystem operations such as `fs.remove` and `fs.removeAll` require `--allow-fs-mutate`.
- `fs.remove` and `fs.removeAll` refuse empty paths, `.` and `/` even when mutation is allowed.
- native builds refuse `os.exec`, socket/database, and destructive filesystem builtins unless the corresponding capability is granted.

### Package fetching

`bak get` and `bak install` may fetch code from git repositories.

Current hardening:

- lockfile entries now include commit and content checksum,
- cached packages are keyed by source plus commit instead of repo name alone,
- `bak install --offline` avoids network access,
- `bak install --frozen-lockfile` refuses dependency drift relative to `bak.toml`.

Current limitations:

- there is no signature verification,
- there is no repository allowlist,
- there is no sandboxed package build step,
- fetching a dependency still implies trusting the referenced source.

## Safe Usage Guidance

Until Bak has a stronger permission model, follow these rules:

1. Do not run untrusted Bak programs on machines you care about.
2. Prefer containers, VMs, or separate users for risky code.
3. Prefer `bak install --offline` in reproducible environments.
4. Commit `bak.lock` and review dependency changes before updating it.
5. Treat `--allow-exec`, `--allow-net`, and `--allow-fs-mutate` as privileged opt-ins.
6. Keep `--exec-timeout` and `--exec-max-output-bytes` conservative when running questionable code.
7. Treat native executables built by `bak build` as enforced at compile time, not by a runtime sandbox.
8. Avoid `fs.removeAll` on ambiguous paths even with permissions enabled.

## Near-Term Roadmap

The active Go-first roadmap calls for additional hardening:

1. Runtime permission model or explicit capability gating for dangerous operations.
2. Better process controls around external command execution.
3. More lockfile integrity checks and negative-path tests.
4. Extend or redesign the permission model and exec hardening story for native executables.

See:

- `GO_FIRST_ROADMAP.md`
- `BOOTSTRAP.md`

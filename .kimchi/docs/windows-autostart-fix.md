# Windows autostart fix spec

## Problem
`routatic-proxy autostart enable` writes an `HKCU\...\Run` value so the proxy starts on Windows login. Users report the registry entry is created and `autostart status` says "enabled", but after reboot the server is not running.

The current value looks like:

```text
"C:\Users\<user>\AppData\Local\Microsoft\WindowsApps\routatic-proxy.exe" "serve" "--background"
```

Two likely failure modes:

1. **AppExecLink / WindowsApps alias problem.** The recorded binary path can be an AppExecLink reparse point inside `WindowsApps`. When launched via the registry `Run` key with an absolute path, `CreateProcess` sometimes fails to resolve the alias, so the process never starts. The same alias works from a terminal because the shell resolves it through `PATH`.
2. **Crash on startup is invisible.** If the forked background process crashes (missing env var, bad config, port in use), the error goes to the log file, but `autostart status` only checks the registry entry and binary existence — it does not verify the server process is alive.

## Goals
1. Make the registry command robust against WindowsApps aliases.
2. Use conventional Windows command-line quoting (quote only the executable path and arguments that contain spaces, not every token).
3. Improve `autostart status` so it reports whether the server process is actually running.
4. Surface fork/start errors in the log file instead of losing them.
5. Add unit tests for the Windows-specific command-line building and path-extraction helpers.

## Changes

### `internal/daemon/autostart_windows.go`

1. `buildAutostartArgs(configPath string, port int) string`
   - Return a plain command-line string, not a collection of individually quoted tokens.
   - Format: `serve --background` optionally followed by ` --config "<abs path>"` and/or ` --port <n>`.
   - Quote the config path only if it contains spaces; on Windows the simplest safe form is to always quote the config path.

2. `registryBinaryRef(binaryPath string) string`
   - New helper.
   - If `binaryPath` is inside a `WindowsApps` directory (case-insensitive), return `filepath.Base(binaryPath)` (e.g. `routatic-proxy.exe`). This lets Windows resolve the alias via `PATH` at login instead of relying on a direct absolute-path launch of a reparse point.
   - Otherwise return the absolute path wrapped in double quotes.

3. `EnableAutostart`
   - Build the registry value as: `<binary-ref> <args>`.
   - Validate the chosen binary reference by checking `exec.LookPath` when the bare name is used, or `os.Stat` when an absolute path is used, and return a clear error if the binary cannot be found.

4. `AutostartStatus`
   - Keep the existing registry-entry and binary-existence checks.
   - Add a process-running check: read the PID file via `daemon.GetPID(paths.PIDFile)` and `daemon.IsProcessRunning(pid)`. If the PID file is missing or the process is not running, print a warning (the registry entry is still enabled, but the server is not currently alive).

5. `extractBinaryPath`
   - Keep the existing parser but add a unit test.

### `internal/daemon/background.go`

1. In `ForkIntoBackground`, if `cmd.Start()` fails, write the error to `paths.LogFile` before returning it. This ensures that a startup failure at login leaves evidence in the log.

### `internal/daemon/autostart_windows_test.go`

New file, `//go:build windows`. Tests:

- `TestBuildAutostartArgs` — verifies the command-line string for empty, config-only, port-only, and config+port cases. Assert no quotes around bare flags; config path is quoted.
- `TestRegistryBinaryRef` — verifies that a path under `WindowsApps` resolves to the base name, while a normal path resolves to the quoted absolute path.
- `TestExtractBinaryPath` — verifies parsing of quoted paths, unquoted paths, paths with spaces, and missing trailing quote.

## Verification

- Compile the Windows daemon package: `GOOS=windows go vet ./internal/daemon/`
- Run daemon tests with Windows build tags: `GOOS=windows go test ./internal/daemon/`
- Run the repo-wide test suite on the host: `go test ./...`
- Run lint: `make lint`

## Acceptance criteria
- `routatic-proxy autostart status` on Windows reports whether the server process is running.
- `buildAutostartArgs` produces a conventional Windows command line without over-quoting flags.
- A binary path inside `WindowsApps` is recorded as the bare executable name in the registry value.
- Fork failures in `ForkIntoBackground` are appended to `routatic-proxy.log`.
- All new and existing tests pass.

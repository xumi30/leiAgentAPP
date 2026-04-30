# Release readiness

This checklist tracks the work needed before shipping leiAgent outside a trusted development machine.

## Current status

- Go packages compile and `go test ./...` runs.
- Wails generates `frontend/wailsjs` bindings before building the React frontend.
- CI uses `wails build -clean -tags webkit2_41` as the Linux smoke build.
- Release hardening is still in progress.

## Before every release

Run:

```bash
./scripts/release-check.sh
```

The release check scans for committed-looking API keys, verifies Go formatting, runs Go tests, and builds the app through Wails so generated frontend bindings exist before Vite runs.

## Required before public distribution

- Replace all real credentials with placeholders or environment variables.
- Rotate any credentials that have ever been committed, then rewrite Git history before publishing.
- Keep user data out of the repository and out of the app bundle. Runtime folders such as `workspace/`, `localmemory/`, `profiles/`, `logs/`, and local databases should remain user data.
- Add smoke tests for the main app workflows: model configuration, chat send/stream, local memory, memo storage, scheduled tasks, MCP setup, and workspace file operations.
- Define a tool permission policy for file, shell, browser, download, and MCP tools.
- Add release signing:
  - macOS: Developer ID signing and notarization.
  - Windows: code signing for the executable and installer.
- Write privacy and data handling notes for API keys, prompts, local files, logs, and third-party model providers.
- Decide the versioning scheme and update `wails.json`, package metadata, and release notes together.

## Recommended hardening

- Move default runtime data to an OS-specific app data directory while preserving an explicit workspace location for user documents.
- Add data migrations for local SQLite and YAML state.
- Add log rotation and redaction for credentials and private prompt content.
- Add an update channel or a documented manual update flow.
- Add platform-specific install and uninstall tests.

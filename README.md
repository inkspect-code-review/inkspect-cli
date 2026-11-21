# Inkspect CLI

A native Cloud client for source context and code reviews. No engine, indexer or
language runtime is installed on the user's machine.

**Release:** v0.1.2. Native downloads and installation commands are available on
the official Inkspect website.

## Install

macOS / Linux:

```sh
curl -fsSL https://inkspect.si/cli/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://inkspect.si/cli/install.ps1 | iex
```

Windows cmd:

```bat
powershell -NoProfile -Command "irm https://inkspect.si/cli/install.ps1 | iex"
```

Installers verify SHA-256 and install into a user directory. macOS/Linux users
may need to add `~/.local/bin` to PATH. Windows adds `%LOCALAPPDATA%\Inkspect\bin`
to the user PATH; reopen the terminal. x64 and ARM64 builds are provided for each
OS. Users do not need Go. Developers need Go 1.25.5+ to build this source.

## Connect and ask

Run `inkspect login` to open Cloud in your browser, choose a workspace and approve
the matching terminal code. Use `inkspect login --no-browser` on a remote machine.
For automation, create an **Inkspect CLI** key in Cloud → API keys and use
`INKSPECT_API_KEY` or `inkspect login --token-stdin`. Never put keys in arguments
or project files. Workspace owner/admin access is required.

```sh
inkspect login
inkspect whoami
inkspect ask
inkspect ask "Payment provider extension points and tests" --format json --explain
inkspect index
inkspect index status
inkspect source <source-id> --around 40
inkspect skill install --agent codex
inkspect mcp serve
inkspect skill updates
inkspect skill update
inkspect update
```

`ask` detects the local repository or searches accessible repositories in your
current Cloud workspace. No `init` is required. Use `--repo owner/name` to narrow
the search, `--all` for workspace search, or `init` to save an automatic mapping.

See [Usage](docs/USAGE.md), [Architecture](docs/ARCHITECTURE.md),
[release checklist](docs/RELEASE.md) and the [tracked PRD](docs/PRD.md).

## Develop

```sh
go test -race ./...
go vet ./...
go build -buildvcs=false -o inkspect ./cmd/inkspect
go run scripts/build.go
```

The build script creates all six native archives and `dist/SHA256SUMS`. It does
not publish or run Git. CI runs native tests on Windows, macOS and Linux.

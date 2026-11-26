# Inkspect CLI

A native terminal client for Inkspect Cloud. Search indexed repositories for
cited source, expand a result at its recorded revision, or submit a code review.
[Native downloads for macOS, Linux and Windows](https://inkspect.si/cli)

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
OS.

## Get source context

Sign in as a workspace owner or admin. Compare the terminal code with the one
in the browser, then approve the workspace. Over SSH, use `inkspect login --no-browser`.
For automation, supply a CLI-purpose key via `INKSPECT_API_KEY` or `login --token-stdin`.

```sh
inkspect login
inkspect whoami
inkspect ask "Where are review findings validated?" --format json
```

From a connected checkout, `ask` detects the repository automatically. Use
`--repo owner/name` to choose one explicitly or `--all` to search across the
workspace. Pass a returned source ID to `inkspect source <source-id> --around 40`
to read the surrounding lines at the same revision.

To inspect the changes a local review would send, run `inkspect review local --dry-run`.
Submitting the review uploads the selected source to Cloud and consumes workspace credits.
The [command guide](docs/USAGE.md) covers reviews, indexing, MCP and agent skills.

## Develop

When handling `ask`, the [command dispatcher](internal/cli/run.go) resolves the
workspace and repository through [scope.go](internal/cli/scope.go), then sends
the question to `/api/cli/context` through the [HTTP client](internal/cli/client.go).
Local reviews collect their before/after files in [snapshot.go](internal/cli/snapshot.go).

CI uses Go 1.25.5. Run its checks from the repository root.

```sh
go test -race ./...
go vet ./...
go build -buildvcs=false -o inkspect ./cmd/inkspect
```

See [architecture](docs/ARCHITECTURE.md) for the client/Cloud boundary and the
[release checklist](docs/RELEASE.md) for building the six native archives.

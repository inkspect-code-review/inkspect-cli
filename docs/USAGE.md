# CLI usage

## Authentication

`inkspect login` opens the Cloud approval page in your default browser. Sign in,
compare the displayed code with your terminal, choose a workspace and explicitly
allow CLI access. The terminal waits for approval and saves the credential only
after successful exchange and account verification. Use `--no-browser` to print
the link/code without opening a browser (also works over SSH). Noninteractive
login must explicitly choose `--no-browser` or `--token-stdin`.

Codes expire after 10 minutes, can only be exchanged once, and can be declined.
Ctrl+C cancels waiting. Failed, declined or expired login preserves saved access.
If an exchange response is lost, start login again; revoke any unused entry in
Cloud → API keys. Browser approval creates a CLI-purpose key valid for 90 days,
subject to the same owner/admin, workspace lifecycle and active-key limits.

`inkspect whoami` shows account, workspace, role and access expiry without reading
project files or repository indexes. Use `inkspect whoami --format json` in scripts.
It validates current Cloud access rather than trusting cached account information.

`INKSPECT_API_KEY` takes precedence over saved credentials. Unset it before browser
login. For automation, `login --token-stdin` validates an existing CLI-purpose key.
Credentials use the OS user configuration directory with private directory/file
permissions (a user-only ACL on Windows). `logout` removes saved credentials;
environment keys remain active. Revoke keys in Cloud to disable them everywhere.

`--api-url` is accepted by login. `INKSPECT_API_URL` overrides the saved origin
for an intentionally selected Cloud environment. HTTPS is required, except
localhost for development. Project configuration cannot select a server origin.

`init` detects the GitHub origin and creates `.inkspect.json` with the repository
and workspace. `init --repo owner/name` pins an explicit repository. Outside an
accessible checkout, `init` verifies the workspace without writing a project file.
It does not overwrite existing configuration. `status` and `doctor` check Cloud
access and report the local origin/HEAD/dirty state. These commands do not index.

## Context

`ask` opens a terminal session; `ask "question"` is a single request; `ask -`
reads stdin. `/clear` resets interactive question history, `/exit` exits. A
nonterminal with no question errors immediately. Redirected stdout is clean;
prompts, progress and errors use stderr. `--format json` is intended for agents.

No initialization is required. Scope precedence is `--repo owner/name`, project
configuration, an accessible GitHub origin, then all accessible repositories in
the signed-in workspace. `--all` forces workspace search even inside a project;
it cannot be combined with `--repo`. Explicit project/flag failures never silently
widen the search. SSH and HTTPS GitHub origins are supported; no Git installation
is needed for general search.

General search merges per-repository rerank scores into one shared output budget.
Every result retains its repository and signed source reference. Missing indexes,
failed repositories and search-time limits are reported as omissions. It searches
only the workspace selected at login, not other workspaces or the public web.
Each indexed repository currently uses its own query embedding and rerank when
candidates exist; model usage can exceed a single-repository search. `scope` and
`--explain` show coverage. General search freshness is `unknown`.

`--repo owner/name` overrides the project repository. `--budget 12000` limits
source bytes using a three-bytes-per-token estimate (metadata is additional).
`--explain` includes actual strategy, scores and budget omissions. Retrieval is
not proof of exhaustive dependency coverage and uses Cloud credits.

In an interactive terminal, each submitted question shows a `Thinking...`
animation until the request finishes. It is cleared before results or errors.
Pipe, redirected output, JSON and MCP modes do not animate. Set
`INKSPECT_NO_PROGRESS=1` to disable progress; `TERM=dumb` also disables it.
Ctrl+C cancels the active context request.

`freshness` is `current`, `stale`, `local_changes` or `unknown`. A matching SHA in
an unrelated checkout is never enough to claim current context. `--require-current`
rejects unknown/stale/dirty or incomplete context. Read local files before edits.

`source <id> --around 40` expands a source within the same revision, capped at
401 lines. References expire after an hour and require current source access.

## Cloud indexing

```sh
inkspect index
inkspect index --repo owner/repository
inkspect index status --repo owner/repository
inkspect index --repo owner/repository --no-wait --format json
```

`index` selects the configured or automatically detected accessible repository.
Outside a connected checkout, choose one with `--repo`; it never starts indexing
the entire workspace. Cloud owns the queue, GitHub access, size checks and indexer.
No local source is uploaded and no engine is installed. Existing Cloud indexing
policy applies; indexing itself does not consume AI Credits.

By default the CLI follows the accepted operation until completion, showing stage,
file/chunk counts, embedded chunks and reused chunks in terminal stderr. Existing
queued/running work is reused; an up-to-date index is reported without another job.
`index status` reports the latest operation without starting work. `--no-wait`
returns immediately; `--timeout` bounds waiting (default 1800 seconds).

Ctrl+C or a timeout stops local waiting, not the Cloud job. The last known status
is returned and the CLI exits with an error; inspect `index status` before retrying.
The CLI never blindly retries a start request. JSON mode emits one final status
object with no animated progress. A failed job returns a nonzero exit status.

## Reviews

```sh
inkspect review local --dry-run
inkspect review local --staged --task "Add a new payment provider"
inkspect review local --base main --include-untracked --no-wait
inkspect review pr 123 --repo owner/name
inkspect review rerun <review-id> --repo owner/name
inkspect review status <review-id>
inkspect review show <review-id> --format json
```

Local review uploads selected source to Cloud and consumes credits. It never
executes project code or tests. It needs Git and an existing local commit. Default
scope is tracked working changes against HEAD; `--staged` uses index contents;
`--base` uses the merge base. `--include-untracked` opts new files in. Renames are
represented as deletion/addition. Symlinks, sensitive paths, detected credentials,
binary files and excess source are omitted and reported. Ignore policy is read
from the base; current `.inkspectignore` can additionally restrict uploads. An
invalid policy fails closed. `--dry-run` requires neither login nor network.

Limits match Cloud: 80 files, 64,000 bytes per version, 400,000 source bytes,
750,000 encoded request bytes and 200 omissions. Narrow scope if the request
cannot fit. Source discovery and syntax conversion do not execute Git hooks or
external diff programs. A review covers its submitted snapshot, not later edits.

PR review resolves the current PR on the server. Rerun retains the saved SHA/model
and fails if the open PR moved. Exact BYOK reruns are unavailable. The default
wait is 900 seconds, `--timeout` is 1–3600, `--no-wait` returns immediately. A wait
timeout does not cancel the job. Poll its ID rather than resubmitting. Matching
requests reuse backend idempotency. Findings require human/agent assessment;
successful transport does not imply a clean review. No GitHub comment is posted.

## Skill and MCP

`skill install|update|updates|list|uninstall --agent codex --scope project|global` manages
the latest compatible instruction-only skill. Project scope is default and adds
a delimited section to AGENTS.md without replacing existing instructions. Global
scope installs under `~/.agents/skills/inkspect` without editing global rules.
Edited skill files or managed instructions are preserved by refusing updates.

MCP uses newline-delimited JSON-RPC 2.0 over stdio, protocol `2025-11-25`. It
exposes context/source/status, not paid review mutations. Configure the process
as `inkspect mcp serve`; inherit `INKSPECT_API_KEY` or use saved login. Launch in
the project directory or specify repository in the tool. Initialize, send
`notifications/initialized`, then list/call tools. All stdout is protocol data.

```toml
[mcp_servers.inkspect]
command = "inkspect"
args = ["mcp", "serve"]
env_vars = ["INKSPECT_API_KEY"]
```

Exit code 0 means successful command completion (inspect review findings in the
result); 1 means a command, validation, authorization or execution error. Partial
context is returned with `incomplete: true`, unless `--require-current` is used.

## Updates

Normal terminal commands check the public release metadata at most once a day
and display notices on stderr when a newer published CLI or installed skill is
available. Checks time out after two seconds and fail silently. Pipes, JSON mode
and MCP do not perform automatic checks or emit notices. Set
`INKSPECT_NO_UPDATE_CHECK=1` to disable automatic checks entirely.

- `inkspect skill updates` refreshes skill availability and compatibility.
- `inkspect skill update` installs the latest skill supported by your CLI.
- `inkspect update` downloads the native release, verifies its SHA-256 and replaces
  the current executable. No compiler, installer script or elevation is invoked.

Updates are explicit, never automatic. Restart MCP sessions after updating the
CLI. A skill can have a newer version while supporting the same CLI; if it needs
a newer client, the command tells you to update the CLI first. Local skill edits
are never overwritten. Update metadata contains only versions and compatibility.
On Windows a locked previous executable may remain beside the new executable
with a `.previous-` suffix until it can be removed after all old processes exit.
System-managed or unwritable installs must be updated through their installation
method instead. Unpublished releases are not advertised or installed.

# Inkspect CLI — v0.1 product requirements

Status: CLI/skill 0.1.2 published with coordinated Cloud and website deployment on 2026-11-20; automatic workspace search and read-only index status verified in production. Windows-native, production index-start and production source/review verification remain open. Owner: standalone `inkspect-cli` client.

## Purpose

Give people and coding agents bounded, cited source context from Inkspect Cloud,
and let them request Cloud reviews before or after opening a pull request.
The CLI contains no review engine, embedding model, indexer or billing policy.

## Agreed scope

- Windows, macOS and Linux; native binaries for amd64 and arm64.
- Interactive `ask`, single-request and stdin/pipe modes; Markdown and JSON.
- `--explain`, token budget, explicit repository selection and freshness reporting.
- `source` expands an authorized, revision-pinned source reference.
- `review local`, `review pr`, `review rerun`, `review status`, `review show`.
- `mcp serve` exposes context and source tools over stdio, without prompts or banners.
- Downloadable Codex skill, project instruction integration and lifecycle commands.
- One-line POSIX shell, PowerShell and cmd installation; checksummed releases.
- Product page and searchable website documentation.

## User stories / completion checklist

### Follow-up — Cloud index command and terminal feedback
- [x] `index` starts one selected Cloud repository and follows its accepted operation.
- [x] Existing Cloud eligibility, current GitHub access and deduplicated queue admission reused.
- [x] `index status`, `--no-wait`, bounded waiting and cancellation without cancelling Cloud work.
- [x] `ask` shows `Thinking...` only for terminal requests and clears it on result/error/cancellation.
- [x] Pipe, JSON, MCP and redirected streams remain free of animation.
- [x] Publish the index/progress follow-up as CLI/skill 0.1.2; verify public downloads, macOS/Linux installation and CLI/skill upgrades from 0.1.1.

### US-01 — independent portable client
- [x] Go module, command entrypoint, HTTP/config/output layers; no engine dependency.
- [x] Native builds for six OS/architecture targets.
- [x] Automated test matrix for Windows, macOS and Linux configured.
- [x] Release archive/checksum generation and explicit manual release workflow.

### US-02 — authentication and project setup
- [x] Explicit CLI-purpose Cloud keys; old provider keys do not gain source access.
- [x] Browser `login`, explicit code approval, workspace selection, `--no-browser` fallback.
- [x] `whoami` shows the current account/workspace/role/expiry, including JSON output.
- [x] `login --token-stdin`, environment credentials, `logout`, `init`, `status`, `doctor`.
- [x] Device codes expire, are stored as hashes, require explicit consent and exchange only once.
- [x] Database rate limits, origin/session/membership checks and concurrent exchange regressions.
- [x] Credentials stay out of project config, logs, output and URL arguments.
- [x] Noninteractive commands fail clearly instead of waiting for input.

### US-03 — context
- [x] Automatic scope: explicit repository, saved mapping, accessible GitHub origin, then workspace search.
- [x] `init` auto-detects a project; outside a project it verifies access without creating a binding.
- [x] `--all` and MCP `all` force workspace search with shared budget, source identities and coverage.
- [x] Publish CLI/skill 0.1.1 and coordinated Cloud/website update; verify the user's no-repo query in production.
- [x] Authorized Cloud context endpoint reuses existing retrieval, metering and access checks.
- [x] Raw snippets include repository, revision, path, lines and opaque source reference.
- [x] Interactive follow-ups and single-shot/stdin JSON or Markdown output.
- [x] Bounded output, omissions, empty results and index availability are explicit.
- [x] `--explain` reports actual selection evidence, not invented model reasoning.
- [x] Local revision/dirty comparison; `--require-current` fails on stale or unknown state.
- [x] Source expansion checks expiry, signature and current repository authorization.

### US-04 — reviews
- [x] Local snapshot collection: staged, tracked working changes, base and opt-in untracked.
- [x] Sensitive paths/content, symlinks, binaries, ignore rules and size limits enforced.
- [x] Dry run shows selected paths and omissions without uploading source.
- [x] Local snapshots use the existing Cloud agent-review contract and idempotency.
- [x] PR review uses current server-resolved PR snapshot and existing admission policies.
- [x] Rerun preserves prior revision/model or fails explicitly if unavailable.
- [x] Polling, no-wait, bounded timeout and machine-readable results.

### US-05 — integrations
- [x] MCP initialize, tools/list, tools/call, JSON-RPC errors and clean stdio transport.
- [x] Context/source/status tools use the same client operations and authorization.
- [x] Versioned skill download with hash verification, project/global install, update/list/uninstall.
- [x] Preserve user-edited skills and unrelated AGENTS.md content.
- [x] Skill explains context freshness, untrusted retrieved text and review authorization.

### US-06 — installation and website
- [x] POSIX and PowerShell installers detect OS/architecture and verify artifact SHA-256.
- [x] cmd one-liner invokes the same PowerShell installer.
- [x] No administrator requirement; install in a user directory and explain PATH.
- [x] `/cli` page, CLI documentation, navigation/search/sitemap integration.
- [x] Public installers and versioned skill assets generated from CLI-owned sources.
- [x] Unpublished release status is explicit; do not advertise working release URLs before publication.

### US-07 — verification and release gates
- [x] CLI unit/protocol/HTTP/snapshot tests, vet and cross-compilation.
- [x] Cloud validation/auth/replay/source-scope regressions and typecheck/build.
- [x] Website lint/typecheck/build and documentation checks.
- [x] Cloud UI key-purpose selector and consumed API package refreshed if changed.
- [x] Native macOS and Linux runtime suites pass locally.
- [ ] Windows runtime/installer validation and hosted OS matrix pass (requires authorized CI execution).
- [x] Disposable PostgreSQL integration checks pass.
- [x] Published downloads/checksums, macOS/Linux installation, skill/update discovery and live unauthenticated login checks.
- [ ] Authenticated production end-to-end context/source/review checks.
- [x] Browser QA for login, workspace selection, decline, successful exchange and reused-code rejection.
- [ ] Broader product/website UI QA beyond the login flow.
- [x] CLI/skill 0.1.0 published and coordinated Cloud/website deployment completed with explicit authorization.

### US-08 — update discovery and installation
- [x] Startup notices for newer published CLI and installed skill versions.
- [x] Daily cached, bounded public checks; suppress checks/notices in pipes, JSON and MCP.
- [x] `inkspect skill updates` reports availability and CLI compatibility.
- [x] `inkspect update` verifies the native archive and replaces the executable.
- [x] Skill versions can advance independently with a minimum supported CLI.
- [x] User-edited skills remain protected; unpublished releases are never offered.
- [x] Update/cache/version/live-replacement tests and website documentation.

## Functional requirements

FR-1: Cloud owns retrieval, authorization, admission, prices and review execution.
FR-2: CLI stdout is data; diagnostics/progress use stderr. MCP stdout is JSON-RPC only.
FR-3: CLI defaults never publish GitHub comments or push/commit changes.
FR-4: Local means source origin, not offline inference. Selected source is uploaded to Cloud.
FR-5: Local files are never uploaded by `ask`; local freshness reads only repository metadata.
FR-6: All paid mutations have stable operation identifiers; no blind POST retries.
FR-7: API authorization is checked again before returning repository source.
FR-8: Installation and skill updates must fail closed on hash mismatch or unexpected paths.
FR-9: Empty/incomplete context is distinguished from backend errors; no fabricated citations.
FR-10: A token budget is an approximate byte-derived estimate, labeled as such, not an exact tokenizer count.

## Non-goals

Offline/local indexing, bundled engine, code edits, autofix, test execution, automatic
review loops, automatic deployment, browser QA or public package publication in this task.
Browser device approval and scoped API keys are supported; no refresh token or offline login is included.

## Design

Native Go executable with standard-library dependencies. Cloud-only API and policy
stay in private Cloud repositories. Website follows existing Inkspect typography,
ink/white palette and layout, with a terminal-led product example and platform-specific
installation commands. MCP initially supports the established 2025-11-25 stdio protocol.

## Success measures

Measure relevant-file coverage on payment-provider, auth and cross-repo tasks; compare
discovery token use with ordinary file search; record p50/p95 latency, context bytes,
omissions and stale-result rate. No performance claims before measurements.

## Delivery notes

No Git operations, manual UI QA, deploy or publication authorized. Mark implemented
source and completed automated checks separately from external release/runtime gates.

Verification evidence and remaining limits: [VERIFICATION.md](VERIFICATION.md).

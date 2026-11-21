# Verification — 2026-11-20

CLI 0.1.2 and skill 0.1.2 are published with Cloud API/UI and website updates on
2026-11-20. The engine remains at 0.0.40. The original 0.1.0 publication and the
0.1.1 automatic-scope follow-up completed on 2026-11-19. No Git operations were
performed. Login browser QA was completed previously in a disposable local
environment; this release used automated checks without manual UI QA.

## Completed checks

### 0.1.2 index/progress follow-up

- Added Cloud index start/status with existing live access, eligibility and queue
  admission; progress follows the exact accepted job, including an existing job.
  Local cancellation does not cancel Cloud indexing. No live indexing was started.
- Added terminal-only `Thinking...` animation with cleanup on completion/error/
  cancellation. Pipe, JSON and MCP output remain animation-free; console rendering
  is covered by automated stream checks and native macOS PTY checks, not manual
  OS terminal QA. The PTY checks verify animation, cleanup, Ctrl+C, and clean
  JSON/pipe output.
- Go race suite and vet passed, including polling, no-wait, status, failed jobs,
  cancellation, machine output and HTTP cancellation. Six 0.1.2 native archives
  built and published as immutable assets; Windows PE architectures matched.
- Cloud API audit/typecheck/build: 188 tests passed, 33 skipped. Cloud UI consumed
  package refreshed; audit/build and 48 tests passed. The compiled API/UI runtime
  smoke test passed with a disposable PostgreSQL schema and synthetic users.
- Test services/database were removed. The native CLI also passed an indexing
  contract check against the real Cloud router with synthetic identities and an
  existing job, including exact-ID polling through completion.
- Cloud rollout completed: API 0.0.50 and UI 0.0.65. The engine remains 0.0.40.
  Authenticated production capability and read-only index status checks passed;
  no production indexing job was started.
- Cloud cleanup removed two obsolete stopped API/UI containers; active and
  previous releases were retained. No Git operations were performed.
- Website 0.1.44 is active. Lint/typecheck/build and the skill validator passed.
  Public health, CLI page, indexing/progress documentation and the homepage's
  removal of the mandatory-init example were verified. Cleanup removed one older
  website container, image and source snapshot; active and rollback retained.
- All six public 0.1.2 archives passed download/checksum/content checks. Published
  installers match the owned source, and the versioned skill checksum matched.
  Actual macOS ARM64 and Linux AMD64 installations ran CLI 0.1.2 successfully.
  Actual CLI and skill upgrades from 0.1.1 to 0.1.2 passed against public assets;
  previous release and skill URLs remain available.
- Native Windows execution and PowerShell/cmd installation remain unverified on
  a Windows host; cross-compilation and PE checks do not establish runtime support.

### 0.1.1 automatic scope follow-up

- macOS Go race suite and vet passed, including SSH/HTTPS remote detection,
  inaccessible remote fallback, explicit/project scope precedence, forced
  workspace search, and `init` both inside and outside a repository.
- Cloud API: 183 passed, 33 skipped, no failures; audit/typecheck/build passed.
  Workspace tests cover shared output budgets, repository-bound source references,
  missing indexes, upstream errors and revocation before returning source.
- Disposable PostgreSQL provider/CLI integration passed with real workspace
  admission, per-repository admission, cross-workspace/user denial and revocation.
- Cloud UI: consumed API package regenerated; audit, 48 tests and build passed.
- Website lint/typecheck/build and the updated skill validator passed. All six
  0.1.1 archives and checksums were validated; Windows PE architectures matched.
- The built native CLI was exercised against the real Cloud router with synthetic
  engine responses: no-repo `init` and `ask`, multi-repository ordering, signed
  source expansion and explicit repository boundaries all passed. No real source,
  model invocation or credit charge was used in this contract check.
- A production workspace query succeeded without `--repo` and reported
  incomplete coverage. Production `init` without arguments also succeeded without
  creating a project file. No source or credential was written to the test log.
- Production rollout completed: UI 0.0.64, API 0.0.49, engine gateway 0.0.40,
  website 0.1.42. Readiness and live unauthenticated sign-in checks passed.
- All six public archives passed download/checksum checks; official macOS and
  Linux installers ran CLI 0.1.1. Actual CLI and skill upgrades from 0.1.0 to
  0.1.1 passed against published assets. The old release and skill remain available.
- The homepage's "Context first. Then the change." code box no longer shows
  `inkspect init --repo owner/repository`; the live page and docs were verified.
- Scoped cleanup removed three older stopped Cloud containers and one older
  website container/image/source snapshot; active and previous releases retained.

### Original 0.1.0 checks

- macOS: Go test suite with race detection, `go vet`, native build.
- Linux ARM64: compiled Go test suite executed in an isolated read-only container;
  all tests passed, including replacing a running temporary executable.
- All six macOS/Linux/Windows amd64/arm64 release archives built successfully.
  Each archive contains only the expected executable; SHA256SUMS verified.
- CLI regressions cover credential redirects/storage, no blind HTTP retries,
  interactive/noninteractive boundaries, MCP lifecycle, source/snapshot limits,
  ignore rules, staged data, sensitive-source exclusions, freshness, managed skill
  preservation, installer checksums, update caching, compatibility, unpublished
  releases and replacement of a running temporary executable.
- Cloud API audit, typecheck and build passed. Main suite: 178 passed, 33 skipped,
  0 failed. Disposable PostgreSQL integration: 64 passed, 6 skipped, 0 failed.
  Integration includes purpose-scoped keys, source admission/revocation, provider
  regression coverage and existing review/billing/lifecycle behavior.
- Cloud UI audit/build and 48 tests passed. Updated Cloud API source package is
  consumed by the dashboard; key-purpose selection and CLI forwarding are present.
- Website lint/typecheck/build and 3 documentation-search tests passed. Generated
  search corpus covers 35 pages and 121 chunks. Installers, release metadata and
  the versioned skill manifest are generated from the CLI; skill SHA-256 verified.

## Remaining verification gaps

- Native Windows execution, PowerShell/cmd installer behavior and Windows ACL/live
  executable replacement remain untested on a Windows host. Cross-compilation is
  not a substitute. The repository includes a native three-OS CI matrix, but no
  hosted CI run is claimed for this publication.
- Authenticated live source expansion and review submission were not exercised
  in production. A single authenticated workspace-context query was verified in
  the 0.1.1 follow-up above; broader inference/review outcomes are not claimed.
- Broader browser/UI QA and code signing/notarization were not performed.
  Signing/notarization are outside this initial implementation.

## Authorized production publication

- Published six immutable native archives plus SHA256SUMS under
  `https://inkspect.si/cli/releases/v0.1.0/`. All six were downloaded from their
  public URLs and their hashes and archive contents verified. Windows executable
  headers were also checked for the expected amd64/arm64 machine types.
- Official website installers match the CLI source. The published shell installer
  installed and ran CLI 0.1.0 in temporary directories on macOS ARM64 and Linux
  amd64. Public skill installation, `skill updates --format json` and `update`
  passed on macOS; the versioned skill checksum and published metadata matched.
- Cloud UI 0.0.63, API 0.0.48 and engine gateway 0.0.39 are active. Database
  migrations completed successfully. Readiness checks passed; local-review
  admission is enabled in all three new candidates through the existing gate.
  The public Rust engine source was unchanged; its private gateway was updated.
- Live device start, pending polling, slowdown, unauthenticated identity/session/
  approval rejection and same-origin login continuation passed. No CLI credential
  was issued and no paid review was started by these production checks.
  The publicly installed macOS CLI also reached live approval polling and handled
  cancellation without changing existing credentials.
- Website 0.1.41 is active with CLI pages, public release/skill metadata and native
  archive routing. Website lint/typecheck/build and 57 deployment-tool tests passed.
- Scoped cleanup removed three obsolete stopped Cloud containers and one older
  website container/image/source snapshot. Active and previous releases were
  retained; no persistent volumes or shared databases were removed.

## Update behavior

Normal terminal commands check public release metadata at most once per day, with
a two-second network bound. Failure is silent and cached; no credential is sent.
Pipe/JSON/MCP modes suppress automatic checks and notices. Explicit `skill updates`
refreshes metadata; `skill update` preserves local edits and enforces minimum CLI
compatibility. `update` verifies the native archive checksum before replacement.
Windows may retain the previous executable until its process has exited; this
release does not automatically delete such backups on later launches.

## Browser login and whoami follow-up

- macOS race tests and Linux ARM64 runtime tests include approval polling,
  required slowdown, headless login, unsafe verification links, declined login,
  credential preservation, secret-free output and JSON whoami.
- Focused PostgreSQL suite: 8 passed, 0 failed. Covers concurrent one-time exchange,
  explicit consent, expiry/denial, CSRF origin checks, session expiry, role/workspace
  revocation, guessing limits, CLI-purpose issuance and existing provider behavior.
- Cloud UI route tests verify bounded user-code handoff, HttpOnly/SameSite cookie
  properties and consumption after account sign-in, without an external redirect.
- Cloud API and Cloud UI production builds pass. Website documentation, skill
  assets and six native release archives were regenerated and verified.
- At initial implementation, browser interaction and OS browser launching had not been manually tested.
  Native Windows execution remains a release gate. Nothing was deployed/published.

## Manual browser QA follow-up — Chrome on macOS

Completed against the actual local Cloud UI and device endpoints with a disposable
PostgreSQL database, two synthetic workspaces and a local email sink. The real
Better Auth magic-link handler was used with mail delivery redirected to that sink.
A temporary Go harness called the unchanged production CLI Run function with an
isolated credentials path; it did not replace the user's saved login.

Passed:
- Anonymous CLI link leads to account sign-in on the same public origin.
- Email sign-in returns to CLI consent with the terminal code retained.
- Workspace search and keyboard selection work; explicit code confirmation is required.
- Decline is shown in the browser and rejected by the waiting CLI without saving credentials.
- Approval completes the waiting CLI login; JSON whoami reports the selected secondary workspace.
- Reusing the consumed code shows an explicit expired/already-used error.

Found and fixed:
- Absolute continuation redirects exposed the internal Next server host behind a proxy.
  Relative Location headers now preserve the public origin; added a regression test.
- Global form styling stretched the consent checkbox. Its dimensions are now explicitly bounded.

The development proxy initially lacked its hot-reload connection, delaying client
hydration. The temporary QA proxy was corrected before completing the scenarios.
No production credentials, real emails, paid reviews or deployment were involved.
Actual macOS browser launching via `open`, external OAuth providers and native
Windows execution were not covered by this browser session.

Post-QA checks: all 48 Cloud UI tests, focused lint and production build passed.
Temporary QA services, database and credentials were removed.

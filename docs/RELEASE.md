# Release procedure

Publishing and deployment require explicit authorization. This document does not
authorize either operation. CLI/skill 0.1.2 are published as the index/progress
follow-up on 2026-11-20. See VERIFICATION.md for completed checks and the remaining
native Windows, production index-start and production source/review gaps.

1. Complete PRD automated checks and disposable Cloud integration checks.
2. Run the native CI matrix on Windows, macOS and Linux, including native smoke
   checks. Cross-compilation alone does not verify runtime or OS installers.
3. Review the six archives, their sole executable entry and SHA256SUMS. Exclude
   source, credentials, local configuration, logs and customer data.
4. Coordinate the Cloud API/UI release and its CLI-key/context and device-approval schema migrations.
   Keep the normal agent-review feature gate and existing billing policies.
5. After authorization, publish the checksummed native assets at `https://inkspect.si/cli/releases/v<VERSION>/`.
   Published versions are immutable; never replace an existing version’s assets.
   No automatic tag/push/publish is performed by the build script or CI checks.
6. Generate website installers and skill assets from this source:

   ```sh
   go run scripts/site-assets.go ../inkspect-website/apps/web/public
   ```

7. Verify actual release download URLs, SHA-256 and native installer behavior;
   then remove website preview wording and deploy the website when authorized.
8. Update CLI Version and installer pins for a new binary release. Skill versions
   can advance independently in `release.json`; set their `minimum_cli` explicitly.
   Publish versioned skill assets before marking the skill published. Mark the CLI
   published only after all six archives and checksums are actually downloadable.
   Regenerate website assets; `/cli/releases.json` drives update checks.

The one-line cmd installer calls PowerShell; it does not require Go or a compiler.
Windows code signing/macOS notarization are not included in this initial build.

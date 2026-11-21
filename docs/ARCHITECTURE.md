# Architecture

The CLI is a Go standard-library client. Its runtime has no engine or model
dependency. HTTP transport, configuration, context presentation, local snapshot
collection, skills and MCP are separate modules under `internal/cli`.

Cloud authenticates explicitly CLI-purpose keys, derives workspace/user identity,
checks repository membership and forwards context to the existing retrieval
service. The CLI never receives database, engine-service or model credentials.
Cloud owns admission, credits, rate limits, snapshot execution and metering.

For a repository-scoped `ask`, Cloud embeds the question with the configured Bedrock embedding
model (`search_query`), searches the repository's saved Qdrant snapshot, validates
the candidate chunks and reranks nonempty candidates with the configured rerank
model. Repository chunks were embedded during indexing (`search_document`); they
are not re-embedded for every question. This path has no query-embedding cache and
does not invoke a conversational model to compose an answer. Model usage therefore
includes the query embedding and, when candidates exist, reranking. `source`
expansion reads the saved snapshot directly without embedding or reranking;
`--explain` reports retrieval measurements without an extra model call.

Context is returned as versioned JSON with original source and revision metadata.
`--explain` exposes measured retrieval information. Source references are signed,
expire after one hour and are checked against current access on expansion.
Freshness compares remote source revisions with local repository identity/HEAD
and dirty state. `ask` never uploads the local working tree.

Local review reads Git objects and regular files, applies exclusions, computes
the canonical before/after snapshot digest, and submits to Cloud's existing agent
review service. The client only transports source; it never reviews or executes
it. Polling uses returned IDs. Source response text is untrusted data.

Installers fetch version-pinned native archives over HTTPS and verify SHA-256.
The checksum protects against mismatched/corrupt artifacts; HTTPS and the release
publisher remain the distribution trust boundary. Future publisher signing is a
separate release-hardening feature, not a property claimed by SHA-256 alone.

MCP and terminal commands use the same client operations. The initial MCP surface
is retrieval-only; review mutations require explicit CLI use and authorization.

## Sign-in

The native client starts device approval, opens the same-origin Cloud consent URL
and polls until approval, rejection or expiry. The browser supplies the authenticated
account and selected workspace; these are never trusted from the CLI. Approval uses
the existing session system. A successful, atomic one-time exchange issues a normal
CLI-purpose credential. No browser cookie or device secret is printed or persisted
in the project. `whoami` resolves current identity directly from Cloud.

## Workspace search

Without an explicit or automatically detected repository, Cloud fans out only
across the caller’s accessible workspace repositories, two calls at a time. One
workspace admission is shared; each engine call still validates repository claims
and meters its own models. Results retain signed repo/revision identities and are
merged by per-repository rerank score into one byte budget. This is not a joint
rerank or shared query-embedding cache. Launches stop after 70 seconds so the last
85-second upstream timeout fits admission expiry; unsearched repositories and
failures are visible omissions. Current access is rechecked before returning any
source. The engine contains no CLI-specific search orchestration.

## Cloud indexing and terminal feedback

The CLI starts an index with a random UUID, then polls that exact accepted job ID.
Cloud resolves identity/repository scope, checks current GitHub access and size,
and uses the existing index admission and queue serialization. Polling is scoped
to workspace, repository and connection. Terminal interruption stops observation;
the client does not send a Cloud cancellation request. No indexer is bundled.

Terminal-only progress writes to stderr. Ask animation uses plain carriage-return
frames supported by Windows, macOS and Linux; it is stopped and cleared before
output, errors or cancellation. JSON, MCP and pipe modes do not render progress.

# Server nomenclature migration — 2026-10-13

Scope: `/Users/docup/Projects/Liapoldus Engine/services/server` only. Baseline: clean `main`, HEAD `b40e38f`.
No sibling repository edits, commits, pushes, releases or documentation pin updates.

Runtime names, Go types/fields, owner JSON contracts, permissions, identities,
imports, instructions, fixtures and generators use Service terminology.
Modules: `liapoldus.local/server-service`,
`github.com/Liapoldus/service-sdk/v2`, `github.com/Liapoldus/wool/v3`.
Majors remain unchanged. There was no `peerplugin` package in this repository.

Control plane: Studio → CLI → Core → Service SDK REST.
Data plane: independent services → Wool. Only editor extensions are StudioPlugins.
The REST adapter is `restservice`, the owner manifest is `service.json`,
dispatch uses `handler.type=service` and `http.handlers.liapoldus_service`.
There are no aliases for the removed runtime names.

Neutral IDs and application methods (`server`, `server.sites.*`,
`server.operations.get`, registered peer methods) are unchanged.
Existing generation bytes are not migrated or remarshalled.
The native generation regression checks raw whitespace/order, digest,
opaque `plugin-marker` application data, and same-generation byte conflict.
Dated TODO reports and completed acceptance evidence preserve historical wording.
Neutral settings/site semantics and vectors plus `http-stream.json`
remain byte-identical to HEAD.

## Checks

All commands ran from the owner root. Node PATH:
`/Users/docup/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH`.

- PASS: `GOWORK=off GOFLAGS=-p=1 go run ./cmd/contracts` (regenerated code-owned artifacts).
- PASS: `GOWORK=off GOFLAGS=-p=1 go run ./cmd/contracts --check`.
- PASS: `GOWORK=off GOFLAGS=-p=1 go test ./contracts/... ./internal/application/settings ./internal/presentation/restservice ./internal/infrastructure/peercaller ./internal/infrastructure/caddy`; affected packages passed after the Wool owner regenerated its protobuf descriptor. The first focused attempt had failed in Wool descriptor initialization.
- PASS: `GOWORK=off GOFLAGS=-p=1 go test ./internal/presentation/restservice ./internal/infrastructure/peercaller ./internal/infrastructure/caddy` after adding the generation regression.
- PASS: `GOWORK=off GOFLAGS=-p=1 go test ./...`.
- PASS: `GOWORK=off GOFLAGS=-p=1 go build ./...`.
- PASS: `GOWORK=off GOFLAGS=-p=1 go vet ./...`.
- PASS: `GOWORK=off GOFLAGS=-p=1 npx vitest run tests/build/wool-module-version.test.ts tests/settings/schema.test.ts tests/admin/admin-surface.test.ts --maxWorkers=1` — 3 files, 25 tests.
- PASS: `GOWORK=off GOFLAGS=-p=1 npx vitest run tests/lifecycle/service-sdk-reload.test.ts tests/http/dispatch.test.ts tests/http/websocket-dispatch.test.ts --maxWorkers=1` — 3 files, 3 tests.
- PASS: `GOWORK=off GOFLAGS=-p=1 npx vitest run tests --maxWorkers=1` — 33 files, 83 tests; real Caddy processes, HTTP/1.1–HTTP/3, mTLS Reload, Wool dispatch/streams/WebSocket, site publish/recovery and Pebble ACME issuance/renewal.
- PASS: `git diff --check`.
- PASS: source audit against HEAD: registered peer application methods unchanged; neutral settings/site vectors/semantics and HTTP-stream contract bytes unchanged; generated artifacts match definitions; removed module/package/HTTP terminology appears only in historical or negative assertions and opaque application data.
- FAIL, existing baseline: `npx tsc --noEmit` — 19 diagnostics, exactly identical to an isolated clean HEAD export.
- FAIL, existing baseline: `npx eslint tests` — 51 errors, also reproduced on the isolated clean HEAD export. No new migration diagnostics.

## Remaining boundaries/blockers

No Server runtime migration failure remains in the executed tests.
The existing TypeScript type/lint debt prevents claiming an entirely green
optional static quality gate.
Published SDK/Wool revisions and renamed remotes were not released or verified
by this task. Hosted Linux CI and the full Studio → CLI → Core workflow are not
proven by the owner-local tests. The aggregate workspace separately passed its
local VitePress sync/build and sidebar checks; exact remote source pins still
wait for publishing the owner repositories. Shared contract changes and results
were sent to the Core coordination chat.

## Exact changed paths

Paths below are relative to the absolute owner root above; no globs.
Renames include both removed and replacement paths.

### Renamed

- `contracts/definitions/plugin/plugin.go` → `contracts/definitions/service/service.go`
- `contracts/plugin_manifest.go` → `contracts/service_manifest.go`
- `contracts/v1/plugin.json` → `contracts/v1/service.json`
- `docs/site/plugins/server.md` → `docs/site/services/server.md`
- `internal/presentation/restplugin/adapter.go` → `internal/presentation/restservice/adapter.go`
- `internal/presentation/restplugin/purposes.go` → `internal/presentation/restservice/purposes.go`
- `internal/presentation/restplugin/purposes_test.go` → `internal/presentation/restservice/purposes_test.go`
- `internal/presentation/restplugin/settings_errors_test.go` → `internal/presentation/restservice/settings_errors_test.go`
- `internal/presentation/restplugin/site_actions.go` → `internal/presentation/restservice/site_actions.go`
- `tests/build/pluginprotocol-module-version.test.ts` → `tests/build/wool-module-version.test.ts`
- `tests/fixtures/plugin-sdk-reload/main.go` → `tests/fixtures/service-sdk-reload/main.go`
- `tests/lifecycle/plugin-sdk-reload.test.ts` → `tests/lifecycle/service-sdk-reload.test.ts`

### Modified

```text
.github/workflows/verify.yml
AGENTS.md
README.md
TODO.md
cmd/contracts/main.go
cmd/server/main.go
contracts/admin_queries.go
contracts/assets.go
contracts/definitions/admin/actions.go
contracts/definitions/admin/actions_schema.go
contracts/definitions/admin/query.go
contracts/definitions/admin/query_schema.go
contracts/definitions/admin/surface.go
contracts/definitions/admin/surface_schema.go
contracts/definitions/admin/surface_vectors.go
contracts/definitions/documents.go
contracts/definitions/documents_test.go
contracts/definitions/http/dispatch.go
contracts/definitions/http/response_schema.go
contracts/definitions/http/stream_schema.go
contracts/definitions/http/stream_vectors.go
contracts/definitions/settings/settings_schema.go
contracts/definitions/settings/vectors.go
contracts/definitions/site/manifest_schema.go
contracts/definitions/site/manifest_vectors.go
contracts/definitions/site/metadata_schema.go
contracts/definitions/site/publish.go
contracts/definitions/site/receipt_schema.go
contracts/dispatch.go
contracts/settings_definition_test.go
contracts/site_archive.go
contracts/site_artifact.go
contracts/site_manifest.go
contracts/v1/admin-actions.json
contracts/v1/admin-actions.schema.json
contracts/v1/admin-query.json
contracts/v1/admin-query.schema.json
contracts/v1/admin-surface-vectors.json
contracts/v1/admin-surface.json
contracts/v1/admin-surface.schema.json
contracts/v1/artifact-metadata.schema.json
contracts/v1/artifact-operation-result.schema.json
contracts/v1/http-dispatch.json
contracts/v1/http-response-action.schema.json
contracts/v1/http-stream-vectors.json
contracts/v1/http-stream.schema.json
contracts/v1/settings.schema.json
contracts/v1/site-manifest.schema.json
contracts/v1/site-publish-manifest.json
docs/site/core/examples/proxy.md
docs/site/core/examples/simple-site.md
docs/site/core/examples/tls.md
go.mod
go.sum
internal/application/settings/service.go
internal/application/settings/service_test.go
internal/application/site/publish.go
internal/domain/interfaces/certificate_inventory.go
internal/domain/interfaces/site_release_repository.go
internal/domain/models/settings/model.go
internal/domain/models/settings/model_test.go
internal/domain/models/site/release.go
internal/infrastructure/caddy/compiler.go
internal/infrastructure/caddy/dispatch_caller.go
internal/infrastructure/caddy/header_limit.go
internal/infrastructure/caddy/http_dispatch.go
internal/infrastructure/caddy/peer_directory_dispatch_test.go
internal/infrastructure/caddy/runtime.go
internal/infrastructure/caddy/site_static.go
internal/infrastructure/peercaller/caller.go
internal/infrastructure/peercaller/diagnostics_test.go
internal/infrastructure/site/archive.go
internal/infrastructure/site/manifest.go
internal/infrastructure/site/store.go
tests/admin/admin-surface.test.ts
tests/build/caddy-fixture-storage-isolation.test.ts
tests/build/http2-http3.test.ts
tests/build/v1-scope.test.ts
tests/fixtures/caddy-activation/main.go
tests/fixtures/caddy-rest-process/main.go
tests/fixtures/caddy-storage-isolation/main.go
tests/fixtures/config-apply-schema/main.go
tests/fixtures/config-apply/main.go
tests/fixtures/custom-tls/main.go
tests/fixtures/dns-provider-inventory/main.go
tests/fixtures/http-dispatch/main.go
tests/fixtures/http-header-limits/main.go
tests/fixtures/http-stream-limits/main.go
tests/fixtures/http2-http3/main.go
tests/fixtures/settings-vectors/main.go
tests/fixtures/shared/configuration.go
tests/fixtures/shared/peer_caller.go
tests/fixtures/shared/site_documents.go
tests/fixtures/site-archive/main.go
tests/fixtures/site-manifest/main.go
tests/fixtures/site-publish-concurrency/main.go
tests/fixtures/site-publish-recovery/main.go
tests/fixtures/site-publish/main.go
tests/fixtures/upstream-ca/main.go
tests/fixtures/websocket-dispatch/main.go
tests/http/dispatch.test.ts
tests/http/stream.test.ts
tests/http/websocket-dispatch.test.ts
tests/lifecycle/caddy-rest-process.test.ts
tests/lifecycle/manual-sdk-process.test.ts
tests/settings/compiler.test.ts
tests/settings/config-apply.test.ts
tests/settings/request-cookie-settings.test.ts
tests/settings/schema.test.ts
tests/settings/stream-limits.test.ts
tests/site/manifest.test.ts
tests/support/contracts.ts
tests/support/models.ts
tests/tls/acme-lifecycle.test.ts
tools/migrate.mjs
tools/organize.mjs
tools/seed.mjs
tools/typefinish.mjs
tools/typefix.mjs
tools/types.mjs
```

### Added

```text
internal/presentation/restservice/generation_test.go
docs/nomenclature-migration-report.md
```


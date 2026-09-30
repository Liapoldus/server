# TODO — Server plugin v1

## Документация

- [x] Server-owned Markdown/examples перенесены в `docs/site/`; общие Core
  страницы принадлежат Core, сайт агрегирует эту документацию по pinned SHA.
- [ ] После изменения owner docs обновить pin в
  `liapoldus.github.io/docs-sources.json` и проверить единый сайт.

## Проверенное состояние на 2026-09-30

Последний локальный commit: `075fb52`. Settings compiler, site archive/manifest,
Admin Surface contracts и SDK adapter добавлены, но это не завершённая миграция.
После обновления `plugins/go.work` до Go 1.26.0 команда
`go test ./server/... ./forms-db/...` запускается и падает: Server и его fixtures
всё ещё импортируют удалённые `pluginprotocol/pluginv1` и
`pluginprotocol/presentation/sdk`; `internal/presentation/restplugin` также не
совпадает с текущим Plugin SDK API. До сборки `cmd/server` и реального
Core→SDK→Server Reload/pull/ACK smoke Server v1 не готов.

Нормативная цель: [Core target](https://liapoldus.github.io/core/architecture/target),
[v1 acceptance](https://liapoldus.github.io/core/configuration/acceptance) и
[Server contract](https://liapoldus.github.io/plugins/server). Агентное задание:
[`tasks/prompts/server-plugin.md`](../../tasks/prompts/server-plugin.md).
Этот репозиторий — отдельный HTTP Server plugin; `server` — его product/API
identity, Caddy — реализация внутри binary.

## Цель и фиксированные ограничения

- Оператор вручную устанавливает и запускает Server plugin. Core не владеет
  процессом, не может запускать/останавливать/перезапускать/масштабировать или
  удалять его.
- Public v1 surface: HTTP/1.1, HTTP/2, HTTP/3, TLS/ACME, static sites,
  reverse proxy, WebSocket/SSE и plugin dispatch. Caddy-L4/public TCP/UDP
  listeners/relay/P2P/NAT traversal — v2, не binary/schema/API/conformance v1.
- HTTP/3 остаётся HTTP. Core содержит только Management/control plane и не
  содержит Caddy runtime/data plane.
- Core↔plugin lifecycle/config — Plugin SDK REST + unique per-replica mTLS:
  `Reload`, plugin-initiated exact generation pull, local validate/apply и
  digest ACK. `pluginprotocol` — только generic plugin↔plugin API.
- Settings schema, site archive/manifest, product errors, capabilities и Admin
  Surface принадлежат только этому repo. Общие settings редактируются через
  Core generic API; product Admin Surface не заменяет его control RPCs.
- ACME и certificate renewal — Caddy/CertMagic; custom certificates задаются
  secret references. Внешний Admin API Caddy не публикуется. Persistence
  сертификатов и release artifacts принадлежит Server plugin.
- `current`/`previous` сохраняются; site release immutable и переключается
  атомарно после успешной validation/operation.

## Что уже зафиксировано контрактами

- [x] В public plugin identity, contracts, CLI/management naming использовать
  `server`; Caddy допустим как имя runtime/library/technology, не как отдельный
  public service identity. Внутренние fixture/package names разрешены, пока не
  попадают в установленный API.
- [x] Удалить direct Caddy-L4 module registration/import и L4 listener/route
  branches из v1 settings contract; не добавлять Go `net` fallback.
- [x] Settings schema закрывает HTTP/HTTPS route model, TLS, matcher semantics,
  upstream pools и site/config references.

Эти пункты отмечены по существующим source changes; перед release повторить
проверки по фактическому binary/dependency graph и всем consumers.

## P0 — единый SDK lifecycle

- [ ] Полностью перенести production `cmd/server` startup, manifest/schema,
  health/readiness, settings pull/apply и ACK на Plugin SDK; сохранить real
  HTTP child process и private mTLS.
- [ ] Удалить legacy `pluginprotocol` ConfigSchema/ConfigApply/Shutdown and
  Bootstrap lifecycle service, generated product stubs, fixtures и dependencies
  после migration всех consumers. Не оставлять dual API/fallback.
- [ ] Подключить Core-managed per-replica identity/mTLS и scoped grants для
  certificate/private key references. При отсутствии scoped grant отказать
  безопасно; не читать ключи через environment или arbitrary filesystem paths.
- [ ] Для config reload проверить invalid/candidate activation failure сохраняет
  прошлый рабочий Caddy snapshot; Core desired generation и per-replica ACK
  остаются наблюдаемыми.
- [ ] Проверить product Manifest capability→invocation mode для всех объявленных
  HTTP actions; activation не проходит с неизвестным plugin/mode.

## P1 — HTTP data plane, TLS и policies

- [ ] Запустить HTTP/1.1, HTTP/2 и HTTP/3 через реальный Server binary на
  поддерживаемых платформах. Redirect, TLS versions, HTTP3 listener, ACME
  ownership и custom certificate refs сверить с versioned settings contracts.
- [ ] Проверить host/path normalization, exact/prefix/glob/RE2 semantics,
  first-match ordering, duplicate/overlap rejection, methods, percent-decoding,
  dot segments, UTF-8/control/NUL handling и static path traversal.
- [ ] Прогнать reverse proxy 1–32 origins, weighted smooth round-robin
  (weight 1..100), required upstream TLS verification, optional CA reference,
  failure cooldown и максимум connect-before-bytes retry; не replay request
  после отправки любых request bytes.
- [ ] Прогнать WebSocket accept/reject/subprotocol/message boundaries, SSE
  serialization, HTTP streaming/cancel/backpressure and response-start failure
  semantics, если v1 target/acceptance требует их для этого plugin.
- [ ] Проверить request/header/body/response/concurrency/idle/duration limits,
  cookie allow-list, typed ordinary/HttpOnly response actions, atomic validation
  before headers/101 and secret/cookie redaction.
- [ ] Проверить TLS issuance/readiness independently: activation не блокируется
  ACME; renewal/revoke работают только для Caddy-managed certificates; DNS
  provider modules — только явные build modules и owner inventory.

## P2 — sites и plugin admin actions

- [ ] Прогнать site publish multipart metadata + один `.tar.gz`, durable `202`
  operation, idempotency, expected-current CAS, streaming digest и bounded
  memory. Обязателен корневой `site-manifest.json` с schemaVersion, siteId,
  documentRoot и indexDocument.
- [ ] Проверить пределы: 128 MiB compressed artifact, 64 KiB metadata, 64 KiB
  multipart overhead (request envelope ≤128 MiB +128 KiB), expanded ≤512 MiB,
  max 10,000 logical entries, ratio ≤100:1. Reject traversal/absolute paths,
  symlinks/hardlinks, special files, duplicate/case/NFC collisions, corrupt or
  truncated gzip/tar, invalid digest/manifest.
- [ ] Проверить immutable revisions и атомарную current/previous swap; failed
  operation и crash/restart до/после pointer switch не повреждают serving tree.
- [ ] Admin Surface предоставляет только plugin-owned site/release/certificate
  operations и status. Любые common settings или Core-wide service keys не
  принимаются Caddy-specific endpoints.

## P3 — final integration/removal

- [ ] Реальный ручной процесс smoke: Core→SDK REST/mTLS→Reload→exact pull→apply
  →ACK, затем external HTTP request проходит новым runtime generation. Повторить
  restart Core, restart Server, invalid config, mTLS reject и offline Core.
- [ ] Проверить прямой plugin dispatch/policies без Core traffic proxy и без
  protocol product methods. Отдельно доказать disabled/unreachable Caddy Admin
  API и отсутствие слушающих legacy public L4 endpoints.
- [ ] Удалить dead Caddy-L4/TCP/UDP code/contracts/vectors после import/test
  audit; удалить устаревшие Caddy-named API aliases. Сохранить `Caddy` только
  как technology/internal implementation name.
- [ ] Полный plugin suite: `npx vitest run tests`, `go test ./...` где есть Go
  tests, `go build ./...`, `go vet ./...`, macOS/Linux, HTTP/TLS/ACME and
  Core/SDK/forms-db integration; отметить только реально исполненные gates.

## Отложено до v2

Caddy-L4, public TCP/UDP relay, P2P/NAT traversal, CAPTCHA, Identity/OIDC/OAuth,
TUF/install, Core-managed process/container lifecycle, Docker/Compose/Swarm/
Kubernetes. Не добавлять v2 API/dependencies в v1 binary или v1 test matrix.

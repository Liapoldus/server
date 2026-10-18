# TODO — Server service v1

## Current local secret-capable runtime preflight — 2026-10-18

- Typed `service status server --target local` from the Studio-selected CLI
  reports active Server 1.1.11, digest
  `sha256:3f3f66e4d8b852b337d07f39276aa949ca1df5095fa411b88ca976286d4075a1`,
  generation 92 and ready/acknowledged. The active readiness envelope still
  reports schemaVersion 1; Project settings are v2 and Studio reports them as
  unapplied. No apply was attempted in this preflight.
- Studio's Server Editor 0.3.64 opens from the service Inspector and completes
  its handshake. The `/echo` route editor exposes the auth controls and its
  contextual picker reports `Нет активных ссылок` for the declared purpose
  `server.http.auth.credential`. Studio's global reference catalog also reads
  successfully and is empty. This is UI/read-path evidence only; no credential
  was written or delivered to Server.
- Re-ran `vitest run tests/lifecycle/caddy-rest-process.test.ts --maxWorkers=1`:
  1/1 pass. This is fixture coverage, not live Core grant/rotation acceptance.
  No Server source, Project, runtime or secret state changed.

## Current local runtime and uninstalled 1.1.11 candidate — 2026-10-18

- Read-only typed CLI status for `Server Echo Studio` / `local` reports active
  Server 1.1.8, artifact digest
  `sha256:5a8e02a65624441b4654eda80103e7e91ea9406f069236f12ce2e128ae6807b2`,
  generation 92 and ready. The Project still pins Server 1.1.10; do not treat
  that desired pin as the active artifact.
- Source 1.1.11 declares the Admin Surface `queryResource` required by the
  installed editor and the HTTP-auth, TLS certificate, private-key and upstream
  CA secret purposes. Its Linux/arm64 candidate SHA-256 is
  `3f3f66e4d8b852b337d07f39276aa949ca1df5095fa411b88ca976286d4075a1`; it has
  not been trusted or installed. Live migration, Admin Surface and scoped-grant
  acceptance remain open pending the exact-digest Studio flow.
- This status check was read-only and changed no Project, Core, Server or secret
  state.
- Re-ran the real child-process path with
  `PATH=/Users/docup/.cache/codex-runtimes/codex-primary-runtime/dependencies/
  node/bin:$PATH ./node_modules/.bin/vitest run
  tests/lifecycle/caddy-rest-process.test.ts --maxWorkers=1`: 1/1 pass. This
  exercises Server and SDK grant delivery in an isolated fixture; it does not
  verify the uninstalled 1.1.11 candidate against the live Core target.

## One-time settings v1 bootstrap for the Studio v2 migration — 2026-10-18

- Server 1.1.10 could not start against the already-active Project envelope
  `schemaVersion: 1`: the runtime accepted only v2, so Core rolled the artifact
  back before Studio could read the v2 schema and offer its migration preview.
- Server 1.1.11 adds a bounded bootstrap path: a v1 payload is validated against
  the current v2 settings schema with only the validation envelope rewritten;
  the original runtime-config bytes remain unchanged. The first active v1
  generation can be loaded, while changed/new v1 generations are rejected after
  activation. Studio then owns the reviewed Project migration and v2 apply.
- Targeted Go tests pass for `internal/domain/models/settings`,
  `internal/application/settings`, `contracts`, and
  `internal/presentation/restservice`; `go vet ./...`,
  `go run ./cmd/contracts --check`, and `git diff --check` pass.
- The actual `Server Echo Studio/services/server/settings.json` values pass the
  current Server v2 JSON Schema after changing only the candidate root
  `schemaVersion` in memory; no Project bytes were modified by this check.
- Initial full Vitest attempts at low disk space stopped at linker failures
  (`no space left on device`). After free space increased, the complete owner
  checks passed: `GOTOOLCHAIN=go1.26.0 GOWORK=off GOFLAGS=-p=1 go test ./...
  -count=1`; `PATH=/Users/docup/.cache/codex-runtimes/codex-primary-runtime/
  dependencies/node/bin:$PATH GOTOOLCHAIN=go1.26.0 GOWORK=off GOFLAGS=-p=1
  npm test -- --maxWorkers=1 --no-file-parallelism` (34 files / 88 tests); and
  `GOTOOLCHAIN=go1.26.0 GOWORK=off GOFLAGS=-p=1 go build ./...`. Previously
  passing `GOWORK=off GOFLAGS=-p=1 go vet ./...` and `GOWORK=off GOFLAGS=-p=1
  go run ./cmd/contracts --check` were run on the same unchanged Server source
  commit. No caches or unrelated files were removed.
- Production artifact build subsequently passed:
  `GOTOOLCHAIN=go1.26.0 GOWORK=off GOOS=linux GOARCH=arm64 CGO_ENABLED=0
  GOFLAGS=-p=1 go build -trimpath -ldflags '-s -w' -o
  /tmp/liapoldus-server-20261018-a46ce85 ./cmd/server`. SHA-256 is
  `3f3f66e4d8b852b337d07f39276aa949ca1df5095fa411b88ca976286d4075a1`;
  `file` confirms a static ELF aarch64 executable and Go build metadata records
  Server commit `a46ce85625eff3efebe06f90efcf37e90065538e`,
  `vcs.modified=false`, service-sdk `a5e55efdc9091b9f8e631041d800f1d8f1c2cb21`
  and Wool `c2b51ed192af5ec02fe0295b97a7f6d4c4834c3d`. It is not installed or
  running; exact-digest local trust and live migration remain pending.
- The shared volume currently has 4.3 GiB available. No user data or caches
  were removed.

## Exact-source current runtime candidate — 2026-10-18

- Built Linux/arm64 Server from published source HEAD
  `6dcb45cf01b8a7966e3e6913778b664558d2463a` to
  `/tmp/liapoldus-server-20261018-6dcb45c`, SHA-256
  `ed9f865367e9389c5a2fad2a224ab8ff407c3c7cb846e6ebb8e198bdc54995f9`.
  `file` confirms static ELF aarch64; `go version -m` records the exact
  revision and `vcs.modified=false`. The binary reports the current Server
  source release `1.1.10`.
- `GOWORK=off go test ./...`, `GOWORK=off GOFLAGS=-p=1 go vet ./...`, and the
  34-file / 88-test Vitest suite pass. The exact production `./cmd/server`
  cross-build passes. Full `GOWORK=off GOFLAGS=-p=1 go build ./...` was
  attempted but could not link fixture executables because the host temporary
  volume ran out of space; it is not a pass.
- The candidate has not been installed or run. Live settings/editor/secret-grant
  acceptance remains open pending the exact-digest Studio flow; no Project,
  Core or runtime state changed while preparing it.

## Exact-source runtime candidate for current editor contract — 2026-10-18

- Built Linux/arm64 Server from published source HEAD `5780448923ac69652433e4c6927d80e1b65aed8d` to `/tmp/liapoldus-server-current` with `GOTOOLCHAIN=go1.26.0 GOWORK=off GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /tmp/liapoldus-server-current ./cmd/server`.
- SHA-256: `25e372207d1dc24a93510142e108d5274770b264138345bd08626459729658a4`. `file` reports a static ELF aarch64 executable; `go version -m` records the exact revision and `vcs.modified=false`. This source release advertises Server `1.1.10`.
- The current source has passing focused Admin Surface tests, Go tests, all 34 Vitest files / 88 tests, contract generation/checks and `go vet`; the Linux/arm64 production build passed. A full fixture build previously hit the host temp-volume limit and is not represented as passed. This candidate is not installed; live settings, editor and secret-grant acceptance remain open.
- Fresh focused runtime checks passed: `go test ./internal/infrastructure/caddy ./internal/presentation/restservice -run 'Secret|Auth|Purpose' -count=1`.

## Admin Surface query and compare-and-swap metadata — 2026-10-18

- Added optional `queryResource` to table sections and `ifMatchField` to
  actions. All Server tables name their typed query resource; rollback binds
  its `expectedCurrentRevision` input to the host's `If-Match` request header.
- Bumped the authenticated Server release claim to `1.1.10`; the Admin Surface
  remains additive within v1 and does not add a second action execution path.
- `GOWORK=off go test -ldflags=-w ./... -count=1` passes; the focused Admin
  Surface suite passes 11/11, and full Vitest passes 34 files / 88 tests.
  `GOWORK=off go run -ldflags=-w ./cmd/contracts --check`, `go vet ./...`
  and `git diff --check` pass. Full `go build -ldflags=-w ./...` could not
  link all fixture binaries because the host temporary volume ran out of space;
  production `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/server`
  passes. That pre-commit diagnostic binary embeds a dirty-tree marker and is
  not an install candidate. No runtime binary has been trusted or installed;
  live local acceptance requires a clean exact-digest build and Studio trust.

## Current Server 1.1.9 candidate and full owner gates — 2026-10-18

- Server 1.1.9 Linux/arm64 was rebuilt from source commit `a2b1bdc` at
  `/tmp/liapoldus-server-linux-arm64-2026-10-18-a2b1bdc`, SHA-256
  `784923bea75c65b2ea7eb4ca325bb413e3ef0eef1a84005798e7059762280b60`.
  `file` confirms static ELF aarch64 and `go version -m` records the expected
  source module. It has not been registered or installed; exact-digest Studio
  trust is pending.
- Current HEAD `e7aaf3c` adds only this TODO checkpoint; the candidate contains
  the same Server source code and records its build revision in Go metadata.
- Current source checks pass: `GOWORK=off go test -ldflags=-w ./... -count=1`,
  `GOWORK=off go build -ldflags=-w ./...`, `GOWORK=off go vet ./...`,
  `GOWORK=off go run -ldflags=-w ./cmd/contracts --check`, and the full Vitest
  suite (34 files / 87 tests). The earlier test attempt that failed under low
  disk space is superseded by this complete successful run.
- No live Server/Core/Project configuration was changed while building and
  checking this candidate. Runtime/editor acceptance remains pending the
  exact-digest confirmation and native Studio trust/update flow.

## Active upstream health-check runtime fix — 2026-10-18

- The settings-v2 contract advertised active reverse-proxy health probes, and
  the Caddy compiler emitted `health_checks.active`, but the custom upstream
  transport could not resolve Caddy's health-check upstream pointer before the
  first user request. This made every initial probe fail without reaching the
  origin. The transport now resolves a unique configured dial address for the
  first probe and caches that pointer mapping; duplicate dial addresses are
  rejected when active probes are enabled because their per-origin transport
  settings cannot be selected safely.
- Verified by an actual Caddy HTTP integration test: a 503 `/ready` upstream is
  probed and excluded while the healthy origin serves the route. Go Caddy tests,
  settings schema tests, and deterministic contract export check pass.
- Server runtime version is now `1.1.9`. Linux/arm64 artifact
  `/tmp/liapoldus-server-1.1.9-linux-arm64` was built from this commit, verified
  as ELF aarch64 and Go revision `f32e85438a3c06f92efc179faf3bf854441d1cc7`,
  SHA-256 `a76ca53025f810dc75dc44c2a75e5b698bcdb4d205c2a58f2b5f6a0e5902e3a4`.
  It is a local candidate only; the normal Studio trust step is still required
  before installation, and no live runtime has been changed.
- The canonical settings guide now describes the schema v2 feature defaults,
  auth references, rate limiting, rewrites, redirects, active probes and current
  limits. A full Vitest run was attempted but 12 tests failed while the host had
  only about 500 MiB free; most failures are Go linker `No space left on device`.
  The targeted health runtime and schema tests pass.

## Settings-v2 HTTP and secret-reference checkpoint — 2026-10-18

- Server settings schema v2 now covers service, static, reverse-proxy and
  redirect routes; inherited route defaults and per-route overrides; request/
  response headers, CORS, auth references, rewrite, rate limiting, cache,
  compression and reverse-proxy active health checks. Disabled features retain
  their settings. Runtime compilation validates and activates only complete
  candidates; failed validation keeps the active Caddy snapshot untouched.
- Local service secret values are resolved only through Core-issued, scoped
  SDK grants. Caddy configuration stores keyed MACs for auth comparison, not
  plaintext credentials or opaque references. Bearer/API-key/Basic checks,
  trusted-proxy rate limiting, redirects, rewrite and active health probes have
  Go regressions. Static assets use immutable release IDs and safe path
  resolution beneath a release root.
- Verification on 2026-10-18: `go test -ldflags=-w ./... -count=1`,
  `go build ./...`, `go vet ./...`, `go run -ldflags=-w ./cmd/contracts --check`,
  and Vitest (34 files / 86 tests) pass. The `-ldflags=-w` on test/contract
  binaries only omits debug symbols; it was used after the host temporary volume
  ran low while linking fixture binaries. OrbStack was started with `orbctl
  start` for the ACME Pebble test and Docker reports 29.8.2.
- The previous Server runtime release was `1.1.8`; its owner source and contract
  changes are committed and pushed as `2bc9f56`; the central documentation source pin is
  published. The matching Server Editor `0.3.59` package passes
  73/73 tests and Studio's package inspector, but it has not been installed;
  exact digest `682bd7af7b2d61c312403628b7407fda7a621b9d9d565e65fa7004849cd9094f`
  still needs the normal native Studio trust confirmation.
- Live acceptance against the selected local Core/Server Project, multi-replica
  rotation ACK/timeout/restart recovery, the packaged macOS WebView, and hosted
  cross-platform workflows remain open. Do not mark source tests as live
  runtime acceptance.

Additional live preflight on 2026-10-18: typed runtime start registered
Server 1.1.8 (trusted artifact SHA-256
5a8e02a65624441b4654eda80103e7e91ea9406f069236f12ce2e128ae6807b2) and
Echo 1.2.2; Core-backed status reports both ready at generation 92, and
GET http://127.0.0.1:19080/echo?text=studio-http-check-20261018 returned the
matching Echo body. The live Server manifest publishes
service-editor-buttons/v1 with server.endpoints. The editor and multi-replica
secret retirement gates remain open.

## Live unknown-route retest — 2026-10-16 20:54 UTC

- Earlier notes below saying Server `1.1.5` returns `200` for unmatched paths
  are historical and superseded. On the current running target, an unknown
  path returned `HTTP 404 Not Found`; `/echo?text=continuation-current-runtime`
  returned `HTTP 200` and `echo: continuation-current-runtime`.
- Read-only Docker status showed `liapoldus-server` and `liapoldus-echo` both
  Up. No runtime lifecycle or service configuration was changed in this check.
- Keep the source regression tests as the permanent gate; live behavior is
  now verified on the current Server runtime.

Текущая номенклатура: Server service, Service SDK REST и Wool. Control plane:
Studio → CLI → Core → Service SDK REST; прямой data plane — сервисы → Wool.
Старые имена и пути в датированных отчётах и завершённых acceptance-записях ниже
сохранены как историческое evidence, а не поддерживаемые API или команды.
Актуальный reload regression: `tests/lifecycle/service-sdk-reload.test.ts`.

## Breaking nomenclature migration — 2026-10-13

Server-owned code/contracts/docs/tests/generators переведены на Service SDK v2
и Wool v3; majors, нейтральные IDs/application methods и immutable generation
bytes/digests сохранены. Старый dispatch handler type отклоняется без alias.
Go test/build/vet, deterministic contract check и полный Vitest прошли
(33 файла / 83 теста). `tsc` (19 ошибок) и ESLint (51 ошибка) воспроизводят
существующий clean-HEAD baseline. Локальный VitePress sync/build после миграции
прошёл; hosted CI и remote source pins ждут публикации owner repositories.
Точные пути, команды и ограничения: [отчёт](docs/nomenclature-migration-report.md).

## Одобренный scope конфигуратора Studio — 2026-10-13

Это новое активное направление по [`../../studio/plan.md`](../../studio/plan.md);
прежние датированные записи «product scope remains outside this slice» описывают
предыдущие срезы, а не отменяют эту явную цель.

- [x] Публиковать `service-configuration-ui/v1` через Service SDK manifest;
  descriptor валидируется SDK против Server-owned JSON Schema и задаёт
  декларативные hints для listeners, service routes и typed HTTP parameters.
  JSON Schema остаётся единственным validation authority.
- [x] Ввести Server-owned `server-http/v1`: method + template path, typed
  path/query/header scalar/array params, explicit repeat policy, required/bounds,
  configurable unknown-param policy, JSON request-body schema, response
  status/contentType/schema/example. Legacy untyped service routes и дублирование
  method/path в `match` отклоняются; live response schemas не применяются.
- [x] Валидировать method/path/query/headers/body и response examples до peer
  dispatch; ошибки запроса — generic 400 без вызова сервиса. Query/body/schema
  sizes и parameter counts bounded; settings candidate rejection сохраняет
  текущий snapshot через существующий atomic apply pipeline.
- [~] Runtime/contract adversarial tests и generated request-parameters schema
  добавлены; полный child-process suite и cross-repo compatibility gates
  остаются отдельными acceptance-пунктами.
- [x] Детерминированный migration preview/recovery для существующих untyped
  service routes реализован в Studio Server Editor. Preview предлагается только
  для представимых без потери полей routes, показывает добавляемый response
  template, а host до patch сохраняет точные байты Project в mode-0600 recovery
  copy. Неизвестные и непредставимые routes остаются без изменений. Studio
  покрывает отказ при stale source и восстановление. Пакет редактора `.60`
  прошёл 74/74 теста; digest и статус live WebView acceptance записаны в
  актуальном Studio checkpoint. Установка ожидает native trust confirmation.
- [~] Specialized editor реализуется только в `plugins/studio-server/` через
  Studio declarative extension/host bridge; пакет `0.3.0` редактирует
  `server-http/v1` endpoints и меняет общий Project draft. Server не получает
  Studio, Core или CLI зависимости. GUI install/trust и resource/host/matcher
  редакторы остаются открытыми.
- [~] JSON Schema settings fields для request body и response помечены
  `x-studio-json-document: true`, чтобы Studio могла заменять документ как
  ограниченное типизированное значение, сохраняя fail-closed поведение для
  обычных `additionalProperties: true`. Runtime version поднят до 1.1.5;
  contract generation/check и полный `go test ./...` прошли, Linux/arm64 binary собран
  с site-release OS advisory lock и schema fix (SHA-256
  `0b741f53186e7d4a4eb05a9e9e20ac10c0647fee56cb6884e64d877edc556a34`).
  Unit/race и child-process publish-race проверки прошли. В Docker бинарник ещё
  не запускался; live manifest/schema и отсутствие повторных Caddy
  lock-maintenance ошибок требуют подтверждённого Studio runtime restart.
- [ ] Сверить независимость от Echo и подтвердить прямой Server → Wool → Echo
  путь без Core/CLI business proxy, затем пройти UI-driven Docker/curl acceptance.

До исполнения пункты OPEN. Текущий Server runtime не считается обновлённым
только потому, что Studio plan описывает желаемый редактор.

## Studio endpoint add/remove и unmatched HTTP fallback — 2026-10-16

- Новый live recheck текущего target `local`: обычный
  `GET /_acceptance-unknown-route-20261016` через уже работающий Server `1.1.5`
  вернул HTTP 200 с пустым body. Это подтверждает, что исправление ещё не
  присутствует в запущенном контейнере.
- Source regression `TestCompileSettingsReturns404ForUnmatchedRequests`
  подтверждает terminal catch-all 404 и Caddy validation. Исправлено
  обнаруженное release metadata расхождение: Core-facing manifest и replica
  registration теперь берут одну `restservice.ServiceVersion` (`1.1.6`). Новый
  manifest assertion сначала воспроизвёл `1.1.5`, затем прошёл после фикса.
- Точечные проверки прошли:
  `GOWORK=off go test ./internal/infrastructure/caddy ./internal/presentation/restservice
  -run '^(TestCompileSettingsReturns404ForUnmatchedRequests|TestServerManifestPublishesValidatedConfigurationUI)$' -count=1`.
  Дополнительно добавлен runtime HTTP regression:
  `TestRuntimeReturns404ForUnmatchedRequest` активирует Caddy на loopback и
  проверяет фактический HTTP status неизвестного маршрута; точечный Caddy и
  manifest набор прошёл.
  Повторно запущен точный runtime gate:
  `GOWORK=off go test ./internal/infrastructure/caddy -run
  '^(TestRuntimeReturns404ForUnmatchedRequest|TestCompileSettingsReturns404ForUnmatchedRequests)$'
  -count=1 -v` — обе проверки PASS. Текущий Server `1.1.5` в target `local`
  по-прежнему возвращает `HTTP 200` с пустым body для
  `/_acceptance-unknown-route-20261016`.
  Linux/arm64 candidate собран из текущего worktree:
  `/tmp/liapoldus-server-404.wjKFck/server`, SHA-256
  `efe16ac1343a03935ec797cfa968c7382e237521db6a6d817f134b408564f494`.
  ELF architecture, Go build metadata и module replacements на локальные
  Service SDK/Wool проверены. Через Studio candidate зарегистрирован как
  `server@1.1.6`, `linux/arm64`; registry показывает точный digest выше.
  Candidate остаётся untrusted, Project binding и runtime не менялись.
- Следующий live gate — exact trust/launch через Studio, заменить только Server
  в Project `Server Echo Studio` / target `local`, проверить readiness, `/echo`
  и unknown-path 404 через curl, затем проверить Echo logs. Core, Echo, БД,
  credentials и certificates не затрагивать.

- Через текущий UI Server Editor в Project `Server Echo Studio` выбран live
  method Echo `echo.handle`, добавлен `GET /echo-ui-probe` в общий draft,
  сохранён в Project и применён в Core. `curl` вернул
  `echo: ui-draft-acceptance` / HTTP 200; Echo logs, открытые кнопкой Logs в
  Canvas inspector, содержат точный текст запроса.
- Временный route удалён кнопкой редактора и сохранён; Project содержит только
  исходный `echo-route`. Core desired/applied generation совпали на 42. Однако
  старый Server binary возвращал HTTP 200 с пустым body для неизвестного пути.
- Исправлен Caddy compiler: каждый listener получил terminal catch-all
  `static_response` 404 после configured routes и TLS hostname fallback.
  `TestCompileSettingsReturns404ForUnmatchedRequests` проходит и дополнительно
  вызывает Caddy `Validate` для итоговой конфигурации.
- Canonical Server docs фиксируют 404 для unmatched HTTP route. Подготовлен
  Server `1.1.6` Linux/arm64 candidate с catch-all 404 и JSON-value annotation:
  `/var/folders/qk/694xx2l56_l01zmd37h82szh0000gn/T/liapoldus-server-configfix.XXXXXX.4UyCSUABmN/server`, SHA-256
  `264f144bbead45926cd0e65a1acaa6d0e9587ad279a2750d280c100b891e411b`.
  ELF format/platform подтверждены; candidate не регистрировался, не доверялся
  и не запускался. Live Docker 404 acceptance и Core-backed schema refresh
  остаются открытыми.

## Generic Studio JSON value annotation — 2026-10-16

- `apiResponse.example` принимает любое JSON-значение, но schema была пустой и
  generic Studio inspector fail-closed скрывал весь `config.routes`. Добавлена
  UI-only `x-studio-json-value`; Server schema generator и AJV test helper
  обновлены. `apiRequestBody.schema` и `apiResponse.schema` остаются отдельными
  object-документами `x-studio-json-document`.
- Studio host валидирует и распаковывает помеченный JSON value envelope в draft;
  служебный ключ не записывается в canonical settings. Server contract package
  tests и `cmd/contracts --check` прошли. Полное acceptance редактора выполняется
  через Studio позже; runtime schema semantics не менялись.

## Технический settings slice — 2026-10-09

По явному разрешению пользователя выполнена ограниченная миграция внутренних
settings definitions с сохранением v1 behavior, без разморозки product scope.
Typed Go constants в `internal/domain/models/settings_definition.go` задают
точные значения envelope: `1`, `schemaVersion`, `config`. Все шесть production/
fixture consumers используют один `DecodeSettings(contents)`; runtime не
извлекает эти значения из public manifest. Удалены `contracts.Load()`, типы
`contracts.Plugin`/`ConfigField`, поле `Adapter.contract` и три параметра
decoder, которые каждый consumer всегда задавал одинаково.

Тип `SettingsSecretPurpose` и константы REST adapter сохраняют точные значения
`server.tls.certificate`, `server.tls.private-key`, `server.upstream.ca`.
SDK получает те же строки; settings adapter и два SDK/Caddy fixtures
мигрированы. После consumer audit удалены только
`contracts/secret_purposes.go` (тип `SecretPurposes` и `LoadSecretPurposes()`)
и внутренний `contracts/v1/secret-purposes.json`. Значения сохраняются в Go;
tracked исходники также восстанавливаются из Git history.

Native regressions прошли сначала против старых parsers, затем против новых
definitions до удаления. Семь Go tests (включая 15 decoder subtests) фиксируют
шесть точных constants, raw config bytes, error messages/sentinels, revision
replay/conflict, candidate/activation failure и доставку всех трёх grant
purposes. Внутренние settings error messages уже были source-owned в Go и
сохранили строки и identity. Public settings schema/semantics, manifest,
Admin descriptors и site artifacts не менялись; embedded loaders для них
сохранены. Общая миграция contracts в code-first этим срезом не завершена.
Owner AGENTS/docs обновлены только для этого settings/test slice.

`GOWORK=off go test ./... -count=1`, `go build ./...`, `go vet ./...` и
`git diff --check` — PASS. Focused settings/schema/compiler/config/reload
Vitest — 6 файлов / 27 тестов PASS; после миграции grant purposes дополнительный
focused SDK/Caddy/upstream набор — 3 файла / 7 тестов PASS. Пропущенный import
в мигрированном Caddy fixture исправлен; повтор полного Go gate прошёл.
Первый полный Vitest: 32 файла / 81 тест PASS, один standalone build test
завершился timeout 120 s (весь suite: 33 файла / 82 теста). Это не PASS
полного suite; финальный повтор с `GOWORK=off GOFLAGS=-p=1
npm test -- --maxWorkers=1` после освобождения Go build cache прошёл:
33 файла / 82 теста PASS, включая standalone build. Повторная независимая
`GOWORK=off GOFLAGS=-p=1 go build ./...` также прошла. Strict pinned
golangci-lint и typed ESLint не выполнялись: gate OPEN, `go vet` их не заменяет.
Ни другие репозитории, ни исходный site-publication WIP, ни commits/push
не затронуты. Product/v3 задачи ниже остаются вне этого среза.

## Проверка публикации — 2026-10-05

Коммит `8e8b653` опубликован в `origin/main`; hosted Ubuntu verify прошёл,
включая `go build`, `go vet`, `go test` и полный TypeScript suite (30 файлов /
79 тестов, в том числе Pebble/ACME). Локально `GOWORK=off` build/vet прошли,
30 файлов / 79 тестов прошли без локального ACME test: OrbStack показывает
`Running`, но его Docker API socket не отвечает. Server использует SDK
`v1.0.0` и protocol module `/v2 v2.0.0` без локальных `replace`.

## Повторная проверка — 2026-10-04

Текущий worktree прошёл `GOWORK=off npm test -- --maxWorkers=1`
(30 файлов / 79 тестов), `GOWORK=off go test ./...`, `go build ./...`,
`go vet ./...` и `git diff --check`. Это macOS runtime; на дату записи hosted
CI и docs pins оставались открытыми, последующий результат зафиксирован выше
в проверке публикации 2026-10-05.

Linux-проверка 2026-10-04: в Ubuntu 24.04.5 ARM64 VM под OrbStack прошли
полный Server suite (30 файлов / 79 тестов), `go test ./...`, `go build ./...`
и `go vet ./...`; suite включает HTTP/1.1, HTTP/2/3, ACME Pebble, Caddy REST
child process, site artifacts, WebSocket и SSE. Это Linux VM runtime evidence;
на дату snapshot-а hosted CI и docs pins оставались открытыми, оба gate прошли
2026-10-05. Отдельный bare-metal host не требуется для v1.

Дополнение 2026-10-04: `AGENTS.md` теперь различает correlation-only
`Idempotency-Key` у синхронных JSON actions и operation-level dedup для
artifact actions; формулировки тестов settings больше не называют SDK Reload
старым `ConfigApply`. Целевые suites прошли: Admin Surface contract 10/10,
settings compiler/schema 16/16; schema/manifest secret-reference wording uses
Core terminology. Focused settings schema suite passed 13/13; `git diff --check`
прошёл.
Тот же актуальный targeted набор после синхронизации файлов прошёл в OrbStack
Ubuntu 24.04.5 ARM64: 3 файла / 26 тестов.

## Документация

- [x] Server-owned Markdown/examples перенесены в `docs/site/`; общие Core
  страницы принадлежат Core, сайт агрегирует эту документацию по pinned SHA.
- [x] Owner docs опубликованы; pin
  `037dc8e863f28a0e5e78f288ab6776b8dfd4c031` синхронизирован, VitePress
  build/deployment прошли.

## Актуальная проверка — 2026-10-02

Дополнение 2026-10-03: полный `npx vitest run tests --maxWorkers=1` прошёл
(27 файлов / 75 тестов), `go test ./...`, `go build ./...`, `go vet ./...`
и `git diff --check` прошли. Сквозной Core→Server→forms-db Linux/arm64
memory walkthrough завершился с кодом 0; HTTP/2/3 и ACME release gates
ниже остаются открытыми.

Дополнение 2026-10-03: настоящий Caddy runtime успешно получил ACME leaf от
ephemeral Pebble CA, выполнил принудительный CertMagic renewal в том же storage,
после перезапуска загрузил новый serial и предъявил его HTTPS-клиенту с полной
проверкой issuer chain (`tests/acme-lifecycle.test.ts`). Pebble VA пропускает
внешнюю challenge-проверку; публичный CA, DNS и production trust не затрагиваются.
Следующая проверка module registry настоящей Server-сборки подтвердила, что
`dns.providers` пуст: DNS-01 modules в v1 binary не включены. Это закреплено в
`tests/dns-provider-inventory.test.ts` и Server docs. После неё полный
`npm test -- --maxWorkers=1` прошёл (30 файлов / 78 тестов), а также
`GOWORK=off go test ./...`, `GOWORK=off go build ./...`,
`GOWORK=off go vet ./...`, Linux/amd64 и Linux/arm64 cross-builds и
`git diff --check`. Native Linux runtime и общая release matrix остаются
открытыми.

`GOFLAGS=-p=1 npx vitest run tests --maxWorkers=1` прошёл (26 файлов / 73
теста); `GOWORK=off GOFLAGS=-p=1 go test -p=1 ./...`, `go build -p=1 ./...`,
`go vet -p=1 ./...` и `git diff --check` прошли. В текущем продолжении отдельно
прошли `caddy-rest-process`, custom TLS и HTTP/2+HTTP/3 child-process tests
(3/3), затем полный Server suite и Go test/build/vet. Сериализация сборки нужна для ограничения
пикового использования диска при компиляции fixture binaries. Core→SDK→реальный
Server child-process artifact publish, durable operation, serving и restart
тоже прошли на macOS. HTTP stream limits и WebSocket accept/reject, subprotocol,
message boundaries и oversized close проверены сквозными Caddy→mTLS peer
fixtures; WebSocket в v1 ограничен RFC 6455 upgrade по HTTP/1.1. Этот plugin
ещё не release-ready: часть cross-platform и security gates открыта. Read-only
certificate list/get реализованы через Admin Surface; custom TLS certificate
проверяется реальным Caddy runtime fixture. Ручные renew/revoke capabilities
удалены из v1; автоматическое продление остаётся ответственностью Caddy/CertMagic.
Полный тестовый прогон 2026-10-02 также подтвердил peer session reset после
ошибки и повторное подключение к вручную перезапущенному forms-db без replay.
Последний повтор после закрытия test-fixture security gaps: `npx vitest run
tests --maxWorkers=1` — 26 файлов / 73 теста; `GOWORK=off go test -p 1 ./...`,
`go build -p 1 ./...`, `go vet -p 1 ./...` и `git diff --check` — PASS.
SDK lifecycle logs и Prometheus metrics не содержат custom certificate/private
key PEM, secret references или grant handles. Server test fixtures используют
loopback-only ACME authority и проверяют удаление собственного временного data
root.

Нормативная цель: [Core target](https://liapoldus.github.io/core/architecture/target),
[v1 acceptance](https://liapoldus.github.io/core/configuration/acceptance) и
[Server contract](https://liapoldus.github.io/services/server). Агентное задание:
[задачи по субъектам v2](../../tasks/README.md).
Этот репозиторий — отдельный HTTP Server service; `server` — его product/API
identity, Caddy — реализация внутри binary.

## Цель и фиксированные ограничения

- Оператор вручную устанавливает и запускает Server service. Core не владеет
  процессом, не может запускать/останавливать/перезапускать/масштабировать или
  удалять его.
- Public v1 surface: HTTP/1.1, HTTP/2, HTTP/3, TLS/ACME, static sites,
  reverse proxy, SSE и service dispatch; WebSocket поддерживает RFC 6455 over
  HTTP/1.1. Caddy-L4/public TCP/UDP
  listeners/relay/P2P/NAT traversal — v3, не binary/schema/API/conformance v1.
- HTTP/3 остаётся HTTP. Core содержит только Management/control plane и не
  содержит Caddy runtime/data plane.
- Core↔service lifecycle/config — Service SDK REST + unique per-replica mTLS:
  `Reload`, service-initiated exact generation pull, local validate/apply и
  digest ACK. `wool` — только generic service↔service API.
- Settings schema, site archive/manifest, product errors, capabilities и Admin
  Surface принадлежат только этому repo. Общие settings редактируются через
  Core generic API; product Admin Surface не заменяет его control RPCs.
- ACME issuance/automatic renewal — Caddy/CertMagic; ручные renew/revoke API
  отсутствуют в v1; custom certificates задаются
  secret references. Внешний Admin API Caddy не публикуется. Persistence
  сертификатов и release artifacts принадлежит Server service.
- `current`/`previous` сохраняются; site release immutable и переключается
  атомарно после успешной validation/operation.

## Что уже зафиксировано контрактами

- [x] В public service identity, contracts, CLI/management naming использовать
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

- [x] Перенести production `cmd/server` startup, manifest/schema,
  health/readiness, settings pull/apply и ACK на Plugin SDK; сохранить real
  HTTP child process и private mTLS; доказать сквозным smoke отдельно.
- [x] Удалить Server-owned legacy `pluginprotocol` ConfigSchema/ConfigApply/
  Shutdown/Bootstrap service и заменить generic HTTP dispatch на generic peer
  Call. Не оставлять dual API/fallback.
- [x] Удалить legacy lifecycle imports/API. Оставить `pluginprotocol` только
  для generic plugin↔plugin peer calls; эту зависимость не удалять.
- [x] Подключить operator-declared per-replica mTLS и scoped grants для
  certificate/private-key references. Отсутствующий/неверный grant закрывает
  активацию; secrets не читаются из environment или произвольных путей.
- [x] Config reload failure safety: `tests/plugin-sdk-reload.test.ts`
  отправляет schema-valid candidate, который отклоняется при runtime activation;
  проверяет, что generation/revision остаётся `generation-1`, replica сообщает
  `notReady`, а Caddy сохраняет last-good runtime и реальный HTTP listener
  продолжает отдавать прежний ответ. Core видит drift и не объявляет replica
  подтверждённой для нового active generation. Целевой тест проходит на SDK
  v1 lifecycle.
- [x] Settings schema ограничивает route invocation mode значениями,
  поддерживаемыми Server. Проверка наличия product method на remote peer не
  выполняется при activation: централизованный capability registry и
  `DispatchApply` не входят в v1. Доступность метода и его caller-owned
  authorization проверяются generic peer-вызовом при dispatch.
- [x] После потери peer-сессии не воспроизводить текущий вызов; сбросить
  соединение и подключиться заново на следующем независимом вызове. Проверено
  сквозным Core→Server→forms-db child-process сценарием с ручным restart
  forms-db и повторным чтением через Server route.

## P1 — HTTP data plane, TLS и policies

- [x] Запустить HTTP/1.1, HTTP/2 и HTTP/3 через реальный Server binary.
  `tests/server-binary-http-protocols.test.ts` прошло на macOS; 2026-10-04
  `go run ./tests/fixtures/caddy-rest-process` также прошло в Linux/arm64
  `golang:1.26` container и вернуло ожидаемые ALPN `http/1.1`, `h2`, `h3`,
  status 200 и опубликованное содержимое. Linux VM runtime matrix прошла в
  OrbStack 2026-10-04.
  Redirect method/query semantics и TLS 1.2–1.3 bounds проверяются
  `tests/settings-vectors-runtime.test.ts`, `tests/settings-schema.test.ts` и
  `tests/custom-tls.test.ts`; HTTP/3/TLS 1.2 rejection —
  `tests/http2-http3.test.ts`; certificate refs и ACME ownership —
  `tests/config-apply.test.ts` и `tests/acme-lifecycle.test.ts`.
- [x] Проверить host/path normalization, exact/prefix/glob/RE2 semantics,
  first-match ordering, canonical-equivalent matcher rejection, methods,
  percent-decoding, dot segments, UTF-8/control/NUL handling и canonical static
  path. Исполняемая проверка настоящего Caddy runtime:
  `tests/route-matching.test.ts`; archive/site path traversal отказ закреплён
  в `tests/site-archive-runtime.test.ts` и `tests/site-manifest-runtime.test.ts`.
- [x] Reverse proxy использует 1–32 origins, weighted smooth round-robin
  (weight 1..100), обязательную upstream TLS verification, optional CA
  reference, cooldown после connect refusal и максимум один connect-before-bytes
  retry; после отправки request bytes request не повторяется. Настоящий runtime
  проходит `tests/upstream-pools.test.ts`; Settings schema ограничивает pool
  size и веса.
- [x] Unary `call` и базовый двунаправленный `http_stream` проходят через
  реальный Caddy runtime и child-process plugin; chunked body свыше лимита
  отклоняется `413` до response-start. Проверка: `tests/http-dispatch.test.ts`.
- [x] `http_stream` не наследует unary timeout; отдельные route/instance
  concurrency limits, idle timeout 60 s, max duration 1 h и timeout response
  status закреплены в `contracts/v1/http-dispatch.json` и `settings.schema.json`.
  Default concurrency — 128 на route и instance; явные route limits могут быть
  только ниже. Сквозной тест `tests/http-stream-limits-runtime.test.ts`
  проверяет длительность сверх unary timeout, saturation, idle close и max
  duration close.
- [x] Проверить oversized response frame до и после `response_start`, а также
  ограниченную peer send queue при медленном HTTP consumer. До начала ответа
  oversized frame получает обычную ошибку; после начала сохраняются status и
  уже записанные bytes, а поток закрывается. `ErrSendQueueFull` не вызывает
  неограниченное буферирование; HTTP-side producer повторяет отправку только
  пока stream активен. Проверка: `tests/http-stream-limits-runtime.test.ts`.
- [x] SSE event/data/id/retry serialization: `sse_event` валидируется до
  сериализации; Caddy устанавливает `text/event-stream`, сохраняет границы
  событий, нормализует многострочный data и fail-closes invalid events после
  response-start. Проверено `tests/http-stream-limits-runtime.test.ts`.
- [x] WebSocket handshake accept/reject до `101`, выбор только предложенного
  subprotocol, text/binary boundaries, plugin close и 1 MiB message bound
  проверены настоящим Caddy→mTLS peer child process в
  `tests/websocket-dispatch-runtime.test.ts`, включая отказ не предложенного
  subprotocol и close `1009` для oversized message. Ping/pong ведёт Server.
  V1 поддерживает RFC 6455 upgrade поверх HTTP/1.1; HTTP/2 Extended CONNECT и
  WebSocket over HTTP/3 не входят в контракт. Handshake/framing описаны в
  `contracts/v1/http-stream.json`.
- [x] Ограничить агрегированный размер HTTP request headers до 64 KiB; лишние
  заголовки получают 431 до вызова capability. Caddy parser имеет собственный
  bounded read limit, а Liapoldus handler проверяет точный размер нормализованных
  header fields. Реальная Caddy HTTP/1.1 проверка на точной границе и на один
  байт выше: `tests/header-limits.test.ts`.
- [x] Проверить HTTP body, response, concurrency, idle и duration limits на
  runtime пути. Request body limit 1 MiB считает фактически полученные байты,
  включая chunked; stream defaults — 128 одновременных вызовов на instance,
  idle 60 s и max duration 1 h. Проверены request overflow до/после начала
  ответа, stream frame overflow, bounded peer queue/backpressure, idle и duration
  через `tests/http-dispatch.test.ts` и `tests/http-stream-limits-runtime.test.ts`.
  Это функциональный conformance, не отдельный длительный нагрузочный benchmark.
- [x] Route-scoped incoming cookie allow-list, typed ordinary/HttpOnly response
  actions и атомарный отказ до headers/101 реализованы. Проверки: settings schema,
  unary/HTTP-stream runtime и WebSocket child-process tests; неразрешённые cookies
  не попадают в plugin context, raw `Cookie` и generic `Set-Cookie` headers
  заблокированы.
- [x] SDK lifecycle structured logs и Prometheus metrics не содержат custom
  certificate/private-key PEM, opaque secret references или grant handles;
  это проверяет настоящий Caddy/SDK mTLS Reload child-process fixture, включая
  отказ runtime candidate (`tests/plugin-sdk-reload.test.ts`).
- [x] Проверить redaction входящих cookie values на реальном Core→Server→forms-db
  пути: allow-listed и неразрешённое значения отправляются в одном HTTP-запросе;
  тест подтверждает их отсутствие в stdout/stderr Server и forms-db, логах Core,
  SQLite/WAL/SHM и полях Core audit. Общая allow-list/runtime проверка Server
  остаётся отдельно в `tests/http-dispatch.test.ts`. Evidence:
  `core/tests/integration/manual-core-server.test.ts` и
  `core/tests/integration/manual-core-server-sql.test.ts` (PostgreSQL 16,
  MySQL 8.0, MariaDB 11.4, 3/3). Product consumer проверяет, что
  transport-envelope с cookie metadata принимает только body как forms data:
  `tests/integration/peer-dispatch.test.ts`.
- [x] Подтвердить независимость активации от выпуска: реальный Caddy runtime
  активирует automatic-TLS listener с недоступным loopback test ACME authority,
  сообщает активную revision и отдельно возвращает сертификат как `pending`.
  Проверено `tests/config-apply.test.ts` без публичного CA.
- [x] Проверить успешную ACME issuance и renewal через настоящий Caddy runtime
  с ephemeral Pebble CA (`tests/acme-lifecycle.test.ts`): Caddy получает leaf,
  CertMagic принудительно выполняет renewal в том же persistent storage, после
  перезапуска Caddy предъявляет новый leaf HTTPS-клиенту, проверяющему цепочку
  Pebble issuer CA. Pebble VA в этом тесте настроен на успешную валидацию без
  внешнего DNS/публичного CA; production settings/trust не меняются.
- [x] Инвентаризировать DNS provider modules в фактическом Caddy module
  registry production Server binary: список пуст. `tests/dns-provider-inventory.test.ts`
  запускает настоящий module registry с теми же импортами, что и `cmd/server`,
  и требует ноль `dns.providers`. Текущая v1-сборка поэтому поддерживает ACME
  HTTP-01 и TLS-ALPN-01, но не DNS-01; это также указано в Server docs.
- [x] Реализовать read-only `server.certificates.list` и
  `server.certificates.get`: показывать только домены active TLS settings,
  источник (`acme`/`custom`), readiness и публичные leaf metadata; private key,
  PEM и внутренние storage paths не возвращаются. Runtime fixture покрывает
  custom certificate, неизвестный домен и redaction; успешная ACME issuance и
  renewal проходят `tests/acme-lifecycle.test.ts`. Ручной renew/revoke не входят
  в v1.

## P2 — sites и plugin admin actions

- [x] Прогнать site publish multipart metadata + один `.tar.gz`, durable `202`
  operation, idempotency, expected-current CAS, streaming digest и bounded
  memory. Обязателен корневой `site-manifest.json` с schemaVersion, siteId,
  documentRoot и indexDocument. Реальный тест: `tests/site-publish-runtime.test.ts`
  (первая публикация, idempotency/replay, конфликт ключа, CAS, digest и
  duplicate metadata, restart recovery, post-switch recovery, serving и rollback).
  Принятие завершается после fsync ограниченного artifact и совпадения SHA-256;
  gzip/tar/manifest validation выполняется durable worker-ом. Child-process
  сценарий также подтверждает, что невалидный архив получает `202`, затем
  `failed/operation_failed`, не меняя `current`.
- [x] Разделить семантику Admin Action и artifact operation: у синхронного
  `server.sites.rollback` повтор с тем же `Idempotency-Key` вызывает действие
  заново; повтор со старым `expectedCurrentRevision` получает CAS `conflict`,
  а не сохранённый первый результат. Ключ — только correlation metadata.
  У `server.sites.publish` остаётся operation-level dedup: тот же ключ и тот же
  artifact возвращают существующую durable operation; иной input конфликтует.
  Проверено контрактным тестом и runtime `tests/site-publish-runtime.test.ts`.
- [x] Проверить пределы: 128 MiB compressed artifact, 64 KiB metadata, 64 KiB
  multipart overhead (request envelope ≤128 MiB +128 KiB), expanded ≤512 MiB,
  max 10,000 logical entries, ratio ≤100:1. Archive/manifest failures
  завершают operation до pointer activation; невалидная длина или digest
  отклоняются до durable acceptance. Проверяются traversal/absolute paths,
  symlinks/hardlinks, special files, duplicate/case/NFC collisions, corrupt or
  truncated gzip/tar. Исполняемые archive scenarios:
  `tests/site-archive-runtime.test.ts`; multipart/operation scenarios:
  `tests/site-publish-runtime.test.ts`.
- [x] Проверить immutable revisions и атомарную current/previous swap; failed
  operation и crash/restart до/после pointer switch не повреждают serving tree.
  `tests/site-publish-runtime.test.ts` подтверждает digest-addressed revisions,
  rollback/CAS, restart до activation и recovery после durable pointer switch;
  `tests/site-publish-runtime.test.ts` также проверяет сохранность serving root.
- [x] Убрать пустой site-journal polling каждые 100 мс: worker теперь
  пробуждается после durable acceptance и обрабатывает весь журнал одним
  проходом; startup recovery и синхронный rollback сохранены. Regression test
  подтверждает отсутствие фоновых lock-вызовов без принятой публикации и
  обработку принятой операции. Проверены `go test ./...`; оставлена runtime
  проверка нового контейнера Server перед подтверждением отсутствия повторных
  Caddy lock-maintenance ошибок.
- [x] Admin Surface предоставляет plugin-owned certificate list/status,
  site/release/operation actions и status. Ручные renew/revoke не входят в v1.
  Любые common settings или
  Core-wide service keys не принимаются Caddy-specific endpoints.

## P3 — final integration/removal

- [x] Реальный ручной процесс smoke: Core→SDK REST/mTLS→Reload→exact pull→apply
  →ACK, HTTP request проходит новым runtime generation, rollback/Server restart
  сохраняют ожидаемое состояние. Отдельный Core-process restart теперь также
  проверен: после рестарта status сходится к `ready` и `drift=false`.
- [x] Проверить прямой HTTP→plugin peer dispatch без Core traffic proxy и
  protocol product methods: настоящий Core→Server→forms-db child-process E2E
  проходит submit/list, а Core не переносит request payload. Вызов защищён
  generic `pluginprotocol` mTLS; HTTP route использует только объявленный peer
  method. Проверено Core и forms-db integration suites.
- [x] Проверить, что Caddy Admin API отключён: real Caddy/SDK child-process
  fixture запускается с занятым адресом, указанным через Caddy's
  `CADDY_ADMIN`; runtime activation succeeds, а Caddy сообщает `admin endpoint
  disabled` (`tests/plugin-sdk-reload.test.ts`). Server не публикует этот API.
- [x] Отсутствие Caddy-L4 и legacy public TCP/UDP listeners/relay закреплено
  runtime/import-surface checks (`tests/v1-scope.test.ts`). В v1 Server имеет
  один вручную объявленный peer target; fan-out/pool lifecycle здесь не
  заявляется и не входит в зафиксированный v1 traffic topology.
- [x] Проверить import/runtime surface для v1 L4: production `contracts/`,
  `internal/`, `cmd/` и `go.mod` не содержат Caddy-L4, TCP/UDP listener/relay
  ветвей или активных L4 vectors. Negative assertions в
  `tests/v1-scope.test.ts` и `tests/settings-schema.test.ts` намеренно остаются:
  они закрепляют запрет v1, а не мёртвую реализацию.
- [x] Проверить публичные Admin Surface/API identifiers на Caddy-named aliases:
  `contracts/v1/admin-actions.json` объявляет только product IDs `server.*`
  (`server.certificates.*`, `server.sites.*`, `server.operations.*`); Caddy
  остаётся внутренним technology/implementation name.
- [x] Прогнать полный текущий-host suite: `npm test` (27 файлов / 75 tests),
  `go test ./...`, `go build ./...`, `go vet ./...` и `git diff --check` — все
  проходят на macOS после добавления ACME-pending lifecycle test и
  детерминизации stream overflow fixture.
- [x] Linux runtime matrix пройдена в Ubuntu 24.04.5 ARM64 VM под OrbStack;
  полный Core→Server→forms-db production walkthrough прошёл на memory и на
  PostgreSQL 16, MySQL 8.0, MariaDB 11.4. Server HTTP/1.1–3, ACME Pebble,
  site publish, WebSocket и SSE также проверены в Linux guest.
- [x] Release matrix закрыта: hosted Server CI (включая Pebble/ACME) и Core
  cross-repository integration прошли; docs pin опубликован; tag `v1.0.1`
  создан после gates. Bare-metal Linux host не является отдельным v1 gate.
- [x] Изолировать Caddy runtime fixtures от пользовательского app data: общий
  helper задаёт временные XDG data/config paths и переназначает Caddy globals,
  которые инициализируются при import до `main`. Все 10 runtime fixtures
  используют helper; тест проверяет `DefaultStorage`, loopback-only ACME test
  authority и отсутствие временного root после cleanup. Cleanup ждёт завершения
  фоновой storage-cleanup работы Caddy; fixtures не пишут в пользовательский
  `Application Support/Caddy` и не оставляют свои temp roots.

## V3 — весь дальнейший Server development

Server не является активным v2 субъектом. Текущая v1 реализация остаётся
неизменённым regression baseline; generic Core/SDK v2 lifecycle и rollout
проверяются на fixtures. Нижеследующие ранее записанные v2 задачи перенесены в
v3 и не являются текущими v2 обязательствами.

Повторная проверка текущего worktree 2026-10-06: реальные child-process
publish/recovery сценарии выше прошли 2/2 на macOS. Полный `GOWORK=off
GOTOOLCHAIN=go1.26.0 npm test -- --maxWorkers=1` прошёл (33 файла / 82 теста),
`go test ./...`, `go build ./...`, `go vet ./...` и `git diff --check` прошли.
Это не закрывает отложенный v3 gate: тесты используют общую локальную FS и
не устанавливают поддержку какого-либо RWX/network filesystem для нескольких
host-ов.

- [x] Сериализовать durable site-release mutations между независимыми Server
  process через стабильный OS advisory lock (`gofrs/flock`): publish, recovery worker и
  rollback используют один coordination lock, поэтому pending-operation check,
  orphan-staging cleanup и `current`/`previous` switch не расходятся между
  process-local mutex-ами. Проверено отдельным TypeScript-driven test, который
  запускает два child process на общем локальном каталоге одновременно и
  требует ровно один accepted publish плюс один conflict:
  `npx vitest run tests/site-publish-cross-process.test.ts --maxWorkers=1`.
  Отдельный child-process test принудительно завершает принимающий процесс
  сразу после durable acceptance, затем одновременно запускает recovery в двух
  независимых Server-процессах на том же storage. Оба процесса завершаются без
  ошибки и наблюдают одну completed operation, одинаковый active revision и
  одинаковый served document:
  `npx vitest run tests/site-publish-cross-process-recovery.test.ts --maxWorkers=1`.
  Это доказывает только межпроцессную координацию на тестовой локальной FS; ни
  один RWX/network filesystem profile ещё не объявлен поддерживаемым.
- [ ] Саморегистрация каждой replica через Service SDK; SemVer/digest и
  совместимость shared state/HTTP/peer contracts. Gate: Core restart, lease
  expiry, two-cohort rolling/canary, ingress sticky и bounded stream drain.
- [ ] Multi-replica site/ACME storage на общем RWX filesystem. Зафиксировать
  конкретный поддерживаемый profile только после conformance atomic locking,
  concurrent ACME, immutable publish/rollback, failure и crash recovery.
  Несовместимые storage layouts не допускать к смешанному rollout.

### Дополнительные Server возможности v3

- [ ] Caddy-L4 и public TCP/UDP relay за Server-owned контрактами и отдельным
  real-runtime conformance. P2P/NAT traversal не подразумевается обычным relay.
- [ ] CAPTCHA/Identity integration — только после явной разморозки владельцев,
  без специальных Core/product методов в SDK или `wool`.
Установку и плановые обновления Server выполняет оператор выбранными средствами; Core никогда не управляет workloads или числом
replicas. Delivery/provider API не является задачей Server/Core v3.

Не добавлять v2/v3 API/dependencies в v1 binary или v1 test matrix.


## Config preflight endpoint — 2026-10-17

- Server settings now expose a pure service-owned candidate validator; invalid route configurations are rejected before activation. Service release candidate is 1.1.8 Linux/arm64, SHA-256 `befcb98b28424b04682ff730f6d0da9b7bf7f2d8222fdb42e9dfc294407d28b1`; full Go tests, vet and build passed.
- The current local Project still pins 1.1.7. Installing/trusting/running 1.1.8 and publishing this owner change await the exact local digest approval and commit/push.

# AGENTS.md — Server service

Runtime entities называются Services; StudioPlugins — только расширения Studio.
Control plane: Studio → CLI → Core → Service SDK REST; data plane:
независимые сервисы → Wool. Modules: `github.com/Liapoldus/service-sdk/v2`,
`github.com/Liapoldus/wool/v3`; локальный Server — `liapoldus.local/server-service`.
Старые runtime names, HTTP paths и CLI commands не поддерживаются.
Нейтральные IDs/application methods и точные immutable generation bytes/digests
сохраняются. Исторические результаты проверок не переименовываются.

## Назначение

`services/server/` — отдельный product service с Caddy внутри собственного
binary. Он реализует публичный HTTP/HTTPS data plane; Core не содержит его
runtime. В v1 поддерживаются HTTP/1.1, HTTP/2, HTTP/3, TLS/ACME, static sites,
reverse proxy, WebSocket/SSE и согласованный service dispatch.

## Жёсткие границы v1

- В v1 нет Caddy-L4, public TCP/UDP listeners/relay, P2P/NAT traversal,
  embedded Caddy в Core, публичного Caddy Admin API, TUF/install,
  process/container orchestration, CAPTCHA, Identity/OIDC/OAuth или `tls-issuer`.
- HTTP/3 — HTTP surface и не разрешает generic UDP relay.
- Оператор сам устанавливает и запускает Core и Server service. Core не имеет
  права устанавливать, стартовать, останавливать, рестартовать, масштабировать
  или удалять Server process/container.
- В v2 Core также не управляет workload или replica count, но `services/server/`
  не входит в активную v2 разработку. Текущая Server v1 реализация остаётся
  regression baseline; generic Core/SDK rollout проверяется на fixtures.
- В v3 возобновляется весь будущий Server scope: replica integration,
  multi-replica storage/ACME и conformance, Caddy-L4/public TCP/UDP.
  Установка и плановые обновления во всех версиях принадлежат оператору;
  Core не получает provider API.
- Явно запрошенная техническая миграция внутренних settings definitions
  разрешена с сохранением всех v1 semantics; это не разморозка product scope.
  Envelope constants принадлежат `internal/domain/models/settings_definition.go`.
  Grant-purpose constants принадлежат
  `internal/presentation/restservice/settings_secret_purposes.go`; SDK получает
  их точные string values, а не результат загрузки внутреннего JSON asset.
  Public settings schemas/semantics, Admin descriptors и site artifacts в этот
  срез не входят; runtime не извлекает имена полей envelope из manifest-файла.
- Core↔service lifecycle/config — только Service SDK REST + per-replica mTLS:
  `Reload(generation)`, service pull точных bytes, локальная validate/apply и
  ACK. Нельзя поддерживать product lifecycle через `wool`.
- Server composition берёт `GET /healthz` и `GET /metrics` только из Service SDK
  `HandlerSet`; публичный listener монтирует SDK `NewObservabilityHandler`.
  Не добавлять собственные handlers, пути, метрики health или response shapes.
- `wool` — только generic service↔service communication. Server не
  добавляет туда HTTP/Caddy capability contracts или lifecycle methods.
- Server владеет собственными `contracts/v1/`, settings schema, site manifest,
  capabilities, errors и Admin Surface data/actions. Общие Core API и SDK REST
  shapes не дублируются здесь.
- Caddy Admin API не публикуется и не обходится через service Admin Surface.
  ACME/renewal принадлежит Caddy/CertMagic; private certificate/site state
  размещён на persistent storage Server service. Секреты только references и
  scoped grants; не помещать plaintext secrets в env/argv/config/logs/errors.

## Код и архитектура

- Держать ровно четыре слоя `domain/`, `application/`, `infrastructure/`,
  `presentation/`; зависимости направлять внутрь. Не помещать Caddy types в
  domain/application/public Management API.
- Конфигурация — строгий Server-owned JSON contract. Caddy runtime config
  строится только внутри service. Candidate validation/adaptation/application
  атомарны: отказ оставляет прошлый рабочий snapshot.
- Site releases принадлежат Server service и используют immutable release
  directories с атомарными `current`/`previous`. Архив полностью проверяется
  до activation; путь и записи не могут escape release root.
- Любой Admin Action/auth operation имеет bounded input и audit context по
  владельцу API. Для синхронного JSON Admin Action `Idempotency-Key` обязателен,
  но служит только correlation metadata: повторный принятый запрос вызывает
  действие заново и не обещает exactly-once effects. Durable artifact actions
  используют operation-level idempotency своего versioned contract. Не
  раскрывать cookies, authorization, credentials, private keys, grants или file
  paths.
- Product-specific strings and schemas имеют один owner contract; не копировать
  API docs/contracts в другой репозиторий как второй источник истины.

## Документация

- Каноническая архитектура, settings/runtime guide и примеры Server service
  хранятся в `docs/site/`; Mermaid исходники принадлежат этому репозиторию.
  Единый VitePress сайт синхронизирует закреплённый commit и сохраняет
  публичные URL. Не редактировать generated copy в `liapoldus.github.io`.
- Общие Core lifecycle и API описывает Core; этот репозиторий описывает только
  Server-owned product behavior и ссылается на опубликованные Core pages.

## Одобренная доработка конфигуратора Studio

План `studio/plan.md` от 2026-10-13 явно включает Server settings editor и
типизированные HTTP request/response contracts. Это точечное разрешение
изменять Server-owned schema, code-first contract, validation и runtime только
для описанного API-конфигуратора; оно supersedes прежнее ограничение на
settings-schema changes в рамках этого конкретного продукта. Не расширять его
на Caddy-L4, CAPTCHA/Identity, внешний deployment или Core orchestration.

Query/path/header/body/response definitions и их JSON Schema принадлежат только
Server. Они должны валидироваться до dispatch, иметь bounded inputs и сохранять
атомарное применение поколения. Studio получает schema/UI metadata через
Core-backed CLI; Server и Echo остаются независимыми и знают друг о друге только
через собственные opaque peer contracts/Wool. Добавлять совместимость только
если она явно входит в новый versioned Server contract; не оставлять старую и
новую route модель параллельно.

Service endpoints используют только `server-http/v1`: method и template path
задаются в `route.api`, typed path/query/header параметрами, optional JSON body
schema и documented response schemas/examples. Для service route поля path и
methods запрещены в `route.match`; старые untyped service routes отклоняются.
Server передаёт нормализованные значения в `parameters` согласно
`contracts/v1/http-request-parameters.schema.json`; unknown query/headers
разрешены по умолчанию, каждая повторная семантика задана явно. Response schemas
проверяют examples при candidate apply, но не валидируют живой ответ.
Специализированный Studio editor принадлежит только
`plugins/studio-server/`; не переносить его в `services/server/`.

## Изменения и проверки

- Перед работой снять `git status --short`, branch/HEAD/remotes; посмотреть diff
  пересекающихся dirty/untracked файлов. Не очищать WIP другого агента.
- Детерминированные native Go unit/regression tests держать рядом с кодом в
  `*_test.go`; TypeScript child-process/E2E tests и fixtures — под `tests/`.
  Перед удалением file parser зафиксировать точные значения и существующие
  error messages/semantics native regression tests. Использовать штатные Vitest
  и Go test workflow этого репозитория.
- Для lifecycle/security assertions использовать настоящий child process и
  mTLS, а не только unit mock. Тестировать реальный HTTP client/data plane.
- Перед handoff выполнить полный `npx vitest run tests`, `go test ./...` (если
  Go tests присутствуют), `go build ./...`, `go vet ./...` и применимые schema,
  cross-platform, SDK integration gates. Называть PASS только исполненную
  проверку; contract-only тест не заменяет runtime test.
- Обновлять `TODO.md` по проверенным фактам. Не делать push/release и не
  назначать неизвестный remote.

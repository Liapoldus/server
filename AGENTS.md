# AGENTS.md — Server plugin

## Назначение

`plugins/server/` — отдельный product plugin с Caddy внутри собственного
binary. Он реализует публичный HTTP/HTTPS data plane; Core не содержит его
runtime. В v1 поддерживаются HTTP/1.1, HTTP/2, HTTP/3, TLS/ACME, static sites,
reverse proxy, WebSocket/SSE и согласованный plugin dispatch.

## Жёсткие границы v1

- В v1 нет Caddy-L4, public TCP/UDP listeners/relay, P2P/NAT traversal,
  embedded Caddy в Core, публичного Caddy Admin API, TUF/install,
  process/container orchestration, CAPTCHA, Identity/OIDC/OAuth или `tls-issuer`.
- HTTP/3 — HTTP surface и не разрешает generic UDP relay.
- Оператор сам устанавливает и запускает Core и Server plugin. Core не имеет
  права устанавливать, стартовать, останавливать, рестартовать, масштабировать
  или удалять Server process/container.
- Core↔plugin lifecycle/config — только Plugin SDK REST + per-replica mTLS:
  `Reload(generation)`, plugin pull точных bytes, локальная validate/apply и
  ACK. Нельзя поддерживать product lifecycle через `pluginprotocol`.
- `pluginprotocol` — только generic plugin↔plugin communication. Server не
  добавляет туда HTTP/Caddy capability contracts или lifecycle methods.
- Server владеет собственными `contracts/v1/`, settings schema, site manifest,
  capabilities, errors и Admin Surface data/actions. Общие Core API и SDK REST
  shapes не дублируются здесь.
- Caddy Admin API не публикуется и не обходится через plugin Admin Surface.
  ACME/renewal принадлежит Caddy/CertMagic; private certificate/site state
  размещён на persistent storage Server plugin. Секреты только references и
  scoped grants; не помещать plaintext secrets в env/argv/config/logs/errors.

## Код и архитектура

- Держать ровно четыре слоя `domain/`, `application/`, `infrastructure/`,
  `presentation/`; зависимости направлять внутрь. Не помещать Caddy types в
  domain/application/public Management API.
- Конфигурация — строгий Server-owned JSON contract. Caddy runtime config
  строится только внутри plugin. Candidate validation/adaptation/application
  атомарны: отказ оставляет прошлый рабочий snapshot.
- Site releases принадлежат Server plugin и используют immutable release
  directories с атомарными `current`/`previous`. Архив полностью проверяется
  до activation; путь и записи не могут escape release root.
- Любой Admin Action/auth operation имеет bounded input, idempotency и audit
  context по владельцу API. Не раскрывать cookies, authorization, credentials,
  private keys, grants или file paths.
- Product-specific strings and schemas имеют один owner contract; не копировать
  API docs/contracts в другой репозиторий как второй источник истины.

## Документация

- Каноническая архитектура, settings/runtime guide и примеры Server plugin
  хранятся в `docs/site/`; Mermaid исходники принадлежат этому репозиторию.
  Единый VitePress сайт синхронизирует закреплённый commit и сохраняет
  публичные URL. Не редактировать generated copy в `liapoldus.github.io`.
- Общие Core lifecycle и API описывает Core; этот репозиторий описывает только
  Server-owned product behavior и ссылается на опубликованные Core pages.

## Изменения и проверки

- Перед работой снять `git status --short`, branch/HEAD/remotes; посмотреть diff
  пересекающихся dirty/untracked файлов. Не очищать WIP другого агента.
- Тесты/fixtures держать под `tests/`; не добавлять тесты/fixtures в production
  пакеты. Использовать штатные Vitest и Go test workflow этого репозитория.
- Для lifecycle/security assertions использовать настоящий child process и
  mTLS, а не только unit mock. Тестировать реальный HTTP client/data plane.
- Перед handoff выполнить полный `npx vitest run tests`, `go test ./...` (если
  Go tests присутствуют), `go build ./...`, `go vet ./...` и применимые schema,
  cross-platform, SDK integration gates. Называть PASS только исполненную
  проверку; contract-only тест не заменяет runtime test.
- Обновлять `TODO.md` по проверенным фактам. Не делать push/release и не
  назначать неизвестный remote.

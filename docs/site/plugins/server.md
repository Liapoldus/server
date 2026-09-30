# Server plugin

Server plugin — отдельный сервис на базе Caddy. Он обслуживает пользовательский
HTTP/HTTPS traffic; Core хранит его desired JSON configuration, но не содержит
Caddy runtime и не проксирует запросы. В v1 оператор отдельно устанавливает и
запускает Core, Server plugin и forms-db.

Общий Core↔plugin lifecycle принадлежит Plugin SDK REST: Core вызывает
`Reload(generation)`, plugin сам запрашивает точное поколение и подтверждает
digest после применения. `pluginprotocol` используется только для generic
plugin↔plugin communication.

## Ответственность и состояние

| Область | Владелец |
| --- | --- |
| Desired HTTP settings | Core SQLite хранит исходные JSON bytes с поколениями `active`/`previous`. |
| Settings schema и runtime validation | Server plugin; Core применяет только общий plugin-owned schema validation. |
| HTTP/TLS runtime | Server plugin на Caddy; конфигурация JSON компилируется во внутренний runtime snapshot. |
| ACME и certificates | Server plugin/Caddy; состояние находится в его persistent storage. |
| Static site artifacts | Server plugin; immutable releases с указателями `current`/`previous`. |
| Process lifecycle | Оператор средствами ОС; Core не устанавливает, не запускает, не останавливает и не перезапускает процесс. |

В v1 допускается одна Server plugin replica на Core. Оператор вручную сохраняет
один и тот же persistent directory при обновлении или рестарте. Внешний Caddy
Admin API не публикуется и не является интерфейсом Core.

## HTTP configuration v1

Server plugin принимает только собственный versioned JSON contract, не raw Caddy
JSON и не Caddyfile. Настройки задают public HTTP/HTTPS listeners и ordered
routes. Matcher v1 поддерживает hostname/method и ровно один path режим
`exact`, `prefix`, `glob` или Go RE2 `regex`; совпавший route использует один
terminal handler: static site, HTTP(S) reverse-proxy pool или plugin capability.
Middleware chain и общий `continue` не входят в v1.

Reverse-proxy pool содержит 1–32 HTTP(S) origins с весами 1–100 и smooth
weighted round-robin. Passive connection failure допускает не более одного
retry до отправки request bytes; HTTPS verification обязательна, private CA
задаётся внешним `caRef`. Active health probes и отключение проверки TLS не
поддерживаются.

TLS использует ACME либо custom certificate/key references; private key bytes не
передаются через settings. Валидная конфигурация может активироваться до
завершения ACME issuance; готовность сертификата отслеживается отдельно.
HTTP/2 и HTTP/3 доступны на TLS listener при поддержке сборки Caddy.

Site publication выполняется через generic Admin Surface artifact action и
Plugin SDK. Server plugin проверяет manifest, digest, tar.gz integrity, лимиты
и небезопасные archive entries, затем атомарно активирует immutable release,
сохраняя `previous`.

## Явно вне v1 — v2

- Caddy-L4 и public TCP/UDP listeners/relay.
- CAPTCHA и Identity (OIDC/OAuth) plugins.
- TUF catalog, проверка и установка plugin releases через Core.
- Core-managed local process supervision и Docker/Compose/Swarm/Kubernetes.

Эти возможности не входят в v1 binary, API, SQLite state, documentation gates
или tests. Полная граница версий находится в
[целевой архитектуре Core](../core/architecture/target).

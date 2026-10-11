# Server plugin

Отдельный HTTP/HTTPS plugin service на базе Caddy. Core не содержит Caddy
runtime: оператор независимо устанавливает и запускает оба бинарника, а Core
подключается к фиксированному endpoint Server plugin.

Общий lifecycle работает через Plugin SDK REST: Core вызывает `Reload`, plugin
pull-ит точное поколение конфигурации из Core и подтверждает digest после
успешной атомарной активации. `pluginprotocol` применяется только для
настраиваемой generic plugin↔plugin связи.

Оператор запускает `cmd/server` с обязательными `--instance-id`,
`--replica-id`, `--rest-listen`, `--core-url` и абсолютными путями
`--ca-file`, `--server-cert`, `--server-key`, `--client-cert`,
`--client-key`, `--crl-file`. Проверяемые TLS identity разделены:
`--core-common-name` обозначает Core HTTPS server для config pull,
`--core-client-common-name` — Core mTLS client для входящего `Reload`;
`--core-server-name` задаёт TLS hostname. При необходимости для каждой
роли отдельно задаются `--core-uri` и `--core-client-uri`. Продуктовая
конфигурация не передаётся через CLI или environment.

Server не получает адреса соседних plugins из CLI или environment. Для
аутентифицированных прямых вызовов он использует локальную peer identity
(`--peer-identity`, `--peer-ca-file`, `--peer-cert`, `--peer-key`), а разрешённый
endpoint и ожидаемую identity каждый раз разрешает по peer directory, выданному
Core. Caddy settings содержат только opaque instance ID и capability; трафик
идёт напрямую Server → `pluginprotocol` → plugin без proxy со стороны Core или
CLI. Ни один plugin не требует знать реализации соседнего plugin.

v1 включает HTTP/HTTPS, TLS/ACME, HTTP/2/3, статические сайты и reverse proxy.
Публичный TCP/UDP relay и Caddy-L4 исключены из binary и settings schema и
отложены до v3. CAPTCHA и Identity/OIDC/OAuth также относятся к v3 и
заморожены. Установку и плановые обновления Core и Server выполняет оператор
выбранными средствами; Core не управляет процессами
или контейнерами ни в одной версии.

Полные границы и acceptance см. в
[целевой архитектуре Core](../../liapoldus.github.io/core/architecture/target)
и [TODO](TODO.md).

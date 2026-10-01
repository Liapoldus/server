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

Для прямой связи с другим plugin оператор может дополнительно задать один
peer target: `--peer-target-id`, `--peer-endpoint`, `--peer-identity`,
`--peer-expected-identity`, `--peer-ca-file`, `--peer-cert`, `--peer-key`.
Параметры задаются вместе; сертификат и ключ читаются из абсолютных путей.
Соединение Server→plugin использует взаимный TLS, проверяет peer identity,
а приватный ключ не включается в Caddy runtime JSON.

v1 включает HTTP/HTTPS, TLS/ACME, HTTP/2/3, статические сайты и reverse proxy.
Публичный TCP/UDP relay и Caddy-L4 исключены из binary и settings schema и
отложены до v2. Также в v2 отложены CAPTCHA, Identity/OIDC/OAuth, TUF/install
через Core и управление процессами или контейнерами.

Полные границы и acceptance см. в
[целевой архитектуре Core](../../liapoldus.github.io/core/architecture/target)
и [TODO](TODO.md).

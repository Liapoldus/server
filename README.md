# Server plugin

Отдельный HTTP/HTTPS plugin service на базе Caddy. Core не содержит Caddy
runtime: оператор независимо устанавливает и запускает оба бинарника, а Core
подключается к фиксированному endpoint Server plugin.

Общий lifecycle работает через Plugin SDK REST: Core вызывает `Reload`, plugin
pull-ит точное поколение конфигурации из Core и подтверждает digest после
успешной атомарной активации. `pluginprotocol` применяется только для
настраиваемой generic plugin↔plugin связи.

v1 включает HTTP/HTTPS, TLS/ACME, HTTP/2/3, статические сайты и reverse proxy.
Публичный TCP/UDP relay и Caddy-L4 исключены из binary и settings schema и
отложены до v2. Также в v2 отложены CAPTCHA, Identity/OIDC/OAuth, TUF/install
через Core и управление процессами или контейнерами.

Полные границы и acceptance см. в
[целевой архитектуре Core](../../liapoldus.github.io/core/architecture/target)
и [TODO](TODO.md).

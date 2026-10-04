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

Общий размер HTTP request headers ограничен 64 KiB: считается сумма каждого
имени поля, разделителя `: `, значения и CRLF; `Host` включён, request line и
завершающая пустая строка не включены. Превышение отклоняется со статусом 431
до выполнения route handler. Для защиты самого HTTP parser Caddy также задаёт
bounded `max_header_bytes`; точная семантика размера закреплена в
`contracts/v1/http-dispatch.json`.

У plugin route запрос может задавать `requestCookieNames` — уникальный allow-list
имён Cookie, которые будут переданы выбранной capability в typed `cookies[]`.
Сопоставление имён точное и регистрозависимое; порядок и повторения разрешённых
cookies сохраняются. По умолчанию список пуст, сырой заголовок `Cookie` никогда
не передаётся, остальные имена отбрасываются. Это часть Server route settings,
не Core policy/API.

Unary response action и metadata `response_start` могут содержать typed `cookies[]`
с полями `name`, `value`, `path`, `domain`, `expires`, `maxAge`, `secure`,
`httpOnly` и `sameSite` (`lax`, `strict` или `none`). Для `SameSite=None` обязателен
`secure`; `Set-Cookie` нельзя вернуть через обычный `headers` map. Server
проверяет весь response action целиком до отправки status/headers; для WebSocket
cookies входят в handshake decision и проверяются до `101`. Невалидный элемент
отклоняет всю группу cookies без частичного ответа. Полная JSON-схема находится
в [`http-response-action.schema.json`](../../../contracts/v1/http-response-action.schema.json).

Reverse-proxy pool содержит 1–32 HTTP(S) origins с весами 1–100 и smooth
weighted round-robin. Passive connection failure допускает не более одного
retry до отправки request bytes; HTTPS verification обязательна, private CA
задаётся внешним `caRef`. Active health probes и отключение проверки TLS не
поддерживаются.

TLS использует ACME либо custom certificate/key references; private key bytes не
передаются через settings. Валидная конфигурация может активироваться до
завершения ACME issuance; готовность сертификата отслеживается отдельно.
В текущую v1-сборку не включены DNS provider modules: доступны встроенные
Caddy challenges HTTP-01 и TLS-ALPN-01. DNS-01 потребует отдельного изменения
состава бинарника и его conformance-проверок.
HTTP/2 и HTTP/3 доступны на TLS listener при поддержке сборки Caddy.
Продление выполняет Caddy автоматически; ручные `renew` и `revoke` операции
не предоставляются через Admin Surface в v1.

Admin Surface содержит read-only capabilities `server.certificates.list` и
`server.certificates.get`. Они показывают только домены активных TLS listeners,
источник сертификата (`acme` или `custom`), readiness и публичные метаданные
leaf-сертификата: serial, `notBefore` и `notAfter`, когда сертификат уже загружен
в Caddy. Для ACME до выпуска readiness равен `pending`; неизвестный Caddy
сертификат не превращается в успешный статус. Неактивные домены и внутренние
пути хранилища не перечисляются. PEM, private key и secret references никогда
не входят в ответ. Неизвестный домен возвращает `not_found`.

Site publication выполняется через generic Admin Surface artifact action и
Plugin SDK. `202 Accepted` возвращается после durable staging artifact с
проверенными размером и SHA-256; digest относится к сжатым байтам. Фоновая
operation распаковывает и проверяет gzip/tar, manifest, лимиты и archive
entries. Только после полной проверки immutable release атомарно становится
`current`, а прежний `current` — `previous`. Ошибка проверки переводит operation
в `failed` и оставляет оба активных указателя без изменений.

Для terminal plugin route `mode: "call"` передаёт конечный JSON request/response.
`mode: "http_stream"` передаёт metadata отдельным `request_start`, затем поток
body-кадров и `request_end`; ответ начинается единственным `response_start` и
завершается `response_end`. Server считает реальные body octets после HTTP
transfer framing, включая chunked, и не буферизует всё тело. Превышение лимита
до начала ответа даёт `413`; после начала ответа поток закрывается без подмены
уже отправленных status/headers. Полный envelope и лимиты заданы в
[`http-stream.json`](../../../contracts/v1/http-stream.json) и
[`http-stream.schema.json`](../../../contracts/v1/http-stream.schema.json).
Потоковая отправка Server повторяет временный `ErrSendQueueFull`, пока вызов
остаётся активным, поэтому медленный peer не приводит к неограниченной очереди.
Если producer сам прекращает отправку при заполненной ограниченной очереди,
ошибка после `response_start` закрывает только текущий HTTP stream — уже
отправленные headers и status не заменяются.
`http_stream` имеет отдельные ограничения от unary `call`: по умолчанию не более
128 одновременных streams на instance и route, idle timeout — 60 секунд,
максимальная длительность — 1 час. Route может только уменьшить concurrency,
idle timeout и max duration; предел instance остаётся общим. При достижении
лимита до начала ответа возвращается `504`; после response-start stream
закрывается без замены уже отправленного ответа. Вызов не переигрывается.
SSE использует тот же bidi peer stream, но передаёт структурированные
`event/data/id/retry` поля; Server проверяет event целиком до сериализации в
`text/event-stream`, нормализует перевод строки в `data` и запрещает CR/LF/NUL
в управляющих полях. WebSocket использует отдельный handshake frame внутри
того же bidi stream: plugin принимает или отклоняет upgrade и может выбрать
только предложенный клиентом subprotocol. После успешного `101` сообщения
передаются как пары `message_start/chunk/end`; тип text/binary и граница каждого
сообщения сохраняются. Ping/pong остаётся на стороне Server, а сообщения
ограничены 1 MiB. До upgrade неверное решение plugin возвращает обычную
ошибку Server; после upgrade некорректный frame или разрыв закрывает только
этот WebSocket. Реальный Caddy→mTLS-peer путь для accept/reject, subprotocol,
text/binary и message boundaries проверяет
`tests/websocket-dispatch-runtime.test.ts`.
WebSocket handshake может выставить те же typed cookies; они передаются только
если весь handshake action валиден и plugin принимает upgrade.
В v1 WebSocket использует классический RFC 6455 upgrade поверх HTTP/1.1;
HTTP/2 Extended CONNECT и WebSocket over HTTP/3 не объявляются поддерживаемыми.

## Восстановление plugin-to-plugin соединения

Server устанавливает peer-соединение при активации runtime snapshot. Если целевой
plugin перезапустился, текущий вызов завершается ограниченной ошибкой и не
повторяется: результат уже отправленного вызова может быть неизвестен. Server
сбрасывает закрытую peer-сессию; следующий независимый запрос устанавливает
новое аутентифицированное соединение к тому же объявленному endpoint. Поэтому
операторские клиенты могут повторить безопасный read-only запрос, но не должны
автоматически переигрывать неизвестный результат изменяющей операции. Core не
перезапускает удалённые plugin processes и не проксирует peer traffic.

## Явно вне v1 — v2

- Caddy-L4 и public TCP/UDP listeners/relay.
- CAPTCHA и Identity (OIDC/OAuth) plugins.
- TUF catalog, проверка и установка plugin releases через Core.
- Core-managed local process supervision и Docker/Compose/Swarm/Kubernetes.

Эти возможности не входят в v1 binary, API, SQLite state, documentation gates
или tests. Полная граница версий находится в
[целевой архитектуре Core](../core/architecture/target).

## Ручной запуск v1

Оператор сначала собирает Server binary и задаёт значения bootstrap-флагов из
окружения своего deployment. В примере показан полный набор параметров для
вызовов forms-db; группа `--peer-*` необязательна, если Server не вызывает другие
плагины. Для включённой группы нужны все её флаги. Пути сертификата, ключа, CA и
CRL должны быть абсолютными. Имена сертификатов в Core bootstrap и аргументах
плагина должны совпадать точно:

```bash
./server \
  --instance-id=server \
  --replica-id=server-1 \
  --rest-listen=127.0.0.1:9543 \
  --core-url=https://127.0.0.1:9444 \
  --core-server-name=core.internal \
  --core-common-name=core-control \
  --core-client-common-name=core-control \
  --ca-file=/absolute/path/to/control-ca.pem \
  --server-cert=/absolute/path/to/server-control.pem \
  --server-key=/absolute/path/to/server-control-key.pem \
  --client-cert=/absolute/path/to/server-client.pem \
  --client-key=/absolute/path/to/server-client-key.pem \
  --crl-file=/absolute/path/to/control-crl.pem \
  --peer-target-id=forms-db \
  --peer-endpoint=127.0.0.1:9643 \
  --peer-identity=spiffe://liapoldus.example/server \
  --peer-expected-identity=spiffe://liapoldus.example/forms-db \
  --peer-ca-file=/absolute/path/to/peer-ca.pem \
  --peer-cert=/absolute/path/to/server-peer.pem \
  --peer-key=/absolute/path/to/server-peer-key.pem
```

Замените демонстрационные IDs, адреса и сертификаты значениями `core.yaml` и
своего deployment; placeholder paths нужно заменить существующими абсолютными
путями. `--server-cert`/`--server-key` — REST identity, которую Core сверяет с
`plugins[].replicas[].expectedPeerIdentity`. `--client-cert`/`--client-key` —
отдельная identity, предъявляемая Core при pull конфигурации. Core client
identity для входящего `Reload` проверяется по
`--core-client-common-name`; Core HTTPS server при pull проверяется по
`--core-common-name` и DNS SAN `--core-server-name`. URI SAN можно закрепить
дополнительно через `--core-uri` и `--core-client-uri`; заданное значение должно
точно совпадать с URI SAN соответствующего Core-сертификата. Management API
использует отдельную identity и trust root.

Peer identity Server должна совпасть с `forms-db --peer-allowed-caller`, а
`--peer-expected-identity` — с `forms-db --peer-identity`. Для запуска на одной
машине используйте loopback endpoint; для разных машин — только адрес private
network, доступный вызывающему плагину. Продуктовые settings через flags,
environment или локальный application config не передаются: Core хранит их в
SQLite и уведомляет Server посредством Plugin SDK `Reload`.

После запуска проверьте health/replica readiness через Core Management API и
проверьте HTTP listener Server отдельно. Не публикуйте Plugin SDK REST endpoint,
peer endpoint или Core control listener в публичную сеть.

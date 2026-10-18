# Server service

Control plane: Studio → CLI → Core → Service SDK REST. Data plane:
независимые сервисы → Wool. Runtime entities — Services; StudioPlugins
обозначают только расширения редактора Studio.

Server service — отдельный сервис на базе Caddy. Он обслуживает пользовательский
HTTP/HTTPS traffic; Core хранит его desired JSON configuration, но не содержит
Caddy runtime и не проксирует запросы. В v1 оператор отдельно устанавливает и
запускает Core, Server service и forms-db.

Общий Core↔service lifecycle принадлежит Service SDK REST: Core вызывает
`Reload(generation)`, service сам запрашивает точное поколение и подтверждает
digest после применения. `wool` используется только для generic
service↔service communication.

Server binary обязан использовать Service SDK `HandlerSet`. SDK единолично
обслуживает универсальные `GET /healthz` и `GET /metrics` на control REST
listener и предоставляет `NewObservabilityHandler` для публичного listener.
Server не реализует и не переопределяет эти endpoints; product HTTP surface
добавляет только Server-owned routes.

Публичный `POST /api/peer?target=<instance>&method=<method>` принимает один
ограниченный JSON peer payload и вызывает указанный opaque method через
`wool`. Server разрешает target только по peer directory, выданному
Core, и возвращает ответ peer без Core/CLI proxy. Эта поверхность общая для
любых совместимых services и не содержит их имён, SDK REST URL или специальных
веток по продуктам. SDK observability endpoints остаются отдельными и не
перехватываются peer dispatch.

## Ответственность и состояние

| Область | Владелец |
| --- | --- |
| Desired HTTP settings | Core SQLite хранит исходные JSON bytes с поколениями `active`/`previous`. |
| Settings schema и runtime validation | Server service; Core применяет только общий service-owned schema validation. |
| HTTP/TLS runtime | Server service на Caddy; конфигурация JSON компилируется во внутренний runtime snapshot. |
| ACME и certificates | Server service/Caddy; состояние находится в его persistent storage. |
| Static site artifacts | Server service; immutable releases с указателями `current`/`previous`. |
| Process lifecycle | Оператор средствами ОС; Core не устанавливает, не запускает, не останавливает и не перезапускает процесс. |

В v1 допускается одна Server service replica на Core. Оператор вручную сохраняет
один и тот же persistent directory при обновлении или рестарте. Внешний Caddy
Admin API не публикуется и не является интерфейсом Core.

## HTTP settings contract v2

Внутренний decoder settings использует typed Go-константы из
`internal/domain/models/settings_definition.go`: version `2`, поле версии
`schemaVersion` и поле runtime config `config`. Runtime не читает эти значения
из `service.json`. JSON Schema и семантика settings принадлежат Server и
экспортируются из code-first definitions в `contracts/v1/`; `go run
./cmd/contracts --check` проверяет совпадение опубликованных файлов с исходниками.
Четыре grant purposes для TLS certificate, private key, upstream CA и HTTP
credentials определены типом `SettingsSecretPurpose` в REST adapter. Значения
секретов не входят в Project settings: там хранится только opaque reference.

Server service принимает только собственный versioned JSON contract, не raw Caddy
JSON и не Caddyfile. Настройки задают public HTTP/HTTPS listeners и ordered
routes. Для reverse-proxy routes `match` поддерживает hostname и ровно один
path режим `exact`, `prefix`, `glob` или Go RE2 `regex`. Static route задаёт
`handler.mountPrefix`, `handler.siteId` и точный `handler.releaseId` из каталога
неизменяемых релизов. Server обслуживает остаток URL внутри выбранного релиза,
отдаёт index document на корне mount и отклоняет traversal и encoded separators.
Публикация нового релиза не меняет уже закреплённый маршрут. Service route
обязан содержать versioned `api` contract `server-http/v1`: method и template
path задаются в нём, а не дублируются в `match`; `match` для такого route может
задавать только host. Named path placeholders имеют форму `{name}` и занимают
ровно один сегмент. Старый untyped service route без `api` contract не
принимается. Совпавший route использует один terminal handler: static site,
HTTP(S) reverse-proxy pool или service capability. Middleware chain и общий
`continue` не входят в текущий settings contract.

`config.defaults` задаёт общие режимы `compression`, `cache`, `headers`, `cors`,
`auth`, `rateLimit`, `healthCheck` и `rewrite` по типам routes. Для каждой функции
route выбирает `inherit`, `enabled` или `disabled`. При отключении её значения
остаются в Project и доступны при повторном включении. Без явных defaults
функции выключены; исключение — static compression (`zstd`/`gzip`) и публичный
`Cache-Control: max-age=3600`. Для service и reverse-proxy cache и compression
по умолчанию выключены.

`headers` задаёт фиксированные allowlisted значения request/response headers.
`cors` поддерживает exact origins, методы, request/response header allowlists,
credentials и кэширование preflight. Server отвечает на разрешённый preflight
сам и блокирует запросы с неперечисленными origin, методом или заголовком;
wildcard origin нельзя совмещать с credentials. Cache здесь задаёт только
`Cache-Control`, а не хранит ответы; включать его для персонализированного API
можно только явно.

`auth` поддерживает Bearer token, API key в выбранном заголовке и Basic с
username. Конфигурация ссылается на service-scoped secret reference; plaintext
принимается host-owned потоком Studio/CLI, остаётся в системном хранилище и не
передаётся плагину, Project или Caddy JSON. Runtime получает scoped grant при
применении настроек и проверяет credentials по keyed MAC в памяти.

`rateLimit` использует instance-local token bucket на route и IP клиента и
возвращает `429`. Forwarded IP учитывается только для явно перечисленных
`trustedProxies`; без доверенного proxy используется сетевой адрес клиента.
`rewrite` меняет request path до передачи reverse-proxy upstream. Для
reverse-proxy route `healthCheck` включает активные HTTP GET probes с заданными
path, interval, timeout и ожидаемым status; Caddy исключает нездоровый upstream
из выбора до успешного последующего probe. `healthCheck` применяется только к
reverse-proxy routes.

Redirect — отдельный terminal route handler с локальным абсолютным path либо
явным `http`/`https` URL target и статусом 301, 302, 307 или 308. Отдельно
настраиваемый listener может переводить HTTP на
явно заданный TLS listener с проверкой покрытия hostname; он не создаёт HTTP
socket автоматически.

### Typed request/response contract

`api` задаёт typed path/query/header параметры: scalar или array типов string,
integer, number и boolean; optional/required, enum, длины и числовые границы.
Для repeated values требуется явная политика: `single` отклоняет повторы,
`first`/`last` выбирают одно значение, `repeat` формирует массив из повторных
query/header значений, а `commaDelimited` разбивает одно значение по запятым.
Неизвестные query и headers разрешены по умолчанию и могут быть запрещены
независимо (`allowUnknownQuery`, `allowUnknownHeaders`). Секретные и hop-by-hop
headers нельзя объявить как параметры; Cookie передаётся только отдельным
`requestCookieNames` allow-list.

Пример конфигурации `GET /echo?text=...`:

```yaml
api:
  version: server-http/v1
  method: GET
  path: /echo
  queryParameters:
    - name: text
      type: string
      repeated: single
      required: true
      minLength: 1
      maxLength: 2048
  responses:
    - status: 200
      contentType: application/json
      schema:
        type: object
        required: [text]
        properties:
          text: { type: string }
      example: { text: hello }
```

Унарный `body` задаётся как `application/json` JSON Schema с обязательностью и
лимитом до 1 MiB. Server валидирует request method/path/параметры/body до
dispatch; ошибка возвращает общий `400` без отражения пользовательских значений
и без вызова capability. Сервис получает нормализованные данные в
`parameters` (`version`, `path`, `query`, `headers`, optional `body`) согласно
[`http-request-parameters.schema.json`](../../../contracts/v1/http-request-parameters.schema.json).
JSON body копируется без преобразования. Непустое unary body запрещено, если
schema не объявлена; stream modes передают body отдельными bounded frames.

`responses` описывают status, content type, schema и example. При candidate
apply Server проверяет каждую example против schema; response schema служит
документацией и не валидирует живой ответ, не преобразует его и не ограничивает
продуктовые status codes.

`schema` у request body и response — это schema-owned JSON document; в SDK
configuration schema он помечен `x-studio-json-document: true`. Studio может
заменять документ целиком через bounded typed draft operation, но не редактирует
его произвольные вложенные пути. Остальные открытые объекты остаются закрытыми
для полной замены.
`example` — отдельное bounded JSON value (`x-studio-json-value: true`): он может
быть объектом, массивом или scalar и редактируется как единое значение. Server
сверяет его с sibling response schema при apply.

Общий размер HTTP request headers ограничен 64 KiB: считается сумма каждого
имени поля, разделителя `: `, значения и CRLF; `Host` включён, request line и
завершающая пустая строка не включены. Превышение отклоняется со статусом 431
до выполнения route handler. Для защиты самого HTTP parser Caddy также задаёт
bounded `max_header_bytes`; точная семантика размера закреплена в
`contracts/v1/http-dispatch.json`.

У service route запрос может задавать `requestCookieNames` — уникальный allow-list
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
задаётся внешним `caRef`. Настройка `features.healthCheck` дополнительно
проверяет upstream активными probes и исключает origin, пока он не станет
здоровым. Без явного `healthCheck.mode: enabled` probes не выполняются.
Отключить проверку TLS сертификата upstream нельзя.

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

Admin Surface связывает каждую табличную секцию с явным `queryResource` и
списком разрешённых `columns`; host запрашивает данные только по этому
идентификатору через typed CLI → Core и отбрасывает поля вне списка. Для
действий с конкурентным обновлением дескриптор может объявить `ifMatchField`:
это обязательное поле input, значение которого host передаёт как `If-Match`.
Например, rollback сайта сравнивает `expectedCurrentRevision` с текущей
ревизией и отклоняет устаревший запрос, не переключая активный релиз.

HTTP-запрос, который не совпал ни с одним настроенным route, завершается
`404 Not Found`; успешный пустой ответ не используется как неявный fallback.

Site publication выполняется через generic Admin Surface artifact action и
Service SDK. `202 Accepted` возвращается после durable staging artifact с
проверенными размером и SHA-256; digest относится к сжатым байтам. Фоновая
operation распаковывает и проверяет gzip/tar, manifest, лимиты и archive
entries. Только после полной проверки immutable release атомарно становится
`current`, а прежний `current` — `previous`. Ошибка проверки переводит operation
в `failed` и оставляет оба активных указателя без изменений.
Manifest задаёт `siteId`, `documentRoot` и `indexDocument`. `site-releases`
каталог через Service SDK/CLI возвращает доступные immutable revision IDs;
маршрут хранит выбранный digest вместе с site ID. При проверке кандидатной
конфигурации Server сверяет пару с каталогом релизов до передачи её runtime;
несуществующий digest или релиз другого сайта отклоняется до активации.

Для terminal service route `mode: "call"` передаёт конечный JSON request/response.
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
того же bidi stream: service принимает или отклоняет upgrade и может выбрать
только предложенный клиентом subprotocol. После успешного `101` сообщения
передаются как пары `message_start/chunk/end`; тип text/binary и граница каждого
сообщения сохраняются. Ping/pong остаётся на стороне Server, а сообщения
ограничены 1 MiB. До upgrade неверное решение service возвращает обычную
ошибку Server; после upgrade некорректный frame или разрыв закрывает только
этот WebSocket. Реальный Caddy→mTLS-peer путь для accept/reject, subprotocol,
text/binary и message boundaries проверяет
`tests/websocket-dispatch-runtime.test.ts`.
WebSocket handshake может выставить те же typed cookies; они передаются только
если весь handshake action валиден и service принимает upgrade.
В v1 WebSocket использует классический RFC 6455 upgrade поверх HTTP/1.1;
HTTP/2 Extended CONNECT и WebSocket over HTTP/3 не объявляются поддерживаемыми.

## Восстановление service-to-service соединения

Server устанавливает peer-соединение при активации runtime snapshot. Если целевой
service перезапустился, текущий вызов завершается ограниченной ошибкой и не
повторяется: результат уже отправленного вызова может быть неизвестен. Server
сбрасывает закрытую peer-сессию; следующий независимый запрос устанавливает
новое аутентифицированное соединение к тому же объявленному endpoint. Поэтому
операторские клиенты могут повторить безопасный read-only запрос, но не должны
автоматически переигрывать неизвестный результат изменяющей операции. Core не
перезапускает удалённые service processes и не проксирует peer traffic.

## Версионные границы

- V2: общие Core/Service SDK registration, leases и rollout без изменения
  Server repository. Проверяй эту платформенную работу на generic fixtures;
  существующий Server v1 используется только как неизменённый regression target.
- V3: Server-specific replica registration/compatibility, rollout, scaling и
  shared storage/ACME conformance.
- V3: Caddy-L4 и public TCP/UDP listeners/relay; Identity/CAPTCHA integrations
  только после отдельной разморозки соответствующих service owners.
- Во всех версиях оператор отвечает за проверку immutable release artifact,
  установку и плановое обновление Server.
  Core не получает API установки или управления workloads.

Эти возможности не входят в v1 binary, API, SQLite state, documentation gates
или tests. Полная граница версий находится в
[целевой архитектуре Core](../core/architecture/target).

## Масштабируемый Server в v3 — отложенный проектный контракт

В отличие от однорепличного v1, будущий v3 допускает несколько Server replicas одного
instance. Каждая должна регистрироваться через Service SDK с уникальной mTLS identity,
SemVer/digest, placement и applied generation. Одновременно могут обслуживать
две совместимые когорты `(release digest, config generation)`; Core назначает
когорты и проверяет ACK, а внешний ingress применяет и подтверждает публичные
веса. HTTP affinity во время canary принадлежит ingress. Незавершённые streams
доживают или завершаются при bounded drain без replay.

Все Server replicas, обслуживающие один сайт, используют общий RWX filesystem
для immutable site releases, `current`/`previous` и CertMagic storage. Публикация
release требует single-writer coordination, атомарного переключения указателей
и восстановления после сбоя; локальные каталоги replicas не являются общим
источником истины. CertMagic storage должен корректно выполнять атомарные
операции и распределённые блокировки для ACME. Конкретный RWX filesystem
объявляется поддерживаемым только после conformance на concurrent issuance,
locking, publish/rollback, crash recovery и повреждение/недоступность тома.
Простой RWX mount без этих проверок не доказывает готовность к эксплуатации.

Текущие child-process проверки подтверждают сериализацию конкурирующей
публикации, recovery принятой durable operation после принудительного
завершения принимающего процесса и одновременный recovery двумя независимыми
Server-процессами на локальной файловой системе. Оба восстановителя должны
наблюдать одну completed operation и один активный revision. Это не
подтверждает семантику блокировок, crash recovery или ACME coordination для
NFS/другого RWX volume; поддерживаемый multi-host storage profile пока не
выбран и не заявляется.

Rollout двух версий допускается только если Server Manifest объявляет
совместимость shared storage layout, site release format и peer/HTTP contracts.
Несовместимые версии требуют отдельного maintenance rollout без совместного
доступа к state. Caddy-L4 и public TCP/UDP relay также относятся к v3, но
проверяются отдельными Server-owned gates. Каноническая модель регистрации и размещения приведена в
[документации Core](../core/architecture/service-deployment).

## Ручной запуск v1

Оператор собирает Server binary и задаёт bootstrap-флаги из окружения своего
deployment. Для прямых вызовов services Server получает только собственную
peer identity и доверенный CA; target endpoint и peer identity не задаются
флагами и не помещаются в Server settings. Они разрешаются при каждом вызове из
Core-issued peer directory. Пути сертификатов, ключей, CA и CRL должны быть
абсолютными. Имена Core certificates должны совпадать с Core bootstrap:

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
  --peer-identity=spiffe://liapoldus.example/server \
  --peer-ca-file=/absolute/path/to/peer-ca.pem \
  --peer-cert=/absolute/path/to/server-peer.pem \
  --peer-key=/absolute/path/to/server-peer-key.pem
```

Замените демонстрационные IDs, адреса и сертификаты значениями `core.yaml` и
своего deployment; placeholder paths нужно заменить существующими абсолютными
путями. `--server-cert`/`--server-key` — REST identity, которую Core сверяет с
`services[].replicas[].expectedPeerIdentity`. `--client-cert`/`--client-key` —
отдельная identity, предъявляемая Core при pull конфигурации. Core client
identity для входящего `Reload` проверяется по
`--core-client-common-name`; Core HTTPS server при pull проверяется по
`--core-common-name` и DNS SAN `--core-server-name`. URI SAN можно закрепить
дополнительно через `--core-uri` и `--core-client-uri`; заданное значение должно
точно совпадать с URI SAN соответствующего Core-сертификата. Management API
использует отдельную identity и trust root.

`--peer-identity` — локальная SPIFFE identity, которую Server предъявляет
непосредственному peer. Ожидаемая identity, carrier и endpoint выбираются из
актуального Core-issued peer directory; caller не подключается к адресу из
проектного файла или CLI-флага. Core применяет generic link policy и конфигурацию,
но не проксирует бизнес-вызовы. Server не содержит веток или зависимостей,
связанных с конкретными соседними services. Продуктовые settings через flags,
environment или локальный application config не передаются: Core хранит их в
SQLite и уведомляет Server посредством Service SDK `Reload`.

После запуска проверьте health/replica readiness через Core Management API и
проверьте HTTP listener Server отдельно. Не публикуйте Service SDK REST endpoint,
peer endpoint или Core control listener в публичную сеть.

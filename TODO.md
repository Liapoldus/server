# TODO — Caddy plugin

Срез на 2026-09-29. Caddy — отдельный data-plane plugin. Core не должен
содержать Caddy runtime, его зависимости или traffic handlers. Общие lifecycle,
networking, mTLS и stream contracts принадлежат
[`pluginprotocol`](../../pluginprotocol/); Caddy-specific settings/runtime —
этому модулю.

## Реализовано

- Рабочий Go module/binary собирает Caddy и Caddy-L4.
- Plugin принимает settings через `ConfigApply`, валидирует JSON settings и
  атомарно сохраняет активную revision in-memory.
- Unary HTTP dispatch вызывает целевой plugin по protocol и применяет
  типизированный response action.
- Local supervised process использует inherited listener и launch-scoped mTLS
  bootstrap из `pluginprotocol`.
- TypeScript child-process tests покрывают config activation и unary HTTP
  dispatch; Go build/vet и npm tests проходят.

## Осталось — по зависимостям

1. **Конфигурация маршрутов и lifecycle:** собирать listener/route bindings из
   versioned Caddy settings; до активации проверять Manifest capability→mode;
   сохранить управление настройками только через Core `ConfigApply`.
2. **HTTP streaming:** request/response chunks, response-start validation,
   фактический byte limit, cancellation, backpressure, idle/max-duration и
   закрытие после отправленных headers.
3. **WebSocket и SSE:** handshake accept/reject и subprotocol selection,
   сохранение text/binary message boundaries; structured SSE event serialization.
4. **L4:** TCP connection stream и отдельный UDP stream на datagram через
   `pluginprotocol.Stream`; проверить Caddy-L4 adapter conformance без Go `net`
   fallback в Core.
5. **Состояние и восстановление:** persistent CertMagic/ACME state, site
   artifacts/releases и `current`/`previous`; crash/restart recovery без
   изменения Core desired state и без хранения application settings в Caddy
   autosave как источнике истины.
6. **Remote profile:** принимать только внешне provisioned workload identity,
   fail closed без mTLS; Core не запускает процесс и не обращается к Docker или
   Kubernetes API.
7. **Сквозной gate:** process-level ConfigSchema/ConfigApply/health/shutdown,
   HTTP/HTTP3/TLS/ACME, Stream modes, grants, cookie actions, limits, restart,
   persistence и parity для supervised/external deployment.
8. **GitHub publication:** локальный Git repository создаётся в этом каталоге.
   Canonical remote пока не назначен: не угадывать URL и не выполнять push,
   пока владелец не сообщит адрес репозитория.

Единый приоритет и acceptance matrix находятся в
[workspace roadmap](https://liapoldus.github.io/gateway/architecture/v1-migration-roadmap).

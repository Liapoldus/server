# Пример reverse proxy

Core не принимает Caddyfile или собственную route DSL. Reverse-proxy settings входят в versioned JSON schema Server service, хранятся в SQLite, а Server service получает их pull-ом после REST `Reload(generation)`.

См. [транспорты](../configuration/transports) и [целевую архитектуру](../architecture/target).

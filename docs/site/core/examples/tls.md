# TLS и Management access

Server plugin владеет публичным TLS и ACME, а Management API использует отдельные bind, TLS identity и authorization. В v1 Server plugin имеет одну replica и собственный persistent filesystem для CertMagic state.

См. [Security](../configuration/security) и [target architecture](../architecture/target).

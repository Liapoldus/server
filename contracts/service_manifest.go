package contracts

import "liapoldus.local/server-service/contracts/definitions"

func ServiceManifest() ([]byte, error) { return definitions.Bytes("service.json") }

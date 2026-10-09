package contracts

import "liapoldus.local/server-plugin/contracts/definitions"

func PluginManifest() ([]byte, error) { return definitions.Bytes("plugin.json") }

package main

import (
	"encoding/json"
	"os"

	"github.com/Liapoldus/caddy-plugin/contracts"
	pluginsdk "github.com/Liapoldus/pluginprotocol/presentation/sdk"
)

func main() {
	if len(os.Args) != 6 {
		os.Exit(2)
	}
	targetAddress, publicAddress, storageDirectory := os.Args[1], os.Args[2], os.Args[3]
	settingsPath, manifestPath := os.Args[4], os.Args[5]
	plugin, err := contracts.Load()
	check(err)
	dispatch, err := contracts.LoadHTTPDispatch()
	check(err)
	manifest, err := json.Marshal(map[string]any{
		"name": plugin.Name, "protocolVersion": pluginsdk.ProtocolVersion,
		"capabilities": []string{}, "capabilityDescriptors": []any{},
	})
	check(err)
	configuration, err := json.Marshal(map[string]any{
		"schemaVersion": plugin.Configuration.SchemaVersion,
		"config": map[string]any{
			"admin": map[string]any{"disabled": true, "config": map[string]any{"persist": false}},
			"storage": map[string]any{"module": "file_system", "root": storageDirectory},
			"apps": map[string]any{
				dispatch.App: map[string]any{"instances": []any{map[string]any{
					"id": "http-target", "endpoint": targetAddress, "timeoutMillis": dispatch.DefaultTimeoutMillis,
				}}},
				"http": map[string]any{"servers": map[string]any{"test": map[string]any{
					"listen": []string{publicAddress},
					"routes": []any{map[string]any{"handle": []any{map[string]any{
						"handler": moduleName(dispatch.Module), "instance": "http-target", "capability": "http.echo",
					}}}},
				}}},
			},
		},
	})
	check(err)
	check(os.WriteFile(settingsPath, configuration, 0o600))
	check(os.WriteFile(manifestPath, manifest, 0o600))
}

func moduleName(module string) string {
	const prefix = "http.handlers."
	if len(module) >= len(prefix) && module[:len(prefix)] == prefix {
		return module[len(prefix):]
	}
	return module
}

func check(err error) {
	if err != nil {
		os.Exit(1)
	}
}

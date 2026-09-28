package models

import (
	"bytes"
	"encoding/json"
	"errors"
)

var ErrInvalidSettings = errors.New("invalid Caddy plugin settings")

type Settings struct {
	SchemaVersion int
	RuntimeConfig json.RawMessage
}

func DecodeSettings(contents []byte, versionField, runtimeConfigField string, schemaVersion int) (Settings, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(contents, &raw); err != nil || raw == nil {
		return Settings{}, ErrInvalidSettings
	}
	if len(raw) != 2 {
		return Settings{}, ErrInvalidSettings
	}
	var version int
	var runtimeConfig json.RawMessage
	if err := json.Unmarshal(raw[versionField], &version); err != nil || version != schemaVersion {
		return Settings{}, ErrInvalidSettings
	}
	if err := json.Unmarshal(raw[runtimeConfigField], &runtimeConfig); err != nil || len(bytes.TrimSpace(runtimeConfig)) == 0 || bytes.TrimSpace(runtimeConfig)[0] != '{' {
		return Settings{}, ErrInvalidSettings
	}
	return Settings{SchemaVersion: version, RuntimeConfig: runtimeConfig}, nil
}

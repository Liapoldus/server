package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
)

var ErrInvalidSettings = errors.New("invalid Caddy plugin settings")

type Settings struct {
	SchemaVersion int
	RuntimeConfig json.RawMessage
}

func (settings Settings) UpstreamCAReferences() ([]string, error) {
	var config struct {
		Routes []struct {
			Handler struct {
				Type      string `json:"type"`
				Upstreams []struct {
					CARef string `json:"caRef"`
				} `json:"upstreams"`
			} `json:"handler"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(settings.RuntimeConfig, &config); err != nil {
		return nil, ErrInvalidSettings
	}
	references := make(map[string]struct{})
	for _, route := range config.Routes {
		if route.Handler.Type != "reverseProxy" {
			continue
		}
		for _, upstream := range route.Handler.Upstreams {
			if upstream.CARef == "" {
				continue
			}
			references[upstream.CARef] = struct{}{}
		}
	}
	result := make([]string, 0, len(references))
	for reference := range references {
		result = append(result, reference)
	}
	sort.Strings(result)
	return result, nil
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

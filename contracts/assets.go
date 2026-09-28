package contracts

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
)

//go:embed v1/*.json
var files embed.FS

var ErrInvalidAssets = errors.New("")

type Plugin struct {
	Name          string `json:"name"`
	Configuration struct {
		SchemaVersion      int    `json:"schemaVersion"`
		VersionField       string `json:"versionField"`
		RuntimeConfigField string `json:"runtimeConfigField"`
	} `json:"configuration"`
	ConfigSchema struct {
		Fields []ConfigField `json:"fields"`
	} `json:"configSchema"`
}

type ConfigField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

func Load() (Plugin, error) {
	contents, err := fs.ReadFile(files, "v1/plugin.json")
	if err != nil {
		return Plugin{}, ErrInvalidAssets
	}
	var contract Plugin
	if err := json.Unmarshal(contents, &contract); err != nil ||
		contract.Name == "" || contract.Configuration.SchemaVersion <= 0 ||
		contract.Configuration.VersionField == "" || contract.Configuration.RuntimeConfigField == "" ||
		len(contract.ConfigSchema.Fields) == 0 {
		return Plugin{}, ErrInvalidAssets
	}
	return contract, nil
}

func SettingsSchema() ([]byte, error) {
	contents, err := fs.ReadFile(files, "v1/settings.schema.json")
	if err != nil || !json.Valid(contents) {
		return nil, ErrInvalidAssets
	}
	return contents, nil
}

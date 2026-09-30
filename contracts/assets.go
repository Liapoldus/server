package contracts

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed v1/*.json
var files embed.FS

var ErrInvalidAssets = errors.New("")

var (
	settingsSchemaOnce sync.Once
	settingsSchema     *jsonschema.Schema
	settingsSchemaErr  error
)

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

func ValidateSettings(contents []byte) error {
	schema, err := compiledSettingsSchema()
	if err != nil {
		return ErrInvalidAssets
	}
	var candidate any
	if err := json.Unmarshal(contents, &candidate); err != nil {
		return ErrInvalidAssets
	}
	if err := schema.Validate(candidate); err != nil {
		return ErrInvalidAssets
	}
	return nil
}

func compiledSettingsSchema() (*jsonschema.Schema, error) {
	settingsSchemaOnce.Do(func() {
		contents, err := SettingsSchema()
		if err != nil {
			settingsSchemaErr = err
			return
		}
		var document any
		if err := json.Unmarshal(contents, &document); err != nil {
			settingsSchemaErr = ErrInvalidAssets
			return
		}
		var metadata struct {
			ID string `json:"$id"`
		}
		if err := json.Unmarshal(contents, &metadata); err != nil || metadata.ID == "" {
			settingsSchemaErr = ErrInvalidAssets
			return
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		if err := compiler.AddResource(metadata.ID, document); err != nil {
			settingsSchemaErr = ErrInvalidAssets
			return
		}
		settingsSchema, settingsSchemaErr = compiler.Compile(metadata.ID)
	})
	return settingsSchema, settingsSchemaErr
}

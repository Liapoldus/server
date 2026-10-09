package contracts

import (
	"encoding/json"
	"errors"
	"liapoldus.local/server-plugin/contracts/definitions"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var ErrInvalidAssets = errors.New("")

var (
	settingsSchemaOnce sync.Once
	settingsSchema     *jsonschema.Schema
	settingsSchemaErr  error
)

func SettingsSchema() ([]byte, error) {
	contents, err := definitions.Bytes("settings.schema.json")
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

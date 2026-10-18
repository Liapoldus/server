package contracts

import (
	"encoding/json"
	"errors"
	"liapoldus.local/server-service/contracts/definitions"
	settingsmodel "liapoldus.local/server-service/internal/domain/models/settings"
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
	return validateSettingsAgainstCurrentSchema(contents)
}

// ValidateSettingsForBootstrap accepts the immediately previous envelope
// only when the caller has proven that this runtime has not applied any
// generation yet. The previous config payload is validated against the
// current schema after changing only the temporary validation envelope; the
// caller must retain and apply the original bytes unchanged.
func ValidateSettingsForBootstrap(contents []byte, allowPreviousVersion bool) error {
	if !allowPreviousVersion {
		return validateSettingsAgainstCurrentSchema(contents)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(contents, &envelope); err != nil || envelope == nil {
		return ErrInvalidAssets
	}
	var version int
	if err := json.Unmarshal(envelope[settingsmodel.SettingsVersionField], &version); err != nil {
		return ErrInvalidAssets
	}
	if version != settingsmodel.BootstrapSettingsVersion {
		return validateSettingsAgainstCurrentSchema(contents)
	}
	currentVersion, err := json.Marshal(settingsmodel.SettingsSchemaVersion)
	if err != nil {
		return ErrInvalidAssets
	}
	envelope[settingsmodel.SettingsVersionField] = currentVersion
	candidate, err := json.Marshal(envelope)
	if err != nil {
		return ErrInvalidAssets
	}
	return validateSettingsAgainstCurrentSchema(candidate)
}

func validateSettingsAgainstCurrentSchema(contents []byte) error {
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

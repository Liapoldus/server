package shared

import (
	"liapoldus.local/server-plugin/contracts"
	"liapoldus.local/server-plugin/internal/application"
	"liapoldus.local/server-plugin/internal/domain/models"
)

func Apply(configuration *application.Configuration, raw []byte, revision string, secrets map[string][]byte) error {
	contract, err := contracts.Load()
	if err != nil || contracts.ValidateSettings(raw) != nil {
		return contracts.ErrInvalidAssets
	}
	settings, err := models.DecodeSettings(raw, contract.Configuration.VersionField, contract.Configuration.RuntimeConfigField, contract.Configuration.SchemaVersion)
	if err != nil {
		return models.ErrInvalidSettings
	}
	if secrets == nil {
		return configuration.Apply(settings, revision)
	}
	return configuration.ApplyWithSecrets(settings, revision, secrets)
}

package shared

import (
	"liapoldus.local/server-plugin/contracts"
	settingsapp "liapoldus.local/server-plugin/internal/application/settings"
	settingsmodel "liapoldus.local/server-plugin/internal/domain/models/settings"
)

func Apply(configuration *settingsapp.Configuration, raw []byte, revision string, secrets map[string][]byte) error {
	if contracts.ValidateSettings(raw) != nil {
		return contracts.ErrInvalidAssets
	}
	settings, err := settingsmodel.DecodeSettings(raw)
	if err != nil {
		return settingsmodel.ErrInvalidSettings
	}
	if secrets == nil {
		return configuration.Apply(settings, revision)
	}
	return configuration.ApplyWithSecrets(settings, revision, secrets)
}

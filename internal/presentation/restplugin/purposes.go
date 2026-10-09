package restplugin

// SettingsSecretPurpose is the immutable grant scope for a Server settings
// reference. The SDK boundary receives its exact string value.
type SettingsSecretPurpose string

const (
	SettingsCertificatePurpose SettingsSecretPurpose = "server.tls.certificate"
	SettingsPrivateKeyPurpose  SettingsSecretPurpose = "server.tls.private-key"
	SettingsUpstreamCAPurpose  SettingsSecretPurpose = "server.upstream.ca"
)

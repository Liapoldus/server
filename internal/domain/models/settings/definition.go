package settings

// The immutable Server v1 settings envelope belongs to its model decoder.
// Public manifest/schema artifacts are checked against these values by tests;
// runtime decoding does not discover field names or versions from files.
const (
	SettingsSchemaVersion      int    = 1
	SettingsVersionField       string = "schemaVersion"
	SettingsRuntimeConfigField string = "config"
)

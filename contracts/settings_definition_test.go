package contracts

import (
	"encoding/json"
	"testing"

	settingsmodel "liapoldus.local/server-plugin/internal/domain/models/settings"
)

// Preserve the values captured from the runtime metadata parser before removal.
// The published manifest remains a public artifact and must keep these values.
func TestSettingsDefinitionPreservation(t *testing.T) {
	if settingsmodel.SettingsSchemaVersion != 1 || settingsmodel.SettingsVersionField != "schemaVersion" || settingsmodel.SettingsRuntimeConfigField != "config" {
		t.Fatal("internal settings definition changed")
	}
	contents, err := PluginManifest()
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Configuration struct {
			VersionField       string `json:"versionField"`
			RuntimeConfigField string `json:"runtimeConfigField"`
			SchemaVersion      int    `json:"schemaVersion"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(contents, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Configuration.SchemaVersion != settingsmodel.SettingsSchemaVersion || manifest.Configuration.VersionField != settingsmodel.SettingsVersionField || manifest.Configuration.RuntimeConfigField != settingsmodel.SettingsRuntimeConfigField {
		t.Fatal("public manifest differs from internal settings definition")
	}
}

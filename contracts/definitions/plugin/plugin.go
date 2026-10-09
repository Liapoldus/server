package plugin

type PluginDocumentConfiguration struct {
	VersionField       string `json:"versionField"`
	RuntimeConfigField string `json:"runtimeConfigField"`
	SchemaVersion      int    `json:"schemaVersion"`
}

type PluginDocumentConfigSchemaFieldsItem struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type PluginDocumentConfigSchema struct {
	Semantics          string                                 `json:"semantics"`
	ConformanceVectors string                                 `json:"conformanceVectors"`
	Fields             []PluginDocumentConfigSchemaFieldsItem `json:"fields"`
}

type PluginDocumentAdminSurface struct {
	Descriptor         string `json:"descriptor"`
	Schema             string `json:"schema"`
	Actions            string `json:"actions"`
	ActionsSchema      string `json:"actionsSchema"`
	ConformanceVectors string `json:"conformanceVectors"`
	Version            int    `json:"version"`
}

type PluginDocument struct {
	AdminSurface  PluginDocumentAdminSurface  `json:"adminSurface"`
	Configuration PluginDocumentConfiguration `json:"configuration"`
	Name          string                      `json:"name"`
	ConfigSchema  PluginDocumentConfigSchema  `json:"configSchema"`
}

func Plugin() PluginDocument {
	return PluginDocument{
		Name: "server",
		Configuration: PluginDocumentConfiguration{
			SchemaVersion:      int(1),
			VersionField:       "schemaVersion",
			RuntimeConfigField: "config",
		},
		ConfigSchema: PluginDocumentConfigSchema{
			Semantics:          "contracts/v1/settings-semantics.json",
			ConformanceVectors: "contracts/v1/settings-vectors.json",
			Fields: []PluginDocumentConfigSchemaFieldsItem{{
				Name:        "listeners",
				Type:        "array",
				Required:    true,
				Description: "Public HTTP and HTTPS listeners. Validate the complete settings revision against the plugin-owned settings.schema.json contract.",
			}, {
				Name:        "routes",
				Type:        "array",
				Required:    true,
				Description: "Ordered terminal HTTP routes. Native Caddy JSON and Caddyfile input are not accepted.",
			}},
		},
		AdminSurface: PluginDocumentAdminSurface{
			Version:            int(1),
			Descriptor:         "contracts/v1/admin-surface.json",
			Schema:             "contracts/v1/admin-surface.schema.json",
			Actions:            "contracts/v1/admin-actions.json",
			ActionsSchema:      "contracts/v1/admin-actions.schema.json",
			ConformanceVectors: "contracts/v1/admin-surface-vectors.json",
		},
	}
}

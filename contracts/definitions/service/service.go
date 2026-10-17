package service

type ServiceDocumentConfiguration struct {
	VersionField       string `json:"versionField"`
	RuntimeConfigField string `json:"runtimeConfigField"`
	SchemaVersion      int    `json:"schemaVersion"`
}

type ServiceDocumentConfigSchemaFieldsItem struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type ServiceDocumentConfigSchema struct {
	Semantics          string                                  `json:"semantics"`
	ConformanceVectors string                                  `json:"conformanceVectors"`
	Fields             []ServiceDocumentConfigSchemaFieldsItem `json:"fields"`
}

type ServiceDocumentAdminSurface struct {
	Descriptor         string `json:"descriptor"`
	Schema             string `json:"schema"`
	Actions            string `json:"actions"`
	ActionsSchema      string `json:"actionsSchema"`
	ConformanceVectors string `json:"conformanceVectors"`
	Version            int    `json:"version"`
}

type ServiceDocument struct {
	AdminSurface  ServiceDocumentAdminSurface  `json:"adminSurface"`
	Configuration ServiceDocumentConfiguration `json:"configuration"`
	Name          string                       `json:"name"`
	ConfigSchema  ServiceDocumentConfigSchema  `json:"configSchema"`
}

func Service() ServiceDocument {
	return ServiceDocument{
		Name: "server",
		Configuration: ServiceDocumentConfiguration{
			SchemaVersion:      int(1),
			VersionField:       "schemaVersion",
			RuntimeConfigField: "config",
		},
		ConfigSchema: ServiceDocumentConfigSchema{
			Semantics:          "contracts/v1/settings-semantics.json",
			ConformanceVectors: "contracts/v1/settings-vectors.json",
			Fields: []ServiceDocumentConfigSchemaFieldsItem{{
				Name:        "listeners",
				Type:        "array",
				Required:    true,
				Description: "Public HTTP and HTTPS listeners. Validate the complete settings revision against the service-owned settings.schema.json contract.",
			}, {
				Name:        "routes",
				Type:        "array",
				Required:    true,
				Description: "Ordered terminal HTTP routes. Native Caddy JSON and Caddyfile input are not accepted.",
			}},
		},
		AdminSurface: ServiceDocumentAdminSurface{
			Version:            int(1),
			Descriptor:         "contracts/v1/admin-surface.json",
			Schema:             "contracts/v1/admin-surface.schema.json",
			Actions:            "contracts/v1/admin-actions.json",
			ActionsSchema:      "contracts/v1/admin-actions.schema.json",
			ConformanceVectors: "contracts/v1/admin-surface-vectors.json",
		},
	}
}

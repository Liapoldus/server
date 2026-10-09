package admin

import "liapoldus.local/server-plugin/contracts/schema"

func AdminSurfaceSchema() schema.Definition {
	return schema.Definition{
		Definition:           "https://json-schema.org/draft/2020-12/schema",
		ID:                   "https://liapoldus.github.io/plugins/server/admin-ui/v1/schema.json",
		Title:                "Server plugin Admin Surface v1",
		Description:          "Strict declarative plugin administration surface. It contains no executable UI, URL, listener, or secret value.",
		Type:                 "object",
		AdditionalProperties: false,
		Required:             schema.Value([]string{"version", "plugin", "requiredCapabilities", "pages"}),
		Properties: map[string]schema.Definition{"version": {
			Const: 1,
		}, "plugin": {
			Const: "server",
		}, "requiredCapabilities": {
			Type:        "array",
			MinItems:    schema.Value(int(1)),
			UniqueItems: schema.Value(true),
			Items: schema.Value(schema.Definition{
				Ref: "#/$defs/capability",
			}),
		}, "pages": {
			Type:     "array",
			MinItems: schema.Value(int(1)),
			MaxItems: schema.Value(int(32)),
			Items: schema.Value(schema.Definition{
				Ref: "#/$defs/page",
			}),
		}},
		Defs: map[string]schema.Definition{"nonEmptyString": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
			MaxLength: schema.Value(int(256)),
		}, "capability": {
			Type:      "string",
			Pattern:   "^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$",
			MaxLength: schema.Value(int(128)),
		}, "pageId": {
			Type:    "string",
			Pattern: "^[a-z][a-z0-9-]{0,62}$",
		}, "columnId": {
			Type:    "string",
			Pattern: "^[A-Za-z][A-Za-z0-9_-]{0,127}$",
		}, "field": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"key", "type"}),
			Properties: map[string]schema.Definition{"key": {
				Ref: "#/$defs/nonEmptyString",
			}, "type": {
				Enum: []any{"string", "number", "boolean", "select", "multiselect", "secret", "file", "directory", "duration", "size", "code", "keyValue", "array", "object"},
			}, "required": {
				Type: "boolean",
			}, "title": {
				Type:      "string",
				MaxLength: schema.Value(int(120)),
			}, "description": {
				Type:      "string",
				MaxLength: schema.Value(int(1024)),
			}},
		}, "inputField": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"type"}),
			Properties: map[string]schema.Definition{"type": {
				Enum: []any{"string", "number", "integer", "boolean", "null"},
			}, "minLength": {
				Type:    "integer",
				Minimum: schema.Value(int(0)),
			}, "maxLength": {
				Type:    "integer",
				Minimum: schema.Value(int(0)),
			}, "minimum": {
				Type: "number",
			}, "maximum": {
				Type: "number",
			}, "enum": {
				Type:        "array",
				MinItems:    schema.Value(int(1)),
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Type: []any{"string", "number", "boolean", "null"},
				}),
			}},
		}, "actionInputSchema": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"type", "properties", "required", "additionalProperties"}),
			Properties: map[string]schema.Definition{"type": {
				Const: "object",
			}, "properties": {
				Type: "object",
				AdditionalProperties: schema.Definition{
					Ref: "#/$defs/inputField",
				},
			}, "required": {
				Type:        "array",
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Type: "string",
				}),
			}, "additionalProperties": {
				Const: false,
			}},
		}, "artifactInput": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"mediaTypes", "maxBytes", "maxMetadataBytes", "maxMultipartOverheadBytes"}),
			Properties: map[string]schema.Definition{"mediaTypes": {
				Type:        "array",
				MinItems:    schema.Value(int(1)),
				MaxItems:    schema.Value(int(32)),
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Type:      "string",
					MaxLength: schema.Value(int(127)),
					Pattern:   "^[a-z0-9!#$%&'*+.^_`|~-]+/[a-z0-9!#$%&'*+.^_`|~-]+$",
				}),
			}, "maxBytes": {
				Type:    "integer",
				Minimum: schema.Value(int(1)),
				Maximum: schema.Value(int(134217728)),
			}, "maxMetadataBytes": {
				Type:    "integer",
				Minimum: schema.Value(int(2)),
				Maximum: schema.Value(int(65536)),
			}, "maxMultipartOverheadBytes": {
				Type:    "integer",
				Minimum: schema.Value(int(0)),
				Maximum: schema.Value(int(65536)),
			}},
		}, "action": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"id", "title", "capability", "inputSchema"}),
			Properties: map[string]schema.Definition{"id": {
				Ref: "#/$defs/pageId",
			}, "title": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
				MaxLength: schema.Value(int(120)),
			}, "capability": {
				Ref: "#/$defs/capability",
			}, "confirmation": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
				MaxLength: schema.Value(int(512)),
			}, "dangerous": {
				Type: "boolean",
			}, "inputSchema": {
				Ref: "#/$defs/actionInputSchema",
			}, "rowInput": {
				Type:          "object",
				MinProperties: schema.Value(int(1)),
				AdditionalProperties: schema.Definition{
					Ref: "#/$defs/nonEmptyString",
				},
			}, "artifactInput": {
				Ref: "#/$defs/artifactInput",
			}},
		}, "tableSection": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"id", "kind", "dataCapability", "columns", "actions"}),
			Properties: map[string]schema.Definition{"id": {
				Ref: "#/$defs/pageId",
			}, "kind": {
				Const: "table",
			}, "title": {
				Type:      "string",
				MaxLength: schema.Value(int(120)),
			}, "dataCapability": {
				Ref: "#/$defs/capability",
			}, "columns": {
				Type:        "array",
				MinItems:    schema.Value(int(1)),
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/columnId",
				}),
			}, "actions": {
				Type: "array",
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/action",
				}),
			}},
		}, "detailSection": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"id", "kind", "dataCapability", "fields"}),
			Properties: map[string]schema.Definition{"id": {
				Ref: "#/$defs/pageId",
			}, "kind": {
				Const: "detail",
			}, "title": {
				Type:      "string",
				MaxLength: schema.Value(int(120)),
			}, "dataCapability": {
				Ref: "#/$defs/capability",
			}, "fields": {
				Type:     "array",
				MinItems: schema.Value(int(1)),
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/field",
				}),
			}},
		}, "section": {
			OneOf: []schema.Definition{{
				Ref: "#/$defs/tableSection",
			}, {
				Ref: "#/$defs/detailSection",
			}},
		}, "page": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"id", "title", "capability", "sections"}),
			Properties: map[string]schema.Definition{"id": {
				Ref: "#/$defs/pageId",
			}, "title": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
				MaxLength: schema.Value(int(120)),
			}, "capability": {
				Ref: "#/$defs/capability",
			}, "permissions": {
				Type:        "array",
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/nonEmptyString",
				}),
			}, "sections": {
				Type:     "array",
				MinItems: schema.Value(int(1)),
				MaxItems: schema.Value(int(32)),
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/section",
				}),
			}},
		}},
	}
}

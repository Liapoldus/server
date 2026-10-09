package admin

import "liapoldus.local/server-plugin/contracts/schema"

func AdminQuerySchema() schema.Definition {
	return schema.Definition{
		Definition: "https://json-schema.org/draft/2020-12/schema",
		ID:         "https://liapoldus.github.io/plugins/server/v1/admin-query.schema.json",
		Type:       "object",
		Properties: map[string]schema.Definition{"resource": {
			Enum: []any{"sites", "releases", "operations", "certificates"},
		}},
		Required:             schema.Value([]string{"resource"}),
		AdditionalProperties: true,
	}
}

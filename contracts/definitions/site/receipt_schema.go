package site

import "liapoldus.local/server-plugin/contracts/schema"

func ArtifactOperationResultSchema() schema.Definition {
	return schema.Definition{
		Definition:           "https://json-schema.org/draft/2020-12/schema",
		ID:                   "https://liapoldus.github.io/plugins/server/v1/artifact-operation-result.schema.json",
		Type:                 "object",
		AdditionalProperties: false,
		Required:             schema.Value([]string{"version", "operationId", "state"}),
		Properties: map[string]schema.Definition{"version": {
			Const: 1,
		}, "operationId": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
			MaxLength: schema.Value(int(256)),
		}, "state": {
			Const: "accepted",
		}},
	}
}

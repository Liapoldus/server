package site

import "liapoldus.local/server-plugin/contracts/schema"

func ArtifactMetadataSchema() schema.Definition {
	return schema.Definition{
		Definition:           "https://json-schema.org/draft/2020-12/schema",
		ID:                   "https://liapoldus.github.io/plugins/server/v1/artifact-metadata.schema.json",
		Type:                 "object",
		AdditionalProperties: false,
		Required:             schema.Value([]string{"version", "artifact", "payload"}),
		Properties: map[string]schema.Definition{"version": {
			Const: 1,
		}, "artifact": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"mediaType", "byteLength", "sha256"}),
			Properties: map[string]schema.Definition{"mediaType": {
				Const: "application/gzip",
			}, "byteLength": {
				Type:    "integer",
				Minimum: schema.Value(int(1)),
				Maximum: schema.Value(int(134217728)),
			}, "sha256": {
				Type:    "string",
				Pattern: "^sha256:[a-f0-9]{64}$",
			}},
		}, "payload": {
			Description:          "Server capability validates its own schema before accepting the artifact.",
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"siteId"}),
			Properties: map[string]schema.Definition{"siteId": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
				MaxLength: schema.Value(int(128)),
			}, "expectedCurrentRevision": {
				Type:    "string",
				Pattern: "^sha256:[a-f0-9]{64}$",
			}},
		}},
	}
}

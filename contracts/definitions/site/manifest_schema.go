package site

import "liapoldus.local/server-plugin/contracts/schema"

func SiteManifestSchema() schema.Definition {
	return schema.Definition{
		Definition:           "https://json-schema.org/draft/2020-12/schema",
		ID:                   "https://liapoldus.github.io/plugins/server/site-publish/v1/site-manifest.schema.json",
		Title:                "Server site archive manifest v1",
		Type:                 "object",
		AdditionalProperties: false,
		Required:             schema.Value([]string{"schemaVersion", "siteId", "documentRoot", "indexDocument"}),
		Properties: map[string]schema.Definition{"schemaVersion": {
			Const: 1,
		}, "siteId": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
			MaxLength: schema.Value(int(128)),
		}, "documentRoot": {
			Type:        "string",
			MinLength:   schema.Value(int(1)),
			Pattern:     "^(?:\\.|(?:[^./\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*|\\.[^./\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*|\\.\\.[^/\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*)(?:/(?:[^./\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*|\\.[^./\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*|\\.\\.[^/\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*))*)$",
			Description: "POSIX relative archive directory without ASCII control characters; dot denotes the archive root.",
		}, "indexDocument": {
			Type:        "string",
			MinLength:   schema.Value(int(1)),
			Pattern:     "^(?:[^./\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*|\\.[^./\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*|\\.\\.[^/\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*)(?:/(?:[^./\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*|\\.[^./\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*|\\.\\.[^/\\\\\\x00-\\x1f\\x7f][^/\\\\\\x00-\\x1f\\x7f]*))*$",
			Description: "POSIX relative regular-file path without ASCII control characters, resolved inside documentRoot.",
		}},
		AllOf: []schema.Definition{{
			If: schema.Value(schema.Definition{
				Properties: map[string]schema.Definition{"documentRoot": {
					Const: ".",
				}},
				Required: schema.Value([]string{"documentRoot"}),
			}),
			Then: schema.Value(schema.Definition{
				Properties: map[string]schema.Definition{"indexDocument": {
					Not: schema.Value(schema.Definition{
						Const: "site-manifest.json",
					}),
				}},
				Required: schema.Value([]string{"indexDocument"}),
			}),
		}},
	}
}

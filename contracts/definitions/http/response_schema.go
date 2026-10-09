package http

import "liapoldus.local/server-plugin/contracts/schema"

func HTTPResponseActionSchema() schema.Definition {
	return schema.Definition{
		Definition:           "https://json-schema.org/draft/2020-12/schema",
		ID:                   "https://liapoldus.github.io/plugins/server/contracts/v1/http-response-action.schema.json",
		Title:                "Server plugin HTTP response action",
		Type:                 "object",
		AdditionalProperties: false,
		Required:             schema.Value([]string{"status"}),
		Properties: map[string]schema.Definition{"status": {
			Type:    "integer",
			Minimum: schema.Value(int(200)),
			Maximum: schema.Value(int(599)),
		}, "headers": {
			Type: "object",
			PropertyNames: schema.Value(schema.Definition{
				Not: schema.Value(schema.Definition{
					Const: "Set-Cookie",
				}),
			}),
			AdditionalProperties: schema.Definition{
				Type:    "string",
				Pattern: "^[^\\r\\n]*$",
			},
		}, "cookies": {
			Type: "array",
			Items: schema.Value(schema.Definition{
				Ref: "#/$defs/cookie",
			}),
		}, "body": {
			Type: "string",
		}},
		Defs: map[string]schema.Definition{"cookie": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"name", "value"}),
			Properties: map[string]schema.Definition{"name": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
				Pattern:   "^[!#$%&'*+.^_`|~0-9A-Za-z-]+$",
			}, "value": {
				Type: "string",
			}, "path": {
				Type:    "string",
				Pattern: "^/",
			}, "domain": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "expires": {
				Type:   "string",
				Format: "date-time",
			}, "maxAge": {
				Type:    "integer",
				Minimum: schema.Value(int(-1)),
			}, "secure": {
				Type: "boolean",
			}, "httpOnly": {
				Type: "boolean",
			}, "sameSite": {
				Enum: []any{"lax", "strict", "none"},
			}},
			AllOf: []schema.Definition{{
				If: schema.Value(schema.Definition{
					Properties: map[string]schema.Definition{"sameSite": {
						Const: "none",
					}},
					Required: schema.Value([]string{"sameSite"}),
				}),
				Then: schema.Value(schema.Definition{
					Properties: map[string]schema.Definition{"secure": {
						Const: true,
					}},
					Required: schema.Value([]string{"secure"}),
				}),
			}},
		}},
	}
}

package http

import "liapoldus.local/server-service/contracts/schema"

func HTTPRequestParametersSchema() schema.Definition {
	return schema.Definition{
		Definition:           "https://json-schema.org/draft/2020-12/schema",
		ID:                   "https://liapoldus.github.io/services/server/http-request-parameters/v1/schema.json",
		Title:                "Server service typed HTTP request parameters v1",
		Description:          "Normalized, bounded values extracted and validated by Server from a route's server-http/v1 request contract before dispatch. The HTTP envelope may also retain raw path/query fields for request context; consumers should use this typed object for configured parameters.",
		Type:                 "object",
		AdditionalProperties: false,
		Required:             schema.Value([]string{"version"}),
		Properties: map[string]schema.Definition{
			"version": {Const: "server-http/v1"},
			"path":    {Ref: "#/$defs/parameterMap"},
			"query":   {Ref: "#/$defs/parameterMap"},
			"headers": {Ref: "#/$defs/parameterMap"},
			"body":    {},
		},
		Defs: map[string]schema.Definition{
			"scalar": {AnyOf: []schema.Definition{
				{Type: "string"},
				{Type: "integer"},
				{Type: "number"},
				{Type: "boolean"},
			}},
			"parameterValue": {AnyOf: []schema.Definition{
				{Ref: "#/$defs/scalar"},
				{Type: "array", Items: schema.Value(schema.Definition{Ref: "#/$defs/scalar"})},
			}},
			"parameterMap": {
				Type:                 "object",
				AdditionalProperties: schema.Value(schema.Definition{Ref: "#/$defs/parameterValue"}),
			},
		},
	}
}

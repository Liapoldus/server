package http

import "liapoldus.local/server-plugin/contracts/schema"

func HTTPStreamSchema() schema.Definition {
	return schema.Definition{
		Definition:           "https://json-schema.org/draft/2020-12/schema",
		ID:                   "https://liapoldus.github.io/spec/server/http-stream.v1.schema.json",
		Title:                "Server plugin HTTP stream envelope contract",
		Type:                 "object",
		AdditionalProperties: false,
		Required:             schema.Value([]string{"version", "kindField", "dataField", "requestField", "statusField", "headersField", "maxChunkBytes", "maxFrameBytes", "requestStartKind", "requestChunkKind", "requestEndKind", "responseStartKind", "responseChunkKind", "responseEndKind", "websocket", "sse"}),
		Properties: map[string]schema.Definition{"version": {
			Const: 1,
		}, "kindField": {
			Const: "kind",
		}, "dataField": {
			Const: "data",
		}, "requestField": {
			Const: "request",
		}, "statusField": {
			Const: "status",
		}, "headersField": {
			Const: "headers",
		}, "maxChunkBytes": {
			Type:    "integer",
			Minimum: schema.Value(int(1)),
		}, "maxFrameBytes": {
			Type:    "integer",
			Minimum: schema.Value(int(1)),
		}, "requestStartKind": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
		}, "requestChunkKind": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
		}, "requestEndKind": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
		}, "responseStartKind": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
		}, "responseChunkKind": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
		}, "responseEndKind": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
		}, "websocket": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"maxMessageBytes", "requestField", "handshakeKind", "acceptField", "subprotocolsField", "subprotocolField", "messageStartKind", "messageChunkKind", "messageEndKind", "messageTypeField", "textMessageType", "binaryMessageType", "closeKind", "codeField", "reasonField", "defaultRejectStatus"}),
			Properties: map[string]schema.Definition{"maxMessageBytes": {
				Type:    "integer",
				Minimum: schema.Value(int(1)),
			}, "requestField": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "handshakeKind": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "acceptField": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "subprotocolsField": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "subprotocolField": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "messageStartKind": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "messageChunkKind": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "messageEndKind": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "messageTypeField": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "textMessageType": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "binaryMessageType": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "closeKind": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "codeField": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "reasonField": {
				Type:      "string",
				MinLength: schema.Value(int(1)),
			}, "defaultRejectStatus": {
				Type:    "integer",
				Minimum: schema.Value(int(400)),
				Maximum: schema.Value(int(599)),
			}},
		}, "sse": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"eventKind", "contentType", "contentTypeHeader", "eventField", "dataField", "idField", "retryField", "eventPrefix", "dataPrefix", "idPrefix", "retryPrefix", "lineEnding", "eventTerminator", "maxRetryMillis"}),
			Properties: map[string]schema.Definition{"eventKind": {
				Const: "sse_event",
			}, "contentType": {
				Const: "text/event-stream",
			}, "contentTypeHeader": {
				Const: "Content-Type",
			}, "eventField": {
				Const: "event",
			}, "dataField": {
				Const: "data",
			}, "idField": {
				Const: "id",
			}, "retryField": {
				Const: "retry",
			}, "eventPrefix": {
				Const: "event: ",
			}, "dataPrefix": {
				Const: "data: ",
			}, "idPrefix": {
				Const: "id: ",
			}, "retryPrefix": {
				Const: "retry: ",
			}, "lineEnding": {
				Const: "\n",
			}, "eventTerminator": {
				Const: "\n",
			}, "maxRetryMillis": {
				Type:    "integer",
				Minimum: schema.Value(int(0)),
				Maximum: schema.Value(int(2147483647)),
			}},
		}},
	}
}

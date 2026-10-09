package http

import "liapoldus.local/server-plugin/contracts/schema"

type HTTPStreamVectorsDocumentScenariosItemExpected struct {
	MaxConcurrentPerInstance       *int      `json:"maxConcurrentPerInstance,omitempty"`
	IdleTimeoutMillis              *int      `json:"idleTimeoutMillis,omitempty"`
	MaxDurationMillis              *int      `json:"maxDurationMillis,omitempty"`
	TimeoutIsIndependentFromUnary  *bool     `json:"timeoutIsIndependentFromUnary,omitempty"`
	ExtraRequestStatus             *int      `json:"extraRequestStatus,omitempty"`
	TargetDispatch                 *bool     `json:"targetDispatch,omitempty"`
	Stream                         *string   `json:"stream,omitempty"`
	ReplaceResponse                *bool     `json:"replaceResponse,omitempty"`
	ForwardedBytes                 *string   `json:"forwardedBytes,omitempty"`
	RequestEndSent                 *bool     `json:"requestEndSent,omitempty"`
	Response                       *string   `json:"response,omitempty"`
	ContentType                    *string   `json:"contentType,omitempty"`
	Body                           *string   `json:"body,omitempty"`
	CancelStream                   *bool     `json:"cancelStream,omitempty"`
	Status                         *any      `json:"status,omitempty"`
	Subprotocol                    *string   `json:"subprotocol,omitempty"`
	Upgraded                       *bool     `json:"upgraded,omitempty"`
	MessageTypes                   *[]string `json:"messageTypes,omitempty"`
	MessageCount                   *int      `json:"messageCount,omitempty"`
	BoundariesPreserved            *bool     `json:"boundariesPreserved,omitempty"`
	CloseCode                      *int      `json:"closeCode,omitempty"`
	PluginReceivesOversizedMessage *bool     `json:"pluginReceivesOversizedMessage,omitempty"`
}

type HTTPStreamVectorsDocumentScenariosItemInput struct {
	Event *string `json:"event,omitempty"`
	Retry *int    `json:"retry,omitempty"`
	Data  string  `json:"data"`
	ID    string  `json:"id"`
}

type HTTPStreamVectorsDocumentScenariosItemPluginDecision struct {
	Subprotocol *string `json:"subprotocol,omitempty"`
	Status      *int    `json:"status,omitempty"`
	Accept      bool    `json:"accept"`
}

type HTTPStreamVectorsDocumentScenariosItemMessagesItem struct {
	Chunks       *[]string `json:"chunks,omitempty"`
	ChunksBase64 *[]string `json:"chunksBase64,omitempty"`
	Type         string    `json:"type"`
}

type HTTPStreamVectorsDocumentScenariosItem struct {
	Expected                 HTTPStreamVectorsDocumentScenariosItemExpected        `json:"expected"`
	ActivityEveryMillis      *int                                                  `json:"activityEveryMillis,omitempty"`
	TransferEncoding         *string                                               `json:"transferEncoding,omitempty"`
	IdleTimeoutMillis        *int                                                  `json:"idleTimeoutMillis,omitempty"`
	ResponseStarted          *bool                                                 `json:"responseStarted,omitempty"`
	MaxDurationMillis        *int                                                  `json:"maxDurationMillis,omitempty"`
	MessageBytes             *string                                               `json:"messageBytes,omitempty"`
	Mode                     *string                                               `json:"mode,omitempty"`
	RouteMaxConcurrency      *int                                                  `json:"routeMaxConcurrency,omitempty"`
	BodyLengthDeltaFromLimit *int                                                  `json:"bodyLengthDeltaFromLimit,omitempty"`
	Input                    *HTTPStreamVectorsDocumentScenariosItemInput          `json:"input,omitempty"`
	ClientSubprotocols       *[]string                                             `json:"clientSubprotocols,omitempty"`
	PluginDecision           *HTTPStreamVectorsDocumentScenariosItemPluginDecision `json:"pluginDecision,omitempty"`
	Messages                 *[]HTTPStreamVectorsDocumentScenariosItemMessagesItem `json:"messages,omitempty"`
	ID                       string                                                `json:"id"`
}

type HTTPStreamVectorsDocument struct {
	Contract  string                                   `json:"contract"`
	Scenarios []HTTPStreamVectorsDocumentScenariosItem `json:"scenarios"`
}

func HTTPStreamVectors() HTTPStreamVectorsDocument {
	return HTTPStreamVectorsDocument{
		Contract: "contracts/v1/http-dispatch.json",
		Scenarios: []HTTPStreamVectorsDocumentScenariosItem{{
			ID: "http-stream-limits-defaults",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				MaxConcurrentPerInstance:      schema.Value(int(128)),
				IdleTimeoutMillis:             schema.Value(int(60000)),
				MaxDurationMillis:             schema.Value(int(3600000)),
				TimeoutIsIndependentFromUnary: schema.Value(true),
			},
		}, {
			ID: "http-stream-route-concurrency",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				ExtraRequestStatus: schema.Value(int(503)),
				TargetDispatch:     schema.Value(false),
			},
			RouteMaxConcurrency: schema.Value(int(1)),
		}, {
			ID: "http-stream-idle-timeout",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				Stream: schema.Value("close-without-replacing-response"),
			},
			IdleTimeoutMillis: schema.Value(int(1000)),
			ResponseStarted:   schema.Value(true),
		}, {
			ID: "http-stream-max-duration",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				Stream:          schema.Value("close-at-duration-even-while-active"),
				ReplaceResponse: schema.Value(false),
			},
			MaxDurationMillis:   schema.Value(int(1800)),
			ActivityEveryMillis: schema.Value(int(70)),
		}, {
			ID: "http-stream-chunked-at-limit",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				ForwardedBytes: schema.Value("maxRequestBytes"),
				RequestEndSent: schema.Value(true),
				Response:       schema.Value("plugin-response"),
			},
			ResponseStarted:          schema.Value(false),
			Mode:                     schema.Value("http_stream"),
			TransferEncoding:         schema.Value("chunked"),
			BodyLengthDeltaFromLimit: schema.Value(int(0)),
		}, {
			ID: "sse-structured-event-serialization",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				ContentType: schema.Value("text/event-stream"),
				Body:        schema.Value("event: update\\ndata: first\\ndata: second\\nid: 17\\nretry: 1000\\n\\n"),
			},
			Mode: schema.Value("sse"),
			Input: schema.Value(HTTPStreamVectorsDocumentScenariosItemInput{
				Event: schema.Value("update"),
				Data:  "first\nsecond",
				ID:    "17",
				Retry: schema.Value(int(1000)),
			}),
		}, {
			ID: "sse-invalid-event-after-response-start",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				Stream:          schema.Value("close"),
				ReplaceResponse: schema.Value(false),
			},
			Mode: schema.Value("sse"),
			Input: schema.Value(HTTPStreamVectorsDocumentScenariosItemInput{
				Data: "safe",
				ID:   "bad\\nInjected: value",
			}),
		}, {
			ID: "http-stream-chunked-over-limit-before-response-start",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				ForwardedBytes: schema.Value("maxRequestBytes"),
				RequestEndSent: schema.Value(false),
				Response:       schema.Value("requestTooLargeStatus"),
				CancelStream:   schema.Value(true),
			},
			ResponseStarted:          schema.Value(false),
			Mode:                     schema.Value("http_stream"),
			TransferEncoding:         schema.Value("chunked"),
			BodyLengthDeltaFromLimit: schema.Value(int(1)),
		}, {
			ID: "http-stream-chunked-over-limit-after-response-start",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				ReplaceResponse: schema.Value(false),
				ForwardedBytes:  schema.Value("maxRequestBytes"),
				Response:        schema.Value("abort-stream"),
				CancelStream:    schema.Value(true),
			},
			ResponseStarted:          schema.Value(true),
			Mode:                     schema.Value("http_stream"),
			TransferEncoding:         schema.Value("chunked"),
			BodyLengthDeltaFromLimit: schema.Value(int(1)),
		}, {
			ID: "websocket-plugin-accepts-before-upgrade-and-selects-offered-subprotocol",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				Status:      schema.Value[any](101),
				Subprotocol: schema.Value("forms.v1"),
			},
			Mode:               schema.Value("websocket"),
			ClientSubprotocols: schema.Value([]string{"forms.v1", "forms.v2"}),
			PluginDecision: schema.Value(HTTPStreamVectorsDocumentScenariosItemPluginDecision{
				Accept:      true,
				Subprotocol: schema.Value("forms.v1"),
			}),
		}, {
			ID: "websocket-plugin-rejects-before-upgrade",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				Status:   schema.Value[any](403),
				Upgraded: schema.Value(false),
			},
			Mode: schema.Value("websocket"),
			PluginDecision: schema.Value(HTTPStreamVectorsDocumentScenariosItemPluginDecision{
				Accept: false,
				Status: schema.Value(int(403)),
			}),
		}, {
			ID: "websocket-unoffered-subprotocol-is-rejected-before-upgrade",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				Status:   schema.Value[any]("invalidResponseStatus"),
				Upgraded: schema.Value(false),
			},
			Mode:               schema.Value("websocket"),
			ClientSubprotocols: schema.Value([]string{"forms.v1"}),
			PluginDecision: schema.Value(HTTPStreamVectorsDocumentScenariosItemPluginDecision{
				Accept:      true,
				Subprotocol: schema.Value("forms.private"),
			}),
		}, {
			ID: "websocket-text-binary-message-boundaries",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				MessageTypes:        schema.Value([]string{"text", "binary"}),
				MessageCount:        schema.Value(int(2)),
				BoundariesPreserved: schema.Value(true),
			},
			Mode: schema.Value("websocket"),
			Messages: schema.Value([]HTTPStreamVectorsDocumentScenariosItemMessagesItem{{
				Type:   "text",
				Chunks: schema.Value([]string{"hel", "lo"}),
			}, {
				Type:         "binary",
				ChunksBase64: schema.Value([]string{"AAEC", "/w=="}),
			}}),
		}, {
			ID: "websocket-message-over-limit-closes-with-1009",
			Expected: HTTPStreamVectorsDocumentScenariosItemExpected{
				CloseCode:                      schema.Value(int(1009)),
				PluginReceivesOversizedMessage: schema.Value(false),
			},
			Mode:         schema.Value("websocket"),
			MessageBytes: schema.Value("maxMessageBytes+1"),
		}},
	}
}

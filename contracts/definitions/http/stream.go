package http

type HTTPStreamDocumentWebsocket struct {
	MessageEndKind      string `json:"messageEndKind"`
	MessageTypeField    string `json:"messageTypeField"`
	HandshakeKind       string `json:"handshakeKind"`
	AcceptField         string `json:"acceptField"`
	SubprotocolsField   string `json:"subprotocolsField"`
	SubprotocolField    string `json:"subprotocolField"`
	RequestField        string `json:"requestField"`
	MessageStartKind    string `json:"messageStartKind"`
	ReasonField         string `json:"reasonField"`
	MessageChunkKind    string `json:"messageChunkKind"`
	TextMessageType     string `json:"textMessageType"`
	BinaryMessageType   string `json:"binaryMessageType"`
	CloseKind           string `json:"closeKind"`
	CodeField           string `json:"codeField"`
	MaxMessageBytes     int    `json:"maxMessageBytes"`
	DefaultRejectStatus int    `json:"defaultRejectStatus"`
}

type HTTPStreamDocumentSse struct {
	EventKind         string `json:"eventKind"`
	ContentType       string `json:"contentType"`
	ContentTypeHeader string `json:"contentTypeHeader"`
	EventField        string `json:"eventField"`
	DataField         string `json:"dataField"`
	IdField           string `json:"idField"`
	RetryField        string `json:"retryField"`
	EventPrefix       string `json:"eventPrefix"`
	DataPrefix        string `json:"dataPrefix"`
	IdPrefix          string `json:"idPrefix"`
	RetryPrefix       string `json:"retryPrefix"`
	LineEnding        string `json:"lineEnding"`
	EventTerminator   string `json:"eventTerminator"`
	MaxRetryMillis    int    `json:"maxRetryMillis"`
}

type HTTPStreamDocument struct {
	RequestStartKind  string                      `json:"requestStartKind"`
	RequestChunkKind  string                      `json:"requestChunkKind"`
	DataField         string                      `json:"dataField"`
	RequestField      string                      `json:"requestField"`
	StatusField       string                      `json:"statusField"`
	HeadersField      string                      `json:"headersField"`
	KindField         string                      `json:"kindField"`
	ResponseEndKind   string                      `json:"responseEndKind"`
	ResponseChunkKind string                      `json:"responseChunkKind"`
	ResponseStartKind string                      `json:"responseStartKind"`
	RequestEndKind    string                      `json:"requestEndKind"`
	Websocket         HTTPStreamDocumentWebsocket `json:"websocket"`
	Sse               HTTPStreamDocumentSse       `json:"sse"`
	MaxFrameBytes     int                         `json:"maxFrameBytes"`
	Version           int                         `json:"version"`
	MaxChunkBytes     int                         `json:"maxChunkBytes"`
}

func HTTPStream() HTTPStreamDocument {
	return HTTPStreamDocument{
		Version:           int(1),
		KindField:         "kind",
		DataField:         "data",
		RequestField:      "request",
		StatusField:       "status",
		HeadersField:      "headers",
		MaxChunkBytes:     int(24000),
		MaxFrameBytes:     int(32768),
		RequestStartKind:  "request_start",
		RequestChunkKind:  "request_chunk",
		RequestEndKind:    "request_end",
		ResponseStartKind: "response_start",
		ResponseChunkKind: "response_chunk",
		ResponseEndKind:   "response_end",
		Websocket: HTTPStreamDocumentWebsocket{
			MaxMessageBytes:     int(1048576),
			RequestField:        "websocket",
			HandshakeKind:       "websocket_handshake",
			AcceptField:         "accept",
			SubprotocolsField:   "subprotocols",
			SubprotocolField:    "subprotocol",
			MessageStartKind:    "websocket_message_start",
			MessageChunkKind:    "websocket_message_chunk",
			MessageEndKind:      "websocket_message_end",
			MessageTypeField:    "messageType",
			TextMessageType:     "text",
			BinaryMessageType:   "binary",
			CloseKind:           "websocket_close",
			CodeField:           "code",
			ReasonField:         "reason",
			DefaultRejectStatus: int(403),
		},
		Sse: HTTPStreamDocumentSse{
			EventKind:         "sse_event",
			ContentType:       "text/event-stream",
			ContentTypeHeader: "Content-Type",
			EventField:        "event",
			DataField:         "data",
			IdField:           "id",
			RetryField:        "retry",
			EventPrefix:       "event: ",
			DataPrefix:        "data: ",
			IdPrefix:          "id: ",
			RetryPrefix:       "retry: ",
			LineEnding:        "\n",
			EventTerminator:   "\n",
			MaxRetryMillis:    int(2147483647),
		},
	}
}

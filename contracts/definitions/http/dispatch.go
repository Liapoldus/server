package http

type HTTPDispatchDocumentRequestBodySemantics struct {
	Count                       string   `json:"count"`
	OverflowBeforeResponseStart string   `json:"overflowBeforeResponseStart"`
	OverflowAfterResponseStart  string   `json:"overflowAfterResponseStart"`
	Modes                       []string `json:"modes"`
	ChunkedIncluded             bool     `json:"chunkedIncluded"`
}

type HTTPDispatchDocumentResponseCookies struct {
	Field                      string   `json:"field"`
	Schema                     string   `json:"schema"`
	SameSiteValues             []string `json:"sameSiteValues"`
	MaxAgeMinimum              int      `json:"maxAgeMinimum"`
	SameSiteNoneRequiresSecure bool     `json:"sameSiteNoneRequiresSecure"`
	ValidateAllBeforeHeaders   bool     `json:"validateAllBeforeHeaders"`
}

type HTTPDispatchDocument struct {
	RequestBodySemantics            HTTPDispatchDocumentRequestBodySemantics `json:"requestBodySemantics"`
	Module                          string                                   `json:"module"`
	App                             string                                   `json:"app"`
	SetCookieHeader                 string                                   `json:"setCookieHeader"`
	CookieHeader                    string                                   `json:"cookieHeader"`
	RequestIDHeader                 string                                   `json:"requestIDHeader"`
	RequestHeaderByteSemantics      string                                   `json:"requestHeaderByteSemantics"`
	HeaderLimitModule               string                                   `json:"headerLimitModule"`
	BlockedHeaders                  []string                                 `json:"blockedHeaders"`
	ResponseCookies                 HTTPDispatchDocumentResponseCookies      `json:"responseCookies"`
	RequestHeaderTooLargeStatus     int                                      `json:"requestHeaderTooLargeStatus"`
	MaxRequestHeaderBytes           int                                      `json:"maxRequestHeaderBytes"`
	UnavailableStatus               int                                      `json:"unavailableStatus"`
	InvalidResponseStatus           int                                      `json:"invalidResponseStatus"`
	RequestTooLargeStatus           int                                      `json:"requestTooLargeStatus"`
	StreamTimeoutStatus             int                                      `json:"streamTimeoutStatus"`
	InvalidRequestStatus            int                                      `json:"invalidRequestStatus"`
	MaxRequestBytes                 int                                      `json:"maxRequestBytes"`
	DefaultStreamMaxDurationMillis  int                                      `json:"defaultStreamMaxDurationMillis"`
	DefaultStreamIdleTimeoutMillis  int                                      `json:"defaultStreamIdleTimeoutMillis"`
	MaxStreamConcurrencyPerInstance int                                      `json:"maxStreamConcurrencyPerInstance"`
	DefaultTimeoutMillis            int                                      `json:"defaultTimeoutMillis"`
}

func HTTPDispatch() HTTPDispatchDocument {
	return HTTPDispatchDocument{
		Module:                          "http.handlers.liapoldus_plugin",
		App:                             "liapoldus.dispatch",
		DefaultTimeoutMillis:            int(5000),
		MaxStreamConcurrencyPerInstance: int(128),
		DefaultStreamIdleTimeoutMillis:  int(60000),
		DefaultStreamMaxDurationMillis:  int(3600000),
		MaxRequestBytes:                 int(1048576),
		MaxRequestHeaderBytes:           int(65536),
		RequestHeaderByteSemantics:      "each parsed field name, colon-space, value, and CRLF; includes Host; excludes request line and final empty line",
		HeaderLimitModule:               "http.handlers.liapoldus_request_header_limit",
		RequestHeaderTooLargeStatus:     int(431),
		RequestBodySemantics: HTTPDispatchDocumentRequestBodySemantics{
			Count:                       "body-octets-after-transfer-framing",
			ChunkedIncluded:             true,
			Modes:                       []string{"call", "http_stream", "sse"},
			OverflowBeforeResponseStart: "requestTooLargeStatus-and-cancel-stream",
			OverflowAfterResponseStart:  "abort-stream-without-replacing-response",
		},
		UnavailableStatus:     int(503),
		InvalidResponseStatus: int(502),
		RequestTooLargeStatus: int(413),
		StreamTimeoutStatus:   int(504),
		InvalidRequestStatus:  int(400),
		RequestIDHeader:       "X-Request-ID",
		CookieHeader:          "Cookie",
		SetCookieHeader:       "Set-Cookie",
		ResponseCookies: HTTPDispatchDocumentResponseCookies{
			Field:                      "cookies",
			Schema:                     "http-response-action.schema.json",
			SameSiteValues:             []string{"lax", "strict", "none"},
			MaxAgeMinimum:              int(-1),
			SameSiteNoneRequiresSecure: true,
			ValidateAllBeforeHeaders:   true,
		},
		BlockedHeaders: []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade", "Authorization"},
	}
}

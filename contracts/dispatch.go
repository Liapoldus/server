package contracts

import (
	"errors"
	httpdef "liapoldus.local/server-plugin/contracts/definitions/http"
)

type HTTPDispatch struct {
	Module                          string              `json:"module"`
	App                             string              `json:"app"`
	SetCookieHeader                 string              `json:"setCookieHeader"`
	CookieHeader                    string              `json:"cookieHeader"`
	RequestIDHeader                 string              `json:"requestIDHeader"`
	RequestHeaderByteSemantics      string              `json:"requestHeaderByteSemantics"`
	HeaderLimitModule               string              `json:"headerLimitModule"`
	Stream                          HTTPStream          `json:"-"`
	BlockedHeaders                  []string            `json:"blockedHeaders"`
	ResponseCookies                 HTTPResponseCookies `json:"responseCookies"`
	RequestHeaderTooLargeStatus     int                 `json:"requestHeaderTooLargeStatus"`
	UnavailableStatus               int                 `json:"unavailableStatus"`
	InvalidResponseStatus           int                 `json:"invalidResponseStatus"`
	RequestTooLargeStatus           int                 `json:"requestTooLargeStatus"`
	StreamTimeoutStatus             int                 `json:"streamTimeoutStatus"`
	InvalidRequestStatus            int                 `json:"invalidRequestStatus"`
	MaxRequestHeaderBytes           int                 `json:"maxRequestHeaderBytes"`
	MaxRequestBytes                 int64               `json:"maxRequestBytes"`
	DefaultStreamMaxDurationMillis  int                 `json:"defaultStreamMaxDurationMillis"`
	DefaultStreamIdleTimeoutMillis  int                 `json:"defaultStreamIdleTimeoutMillis"`
	MaxStreamConcurrencyPerInstance int                 `json:"maxStreamConcurrencyPerInstance"`
	DefaultTimeoutMillis            int                 `json:"defaultTimeoutMillis"`
}

type HTTPResponseCookies struct {
	Field                      string   `json:"field"`
	Schema                     string   `json:"schema"`
	SameSiteValues             []string `json:"sameSiteValues"`
	MaxAgeMinimum              int      `json:"maxAgeMinimum"`
	SameSiteNoneRequiresSecure bool     `json:"sameSiteNoneRequiresSecure"`
	ValidateAllBeforeHeaders   bool     `json:"validateAllBeforeHeaders"`
}

type HTTPStream struct {
	RequestStartKind  string              `json:"requestStartKind"`
	RequestChunkKind  string              `json:"requestChunkKind"`
	DataField         string              `json:"dataField"`
	RequestField      string              `json:"requestField"`
	StatusField       string              `json:"statusField"`
	HeadersField      string              `json:"headersField"`
	KindField         string              `json:"kindField"`
	ResponseEndKind   string              `json:"responseEndKind"`
	ResponseChunkKind string              `json:"responseChunkKind"`
	ResponseStartKind string              `json:"responseStartKind"`
	RequestEndKind    string              `json:"requestEndKind"`
	WebSocket         HTTPStreamWebSocket `json:"websocket"`
	SSE               HTTPStreamSSE       `json:"sse"`
	MaxFrameBytes     int                 `json:"maxFrameBytes"`
	Version           int                 `json:"version"`
	MaxChunkBytes     int                 `json:"maxChunkBytes"`
}

type HTTPStreamWebSocket struct {
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
	MaxMessageBytes     int64  `json:"maxMessageBytes"`
	DefaultRejectStatus int    `json:"defaultRejectStatus"`
}

type HTTPStreamSSE struct {
	EventKind         string `json:"eventKind"`
	ContentType       string `json:"contentType"`
	ContentTypeHeader string `json:"contentTypeHeader"`
	EventField        string `json:"eventField"`
	DataField         string `json:"dataField"`
	IDField           string `json:"idField"`
	RetryField        string `json:"retryField"`
	EventPrefix       string `json:"eventPrefix"`
	DataPrefix        string `json:"dataPrefix"`
	IDPrefix          string `json:"idPrefix"`
	RetryPrefix       string `json:"retryPrefix"`
	LineEnding        string `json:"lineEnding"`
	EventTerminator   string `json:"eventTerminator"`
	MaxRetryMillis    int64  `json:"maxRetryMillis"`
}

var ErrInvalidDispatchAssets = errors.New("")

func LoadHTTPDispatch() (HTTPDispatch, error) {
	v := httpdef.HTTPDispatch()
	stream, err := LoadHTTPStream()
	if err != nil {
		return HTTPDispatch{}, err
	}
	return HTTPDispatch{Module: v.Module,
		App:                             v.App,
		DefaultTimeoutMillis:            v.DefaultTimeoutMillis,
		MaxStreamConcurrencyPerInstance: v.MaxStreamConcurrencyPerInstance,
		DefaultStreamIdleTimeoutMillis:  v.DefaultStreamIdleTimeoutMillis,
		DefaultStreamMaxDurationMillis:  v.DefaultStreamMaxDurationMillis,
		MaxRequestBytes:                 int64(v.MaxRequestBytes),
		MaxRequestHeaderBytes:           v.MaxRequestHeaderBytes,
		RequestHeaderByteSemantics:      v.RequestHeaderByteSemantics,
		HeaderLimitModule:               v.HeaderLimitModule,
		RequestHeaderTooLargeStatus:     v.RequestHeaderTooLargeStatus,
		UnavailableStatus:               v.UnavailableStatus,
		InvalidResponseStatus:           v.InvalidResponseStatus,
		RequestTooLargeStatus:           v.RequestTooLargeStatus,
		StreamTimeoutStatus:             v.StreamTimeoutStatus,
		InvalidRequestStatus:            v.InvalidRequestStatus,
		RequestIDHeader:                 v.RequestIDHeader,
		CookieHeader:                    v.CookieHeader,
		SetCookieHeader:                 v.SetCookieHeader,
		ResponseCookies: HTTPResponseCookies{Field: v.ResponseCookies.Field,
			Schema:                     v.ResponseCookies.Schema,
			MaxAgeMinimum:              v.ResponseCookies.MaxAgeMinimum,
			SameSiteNoneRequiresSecure: v.ResponseCookies.SameSiteNoneRequiresSecure,
			ValidateAllBeforeHeaders:   v.ResponseCookies.ValidateAllBeforeHeaders, SameSiteValues: v.ResponseCookies.SameSiteValues},
		BlockedHeaders: v.BlockedHeaders, Stream: stream}, nil
}

func LoadHTTPStream() (HTTPStream, error) {
	v := httpdef.HTTPStream()
	return HTTPStream{Version: v.Version,
		KindField:         v.KindField,
		DataField:         v.DataField,
		RequestField:      v.RequestField,
		StatusField:       v.StatusField,
		HeadersField:      v.HeadersField,
		MaxChunkBytes:     v.MaxChunkBytes,
		MaxFrameBytes:     v.MaxFrameBytes,
		RequestStartKind:  v.RequestStartKind,
		RequestChunkKind:  v.RequestChunkKind,
		RequestEndKind:    v.RequestEndKind,
		ResponseStartKind: v.ResponseStartKind,
		ResponseChunkKind: v.ResponseChunkKind,
		ResponseEndKind:   v.ResponseEndKind,
		WebSocket: HTTPStreamWebSocket{MaxMessageBytes: int64(v.Websocket.MaxMessageBytes),
			RequestField:        v.Websocket.RequestField,
			HandshakeKind:       v.Websocket.HandshakeKind,
			AcceptField:         v.Websocket.AcceptField,
			SubprotocolsField:   v.Websocket.SubprotocolsField,
			SubprotocolField:    v.Websocket.SubprotocolField,
			MessageStartKind:    v.Websocket.MessageStartKind,
			MessageChunkKind:    v.Websocket.MessageChunkKind,
			MessageEndKind:      v.Websocket.MessageEndKind,
			MessageTypeField:    v.Websocket.MessageTypeField,
			TextMessageType:     v.Websocket.TextMessageType,
			BinaryMessageType:   v.Websocket.BinaryMessageType,
			CloseKind:           v.Websocket.CloseKind,
			CodeField:           v.Websocket.CodeField,
			ReasonField:         v.Websocket.ReasonField,
			DefaultRejectStatus: v.Websocket.DefaultRejectStatus},
		SSE: HTTPStreamSSE{EventKind: v.Sse.EventKind,
			ContentType:       v.Sse.ContentType,
			ContentTypeHeader: v.Sse.ContentTypeHeader,
			EventField:        v.Sse.EventField,
			DataField:         v.Sse.DataField,
			IDField:           v.Sse.IdField,
			RetryField:        v.Sse.RetryField,
			EventPrefix:       v.Sse.EventPrefix,
			DataPrefix:        v.Sse.DataPrefix,
			IDPrefix:          v.Sse.IdPrefix,
			RetryPrefix:       v.Sse.RetryPrefix,
			LineEnding:        v.Sse.LineEnding,
			EventTerminator:   v.Sse.EventTerminator,
			MaxRetryMillis:    int64(v.Sse.MaxRetryMillis)}}, nil
}

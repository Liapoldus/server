package contracts

import (
	"encoding/json"
	"errors"
	"io/fs"
)

type HTTPDispatch struct {
	Module                          string              `json:"module"`
	App                             string              `json:"app"`
	DefaultTimeoutMillis            int                 `json:"defaultTimeoutMillis"`
	MaxStreamConcurrencyPerInstance int                 `json:"maxStreamConcurrencyPerInstance"`
	DefaultStreamIdleTimeoutMillis  int                 `json:"defaultStreamIdleTimeoutMillis"`
	DefaultStreamMaxDurationMillis  int                 `json:"defaultStreamMaxDurationMillis"`
	MaxRequestBytes                 int64               `json:"maxRequestBytes"`
	MaxRequestHeaderBytes           int                 `json:"maxRequestHeaderBytes"`
	RequestHeaderByteSemantics      string              `json:"requestHeaderByteSemantics"`
	HeaderLimitModule               string              `json:"headerLimitModule"`
	RequestHeaderTooLargeStatus     int                 `json:"requestHeaderTooLargeStatus"`
	UnavailableStatus               int                 `json:"unavailableStatus"`
	InvalidResponseStatus           int                 `json:"invalidResponseStatus"`
	RequestTooLargeStatus           int                 `json:"requestTooLargeStatus"`
	StreamTimeoutStatus             int                 `json:"streamTimeoutStatus"`
	InvalidRequestStatus            int                 `json:"invalidRequestStatus"`
	RequestIDHeader                 string              `json:"requestIDHeader"`
	CookieHeader                    string              `json:"cookieHeader"`
	SetCookieHeader                 string              `json:"setCookieHeader"`
	ResponseCookies                 HTTPResponseCookies `json:"responseCookies"`
	BlockedHeaders                  []string            `json:"blockedHeaders"`
	Stream                          HTTPStream          `json:"-"`
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
	Version           int                 `json:"version"`
	KindField         string              `json:"kindField"`
	DataField         string              `json:"dataField"`
	RequestField      string              `json:"requestField"`
	StatusField       string              `json:"statusField"`
	HeadersField      string              `json:"headersField"`
	MaxChunkBytes     int                 `json:"maxChunkBytes"`
	MaxFrameBytes     int                 `json:"maxFrameBytes"`
	RequestStartKind  string              `json:"requestStartKind"`
	RequestChunkKind  string              `json:"requestChunkKind"`
	RequestEndKind    string              `json:"requestEndKind"`
	ResponseStartKind string              `json:"responseStartKind"`
	ResponseChunkKind string              `json:"responseChunkKind"`
	ResponseEndKind   string              `json:"responseEndKind"`
	WebSocket         HTTPStreamWebSocket `json:"websocket"`
	SSE               HTTPStreamSSE       `json:"sse"`
}

type HTTPStreamWebSocket struct {
	MaxMessageBytes     int64  `json:"maxMessageBytes"`
	RequestField        string `json:"requestField"`
	HandshakeKind       string `json:"handshakeKind"`
	AcceptField         string `json:"acceptField"`
	SubprotocolsField   string `json:"subprotocolsField"`
	SubprotocolField    string `json:"subprotocolField"`
	MessageStartKind    string `json:"messageStartKind"`
	MessageChunkKind    string `json:"messageChunkKind"`
	MessageEndKind      string `json:"messageEndKind"`
	MessageTypeField    string `json:"messageTypeField"`
	TextMessageType     string `json:"textMessageType"`
	BinaryMessageType   string `json:"binaryMessageType"`
	CloseKind           string `json:"closeKind"`
	CodeField           string `json:"codeField"`
	ReasonField         string `json:"reasonField"`
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
	contents, err := fs.ReadFile(files, "v1/http-dispatch.json")
	if err != nil {
		return HTTPDispatch{}, ErrInvalidDispatchAssets
	}
	var contract HTTPDispatch
	if err := json.Unmarshal(contents, &contract); err != nil || contract.Module == "" || contract.App == "" || contract.DefaultTimeoutMillis < 1 || contract.MaxStreamConcurrencyPerInstance < 1 || contract.DefaultStreamIdleTimeoutMillis < 1 || contract.DefaultStreamMaxDurationMillis < 1 || contract.MaxRequestBytes < 1 || contract.MaxRequestHeaderBytes < 1 || contract.RequestHeaderByteSemantics == "" || contract.HeaderLimitModule == "" || contract.RequestHeaderTooLargeStatus != 431 || contract.UnavailableStatus < 100 || contract.InvalidResponseStatus < 100 || contract.RequestTooLargeStatus < 100 || contract.StreamTimeoutStatus < 100 || contract.InvalidRequestStatus < 100 || len(contract.BlockedHeaders) == 0 || contract.CookieHeader == "" || contract.SetCookieHeader == "" || contract.ResponseCookies.Field != "cookies" || contract.ResponseCookies.Schema == "" || len(contract.ResponseCookies.SameSiteValues) != 3 || contract.ResponseCookies.MaxAgeMinimum != -1 || !contract.ResponseCookies.SameSiteNoneRequiresSecure || !contract.ResponseCookies.ValidateAllBeforeHeaders {
		return HTTPDispatch{}, ErrInvalidDispatchAssets
	}
	stream, err := LoadHTTPStream()
	if err != nil {
		return HTTPDispatch{}, err
	}
	contract.Stream = stream
	return contract, nil
}

func LoadHTTPStream() (HTTPStream, error) {
	contents, err := fs.ReadFile(files, "v1/http-stream.json")
	if err != nil {
		return HTTPStream{}, ErrInvalidDispatchAssets
	}
	var contract HTTPStream
	if err := json.Unmarshal(contents, &contract); err != nil || contract.Version != 1 || contract.KindField == "" || contract.DataField == "" || contract.RequestField == "" || contract.StatusField == "" || contract.HeadersField == "" || contract.MaxChunkBytes < 1 || contract.MaxFrameBytes < contract.MaxChunkBytes || contract.RequestStartKind == "" || contract.RequestChunkKind == "" || contract.RequestEndKind == "" || contract.ResponseStartKind == "" || contract.ResponseChunkKind == "" || contract.ResponseEndKind == "" || contract.WebSocket.MaxMessageBytes < 1 || contract.WebSocket.RequestField == "" || contract.WebSocket.HandshakeKind == "" || contract.WebSocket.AcceptField == "" || contract.WebSocket.SubprotocolsField == "" || contract.WebSocket.SubprotocolField == "" || contract.WebSocket.MessageStartKind == "" || contract.WebSocket.MessageChunkKind == "" || contract.WebSocket.MessageEndKind == "" || contract.WebSocket.MessageTypeField == "" || contract.WebSocket.TextMessageType == "" || contract.WebSocket.BinaryMessageType == "" || contract.WebSocket.CloseKind == "" || contract.WebSocket.CodeField == "" || contract.WebSocket.ReasonField == "" || contract.WebSocket.DefaultRejectStatus < 400 || contract.WebSocket.DefaultRejectStatus > 599 || contract.SSE.EventKind == "" || contract.SSE.ContentType == "" || contract.SSE.ContentTypeHeader == "" || contract.SSE.EventField == "" || contract.SSE.DataField == "" || contract.SSE.IDField == "" || contract.SSE.RetryField == "" || contract.SSE.EventPrefix == "" || contract.SSE.DataPrefix == "" || contract.SSE.IDPrefix == "" || contract.SSE.RetryPrefix == "" || contract.SSE.LineEnding == "" || contract.SSE.EventTerminator == "" || contract.SSE.MaxRetryMillis < 0 {
		return HTTPStream{}, ErrInvalidDispatchAssets
	}
	return contract, nil
}

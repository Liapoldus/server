package caddy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
	caddycore "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/gorilla/websocket"
	"liapoldus.local/server-plugin/contracts"
)

type dispatchInstance struct {
	ID            string   `json:"id"`
	Endpoint      string   `json:"endpoint"`
	TimeoutMillis int      `json:"timeoutMillis,omitempty"`
	Methods       []string `json:"methods"`
}

type dispatchApp struct {
	Instances   []dispatchInstance `json:"instances,omitempty"`
	TargetSetID uint64             `json:"targetSetId,omitempty"`
	bindings    map[string]*dispatchBinding
	contract    contracts.HTTPDispatch
}

type dispatchBinding struct {
	mu       sync.Mutex
	client   peer.Client
	endpoint string
	security peer.SecurityConfig
	handler  peer.Handler
	timeout  time.Duration
	calls    map[string]struct{}
	streams  chan struct{}
}

type httpDispatchHandler struct {
	Instance              string   `json:"instance,omitempty"`
	Capability            string   `json:"capability,omitempty"`
	Mode                  string   `json:"mode,omitempty"`
	RequestCookieNames    []string `json:"requestCookieNames,omitempty"`
	MaxConcurrentStreams  int      `json:"maxConcurrentStreams,omitempty"`
	IdleTimeoutMillis     int      `json:"idleTimeoutMillis,omitempty"`
	MaxDurationMillis     int      `json:"maxDurationMillis,omitempty"`
	binding               *dispatchBinding
	allowedRequestCookies map[string]struct{}
	contract              contracts.HTTPDispatch
	streams               chan struct{}
	idleTimeout           time.Duration
	maxDuration           time.Duration
}

type httpRequestPayload struct {
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Query      string            `json:"query,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Cookies    []cookiePair      `json:"cookies,omitempty"`
	Body       []byte            `json:"body,omitempty"`
	RequestID  string            `json:"requestId"`
	RemoteAddr string            `json:"remoteAddr,omitempty"`
}

type cookiePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type httpResponseAction struct {
	Status  int                `json:"status"`
	Headers map[string]string  `json:"headers,omitempty"`
	Cookies []httpCookieAction `json:"cookies,omitempty"`
	Body    *string            `json:"body,omitempty"`
}

type httpCookieAction struct {
	Name     string     `json:"name"`
	Value    string     `json:"value"`
	Path     string     `json:"path,omitempty"`
	Domain   string     `json:"domain,omitempty"`
	Expires  *time.Time `json:"expires,omitempty"`
	MaxAge   int        `json:"maxAge,omitempty"`
	Secure   bool       `json:"secure,omitempty"`
	HTTPOnly bool       `json:"httpOnly,omitempty"`
	SameSite string     `json:"sameSite,omitempty"`
}

var errHTTPStreamRequestTooLarge = errors.New("")
var errHTTPStreamIdleTimeout = errors.New("")
var errHTTPStreamMaxDuration = errors.New("")

func init() {
	_, err := contracts.LoadHTTPDispatch()
	if err != nil {
		panic(err)
	}
	caddycore.RegisterModule(dispatchApp{})
	caddycore.RegisterModule(httpDispatchHandler{})
}

func (dispatchApp) CaddyModule() caddycore.ModuleInfo {
	contract, _ := contracts.LoadHTTPDispatch()
	return caddycore.ModuleInfo{ID: caddycore.ModuleID(contract.App), New: func() caddycore.Module { return new(dispatchApp) }}
}

func (app *dispatchApp) Provision(_ caddycore.Context) error {
	contract, err := contracts.LoadHTTPDispatch()
	if err != nil {
		return err
	}
	app.contract = contract
	app.bindings = make(map[string]*dispatchBinding, len(app.Instances))
	for _, instance := range app.Instances {
		if instance.ID == "" || instance.Endpoint == "" || len(instance.Methods) == 0 {
			app.close()
			return contracts.ErrInvalidDispatchAssets
		}
		security, ok := lookupDispatchSecurity(app.TargetSetID, instance.ID, instance.Endpoint)
		if !ok {
			app.close()
			return contracts.ErrInvalidDispatchAssets
		}
		if _, duplicate := app.bindings[instance.ID]; duplicate {
			app.close()
			return contracts.ErrInvalidDispatchAssets
		}
		methods := make(map[string]struct{}, len(instance.Methods))
		for _, method := range instance.Methods {
			if method == "" {
				app.close()
				return contracts.ErrInvalidDispatchAssets
			}
			if _, duplicate := methods[method]; duplicate {
				app.close()
				return contracts.ErrInvalidDispatchAssets
			}
			methods[method] = struct{}{}
		}
		timeout := time.Duration(instance.TimeoutMillis) * time.Millisecond
		if timeout <= 0 {
			timeout = time.Duration(contract.DefaultTimeoutMillis) * time.Millisecond
		}
		registry, buildErr := peer.NewRegistry().Build()
		if buildErr != nil {
			app.close()
			return contracts.ErrInvalidDispatchAssets
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		client, dialErr := peer.Dial(ctx, peer.ClientConfig{
			Network:  peer.NetworkConfig{Carrier: peer.CarrierTCP, Endpoint: instance.Endpoint},
			Security: security,
			Handler:  registry,
		})
		cancel()
		if dialErr != nil {
			app.close()
			return peer.ErrUnavailable
		}
		app.bindings[instance.ID] = &dispatchBinding{
			client: client, endpoint: instance.Endpoint, security: security,
			handler: registry, timeout: timeout, calls: methods,
			streams: make(chan struct{}, contract.MaxStreamConcurrencyPerInstance),
		}
	}
	return nil
}

func (app *dispatchApp) Start() error   { return nil }
func (app *dispatchApp) Stop() error    { app.close(); return nil }
func (app *dispatchApp) Cleanup() error { app.close(); return nil }

func (app *dispatchApp) close() {
	for _, binding := range app.bindings {
		binding.close()
	}
	app.bindings = nil
}

func (httpDispatchHandler) CaddyModule() caddycore.ModuleInfo {
	contract, _ := contracts.LoadHTTPDispatch()
	return caddycore.ModuleInfo{ID: caddycore.ModuleID(contract.Module), New: func() caddycore.Module { return new(httpDispatchHandler) }}
}

func (handler *httpDispatchHandler) Provision(ctx caddycore.Context) error {
	contract, err := contracts.LoadHTTPDispatch()
	if err != nil {
		return err
	}
	handler.contract = contract
	value, err := ctx.App(contract.App)
	if err != nil {
		return err
	}
	app, ok := value.(*dispatchApp)
	if !ok {
		return peer.ErrUnavailable
	}
	binding, ok := app.bindings[handler.Instance]
	if !ok {
		return peer.ErrUnavailable
	}
	if _, supported := binding.calls[handler.Capability]; !supported {
		return peer.ErrMethodNotFound
	}
	allowedCookies := make(map[string]struct{}, len(handler.RequestCookieNames))
	for _, name := range handler.RequestCookieNames {
		if name == "" || (&http.Cookie{Name: name, Value: "value"}).Valid() != nil {
			return contracts.ErrInvalidDispatchAssets
		}
		if _, duplicate := allowedCookies[name]; duplicate {
			return contracts.ErrInvalidDispatchAssets
		}
		allowedCookies[name] = struct{}{}
	}
	handler.allowedRequestCookies = allowedCookies
	handler.binding = binding
	if handler.Mode == "http_stream" || handler.Mode == "websocket" || handler.Mode == "sse" {
		if handler.MaxConcurrentStreams == 0 {
			handler.MaxConcurrentStreams = contract.MaxStreamConcurrencyPerInstance
		}
		if handler.IdleTimeoutMillis == 0 {
			handler.IdleTimeoutMillis = contract.DefaultStreamIdleTimeoutMillis
		}
		if handler.MaxDurationMillis == 0 {
			handler.MaxDurationMillis = contract.DefaultStreamMaxDurationMillis
		}
		if handler.MaxConcurrentStreams < 1 || handler.MaxConcurrentStreams > contract.MaxStreamConcurrencyPerInstance || handler.IdleTimeoutMillis < 1 || handler.IdleTimeoutMillis > contract.DefaultStreamIdleTimeoutMillis || handler.MaxDurationMillis < 1 || handler.MaxDurationMillis > contract.DefaultStreamMaxDurationMillis {
			return contracts.ErrInvalidDispatchAssets
		}
		handler.streams = make(chan struct{}, handler.MaxConcurrentStreams)
		handler.idleTimeout = time.Duration(handler.IdleTimeoutMillis) * time.Millisecond
		handler.maxDuration = time.Duration(handler.MaxDurationMillis) * time.Millisecond
	}
	return nil
}

func (handler *httpDispatchHandler) requestCookies(request *http.Request) []cookiePair {
	if len(handler.allowedRequestCookies) == 0 {
		return nil
	}
	var allowed []cookiePair
	for _, cookie := range request.Cookies() {
		if _, ok := handler.allowedRequestCookies[cookie.Name]; ok {
			allowed = append(allowed, cookiePair{Name: cookie.Name, Value: cookie.Value})
		}
	}
	return allowed
}

func (handler *httpDispatchHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request, _ caddyhttp.Handler) error {
	if handler.Instance == "" || handler.Capability == "" {
		writer.WriteHeader(handler.contract.InvalidRequestStatus)
		return nil
	}
	blocked := make(map[string]struct{}, len(handler.contract.BlockedHeaders)+1)
	for _, name := range handler.contract.BlockedHeaders {
		blocked[http.CanonicalHeaderKey(name)] = struct{}{}
	}
	// Never forward the raw Cookie header. Only values selected by this route's
	// explicit requestCookieNames allow-list cross the plugin boundary below.
	blocked[http.CanonicalHeaderKey(handler.contract.CookieHeader)] = struct{}{}
	headers := make(map[string]string, len(request.Header))
	for name, values := range request.Header {
		if _, skip := blocked[http.CanonicalHeaderKey(name)]; skip || len(values) == 0 {
			continue
		}
		headers[name] = strings.Join(values, ",")
	}
	if handler.Mode == "websocket" {
		return handler.serveWebSocket(writer, request, headers)
	}
	if handler.Mode == "http_stream" || handler.Mode == "sse" {
		return handler.serveStream(writer, request, headers)
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, handler.contract.MaxRequestBytes+1))
	if err != nil || int64(len(body)) > handler.contract.MaxRequestBytes {
		writer.WriteHeader(handler.contract.RequestTooLargeStatus)
		return nil
	}
	payload, err := json.Marshal(httpRequestPayload{
		Method: request.Method, Path: request.URL.EscapedPath(), Query: request.URL.RawQuery,
		Headers: headers, Cookies: handler.requestCookies(request), Body: body, RequestID: request.Header.Get(handler.contract.RequestIDHeader), RemoteAddr: request.RemoteAddr,
	})
	if err != nil {
		writer.WriteHeader(handler.contract.InvalidRequestStatus)
		return nil
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.binding.timeout)
	defer cancel()
	response, err := handler.binding.call(ctx, peer.Method(handler.Capability), payload)
	if err != nil {
		writer.WriteHeader(handler.contract.UnavailableStatus)
		return nil
	}
	action, err := decodeHTTPResponseAction(response.Payload, handler.contract)
	if err != nil {
		writer.WriteHeader(handler.contract.InvalidResponseStatus)
		return nil
	}
	for name, value := range action.Headers {
		writer.Header().Set(name, value)
	}
	writeHTTPResponseCookies(writer.Header(), action.Cookies, handler.contract.SetCookieHeader)
	writer.WriteHeader(action.Status)
	if action.Body != nil {
		_, _ = io.WriteString(writer, *action.Body)
	}
	return nil
}

func (handler *httpDispatchHandler) serveWebSocket(writer http.ResponseWriter, request *http.Request, headers map[string]string) error {
	contract := handler.contract.Stream
	ws := contract.WebSocket
	if !websocket.IsWebSocketUpgrade(request) || request.Method != http.MethodGet {
		writer.WriteHeader(handler.contract.InvalidRequestStatus)
		return nil
	}
	defer request.Body.Close()
	select {
	case handler.streams <- struct{}{}:
		defer func() { <-handler.streams }()
	default:
		writer.WriteHeader(handler.contract.UnavailableStatus)
		return nil
	}
	select {
	case handler.binding.streams <- struct{}{}:
		defer func() { <-handler.binding.streams }()
	default:
		writer.WriteHeader(handler.contract.UnavailableStatus)
		return nil
	}
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	stream, err := handler.binding.openStream(ctx, peer.Method(handler.Capability))
	if err != nil {
		writer.WriteHeader(handler.contract.UnavailableStatus)
		return nil
	}
	requestPayload := httpRequestPayload{Method: request.Method, Path: request.URL.EscapedPath(), Query: request.URL.RawQuery,
		Headers: headers, Cookies: handler.requestCookies(request), RequestID: request.Header.Get(handler.contract.RequestIDHeader), RemoteAddr: request.RemoteAddr}
	start, err := json.Marshal(map[string]any{
		contract.KindField:    contract.RequestStartKind,
		contract.RequestField: requestPayload,
		ws.RequestField:       map[string]any{ws.SubprotocolsField: websocket.Subprotocols(request)},
	})
	if err != nil || len(start) > contract.MaxFrameBytes || sendPeerMessage(stream, peer.Message{Payload: start}) != nil {
		writer.WriteHeader(handler.contract.UnavailableStatus)
		return nil
	}
	type received struct {
		message peer.Message
		err     error
	}
	handshakeResult := make(chan received, 1)
	go func() {
		message, receiveErr := stream.Recv()
		handshakeResult <- received{message: message, err: receiveErr}
	}()
	timer := time.NewTimer(handler.idleTimeout)
	defer timer.Stop()
	maxTimer := time.NewTimer(handler.maxDuration)
	defer maxTimer.Stop()
	var first received
	select {
	case first = <-handshakeResult:
	case <-timer.C:
		writer.WriteHeader(handler.contract.StreamTimeoutStatus)
		return nil
	case <-maxTimer.C:
		writer.WriteHeader(handler.contract.StreamTimeoutStatus)
		return nil
	case <-request.Context().Done():
		return nil
	}
	if first.err != nil || len(first.message.Payload) > contract.MaxFrameBytes || !contracts.ValidJSONNoDuplicateKeys(first.message.Payload) {
		writer.WriteHeader(handler.contract.InvalidResponseStatus)
		return nil
	}
	var handshake map[string]json.RawMessage
	if json.Unmarshal(first.message.Payload, &handshake) != nil || len(handshake) < 2 {
		writer.WriteHeader(handler.contract.InvalidResponseStatus)
		return nil
	}
	var kind string
	var accepted bool
	if json.Unmarshal(handshake[contract.KindField], &kind) != nil || kind != ws.HandshakeKind || json.Unmarshal(handshake[ws.AcceptField], &accepted) != nil {
		writer.WriteHeader(handler.contract.InvalidResponseStatus)
		return nil
	}
	var responseCookies []httpCookieAction
	cookieRaw, hasCookies := handshake[handler.contract.ResponseCookies.Field]
	if hasCookies && (string(cookieRaw) == "null" || json.Unmarshal(cookieRaw, &responseCookies) != nil || !validHTTPResponseCookies(responseCookies, handler.contract)) {
		writer.WriteHeader(handler.contract.InvalidResponseStatus)
		return nil
	}
	if !accepted {
		status := ws.DefaultRejectStatus
		if rawStatus, exists := handshake[contract.StatusField]; exists && json.Unmarshal(rawStatus, &status) != nil {
			writer.WriteHeader(handler.contract.InvalidResponseStatus)
			return nil
		}
		if status < 400 || status > 599 {
			writer.WriteHeader(handler.contract.InvalidResponseStatus)
			return nil
		}
		for field := range handshake {
			if field != contract.KindField && field != ws.AcceptField && field != contract.StatusField && field != handler.contract.ResponseCookies.Field {
				writer.WriteHeader(handler.contract.InvalidResponseStatus)
				return nil
			}
		}
		writeHTTPResponseCookies(writer.Header(), responseCookies, handler.contract.SetCookieHeader)
		writer.WriteHeader(status)
		return nil
	}
	var subprotocol string
	if rawProtocol, exists := handshake[ws.SubprotocolField]; exists && json.Unmarshal(rawProtocol, &subprotocol) != nil {
		writer.WriteHeader(handler.contract.InvalidResponseStatus)
		return nil
	}
	if _, exists := handshake[ws.SubprotocolField]; exists && subprotocol == "" {
		writer.WriteHeader(handler.contract.InvalidResponseStatus)
		return nil
	}
	for field := range handshake {
		if field != contract.KindField && field != ws.AcceptField && field != ws.SubprotocolField && field != handler.contract.ResponseCookies.Field {
			writer.WriteHeader(handler.contract.InvalidResponseStatus)
			return nil
		}
	}
	if subprotocol != "" {
		matched := false
		for _, offered := range websocket.Subprotocols(request) {
			if offered == subprotocol {
				matched = true
				break
			}
		}
		if !matched {
			writer.WriteHeader(handler.contract.InvalidResponseStatus)
			return nil
		}
	}
	responseHeaders := make(http.Header)
	writeHTTPResponseCookies(responseHeaders, responseCookies, handler.contract.SetCookieHeader)
	upgrader := websocket.Upgrader{Subprotocols: []string{subprotocol}, CheckOrigin: func(*http.Request) bool { return true }}
	connection, err := upgrader.Upgrade(writer, request, responseHeaders)
	if err != nil {
		return nil
	}
	defer connection.Close()
	connection.SetReadLimit(ws.MaxMessageBytes)
	activity := make(chan struct{}, 1)
	readResult := make(chan error, 1)
	go func() {
		readResult <- relayWebSocketRequest(connection, stream, contract, ws, activity)
	}()
	type streamReceive struct {
		message peer.Message
		err     error
	}
	receiveResult := make(chan streamReceive, 1)
	go func() {
		for {
			message, receiveErr := stream.Recv()
			select {
			case receiveResult <- streamReceive{message: message, err: receiveErr}:
			case <-ctx.Done():
				return
			}
			if receiveErr != nil {
				return
			}
		}
	}()
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(handler.idleTimeout)
	var writerMessageType int
	var writerMessage []byte
	for {
		select {
		case <-activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(handler.idleTimeout)
		case <-timer.C:
			cancel()
			return nil
		case <-maxTimer.C:
			cancel()
			return nil
		case <-request.Context().Done():
			cancel()
			return nil
		case readErr := <-readResult:
			if readErr != nil {
				cancel()
				return nil
			}
			readResult = nil
		case receivedMessage := <-receiveResult:
			message := receivedMessage.message
			if receivedMessage.err != nil || len(message.Payload) > contract.MaxFrameBytes || !contracts.ValidJSONNoDuplicateKeys(message.Payload) {
				cancel()
				return nil
			}
			frame := make(map[string]json.RawMessage)
			if json.Unmarshal(message.Payload, &frame) != nil {
				cancel()
				return nil
			}
			var frameKind string
			if json.Unmarshal(frame[contract.KindField], &frameKind) != nil {
				cancel()
				return nil
			}
			switch frameKind {
			case ws.MessageStartKind:
				var messageType string
				if writerMessage != nil || len(frame) != 2 || json.Unmarshal(frame[ws.MessageTypeField], &messageType) != nil {
					cancel()
					return nil
				}
				switch messageType {
				case ws.TextMessageType:
					writerMessageType = websocket.TextMessage
				case ws.BinaryMessageType:
					writerMessageType = websocket.BinaryMessage
				default:
					cancel()
					return nil
				}
				writerMessage = make([]byte, 0)
			case ws.MessageChunkKind:
				var data []byte
				if writerMessage == nil || len(frame) != 2 || json.Unmarshal(frame[contract.DataField], &data) != nil || len(data) > contract.MaxChunkBytes || int64(len(writerMessage)+len(data)) > ws.MaxMessageBytes {
					cancel()
					return nil
				}
				writerMessage = append(writerMessage, data...)
			case ws.MessageEndKind:
				if writerMessage == nil || len(frame) != 1 || (writerMessageType == websocket.TextMessage && !utf8.Valid(writerMessage)) || connection.WriteMessage(writerMessageType, writerMessage) != nil {
					cancel()
					return nil
				}
				writerMessage = nil
				select {
				case activity <- struct{}{}:
				default:
				}
			case ws.CloseKind:
				code := websocket.CloseNormalClosure
				var reason string
				if len(frame) > 3 || (len(frame) == 3 && json.Unmarshal(frame[ws.CodeField], &code) != nil) || (len(frame) >= 2 && json.Unmarshal(frame[ws.ReasonField], &reason) != nil) || !utf8.ValidString(reason) || len(reason) > 123 || !validWebSocketCloseCode(code) {
					cancel()
					return nil
				}
				for field := range frame {
					if field != contract.KindField && field != ws.CodeField && field != ws.ReasonField {
						cancel()
						return nil
					}
				}
				_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
				cancel()
				return nil
			default:
				cancel()
				return nil
			}
		}
	}
}

func validWebSocketCloseCode(code int) bool {
	if code >= 3000 && code <= 4999 {
		return true
	}
	if code < 1000 || code > 1014 {
		return false
	}
	return code != 1004 && code != 1005 && code != 1006
}

func relayWebSocketRequest(connection *websocket.Conn, stream peer.Stream, contract contracts.HTTPStream, ws contracts.HTTPStreamWebSocket, activity chan<- struct{}) error {
	for {
		messageType, data, err := connection.ReadMessage()
		if err != nil {
			if closeErr, ok := err.(*websocket.CloseError); ok {
				frame, _ := json.Marshal(map[string]any{contract.KindField: ws.CloseKind, ws.CodeField: closeErr.Code, ws.ReasonField: closeErr.Text})
				_ = sendPeerMessage(stream, peer.Message{Payload: frame})
			}
			return err
		}
		messageTypeName := ws.TextMessageType
		if messageType == websocket.BinaryMessage {
			messageTypeName = ws.BinaryMessageType
		} else if messageType != websocket.TextMessage {
			return peer.ErrInvalidRequest
		}
		start, _ := json.Marshal(map[string]any{contract.KindField: ws.MessageStartKind, ws.MessageTypeField: messageTypeName})
		if len(start) > contract.MaxFrameBytes || sendPeerMessage(stream, peer.Message{Payload: start}) != nil {
			return peer.ErrUnavailable
		}
		if int64(len(data)) > ws.MaxMessageBytes || (messageType == websocket.TextMessage && !utf8.Valid(data)) {
			_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseMessageTooBig, ""), time.Now().Add(time.Second))
			return peer.ErrInvalidRequest
		}
		for offset := 0; offset < len(data); {
			end := offset + contract.MaxChunkBytes
			if end > len(data) {
				end = len(data)
			}
			frame, marshalErr := json.Marshal(map[string]any{contract.KindField: ws.MessageChunkKind, contract.DataField: data[offset:end]})
			if marshalErr != nil || len(frame) > contract.MaxFrameBytes || sendPeerMessage(stream, peer.Message{Payload: frame}) != nil {
				return peer.ErrUnavailable
			}
			offset = end
			select {
			case activity <- struct{}{}:
			default:
			}
		}
		end, _ := json.Marshal(map[string]any{contract.KindField: ws.MessageEndKind})
		if sendPeerMessage(stream, peer.Message{Payload: end}) != nil {
			return peer.ErrUnavailable
		}
		select {
		case activity <- struct{}{}:
		default:
		}
	}
}

func (handler *httpDispatchHandler) serveStream(writer http.ResponseWriter, request *http.Request, headers map[string]string) error {
	contract := handler.contract.Stream
	defer request.Body.Close()
	select {
	case handler.streams <- struct{}{}:
		defer func() { <-handler.streams }()
	default:
		writer.WriteHeader(handler.contract.UnavailableStatus)
		return nil
	}
	select {
	case handler.binding.streams <- struct{}{}:
		defer func() { <-handler.binding.streams }()
	default:
		writer.WriteHeader(handler.contract.UnavailableStatus)
		return nil
	}
	ctx, cancel := context.WithCancelCause(request.Context())
	defer cancel(context.Canceled)
	stream, err := handler.binding.openStream(ctx, peer.Method(handler.Capability))
	if err != nil {
		writer.WriteHeader(handler.contract.UnavailableStatus)
		return nil
	}
	requestPayload := httpRequestPayload{Method: request.Method, Path: request.URL.EscapedPath(), Query: request.URL.RawQuery,
		Headers: headers, Cookies: handler.requestCookies(request), RequestID: request.Header.Get(handler.contract.RequestIDHeader), RemoteAddr: request.RemoteAddr}
	start, err := json.Marshal(map[string]any{contract.KindField: contract.RequestStartKind, contract.RequestField: requestPayload})
	if err != nil || len(start) > contract.MaxFrameBytes || sendPeerMessage(stream, peer.Message{Payload: start}) != nil {
		writer.WriteHeader(handler.contract.InvalidRequestStatus)
		return nil
	}
	sendResult := make(chan error, 1)
	activity := make(chan struct{}, 1)
	go func() {
		sendResult <- sendHTTPStreamRequest(stream, request.Body, contract, handler.contract.MaxRequestBytes, func() {
			select {
			case activity <- struct{}{}:
			default:
			}
		})
	}()
	type received struct {
		message peer.Message
		err     error
	}
	receiveResult := make(chan received, 1)
	go func() {
		for {
			message, receiveErr := stream.Recv()
			select {
			case receiveResult <- received{message: message, err: receiveErr}:
			case <-ctx.Done():
				return
			}
			if receiveErr != nil {
				return
			}
		}
	}()
	responseStarted := false
	idleTimer := time.NewTimer(handler.idleTimeout)
	defer idleTimer.Stop()
	maxTimer := time.NewTimer(handler.maxDuration)
	defer maxTimer.Stop()
	for {
		select {
		case <-activity:
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(handler.idleTimeout)
		case <-idleTimer.C:
			cancel(errHTTPStreamIdleTimeout)
			_ = request.Body.Close()
			if !responseStarted {
				writer.WriteHeader(handler.contract.StreamTimeoutStatus)
			}
			return nil
		case <-maxTimer.C:
			cancel(errHTTPStreamMaxDuration)
			_ = request.Body.Close()
			if !responseStarted {
				writer.WriteHeader(handler.contract.StreamTimeoutStatus)
			}
			return nil
		case sendErr := <-sendResult:
			sendResult = nil
			if errors.Is(sendErr, errHTTPStreamRequestTooLarge) {
				cancel(context.Canceled)
				_ = request.Body.Close()
				if !responseStarted {
					writer.WriteHeader(handler.contract.RequestTooLargeStatus)
				}
				return nil
			}
			if sendErr != nil {
				cancel(context.Canceled)
				_ = request.Body.Close()
				if !responseStarted {
					writer.WriteHeader(handler.contract.UnavailableStatus)
				}
				return nil
			}
		case receivedMessage := <-receiveResult:
			if receivedMessage.err == nil {
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(handler.idleTimeout)
			}
			if receivedMessage.err != nil {
				if !responseStarted {
					writer.WriteHeader(handler.contract.UnavailableStatus)
				}
				return nil
			}
			if len(receivedMessage.message.Payload) > contract.MaxFrameBytes {
				if !responseStarted {
					writer.WriteHeader(handler.contract.InvalidResponseStatus)
				}
				return nil
			}
			var frame map[string]json.RawMessage
			if !contracts.ValidJSONNoDuplicateKeys(receivedMessage.message.Payload) || json.Unmarshal(receivedMessage.message.Payload, &frame) != nil {
				if !responseStarted {
					writer.WriteHeader(handler.contract.InvalidResponseStatus)
				}
				return nil
			}
			var kind string
			if json.Unmarshal(frame[contract.KindField], &kind) != nil {
				if !responseStarted {
					writer.WriteHeader(handler.contract.InvalidResponseStatus)
				}
				return nil
			}
			if !responseStarted {
				if kind != contract.ResponseStartKind {
					writer.WriteHeader(handler.contract.InvalidResponseStatus)
					return nil
				}
				var status int
				var responseHeaders map[string]string
				var responseCookies []httpCookieAction
				cookieRaw, hasCookies := frame[handler.contract.ResponseCookies.Field]
				if len(frame) != 3 && !(len(frame) == 4 && hasCookies) || len(frame[contract.HeadersField]) == 0 || string(frame[contract.HeadersField]) == "null" || json.Unmarshal(frame[contract.StatusField], &status) != nil || json.Unmarshal(frame[contract.HeadersField], &responseHeaders) != nil || status < 200 || status > 599 || !validStreamHeaders(responseHeaders) {
					writer.WriteHeader(handler.contract.InvalidResponseStatus)
					return nil
				}
				if hasCookies && (string(cookieRaw) == "null" || json.Unmarshal(cookieRaw, &responseCookies) != nil || !validHTTPResponseCookies(responseCookies, handler.contract)) {
					writer.WriteHeader(handler.contract.InvalidResponseStatus)
					return nil
				}
				if handler.Mode == "sse" {
					if status != http.StatusOK || !hasSSEContentType(responseHeaders, contract.SSE.ContentTypeHeader, contract.SSE.ContentType) {
						writer.WriteHeader(handler.contract.InvalidResponseStatus)
						return nil
					}
				}
				for name, value := range responseHeaders {
					writer.Header().Set(name, value)
				}
				writeHTTPResponseCookies(writer.Header(), responseCookies, handler.contract.SetCookieHeader)
				if handler.Mode == "sse" {
					writer.Header().Set(contract.SSE.ContentTypeHeader, contract.SSE.ContentType)
				}
				writer.WriteHeader(status)
				responseStarted = true
				continue
			}
			switch kind {
			case contract.ResponseChunkKind:
				if handler.Mode == "sse" {
					return nil
				}
				var chunk []byte
				if len(frame) != 2 || json.Unmarshal(frame[contract.DataField], &chunk) != nil || len(chunk) > contract.MaxChunkBytes {
					return nil
				}
				if _, writeErr := writer.Write(chunk); writeErr != nil {
					cancel(context.Canceled)
					return nil
				}
				if flusher, ok := writer.(http.Flusher); ok {
					flusher.Flush()
				}
			case contract.SSE.EventKind:
				if handler.Mode != "sse" || writeSSEEvent(writer, frame, contract.KindField, contract.SSE) != nil {
					return nil
				}
				if flusher, ok := writer.(http.Flusher); ok {
					flusher.Flush()
				}
			case contract.ResponseEndKind:
				if len(frame) != 1 {
					return nil
				}
				cancel(context.Canceled)
				_ = request.Body.Close()
				return nil
			default:
				return nil
			}
		}
	}
}

func hasSSEContentType(headers map[string]string, headerName, expected string) bool {
	for name, value := range headers {
		if !strings.EqualFold(name, headerName) {
			continue
		}
		mediaType, _, err := mime.ParseMediaType(value)
		return err == nil && strings.EqualFold(mediaType, expected)
	}
	return false
}

func writeSSEEvent(writer io.Writer, frame map[string]json.RawMessage, kindField string, contract contracts.HTTPStreamSSE) error {
	if len(frame) < 2 || len(frame) > 5 {
		return peer.ErrInvalidRequest
	}
	allowed := map[string]struct{}{contract.EventField: {}, contract.DataField: {}, contract.IDField: {}, contract.RetryField: {}}
	for field := range frame {
		if field == kindField {
			continue
		}
		if _, ok := allowed[field]; !ok {
			return peer.ErrInvalidRequest
		}
	}
	dataValue, exists := frame[contract.DataField]
	if !exists {
		return peer.ErrInvalidRequest
	}
	var data string
	if json.Unmarshal(dataValue, &data) != nil || !utf8.ValidString(data) {
		return peer.ErrInvalidRequest
	}
	var output strings.Builder
	if value, ok := frame[contract.EventField]; ok {
		var event string
		if json.Unmarshal(value, &event) != nil || strings.ContainsAny(event, "\r\n") || !utf8.ValidString(event) {
			return peer.ErrInvalidRequest
		}
		output.WriteString(contract.EventPrefix)
		output.WriteString(event)
		output.WriteString(contract.LineEnding)
	}
	data = strings.ReplaceAll(strings.ReplaceAll(data, "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.Split(data, "\n") {
		output.WriteString(contract.DataPrefix)
		output.WriteString(line)
		output.WriteString(contract.LineEnding)
	}
	if value, ok := frame[contract.IDField]; ok {
		var id string
		if json.Unmarshal(value, &id) != nil || strings.ContainsAny(id, "\r\n\x00") || !utf8.ValidString(id) {
			return peer.ErrInvalidRequest
		}
		output.WriteString(contract.IDPrefix)
		output.WriteString(id)
		output.WriteString(contract.LineEnding)
	}
	if value, ok := frame[contract.RetryField]; ok {
		var retry int64
		if json.Unmarshal(value, &retry) != nil || retry < 0 || retry > contract.MaxRetryMillis {
			return peer.ErrInvalidRequest
		}
		output.WriteString(contract.RetryPrefix)
		output.WriteString(strconv.FormatInt(retry, 10))
		output.WriteString(contract.LineEnding)
	}
	output.WriteString(contract.EventTerminator)
	_, err := io.WriteString(writer, output.String())
	return err
}

func sendHTTPStreamRequest(stream peer.Stream, body io.Reader, contract contracts.HTTPStream, maxBytes int64, activity func()) error {
	buffer := make([]byte, contract.MaxChunkBytes)
	var total int64
	for {
		readSize := len(buffer)
		if remaining := maxBytes - total; remaining < int64(readSize) {
			readSize = int(remaining) + 1
		}
		if readSize < 1 {
			readSize = 1
		}
		count, readErr := body.Read(buffer[:readSize])
		if count > 0 {
			if total+int64(count) > maxBytes {
				return errHTTPStreamRequestTooLarge
			}
			total += int64(count)
			frame, err := json.Marshal(map[string]any{contract.KindField: contract.RequestChunkKind, contract.DataField: buffer[:count]})
			if err != nil || len(frame) > contract.MaxFrameBytes {
				return peer.ErrInvalidRequest
			}
			if err := sendPeerMessage(stream, peer.Message{Payload: frame}); err != nil {
				return err
			}
			activity()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	frame, err := json.Marshal(map[string]any{contract.KindField: contract.RequestEndKind})
	if err != nil || sendPeerMessage(stream, peer.Message{Payload: frame}) != nil {
		return peer.ErrUnavailable
	}
	activity()
	return nil
}

// sendPeerMessage turns the peer protocol's bounded queue-full signal into
// backpressure at the HTTP boundary. It retries only while this request stream
// remains live, so a slow client cannot cause unbounded buffering or detached work.
func sendPeerMessage(stream peer.Stream, message peer.Message) error {
	for {
		err := stream.Send(message)
		if !errors.Is(err, peer.ErrSendQueueFull) {
			return err
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-stream.Context().Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return stream.Context().Err()
		case <-timer.C:
		}
	}
}

func validStreamHeaders(headers map[string]string) bool {
	for name, value := range headers {
		canonical := http.CanonicalHeaderKey(name)
		if canonical == "" || canonical == http.CanonicalHeaderKey("Set-Cookie") || canonical == http.CanonicalHeaderKey("Content-Length") || strings.ContainsAny(name+value, "\r\n") || textproto.CanonicalMIMEHeaderKey(name) == "" {
			return false
		}
	}
	return true
}

// call uses the current authenticated session once. If the peer has restarted,
// the current invocation is not replayed because its outcome may be unknown; the
// failed session is discarded so the next independent request can reconnect.
func (binding *dispatchBinding) call(ctx context.Context, method peer.Method, payload []byte) (peer.Result, error) {
	if err := ctx.Err(); err != nil {
		return peer.Result{}, err
	}
	client, err := binding.current(ctx)
	if err != nil {
		return peer.Result{}, err
	}
	result, err := client.Call(ctx, method, payload)
	if err != nil {
		binding.invalidate(client)
	}
	return result, err
}

func (binding *dispatchBinding) openStream(ctx context.Context, method peer.Method) (peer.Stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client, err := binding.current(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := client.OpenStream(ctx, method)
	if err != nil {
		binding.invalidate(client)
		return nil, err
	}
	return stream, nil
}

func (binding *dispatchBinding) current(ctx context.Context) (peer.Client, error) {
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if binding.client != nil {
		return binding.client, nil
	}
	client, err := peer.Dial(ctx, peer.ClientConfig{
		Network:  peer.NetworkConfig{Carrier: peer.CarrierTCP, Endpoint: binding.endpoint},
		Security: binding.security, Handler: binding.handler,
	})
	if err != nil {
		return nil, err
	}
	binding.client = client
	return client, nil
}

func (binding *dispatchBinding) invalidate(client peer.Client) {
	binding.mu.Lock()
	if binding.client == client {
		binding.client = nil
		binding.mu.Unlock()
		_ = client.Close()
		return
	}
	binding.mu.Unlock()
}

func (binding *dispatchBinding) close() {
	binding.mu.Lock()
	client := binding.client
	binding.client = nil
	binding.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
}

func decodeHTTPResponseAction(payload []byte, contract contracts.HTTPDispatch) (httpResponseAction, error) {
	var action httpResponseAction
	if !contracts.ValidJSONNoDuplicateKeys(payload) {
		return httpResponseAction{}, peer.ErrInvalidRequest
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&action); err != nil || action.Status < 200 || action.Status > 599 {
		return httpResponseAction{}, peer.ErrInvalidRequest
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return httpResponseAction{}, peer.ErrInvalidRequest
	}
	for name, value := range action.Headers {
		canonical := http.CanonicalHeaderKey(name)
		if canonical == "" || canonical == http.CanonicalHeaderKey("Set-Cookie") || strings.ContainsAny(name+value, "\r\n") {
			return httpResponseAction{}, peer.ErrInvalidRequest
		}
		if textproto.CanonicalMIMEHeaderKey(name) == "" {
			return httpResponseAction{}, peer.ErrInvalidRequest
		}
	}
	if !validHTTPResponseCookies(action.Cookies, contract) {
		return httpResponseAction{}, peer.ErrInvalidRequest
	}
	return action, nil
}

func validHTTPResponseCookies(cookies []httpCookieAction, contract contracts.HTTPDispatch) bool {
	for _, actionCookie := range cookies {
		cookie := actionCookie.httpCookie()
		if actionCookie.Name == "" || actionCookie.MaxAge < contract.ResponseCookies.MaxAgeMinimum || (actionCookie.Path != "" && !strings.HasPrefix(actionCookie.Path, "/")) || cookie.Valid() != nil {
			return false
		}
		if actionCookie.SameSite != "" && !containsCookieContractValue(contract.ResponseCookies.SameSiteValues, actionCookie.SameSite) {
			return false
		}
		if contract.ResponseCookies.SameSiteNoneRequiresSecure && actionCookie.SameSite == "none" && !actionCookie.Secure {
			return false
		}
	}
	return true
}

func containsCookieContractValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func writeHTTPResponseCookies(headers http.Header, cookies []httpCookieAction, setCookieHeader string) {
	for _, cookie := range cookies {
		headers.Add(setCookieHeader, cookie.httpCookie().String())
	}
}

func (actionCookie httpCookieAction) httpCookie() *http.Cookie {
	cookie := &http.Cookie{
		Name: actionCookie.Name, Value: actionCookie.Value, Path: actionCookie.Path,
		Domain: actionCookie.Domain, MaxAge: actionCookie.MaxAge, Secure: actionCookie.Secure,
		HttpOnly: actionCookie.HTTPOnly,
	}
	if actionCookie.Expires != nil {
		cookie.Expires = *actionCookie.Expires
	}
	switch actionCookie.SameSite {
	case "lax":
		cookie.SameSite = http.SameSiteLaxMode
	case "strict":
		cookie.SameSite = http.SameSiteStrictMode
	case "none":
		cookie.SameSite = http.SameSiteNoneMode
	case "":
		cookie.SameSite = http.SameSiteDefaultMode
	default:
		cookie.SameSite = http.SameSite(255)
	}
	return cookie
}

func endpointIsLoopback(endpoint string) bool {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

var _ caddyhttp.MiddlewareHandler = (*httpDispatchHandler)(nil)
var _ caddycore.App = (*dispatchApp)(nil)

package caddy

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/Liapoldus/pluginprotocol/presentation/peer"
	caddycore "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
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
	bindings    map[string]dispatchBinding
	contract    contracts.HTTPDispatch
}

type dispatchBinding struct {
	client  peer.Client
	timeout time.Duration
	calls   map[string]struct{}
}

type httpDispatchHandler struct {
	Instance   string `json:"instance,omitempty"`
	Capability string `json:"capability,omitempty"`
	binding    dispatchBinding
	contract   contracts.HTTPDispatch
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
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    *string           `json:"body,omitempty"`
}

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
	app.bindings = make(map[string]dispatchBinding, len(app.Instances))
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
		app.bindings[instance.ID] = dispatchBinding{client: client, timeout: timeout, calls: methods}
	}
	return nil
}

func (app *dispatchApp) Start() error   { return nil }
func (app *dispatchApp) Stop() error    { app.close(); return nil }
func (app *dispatchApp) Cleanup() error { app.close(); return nil }

func (app *dispatchApp) close() {
	for _, binding := range app.bindings {
		_ = binding.client.Close()
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
	handler.binding = binding
	return nil
}

func (handler *httpDispatchHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request, _ caddyhttp.Handler) error {
	if handler.Instance == "" || handler.Capability == "" {
		writer.WriteHeader(handler.contract.InvalidRequestStatus)
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, handler.contract.MaxRequestBytes+1))
	if err != nil || int64(len(body)) > handler.contract.MaxRequestBytes {
		writer.WriteHeader(handler.contract.RequestTooLargeStatus)
		return nil
	}
	blocked := make(map[string]struct{}, len(handler.contract.BlockedHeaders)+1)
	for _, name := range handler.contract.BlockedHeaders {
		blocked[http.CanonicalHeaderKey(name)] = struct{}{}
	}
	// Cookies are deliberately omitted until the per-instance/capability allow-list
	// is supplied by the Core dispatch snapshot. Forwarding an unfiltered Cookie
	// header would cross the product security boundary.
	blocked[http.CanonicalHeaderKey(handler.contract.CookieHeader)] = struct{}{}
	headers := make(map[string]string, len(request.Header))
	for name, values := range request.Header {
		if _, skip := blocked[http.CanonicalHeaderKey(name)]; skip || len(values) == 0 {
			continue
		}
		headers[name] = strings.Join(values, ",")
	}
	payload, err := json.Marshal(httpRequestPayload{
		Method: request.Method, Path: request.URL.EscapedPath(), Query: request.URL.RawQuery,
		Headers: headers, Body: body, RequestID: request.Header.Get(handler.contract.RequestIDHeader), RemoteAddr: request.RemoteAddr,
	})
	if err != nil {
		writer.WriteHeader(handler.contract.InvalidRequestStatus)
		return nil
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.binding.timeout)
	defer cancel()
	response, err := handler.binding.client.Call(ctx, peer.Method(handler.Capability), payload)
	if err != nil {
		writer.WriteHeader(handler.contract.UnavailableStatus)
		return nil
	}
	action, err := decodeHTTPResponseAction(response.Payload)
	if err != nil {
		writer.WriteHeader(handler.contract.InvalidResponseStatus)
		return nil
	}
	for name, value := range action.Headers {
		writer.Header().Set(name, value)
	}
	writer.WriteHeader(action.Status)
	if action.Body != nil {
		_, _ = io.WriteString(writer, *action.Body)
	}
	return nil
}

func decodeHTTPResponseAction(payload []byte) (httpResponseAction, error) {
	var action httpResponseAction
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&action); err != nil || action.Status < 200 || action.Status > 599 {
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
	return action, nil
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

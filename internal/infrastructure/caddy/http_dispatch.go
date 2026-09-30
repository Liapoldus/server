package caddy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"liapoldus.local/server-plugin/contracts"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	pluginsdk "github.com/Liapoldus/pluginprotocol/presentation/sdk"
	caddycore "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

type dispatchInstance struct {
	ID             string                   `json:"id"`
	Endpoint       string                   `json:"endpoint"`
	TimeoutMillis  int                      `json:"timeoutMillis,omitempty"`
	CookiePolicies []pluginsdk.CookiePolicy `json:"cookiePolicies,omitempty"`
}

type dispatchApp struct {
	Instances []dispatchInstance `json:"instances,omitempty"`
	bindings  map[string]dispatchBinding
	contract  contracts.HTTPDispatch
}

type dispatchBinding struct {
	client  *pluginsdk.Client
	timeout time.Duration
	cookies map[string]pluginsdk.CookiePolicy
	calls   map[string]struct{}
}

type httpDispatchHandler struct {
	Instance   string `json:"instance,omitempty"`
	Capability string `json:"capability,omitempty"`
	binding    dispatchBinding
	contract   contracts.HTTPDispatch
}

type httpRequestPayload struct {
	Method     string                 `json:"method"`
	Path       string                 `json:"path"`
	Query      string                 `json:"query,omitempty"`
	Headers    map[string]string      `json:"headers,omitempty"`
	Cookies    []pluginsdk.CookiePair `json:"cookies,omitempty"`
	Body       []byte                 `json:"body,omitempty"`
	RequestID  string                 `json:"requestId"`
	RemoteAddr string                 `json:"remoteAddr,omitempty"`
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
		if instance.ID == "" || instance.Endpoint == "" {
			app.close()
			return contracts.ErrInvalidDispatchAssets
		}
		if _, duplicate := app.bindings[instance.ID]; duplicate {
			app.close()
			return contracts.ErrInvalidDispatchAssets
		}
		timeout := time.Duration(instance.TimeoutMillis) * time.Millisecond
		if timeout <= 0 {
			timeout = time.Duration(contract.DefaultTimeoutMillis) * time.Millisecond
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		client, err := pluginsdk.DialContext(ctx, instance.Endpoint)
		cancel()
		if err != nil {
			app.close()
			return pluginsdk.ErrUnavailable
		}
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		manifest, err := client.Service().Manifest(ctx, &pluginv1.ManifestRequest{})
		if err == nil {
			err = client.CheckHealth(ctx)
		}
		cancel()
		if err != nil || manifest == nil || manifest.GetName() != instance.ID {
			_ = client.Close()
			app.close()
			return pluginsdk.ErrUnavailable
		}
		calls := manifestCallCapabilities(manifest)
		cookiePolicies := make(map[string]pluginsdk.CookiePolicy, len(instance.CookiePolicies))
		for _, policy := range instance.CookiePolicies {
			if policy.InstanceID != instance.ID || policy.Capability == "" {
				_ = client.Close()
				app.close()
				return pluginsdk.ErrInvalidCookiePolicy
			}
			if _, duplicate := cookiePolicies[policy.Capability]; duplicate {
				_ = client.Close()
				app.close()
				return pluginsdk.ErrInvalidCookiePolicy
			}
			cookiePolicies[policy.Capability] = policy
		}
		for _, policy := range instance.CookiePolicies {
			if _, supported := calls[policy.Capability]; !supported {
				_ = client.Close()
				app.close()
				return pluginsdk.ErrInvalidCookiePolicy
			}
		}
		app.bindings[instance.ID] = dispatchBinding{client: client, timeout: timeout, cookies: cookiePolicies, calls: calls}
	}
	return nil
}

func manifestCallCapabilities(manifest *pluginv1.Manifest) map[string]struct{} {
	capabilities := make(map[string]struct{})
	for _, capability := range manifest.GetCapabilities() {
		capabilities[capability] = struct{}{}
	}
	for _, descriptor := range manifest.GetCapabilityDescriptors() {
		for _, mode := range descriptor.GetModes() {
			if mode == pluginv1.InvocationMode_INVOCATION_MODE_CALL {
				capabilities[descriptor.GetCapability()] = struct{}{}
			}
		}
	}
	return capabilities
}

func (app *dispatchApp) Start() error { return nil }

func (app *dispatchApp) Stop() error {
	app.close()
	return nil
}

func (app *dispatchApp) Cleanup() error {
	app.close()
	return nil
}

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
		return pluginsdk.ErrUnavailable
	}
	binding, ok := app.bindings[handler.Instance]
	if !ok {
		return pluginsdk.ErrUnavailable
	}
	if _, supported := binding.calls[handler.Capability]; !supported {
		return pluginsdk.ErrUnavailable
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
	blocked := make(map[string]struct{}, len(handler.contract.BlockedHeaders))
	for _, name := range handler.contract.BlockedHeaders {
		blocked[http.CanonicalHeaderKey(name)] = struct{}{}
	}
	headers := make(map[string]string, len(request.Header))
	for name, values := range request.Header {
		if _, skip := blocked[http.CanonicalHeaderKey(name)]; skip || len(values) == 0 {
			continue
		}
		headers[name] = strings.Join(values, ",")
	}
	var cookies []pluginsdk.CookiePair
	if policy, allowed := handler.binding.cookies[handler.Capability]; allowed {
		pairs, parseErr := pluginsdk.ParseCookieHeader(request.Header.Values(handler.contract.CookieHeader))
		if parseErr != nil {
			writer.WriteHeader(handler.contract.InvalidRequestStatus)
			return nil
		}
		cookies, err = pluginsdk.FilterCookiePairs(policy, handler.Instance, handler.Capability, pairs)
		if err != nil {
			writer.WriteHeader(handler.contract.InvalidRequestStatus)
			return nil
		}
	}
	payload, err := json.Marshal(httpRequestPayload{
		Method: request.Method, Path: request.URL.EscapedPath(), Query: request.URL.RawQuery,
		Headers: headers, Cookies: cookies, Body: body, RequestID: request.Header.Get(handler.contract.RequestIDHeader),
		RemoteAddr: request.RemoteAddr,
	})
	if err != nil {
		writer.WriteHeader(handler.contract.InvalidRequestStatus)
		return nil
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.binding.timeout)
	defer cancel()
	response, err := handler.binding.client.Call(ctx, handler.Capability, payload)
	if err != nil || response == nil {
		writer.WriteHeader(handler.contract.UnavailableStatus)
		return nil
	}
	action, setCookie, err := pluginsdk.DecodeHTTPResponseAction(response.GetPayload(), request.Host)
	if err != nil {
		writer.WriteHeader(handler.contract.InvalidResponseStatus)
		return nil
	}
	for name, value := range action.Headers {
		writer.Header().Set(name, value)
	}
	for _, cookie := range setCookie {
		writer.Header().Add(handler.contract.SetCookieHeader, cookie)
	}
	writer.WriteHeader(action.Status)
	if action.Body != nil {
		_, _ = io.WriteString(writer, *action.Body)
	}
	return nil
}

var _ caddyhttp.MiddlewareHandler = (*httpDispatchHandler)(nil)
var _ caddycore.App = (*dispatchApp)(nil)

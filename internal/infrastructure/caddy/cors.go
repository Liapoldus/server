package caddy

import (
	"net/http"
	"strconv"
	"strings"

	caddycore "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

const corsHandlerModule = "liapoldus_cors"

type corsHandler struct {
	Origins          []string `json:"origins"`
	Methods          []string `json:"methods"`
	AllowedHeaders   []string `json:"allowedHeaders,omitempty"`
	ExposedHeaders   []string `json:"exposedHeaders,omitempty"`
	AllowCredentials bool     `json:"allowCredentials,omitempty"`
	MaxAgeSeconds    int      `json:"maxAgeSeconds"`
}

func init() {
	caddycore.RegisterModule(corsHandler{})
}

func (corsHandler) CaddyModule() caddycore.ModuleInfo {
	return caddycore.ModuleInfo{
		ID:  caddycore.ModuleID("http.handlers." + corsHandlerModule),
		New: func() caddycore.Module { return new(corsHandler) },
	}
}

func (handler *corsHandler) Provision(caddycore.Context) error {
	credentials := handler.AllowCredentials
	feature := corsFeature{
		Mode: "enabled", Origins: handler.Origins, Methods: handler.Methods,
		AllowedHeaders: handler.AllowedHeaders, ExposedHeaders: handler.ExposedHeaders,
		AllowCredentials: &credentials, MaxAgeSeconds: featureIntPointer(handler.MaxAgeSeconds),
	}
	if !validCORSFeature(feature, false) {
		return errUnsupportedSettings
	}
	return nil
}

func (handler *corsHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request, next caddyhttp.Handler) error {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return next.ServeHTTP(writer, request)
	}
	addVary(writer.Header(), "Origin")
	allowedOrigin, ok := handler.allowedOrigin(origin)
	if !ok {
		writer.WriteHeader(http.StatusForbidden)
		return nil
	}
	requestedMethod := request.Header.Get("Access-Control-Request-Method")
	preflight := request.Method == http.MethodOptions && requestedMethod != ""
	if preflight {
		if !containsExact(handler.Methods, requestedMethod) {
			writer.WriteHeader(http.StatusForbidden)
			return nil
		}
		if !requestedHeadersAllowed(request.Header.Get("Access-Control-Request-Headers"), handler.AllowedHeaders) {
			writer.WriteHeader(http.StatusForbidden)
			return nil
		}
	} else if !containsExact(handler.Methods, request.Method) {
		writer.WriteHeader(http.StatusForbidden)
		return nil
	}
	writer.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
	if handler.AllowCredentials {
		writer.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	if len(handler.ExposedHeaders) > 0 {
		writer.Header().Set("Access-Control-Expose-Headers", strings.Join(handler.ExposedHeaders, ", "))
	}
	if preflight {
		addVary(writer.Header(), "Access-Control-Request-Method", "Access-Control-Request-Headers")
		writer.Header().Set("Access-Control-Allow-Methods", strings.Join(handler.Methods, ", "))
		if len(handler.AllowedHeaders) > 0 {
			writer.Header().Set("Access-Control-Allow-Headers", strings.Join(handler.AllowedHeaders, ", "))
		}
		if handler.MaxAgeSeconds > 0 {
			writer.Header().Set("Access-Control-Max-Age", strconv.Itoa(handler.MaxAgeSeconds))
		}
		writer.WriteHeader(http.StatusNoContent)
		return nil
	}
	return next.ServeHTTP(writer, request)
}

func (handler *corsHandler) allowedOrigin(origin string) (string, bool) {
	for _, configured := range handler.Origins {
		if configured == "*" {
			return "*", true
		}
		if configured == origin {
			return origin, true
		}
	}
	return "", false
}

func requestedHeadersAllowed(header string, allowed []string) bool {
	if strings.TrimSpace(header) == "" {
		return true
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[strings.ToLower(name)] = struct{}{}
	}
	for _, requested := range strings.Split(header, ",") {
		name := strings.TrimSpace(requested)
		if name == "" {
			return false
		}
		if _, exists := allowedSet[strings.ToLower(name)]; !exists {
			return false
		}
	}
	return true
}

func containsExact(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func addVary(header http.Header, names ...string) {
	seen := make(map[string]struct{})
	for _, value := range header.Values("Vary") {
		for _, name := range strings.Split(value, ",") {
			seen[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
		}
	}
	for _, name := range names {
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		header.Add("Vary", name)
		seen[key] = struct{}{}
	}
}

var _ caddycore.Module = (*corsHandler)(nil)
var _ caddyhttp.MiddlewareHandler = (*corsHandler)(nil)

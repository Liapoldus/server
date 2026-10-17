package caddy

import (
	"net/http"
	"net/url"
	"strings"

	caddycore "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

const redirectHandlerModule = "liapoldus_redirect"

type redirectHandler struct {
	Target     string `json:"target"`
	StatusCode int    `json:"statusCode"`
}

func init() { caddycore.RegisterModule(redirectHandler{}) }

func (redirectHandler) CaddyModule() caddycore.ModuleInfo {
	return caddycore.ModuleInfo{ID: caddycore.ModuleID("http.handlers." + redirectHandlerModule), New: func() caddycore.Module { return new(redirectHandler) }}
}

func (handler *redirectHandler) Provision(caddycore.Context) error {
	if handler == nil || !validRedirectTarget(handler.Target) ||
		(handler.StatusCode != http.StatusMovedPermanently && handler.StatusCode != http.StatusFound && handler.StatusCode != http.StatusTemporaryRedirect && handler.StatusCode != http.StatusPermanentRedirect) {
		return errUnsupportedSettings
	}
	return nil
}

func validRedirectTarget(value string) bool {
	if value == "" || len(value) > 2048 || strings.ContainsAny(value, "\\\r\n\x00?#") {
		return false
	}
	if strings.HasPrefix(value, "/") {
		return validConfiguredPath(value)
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.Path == "" || validConfiguredPath(parsed.Path)
}

func (handler *redirectHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request, _ caddyhttp.Handler) error {
	if _, err := canonicalRequestPath(request); err != nil {
		return caddyhttp.Error(http.StatusBadRequest, errInvalidRequestPath)
	}
	target := handler.Target
	if request.URL.RawQuery != "" {
		target += "?" + request.URL.RawQuery
	}
	writer.Header().Set("Location", target)
	writer.WriteHeader(handler.StatusCode)
	return nil
}

var _ caddycore.Module = (*redirectHandler)(nil)
var _ caddyhttp.MiddlewareHandler = (*redirectHandler)(nil)

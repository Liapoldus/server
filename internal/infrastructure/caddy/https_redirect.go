package caddy

import (
	"net/http"
	"strings"

	caddycore "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

const httpsRedirectModule = "liapoldus_https_redirect"

type httpsRedirectHandler struct {
	TargetHostnames []string `json:"targetHostnames,omitempty"`
	hosts           []compiledHost
}

func init() {
	caddycore.RegisterModule(httpsRedirectHandler{})
}

func (httpsRedirectHandler) CaddyModule() caddycore.ModuleInfo {
	return caddycore.ModuleInfo{
		ID:  caddycore.ModuleID("http.handlers." + httpsRedirectModule),
		New: func() caddycore.Module { return new(httpsRedirectHandler) },
	}
}

func (handler *httpsRedirectHandler) Provision(caddycore.Context) error {
	if len(handler.TargetHostnames) == 0 {
		return errUnsupportedSettings
	}
	handler.hosts = make([]compiledHost, 0, len(handler.TargetHostnames))
	for _, configured := range handler.TargetHostnames {
		canonical, err := canonicalConfiguredHost(configured)
		if err != nil {
			return errUnsupportedSettings
		}
		host, err := parseCanonicalHost(canonical)
		if err != nil {
			return errUnsupportedSettings
		}
		handler.hosts = append(handler.hosts, host)
	}
	return nil
}

func (handler *httpsRedirectHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request, _ caddyhttp.Handler) error {
	host := canonicalRequestHost(request.Host)
	for _, target := range handler.hosts {
		if target.matches(host) {
			canonicalPath, err := canonicalRequestPath(request)
			if err != nil {
				return caddyhttp.Error(http.StatusBadRequest, errInvalidRequestPath)
			}
			_, query, hasQuery := strings.Cut(request.RequestURI, "?")
			location := "https://" + host + canonicalPath
			if hasQuery {
				location += "?" + query
			}
			writer.Header().Set("Location", location)
			writer.WriteHeader(http.StatusPermanentRedirect)
			return nil
		}
	}
	writer.WriteHeader(http.StatusMisdirectedRequest)
	return nil
}

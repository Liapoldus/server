package caddy

import (
	"errors"
	"net/http"
	"strings"

	caddycore "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"liapoldus.local/server-plugin/contracts"
)

type requestHeaderLimit struct {
	MaxBytes int `json:"maxBytes"`
	Status   int `json:"status"`
}

func init() {
	caddycore.RegisterModule(requestHeaderLimit{})
}

func (requestHeaderLimit) CaddyModule() caddycore.ModuleInfo {
	contract, err := contracts.LoadHTTPDispatch()
	if err != nil {
		panic("invalid embedded HTTP dispatch contract")
	}
	return caddycore.ModuleInfo{
		ID: caddycore.ModuleID(contract.HeaderLimitModule),
		New: func() caddycore.Module {
			return new(requestHeaderLimit)
		},
	}
}

func (handler *requestHeaderLimit) ServeHTTP(writer http.ResponseWriter, request *http.Request, next caddyhttp.Handler) error {
	if handler == nil || handler.MaxBytes < 1 || handler.Status < 400 || handler.Status > 599 {
		return caddyhttp.Error(http.StatusInternalServerError, errors.New("invalid request header limit"))
	}
	if requestHeaderBytes(request) > handler.MaxBytes {
		writer.WriteHeader(handler.Status)
		return nil
	}
	return next.ServeHTTP(writer, request)
}

func requestHeaderBytes(request *http.Request) int {
	bytes := 0
	if request.Host != "" {
		bytes += len("Host") + 2 + len(request.Host) + 2
	}
	for name, values := range request.Header {
		if strings.EqualFold(name, "host") {
			continue
		}
		for _, value := range values {
			bytes += len(name) + 2 + len(value) + 2
		}
	}
	return bytes
}

func headerLimitHandler(contract contracts.HTTPDispatch) map[string]any {
	return map[string]any{
		"handler":  strings.TrimPrefix(contract.HeaderLimitModule, "http.handlers."),
		"maxBytes": contract.MaxRequestHeaderBytes,
		"status":   contract.RequestHeaderTooLargeStatus,
	}
}

var _ caddyhttp.MiddlewareHandler = (*requestHeaderLimit)(nil)

package caddy

import (
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	caddycore "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

const (
	rateLimitHandlerModule = "liapoldus_rate_limit"
	rateLimitMaxClients    = 10000
	rateLimitClientTTL     = 5 * time.Minute
)

type rateLimitHandler struct {
	RequestsPerSecond int      `json:"requestsPerSecond"`
	Burst             int      `json:"burst"`
	TrustedProxies    []string `json:"trustedProxies,omitempty"`
	mu                sync.Mutex
	clients           map[string]rateLimitBucket
	trusted           []netip.Prefix
}

type rateLimitBucket struct {
	tokens    float64
	updatedAt time.Time
}

func init() { caddycore.RegisterModule((*rateLimitHandler)(nil)) }

func (*rateLimitHandler) CaddyModule() caddycore.ModuleInfo {
	return caddycore.ModuleInfo{ID: caddycore.ModuleID("http.handlers." + rateLimitHandlerModule), New: func() caddycore.Module { return new(rateLimitHandler) }}
}

func (handler *rateLimitHandler) Provision(caddycore.Context) error {
	if handler == nil || handler.RequestsPerSecond < 1 || handler.RequestsPerSecond > 10000 || handler.Burst < 1 || handler.Burst > 10000 || len(handler.TrustedProxies) > 32 {
		return errUnsupportedSettings
	}
	handler.trusted = make([]netip.Prefix, 0, len(handler.TrustedProxies))
	for _, raw := range handler.TrustedProxies {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || prefix != prefix.Masked() {
			return errUnsupportedSettings
		}
		handler.trusted = append(handler.trusted, prefix)
	}
	handler.clients = make(map[string]rateLimitBucket)
	return nil
}

func (handler *rateLimitHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request, next caddyhttp.Handler) error {
	client := handler.clientAddress(request)
	if client == "" || !handler.allow(client, time.Now()) {
		writer.Header().Set("Retry-After", "1")
		writer.WriteHeader(http.StatusTooManyRequests)
		return nil
	}
	return next.ServeHTTP(writer, request)
}

func (handler *rateLimitHandler) allow(client string, now time.Time) bool {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if handler.clients == nil {
		return false
	}
	if len(handler.clients) >= rateLimitMaxClients {
		for key, bucket := range handler.clients {
			if now.Sub(bucket.updatedAt) > rateLimitClientTTL {
				delete(handler.clients, key)
			}
		}
		if len(handler.clients) >= rateLimitMaxClients {
			if _, exists := handler.clients[client]; !exists {
				return false
			}
		}
	}
	bucket, exists := handler.clients[client]
	if !exists {
		bucket = rateLimitBucket{tokens: float64(handler.Burst), updatedAt: now}
	}
	if now.After(bucket.updatedAt) {
		bucket.tokens += now.Sub(bucket.updatedAt).Seconds() * float64(handler.RequestsPerSecond)
		if bucket.tokens > float64(handler.Burst) {
			bucket.tokens = float64(handler.Burst)
		}
	}
	allowed := bucket.tokens >= 1
	if allowed {
		bucket.tokens--
	}
	bucket.updatedAt = now
	handler.clients[client] = bucket
	return allowed
}

func (handler *rateLimitHandler) clientAddress(request *http.Request) string {
	remote, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		remote = request.RemoteAddr
	}
	client, err := netip.ParseAddr(remote)
	if err != nil {
		return ""
	}
	client = client.Unmap()
	if !handler.isTrusted(client) {
		return client.String()
	}
	values := request.Header.Values("X-Forwarded-For")
	if len(values) != 1 || len(values[0]) > 4096 {
		return client.String()
	}
	chain := strings.Split(values[0], ",")
	if len(chain) > 32 {
		return client.String()
	}
	for index := len(chain) - 1; index >= 0 && handler.isTrusted(client); index-- {
		candidate, parseErr := netip.ParseAddr(strings.TrimSpace(chain[index]))
		if parseErr != nil {
			return client.String()
		}
		client = candidate.Unmap()
	}
	return client.String()
}

func (handler *rateLimitHandler) isTrusted(address netip.Addr) bool {
	for _, prefix := range handler.trusted {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func rateLimitConfig(feature rateLimitFeature) map[string]any {
	return map[string]any{"handler": rateLimitHandlerModule, "requestsPerSecond": feature.RequestsPerSecond, "burst": feature.Burst, "trustedProxies": append([]string(nil), feature.TrustedProxies...)}
}

func activeHealthCheckConfig(feature healthCheckFeature) map[string]any {
	return map[string]any{"uri": feature.Path, "interval": strconv.Itoa(feature.IntervalSeconds) + "s", "timeout": strconv.Itoa(feature.TimeoutSeconds) + "s", "expect_status": feature.ExpectedStatus}
}

var _ caddycore.Module = (*rateLimitHandler)(nil)
var _ caddyhttp.MiddlewareHandler = (*rateLimitHandler)(nil)

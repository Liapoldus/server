package caddy

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func TestRateLimitHandlerReturns429AndRefillsBoundedTokenBucket(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	handler := &rateLimitHandler{
		RequestsPerSecond: 1,
		Burst:             1,
		clients:           make(map[string]rateLimitBucket),
	}
	if !handler.allow("192.0.2.20", now) || handler.allow("192.0.2.20", now) {
		t.Fatal("one-request burst did not enforce the per-client limit")
	}
	if !handler.allow("192.0.2.20", now.Add(time.Second)) {
		t.Fatal("token bucket did not refill at the configured rate")
	}

	handler.clients = make(map[string]rateLimitBucket)
	request := httptest.NewRequest(http.MethodGet, "http://server.example/", nil)
	request.RemoteAddr = "192.0.2.20:4150"
	response := httptest.NewRecorder()
	nextCalls := 0
	next := caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error { nextCalls++; return nil })
	if err := handler.ServeHTTP(response, request, next); err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	response = httptest.NewRecorder()
	if err := handler.ServeHTTP(response, request, next); err != nil {
		t.Fatalf("limited request returned an error: %v", err)
	}
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "1" || nextCalls != 1 {
		t.Fatalf("rate-limit response is not bounded and typed: status=%d headers=%v nextCalls=%d", response.Code, response.Header(), nextCalls)
	}
}

func TestRateLimitUsesForwardedAddressOnlyBehindTrustedProxies(t *testing.T) {
	handler := &rateLimitHandler{trusted: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}
	request := httptest.NewRequest(http.MethodGet, "http://server.example/", nil)
	request.RemoteAddr = "192.0.2.20:4150"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := handler.clientAddress(request); got != "192.0.2.20" {
		t.Fatalf("untrusted caller controlled X-Forwarded-For: %s", got)
	}

	request.RemoteAddr = "10.0.0.2:4150"
	request.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")
	if got := handler.clientAddress(request); got != "198.51.100.7" {
		t.Fatalf("trusted proxy chain did not resolve the first untrusted client: %s", got)
	}

	request.Header.Set("X-Forwarded-For", "198.51.100.7, malformed")
	if got := handler.clientAddress(request); got != "10.0.0.2" {
		t.Fatalf("malformed proxy chain was trusted: %s", got)
	}
}

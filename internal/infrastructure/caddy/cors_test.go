package caddy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func TestCORSHandlerAllowsConfiguredOriginAndMethod(t *testing.T) {
	handler := testCORSHandler()
	nextCalls := 0
	next := caddyhttp.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) error {
		nextCalls++
		writer.WriteHeader(http.StatusOK)
		return nil
	})
	request := httptest.NewRequest(http.MethodGet, "https://api.example/items", nil)
	request.Header.Set("Origin", "https://console.example")
	response := httptest.NewRecorder()
	if err := handler.ServeHTTP(response, request, next); err != nil {
		t.Fatalf("serve CORS request: %v", err)
	}
	if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "https://console.example" || nextCalls != 1 {
		t.Fatalf("configured origin should continue with CORS headers: status=%d headers=%v nextCalls=%d", response.Code, response.Header(), nextCalls)
	}
}

func TestCORSHandlerRejectsUnlistedOriginAndMethod(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
		origin string
	}{
		{name: "unlisted origin", method: http.MethodGet, origin: "https://evil.example"},
		{name: "unlisted method", method: http.MethodDelete, origin: "https://console.example"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := testCORSHandler()
			nextCalls := 0
			next := caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error { nextCalls++; return nil })
			request := httptest.NewRequest(test.method, "https://api.example/items", nil)
			request.Header.Set("Origin", test.origin)
			response := httptest.NewRecorder()
			if err := handler.ServeHTTP(response, request, next); err != nil {
				t.Fatalf("serve CORS request: %v", err)
			}
			if response.Code != http.StatusForbidden || response.Header().Get("Access-Control-Allow-Origin") != "" || nextCalls != 0 {
				t.Fatalf("disallowed cross-origin request was not blocked: status=%d headers=%v nextCalls=%d", response.Code, response.Header(), nextCalls)
			}
		})
	}
}

func TestCORSHandlerAnswersValidPreflightWithoutCallingNext(t *testing.T) {
	handler := testCORSHandler()
	nextCalls := 0
	next := caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error { nextCalls++; return nil })
	request := httptest.NewRequest(http.MethodOptions, "https://api.example/items", nil)
	request.Header.Set("Origin", "https://console.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "content-type, x-request-id")
	response := httptest.NewRecorder()
	if err := handler.ServeHTTP(response, request, next); err != nil {
		t.Fatalf("serve CORS preflight: %v", err)
	}
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "https://console.example" ||
		response.Header().Get("Access-Control-Allow-Methods") != "GET, POST" || response.Header().Get("Access-Control-Allow-Headers") != "Content-Type, X-Request-ID" ||
		response.Header().Get("Access-Control-Max-Age") != "300" || nextCalls != 0 {
		t.Fatalf("valid preflight response is incomplete: status=%d headers=%v nextCalls=%d", response.Code, response.Header(), nextCalls)
	}
}

func TestCORSHandlerRejectsPreflightHeadersOutsideAllowlist(t *testing.T) {
	handler := testCORSHandler()
	request := httptest.NewRequest(http.MethodOptions, "https://api.example/items", nil)
	request.Header.Set("Origin", "https://console.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "authorization")
	response := httptest.NewRecorder()
	if err := handler.ServeHTTP(response, request, caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error { return nil })); err != nil {
		t.Fatalf("serve CORS preflight: %v", err)
	}
	if response.Code != http.StatusForbidden {
		t.Fatalf("unlisted preflight header should be blocked, got %d", response.Code)
	}
}

func testCORSHandler() *corsHandler {
	return &corsHandler{
		Origins: []string{"https://console.example"}, Methods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Content-Type", "X-Request-ID"}, MaxAgeSeconds: 300,
	}
}

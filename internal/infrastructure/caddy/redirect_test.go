package caddy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func TestRedirectHandlerPreservesQueryAndDoesNotCallNext(t *testing.T) {
	handler := &redirectHandler{Target: "/new-path", StatusCode: http.StatusPermanentRedirect}
	request := httptest.NewRequest(http.MethodGet, "/old?search=docs", nil)
	response := httptest.NewRecorder()
	nextCalls := 0
	err := handler.ServeHTTP(response, request, caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error { nextCalls++; return nil }))
	if err != nil {
		t.Fatalf("redirect request failed: %v", err)
	}
	if response.Code != http.StatusPermanentRedirect || response.Header().Get("Location") != "/new-path?search=docs" || nextCalls != 0 {
		t.Fatalf("redirect response is incorrect: status=%d location=%q nextCalls=%d", response.Code, response.Header().Get("Location"), nextCalls)
	}
}

func TestRedirectTargetRejectsUnsafeAndAmbiguousDestinations(t *testing.T) {
	for _, target := range []string{
		"javascript:alert(1)",
		"//attacker.example/path",
		"https://user:password@example.test/",
		"https://example.test/path?token=secret",
		"/../outside",
		"/path#fragment",
	} {
		t.Run(target, func(t *testing.T) {
			if validRedirectTarget(target) {
				t.Fatalf("unsafe or ambiguous redirect target was accepted: %q", target)
			}
		})
	}
}

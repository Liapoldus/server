package caddy

import (
	"encoding/json"
	"testing"
)

func TestCompileSettingsReturns404ForUnmatchedRequests(t *testing.T) {
	configuration := []byte(`{"listeners":[{"id":"public-http","kind":"http","address":"127.0.0.1:19898","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[]}`)
	if err := New().Validate(configuration); err != nil {
		t.Fatalf("Caddy must accept the compiled unmatched-request fallback: %v", err)
	}
	compiled, err := compileSettings(configuration, 1, nil)
	if err != nil {
		t.Fatalf("compile settings: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(compiled, &document); err != nil {
		t.Fatalf("decode compiled Caddy config: %v", err)
	}
	apps, _ := document["apps"].(map[string]any)
	httpApp, _ := apps["http"].(map[string]any)
	servers, _ := httpApp["servers"].(map[string]any)
	server, _ := servers["public-http"].(map[string]any)
	routes, _ := server["routes"].([]any)
	if len(routes) < 2 {
		t.Fatalf("expected the header guard and unmatched-request fallback, got %#v", routes)
	}
	last, _ := routes[len(routes)-1].(map[string]any)
	if _, hasMatcher := last["match"]; hasMatcher {
		t.Fatalf("unmatched-request fallback must match all remaining requests: %#v", last)
	}
	if last["terminal"] != true {
		t.Fatalf("unmatched-request fallback must be terminal: %#v", last)
	}
	handlers, _ := last["handle"].([]any)
	if len(handlers) != 1 {
		t.Fatalf("expected one fallback handler, got %#v", handlers)
	}
	handler, _ := handlers[0].(map[string]any)
	if handler["handler"] != "static_response" || handler["status_code"] != float64(404) {
		t.Fatalf("unmatched requests must receive HTTP 404: %#v", handler)
	}
}

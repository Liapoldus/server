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

func TestCompileStaticReleaseAllowsMatchingLegacyPrefixDuringMigration(t *testing.T) {
	configuration := []byte(`{"listeners":[{"id":"web","kind":"http","address":"127.0.0.1:19899","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[{"id":"docs","listenerId":"web","match":{"methods":["GET"],"path":{"type":"prefix","value":"/docs"}},"handler":{"type":"static","siteId":"docs","releaseId":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","mountPrefix":"/docs"}}]}`)
	if err := New().Validate(configuration); err != nil {
		t.Fatalf("matching legacy prefix must migrate to its immutable release mount: %v", err)
	}
	invalid := []byte(`{"listeners":[{"id":"web","kind":"http","address":"127.0.0.1:19899","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[{"id":"docs","listenerId":"web","match":{"methods":["POST"],"path":{"type":"exact","value":"/docs"}},"handler":{"type":"static","siteId":"docs","releaseId":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","mountPrefix":"/docs"}}]}`)
	if err := New().Validate(invalid); err == nil {
		t.Fatal("incompatible legacy static matcher was accepted")
	}
}

func TestCompileStaticRouteEnablesCompressionAndCacheByDefault(t *testing.T) {
	configuration := []byte(`{"listeners":[{"id":"web","kind":"http","address":"127.0.0.1:19899","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[{"id":"docs","listenerId":"web","handler":{"type":"static","siteId":"docs","releaseId":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","mountPrefix":"/docs"}}]}`)
	if err := New().Validate(configuration); err != nil {
		t.Fatalf("Caddy must accept the compiled default feature handlers: %v", err)
	}
	compiled, err := compileSettings(configuration, 1, nil)
	if err != nil {
		t.Fatalf("compile settings: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(compiled, &document); err != nil {
		t.Fatalf("decode compiled Caddy config: %v", err)
	}
	server := document["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["web"].(map[string]any)
	routes := server["routes"].([]any)
	handlers := routes[1].(map[string]any)["handle"].([]any)
	if len(handlers) != 3 {
		t.Fatalf("static route should compile compression, cache policy and file server handlers: %#v", handlers)
	}
	if handlers[0].(map[string]any)["handler"] != "encode" {
		t.Fatalf("static route compression is not enabled by default: %#v", handlers[0])
	}
	cache := handlers[1].(map[string]any)
	if cache["handler"] != "headers" {
		t.Fatalf("static route cache policy is missing: %#v", cache)
	}
	response := cache["response"].(map[string]any)
	set := response["set"].(map[string]any)
	if got := set["Cache-Control"].([]any)[0]; got != "public, max-age=3600" {
		t.Fatalf("unexpected static default cache policy: %#v", got)
	}
}

func TestRouteFeatureOverridesCanDisableDefaultsWithoutDroppingValues(t *testing.T) {
	defaults := routeFeatureDefaults{Compression: compressionFeature{Mode: "enabled", Algorithms: []string{"zstd", "gzip"}}, Cache: cacheFeature{Mode: "enabled", Visibility: "public", MaxAgeSeconds: intPointer(3600)}}
	overrides := routeFeatureOverrides{Compression: compressionFeature{Mode: "disabled", Algorithms: []string{"gzip"}}, Cache: cacheFeature{Mode: "inherit", Visibility: "private", MaxAgeSeconds: intPointer(120)}}
	resolved, err := resolveRouteFeatures("static", defaults, overrides)
	if err != nil {
		t.Fatalf("resolve route features: %v", err)
	}
	if resolved.Compression.Mode != "disabled" || resolved.Cache.Visibility != "public" || *resolved.Cache.MaxAgeSeconds != 3600 {
		t.Fatalf("disabled and inherited features must resolve against defaults: %#v", resolved)
	}
	if overrides.Compression.Algorithms[0] != "gzip" || *overrides.Cache.MaxAgeSeconds != 120 {
		t.Fatalf("disabled override values must remain available for re-enabling: %#v", overrides)
	}
}

func TestServiceRouteFeatureDefaultsKeepCompressionAndCacheDisabled(t *testing.T) {
	resolved, err := resolveRouteFeatures("service", routeFeatureDefaults{}, routeFeatureOverrides{})
	if err != nil {
		t.Fatalf("resolve service route defaults: %v", err)
	}
	if resolved.Compression.Mode != "disabled" || resolved.Cache.Mode != "disabled" {
		t.Fatalf("API route must not enable compression or caching implicitly: %#v", resolved)
	}
}

func TestCompileRouteCORSAndHeadersFeatures(t *testing.T) {
	configuration := []byte(`{"listeners":[{"id":"web","kind":"http","address":"127.0.0.1:19901","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"defaults":{"reverseProxy":{"cors":{"mode":"enabled","origins":["https://console.example"],"methods":["GET","POST"],"allowedHeaders":["Content-Type"],"maxAgeSeconds":300},"headers":{"mode":"enabled","request":{"X-Edge":"studio"},"response":{"X-Frame-Options":"DENY"}}}},"routes":[{"id":"api","listenerId":"web","features":{"cors":{"mode":"disabled","origins":["https://other.example"]},"headers":{"mode":"enabled","response":{"X-Route":"api"}}},"handler":{"type":"reverseProxy","upstreams":[{"origin":"http://127.0.0.1:19902","weight":100}]}}]}`)
	if err := New().Validate(configuration); err != nil {
		t.Fatalf("Caddy must accept CORS and header route handlers: %v", err)
	}
	compiled, err := compileSettings(configuration, 1, nil)
	if err != nil {
		t.Fatalf("compile settings: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(compiled, &document); err != nil {
		t.Fatalf("decode compiled Caddy config: %v", err)
	}
	server := document["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["web"].(map[string]any)
	routes := server["routes"].([]any)
	handlers := routes[1].(map[string]any)["handle"].([]any)
	if len(handlers) != 2 {
		t.Fatalf("disabled route CORS should be omitted while headers and proxy compile: %#v", handlers)
	}
	if handlers[0].(map[string]any)["handler"] != "headers" {
		t.Fatalf("shared and route response headers were not compiled: %#v", handlers[0])
	}
	request := handlers[0].(map[string]any)["request"].(map[string]any)["set"].(map[string]any)
	response := handlers[0].(map[string]any)["response"].(map[string]any)["set"].(map[string]any)
	if request["X-Edge"].([]any)[0] != "studio" || response["X-Frame-Options"].([]any)[0] != "DENY" || response["X-Route"].([]any)[0] != "api" {
		t.Fatalf("unexpected effective header operations: request=%#v response=%#v", request, response)
	}
}

func TestRouteCORSOverrideInheritsAndRetainsDisabledValues(t *testing.T) {
	defaults := routeFeatureDefaults{CORS: corsFeature{Mode: "enabled", Origins: []string{"https://console.example"}, Methods: []string{"GET"}, MaxAgeSeconds: intPointer(300)}}
	overrides := routeFeatureOverrides{CORS: corsFeature{Mode: "disabled", Origins: []string{"https://other.example"}}}
	resolved, err := resolveRouteFeatures("reverseProxy", defaults, overrides)
	if err != nil {
		t.Fatalf("resolve route features: %v", err)
	}
	if resolved.CORS.Mode != "disabled" || len(resolved.CORS.Origins) != 1 || resolved.CORS.Origins[0] != "https://console.example" {
		t.Fatalf("disabled override must inherit effective values without losing stored route values: %#v", resolved)
	}
	if overrides.CORS.Origins[0] != "https://other.example" {
		t.Fatalf("disabled route values must remain available for re-enabling: %#v", overrides.CORS)
	}
}

func TestCompileReverseProxyRouteSecurityAndHealthFeatures(t *testing.T) {
	configuration := []byte(`{"listeners":[{"id":"web","kind":"http","address":"127.0.0.1:19920","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[{"id":"api","listenerId":"web","features":{"rateLimit":{"mode":"enabled","requestsPerSecond":12,"burst":20,"trustedProxies":["10.0.0.0/8"]},"healthCheck":{"mode":"enabled","path":"/health","intervalSeconds":5,"timeoutSeconds":1,"expectedStatus":200},"rewrite":{"mode":"enabled","path":"/internal"}},"handler":{"type":"reverseProxy","upstreams":[{"origin":"http://127.0.0.1:19921","weight":100}]}}]}`)
	runtime := New()
	if err := runtime.Validate(configuration); err != nil {
		t.Fatalf("Caddy must accept the compiled security and health features: %v", err)
	}
	compiled, err := compileSettings(configuration, 1, nil)
	if err != nil {
		t.Fatalf("compile reverse proxy features: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(compiled, &document); err != nil {
		t.Fatalf("decode compiled Caddy config: %v", err)
	}
	server := document["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["web"].(map[string]any)
	routes := server["routes"].([]any)
	handlers := routes[1].(map[string]any)["handle"].([]any)
	if len(handlers) != 3 || handlers[0].(map[string]any)["handler"] != rateLimitHandlerModule ||
		handlers[1].(map[string]any)["handler"] != "rewrite" || handlers[1].(map[string]any)["uri"] != "/internal" {
		t.Fatalf("rate limit and rewrite middleware are not ordered before the proxy: %#v", handlers)
	}
	proxy := handlers[2].(map[string]any)
	active, ok := proxy["health_checks"].(map[string]any)["active"].(map[string]any)
	if !ok || active["uri"] != "/health" || active["interval"] != "5s" || active["timeout"] != "1s" || active["expect_status"] != float64(200) {
		t.Fatalf("active upstream health check was not compiled: %#v", proxy)
	}
}

func TestCompileRejectsAmbiguousActiveHealthCheckUpstreams(t *testing.T) {
	configuration := []byte(`{"listeners":[{"id":"web","kind":"http","address":"127.0.0.1:19923","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[{"id":"api","listenerId":"web","features":{"healthCheck":{"mode":"enabled","path":"/health","intervalSeconds":5,"timeoutSeconds":1,"expectedStatus":200}},"handler":{"type":"reverseProxy","upstreams":[{"origin":"http://127.0.0.1:19924","weight":100},{"origin":"http://127.0.0.1:19924","weight":1}]}}]}`)
	if _, err := compileSettings(configuration, 1, nil); err == nil {
		t.Fatal("active probes must reject duplicate dial addresses because their transport settings are ambiguous")
	}
}

func TestCompileRedirectRouteUsesValidatedRedirectHandler(t *testing.T) {
	configuration := []byte(`{"listeners":[{"id":"web","kind":"http","address":"127.0.0.1:19922","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[{"id":"old-path","listenerId":"web","handler":{"type":"redirect","target":"/new-path","statusCode":308}}]}`)
	if err := New().Validate(configuration); err != nil {
		t.Fatalf("Caddy must accept the compiled redirect route: %v", err)
	}
	compiled, err := compileSettings(configuration, 1, nil)
	if err != nil {
		t.Fatalf("compile redirect route: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(compiled, &document); err != nil {
		t.Fatalf("decode compiled Caddy config: %v", err)
	}
	server := document["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["web"].(map[string]any)
	routes := server["routes"].([]any)
	handlers := routes[1].(map[string]any)["handle"].([]any)
	if len(handlers) != 1 || handlers[0].(map[string]any)["handler"] != redirectHandlerModule ||
		handlers[0].(map[string]any)["target"] != "/new-path" || handlers[0].(map[string]any)["statusCode"] != float64(308) {
		t.Fatalf("redirect route was not compiled as its typed handler: %#v", handlers)
	}
}

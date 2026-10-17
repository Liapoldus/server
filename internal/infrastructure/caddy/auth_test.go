package caddy

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func TestServiceAuthHandlerChecksOpaqueSecretForEachScheme(t *testing.T) {
	secret := "server-auth-secret-that-must-not-be-serialized"
	secretMAC, err := authSecretMAC([]byte(secret))
	if err != nil {
		t.Fatalf("create secret MAC: %v", err)
	}
	if strings.Contains(secretMAC, secret) {
		t.Fatal("secret MAC contains the source secret")
	}

	tests := []struct {
		name      string
		handler   serviceAuthHandler
		setHeader func(*http.Request)
	}{
		{
			name:    "bearer",
			handler: serviceAuthHandler{Scheme: "bearer", SecretMAC: secretMAC},
			setHeader: func(request *http.Request) {
				request.Header.Set("Authorization", "Bearer "+secret)
			},
		},
		{
			name:    "api key",
			handler: serviceAuthHandler{Scheme: "apiKey", SecretMAC: secretMAC, HeaderName: "X-API-Key"},
			setHeader: func(request *http.Request) {
				request.Header.Set("X-API-Key", secret)
			},
		},
		{
			name:    "basic",
			handler: serviceAuthHandler{Scheme: "basic", SecretMAC: secretMAC, Username: "operator", Realm: "admin"},
			setHeader: func(request *http.Request) {
				request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("operator:"+secret)))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nextCalls := 0
			next := caddyhttp.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) error {
				nextCalls++
				writer.WriteHeader(http.StatusNoContent)
				return nil
			})
			request := httptest.NewRequest(http.MethodGet, "https://api.example/private", nil)
			test.setHeader(request)
			response := httptest.NewRecorder()
			if err := test.handler.ServeHTTP(response, request, next); err != nil {
				t.Fatalf("authorized request failed: %v", err)
			}
			if response.Code != http.StatusNoContent || nextCalls != 1 {
				t.Fatalf("valid credential was rejected: status=%d nextCalls=%d", response.Code, nextCalls)
			}

			request = httptest.NewRequest(http.MethodGet, "https://api.example/private", nil)
			request.Header.Set("Authorization", "Bearer invalid")
			response = httptest.NewRecorder()
			if err := test.handler.ServeHTTP(response, request, next); err != nil {
				t.Fatalf("unauthorized request returned an error: %v", err)
			}
			if response.Code != http.StatusUnauthorized || nextCalls != 1 {
				t.Fatalf("invalid credential reached the handler: status=%d nextCalls=%d", response.Code, nextCalls)
			}
			if test.handler.Scheme == "basic" && response.Header().Get("WWW-Authenticate") != `Basic realm="admin", charset="UTF-8"` {
				t.Fatalf("Basic challenge is missing: %v", response.Header())
			}
		})
	}
}

func TestCompiledHTTPAuthConfigurationContainsNoSecretValue(t *testing.T) {
	const reference = "ssr:v1:00112233445566778899aabbccddeeff"
	const secret = "compiled-secret-value-must-not-appear"
	configuration := []byte(fmt.Sprintf(`{"listeners":[{"id":"web","kind":"http","address":"127.0.0.1:19910","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[{"id":"api","listenerId":"web","features":{"auth":{"mode":"enabled","scheme":"bearer","secretRef":%q}},"handler":{"type":"reverseProxy","upstreams":[{"origin":"http://127.0.0.1:19911","weight":100}]}}]}`, reference))
	secrets := map[string][]byte{reference: []byte(secret)}
	if err := New().ValidateWithSecrets(configuration, secrets); err != nil {
		t.Fatalf("Caddy must accept the scoped auth configuration: %v", err)
	}
	compiled, err := compileSettings(configuration, 1, secrets)
	if err != nil {
		t.Fatalf("compile auth configuration: %v", err)
	}
	if strings.Contains(string(compiled), secret) {
		t.Fatal("compiled Caddy configuration contains the plaintext secret")
	}
	secretMAC := secretMACForTest(secret, t)
	if !strings.Contains(string(compiled), authHandlerModule) || !strings.Contains(string(compiled), secretMAC) {
		t.Fatalf("compiled auth handler or its derived credential verifier is missing: %s", compiled)
	}
	if strings.Contains(string(compiled), reference) {
		t.Fatal("compiled Caddy configuration contains the Core opaque reference")
	}
}

func secretMACForTest(secret string, t *testing.T) string {
	t.Helper()
	value, err := authSecretMAC([]byte(secret))
	if err != nil {
		t.Fatalf("create test MAC: %v", err)
	}
	return value
}

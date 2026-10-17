package caddy

import (
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestRuntimeReturns404ForUnmatchedRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve listener address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release listener address: %v", err)
	}

	configuration := []byte(fmt.Sprintf(`{"listeners":[{"id":"public-http","kind":"http","address":%q,"hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[]}`, address))
	runtime := New()
	if err := runtime.Activate(configuration); err != nil {
		t.Fatalf("activate Caddy runtime: %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Stop(); err != nil {
			t.Errorf("stop Caddy runtime: %v", err)
		}
	})

	client := &http.Client{Timeout: time.Second}
	var response *http.Response
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err = client.Get("http://" + address + "/_unmatched-route")
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("request unmatched route after Caddy activation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unmatched route status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
}

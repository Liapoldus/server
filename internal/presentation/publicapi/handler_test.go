package publicapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type callerStub struct {
	target  string
	method  string
	payload []byte
	result  []byte
	err     error
}

func (caller *callerStub) Call(_ context.Context, target, method string, payload []byte) ([]byte, error) {
	caller.target, caller.method = target, method
	caller.payload = append([]byte(nil), payload...)
	return append([]byte(nil), caller.result...), caller.err
}

func TestPeerEndpointUsesGenericCallerAndReturnsPeerResult(t *testing.T) {
	caller := &callerStub{result: []byte(`{"status":200,"headers":{"content-type":"application/json"},"body":"ok"}`)}
	handler := Handler(caller, http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodPost, "/api/peer?target=service-a&method=sample.invoke", strings.NewReader(`{"method":"POST","path":"/sample"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != string(caller.result) || response.Header().Get("content-type") != "application/json" {
		t.Fatalf("unexpected peer response: status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
	if caller.target != "service-a" || caller.method != "sample.invoke" || string(caller.payload) != `{"method":"POST","path":"/sample"}` {
		t.Fatalf("unexpected generic peer call: target=%q method=%q payload=%s", caller.target, caller.method, caller.payload)
	}
}

func TestHealthAndMetricsAreDelegatedToSDKObservability(t *testing.T) {
	observability := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-sdk-observability", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	handler := Handler(nil, observability)
	for _, path := range []string{"/healthz", "/metrics"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNoContent || response.Header().Get("x-sdk-observability") != path {
			t.Fatalf("path %s was not delegated to observability: status=%d headers=%v", path, response.Code, response.Header())
		}
	}
}

func TestPeerCallFailureIsRedacted(t *testing.T) {
	caller := &callerStub{err: errors.New("sensitive peer endpoint and payload")}
	handler := Handler(caller, http.NotFoundHandler())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/peer?target=service-a&method=sample.invoke", strings.NewReader(`{}`)))
	body, err := io.ReadAll(response.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusBadGateway || strings.Contains(string(body), "sensitive") || !strings.Contains(string(body), "peer_call_failed") {
		t.Fatalf("peer failure was not safely mapped: status=%d body=%s", response.Code, body)
	}
}

package caddy

import (
	"context"
	"io"
	"strings"
	"testing"

	protocolpeer "github.com/Liapoldus/pluginprotocol/v3/presentation/peer"
)

type dispatchCallerStub struct{}

func (dispatchCallerStub) Call(context.Context, string, string, []byte) ([]byte, error) {
	return nil, protocolpeer.ErrUnavailable
}

func (dispatchCallerStub) OpenStream(context.Context, string, string) (protocolpeer.Stream, io.Closer, error) {
	return nil, nil, protocolpeer.ErrUnavailable
}

func TestPluginRouteValidatesWithoutStaticPeerEndpoint(t *testing.T) {
	runtime := New()
	if err := runtime.SetDispatchCaller(dispatchCallerStub{}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Stop() }()
	settings := []byte(`{"listeners":[{"id":"web","kind":"http","address":"127.0.0.1:19555","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[{"id":"echo","listenerId":"web","match":{"methods":["GET"],"path":{"type":"exact","value":"/echo"}},"handler":{"type":"plugin","instanceId":"echo","capability":"echo.handle","mode":"call"}}]}`)
	if err := runtime.Validate(settings); err != nil {
		t.Fatalf("Core-backed opaque peer route should validate before Echo is dialed: %v", err)
	}
	compiled, err := runtime.compile(settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compiled), `"callerSetId"`) || !strings.Contains(string(compiled), `"id":"echo"`) || strings.Contains(string(compiled), `"endpoint"`) {
		t.Fatalf("compiled route retained a static peer endpoint or lost the injected resolver: %s", compiled)
	}
}

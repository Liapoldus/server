package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/Liapoldus/caddy-plugin/contracts"
	"github.com/Liapoldus/caddy-plugin/internal/application"
	caddyruntime "github.com/Liapoldus/caddy-plugin/internal/infrastructure/caddy"
	pluginadapter "github.com/Liapoldus/caddy-plugin/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	pluginsdk "github.com/Liapoldus/pluginprotocol/presentation/sdk"
)

type target struct {
	pluginv1.UnimplementedPluginServiceServer
	request httpRequest
}

type httpRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

func (target *target) Manifest(context.Context, *pluginv1.ManifestRequest) (*pluginv1.Manifest, error) {
	return &pluginv1.Manifest{
		Name: "fixture", ProtocolVersion: pluginsdk.ProtocolVersion,
		Capabilities: []string{"http.echo"},
		CapabilityDescriptors: []*pluginv1.CapabilityDescriptor{{
			Capability: "http.echo", Modes: []pluginv1.InvocationMode{pluginv1.InvocationMode_INVOCATION_MODE_CALL},
		}},
	}, nil
}

func (*target) ConfigSchema(context.Context, *pluginv1.ConfigSchemaRequest) (*pluginv1.ConfigSchema, error) {
	return &pluginv1.ConfigSchema{}, nil
}

func (instance *target) Call(_ context.Context, request *pluginv1.CallRequest) (*pluginv1.CallResponse, error) {
	if err := json.Unmarshal(request.GetPayload(), &instance.request); err != nil {
		return nil, err
	}
	return &pluginv1.CallResponse{Payload: []byte(`{"status":202,"headers":{"Content-Type":"text/plain"},"body":"plugin-response"}`)}, nil
}

func main() {
	contract, err := contracts.LoadHTTPDispatch()
	check(err)
	storageDirectory, err := os.MkdirTemp("", "liapoldus-caddy-http-dispatch-")
	check(err)
	defer func() { _ = os.RemoveAll(storageDirectory) }()

	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	targetService := &target{}
	targetServer := pluginsdk.NewServer(targetService, pluginsdk.ServerOptions{})
	go func() { _ = targetServer.Serve(targetListener) }()
	defer targetServer.GracefulStop()

	publicListener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	publicAddress := publicListener.Addr().String()
	check(publicListener.Close())

	configuration := caddyruntime.New()
	application, err := application.NewConfiguration(configuration)
	check(err)
	defer func() { _ = application.Stop() }()
	apps := map[string]any{
		contract.App: map[string]any{"instances": []any{map[string]any{
			"id": "fixture", "endpoint": targetListener.Addr().String(), "timeoutMillis": 5000,
		}}},
		"http": map[string]any{"servers": map[string]any{"test": map[string]any{
			"listen": []string{publicAddress},
			"routes": []any{map[string]any{"handle": []any{map[string]any{
				"handler": moduleName(contract.Module), "instance": "fixture", "capability": "http.echo",
			}}}},
		}}},
	}
	settings, err := json.Marshal(map[string]any{"schemaVersion": 1, "config": map[string]any{
		"admin": map[string]any{"disabled": true, "config": map[string]any{"persist": false}}, "apps": apps,
		"storage": map[string]any{"module": "file_system", "root": storageDirectory},
	}})
	check(err)
	service, err := pluginadapter.New(application, func() {})
	check(err)
	result, err := service.ConfigApply(context.Background(), &pluginv1.ConfigApplyRequest{Config: settings, SettingsRevision: "revision-1"})
	check(err)

	client := &http.Client{Timeout: 5 * time.Second}
	request, err := http.NewRequest(http.MethodPost, "http://"+publicAddress+"/submit", nil)
	check(err)
	request.Header.Set("X-Request-ID", "fixture-request")
	response, err := client.Do(request)
	check(err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	check(err)
	check(json.NewEncoder(os.Stdout).Encode(map[string]any{
		"revision": result.GetSettingsRevision(), "status": response.StatusCode,
		"contentType": response.Header.Get("Content-Type"), "body": string(body),
		"method": targetService.request.Method, "path": targetService.request.Path,
	}))
}

func moduleName(module string) string {
	const prefix = "http.handlers."
	if len(module) >= len(prefix) && module[:len(prefix)] == prefix {
		return module[len(prefix):]
	}
	return module
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture failed")
		os.Exit(1)
	}
}

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"liapoldus.local/server-plugin/internal/application"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	pluginadapter "liapoldus.local/server-plugin/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	pluginsdk "github.com/Liapoldus/pluginprotocol/presentation/sdk"
)

const (
	fixtureInstanceID = "fixture"
	fixtureReplicaID  = "urn:liapoldus:plugin:caddy-fixture:replica:one"
	fixtureRelease    = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
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

func (*target) Bootstrap(_ context.Context, request *pluginv1.BootstrapRequest) (*pluginv1.BootstrapResult, error) {
	return &pluginv1.BootstrapResult{Accepted: request.GetInstanceId() == fixtureInstanceID}, nil
}

func (*target) ConfigSchema(context.Context, *pluginv1.ConfigSchemaRequest) (*pluginv1.ConfigSchema, error) {
	return &pluginv1.ConfigSchema{}, nil
}

func (*target) ConfigApply(_ context.Context, request *pluginv1.ConfigApplyRequest) (*pluginv1.ConfigApplyResult, error) {
	return &pluginv1.ConfigApplyResult{Applied: true, SettingsRevision: request.GetSettingsRevision()}, nil
}

func (instance *target) Call(_ context.Context, request *pluginv1.CallRequest) (*pluginv1.CallResponse, error) {
	if err := json.Unmarshal(request.GetPayload(), &instance.request); err != nil {
		return nil, err
	}
	return &pluginv1.CallResponse{Payload: []byte(`{"status":202,"headers":{"Content-Type":"text/plain"},"body":"plugin-response"}`)}, nil
}

func main() {
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	targetService := &target{}
	targetServer := pluginsdk.NewServer(targetService, pluginsdk.ServerOptions{
		InstanceID: fixtureInstanceID, ReplicaIdentityURI: fixtureReplicaID, ReleaseDigest: fixtureRelease,
	})
	go func() { _ = targetServer.Serve(targetListener) }()
	defer targetServer.GracefulStop()

	lifecycleContext, cancelLifecycle := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelLifecycle()
	bootstrapClient, err := pluginsdk.DialContext(lifecycleContext, targetListener.Addr().String())
	check(err)
	defer bootstrapClient.Close()
	bootstrapConfig := []byte("{}")
	_, err = bootstrapClient.BootstrapAndHandshake(lifecycleContext,
		&pluginv1.BootstrapRequest{InstanceId: fixtureInstanceID}, bootstrapConfig, "bootstrap-revision", nil)
	check(err)
	settingsDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(bootstrapConfig))
	_, err = bootstrapClient.ApplyDispatch(lifecycleContext, &pluginv1.DispatchApplyRequest{
		Generation: 1, InstanceId: fixtureInstanceID, SettingsDigest: settingsDigest, ReleaseDigest: fixtureRelease,
		Capabilities: []*pluginv1.CapabilityDispatchScope{{
			Capability: "http.echo", Modes: []pluginv1.InvocationMode{pluginv1.InvocationMode_INVOCATION_MODE_CALL},
		}},
	}, fixtureReplicaID)
	check(err)

	publicListener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	publicAddress := publicListener.Addr().String()
	check(publicListener.Close())

	runtime := caddyruntime.New()
	check(runtime.SetDispatchTargets([]caddyruntime.DispatchTarget{{
		ID: "fixture", Endpoint: targetListener.Addr().String(), TimeoutMillis: 5000,
	}}))
	serviceConfiguration, err := application.NewConfiguration(runtime)
	check(err)
	defer func() { _ = serviceConfiguration.Stop() }()
	settings, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"listeners": []any{map[string]any{
				"id": "web", "kind": "http", "address": publicAddress,
				"hostnames": []string{}, "protocols": []string{"http1"}, "tls": map[string]any{"mode": "disabled"},
			}},
			"routes": []any{map[string]any{
				"id": "dispatch", "listenerId": "web",
				"handler": map[string]any{"type": "plugin", "instanceId": "fixture", "capability": "http.echo", "mode": "call"},
			}},
		},
	})
	check(err)
	service, err := pluginadapter.New(serviceConfiguration, func() {})
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

func check(err error) {
	if err != nil {
		panic(err)
	}
}

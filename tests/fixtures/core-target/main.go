package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/Liapoldus/pluginprotocol/pluginv1"
	pluginsdk "github.com/Liapoldus/pluginprotocol/presentation/sdk"
)

type target struct{ pluginv1.UnimplementedPluginServiceServer }

func (*target) Manifest(context.Context, *pluginv1.ManifestRequest) (*pluginv1.Manifest, error) {
	return &pluginv1.Manifest{
		Name: "http-target", ProtocolVersion: pluginsdk.ProtocolVersion,
		Capabilities: []string{"http.echo"},
		CapabilityDescriptors: []*pluginv1.CapabilityDescriptor{{
			Capability: "http.echo", Modes: []pluginv1.InvocationMode{pluginv1.InvocationMode_INVOCATION_MODE_CALL},
		}},
	}, nil
}

func (*target) ConfigSchema(context.Context, *pluginv1.ConfigSchemaRequest) (*pluginv1.ConfigSchema, error) {
	return &pluginv1.ConfigSchema{}, nil
}

func (*target) Call(context.Context, *pluginv1.CallRequest) (*pluginv1.CallResponse, error) {
	return &pluginv1.CallResponse{Payload: []byte(`{"status":202,"headers":{"Content-Type":"text/plain"},"body":"plugin-response"}`)}, nil
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return os.ErrInvalid
	}
	listener, err := net.Listen("tcp", os.Args[1])
	if err != nil {
		return err
	}
	server := pluginsdk.NewServer(&target{}, pluginsdk.ServerOptions{})
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	select {
	case <-stop:
		server.GracefulStop()
		return nil
	case err := <-serveResult:
		return err
	}
}

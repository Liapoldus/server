package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Liapoldus/caddy-plugin/contracts"
	"github.com/Liapoldus/caddy-plugin/internal/application"
	caddyruntime "github.com/Liapoldus/caddy-plugin/internal/infrastructure/caddy"
	pluginadapter "github.com/Liapoldus/caddy-plugin/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	pluginsdk "github.com/Liapoldus/pluginprotocol/presentation/sdk"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	_, err := contracts.Load()
	if err != nil {
		return err
	}
	runtime := caddyruntime.New()
	configuration, err := application.NewConfiguration(runtime)
	if err != nil {
		return err
	}
	shutdown := make(chan struct{})
	service, err := pluginadapter.New(configuration, func() {
		close(shutdown)
	})
	if err != nil {
		return err
	}
	listener, err := pluginsdk.ListenInherited()
	if err != nil {
		return err
	}
	credentials, err := pluginsdk.AcceptInheritedLocalBootstrap(context.Background())
	if err != nil {
		_ = listener.Close()
		return err
	}
	grpcServer, err := newServer(service, credentials)
	if err != nil {
		_ = listener.Close()
		return err
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- grpcServer.Serve(listener) }()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	select {
	case <-shutdown:
	case <-stop:
	case err := <-serveResult:
		return err
	}
	grpcServer.GracefulStop()
	return configuration.Stop()
}

func newServer(service pluginv1.PluginServiceServer, credentials pluginsdk.LocalPeerCredentials) (*grpc.Server, error) {
	return pluginsdk.NewLocalServer(service, pluginsdk.LocalServerOptions{Credentials: credentials, Limits: pluginsdk.ServerOptions{}})
}

package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"liapoldus.local/server-plugin/internal/application"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	pluginadapter "liapoldus.local/server-plugin/internal/presentation/restplugin"
	pluginsdk "liapoldus.local/plugin-sdk/infrastructure"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	bootstrap, err := pluginsdk.AcceptInheritedProcessBootstrap()
	if err != nil {
		return err
	}
	defer func() { _ = bootstrap.Close() }()

	contract, err := pluginsdk.LoadHTTPContract()
	if err != nil {
		return err
	}
	runtime := caddyruntime.New()
	configuration, err := application.NewConfiguration(runtime)
	if err != nil {
		return err
	}
	defer func() { _ = configuration.Stop() }()

	server, listener, err := pluginadapter.NewMutualTLSServer(
		configuration,
		bootstrap.ConfigurationSource,
		bootstrap.Listener,
		bootstrap.ReplicaCertificate,
		bootstrap.CoreControlPlaneTrustRoots,
		contract.TransportSecurity.MinimumTLSVersion,
	)
	if err != nil {
		return err
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	select {
	case <-stop:
	case err := <-serveResult:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		_ = server.Close()
		return err
	}
	return nil
}

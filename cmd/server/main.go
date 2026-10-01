package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	pluginsdk "github.com/Liapoldus/plugin-sdk/infrastructure"
	"github.com/Liapoldus/pluginprotocol/presentation/peer"
	"liapoldus.local/server-plugin/internal/application"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	pluginadapter "liapoldus.local/server-plugin/internal/presentation/restplugin"
)

type options struct {
	instanceID     string
	replicaID      string
	restListen     string
	coreURL        string
	coreServerName string
	coreName       string
	coreURI        string
	coreClientName string
	coreClientURI  string
	caFile         string
	serverCert     string
	serverKey      string
	clientCert     string
	clientKey      string
	crlFile        string
	peerTargetID   string
	peerEndpoint   string
	peerIdentity   string
	peerExpected   string
	peerCAFile     string
	peerCertFile   string
	peerKeyFile    string
}

func main() {
	if run(os.Args[1:]) != nil {
		os.Exit(1)
	}
}

func run(args []string) (runErr error) {
	stage := "bootstrap"
	defer func() {
		if runErr != nil {
			_, _ = io.WriteString(os.Stderr, "Server failed at "+stage+"\n")
		}
	}()
	settings, err := parseOptions(args)
	if err != nil {
		return err
	}
	stage = "sdk-contract"
	contract, err := pluginsdk.LoadHTTPContract()
	if err != nil {
		return err
	}
	stage = "credentials"
	credentials, err := pluginsdk.NewFileCredentialsProvider(contract, pluginsdk.CredentialsMaterial{
		CAFile:                settings.caFile,
		ServerCertificateFile: settings.serverCert, ServerKeyFile: settings.serverKey,
		ClientCertificateFile: settings.clientCert, ClientKeyFile: settings.clientKey,
	})
	if err != nil {
		return err
	}
	stage = "credentials-load"
	material, err := credentials.Credentials()
	if err != nil {
		return err
	}
	stage = "revocation"
	revocation, err := pluginsdk.NewRevocation(pluginsdk.RevocationConfiguration{
		Authorities: material.TrustAuthorities(), Files: []string{settings.crlFile},
	})
	if err != nil {
		return err
	}
	stage = "peer-identities"
	corePeer, err := sdkmodels.NewPeerIdentity(settings.coreName, settings.coreURI)
	if err != nil {
		return err
	}
	coreClientPeer, err := sdkmodels.NewPeerIdentity(settings.coreClientName, settings.coreClientURI)
	if err != nil {
		return err
	}
	identity, err := sdkmodels.NewReplicaIdentity(settings.instanceID, settings.replicaID)
	if err != nil {
		return err
	}
	stage = "control-client"
	controlClient, err := pluginsdk.NewMutualTLSClient(contract, credentials, pluginsdk.MutualTLSClientConfig{
		Peer: corePeer, ServerName: settings.coreServerName, Revocation: revocation,
	})
	if err != nil {
		return err
	}
	defer controlClient.CloseIdleConnections()
	stage = "config-source"
	source, err := pluginsdk.NewCoreConfigurationSource(contract, settings.coreURL, controlClient)
	if err != nil {
		return err
	}
	stage = "product-runtime"
	runtime := caddyruntime.New()
	if settings.peerTargetID != "" {
		stage = "peer-credentials"
		certificate, err := tls.LoadX509KeyPair(settings.peerCertFile, settings.peerKeyFile)
		if err != nil {
			return err
		}
		rootsPEM, err := os.ReadFile(settings.peerCAFile)
		if err != nil {
			return err
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(rootsPEM) {
			return errors.New("invalid peer trust roots")
		}
		if err := runtime.SetDispatchTargets([]caddyruntime.DispatchTarget{{
			ID: settings.peerTargetID, Endpoint: settings.peerEndpoint,
			Security: peer.SecurityConfig{Identity: settings.peerIdentity, PeerIdentity: settings.peerExpected,
				Certificate: certificate, Roots: roots},
		}}); err != nil {
			return err
		}
	}
	configuration, err := application.NewConfiguration(runtime)
	if err != nil {
		return err
	}
	defer func() { _ = configuration.Stop() }()
	stage = "sdk-rest-server"
	server, _, err := pluginadapter.NewMutualTLSServer(configuration, pluginadapter.LifecycleOptions{
		Source: source, Identity: identity, Credentials: credentials, CorePeer: coreClientPeer,
		Revocation: revocation, ErrorLog: log.New(io.Discard, "", 0), LogOutput: os.Stdout,
	})
	if err != nil {
		return err
	}
	stage = "serve"
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.ListenAndServe(settings.restListen) }()
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
	return server.GracefulShutdown(shutdownContext)
}

func parseOptions(args []string) (options, error) {
	var value options
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&value.instanceID, "instance-id", "", "")
	flags.StringVar(&value.replicaID, "replica-id", "", "")
	flags.StringVar(&value.restListen, "rest-listen", "", "")
	flags.StringVar(&value.coreURL, "core-url", "", "")
	flags.StringVar(&value.coreServerName, "core-server-name", "", "")
	flags.StringVar(&value.coreName, "core-common-name", "", "")
	flags.StringVar(&value.coreURI, "core-uri", "", "")
	flags.StringVar(&value.coreClientName, "core-client-common-name", "", "")
	flags.StringVar(&value.coreClientURI, "core-client-uri", "", "")
	flags.StringVar(&value.caFile, "ca-file", "", "")
	flags.StringVar(&value.serverCert, "server-cert", "", "")
	flags.StringVar(&value.serverKey, "server-key", "", "")
	flags.StringVar(&value.clientCert, "client-cert", "", "")
	flags.StringVar(&value.clientKey, "client-key", "", "")
	flags.StringVar(&value.crlFile, "crl-file", "", "")
	flags.StringVar(&value.peerTargetID, "peer-target-id", "", "")
	flags.StringVar(&value.peerEndpoint, "peer-endpoint", "", "")
	flags.StringVar(&value.peerIdentity, "peer-identity", "", "")
	flags.StringVar(&value.peerExpected, "peer-expected-identity", "", "")
	flags.StringVar(&value.peerCAFile, "peer-ca-file", "", "")
	flags.StringVar(&value.peerCertFile, "peer-cert", "", "")
	flags.StringVar(&value.peerKeyFile, "peer-key", "", "")
	if flags.Parse(args) != nil || flags.NArg() != 0 ||
		value.instanceID == "" || value.replicaID == "" || value.restListen == "" ||
		value.coreURL == "" || value.coreServerName == "" || value.coreName == "" || value.coreClientName == "" ||
		!absoluteFiles(value.caFile, value.serverCert, value.serverKey, value.clientCert, value.clientKey, value.crlFile) {
		return options{}, errors.New("invalid Server bootstrap options")
	}
	if value.peerTargetID != "" {
		if value.peerEndpoint == "" || value.peerIdentity == "" || value.peerExpected == "" ||
			!absoluteFiles(value.peerCAFile, value.peerCertFile, value.peerKeyFile) {
			return options{}, errors.New("invalid Server peer options")
		}
	} else if value.peerEndpoint != "" || value.peerIdentity != "" || value.peerExpected != "" ||
		value.peerCAFile != "" || value.peerCertFile != "" || value.peerKeyFile != "" {
		return options{}, errors.New("incomplete Server peer options")
	}
	return value, nil
}

func absoluteFiles(paths ...string) bool {
	for _, path := range paths {
		if path == "" || !filepath.IsAbs(path) {
			return false
		}
	}
	return true
}

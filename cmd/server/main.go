package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	sdkmodels "github.com/Liapoldus/service-sdk/v2/domain/models"
	servicesdk "github.com/Liapoldus/service-sdk/v2/infrastructure"
	settingsapp "liapoldus.local/server-service/internal/application/settings"
	siteapp "liapoldus.local/server-service/internal/application/site"
	caddyruntime "liapoldus.local/server-service/internal/infrastructure/caddy"
	"liapoldus.local/server-service/internal/infrastructure/peercaller"
	"liapoldus.local/server-service/internal/infrastructure/site"
	publicapi "liapoldus.local/server-service/internal/presentation/publicapi"
	serviceadapter "liapoldus.local/server-service/internal/presentation/restservice"
)

type options struct {
	instanceID     string
	replicaID      string
	restListen     string
	restAdvertise  string
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
	peerIdentity   string
	peerCAFile     string
	peerCertFile   string
	peerKeyFile    string
	publicListen   string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		// Keep the process boundary diagnosable. The message is produced by
		// typed SDK/Core errors and never contains credentials or payloads.
		_, _ = fmt.Fprintf(os.Stderr, "Server bootstrap error: %v\n", err)
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
	contract, err := servicesdk.LoadHTTPContract()
	if err != nil {
		return err
	}
	stage = "credentials"
	credentials, err := servicesdk.NewFileCredentialsProvider(contract, servicesdk.CredentialsMaterial{
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
	stage = "replica-identity"
	replicaContract, err := servicesdk.LoadReplicaLifecycleContract()
	if err != nil {
		return err
	}
	clientCertificate, ok := material.ClientCertificate()
	if !ok || clientCertificate.Leaf == nil {
		return errors.New("SDK Core client certificate identity is unavailable")
	}
	placementID := os.Getenv("LIAPOLDUS_PLACEMENT_ID")
	if placementID == "" {
		placementID = "local-" + settings.replicaID
	}
	replicaIdentity, err := servicesdk.ReplicaIdentityFromClientCertificate(replicaContract, clientCertificate.Leaf, settings.instanceID, settings.replicaID, placementID)
	if err != nil {
		return fmt.Errorf("read SDK replica identity from client certificate: %w", err)
	}
	stage = "revocation"
	revocation, err := servicesdk.NewRevocation(servicesdk.RevocationConfiguration{
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
	controlClient, err := servicesdk.NewMutualTLSClient(contract, credentials, servicesdk.MutualTLSClientConfig{
		Peer: corePeer, ServerName: settings.coreServerName, Revocation: revocation,
	})
	if err != nil {
		return err
	}
	defer controlClient.CloseIdleConnections()
	var publicServer *http.Server
	stage = "config-source"
	source, err := servicesdk.NewCoreConfigurationSource(contract, settings.coreURL, controlClient)
	if err != nil {
		return err
	}
	stage = "secret-broker"
	secretBroker, err := servicesdk.NewCoreSecretBroker(contract, settings.coreURL, controlClient, 1024)
	if err != nil {
		return err
	}
	stage = "product-runtime"
	runtime := caddyruntime.New()
	stage = "peer-directory"
	peerCertificate, err := tls.LoadX509KeyPair(settings.peerCertFile, settings.peerKeyFile)
	if err != nil {
		return err
	}
	peerRootsPEM, err := os.ReadFile(settings.peerCAFile)
	if err != nil {
		return err
	}
	peerRoots := x509.NewCertPool()
	if !peerRoots.AppendCertsFromPEM(peerRootsPEM) {
		return errors.New("invalid peer trust roots")
	}
	peerContract, err := servicesdk.LoadPeerDirectoryPollContract()
	if err != nil {
		return err
	}
	// The data plane resolves every peer through Core's mTLS-issued directory.
	// Caddy settings carry only opaque target IDs and methods, never endpoints.
	directory, err := servicesdk.NewPeerDirectoryClient(peerContract, settings.coreURL, controlClient, replicaIdentity)
	if err != nil {
		return err
	}
	peerLogger, err := servicesdk.NewJSONLogger(contract, os.Stderr)
	if err != nil {
		return err
	}
	caller, err := peercaller.New(directory, peerContract, replicaIdentity, settings.peerIdentity, peerCertificate, peerRoots, peerLogger)
	if err != nil {
		return err
	}
	if err := runtime.SetDispatchCaller(caller); err != nil {
		return err
	}
	configuration, err := settingsapp.NewConfiguration(runtime)
	if err != nil {
		return err
	}
	defer func() { _ = configuration.Stop() }()
	stage = "site-release-store"
	releaseStore, err := site.NewReleaseStore(caddyruntime.SiteDataRoot())
	if err != nil {
		return err
	}
	publisher, err := siteapp.NewSitePublisher(releaseStore)
	if err != nil {
		return err
	}
	if err := publisher.ProcessPending(context.Background()); err != nil {
		return err
	}
	if err := caddyruntime.SetSiteDocumentReader(publisher); err != nil {
		return err
	}
	stage = "sdk-rest-server"
	server, lifecycle, collector, err := serviceadapter.NewMutualTLSServer(configuration, publisher, serviceadapter.LifecycleOptions{
		Source: source, Broker: secretBroker, Identity: identity, Credentials: credentials, CorePeer: coreClientPeer,
		Revocation: revocation, ErrorLog: log.New(io.Discard, "", 0), LogOutput: os.Stdout,
	})
	if err != nil {
		return err
	}
	if settings.publicListen != "" {
		observability, err := serviceadapter.NewPublicObservabilityHandler(contract, collector)
		if err != nil {
			return err
		}
		publicServer = &http.Server{Addr: settings.publicListen, Handler: publicapi.Handler(caller, observability), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second}
		go func() { _ = publicServer.ListenAndServe() }()
		defer func() { _ = publicServer.Close() }()
	}
	replicaClient, err := servicesdk.NewReplicaLifecycleClient(replicaContract, settings.coreURL, controlClient, replicaIdentity)
	if err != nil {
		return err
	}
	stage = "serve"
	workerContext, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	workerResult := make(chan error, 1)
	go func() { workerResult <- publisher.RunWorker(workerContext) }()
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.ListenAndServe(settings.restListen) }()
	serveContext, stopServe := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopServe()
	peerEndpoints := make([]sdkmodels.ReplicaPeerEndpoint, 0, 1)
	registration := sdkmodels.ReplicaRegistrationRequest{
		ContractVersion:     replicaContract.ContractVersion,
		Identity:            replicaIdentity,
		RestEndpoint:        settings.restAdvertise,
		PeerEndpoints:       peerEndpoints,
		Release:             sdkmodels.ReplicaRelease{Version: serviceadapter.ServiceVersion, SHA256: executableDigest()},
		AdvertisedContracts: []sdkmodels.ContractVersion{},
		AcceptedContracts:   []sdkmodels.ContractRange{},
		AppliedGeneration:   lifecycle.Readiness().Generation,
		Ready:               lifecycle.Readiness().Ready,
	}
	leaseResult := make(chan error, 1)
	go func() {
		leaseResult <- servicesdk.MaintainReplicaLease(serveContext, servicesdk.ReplicaLeaseMaintenance{
			Client: replicaClient, Registration: registration, Readiness: lifecycle.Readiness,
			RenewInterval: time.Duration(replicaContract.Lease.RenewIntervalSeconds) * time.Second,
		})
	}()
	select {
	case <-serveContext.Done():
	case workerErr := <-workerResult:
		if workerErr != nil {
			return workerErr
		}
	case err := <-serveResult:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case err := <-leaseResult:
		if err != nil {
			return err
		}
	}
	stopWorker()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = replicaClient.Deregister(shutdownContext, sdkmodels.ReplicaDeregistrationRequest{ContractVersion: replicaContract.ContractVersion, Identity: registration.Identity})
	return server.GracefulShutdown(shutdownContext)
}

func executableDigest() string {
	path, err := os.Executable()
	if err != nil {
		return "server-development"
	}
	file, err := os.Open(path)
	if err != nil {
		return "server-development"
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "server-development"
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func parseOptions(args []string) (options, error) {
	var value options
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&value.instanceID, "instance-id", "", "")
	flags.StringVar(&value.replicaID, "replica-id", "", "")
	flags.StringVar(&value.restListen, "rest-listen", "", "")
	flags.StringVar(&value.restAdvertise, "rest-advertise-url", "", "")
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
	flags.StringVar(&value.peerIdentity, "peer-identity", "", "")
	flags.StringVar(&value.peerCAFile, "peer-ca-file", "", "")
	flags.StringVar(&value.peerCertFile, "peer-cert", "", "")
	flags.StringVar(&value.peerKeyFile, "peer-key", "", "")
	flags.StringVar(&value.publicListen, "public-listen", "", "")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return options{}, errors.New("invalid Server bootstrap options")
	}
	value.instanceID = envDefault(value.instanceID, "LIAPOLDUS_INSTANCE_ID")
	value.replicaID = envDefault(value.replicaID, "LIAPOLDUS_REPLICA_ID")
	value.restListen = envDefault(value.restListen, "LIAPOLDUS_REST_LISTEN")
	value.restAdvertise = envDefault(value.restAdvertise, "LIAPOLDUS_REST_ADVERTISE_URL")
	value.coreURL = envDefault(value.coreURL, "LIAPOLDUS_CORE_URL")
	value.coreServerName = envDefault(value.coreServerName, "LIAPOLDUS_CORE_SERVER_NAME")
	value.coreName = envDefault(value.coreName, "LIAPOLDUS_CORE_COMMON_NAME")
	value.coreURI = envDefault(value.coreURI, "LIAPOLDUS_CORE_URI")
	value.coreClientName = envDefault(value.coreClientName, "LIAPOLDUS_CORE_CLIENT_COMMON_NAME")
	value.coreClientURI = envDefault(value.coreClientURI, "LIAPOLDUS_CORE_CLIENT_URI")
	value.caFile = envDefault(value.caFile, "LIAPOLDUS_CA_FILE")
	value.serverCert = envDefault(value.serverCert, "LIAPOLDUS_SERVER_CERT")
	value.serverKey = envDefault(value.serverKey, "LIAPOLDUS_SERVER_KEY")
	value.clientCert = envDefault(value.clientCert, "LIAPOLDUS_CLIENT_CERT")
	value.clientKey = envDefault(value.clientKey, "LIAPOLDUS_CLIENT_KEY")
	value.crlFile = envDefault(value.crlFile, "LIAPOLDUS_CRL_FILE")
	value.peerIdentity = envDefault(value.peerIdentity, "LIAPOLDUS_PEER_IDENTITY")
	value.peerCAFile = envDefault(value.peerCAFile, "LIAPOLDUS_PEER_CA_FILE")
	value.peerCertFile = envDefault(value.peerCertFile, "LIAPOLDUS_PEER_CERT")
	value.peerKeyFile = envDefault(value.peerKeyFile, "LIAPOLDUS_PEER_KEY")
	value.publicListen = envDefault(value.publicListen, "LIAPOLDUS_PUBLIC_LISTEN")
	if value.instanceID == "" || value.replicaID == "" || value.restListen == "" || value.restAdvertise == "" || value.coreURL == "" || value.coreServerName == "" || value.coreName == "" || value.coreClientName == "" || !absoluteFiles(value.caFile, value.serverCert, value.serverKey, value.clientCert, value.clientKey, value.crlFile) {
		return options{}, errors.New("invalid Server bootstrap options")
	}
	if value.peerIdentity == "" || !absoluteFiles(value.peerCAFile, value.peerCertFile, value.peerKeyFile) {
		return options{}, errors.New("incomplete Server peer options")
	}
	return value, nil
}

func envDefault(value, key string) string {
	if value != "" {
		return value
	}
	return os.Getenv(key)
}

func absoluteFiles(paths ...string) bool {
	for _, path := range paths {
		if path == "" || !filepath.IsAbs(path) {
			return false
		}
	}
	return true
}

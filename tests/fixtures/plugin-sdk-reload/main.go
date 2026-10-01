package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkinfra "github.com/Liapoldus/plugin-sdk/infrastructure"
	"liapoldus.local/server-plugin/contracts"
	"liapoldus.local/server-plugin/internal/application"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	"liapoldus.local/server-plugin/internal/presentation/restplugin"
)

type input struct {
	Port int `json:"port"`
}

type output struct {
	Acknowledgement          map[string]any `json:"acknowledgement"`
	ExpectedDigest           string         `json:"expectedDigest"`
	ReadyAfterApply          map[string]any `json:"readyAfterApply"`
	Registration             map[string]any `json:"registration"`
	ManifestName             string         `json:"manifestName"`
	ConfigurationSchemaValid bool           `json:"configurationSchemaValid"`
	MetricsHasReadinessGauge bool           `json:"metricsHasReadinessGauge"`
	FirstResponse            map[string]any `json:"firstResponse"`
	RejectedCandidate        bool           `json:"rejectedCandidate"`
	CandidateFailureStage    string         `json:"candidateFailureStage"`
	RevisionAfterReject      string         `json:"revisionAfterReject"`
	ReadyAfterReject         map[string]any `json:"readyAfterReject"`
	ResponseAfterReject      map[string]any `json:"responseAfterReject"`
}

type identities struct {
	roots        *x509.CertPool
	root         *x509.Certificate
	rootPEM      []byte
	rootKey      *ecdsa.PrivateKey
	coreServer   tlsCertificate
	pluginServer tlsCertificate
	pluginClient tlsCertificate
	coreClient   tlsCertificate
}

type tlsCertificate struct {
	certificate    tls.Certificate
	leaf           *x509.Certificate
	certificatePEM []byte
	privateKeyPEM  []byte
}

func main() {
	var request input
	check(json.NewDecoder(os.Stdin).Decode(&request))
	if request.Port < 1 || request.Port > 65535 {
		fail("invalid fixture port")
	}
	contract, err := sdkinfra.LoadHTTPContract()
	check(err)
	ids, err := createIdentities()
	check(err)
	storageDirectory, err := os.MkdirTemp("", "server-")
	check(err)
	defer func() { _ = os.RemoveAll(storageDirectory) }()
	check(os.Setenv("XDG_DATA_HOME", storageDirectory))
	runtime := caddyruntime.New()
	activationProbe := &activationProbe{runtime: runtime}
	configuration, err := application.NewConfiguration(activationProbe)
	check(err)
	defer func() { _ = configuration.Stop() }()
	siteRoot, err := caddyruntime.SiteRoot("frontend")
	check(err)
	check(os.MkdirAll(siteRoot, 0o700))
	check(os.WriteFile(filepath.Join(siteRoot, "index.html"), []byte("sdk-caddy-active"), 0o600))

	activeSettings := settings(request.Port)
	activeBytes := marshal(activeSettings)
	activeDigest := digest(activeBytes)

	// Keep the candidate address occupied for the full Reload call. Its settings
	// remain schema-valid, but Caddy cannot bind them during runtime activation.
	candidateListener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	defer func() { _ = candidateListener.Close() }()
	candidatePort := candidateListener.Addr().(*net.TCPAddr).Port
	if candidatePort == request.Port {
		check(candidateListener.Close())
		candidateListener, err = net.Listen("tcp", "127.0.0.1:0")
		check(err)
		candidatePort = candidateListener.Addr().(*net.TCPAddr).Port
	}
	failedBytes := marshal(settings(candidatePort))
	check(contracts.ValidateSettings(failedBytes))
	failedDigest := digest(failedBytes)

	coreListener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	revocation := newRevocation(ids)
	coreMux := http.NewServeMux()
	coreMux.HandleFunc(contract.Core.ConfigPull.Method+" "+contract.Core.ConfigPull.PathTemplate, func(writer http.ResponseWriter, request *http.Request) {
		generation := request.PathValue("generation")
		contents, digestValue := activeBytes, activeDigest
		if generation == "generation-2" {
			contents, digestValue = failedBytes, failedDigest
		} else if generation != "generation-1" {
			http.NotFound(writer, request)
			return
		}
		headers := contract.Core.ConfigPull.ResponseHeaders
		writer.Header().Set("content-type", contract.Core.ConfigPull.ResponseMediaType)
		writer.Header().Set(headers["generation"], generation)
		writer.Header().Set(headers["schemaVersion"], "1")
		writer.Header().Set(headers["sha256"], digestValue)
		writer.Header().Set(headers["generationState"], "active")
		_, _ = writer.Write(contents)
	})
	coreProvider := newCredentialsProvider(contract, ids, ids.coreServer, ids.coreClient)
	coreServer, err := sdkinfra.NewMutualTLSServer(contract, sdkinfra.MutualTLSServerConfig{
		Handler: coreMux, Provider: coreProvider,
		Peer:       sdkmodels.PeerIdentity{CommonName: ids.pluginClient.leaf.Subject.CommonName},
		Revocation: revocation,
	})
	check(err)
	go func() { _ = coreServer.Serve(coreListener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = coreServer.GracefulShutdown(shutdown)
	}()

	pluginProvider := newCredentialsProvider(contract, ids, ids.pluginServer, ids.pluginClient)
	pluginCoreTLS, err := sdkinfra.NewMutualTLSClient(contract, pluginProvider, sdkinfra.MutualTLSClientConfig{
		Peer:       sdkmodels.PeerIdentity{CommonName: ids.coreServer.leaf.Subject.CommonName},
		ServerName: "localhost", Revocation: revocation,
	})
	check(err)
	coreSource, err := sdkinfra.NewCoreConfigurationSource(contract, "https://"+coreListener.Addr().String(), pluginCoreTLS)
	check(err)
	pluginListener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	identity, err := sdkmodels.NewReplicaIdentity("server", "replica-1")
	check(err)
	pluginServer, _, err := restplugin.NewMutualTLSServer(configuration, restplugin.LifecycleOptions{
		Source: coreSource, Identity: identity, Credentials: pluginProvider,
		CorePeer:   sdkmodels.PeerIdentity{CommonName: ids.coreClient.leaf.Subject.CommonName},
		Revocation: revocation, LogOutput: io.Discard,
	})
	check(err)
	go func() { _ = pluginServer.Serve(pluginListener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = pluginServer.GracefulShutdown(shutdown)
	}()

	corePluginTLS, err := sdkinfra.NewMutualTLSClient(contract, coreProvider, sdkinfra.MutualTLSClientConfig{
		Peer:       sdkmodels.PeerIdentity{CommonName: ids.pluginServer.leaf.Subject.CommonName},
		ServerName: "localhost", Revocation: revocation,
	})
	check(err)
	pluginClient, err := sdkinfra.NewPluginClient(contract, "https://"+pluginListener.Addr().String(), corePluginTLS,
		sdkmodels.PeerIdentity{CommonName: ids.coreClient.leaf.Subject.CommonName})
	check(err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	firstAck, err := pluginClient.Reload(ctx, sdkmodels.Reload{Generation: "generation-1", SHA256: activeDigest, SchemaVersion: "1"})
	check(err)
	ready, err := pluginClient.Readiness(ctx)
	check(err)
	registration, err := pluginClient.Identity(ctx)
	check(err)
	manifestBytes, err := pluginClient.Manifest(ctx)
	check(err)
	var manifest struct {
		Name string `json:"name"`
	}
	check(json.Unmarshal(manifestBytes, &manifest))
	schemaBytes, err := pluginClient.ConfigurationSchema(ctx)
	check(err)
	metrics, err := pluginClient.Metrics(ctx)
	check(err)
	firstResponse := requestDataPlane(request.Port)
	_, rejectedErr := pluginClient.Reload(ctx, sdkmodels.Reload{Generation: "generation-2", SHA256: failedDigest, SchemaVersion: "1"})
	readyAfterReject, readyErr := pluginClient.Readiness(ctx)
	check(readyErr)
	responseAfterReject := requestDataPlane(request.Port)
	failureStage := ""
	if activationProbe.failed.Load() {
		failureStage = "runtime-activation"
	}
	result := output{
		Acknowledgement: map[string]any{"generation": firstAck.Generation, "sha256": firstAck.SHA256, "applied": firstAck.Applied},
		ExpectedDigest:  activeDigest,
		ReadyAfterApply: map[string]any{"ready": ready.Ready, "generation": ready.Generation},
		Registration:    map[string]any{"instanceId": registration.InstanceID, "replicaId": registration.ReplicaID, "ready": registration.Ready, "appliedGeneration": registration.AppliedGeneration},
		ManifestName:    manifest.Name, ConfigurationSchemaValid: json.Valid(schemaBytes),
		MetricsHasReadinessGauge: strings.Contains(metrics, contract.Plugin.Responses.Metrics.ReadyMetricName),
		FirstResponse:            firstResponse, RejectedCandidate: rejectedErr != nil,
		CandidateFailureStage: failureStage,
		RevisionAfterReject:   configuration.Revision(),
		ReadyAfterReject:      map[string]any{"ready": readyAfterReject.Ready, "generation": readyAfterReject.Generation},
		ResponseAfterReject:   responseAfterReject,
	}
	check(json.NewEncoder(os.Stdout).Encode(result))
}

type activationProbe struct {
	runtime application.Runtime
	failed  atomic.Bool
}

func (probe *activationProbe) Validate(configuration []byte) error {
	return probe.runtime.Validate(configuration)
}

func (probe *activationProbe) Activate(configuration []byte) error {
	if err := probe.runtime.Activate(configuration); err != nil {
		probe.failed.Store(true)
		return err
	}
	return nil
}

func (probe *activationProbe) Stop() error { return probe.runtime.Stop() }

func settings(port int) map[string]any {
	return map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"listeners": []any{map[string]any{
				"id": "web", "kind": "http", "address": fmt.Sprintf("127.0.0.1:%d", port),
				"hostnames": []string{}, "protocols": []string{"http1"}, "tls": map[string]any{"mode": "disabled"},
			}},
			"routes": []any{map[string]any{
				"id": "site", "listenerId": "web", "handler": map[string]any{"type": "static", "siteId": "frontend"},
			}},
		},
	}
}

func requestDataPlane(port int) map[string]any {
	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr == nil {
				return map[string]any{"status": response.StatusCode, "body": string(body)}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	fail("Caddy data plane did not respond")
	return nil
}

func createIdentities() (identities, error) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return identities{}, err
	}
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-only root"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		SubjectKeyId: []byte{1, 2, 3, 4},
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return identities{}, err
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return identities{}, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
	issue := func(serial int64, name string) (tlsCertificate, error) {
		key, keyErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if keyErr != nil {
			return tlsCertificate{}, keyErr
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name},
			DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
			NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
			KeyUsage:    x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		}
		der, issueErr := x509.CreateCertificate(rand.Reader, template, root, &key.PublicKey, rootKey)
		if issueErr != nil {
			return tlsCertificate{}, issueErr
		}
		certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyDER, issueErr := x509.MarshalPKCS8PrivateKey(key)
		if issueErr != nil {
			return tlsCertificate{}, issueErr
		}
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		pair, issueErr := tls.X509KeyPair(certificatePEM, keyPEM)
		if issueErr != nil {
			return tlsCertificate{}, issueErr
		}
		pair.Leaf, issueErr = x509.ParseCertificate(der)
		return tlsCertificate{certificate: pair, leaf: pair.Leaf, certificatePEM: certificatePEM, privateKeyPEM: keyPEM}, issueErr
	}
	coreServer, err := issue(2, "Core test server")
	if err != nil {
		return identities{}, err
	}
	pluginServer, err := issue(3, "plugin test server")
	if err != nil {
		return identities{}, err
	}
	pluginClient, err := issue(4, "plugin test client")
	if err != nil {
		return identities{}, err
	}
	coreClient, err := issue(5, "Core test client")
	if err != nil {
		return identities{}, err
	}
	return identities{roots: pool, root: root, rootPEM: rootPEM, rootKey: rootKey, coreServer: coreServer, pluginServer: pluginServer, pluginClient: pluginClient, coreClient: coreClient}, nil
}

func newCredentialsProvider(contract sdkinfra.HTTPContract, ids identities, server, client tlsCertificate) *sdkinfra.StaticCredentialsProvider {
	credentials, err := sdkinfra.LoadCredentials(contract, sdkinfra.CredentialsMaterial{
		CABundle:             ids.rootPEM,
		ServerCertificatePEM: server.certificatePEM,
		ServerKeyPEM:         server.privateKeyPEM,
		ClientCertificatePEM: client.certificatePEM,
		ClientKeyPEM:         client.privateKeyPEM,
	})
	check(err)
	provider, err := sdkinfra.NewStaticCredentialsProvider(credentials)
	check(err)
	return provider
}

func newRevocation(ids identities) *sdkinfra.Revocation {
	now := time.Now()
	contents, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(1), ThisUpdate: now.Add(-time.Minute), NextUpdate: now.Add(time.Hour),
	}, ids.root, ids.rootKey)
	check(err)
	revocation, err := sdkinfra.NewRevocation(sdkinfra.RevocationConfiguration{
		Authorities: []*x509.Certificate{ids.root}, Bundles: [][]byte{contents},
	})
	check(err)
	return revocation
}

func marshal(value any) []byte {
	contents, err := json.Marshal(value)
	check(err)
	return contents
}

func digest(contents []byte) string {
	value := sha256.Sum256(contents)
	return hex.EncodeToString(value[:])
}

func check(err error) {
	if err != nil {
		fail(err.Error())
	}
}

func fail(message string) { panic(strings.TrimSpace(message)) }

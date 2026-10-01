package main

import (
	"bytes"
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
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkinfra "github.com/Liapoldus/plugin-sdk/infrastructure"
	"liapoldus.local/server-plugin/contracts"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
)

const generation = "generation-rest-1"

type identity struct {
	certificate    tls.Certificate
	certificatePEM []byte
	privateKeyPEM  []byte
}

type identities struct {
	root       *x509.Certificate
	rootKey    *ecdsa.PrivateKey
	rootPEM    []byte
	coreServer identity
	coreClient identity
	replica    identity
}

type result struct {
	Acknowledgement map[string]any `json:"acknowledgement"`
	Readiness       map[string]any `json:"readiness"`
	Response        map[string]any `json:"response"`
	ChildExitCode   int            `json:"childExitCode"`
}

func main() {
	if err := run(); err != nil {
		fail(err)
	}
}

func run() error {
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		return err
	}
	issued, err := createIdentities()
	if err != nil {
		return err
	}
	rootDirectory, err := os.MkdirTemp("", "server-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(rootDirectory) }()
	if err := os.Setenv("XDG_DATA_HOME", rootDirectory); err != nil {
		return err
	}
	siteRoot, err := caddyruntime.SiteRoot("rest-site")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(siteRoot, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(siteRoot, "index.html"), []byte("server-"), 0o600); err != nil {
		return err
	}
	controlListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	controlPort := controlListener.Addr().(*net.TCPAddr).Port
	if err := controlListener.Close(); err != nil {
		return err
	}
	trafficListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	trafficPort := trafficListener.Addr().(*net.TCPAddr).Port
	if err := trafficListener.Close(); err != nil {
		return err
	}
	configBytes, err := json.Marshal(settings(trafficPort))
	if err != nil || contracts.ValidateSettings(configBytes) != nil {
		return errors.New("test Caddy settings are invalid")
	}
	digest := sha256.Sum256(configBytes)
	digestHex := hex.EncodeToString(digest[:])
	coreListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	coreProvider, err := newCredentialsProvider(contract, issued.rootPEM, issued.coreServer, issued.coreClient)
	if err != nil {
		_ = coreListener.Close()
		return err
	}
	revocation, err := newRevocation(issued)
	if err != nil {
		_ = coreListener.Close()
		return err
	}
	coreMux := http.NewServeMux()
	coreMux.HandleFunc(contract.Core.ConfigPull.Method+" "+contract.Core.ConfigPull.PathTemplate, func(writer http.ResponseWriter, request *http.Request) {
		if request.PathValue("generation") != generation {
			http.NotFound(writer, request)
			return
		}
		headers := contract.Core.ConfigPull.ResponseHeaders
		writer.Header().Set("content-type", contract.Core.ConfigPull.ResponseMediaType)
		writer.Header().Set(headers["generation"], generation)
		writer.Header().Set(headers["schemaVersion"], "1")
		writer.Header().Set(headers["sha256"], digestHex)
		writer.Header().Set(headers["generationState"], "active")
		_, _ = writer.Write(configBytes)
	})
	coreServer, err := sdkinfra.NewMutualTLSServer(contract, sdkinfra.MutualTLSServerConfig{
		Handler: coreMux, Provider: coreProvider,
		Peer: sdkmodels.PeerIdentity{CommonName: issued.replica.certificate.Leaf.Subject.CommonName}, Revocation: revocation,
	})
	if err != nil {
		_ = coreListener.Close()
		return err
	}
	go func() { _ = coreServer.Serve(coreListener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = coreServer.GracefulShutdown(shutdown)
	}()
	writePEM := func(name string, content []byte) (string, error) {
		path := filepath.Join(rootDirectory, name)
		return path, os.WriteFile(path, content, 0o600)
	}
	caPath, err := writePEM("ca.pem", issued.rootPEM)
	if err != nil {
		return err
	}
	certPath, err := writePEM("replica.pem", issued.replica.certificatePEM)
	if err != nil {
		return err
	}
	keyPath, err := writePEM("replica-key.pem", issued.replica.privateKeyPEM)
	if err != nil {
		return err
	}
	crlDER, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(1), ThisUpdate: time.Now().UTC().Add(-time.Minute), NextUpdate: time.Now().UTC().Add(time.Hour),
	}, issued.root, issued.rootKey)
	if err != nil {
		return err
	}
	crlPath, err := writePEM("crl.pem", pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: crlDER}))
	if err != nil {
		return err
	}
	binaryPath := filepath.Join(rootDirectory, "server-")
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/server")
	build.Dir = repositoryRoot()
	build.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.26.0")
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		return fmt.Errorf("build Caddy process: %w: %s", buildErr, string(output))
	}
	child := exec.Command(binaryPath,
		"--instance-id=server-", "--replica-id=server-",
		fmt.Sprintf("--rest-listen=127.0.0.1:%d", controlPort),
		"--core-url=https://"+coreListener.Addr().String(),
		"--core-server-name=localhost", "--core-common-name=test Core server",
		"--core-client-common-name=test Core client",
		"--ca-file="+caPath, "--server-cert="+certPath, "--server-key="+keyPath,
		"--client-cert="+certPath, "--client-key="+keyPath, "--crl-file="+crlPath)
	child.Dir = repositoryRoot()
	child.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.26.0")
	var childLogs bytes.Buffer
	child.Stdout = &childLogs
	child.Stderr = &childLogs
	if err := child.Start(); err != nil {
		return err
	}
	pluginProvider, err := newCredentialsProvider(contract, issued.rootPEM, issued.coreClient, issued.coreClient)
	if err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		return err
	}
	pluginHTTP, err := sdkinfra.NewMutualTLSClient(contract, pluginProvider, sdkinfra.MutualTLSClientConfig{
		Peer: sdkmodels.PeerIdentity{CommonName: issued.replica.certificate.Leaf.Subject.CommonName}, ServerName: "localhost", Revocation: revocation,
	})
	if err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		return err
	}
	pluginClient, err := sdkinfra.NewPluginClient(contract, fmt.Sprintf("https://127.0.0.1:%d", controlPort), pluginHTTP,
		sdkmodels.PeerIdentity{CommonName: issued.replica.certificate.Leaf.Subject.CommonName})
	if err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	deadline := time.Now().Add(15 * time.Second)
	var healthErr error
	for time.Now().Before(deadline) {
		healthErr = pluginClient.Health(ctx)
		if healthErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if healthErr != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		return fmt.Errorf("plugin health unavailable: %w; child output: %s", healthErr, childLogs.String())
	}
	ack, err := pluginClient.Reload(ctx, sdkmodels.Reload{Generation: generation, SHA256: digestHex, SchemaVersion: "1"})
	if err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		return fmt.Errorf("REST Reload failed: %w; child output: %s", err, childLogs.String())
	}
	ready, err := pluginClient.Readiness(ctx)
	if err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		return err
	}
	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", trafficPort))
	if err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		return err
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || string(body) != "server-" {
		_ = child.Process.Kill()
		_ = child.Wait()
		return errors.New("Caddy data-plane response did not match active config")
	}
	if err := child.Process.Signal(syscall.SIGTERM); err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		return err
	}
	exit := make(chan error, 1)
	go func() { exit <- child.Wait() }()
	select {
	case err := <-exit:
		if err != nil {
			return fmt.Errorf("Caddy process exited unsuccessfully: %w: %s", err, childLogs.String())
		}
	case <-ctx.Done():
		_ = child.Process.Kill()
		return ctx.Err()
	}
	return json.NewEncoder(os.Stdout).Encode(result{
		Acknowledgement: map[string]any{"generation": ack.Generation, "applied": ack.Applied},
		Readiness:       map[string]any{"ready": ready.Ready, "generation": ready.Generation},
		Response:        map[string]any{"status": response.StatusCode, "body": string(body)},
		ChildExitCode:   0,
	})
}

func repositoryRoot() string {
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile()), "../../.."))
}

func sourceFile() string {
	_, file, _, _ := runtime.Caller(0)
	return file
}

func settings(port int) map[string]any {
	return map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"listeners": []any{map[string]any{
				"id": "web", "kind": "http", "address": fmt.Sprintf("127.0.0.1:%d", port),
				"hostnames": []string{}, "protocols": []string{"http1"}, "tls": map[string]any{"mode": "disabled"},
			}},
			"routes": []any{map[string]any{
				"id": "site", "listenerId": "web", "handler": map[string]any{"type": "static", "siteId": "rest-site"},
			}},
		},
	}
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
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return identities{}, err
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return identities{}, err
	}
	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
	issue := func(serial int64, name string) (identity, error) {
		key, keyErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if keyErr != nil {
			return identity{}, keyErr
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
			return identity{}, issueErr
		}
		certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyDER, issueErr := x509.MarshalPKCS8PrivateKey(key)
		if issueErr != nil {
			return identity{}, issueErr
		}
		privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		pair, issueErr := tls.X509KeyPair(certificatePEM, privateKeyPEM)
		if issueErr != nil {
			return identity{}, issueErr
		}
		return identity{certificate: pair, certificatePEM: certificatePEM, privateKeyPEM: privateKeyPEM}, nil
	}
	coreServer, err := issue(2, "test Core server")
	if err != nil {
		return identities{}, err
	}
	coreClient, err := issue(3, "test Core client")
	if err != nil {
		return identities{}, err
	}
	replica, err := issue(4, "test Caddy replica")
	if err != nil {
		return identities{}, err
	}
	return identities{root: root, rootKey: rootKey, rootPEM: rootPEM, coreServer: coreServer, coreClient: coreClient, replica: replica}, nil
}

func newCredentialsProvider(contract sdkinfra.HTTPContract, roots []byte, server, client identity) (*sdkinfra.StaticCredentialsProvider, error) {
	credentials, err := sdkinfra.LoadCredentials(contract, sdkinfra.CredentialsMaterial{
		CABundle:             roots,
		ServerCertificatePEM: server.certificatePEM, ServerKeyPEM: server.privateKeyPEM,
		ClientCertificatePEM: client.certificatePEM, ClientKeyPEM: client.privateKeyPEM,
	})
	if err != nil {
		return nil, err
	}
	return sdkinfra.NewStaticCredentialsProvider(credentials)
}

func newRevocation(issued identities) (*sdkinfra.Revocation, error) {
	now := time.Now().UTC()
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(1), ThisUpdate: now.Add(-time.Minute), NextUpdate: now.Add(time.Hour),
	}, issued.root, issued.rootKey)
	if err != nil {
		return nil, err
	}
	credentials, err := sdkinfra.LoadCredentials(mustContract(), sdkinfra.CredentialsMaterial{
		CABundle: issued.rootPEM, ServerCertificatePEM: issued.replica.certificatePEM, ServerKeyPEM: issued.replica.privateKeyPEM,
	})
	if err != nil {
		return nil, err
	}
	return sdkinfra.NewRevocation(sdkinfra.RevocationConfiguration{
		Authorities: credentials.TrustAuthorities(), Bundles: [][]byte{der}, Policy: sdkinfra.RevocationFailClosed,
	})
}

func mustContract() sdkinfra.HTTPContract {
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		panic(err)
	}
	return contract
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

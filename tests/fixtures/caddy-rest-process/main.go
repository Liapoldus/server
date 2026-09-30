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

	"liapoldus.local/server-plugin/contracts"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	sdkmodels "liapoldus.local/plugin-sdk/domain/models"
	sdkinfra "liapoldus.local/plugin-sdk/infrastructure"
)

const generation = "generation-rest-1"

type identity struct {
	certificate    tls.Certificate
	certificatePEM []byte
	privateKeyPEM  []byte
}

type identities struct {
	root       *x509.CertPool
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
	listenerFile, err := controlListener.(*net.TCPListener).File()
	if err != nil {
		_ = controlListener.Close()
		return err
	}
	if err := controlListener.Close(); err != nil {
		_ = listenerFile.Close()
		return err
	}
	trafficListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = listenerFile.Close()
		return err
	}
	trafficPort := trafficListener.Addr().(*net.TCPAddr).Port
	if err := trafficListener.Close(); err != nil {
		_ = listenerFile.Close()
		return err
	}
	configBytes, err := json.Marshal(settings(trafficPort))
	if err != nil || contracts.ValidateSettings(configBytes) != nil {
		_ = listenerFile.Close()
		return errors.New("test Caddy settings are invalid")
	}
	digest := sha256.Sum256(configBytes)
	digestHex := hex.EncodeToString(digest[:])
	coreListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = listenerFile.Close()
		return err
	}
	coreTLSListener, err := sdkinfra.NewMutualTLSListener(coreListener, issued.coreServer.certificate, issued.root, contract.TransportSecurity.MinimumTLSVersion)
	if err != nil {
		_ = listenerFile.Close()
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
		_, _ = writer.Write(configBytes)
	})
	coreServer := &http.Server{Handler: coreMux}
	go func() { _ = coreServer.Serve(coreTLSListener) }()
	defer func() { _ = coreServer.Close() }()
	bootstrap, err := json.Marshal(map[string]string{
		"contractVersion":               "liapoldus.plugin-sdk.process-bootstrap.v1",
		"coreURL":                       "https://" + coreListener.Addr().String(),
		"instanceId":                    "server-",
		"replicaId":                     "server-",
		"replicaCertificatePEM":         string(issued.replica.certificatePEM),
		"replicaPrivateKeyPEM":          string(issued.replica.privateKeyPEM),
		"coreControlPlaneTrustRootsPEM": string(issued.rootPEM),
	})
	if err != nil {
		_ = listenerFile.Close()
		return err
	}
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		_ = listenerFile.Close()
		return err
	}
	if _, err = writePipe.Write(bootstrap); err != nil {
		_ = listenerFile.Close()
		_ = readPipe.Close()
		_ = writePipe.Close()
		return err
	}
	if err := writePipe.Close(); err != nil {
		_ = listenerFile.Close()
		_ = readPipe.Close()
		return err
	}
	binaryPath := filepath.Join(rootDirectory, "server-")
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/server")
	build.Dir = repositoryRoot()
	build.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.26.0")
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		_ = listenerFile.Close()
		_ = readPipe.Close()
		return fmt.Errorf("build Caddy process: %w: %s", buildErr, string(output))
	}
	child := exec.Command(binaryPath)
	child.Dir = repositoryRoot()
	child.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.26.0")
	child.ExtraFiles = []*os.File{listenerFile, readPipe}
	var childLogs bytes.Buffer
	child.Stdout = &childLogs
	child.Stderr = &childLogs
	if err := child.Start(); err != nil {
		_ = listenerFile.Close()
		_ = readPipe.Close()
		return err
	}
	_ = listenerFile.Close()
	_ = readPipe.Close()
	pluginHTTP, err := sdkinfra.NewMutualTLSHTTPClient(sdkinfra.MutualTLSClientConfiguration{
		Certificate: issued.coreClient.certificate, TrustedServerRoots: issued.root,
		ServerName: "localhost", MinimumTLSVersion: contract.TransportSecurity.MinimumTLSVersion,
	})
	if err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		return err
	}
	pluginClient, err := sdkinfra.NewPluginClient(fmt.Sprintf("https://127.0.0.1:%d", controlPort), pluginHTTP, contract)
	if err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if pluginClient.Health(ctx) == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	ack, err := pluginClient.Reload(ctx, sdkmodels.Reload{Generation: generation, SHA256: digestHex, SchemaVersion: "1"})
	if err != nil {
		diagnostic, diagnosticErr := diagnoseReload(ctx, controlPort, issued, contract, generation, digestHex)
		_ = child.Process.Kill()
		_ = child.Wait()
		return fmt.Errorf("REST Reload failed: %w; response %s (%v); child output: %s", err, diagnostic, diagnosticErr, childLogs.String())
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

func diagnoseReload(ctx context.Context, port int, issued identities, contract sdkinfra.HTTPContract, generation, digest string) (string, error) {
	body, err := json.Marshal(sdkmodels.Reload{Generation: generation, SHA256: digest, SchemaVersion: "1"})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, contract.Plugin.Endpoints.Reload.Method, fmt.Sprintf("https://127.0.0.1:%d%s", port, contract.Plugin.Endpoints.Reload.Path), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("content-type", "application/json")
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion:   contract.TransportSecurity.MinimumTLSVersion,
		Certificates: []tls.Certificate{issued.coreClient.certificate}, RootCAs: issued.root,
		ServerName: "localhost",
	}}}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	return fmt.Sprintf("status=%d body=%s", response.StatusCode, string(contents)), err
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
	pool := x509.NewCertPool()
	pool.AddCert(root)
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
	return identities{root: pool, rootPEM: rootPEM, coreServer: coreServer, coreClient: coreClient, replica: replica}, nil
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

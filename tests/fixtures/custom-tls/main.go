package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"liapoldus.local/server-plugin/internal/application"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	pluginadapter "liapoldus.local/server-plugin/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	pluginsdk "github.com/Liapoldus/pluginprotocol/presentation/sdk"
	caddycore "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/certmagic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	instanceID      = "server-"
	certificateRef  = "fixture-certificate"
	privateKeyRef   = "fixture-private-key"
	grantHandle     = "fixture-config-grant"
	grantPurpose    = "plugin-config-apply"
	customHostname  = "custom.example.test"
	invalidRevision = "revision-2"
	validRevision   = "revision-3"
	initialRevision = "revision-1"
)

type input struct {
	HTTPPort  int `json:"httpPort"`
	HTTPSPort int `json:"httpsPort"`
}

type fixtureOutput struct {
	InitialRevision          string `json:"initialRevision"`
	MissingGrantCode         string `json:"missingGrantCode"`
	WrongScopeCode           string `json:"wrongScopeCode"`
	InvalidPairCode          string `json:"invalidPairCode"`
	RevisionAfterInvalidPair string `json:"revisionAfterInvalidPair"`
	OldRevisionStatus        int    `json:"oldRevisionStatus"`
	OldRevisionBody          string `json:"oldRevisionBody"`
	ValidCode                string `json:"validCode"`
	ValidRevision            string `json:"validRevision"`
	TLS12                    result `json:"tls12"`
	TLS13                    result `json:"tls13"`
	WrongHostnameRejected    bool   `json:"wrongHostnameRejected"`
}

type result struct {
	NegotiatedProtocol string `json:"negotiatedProtocol"`
	Status             int    `json:"status"`
	Body               string `json:"body"`
}

type grantBroker struct {
	pluginv1.UnimplementedGrantBrokerServer
	secrets map[string][]byte
}

func main() {
	var request input
	check(json.NewDecoder(os.Stdin).Decode(&request))
	storageDirectory, err := os.MkdirTemp("", "liapoldus-caddy-custom-tls-")
	check(err)
	defer func() { _ = os.RemoveAll(storageDirectory) }()
	check(os.Setenv("XDG_DATA_HOME", storageDirectory))
	caddycore.DefaultStorage = &certmagic.FileStorage{Path: storageDirectory}

	certificatePEM, privateKeyPEM, wrongPrivateKeyPEM, rootPEM := makeCertificates()
	runtime := caddyruntime.New()
	siteRoot, err := caddyruntime.SiteRoot("custom-tls-fixture")
	check(err)
	check(os.MkdirAll(siteRoot, 0o700))
	sitePath := filepath.Join(siteRoot, "index.html")
	check(os.WriteFile(sitePath, []byte("old-active"), 0o600))
	configuration, err := application.NewConfiguration(runtime)
	check(err)
	defer func() { _ = configuration.Stop() }()
	service, err := pluginadapter.New(configuration, func() {})
	check(err)
	broker, err := pluginsdk.StartGrantBroker(&grantBroker{secrets: map[string][]byte{
		certificateRef: certificatePEM,
		privateKeyRef:  privateKeyPEM,
		"invalid-key":  wrongPrivateKeyPEM,
	}})
	check(err)
	defer broker.Stop()
	_, err = service.Bootstrap(context.Background(), &pluginv1.BootstrapRequest{
		InstanceId: instanceID, GrantBrokerEndpoint: broker.Endpoint(),
	})
	check(err)

	initialConfig := settings(request.HTTPPort, nil, "")
	initial, err := service.ConfigApply(context.Background(), &pluginv1.ConfigApplyRequest{
		Config: initialConfig, SettingsRevision: initialRevision,
	})
	check(err)
	customConfig := settings(request.HTTPPort, &request.HTTPSPort, "invalid-key")
	_, missingGrantErr := service.ConfigApply(context.Background(), &pluginv1.ConfigApplyRequest{
		Config: customConfig, SettingsRevision: invalidRevision,
	})
	wrongScopeRequest, err := customGrantRequest(customConfig, invalidRevision, certificateRef, "invalid-key")
	check(err)
	for _, grant := range wrongScopeRequest.Grants {
		grant.Scope = pluginv1.GrantScope_GRANT_SCOPE_CALL
	}
	_, wrongScopeErr := service.ConfigApply(context.Background(), wrongScopeRequest)
	wrongPair, err := customGrantRequest(customConfig, invalidRevision, certificateRef, "invalid-key")
	check(err)
	_, invalidPairErr := service.ConfigApply(context.Background(), wrongPair)
	revisionAfterInvalidPair := configuration.Revision()

	oldStatus, oldBody := getHTTP(request.HTTPPort)
	check(os.WriteFile(sitePath, []byte("custom-tls"), 0o600))
	validConfig := settings(request.HTTPPort, &request.HTTPSPort, "")
	validGrant, err := customGrantRequest(validConfig, validRevision, certificateRef, privateKeyRef)
	check(err)
	active, validErr := service.ConfigApply(context.Background(), validGrant)
	output := fixtureOutput{
		InitialRevision: initial.GetSettingsRevision(), MissingGrantCode: status.Code(missingGrantErr).String(),
		WrongScopeCode:  status.Code(wrongScopeErr).String(),
		InvalidPairCode: status.Code(invalidPairErr).String(), RevisionAfterInvalidPair: revisionAfterInvalidPair,
		OldRevisionStatus: oldStatus, OldRevisionBody: oldBody, ValidCode: status.Code(validErr).String(),
	}
	if validErr != nil {
		check(json.NewEncoder(os.Stdout).Encode(output))
		return
	}

	tls12 := requestTLS(request.HTTPSPort, rootPEM, customHostname, tls.VersionTLS12)
	tls13 := requestTLS(request.HTTPSPort, rootPEM, customHostname, tls.VersionTLS13)
	_, wrongHostnameErr := dialTLS(request.HTTPSPort, rootPEM, "wrong.example.test", tls.VersionTLS13)

	output.ValidRevision = active.GetSettingsRevision()
	output.TLS12 = tls12
	output.TLS13 = tls13
	output.WrongHostnameRejected = wrongHostnameErr != nil
	check(json.NewEncoder(os.Stdout).Encode(output))
}

func settings(httpPort int, httpsPort *int, invalidKeyRef string) []byte {
	listeners := []any{map[string]any{
		"id": "old-web", "kind": "http", "address": fmt.Sprintf("127.0.0.1:%d", httpPort),
		"hostnames": []string{}, "protocols": []string{"http1"}, "tls": map[string]any{"mode": "disabled"},
	}}
	if httpsPort != nil {
		keyReference := privateKeyRef
		if invalidKeyRef != "" {
			keyReference = invalidKeyRef
		}
		listeners = append(listeners, map[string]any{
			"id": "secure-web", "kind": "http", "address": fmt.Sprintf("127.0.0.1:%d", *httpsPort),
			"hostnames": []string{customHostname}, "protocols": []string{"http1"},
			"tls": map[string]any{"mode": "custom", "certificateRef": certificateRef, "privateKeyRef": keyReference},
		})
	}
	routes := []any{map[string]any{
		"id": "site", "listenerId": "old-web", "handler": map[string]any{"type": "static", "siteId": "custom-tls-fixture"},
	}}
	if httpsPort != nil {
		routes = append(routes, map[string]any{
			"id": "secure-site", "listenerId": "secure-web", "handler": map[string]any{"type": "static", "siteId": "custom-tls-fixture"},
		})
	}
	contents, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"listeners": listeners,
			"routes":    routes,
		},
	})
	check(err)
	return contents
}

func customGrantRequest(config []byte, revision, certRef, keyRef string) (*pluginv1.ConfigApplyRequest, error) {
	return &pluginv1.ConfigApplyRequest{
		Config: config, SettingsRevision: revision,
		Grants: []*pluginv1.ActiveGrant{
			{Handle: grantHandle, Purpose: grantPurpose, Scope: pluginv1.GrantScope_GRANT_SCOPE_CONFIG_APPLY, InstanceId: instanceID, SettingsRevision: revision, SecretReference: certRef},
			{Handle: grantHandle, Purpose: grantPurpose, Scope: pluginv1.GrantScope_GRANT_SCOPE_CONFIG_APPLY, InstanceId: instanceID, SettingsRevision: revision, SecretReference: keyRef},
		},
	}, nil
}

func (broker *grantBroker) RedeemGrant(_ context.Context, request *pluginv1.RedeemGrantRequest) (*pluginv1.RedeemGrantResponse, error) {
	secret, exists := broker.secrets[request.GetSecretReference()]
	if request.GetHandle() != grantHandle || request.GetPurpose() != grantPurpose || request.GetScope() != pluginv1.GrantScope_GRANT_SCOPE_CONFIG_APPLY ||
		request.GetInstanceId() != instanceID || request.GetSettingsRevision() == "" || request.GetCapability() != "" || request.GetDomain() != "" || !exists {
		return nil, status.Error(codes.PermissionDenied, "")
	}
	return &pluginv1.RedeemGrantResponse{Secret: append([]byte(nil), secret...)}, nil
}

func getHTTP(port int) (int, string) {
	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	check(err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	check(err)
	return response.StatusCode, string(body)
}

func requestTLS(port int, rootPEM []byte, hostname string, version uint16) result {
	connection, err := dialTLS(port, rootPEM, hostname, version)
	check(err)
	defer connection.Close()
	if _, err := fmt.Fprintf(connection, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", hostname); err != nil {
		panic(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: http.MethodGet})
	check(err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	check(err)
	return result{NegotiatedProtocol: connection.ConnectionState().NegotiatedProtocol, Status: response.StatusCode, Body: string(body)}
}

func dialTLS(port int, rootPEM []byte, hostname string, version uint16) (*tls.Conn, error) {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(rootPEM) {
		return nil, fmt.Errorf("fixture CA could not be loaded")
	}
	return tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{
		RootCAs: roots, ServerName: hostname, MinVersion: version, MaxVersion: version,
		NextProtos: []string{"h2", "http/1.1"},
	})
}

func makeCertificates() ([]byte, []byte, []byte, []byte) {
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)
	caSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	check(err)
	caTemplate := &x509.Certificate{SerialNumber: caSerial, Subject: pkix.Name{CommonName: "Caddy custom TLS fixture CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	check(err)
	ca, err := x509.ParseCertificate(caDER)
	check(err)

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)
	serverSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	check(err)
	serverTemplate := &x509.Certificate{SerialNumber: serverSerial, Subject: pkix.Name{CommonName: customHostname}, DNSNames: []string{customHostname},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(48 * time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, ca, &serverKey.PublicKey, caKey)
	check(err)
	serverPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	privateDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	check(err)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	wrongKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)
	wrongDER, err := x509.MarshalPKCS8PrivateKey(wrongKey)
	check(err)
	wrongPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: wrongDER})
	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	return serverPEM, privatePEM, wrongPEM, rootPEM
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

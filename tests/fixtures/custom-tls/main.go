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

	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkpresentation "github.com/Liapoldus/plugin-sdk/presentation"
	"liapoldus.local/server-plugin/contracts"
	settingsapp "liapoldus.local/server-plugin/internal/application/settings"
	siteapp "liapoldus.local/server-plugin/internal/application/site"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	"liapoldus.local/server-plugin/internal/infrastructure/site"
	"liapoldus.local/server-plugin/internal/presentation/restplugin"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

const (
	certificateRef  = "fixture-certificate"
	privateKeyRef   = "fixture-private-key"
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
	CertificateInvalidCursor adminResult `json:"certificateInvalidCursor"`
	CertificateMissing       adminResult `json:"certificateMissing"`
	CertificateStatus        adminResult `json:"certificateStatus"`
	CertificateList          adminResult `json:"certificateList"`
	TLS12                    result      `json:"tls12"`
	TLS13                    result      `json:"tls13"`
	RevisionAfterInvalidPair string      `json:"revisionAfterInvalidPair"`
	InitialRevision          string      `json:"initialRevision"`
	OldRevisionBody          string      `json:"oldRevisionBody"`
	ValidCode                string      `json:"validCode"`
	ValidRevision            string      `json:"validRevision"`
	OldRevisionStatus        int         `json:"oldRevisionStatus"`
	WrongHostnameRejected    bool        `json:"wrongHostnameRejected"`
	InvalidPairRejected      bool        `json:"invalidPairRejected"`
}

type adminResult struct {
	Body   map[string]any `json:"body"`
	Status int            `json:"status"`
}

type result struct {
	NegotiatedProtocol string `json:"negotiatedProtocol"`
	Body               string `json:"body"`
	Status             int    `json:"status"`
}

func main() {
	var request input
	check(json.NewDecoder(os.Stdin).Decode(&request))
	storageDirectory, cleanup, err := shared.IsolateCaddyDataHome()
	check(err)
	defer cleanup()
	check(shared.RegisterSiteDirectoryReader())

	certificatePEM, privateKeyPEM, wrongPrivateKeyPEM, rootPEM := makeCertificates()
	runtime := caddyruntime.New()
	siteRoot, err := caddyruntime.SiteRoot("custom-tls-fixture")
	check(err)
	check(os.MkdirAll(siteRoot, 0o700))
	sitePath := filepath.Join(siteRoot, "index.html")
	check(os.WriteFile(sitePath, []byte("old-active"), 0o600))
	configuration, err := settingsapp.NewConfiguration(runtime)
	check(err)
	defer func() { _ = configuration.Stop() }()
	initialConfig := settings(request.HTTPPort, nil, "")
	check(shared.Apply(configuration, initialConfig, initialRevision, nil))
	customConfig := settings(request.HTTPPort, &request.HTTPSPort, "invalid-key")
	invalidPairErr := shared.Apply(configuration, customConfig, invalidRevision, map[string][]byte{
		certificateRef: certificatePEM, privateKeyRef: wrongPrivateKeyPEM,
	})
	revisionAfterInvalidPair := configuration.Revision()

	oldStatus, oldBody := getHTTP(request.HTTPPort)
	check(os.WriteFile(sitePath, []byte("custom-tls"), 0o600))
	validConfig := settings(request.HTTPPort, &request.HTTPSPort, "")
	validErr := shared.Apply(configuration, validConfig, validRevision, map[string][]byte{
		certificateRef: certificatePEM, privateKeyRef: privateKeyPEM,
	})
	output := fixtureOutput{
		InitialRevision: initialRevision, InvalidPairRejected: invalidPairErr != nil,
		RevisionAfterInvalidPair: revisionAfterInvalidPair,
		OldRevisionStatus:        oldStatus, OldRevisionBody: oldBody, ValidCode: "OK",
	}
	if validErr != nil {
		output.ValidCode = "ERROR"
		check(json.NewEncoder(os.Stdout).Encode(output))
		return
	}

	tls12 := requestTLS(request.HTTPSPort, rootPEM, customHostname, tls.VersionTLS12)
	tls13 := requestTLS(request.HTTPSPort, rootPEM, customHostname, tls.VersionTLS13)
	_, wrongHostnameErr := dialTLS(request.HTTPSPort, rootPEM, "wrong.example.test", tls.VersionTLS13)

	output.ValidRevision = configuration.Revision()
	output.TLS12 = tls12
	output.TLS13 = tls13
	output.WrongHostnameRejected = wrongHostnameErr != nil
	releaseStore, err := site.NewReleaseStore(filepath.Join(storageDirectory, "site-releases"))
	check(err)
	publisher, err := siteapp.NewSitePublisher(releaseStore)
	check(err)
	adapter, err := restplugin.New(configuration, publisher)
	check(err)
	queryAction, err := contracts.AdminQueryActionID()
	check(err)
	output.CertificateList = callAdminAction(adapter, "certificates", queryAction, []byte(`{"resource":"certificates","limit":50}`))
	output.CertificateInvalidCursor = callAdminAction(adapter, "certificates", queryAction, []byte(`{"resource":"certificates","cursor":"%%%","limit":50}`))
	output.CertificateStatus = callAdminAction(adapter, "certificates", "status", []byte(`{"domain":"custom.example.test"}`))
	output.CertificateMissing = callAdminAction(adapter, "certificates", "status", []byte(`{"domain":"missing.example.test"}`))
	check(json.NewEncoder(os.Stdout).Encode(output))
}

func callAdminAction(adapter *restplugin.Adapter, pageID, actionID string, body []byte) adminResult {
	response, err := adapter.HandleAdminAction(context.Background(), sdkpresentation.AdminActionInput{
		Invocation: sdkmodels.AdminActionInvocation{CallerID: "fixture", InstanceID: "server", PageID: pageID,
			ActionID: actionID, SurfaceDigest: "fixture-digest", RequestID: "fixture-request"},
		Body: body,
	})
	check(err)
	var decoded map[string]any
	check(json.Unmarshal(response.Body, &decoded))
	return adminResult{Status: response.StatusCode, Body: decoded}
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

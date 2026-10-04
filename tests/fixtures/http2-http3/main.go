package main

import (
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

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"
	"liapoldus.local/server-plugin/internal/application"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

const (
	certificateRef = "fixture-certificate"
	privateKeyRef  = "fixture-private-key"
	settingsRev    = "http2-http3-revision"
	hostname       = "custom.example.test"
)

type input struct {
	HTTPSPort int `json:"httpsPort"`
}

type fixtureResult struct {
	Protocol   string `json:"protocol"`
	ALPN       string `json:"alpn"`
	Status     int    `json:"status"`
	Body       string `json:"body"`
	TLSVersion string `json:"tlsVersion"`
}

type fixtureOutput struct {
	ConfigApplied bool          `json:"configApplied"`
	HTTP2         fixtureResult `json:"http2"`
	HTTP3         fixtureResult `json:"http3"`
	TLS12         fixtureResult `json:"tls12"`
	TLS13         fixtureResult `json:"tls13"`
	TLS11Rejected bool          `json:"tls11Rejected"`
}

func main() {
	var request input
	check(json.NewDecoder(os.Stdin).Decode(&request))
	_, cleanup, err := shared.IsolateCaddyDataHome()
	check(err)
	defer cleanup()
	check(shared.RegisterSiteDirectoryReader())

	certificatePEM, privateKeyPEM, rootPEM := makeCertificates()
	runtime := caddyruntime.New()
	siteRoot, err := caddyruntime.SiteRoot("http2-http3-fixture")
	check(err)
	check(os.MkdirAll(siteRoot, 0o700))
	check(os.WriteFile(filepath.Join(siteRoot, "index.html"), []byte("secure-site"), 0o600))
	configuration, err := application.NewConfiguration(runtime)
	check(err)
	defer func() { _ = configuration.Stop() }()
	settings := makeSettings(request.HTTPSPort)
	check(shared.Apply(configuration, settings, settingsRev, map[string][]byte{
		certificateRef: certificatePEM,
		privateKeyRef:  privateKeyPEM,
	}))

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(rootPEM) {
		panic("fixture root certificate could not be loaded")
	}
	output := fixtureOutput{ConfigApplied: configuration.Revision() == settingsRev}
	output.HTTP2 = requestHTTP2(request.HTTPSPort, roots)
	output.HTTP3 = requestHTTP3(request.HTTPSPort, roots)
	output.TLS12 = requestHTTP1AtVersion(request.HTTPSPort, roots, tls.VersionTLS12)
	output.TLS13 = requestHTTP1AtVersion(request.HTTPSPort, roots, tls.VersionTLS13)
	output.TLS11Rejected = requestTLS11Rejected(request.HTTPSPort, roots)
	check(json.NewEncoder(os.Stdout).Encode(output))
}

func makeSettings(port int) []byte {
	contents, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"listeners": []any{map[string]any{
				"id": "secure-web", "kind": "http", "address": fmt.Sprintf("127.0.0.1:%d", port),
				"hostnames": []string{hostname}, "protocols": []string{"http1", "http2", "http3"},
				"tls": map[string]any{
					"mode": "custom", "certificateRef": certificateRef, "privateKeyRef": privateKeyRef,
				},
			}},
			"routes": []any{map[string]any{
				"id": "secure-site", "listenerId": "secure-web",
				"handler": map[string]any{"type": "static", "siteId": "http2-http3-fixture"},
			}},
		},
	})
	check(err)
	return contents
}

func requestHTTP2(port int, roots *x509.CertPool) fixtureResult {
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: roots, ServerName: hostname,
			MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS13,
		},
		DialTLSContext: func(ctx context.Context, _, _ string, tlsConfig *tls.Config) (net.Conn, error) {
			connection, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				return nil, err
			}
			tlsConnection := tls.Client(connection, tlsConfig)
			if err := tlsConnection.HandshakeContext(ctx); err != nil {
				_ = connection.Close()
				return nil, err
			}
			if tlsConnection.ConnectionState().NegotiatedProtocol != http2.NextProtoTLS {
				_ = connection.Close()
				return nil, fmt.Errorf("HTTP/2 ALPN was not negotiated")
			}
			return tlsConnection, nil
		},
	}
	defer transport.CloseIdleConnections()
	return doRequest(&http.Client{Transport: transport, Timeout: 10 * time.Second}, "https://"+hostname+fmt.Sprintf(":%d/", port))
}

func requestHTTP3(port int, roots *x509.CertPool) fixtureResult {
	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: roots, ServerName: hostname,
			MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS13,
		},
		Dial: func(ctx context.Context, _ string, tlsConfig *tls.Config, quicConfig *quic.Config) (*quic.Conn, error) {
			return quic.DialAddr(ctx, fmt.Sprintf("127.0.0.1:%d", port), tlsConfig, quicConfig)
		},
	}
	defer transport.Close()
	result := doRequest(&http.Client{Transport: transport, Timeout: 10 * time.Second}, "https://"+hostname+fmt.Sprintf(":%d/", port))
	if result.ALPN != "h3" {
		panic("HTTP/3 ALPN was not negotiated")
	}
	return result
}

func requestHTTP1AtVersion(port int, roots *x509.CertPool, version uint16) fixtureResult {
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: roots, ServerName: hostname, MinVersion: version, MaxVersion: version,
		NextProtos: []string{"http/1.1"},
	}, DialContext: dialContext(port)}
	defer transport.CloseIdleConnections()
	return doRequest(&http.Client{Transport: transport, Timeout: 10 * time.Second}, "https://"+hostname+fmt.Sprintf(":%d/", port))
}

func requestTLS11Rejected(port int, roots *x509.CertPool) bool {
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: roots, ServerName: hostname,
		MinVersion: tls.VersionTLS11, MaxVersion: tls.VersionTLS11,
		NextProtos: []string{"http/1.1"},
	}, DialContext: dialContext(port)}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequest(http.MethodGet, "https://"+hostname+fmt.Sprintf(":%d/", port), nil)
	if err != nil {
		panic(err)
	}
	response, err := transport.RoundTrip(request)
	if response != nil {
		_ = response.Body.Close()
	}
	return err != nil
}

func doRequest(client *http.Client, address string) fixtureResult {
	response, err := client.Get(address)
	check(err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	check(err)
	version := ""
	if response.TLS != nil {
		version = tlsVersion(response.TLS.Version)
	}
	alpn := ""
	if response.TLS != nil {
		alpn = response.TLS.NegotiatedProtocol
	}
	return fixtureResult{Protocol: response.Proto, ALPN: alpn, Status: response.StatusCode, Body: string(body), TLSVersion: version}
}

func dialContext(port int) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	}
}

func tlsVersion(version uint16) string {
	switch version {
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	default:
		return "unknown"
	}
}

func makeCertificates() ([]byte, []byte, []byte) {
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Caddy HTTP2 HTTP3 fixture CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(48 * time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	check(err)
	ca, err := x509.ParseCertificate(caDER)
	check(err)

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: hostname}, DNSNames: []string{hostname},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(48 * time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature, BasicConstraintsValid: true,
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, ca, &serverKey.PublicKey, caKey)
	check(err)
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	check(err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

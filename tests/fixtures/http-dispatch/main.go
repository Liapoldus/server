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
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Liapoldus/pluginprotocol/presentation/peer"
	"liapoldus.local/server-plugin/internal/application"
	"liapoldus.local/server-plugin/internal/domain/models"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
)

type requestPayload struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Cookies []json.RawMessage `json:"cookies"`
}

func main() {
	serverSecurity, clientSecurity := peerCredentials()
	var received requestPayload
	registry, err := peer.NewRegistry().RegisterCall("http.echo", func(_ context.Context, call peer.Call) (peer.Result, error) {
		if err := json.Unmarshal(call.Payload, &received); err != nil {
			return peer.Result{}, err
		}
		return peer.Result{Payload: []byte(`{"status":202,"headers":{"Content-Type":"text/plain"},"body":"plugin-response"}`)}, nil
	}).RegisterCall("http.invalid", func(context.Context, peer.Call) (peer.Result, error) {
		return peer.Result{Payload: []byte(`{"status":200,"headers":{"Set-Cookie":"session=secret; HttpOnly"},"body":"must-not-escape"}`)}, nil
	}).Build()
	check(err)
	peerServer, err := peer.Listen(peer.ServerConfig{
		Network:  peer.NetworkConfig{Carrier: peer.CarrierTCP, Endpoint: "127.0.0.1:0"},
		Security: serverSecurity, Handler: registry,
	})
	check(err)
	peerAddress := peerServer.Addr()
	go func() { _ = peerServer.Sessions(context.Background()) }()
	defer func() { _ = peerServer.Close() }()

	publicListener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	publicAddress := publicListener.Addr().String()
	check(publicListener.Close())

	runtime := caddyruntime.New()
	check(runtime.SetDispatchTargets([]caddyruntime.DispatchTarget{{
		ID: "fixture", Endpoint: peerAddress, TimeoutMillis: 5000, Security: clientSecurity,
	}}))
	configuration, err := application.NewConfiguration(runtime)
	check(err)
	defer func() { _ = configuration.Stop() }()
	settings, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"listeners": []any{map[string]any{
				"id": "web", "kind": "http", "address": publicAddress,
				"hostnames": []string{}, "protocols": []string{"http1"}, "tls": map[string]any{"mode": "disabled"},
			}},
			"routes": []any{
				map[string]any{
					"id": "dispatch", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/submit"}},
					"handler": map[string]any{"type": "plugin", "instanceId": "fixture", "capability": "http.echo", "mode": "call"},
				},
				map[string]any{
					"id": "invalid-action", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/invalid"}},
					"handler": map[string]any{"type": "plugin", "instanceId": "fixture", "capability": "http.invalid", "mode": "call"},
				},
			},
		},
	})
	check(err)
	decoded, err := models.DecodeSettings(settings, "schemaVersion", "config", 1)
	check(err)
	check(configuration.Apply(decoded, "revision-1"))

	client := &http.Client{Timeout: 5 * time.Second}
	request, err := http.NewRequest(http.MethodPost, "http://"+publicAddress+"/submit", nil)
	check(err)
	request.Header.Set("X-Request-ID", "fixture-request")
	request.Header.Set("Cookie", "private=must-not-cross-boundary")
	response, err := client.Do(request)
	check(err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	check(err)
	check(json.NewEncoder(os.Stdout).Encode(map[string]any{
		"revision": configuration.Revision(), "status": response.StatusCode,
		"contentType": response.Header.Get("Content-Type"), "body": string(body),
		"method": received.Method, "path": received.Path,
		"cookieForwarded": received.Headers["Cookie"] != "" || len(received.Cookies) != 0,
		"mutualTLS":       true,
		"invalidAction":   requestInvalidAction(client, publicAddress),
	}))
}

func peerCredentials() (peer.SecurityConfig, peer.SecurityConfig) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture root"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	check(err)
	root, err := x509.ParseCertificate(rootDER)
	check(err)
	roots := x509.NewCertPool()
	roots.AddCert(root)
	issue := func(serial int64, name, uri string) tls.Certificate {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		check(err)
		parsed, err := url.Parse(uri)
		check(err)
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name}, URIs: []*url.URL{parsed},
			DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
			NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
			KeyUsage:    x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, root, &key.PublicKey, rootKey)
		check(err)
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		check(err)
		certificate, err := tls.X509KeyPair(
			pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
			pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		)
		check(err)
		return certificate
	}
	serverURI := "spiffe://liapoldus.test/forms"
	clientURI := "spiffe://liapoldus.test/server"
	return peer.SecurityConfig{Identity: serverURI, Certificate: issue(2, "forms", serverURI), Roots: roots, PeerIdentity: clientURI},
		peer.SecurityConfig{Identity: clientURI, Certificate: issue(3, "server", clientURI), Roots: roots, PeerIdentity: serverURI}
}

func requestInvalidAction(client *http.Client, address string) map[string]any {
	response, err := client.Get("http://" + address + "/invalid")
	check(err)
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	check(err)
	cookies := response.Header.Values("Set-Cookie")
	if cookies == nil {
		cookies = []string{}
	}
	return map[string]any{"status": response.StatusCode, "setCookie": cookies, "body": string(contents)}
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

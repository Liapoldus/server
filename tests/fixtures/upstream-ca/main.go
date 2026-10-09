package main

import (
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
	"net/http/httptest"
	"net/netip"
	"os"
	"time"

	settingsapp "liapoldus.local/server-plugin/internal/application/settings"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

const (
	settingsRevision = "revision-1"
	caReference      = "private-upstream-ca"
)

type input struct {
	Port          int  `json:"port"`
	TrustCA       bool `json:"trustCA"`
	WrongHostname bool `json:"wrongHostname"`
}

func main() {
	var request input
	check(json.NewDecoder(os.Stdin).Decode(&request))
	storageDirectory, cleanup, err := shared.IsolateCaddyDataHome()
	check(err)
	defer cleanup()
	check(os.Setenv("HOME", storageDirectory))

	certificate, caPEM := makeCertificate(request.WrongHostname)
	var fallbackRequests int
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("trusted-origin"))
	}))
	origin.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}}
	origin.StartTLS()
	defer origin.Close()

	upstreams := []map[string]any{{"origin": origin.URL, "weight": 100}}
	if request.TrustCA {
		upstreams[0]["caRef"] = caReference
	}
	if !request.TrustCA || request.WrongHostname {
		fallback := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			fallbackRequests++
			_, _ = writer.Write([]byte("fallback-origin"))
		}))
		defer fallback.Close()
		upstreams = append(upstreams, map[string]any{"origin": fallback.URL, "weight": 1})
	}

	runtime := caddyruntime.New()
	configuration, err := settingsapp.NewConfiguration(runtime)
	check(err)
	requestSettings, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"listeners": []any{map[string]any{
				"id": "web", "kind": "http", "address": fmt.Sprintf("127.0.0.1:%d", request.Port),
				"hostnames": []string{}, "protocols": []string{"http1"}, "tls": map[string]any{"mode": "disabled"},
			}},
			"routes": []any{map[string]any{
				"id": "upstream-ca", "listenerId": "web", "handler": map[string]any{"type": "reverseProxy", "upstreams": upstreams},
			}},
		},
	})
	check(err)

	var secrets map[string][]byte
	if request.TrustCA {
		secrets = map[string][]byte{caReference: caPEM}
	}
	check(shared.Apply(configuration, requestSettings, settingsRevision, secrets))
	defer configuration.Stop()

	requestCount := 1
	if !request.TrustCA || request.WrongHostname {
		requestCount = 2
	}
	statuses := make([]int, 0, requestCount)
	bodies := make([]string, 0, requestCount)
	client := &http.Client{Timeout: 5 * time.Second}
	for index := 0; index < requestCount; index++ {
		response, requestErr := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", request.Port))
		check(requestErr)
		contents, readErr := io.ReadAll(response.Body)
		check(readErr)
		check(response.Body.Close())
		statuses = append(statuses, response.StatusCode)
		bodies = append(bodies, string(contents))
	}
	check(json.NewEncoder(os.Stdout).Encode(map[string]any{
		"revision": configuration.Revision(), "statuses": statuses, "bodies": bodies,
		"fallbackRequests": fallbackRequests,
	}))
}

func makeCertificate(wrongHostname bool) (tls.Certificate, []byte) {
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)
	caSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	check(err)
	caTemplate := &x509.Certificate{
		SerialNumber: caSerial, Subject: pkix.Name{CommonName: "Caddy fixture root"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	check(err)
	ca, err := x509.ParseCertificate(caDER)
	check(err)

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)
	serverSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	check(err)
	addressText := "127.0.0.1"
	if wrongHostname {
		addressText = "127.0.0.2"
	}
	address, err := netip.ParseAddr(addressText)
	check(err)
	serverTemplate := &x509.Certificate{
		SerialNumber: serverSerial, Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		IPAddresses: []net.IP{address.AsSlice()}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true,
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, ca, &serverKey.PublicKey, caKey)
	check(err)
	privateKey, err := x509.MarshalPKCS8PrivateKey(serverKey)
	check(err)
	certificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey}),
	)
	check(err)
	return certificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

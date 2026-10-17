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
	"strings"
	"sync/atomic"
	"time"

	"github.com/Liapoldus/wool/v3/presentation/peer"
	"liapoldus.local/server-service/contracts"
	settingsapp "liapoldus.local/server-service/internal/application/settings"
	settingsmodel "liapoldus.local/server-service/internal/domain/models/settings"
	caddyruntime "liapoldus.local/server-service/internal/infrastructure/caddy"
	"liapoldus.local/server-service/tests/fixtures/shared"
)

type requestPayload struct {
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Headers    map[string]string `json:"headers"`
	Cookies    []cookiePair      `json:"cookies"`
	Parameters struct {
		Version string         `json:"version"`
		Path    map[string]any `json:"path"`
		Query   map[string]any `json:"query"`
	} `json:"parameters"`
}

type cookiePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func main() {
	_, cleanup, err := shared.IsolateCaddyDataHome()
	check(err)
	defer cleanup()

	serverSecurity, clientSecurity := peerCredentials()
	var received requestPayload
	var echoCalls atomic.Int64
	streamContract, err := contracts.LoadHTTPStream()
	check(err)
	var streamReceived, streamMethod, streamPath string
	var streamBytesRead int64
	registry, err := peer.NewRegistry().RegisterCall("http.echo", func(_ context.Context, call peer.Call) (peer.Result, error) {
		echoCalls.Add(1)
		if err := json.Unmarshal(call.Payload, &received); err != nil {
			return peer.Result{}, err
		}
		return peer.Result{Payload: []byte(`{"status":202,"headers":{"Content-Type":"text/plain"},"cookies":[{"name":"session","value":"ordinary-value","path":"/","secure":true,"sameSite":"lax"},{"name":"auth","value":"http-only-value","path":"/","secure":true,"httpOnly":true,"sameSite":"strict"}],"body":"service-response"}`)}, nil
	}).RegisterCall("http.invalid", func(context.Context, peer.Call) (peer.Result, error) {
		return peer.Result{Payload: []byte(`{"status":200,"headers":{"Content-Type":"text/plain"},"cookies":[{"name":"session","value":"secret","httpOnly":true},{"name":"invalid;name","value":"bad"}],"body":"must-not-escape"}`)}, nil
	}).RegisterStream("http.stream", func(stream peer.Stream) error {
		first, receiveErr := stream.Recv()
		if receiveErr != nil {
			return receiveErr
		}
		var start struct {
			Kind    string         `json:"kind"`
			Request requestPayload `json:"request"`
		}
		if json.Unmarshal(first.Payload, &start) != nil || start.Kind != streamContract.RequestStartKind {
			return peer.ErrInvalidRequest
		}
		streamMethod, streamPath = start.Request.Method, start.Request.Path
		var body strings.Builder
		for {
			message, receiveErr := stream.Recv()
			if receiveErr != nil {
				return receiveErr
			}
			var frame struct {
				Kind string `json:"kind"`
				Data []byte `json:"data"`
			}
			if json.Unmarshal(message.Payload, &frame) != nil {
				return peer.ErrInvalidRequest
			}
			switch frame.Kind {
			case streamContract.RequestChunkKind:
				body.Write(frame.Data)
				streamBytesRead += int64(len(frame.Data))
			case streamContract.RequestEndKind:
				streamReceived = body.String()
				startFrame, _ := json.Marshal(map[string]any{"kind": streamContract.ResponseStartKind, "status": 201, "headers": map[string]string{"Content-Type": "text/plain"}, "cookies": []map[string]any{{"name": "stream-session", "value": "stream-value", "httpOnly": true, "secure": true}}})
				if err := stream.Send(peer.Message{Payload: startFrame}); err != nil {
					return err
				}
				for _, part := range [][]byte{[]byte("accepted:"), []byte(streamReceived)} {
					chunk, _ := json.Marshal(map[string]any{"kind": streamContract.ResponseChunkKind, "data": part})
					if err := stream.Send(peer.Message{Payload: chunk}); err != nil {
						return err
					}
				}
				endFrame, _ := json.Marshal(map[string]any{"kind": streamContract.ResponseEndKind})
				if err := stream.Send(peer.Message{Payload: endFrame}); err != nil {
					return err
				}
				// The generic peer API currently ends a stream when the handler
				// returns. Keep the handler alive until the caller consumes the
				// terminal response frame and cancels its stream context.
				<-stream.Context().Done()
				return nil
			default:
				return peer.ErrInvalidRequest
			}
		}
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
	check(runtime.SetDispatchCaller(shared.PeerCaller{Targets: map[string]shared.PeerTarget{
		"fixture": {Endpoint: peerAddress, Security: clientSecurity},
	}}))
	configuration, err := settingsapp.NewConfiguration(runtime)
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
					"id": "dispatch", "listenerId": "web", "api": serviceAPI("POST", "/submit", []any{map[string]any{"name": "text", "type": "string", "repeated": "single", "required": true, "maxLength": 128}}),
					"handler": map[string]any{"type": "service", "instanceId": "fixture", "capability": "http.echo", "mode": "call", "requestCookieNames": []string{"session", "csrf"}},
				},
				map[string]any{
					"id": "invalid-action", "listenerId": "web", "api": serviceAPI("GET", "/invalid", nil),
					"handler": map[string]any{"type": "service", "instanceId": "fixture", "capability": "http.invalid", "mode": "call"},
				},
				map[string]any{
					"id": "stream", "listenerId": "web", "api": serviceAPI("POST", "/stream", nil),
					"handler": map[string]any{"type": "service", "instanceId": "fixture", "capability": "http.stream", "mode": "http_stream"},
				},
			},
		},
	})
	check(err)
	decoded, err := settingsmodel.DecodeSettings(settings)
	check(err)
	check(configuration.Apply(decoded, "revision-1"))

	client := &http.Client{Timeout: 5 * time.Second}
	request, err := http.NewRequest(http.MethodPost, "http://"+publicAddress+"/submit?text=typed-value", nil)
	check(err)
	request.Header.Set("X-Request-ID", "fixture-request")
	request.Header.Set("Cookie", "private=must-not-cross-boundary; session=allowed-value; csrf=csrf-value; session=second-value")
	response, err := client.Do(request)
	check(err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	check(err)
	check(response.Body.Close())
	invalidInputRequest, err := http.NewRequest(http.MethodPost, "http://"+publicAddress+"/submit", nil)
	check(err)
	invalidInputResponse, err := client.Do(invalidInputRequest)
	check(err)
	check(invalidInputResponse.Body.Close())
	streamRequest, err := http.NewRequest(http.MethodPost, "http://"+publicAddress+"/stream", strings.NewReader("alpha-beta"))
	check(err)
	streamResponse, err := client.Do(streamRequest)
	check(err)
	streamBody, err := io.ReadAll(streamResponse.Body)
	check(err)
	check(streamResponse.Body.Close())
	streamCookies := streamResponse.Header.Values("Set-Cookie")
	oversizedRequest, err := http.NewRequest(http.MethodPost, "http://"+publicAddress+"/stream", io.NopCloser(strings.NewReader(strings.Repeat("x", 1_048_577))))
	check(err)
	oversizedRequest.ContentLength = -1
	oversizedResponse, err := client.Do(oversizedRequest)
	check(err)
	check(oversizedResponse.Body.Close())
	check(json.NewEncoder(os.Stdout).Encode(map[string]any{
		"revision": configuration.Revision(), "status": response.StatusCode,
		"contentType": response.Header.Get("Content-Type"), "body": string(body),
		"setCookie": response.Header.Values("Set-Cookie"),
		"method":    received.Method, "path": received.Path,
		"typedRequestVersion":    received.Parameters.Version,
		"typedQueryText":         received.Parameters.Query["text"],
		"invalidInputStatus":     invalidInputResponse.StatusCode,
		"echoServiceCalls":       echoCalls.Load(),
		"cookieForwarded":        len(received.Cookies) != 0,
		"receivedCookies":        received.Cookies,
		"privateCookieForwarded": containsCookie(received.Cookies, "private"),
		"mutualTLS":              true,
		"invalidAction":          requestInvalidAction(client, publicAddress),
		"streamStatus":           streamResponse.StatusCode,
		"streamSetCookie":        streamCookies,
		"streamContentType":      streamResponse.Header.Get("Content-Type"),
		"streamBody":             string(streamBody),
		"streamReceived":         streamReceived,
		"streamMethod":           streamMethod,
		"streamPath":             streamPath,
		"oversizedStatus":        oversizedResponse.StatusCode,
		"streamBytesRead":        streamBytesRead,
	}))
}

func serviceAPI(method, path string, queryParameters []any) map[string]any {
	return map[string]any{
		"version": "server-http/v1", "method": method, "path": path,
		"queryParameters": queryParameters,
		"responses": []any{map[string]any{
			"status": 200, "contentType": "application/json", "schema": map[string]any{}, "example": nil,
		}},
	}
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

func containsCookie(cookies []cookiePair, name string) bool {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return true
		}
	}
	return false
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

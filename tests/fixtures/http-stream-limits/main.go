package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"time"

	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
	"liapoldus.local/server-plugin/internal/application"
	"liapoldus.local/server-plugin/internal/domain/models"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

const backpressureTotalChunks = 4096

type requestStart struct {
	Kind    string `json:"kind"`
	Request struct {
		Path string `json:"path"`
	} `json:"request"`
}

func main() {
	_, cleanup, err := shared.IsolateCaddyDataHome()
	check(err)
	defer cleanup()

	serverSecurity, clientSecurity := peerCredentials()
	var started atomic.Int64
	var backpressureSent atomic.Int64
	var backpressureCompleted atomic.Bool
	var backpressureQueueFull atomic.Bool
	overflowContinue := make(chan struct{})
	registry, err := peer.NewRegistry().RegisterStream("forms.events", func(stream peer.Stream) error {
		first, err := stream.Recv()
		if err != nil {
			return err
		}
		var start requestStart
		if json.Unmarshal(first.Payload, &start) != nil {
			return peer.ErrInvalidRequest
		}
		started.Add(1)
		for {
			message, receiveErr := stream.Recv()
			if receiveErr != nil {
				return receiveErr
			}
			var frame struct {
				Kind string `json:"kind"`
			}
			if json.Unmarshal(message.Payload, &frame) != nil {
				return peer.ErrInvalidRequest
			}
			if frame.Kind == "request_end" {
				break
			}
		}
		send := func(value any) error {
			return sendMessage(stream, value)
		}
		if start.Request.Path == "/slow" {
			time.Sleep(750 * time.Millisecond)
			if err := send(map[string]any{"kind": "response_start", "status": 200, "headers": map[string]string{"Content-Type": "text/plain"}}); err != nil {
				return err
			}
			if err := send(map[string]any{"kind": "response_chunk", "data": []byte("stream-ok")}); err != nil {
				return err
			}
			return send(map[string]any{"kind": "response_end"})
		}
		if start.Request.Path == "/overflow" {
			if err := send(map[string]any{"kind": "response_start", "status": 200, "headers": map[string]string{"Content-Type": "text/plain"}}); err != nil {
				return err
			}
			if err := send(map[string]any{"kind": "response_chunk", "data": []byte("prefix")}); err != nil {
				return err
			}
			<-overflowContinue
			return send(map[string]any{"kind": "response_chunk", "data": bytes.Repeat([]byte("x"), 24001)})
		}
		if start.Request.Path == "/backpressure" {
			if err := send(map[string]any{"kind": "response_start", "status": 200, "headers": map[string]string{"Content-Type": "application/octet-stream"}}); err != nil {
				return err
			}
			chunk := bytes.Repeat([]byte("x"), 24000)
			for range backpressureTotalChunks {
				encoded, err := json.Marshal(map[string]any{"kind": "response_chunk", "data": chunk})
				if err == nil {
					err = stream.Send(peer.Message{Payload: encoded})
				}
				if err != nil {
					backpressureQueueFull.Store(errors.Is(err, peer.ErrSendQueueFull))
					return err
				}
				backpressureSent.Add(1)
			}
			backpressureCompleted.Store(true)
			return send(map[string]any{"kind": "response_end"})
		}
		contentType := "text/plain"
		if start.Request.Path == "/events" || start.Request.Path == "/events-invalid" {
			contentType = "text/event-stream"
		}
		if err := send(map[string]any{"kind": "response_start", "status": 200, "headers": map[string]string{"Content-Type": contentType}}); err != nil {
			return err
		}
		if start.Request.Path == "/idle" {
			<-stream.Context().Done()
			return nil
		}
		if start.Request.Path == "/maximum" {
			if err := send(map[string]any{"kind": "response_chunk", "data": []byte("started")}); err != nil {
				return err
			}
			ticker := time.NewTicker(70 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stream.Context().Done():
					return nil
				case <-ticker.C:
					if err := send(map[string]any{"kind": "response_chunk", "data": []byte(".")}); err != nil {
						return err
					}
				}
			}
		}
		if start.Request.Path == "/events" {
			if err := send(map[string]any{"kind": "sse_event", "event": "update", "data": "first\nsecond", "id": "17", "retry": 1000}); err != nil {
				return err
			}
			return send(map[string]any{"kind": "response_end"})
		}
		if start.Request.Path == "/events-invalid" {
			_ = send(map[string]any{"kind": "sse_event", "data": "safe", "id": "bad\nInjected: value"})
			return nil
		}
		return peer.ErrInvalidRequest
	}).Build()
	check(err)
	peerServer, err := peer.Listen(peer.ServerConfig{Network: peer.NetworkConfig{Carrier: peer.CarrierTCP, Endpoint: "127.0.0.1:0"}, Security: serverSecurity, Handler: registry})
	check(err)
	go func() { _ = peerServer.Sessions(context.Background()) }()
	defer peerServer.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	address := listener.Addr().String()
	check(listener.Close())
	runtime := caddyruntime.New()
	check(runtime.SetDispatchTargets([]caddyruntime.DispatchTarget{{ID: "forms", Endpoint: peerServer.Addr(), TimeoutMillis: 500, Security: clientSecurity}}))
	configuration, err := application.NewConfiguration(runtime)
	check(err)
	defer configuration.Stop()
	settings, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"listeners": []any{map[string]any{"id": "web", "kind": "http", "address": address, "hostnames": []string{}, "protocols": []string{"http1"}, "tls": map[string]any{"mode": "disabled"}}},
			"routes": []any{
				map[string]any{"id": "events", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/slow"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.events", "mode": "http_stream", "streamLimits": map[string]any{"maxConcurrency": 1, "idleTimeoutMillis": 1000, "maxDurationMillis": 1800}}},
				map[string]any{"id": "idle", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/idle"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.events", "mode": "http_stream", "streamLimits": map[string]any{"maxConcurrency": 1, "idleTimeoutMillis": 1000, "maxDurationMillis": 1800}}},
				map[string]any{"id": "maximum", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/maximum"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.events", "mode": "http_stream", "streamLimits": map[string]any{"maxConcurrency": 1, "idleTimeoutMillis": 1000, "maxDurationMillis": 1800}}},
				map[string]any{"id": "sse", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/events"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.events", "mode": "sse", "streamLimits": map[string]any{"maxConcurrency": 1, "idleTimeoutMillis": 1000, "maxDurationMillis": 1800}}},
				map[string]any{"id": "sse-invalid", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/events-invalid"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.events", "mode": "sse", "streamLimits": map[string]any{"maxConcurrency": 1, "idleTimeoutMillis": 1000, "maxDurationMillis": 1800}}},
				map[string]any{"id": "overflow", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/overflow"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.events", "mode": "http_stream", "streamLimits": map[string]any{"maxConcurrency": 1, "idleTimeoutMillis": 5000, "maxDurationMillis": 30000}}},
				map[string]any{"id": "backpressure", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/backpressure"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.events", "mode": "http_stream", "streamLimits": map[string]any{"maxConcurrency": 1, "idleTimeoutMillis": 20000, "maxDurationMillis": 30000}}},
			},
		},
	})
	check(err)
	decoded, err := models.DecodeSettings(settings, "schemaVersion", "config", 1)
	check(err)
	check(runtime.Validate(decoded.RuntimeConfig))
	check(configuration.Apply(decoded, "stream-limits"))
	client := &http.Client{Timeout: 30 * time.Second}
	slowDone := make(chan *http.Response, 1)
	slowErr := make(chan error, 1)
	slowStart := time.Now()
	go func() {
		response, requestErr := client.Get("http://" + address + "/slow")
		if requestErr != nil {
			slowErr <- requestErr
			return
		}
		slowDone <- response
	}()
	deadline := time.Now().Add(time.Second)
	for started.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	concurrent := get(client, "http://"+address+"/slow")
	var slow responseResult
	select {
	case err := <-slowErr:
		check(err)
	case response := <-slowDone:
		slow = readResponse(response)
	}
	slowElapsed := time.Since(slowStart).Milliseconds()
	idleStart := time.Now()
	idle := readResponse(get(client, "http://"+address+"/idle"))
	idleElapsed := time.Since(idleStart).Milliseconds()
	maxStart := time.Now()
	maximum := readResponse(get(client, "http://"+address+"/maximum"))
	maxElapsed := time.Since(maxStart).Milliseconds()
	sseResponse := get(client, "http://"+address+"/events")
	sseBody, err := io.ReadAll(sseResponse.Body)
	check(err)
	check(sseResponse.Body.Close())
	invalidSSEResponse := get(client, "http://"+address+"/events-invalid")
	invalidSSEBody, err := io.ReadAll(invalidSSEResponse.Body)
	check(err)
	check(invalidSSEResponse.Body.Close())
	overflowResponse := get(client, "http://"+address+"/overflow")
	overflowPrefix := make([]byte, len("prefix"))
	_, err = io.ReadFull(overflowResponse.Body, overflowPrefix)
	check(err)
	close(overflowContinue)
	overflowRemainder, _ := io.ReadAll(overflowResponse.Body)
	check(overflowResponse.Body.Close())
	overflowBody := append(overflowPrefix, overflowRemainder...)
	overflow := responseResult{Status: overflowResponse.StatusCode, Body: string(overflowBody)}
	backpressureResponse := get(client, "http://"+address+"/backpressure")
	time.Sleep(250 * time.Millisecond)
	pausedSentChunks := backpressureSent.Load()
	backpressureBytes, err := io.Copy(io.Discard, backpressureResponse.Body)
	check(err)
	check(backpressureResponse.Body.Close())
	check(json.NewEncoder(os.Stdout).Encode(map[string]any{"slow": slow, "slowElapsedMillis": slowElapsed, "concurrent": map[string]int{"status": concurrent.StatusCode}, "idle": idle, "idleElapsedMillis": idleElapsed, "maximum": maximum, "maximumElapsedMillis": maxElapsed, "sse": responseResult{Status: sseResponse.StatusCode, ContentType: sseResponse.Header.Get("Content-Type"), Body: string(sseBody)}, "invalidSSE": responseResult{Status: invalidSSEResponse.StatusCode, Body: string(invalidSSEBody)}, "afterStartOverflow": overflow, "backpressure": map[string]any{"status": backpressureResponse.StatusCode, "completed": backpressureCompleted.Load(), "queueFull": backpressureQueueFull.Load(), "pausedSentChunks": pausedSentChunks, "totalChunks": backpressureTotalChunks, "bodyBytes": backpressureBytes}}))
}

func sendMessage(stream peer.Stream, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	message := peer.Message{Payload: encoded}
	for {
		err = stream.Send(message)
		if !errors.Is(err, peer.ErrSendQueueFull) {
			return err
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-stream.Context().Done():
			if !timer.Stop() {
				<-timer.C
			}
			return stream.Context().Err()
		case <-timer.C:
		}
	}
}

type responseResult struct {
	Status      int    `json:"status"`
	ContentType string `json:"contentType,omitempty"`
	Body        string `json:"body"`
}

func get(client *http.Client, url string) *http.Response {
	response, err := client.Get(url)
	check(err)
	return response
}

func readResponse(response *http.Response) responseResult {
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	check(err)
	return responseResult{Status: response.StatusCode, Body: string(body)}
}

func peerCredentials() (peer.SecurityConfig, peer.SecurityConfig) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)
	rootTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "stream root"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	check(err)
	root, err := x509.ParseCertificate(rootDER)
	check(err)
	roots := x509.NewCertPool()
	roots.AddCert(root)
	issue := func(serial int64, commonName, identity string) tls.Certificate {
		key, keyErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		check(keyErr)
		uri, parseErr := url.Parse(identity)
		check(parseErr)
		certificateDER, certErr := x509.CreateCertificate(rand.Reader, &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: commonName}, URIs: []*url.URL{uri}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}, root, &key.PublicKey, rootKey)
		check(certErr)
		keyDER, marshalErr := x509.MarshalPKCS8PrivateKey(key)
		check(marshalErr)
		pair, pairErr := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
		check(pairErr)
		return pair
	}
	serverID, clientID := "spiffe://liapoldus.test/forms", "spiffe://liapoldus.test/server"
	return peer.SecurityConfig{Identity: serverID, Certificate: issue(2, "forms", serverID), Roots: roots, PeerIdentity: clientID}, peer.SecurityConfig{Identity: clientID, Certificate: issue(3, "server", clientID), Roots: roots, PeerIdentity: serverID}
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
	"github.com/gorilla/websocket"
	"liapoldus.local/server-plugin/internal/application"
	"liapoldus.local/server-plugin/internal/domain/models"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

type peerFrame struct {
	Kind        string `json:"kind"`
	MessageType string `json:"messageType"`
	Data        []byte `json:"data"`
	WebSocket   struct {
		Subprotocols []string `json:"subprotocols"`
	} `json:"websocket"`
	Request struct {
		Path    string            `json:"path"`
		Headers map[string]string `json:"headers"`
	} `json:"request"`
}

type observation struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

func main() {
	_, cleanup, err := shared.IsolateCaddyDataHome()
	check(err)
	defer cleanup()

	serverSecurity, clientSecurity := peerCredentials()
	var received []observation
	registry, err := peer.NewRegistry().RegisterStream("forms.socket", func(stream peer.Stream) error {
		first, receiveErr := stream.Recv()
		if receiveErr != nil {
			return receiveErr
		}
		var start peerFrame
		if json.Unmarshal(first.Payload, &start) != nil {
			return peer.ErrInvalidRequest
		}
		if start.Request.Path == "/reject" {
			return send(stream, map[string]any{"kind": "websocket_handshake", "accept": false, "status": http.StatusForbidden, "cookies": []map[string]any{{"name": "csrf", "value": "ordinary", "secure": true}}})
		}
		if start.Request.Path == "/bad-subprotocol" {
			return send(stream, map[string]any{"kind": "websocket_handshake", "accept": true, "subprotocol": "not-offered"})
		}
		if start.Request.Path == "/bad-cookie" {
			return send(stream, map[string]any{"kind": "websocket_handshake", "accept": true, "cookies": []map[string]any{{"name": "bad;name", "value": "secret"}}})
		}
		if start.Request.Headers["Origin"] != "https://client.example" || len(start.WebSocket.Subprotocols) != 2 {
			return peer.ErrInvalidRequest
		}
		if err := send(stream, map[string]any{"kind": "websocket_handshake", "accept": true, "subprotocol": "forms.v1", "cookies": []map[string]any{{"name": "session", "value": "ordinary", "secure": true}, {"name": "auth", "value": "opaque", "secure": true, "httpOnly": true, "sameSite": "strict"}}}); err != nil {
			return err
		}
		var current observation
		for {
			message, recvErr := stream.Recv()
			if recvErr != nil {
				return recvErr
			}
			var frame peerFrame
			if json.Unmarshal(message.Payload, &frame) != nil {
				return peer.ErrInvalidRequest
			}
			switch frame.Kind {
			case "websocket_message_start":
				current.Type = frame.MessageType
			case "websocket_message_chunk":
				current.Data += string(frame.Data)
			case "websocket_message_end":
				if current.Type == "binary" {
					current.Data = base64.StdEncoding.EncodeToString([]byte(current.Data))
				}
				received = append(received, current)
				if current.Type == "text" {
					if err := sendWSMessage(stream, "text", []byte("accepted")); err != nil {
						return err
					}
				} else {
					if err := sendWSMessage(stream, "binary", []byte{1, 2, 3}); err != nil {
						return err
					}
					if err := send(stream, map[string]any{"kind": "websocket_close", "code": websocket.CloseNormalClosure}); err != nil {
						return err
					}
					return nil
				}
				current = observation{}
			default:
				return peer.ErrInvalidRequest
			}
		}
	}).Build()
	check(err)
	peerServer, err := peer.Listen(peer.ServerConfig{Network: peer.NetworkConfig{Carrier: peer.CarrierTCP, Endpoint: "127.0.0.1:0"}, Security: serverSecurity, Handler: registry})
	check(err)
	go func() { _ = peerServer.Sessions(context.Background()) }()
	defer peerServer.Close()

	publicListener, err := net.Listen("tcp", "127.0.0.1:0")
	check(err)
	publicAddress := publicListener.Addr().String()
	check(publicListener.Close())
	runtime := caddyruntime.New()
	check(runtime.SetDispatchTargets([]caddyruntime.DispatchTarget{{ID: "forms", Endpoint: peerServer.Addr(), TimeoutMillis: 1000, Security: clientSecurity}}))
	configuration, err := application.NewConfiguration(runtime)
	check(err)
	defer configuration.Stop()
	settings, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"listeners": []any{map[string]any{"id": "web", "kind": "http", "address": publicAddress, "hostnames": []string{}, "protocols": []string{"http1"}, "tls": map[string]any{"mode": "disabled"}}},
			"routes": []any{
				map[string]any{"id": "socket", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/socket"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.socket", "mode": "websocket"}},
				map[string]any{"id": "reject", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/reject"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.socket", "mode": "websocket"}},
				map[string]any{"id": "bad-subprotocol", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/bad-subprotocol"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.socket", "mode": "websocket"}},
				map[string]any{"id": "bad-cookie", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/bad-cookie"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.socket", "mode": "websocket"}},
				map[string]any{"id": "oversized", "listenerId": "web", "match": map[string]any{"path": map[string]any{"type": "exact", "value": "/oversized"}}, "handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.socket", "mode": "websocket"}},
			},
		},
	})
	check(err)
	decoded, err := models.DecodeSettings(settings, "schemaVersion", "config", 1)
	check(err)
	check(runtime.Validate(decoded.RuntimeConfig))
	check(configuration.Apply(decoded, "websocket-test"))

	dialer := websocket.Dialer{Subprotocols: []string{"forms.v1", "forms.v2"}, HandshakeTimeout: 3 * time.Second}
	request, _ := http.NewRequest(http.MethodGet, "http://"+publicAddress+"/socket", nil)
	request.Header.Set("Origin", "https://client.example")
	websocketURL := "ws://" + publicAddress + "/socket"
	connection, response, err := dialer.Dial(websocketURL, request.Header)
	check(err)
	if response.StatusCode != http.StatusSwitchingProtocols || connection.Subprotocol() != "forms.v1" {
		panic("websocket plugin handshake result mismatch")
	}
	acceptedCookies := response.Header.Values("Set-Cookie")
	check(connection.WriteMessage(websocket.TextMessage, []byte("hello")))
	check(connection.WriteMessage(websocket.BinaryMessage, []byte{0, 1, 2, 255}))
	_, textResponse, err := connection.ReadMessage()
	check(err)
	_, binaryResponse, err := connection.ReadMessage()
	check(err)
	_, _, closeErr := connection.ReadMessage()
	if closeErr == nil {
		panic("expected plugin close after response messages")
	}
	_ = connection.Close()

	rejectedConnection, rejectedResponse, rejectedErr := dialer.Dial("ws://"+publicAddress+"/reject", request.Header)
	if rejectedErr == nil || rejectedConnection != nil || rejectedResponse == nil || rejectedResponse.StatusCode != http.StatusForbidden {
		panic("websocket plugin rejection was not returned before upgrade")
	}
	_ = rejectedResponse.Body.Close()
	rejectedCookies := rejectedResponse.Header.Values("Set-Cookie")
	badProtocolConnection, badProtocolResponse, badProtocolErr := dialer.Dial("ws://"+publicAddress+"/bad-subprotocol", request.Header)
	if badProtocolErr == nil || badProtocolConnection != nil || badProtocolResponse == nil || badProtocolResponse.StatusCode != http.StatusBadGateway {
		panic("server accepted a plugin subprotocol that the client did not offer")
	}
	_ = badProtocolResponse.Body.Close()
	badCookieConnection, badCookieResponse, badCookieErr := dialer.Dial("ws://"+publicAddress+"/bad-cookie", request.Header)
	if badCookieErr == nil || badCookieConnection != nil || badCookieResponse == nil || badCookieResponse.StatusCode != http.StatusBadGateway || len(badCookieResponse.Header.Values("Set-Cookie")) != 0 {
		panic("invalid cookie action was applied or upgraded the WebSocket")
	}
	_ = badCookieResponse.Body.Close()
	badCookieSetCookie := badCookieResponse.Header.Values("Set-Cookie")
	if badCookieSetCookie == nil {
		badCookieSetCookie = []string{}
	}
	oversizedConnection, oversizedResponse, oversizedDialErr := dialer.Dial("ws://"+publicAddress+"/oversized", request.Header)
	check(oversizedDialErr)
	if oversizedResponse.StatusCode != http.StatusSwitchingProtocols {
		panic("oversized-message route did not complete its WebSocket handshake")
	}
	check(oversizedConnection.WriteMessage(websocket.BinaryMessage, make([]byte, 1_048_577)))
	_, _, oversizedReadErr := oversizedConnection.ReadMessage()
	oversizedCloseErr, isCloseErr := oversizedReadErr.(*websocket.CloseError)
	if !isCloseErr || oversizedCloseErr.Code != websocket.CloseMessageTooBig {
		panic("oversized websocket message was not closed with the bounded-message status")
	}
	_ = oversizedConnection.Close()

	check(json.NewEncoder(os.Stdout).Encode(map[string]any{
		"accepted":           map[string]any{"status": response.StatusCode, "subprotocol": response.Header.Get("Sec-WebSocket-Protocol"), "setCookie": acceptedCookies, "received": received, "replies": []observation{{Type: "text", Data: string(textResponse)}, {Type: "binary", Data: base64.StdEncoding.EncodeToString(binaryResponse)}}},
		"rejected":           map[string]any{"status": rejectedResponse.StatusCode, "setCookie": rejectedCookies, "upgraded": false},
		"invalidSubprotocol": map[string]any{"status": badProtocolResponse.StatusCode, "upgraded": false},
		"invalidCookie":      map[string]any{"status": badCookieResponse.StatusCode, "setCookie": badCookieSetCookie, "upgraded": false},
		"oversizedMessage":   map[string]any{"closeCode": oversizedCloseErr.Code, "pluginReceivesOversizedMessage": false},
	}))
}

func send(stream peer.Stream, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return stream.Send(peer.Message{Payload: encoded})
}

func sendWSMessage(stream peer.Stream, messageType string, data []byte) error {
	for _, value := range []any{
		map[string]any{"kind": "websocket_message_start", "messageType": messageType},
		map[string]any{"kind": "websocket_message_chunk", "data": data},
		map[string]any{"kind": "websocket_message_end"},
	} {
		if err := send(stream, value); err != nil {
			return err
		}
	}
	return nil
}

func peerCredentials() (peer.SecurityConfig, peer.SecurityConfig) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)
	rootTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture root"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	check(err)
	root, err := x509.ParseCertificate(rootDER)
	check(err)
	roots := x509.NewCertPool()
	roots.AddCert(root)
	issue := func(serial int64, name, identity string) tls.Certificate {
		key, keyErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		check(keyErr)
		parsed, parseErr := url.Parse(identity)
		check(parseErr)
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name}, URIs: []*url.URL{parsed}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
		der, createErr := x509.CreateCertificate(rand.Reader, template, root, &key.PublicKey, rootKey)
		check(createErr)
		keyDER, marshalErr := x509.MarshalPKCS8PrivateKey(key)
		check(marshalErr)
		certificate, certErr := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
		check(certErr)
		return certificate
	}
	serverURI := "spiffe://liapoldus.test/forms"
	clientURI := "spiffe://liapoldus.test/server"
	return peer.SecurityConfig{Identity: serverURI, Certificate: issue(2, "forms", serverURI), Roots: roots, PeerIdentity: clientURI}, peer.SecurityConfig{Identity: clientURI, Certificate: issue(3, "server", clientURI), Roots: roots, PeerIdentity: serverURI}
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

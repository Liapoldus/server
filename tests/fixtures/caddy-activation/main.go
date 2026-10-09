package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	caddycore "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/certmagic"
	settingsapp "liapoldus.local/server-plugin/internal/application/settings"
	caddycertificatemodel "liapoldus.local/server-plugin/internal/domain/models/certificate"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

type input struct {
	ACMETransportRootPEM  string            `json:"acmeTransportRootPEM"`
	SiteContent           string            `json:"siteContent"`
	SiteFileContent       string            `json:"siteFileContent"`
	CertificateStatusHost string            `json:"certificateStatusHost"`
	ACMEDirectory         string            `json:"acmeDirectory"`
	ACMERootPEM           string            `json:"acmeRootPEM"`
	Settings              json.RawMessage   `json:"settings"`
	Candidate             json.RawMessage   `json:"candidateSettings"`
	Candidates            []json.RawMessage `json:"candidates"`
	Requests              []httpRequest     `json:"requests"`
	Port                  int               `json:"port"`
	HTTPChallengePort     int               `json:"httpChallengePort"`
	ForceRenew            bool              `json:"forceRenew"`
	SkipRequest           bool              `json:"skipRequest"`
}

type httpRequest struct {
	Target string `json:"target"`
	Host   string `json:"host"`
	Method string `json:"method"`
	Raw    bool   `json:"raw"`
}

type httpResponse struct {
	Body   string `json:"body"`
	Status int    `json:"status"`
}

func main() {
	contents, err := io.ReadAll(os.Stdin)
	check(err)
	var request input
	check(json.Unmarshal(contents, &request))
	_, cleanup, err := shared.IsolateCaddyDataHome()
	check(err)
	defer cleanup()
	var acmeRoots *x509.CertPool
	if request.ACMEDirectory != "" {
		acmeRoots = x509.NewCertPool()
		if !acmeRoots.AppendCertsFromPEM([]byte(request.ACMERootPEM)) || request.HTTPChallengePort < 1 {
			panic("invalid test ACME authority")
		}
		acmeTransportRoots := x509.NewCertPool()
		if !acmeTransportRoots.AppendCertsFromPEM([]byte(request.ACMETransportRootPEM)) {
			panic("invalid test ACME transport CA")
		}
		certmagic.DefaultACME.CA = request.ACMEDirectory
		certmagic.DefaultACME.TestCA = request.ACMEDirectory
		certmagic.DefaultACME.TrustedRoots = acmeTransportRoots
		certmagic.DefaultACME.Agreed = true
		certmagic.DefaultACME.AltHTTPPort = request.HTTPChallengePort
		certmagic.DefaultACME.DisableTLSALPNChallenge = true
		certmagic.Default.Storage = caddycore.DefaultStorage
	}

	runtime := caddyruntime.New()
	check(shared.RegisterSiteDirectoryReader())
	siteRoot, err := caddyruntime.SiteRoot("frontend")
	check(err)
	check(os.MkdirAll(siteRoot, 0o700))
	if request.SiteContent == "" {
		request.SiteContent = "plugin-owned"
	}
	check(os.WriteFile(filepath.Join(siteRoot, "index.html"), []byte(request.SiteContent), 0o600))
	if request.SiteFileContent == "" {
		request.SiteFileContent = "static-canonical"
	}
	siteFile := filepath.Join(siteRoot, "docs", "file.txt")
	check(os.MkdirAll(filepath.Dir(siteFile), 0o700))
	check(os.WriteFile(siteFile, []byte(request.SiteFileContent), 0o600))
	configuration, err := settingsapp.NewConfiguration(runtime)
	check(err)
	settings := request.Settings
	if len(settings) == 0 {
		settings, err = json.Marshal(map[string]any{
			"schemaVersion": 1,
			"config": map[string]any{
				"listeners": []any{map[string]any{
					"id": "web", "kind": "http", "address": fmt.Sprintf("127.0.0.1:%d", request.Port),
					"hostnames": []string{}, "protocols": []string{"http1"}, "tls": map[string]any{"mode": "disabled"},
				}},
				"routes": []any{map[string]any{
					"id": "site", "listenerId": "web", "handler": map[string]any{"type": "static", "siteId": "frontend"},
				}},
			},
		})
		check(err)
	}
	check(shared.Apply(configuration, settings, "revision-1", nil))
	defer func() {
		if configuration != nil {
			_ = configuration.Stop()
		}
	}()
	candidateCode := ""
	if len(request.Candidate) > 0 {
		candidateErr := shared.Apply(configuration, request.Candidate, "revision-2", nil)
		if candidateErr != nil {
			candidateCode = "InvalidArgument"
		}
	}
	candidateCodes := make([]string, 0, len(request.Candidates))
	for index, candidateSettings := range request.Candidates {
		candidateErr := shared.Apply(configuration, candidateSettings, fmt.Sprintf("candidate-%d", index+1), nil)
		if candidateErr != nil {
			candidateCodes = append(candidateCodes, "InvalidArgument")
		} else {
			candidateCodes = append(candidateCodes, "OK")
		}
	}
	requests := request.Requests
	if len(requests) == 0 && !request.SkipRequest {
		requests = []httpRequest{{Target: "/"}}
	}
	responses := make([]httpResponse, 0, len(requests))
	client := &http.Client{Timeout: 5 * time.Second}
	for _, item := range requests {
		response, requestErr := performRequest(client, request.Port, item)
		check(requestErr)
		responses = append(responses, response)
	}
	output := map[string]any{"revision": configuration.Revision()}
	if len(responses) > 0 {
		output["status"] = responses[0].Status
		output["body"] = responses[0].Body
	}
	if len(request.Candidate) > 0 {
		output["candidateCode"] = candidateCode
		output["revisionAfterCandidate"] = configuration.Revision()
	}
	if len(request.Candidates) > 0 {
		output["candidateCodes"] = candidateCodes
		output["revisionAfterCandidates"] = configuration.Revision()
	}
	if len(request.Requests) > 0 {
		output["responses"] = responses
		if len(responses) > 0 {
			delete(output, "status")
			delete(output, "body")
		}
	}
	if request.ACMEDirectory != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		before := waitForCertificate(ctx, runtime, request.CertificateStatusHost)
		cancel()
		output["certificateStatus"] = before
		if request.ForceRenew {
			magic := certmagic.NewDefault()
			check(magic.RenewCertSync(context.Background(), request.CertificateStatusHost, true))
			check(configuration.Stop())
			configuration = nil
			runtime = caddyruntime.New()
			configuration, err = settingsapp.NewConfiguration(runtime)
			check(err)
			check(shared.Apply(configuration, settings, "revision-1", nil))
			ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
			after := waitForCertificate(ctx, runtime, request.CertificateStatusHost)
			cancel()
			output["renewal"] = map[string]any{
				"completed":          true,
				"changedCertificate": before.Serial != nil && after.Serial != nil && *before.Serial != *after.Serial,
				"beforeSerial":       before.Serial,
				"afterSerial":        after.Serial,
			}
			output["certificateStatus"] = after
			response, probeErr := performTLSRequest(request.Port, request.CertificateStatusHost, acmeRoots)
			check(probeErr)
			output["tlsProbe"] = map[string]any{
				"status":     response.Status,
				"body":       response.Body,
				"serverName": request.CertificateStatusHost,
			}
		}
	}
	if request.CertificateStatusHost != "" {
		status, statusErr := runtime.CertificateStatus(context.Background(), request.CertificateStatusHost)
		check(statusErr)
		output["certificateStatus"] = status
	}
	check(json.NewEncoder(os.Stdout).Encode(output))
}

func waitForCertificate(ctx context.Context, runtime *caddyruntime.Runtime, host string) caddycertificatemodel.CertificateStatus {
	if host == "" {
		panic("certificate status host is required")
	}
	for {
		status, err := runtime.CertificateStatus(ctx, host)
		check(err)
		if status.Readiness == "ready" {
			return status
		}
		select {
		case <-ctx.Done():
			panic(errors.New("test ACME certificate did not become ready"))
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func performTLSRequest(port int, host string, roots *x509.CertPool) (httpResponse, error) {
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: host}}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d/", port), nil)
	if err != nil {
		return httpResponse{}, err
	}
	request.Host = host
	response, err := client.Do(request)
	if err != nil {
		return httpResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return httpResponse{}, err
	}
	return httpResponse{Status: response.StatusCode, Body: string(body)}, nil
}

func performRequest(client *http.Client, port int, request httpRequest) (httpResponse, error) {
	method := request.Method
	if method == "" {
		method = http.MethodGet
	}
	if request.Target == "" {
		request.Target = "/"
	}
	address := fmt.Sprintf("127.0.0.1:%d", port)
	if request.Raw {
		return performRawRequest(address, method, request.Target, request.Host)
	}
	clientRequest, err := http.NewRequest(method, "http://"+address+request.Target, nil)
	if err != nil {
		return httpResponse{}, err
	}
	if request.Host != "" {
		clientRequest.Host = request.Host
	}
	response, err := client.Do(clientRequest)
	if err != nil {
		return httpResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return httpResponse{}, err
	}
	return httpResponse{Status: response.StatusCode, Body: string(body)}, nil
}

func performRawRequest(address, method, target, host string) (httpResponse, error) {
	connection, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return httpResponse{}, err
	}
	defer connection.Close()
	if host == "" {
		host = "localhost"
	}
	if _, err := fmt.Fprintf(connection, "%s %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", method, target, host); err != nil {
		return httpResponse{}, err
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: method})
	if err != nil {
		return httpResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return httpResponse{}, err
	}
	return httpResponse{Status: response.StatusCode, Body: string(body)}, nil
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

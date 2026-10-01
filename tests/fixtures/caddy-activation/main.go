package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"liapoldus.local/server-plugin/internal/application"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

type input struct {
	Port            int               `json:"port"`
	Settings        json.RawMessage   `json:"settings"`
	Candidate       json.RawMessage   `json:"candidateSettings"`
	Candidates      []json.RawMessage `json:"candidates"`
	Requests        []httpRequest     `json:"requests"`
	SiteContent     string            `json:"siteContent"`
	SiteFileContent string            `json:"siteFileContent"`
}

type httpRequest struct {
	Target string `json:"target"`
	Host   string `json:"host"`
	Method string `json:"method"`
	Raw    bool   `json:"raw"`
}

type httpResponse struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

func main() {
	contents, err := io.ReadAll(os.Stdin)
	check(err)
	var request input
	check(json.Unmarshal(contents, &request))
	storageDirectory, err := os.MkdirTemp("", "liapoldus-caddy-activation-")
	check(err)
	defer func() { _ = os.RemoveAll(storageDirectory) }()
	check(os.Setenv("XDG_DATA_HOME", storageDirectory))

	runtime := caddyruntime.New()
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
	configuration, err := application.NewConfiguration(runtime)
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
	defer func() { _ = configuration.Stop() }()
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
	if len(requests) == 0 {
		requests = []httpRequest{{Target: "/"}}
	}
	responses := make([]httpResponse, 0, len(requests))
	client := &http.Client{Timeout: 5 * time.Second}
	for _, item := range requests {
		response, requestErr := performRequest(client, request.Port, item)
		check(requestErr)
		responses = append(responses, response)
	}
	output := map[string]any{
		"revision": configuration.Revision(), "status": responses[0].Status, "body": responses[0].Body,
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
		delete(output, "status")
		delete(output, "body")
	}
	check(json.NewEncoder(os.Stdout).Encode(output))
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

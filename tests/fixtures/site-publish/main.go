package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"liapoldus.local/server-plugin/contracts"
	settingsapp "liapoldus.local/server-plugin/internal/application/settings"
	siteapp "liapoldus.local/server-plugin/internal/application/site"
	sitemodel "liapoldus.local/server-plugin/internal/domain/models/site"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	"liapoldus.local/server-plugin/internal/infrastructure/site"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

type input struct {
	Requests []request `json:"requests"`
	Port     int       `json:"port"`
}

type request struct {
	Metadata       string `json:"metadata"`
	ContentType    string `json:"contentType"`
	IdempotencyKey string `json:"idempotencyKey"`
	Artifact       string `json:"artifact"`
}

type outcome struct {
	PreviousRevision *string `json:"previousRevision"`
	OperationID      string  `json:"operationId,omitempty"`
	State            string  `json:"state,omitempty"`
	Code             string  `json:"code,omitempty"`
	CurrentRevision  string  `json:"currentRevision,omitempty"`
	RootDocument     string  `json:"rootDocument,omitempty"`
	Accepted         bool    `json:"accepted"`
}

type report struct {
	PreviousRevision              *string    `json:"previousRevision"`
	AfterSecondPublish            outcome    `json:"afterSecondPublish"`
	BadDigest                     outcome    `json:"badDigest"`
	AfterRestart                  outcome    `json:"afterRestart"`
	Duplicate                     outcome    `json:"duplicate"`
	First                         outcome    `json:"first"`
	IdempotencyConflict           outcome    `json:"idempotencyConflict"`
	Second                        outcome    `json:"second"`
	RollbackRepeat                outcome    `json:"rollbackRepeat"`
	Rollback                      outcome    `json:"rollback"`
	DuplicateMetadata             outcome    `json:"duplicateMetadata"`
	StaleCAS                      outcome    `json:"staleCAS"`
	InvalidArchiveAccepted        outcome    `json:"invalidArchiveAccepted"`
	InvalidArchiveAfterProcessing outcome    `json:"invalidArchiveAfterProcessing"`
	PostSwitchRecovery            outcome    `json:"postSwitchRecovery"`
	StateAfterInvalidArchive      string     `json:"stateAfterInvalidArchive"`
	CurrentRevision               string     `json:"currentRevision,omitempty"`
	FinalState                    finalState `json:"finalState"`
	DuplicateEffectCount          int        `json:"duplicateEffectCount"`
	OrphanStagingRemoved          bool       `json:"orphanStagingRemoved"`
}

type finalState struct {
	PreviousRevision    *string `json:"previousRevision"`
	CurrentRevision     string  `json:"currentRevision"`
	RootDocument        string  `json:"rootDocument"`
	NestedAsset         string  `json:"nestedAsset"`
	DirectoryIndex      string  `json:"directoryIndex"`
	CaddyRootDocument   string  `json:"caddyRootDocument"`
	CaddyNestedAsset    string  `json:"caddyNestedAsset"`
	CaddyDirectoryIndex string  `json:"caddyDirectoryIndex"`
	CaddyManifestStatus int     `json:"caddyManifestStatus"`
	ManifestPublic      bool    `json:"manifestPublic"`
}

func main() {
	var request input
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		panic(err)
	}
	if len(request.Requests) != 8 {
		panic("fixture requires eight requests")
	}
	dataHome, cleanup, err := shared.IsolateCaddyDataHome()
	if err != nil {
		panic(err)
	}
	defer cleanup()
	root := filepath.Join(dataHome, "liapoldus")

	store, err := site.NewReleaseStore(root)
	if err != nil {
		panic(err)
	}
	publisher, err := siteapp.NewSitePublisher(store)
	if err != nil {
		panic(err)
	}
	var output report
	contract, err := contracts.LoadSiteOperationContract()
	if err != nil {
		panic(err)
	}
	output.First = accept(publisher, request.Requests[0])
	if !output.First.Accepted {
		panic("initial site publish was rejected")
	}
	orphanStaging := filepath.Join(root, "sites", "docs", "staging", "orphaned-before-journal")
	if err := os.MkdirAll(orphanStaging, 0o700); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(orphanStaging, "partial"), []byte("incomplete"), 0o600); err != nil {
		panic(err)
	}

	// Simulate a process crash after durable acceptance but before activation.
	store, err = site.NewReleaseStore(root)
	if err != nil {
		panic(err)
	}
	publisher, err = siteapp.NewSitePublisher(store)
	if err != nil {
		panic(err)
	}
	if err := publisher.ProcessPending(context.Background()); err != nil {
		panic(err)
	}
	operation, err := publisher.Operation(context.Background(), output.First.OperationID)
	if err != nil {
		panic(err)
	}
	state, err := publisher.State(context.Background(), "docs")
	if err != nil {
		panic(err)
	}
	output.AfterRestart = outcome{Accepted: operation.State == contract.CompletedState, OperationID: operation.OperationID, State: string(operation.State), CurrentRevision: state.CurrentRevision, PreviousRevision: state.PreviousRevision}
	firstRevision := operation.RevisionID
	output.CurrentRevision = state.CurrentRevision
	output.PreviousRevision = state.PreviousRevision
	_, orphanErr := os.Stat(orphanStaging)
	output.OrphanStagingRemoved = errors.Is(orphanErr, os.ErrNotExist)

	// Simulate a crash after the active pointer became durable but before the
	// operation's terminal state reached its journal file.
	operationPath := filepath.Join(root, "operations", operation.OperationID+".json")
	operationBytes, err := os.ReadFile(operationPath)
	if err != nil {
		panic(err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(operationBytes, &persisted); err != nil {
		panic(err)
	}
	persisted["state"] = contract.RunningState
	operationBytes, err = json.Marshal(persisted)
	if err != nil || os.WriteFile(operationPath, operationBytes, 0o600) != nil {
		panic("could not simulate post-pointer crash")
	}
	store, err = site.NewReleaseStore(root)
	if err != nil {
		panic(err)
	}
	publisher, err = siteapp.NewSitePublisher(store)
	if err != nil || publisher.ProcessPending(context.Background()) != nil {
		panic("post-pointer crash recovery failed")
	}
	operation, err = publisher.Operation(context.Background(), output.First.OperationID)
	if err != nil {
		panic(err)
	}
	state, err = publisher.State(context.Background(), "docs")
	if err != nil {
		panic(err)
	}
	output.PostSwitchRecovery = outcome{Accepted: operation.State == contract.CompletedState, OperationID: operation.OperationID, State: string(operation.State), CurrentRevision: state.CurrentRevision, PreviousRevision: state.PreviousRevision}

	output.Duplicate = accept(publisher, request.Requests[1])
	output.DuplicateEffectCount = 1
	output.IdempotencyConflict = accept(publisher, request.Requests[2])
	output.Second = accept(publisher, request.Requests[3])
	if output.Second.Accepted {
		if err := publisher.ProcessPending(context.Background()); err != nil {
			panic(err)
		}
		operation, err = publisher.Operation(context.Background(), output.Second.OperationID)
		if err != nil {
			panic(err)
		}
		state, err = publisher.State(context.Background(), "docs")
		if err != nil {
			panic(err)
		}
		output.AfterSecondPublish = outcome{Accepted: operation.State == contract.CompletedState, OperationID: operation.OperationID, State: string(operation.State), CurrentRevision: state.CurrentRevision, PreviousRevision: state.PreviousRevision}
	}
	secondOperation, err := publisher.Operation(context.Background(), output.Second.OperationID)
	if err != nil {
		panic(err)
	}
	secondRevision := secondOperation.RevisionID
	output.StaleCAS = accept(publisher, request.Requests[4])
	output.BadDigest = accept(publisher, request.Requests[5])
	output.DuplicateMetadata = accept(publisher, request.Requests[6])
	output.InvalidArchiveAccepted = accept(publisher, request.Requests[7])
	if output.InvalidArchiveAccepted.Accepted {
		store, err = site.NewReleaseStore(root)
		if err != nil {
			panic(err)
		}
		publisher, err = siteapp.NewSitePublisher(store)
		if err != nil {
			panic(err)
		}
		if err := publisher.ProcessPending(context.Background()); err != nil {
			panic(err)
		}
		invalidArchiveOperation, err := publisher.Operation(context.Background(), output.InvalidArchiveAccepted.OperationID)
		if err != nil {
			panic(err)
		}
		output.InvalidArchiveAfterProcessing = outcome{State: string(invalidArchiveOperation.State), Code: invalidArchiveOperation.ErrorCode}
	}
	state, err = publisher.State(context.Background(), "docs")
	if err != nil {
		panic(err)
	}
	output.FinalState.CurrentRevision = state.CurrentRevision
	output.StateAfterInvalidArchive = state.CurrentRevision
	output.FinalState.PreviousRevision = state.PreviousRevision
	output.FinalState.RootDocument, err = readDocument(publisher, "docs", "/")
	if err != nil {
		panic(err)
	}
	output.FinalState.NestedAsset, err = readDocument(publisher, "docs", "/assets/logo.txt")
	if err != nil {
		panic(err)
	}
	output.FinalState.DirectoryIndex, err = readDocument(publisher, "docs", "/assets")
	if err != nil {
		panic(err)
	}
	_, err = publisher.OpenSiteDocument(context.Background(), "docs", "/site-manifest.json")
	output.FinalState.ManifestPublic = err == nil
	if !errors.Is(err, siteapp.ErrDocumentNotFound) {
		panic("manifest was not hidden using the product not-found result")
	}
	output.FinalState.CaddyRootDocument, output.FinalState.CaddyNestedAsset, output.FinalState.CaddyDirectoryIndex, output.FinalState.CaddyManifestStatus = servePublishedSite(root, request.Port)
	rollbackOperation, err := publisher.Rollback(context.Background(), sitemodel.SiteRollbackInput{
		SiteID: "docs", ExpectedCurrentRevision: secondRevision, TargetRevision: firstRevision,
		IdempotencyKey: "rollback-one", Capability: contract.RollbackCapability,
	})
	if err != nil || publisher.ProcessPending(context.Background()) != nil {
		panic("site rollback failed")
	}
	rollbackOperation, err = publisher.Operation(context.Background(), rollbackOperation.OperationID)
	if err != nil {
		panic(err)
	}
	rollbackState, err := publisher.State(context.Background(), "docs")
	if err != nil {
		panic(err)
	}
	rollbackDocument, err := readDocument(publisher, "docs", "/")
	if err != nil {
		panic(err)
	}
	output.Rollback = outcome{Accepted: rollbackOperation.State == contract.CompletedState, OperationID: rollbackOperation.OperationID, State: string(rollbackOperation.State), CurrentRevision: rollbackState.CurrentRevision, PreviousRevision: rollbackState.PreviousRevision, RootDocument: rollbackDocument}
	_, repeatErr := publisher.Rollback(context.Background(), sitemodel.SiteRollbackInput{
		SiteID: "docs", ExpectedCurrentRevision: secondRevision, TargetRevision: firstRevision,
		IdempotencyKey: "rollback-one", Capability: contract.RollbackCapability,
	})
	if errors.Is(repeatErr, sitemodel.ErrSiteConflict) {
		output.RollbackRepeat = outcome{Code: "conflict"}
	} else if repeatErr == nil {
		output.RollbackRepeat = outcome{Code: "unexpected_success"}
	} else {
		output.RollbackRepeat = outcome{Code: "unexpected_error"}
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		panic(err)
	}
}

func servePublishedSite(root string, port int) (string, string, string, int) {
	if port < 1 {
		panic("fixture requires a listening port")
	}
	store, err := site.NewReleaseStore(root)
	if err != nil {
		panic(err)
	}
	if err := caddyruntime.SetSiteDocumentReader(store); err != nil {
		panic(err)
	}
	runtime := caddyruntime.New()
	configuration, err := settingsapp.NewConfiguration(runtime)
	if err != nil {
		panic(err)
	}
	defer func() { _ = configuration.Stop() }()
	settings, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"listeners": []any{map[string]any{
				"id": "web", "kind": "http", "address": net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
				"hostnames": []string{}, "protocols": []string{"http1"}, "tls": map[string]any{"mode": "disabled"},
			}},
			"routes": []any{map[string]any{
				"id": "site", "listenerId": "web", "handler": map[string]any{"type": "static", "siteId": "docs"},
			}},
		},
	})
	if err != nil || shared.Apply(configuration, settings, "site-published-generation", nil) != nil {
		panic("Caddy failed to activate the published site")
	}
	read := func(target string) (int, string) {
		response, err := http.Get("http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + target)
		if err != nil {
			panic(err)
		}
		defer response.Body.Close()
		contents, err := io.ReadAll(response.Body)
		if err != nil {
			panic(err)
		}
		return response.StatusCode, string(contents)
	}
	_, rootBody := read("/")
	_, assetBody := read("/assets/logo.txt")
	_, directoryIndex := read("/assets")
	manifestStatus, _ := read("/site-manifest.json")
	return rootBody, assetBody, directoryIndex, manifestStatus
}

func accept(publisher *siteapp.SitePublisher, request request) outcome {
	artifact, err := base64.StdEncoding.DecodeString(request.Artifact)
	if err != nil {
		panic(err)
	}
	operation, err := publisher.Accept(context.Background(), siteapp.SitePublishInput{
		Metadata: []byte(request.Metadata), ContentType: request.ContentType,
		IdempotencyKey: request.IdempotencyKey, Body: bytes.NewReader(artifact),
	})
	if err != nil {
		return outcome{Code: siteapp.ErrorCode(err)}
	}
	return outcome{Accepted: true, OperationID: operation.OperationID, State: string(operation.State)}
}

func readDocument(publisher *siteapp.SitePublisher, siteID, requestPath string) (string, error) {
	document, err := publisher.OpenSiteDocument(context.Background(), siteID, requestPath)
	if err != nil {
		return "", err
	}
	defer document.Close()
	contents, err := io.ReadAll(document)
	return string(contents), err
}

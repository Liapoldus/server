package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	siteapp "liapoldus.local/server-plugin/internal/application/site"
	sitemodel "liapoldus.local/server-plugin/internal/domain/models/site"
	"liapoldus.local/server-plugin/internal/infrastructure/site"
)

type outcome struct {
	State       string `json:"state"`
	OperationID string `json:"operationId,omitempty"`
}

type report struct {
	Accepted       outcome          `json:"accepted"`
	ExpectedDigest string           `json:"expectedDigest"`
	RecoveredBy    []recoveredState `json:"recoveredBy"`
	ProcessKilled  bool             `json:"acceptingProcessKilled"`
}

type recoveredState struct {
	OperationState  string `json:"operationState"`
	CurrentRevision string `json:"currentRevision"`
	ServedBody      string `json:"servedBody"`
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == "accept" {
		if err := accept(os.Args[2]); err != nil {
			fail(err)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "accept-and-wait" {
		if err := accept(os.Args[2]); err != nil {
			fail(err)
		}
		select {}
	}
	if len(os.Args) == 6 && os.Args[1] == "recover" {
		if err := recoverRelease(os.Args[2], os.Args[3], os.Args[4], os.Args[5]); err != nil {
			fail(err)
		}
		return
	}
	if err := run(); err != nil {
		fail(err)
	}
}

func run() error {
	root, err := os.MkdirTemp("", "liapoldus-cross-process-recovery-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	storeRoot := filepath.Join(root, "store")
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	command := exec.Command(executable, "accept-and-wait", storeRoot)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	acceptedOutput, readErr := bufio.NewReader(stdout).ReadBytes('\n')
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return readErr
	}
	var accepted outcome
	if err := json.Unmarshal(acceptedOutput, &accepted); err != nil || accepted.State != "accepted" || accepted.OperationID == "" {
		_ = command.Process.Kill()
		_ = command.Wait()
		return errors.New("accepting process did not persist a valid operation")
	}
	if err := command.Process.Kill(); err != nil {
		_ = command.Wait()
		return err
	}
	if err := command.Wait(); err == nil {
		return errors.New("accepting process was expected to be killed")
	}
	recoveryBarrier := filepath.Join(root, "recovery-barrier")
	if err := os.Mkdir(recoveryBarrier, 0o700); err != nil {
		return err
	}
	type recoveryResult struct {
		err   error
		state recoveredState
	}
	results := make(chan recoveryResult, 2)
	recoveryContext, cancelRecovery := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelRecovery()
	for index, replicaID := range []string{"replica-a", "replica-b"} {
		go func(index int, replicaID string) {
			command := exec.CommandContext(recoveryContext, executable, "recover", storeRoot, accepted.OperationID, recoveryBarrier, replicaID)
			output, err := command.Output()
			if err != nil {
				results <- recoveryResult{err: err}
				return
			}
			var state recoveredState
			if err := json.Unmarshal(output, &state); err != nil {
				results <- recoveryResult{err: err}
				return
			}
			results <- recoveryResult{state: state}
		}(index, replicaID)
	}
	result := report{Accepted: accepted, ProcessKilled: true, RecoveredBy: make([]recoveredState, 0, 2)}
	for range 2 {
		recovery := <-results
		if recovery.err != nil {
			return recovery.err
		}
		result.RecoveredBy = append(result.RecoveredBy, recovery.state)
	}
	artifact := validArchive()
	digest := sha256.Sum256(artifact)
	result.ExpectedDigest = hex.EncodeToString(digest[:])
	return json.NewEncoder(os.Stdout).Encode(result)
}

func accept(root string) error {
	store, err := site.NewReleaseStore(root)
	if err != nil {
		return err
	}
	publisher, err := siteapp.NewSitePublisher(store)
	if err != nil {
		return err
	}
	artifact := validArchive()
	digest := sha256.Sum256(artifact)
	metadata, err := json.Marshal(map[string]any{
		"version": 1,
		"artifact": map[string]any{
			"mediaType":  "application/gzip",
			"byteLength": len(artifact),
			"sha256":     "sha256:" + hex.EncodeToString(digest[:]),
		},
		"payload": map[string]any{"siteId": "cross-process-site"},
	})
	if err != nil {
		return err
	}
	operation, err := publisher.Accept(context.Background(), sitemodel.SitePublishInput{
		Metadata: metadata, ContentType: "application/gzip", IdempotencyKey: "publish-before-exit", Body: bytes.NewReader(artifact),
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(outcome{State: string(operation.State), OperationID: operation.OperationID})
}

func recoverRelease(root, operationID, barrier, replicaID string) error {
	if err := awaitRecoveryStart(barrier, replicaID); err != nil {
		return err
	}
	store, err := site.NewReleaseStore(root)
	if err != nil {
		return err
	}
	publisher, err := siteapp.NewSitePublisher(store)
	if err != nil {
		return err
	}
	if err := publisher.ProcessPending(context.Background()); err != nil {
		return err
	}
	operation, err := publisher.Operation(context.Background(), operationID)
	if err != nil {
		return err
	}
	state, err := publisher.State(context.Background(), "cross-process-site")
	if err != nil {
		return err
	}
	reader, err := publisher.OpenSiteDocument(context.Background(), "cross-process-site", "/index.html")
	if err != nil {
		return err
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(recoveredState{string(operation.State), state.CurrentRevision, string(body)})
}

func awaitRecoveryStart(barrier, replicaID string) error {
	if err := os.WriteFile(filepath.Join(barrier, replicaID), nil, 0o600); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(barrier)
		if err != nil {
			return err
		}
		if len(entries) == 2 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("recovery replicas did not reach the start barrier")
}

func validArchive() []byte {
	var compressed bytes.Buffer
	zipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(zipWriter)
	entries := []struct {
		name string
		data []byte
	}{
		{"site-manifest.json", []byte(`{"schemaVersion":1,"siteId":"cross-process-site","documentRoot":"public","indexDocument":"index.html"}`)},
		{"public/index.html", []byte("recovered-after-process-exit")},
	}
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: 0o644, Size: int64(len(entry.data)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}
		if err := tarWriter.WriteHeader(header); err != nil {
			panic(err)
		}
		if _, err := tarWriter.Write(entry.data); err != nil {
			panic(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		panic(err)
	}
	if err := zipWriter.Close(); err != nil {
		panic(err)
	}
	return compressed.Bytes()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

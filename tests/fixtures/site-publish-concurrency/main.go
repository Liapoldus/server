package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	siteapp "liapoldus.local/server-plugin/internal/application/site"
	sitemodel "liapoldus.local/server-plugin/internal/domain/models/site"
	"liapoldus.local/server-plugin/internal/infrastructure/site"
)

type outcome struct {
	Code     string `json:"code,omitempty"`
	Accepted bool   `json:"accepted"`
}

type report struct {
	Outcomes []outcome `json:"outcomes"`
}

func main() {
	if len(os.Args) == 5 && os.Args[1] == "worker" {
		if err := runWorker(os.Args[2], os.Args[3], os.Args[4]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := runRace(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runRace() error {
	root, err := os.MkdirTemp("", "liapoldus-site-publish-race-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	barrier := filepath.Join(root, "barrier")
	if err := os.Mkdir(barrier, 0o700); err != nil {
		return err
	}
	storeRoot := filepath.Join(root, "store")
	executable, err := os.Executable()
	if err != nil {
		return err
	}

	commands := make([]*exec.Cmd, 0, 2)
	outputs := make([]bytes.Buffer, 0, 2)
	for _, workerID := range []string{"one", "two"} {
		command := exec.Command(executable, "worker", storeRoot, barrier, workerID)
		command.Env = os.Environ()
		commands = append(commands, command)
		outputs = append(outputs, bytes.Buffer{})
		command.Stdout = &outputs[len(outputs)-1]
		command.Stderr = os.Stderr
	}
	for _, command := range commands {
		if err := command.Start(); err != nil {
			for _, started := range commands {
				if started.Process != nil {
					_ = started.Process.Kill()
				}
			}
			return err
		}
	}
	if err := awaitWorkers(barrier); err != nil {
		for _, command := range commands {
			if command.Process != nil {
				_ = command.Process.Kill()
			}
		}
		for _, command := range commands {
			if command.Process != nil {
				_ = command.Wait()
			}
		}
		return err
	}
	if err := os.WriteFile(filepath.Join(barrier, "start"), nil, 0o600); err != nil {
		return err
	}

	var waitGroup sync.WaitGroup
	waitErrors := make(chan error, len(commands))
	for _, command := range commands {
		waitGroup.Add(1)
		go func(command *exec.Cmd) {
			defer waitGroup.Done()
			waitErrors <- command.Wait()
		}(command)
	}
	done := make(chan struct{})
	go func() {
		waitGroup.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		for _, command := range commands {
			if command.Process != nil {
				_ = command.Process.Kill()
			}
		}
		return errors.New("cross-process publish fixture timed out")
	}
	close(waitErrors)
	for waitErr := range waitErrors {
		if waitErr != nil {
			return waitErr
		}
	}

	result := report{Outcomes: make([]outcome, 0, len(outputs))}
	for _, output := range outputs {
		var value outcome
		if err := json.Unmarshal(output.Bytes(), &value); err != nil {
			return err
		}
		result.Outcomes = append(result.Outcomes, value)
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func runWorker(storeRoot, barrier, workerID string) error {
	if err := waitForStart(barrier, workerID); err != nil {
		return err
	}
	store, err := site.NewReleaseStore(storeRoot)
	if err != nil {
		return err
	}
	publisher, err := siteapp.NewSitePublisher(store)
	if err != nil {
		return err
	}
	body := []byte("artifact-" + workerID)
	digest := sha256.Sum256(body)
	metadata, err := json.Marshal(map[string]any{
		"version": 1,
		"artifact": map[string]any{
			"mediaType":  "application/gzip",
			"byteLength": len(body),
			"sha256":     "sha256:" + hex.EncodeToString(digest[:]),
		},
		"payload": map[string]any{"siteId": "shared-site"},
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, publishErr := publisher.Accept(ctx, sitemodel.SitePublishInput{
		Metadata: metadata, ContentType: "application/gzip", IdempotencyKey: "publish-" + workerID, Body: bytes.NewReader(body),
	})
	value := outcome{Accepted: publishErr == nil}
	if publishErr != nil {
		value.Code = siteapp.ErrorCode(publishErr)
	}
	return json.NewEncoder(os.Stdout).Encode(value)
}

func awaitWorkers(barrier string) error {
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
	return errors.New("publish workers did not reach the start barrier")
}

func waitForStart(barrier, workerID string) error {
	if err := os.WriteFile(filepath.Join(barrier, workerID), nil, 0o600); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(barrier, "start")); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("publish start barrier was not released")
}

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"liapoldus.local/server-plugin/internal/infrastructure/site"
)

type testInput struct {
	Cases []testCase `json:"cases"`
}
type testCase struct {
	Name        string   `json:"name"`
	Artifact    string   `json:"artifact"`
	SiteID      string   `json:"siteId"`
	Preexisting []string `json:"preexisting"`
}
type testOutput struct {
	Manifest        any      `json:"manifest,omitempty"`
	Name            string   `json:"name"`
	Digest          string   `json:"digest,omitempty"`
	Files           []string `json:"files"`
	CompressedBytes int64    `json:"compressedBytes,omitempty"`
	ExpandedBytes   int64    `json:"expandedBytes,omitempty"`
	Accepted        bool     `json:"accepted"`
}

func main() {
	var input testInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		panic(err)
	}
	outputs := make([]testOutput, 0, len(input.Cases))
	for _, candidate := range input.Cases {
		outputs = append(outputs, run(candidate))
	}
	if err := json.NewEncoder(os.Stdout).Encode(outputs); err != nil {
		panic(err)
	}
}

func run(candidate testCase) testOutput {
	output := testOutput{Name: candidate.Name}
	root, err := os.MkdirTemp("", "server-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)
	for _, name := range candidate.Preexisting {
		if err := os.WriteFile(filepath.Join(root, name), []byte("preserve"), 0o600); err != nil {
			panic(err)
		}
	}
	artifact, err := base64.StdEncoding.DecodeString(candidate.Artifact)
	if err != nil {
		panic(err)
	}
	result, err := site.ExtractAndValidateArchive(bytes.NewReader(artifact), root, candidate.SiteID)
	output.Accepted = err == nil
	output.Files = list(root)
	if err == nil {
		output.Digest = result.Digest
		output.CompressedBytes = result.CompressedBytes
		output.ExpandedBytes = result.ExpandedBytes
		output.Manifest = result.Manifest
	}
	return output
}

func list(root string) []string {
	result := make([]string, 0)
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if current == root {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		result = append(result, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		panic(err)
	}
	sort.Strings(result)
	return result
}

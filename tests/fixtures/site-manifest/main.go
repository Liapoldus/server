package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"liapoldus.local/server-plugin/contracts"
	"liapoldus.local/server-plugin/internal/infrastructure/site"
)

type stagedEntry struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
	Target  string `json:"target"`
}

type testCase struct {
	Name          string         `json:"name"`
	PayloadSiteID string         `json:"payloadSiteId"`
	Manifest      map[string]any `json:"manifest"`
	ManifestJSON  string         `json:"manifestJSON"`
	Entries       []stagedEntry  `json:"entries"`
}

type request struct {
	Cases []testCase `json:"cases"`
}

type result struct {
	Manifest *site.Manifest `json:"manifest,omitempty"`
	Name     string         `json:"name"`
	Accepted bool           `json:"accepted"`
}

func main() {
	input, err := io.ReadAll(os.Stdin)
	check(err)
	var request request
	check(json.Unmarshal(input, &request))
	results := make([]result, 0, len(request.Cases))
	for _, candidate := range request.Cases {
		root, err := os.MkdirTemp("", "server-")
		check(err)
		manifestName, err := contracts.SiteManifestArchiveEntry()
		check(err)
		manifestBytes := []byte(candidate.ManifestJSON)
		if candidate.ManifestJSON == "" && candidate.Manifest != nil {
			manifestBytes, err = json.Marshal(candidate.Manifest)
			check(err)
		}
		if candidate.ManifestJSON != "" || candidate.Manifest != nil {
			check(os.WriteFile(filepath.Join(root, manifestName), manifestBytes, 0o600))
		}
		for _, entry := range candidate.Entries {
			path := filepath.Join(root, filepath.FromSlash(entry.Path))
			switch entry.Kind {
			case "directory":
				check(os.MkdirAll(path, 0o700))
			case "file":
				check(os.MkdirAll(filepath.Dir(path), 0o700))
				check(os.WriteFile(path, []byte(entry.Content), 0o600))
			case "symlink":
				check(os.MkdirAll(filepath.Dir(path), 0o700))
				check(os.Symlink(entry.Target, path))
			default:
				panic("invalid test fixture entry")
			}
		}
		manifest, validationErr := site.ValidateStagedManifest(root, candidate.PayloadSiteID)
		item := result{Name: candidate.Name, Accepted: validationErr == nil}
		if validationErr == nil {
			item.Manifest = &manifest
		}
		results = append(results, item)
		check(os.RemoveAll(root))
	}
	check(json.NewEncoder(os.Stdout).Encode(results))
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

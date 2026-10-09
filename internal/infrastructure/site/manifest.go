package site

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"liapoldus.local/server-plugin/contracts"
)

var ErrInvalidManifest = errors.New("invalid site manifest")

type Manifest struct {
	SiteID        string `json:"siteId"`
	DocumentRoot  string `json:"documentRoot"`
	IndexDocument string `json:"indexDocument"`
	SchemaVersion int    `json:"schemaVersion"`
}

// ValidateStagedManifest validates the root manifest and its document-root/index
// targets after an archive has been safely extracted into an isolated staging
// directory. It does not parse or validate the source tar/gzip archive.
func ValidateStagedManifest(stagedRoot, expectedSiteID string) (Manifest, error) {
	if stagedRoot == "" || expectedSiteID == "" || !utf8.ValidString(expectedSiteID) {
		return Manifest{}, ErrInvalidManifest
	}
	rootInfo, err := os.Lstat(stagedRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return Manifest{}, ErrInvalidManifest
	}

	manifestFileName, err := contracts.SiteManifestArchiveEntry()
	if err != nil {
		return Manifest{}, ErrInvalidManifest
	}
	manifestPath := filepath.Join(stagedRoot, manifestFileName)
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil || !manifestInfo.Mode().IsRegular() {
		return Manifest{}, ErrInvalidManifest
	}
	contents, err := os.ReadFile(manifestPath)
	if err != nil || !utf8.Valid(contents) || !contracts.ValidJSONNoDuplicateKeys(contents) {
		return Manifest{}, ErrInvalidManifest
	}
	if err := contracts.ValidateSiteManifest(contents); err != nil {
		return Manifest{}, ErrInvalidManifest
	}

	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, ErrInvalidManifest
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Manifest{}, ErrInvalidManifest
	}
	if manifest.SiteID != expectedSiteID ||
		!safeManifestPath(manifest.DocumentRoot, true) || !safeManifestPath(manifest.IndexDocument, false) {
		return Manifest{}, ErrInvalidManifest
	}
	if manifest.DocumentRoot == "." && manifest.IndexDocument == manifestFileName {
		return Manifest{}, ErrInvalidManifest
	}

	documentRoot := stagedRoot
	if manifest.DocumentRoot != "." {
		documentRoot = filepath.Join(stagedRoot, filepath.FromSlash(manifest.DocumentRoot))
		if err := requirePath(stagedRoot, manifest.DocumentRoot, true); err != nil {
			return Manifest{}, ErrInvalidManifest
		}
	}
	indexRelativePath := manifest.IndexDocument
	if manifest.DocumentRoot != "." {
		indexRelativePath = manifest.DocumentRoot + "/" + manifest.IndexDocument
	}
	if err := requirePath(stagedRoot, indexRelativePath, false); err != nil {
		return Manifest{}, ErrInvalidManifest
	}
	indexInfo, err := os.Lstat(filepath.Join(documentRoot, filepath.FromSlash(manifest.IndexDocument)))
	if err != nil || !indexInfo.Mode().IsRegular() {
		return Manifest{}, ErrInvalidManifest
	}
	return manifest, nil
}

func safeManifestPath(value string, allowRoot bool) bool {
	if !utf8.ValidString(value) || value == "" {
		return false
	}
	if allowRoot && value == "." {
		return true
	}
	if strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.Contains(value, `\`) {
		return false
	}
	for _, r := range value {
		if r <= 0x1f || r == 0x7f {
			return false
		}
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func requirePath(root, relative string, directory bool) error {
	parts := strings.Split(relative, "/")
	current := root
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalidManifest
		}
		last := index == len(parts)-1
		if !last && !info.IsDir() {
			return ErrInvalidManifest
		}
		if last && directory && !info.IsDir() {
			return ErrInvalidManifest
		}
		if last && !directory && !info.Mode().IsRegular() {
			return ErrInvalidManifest
		}
	}
	return nil
}

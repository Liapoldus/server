package shared

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	sitemodel "liapoldus.local/server-service/internal/domain/models/site"
	"liapoldus.local/server-service/internal/infrastructure/site"
)

func FixtureSiteArchive(siteID, content string) ([]byte, error) {
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(compressed)
	manifest, err := json.Marshal(map[string]any{"schemaVersion": 1, "siteId": siteID, "documentRoot": "public", "indexDocument": "index.html"})
	if err != nil {
		return nil, err
	}
	for _, entry := range []struct {
		name string
		body []byte
	}{{"site-manifest.json", manifest}, {"public/index.html", []byte(content)}} {
		if err := archive.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o600, Size: int64(len(entry.body)), Typeflag: tar.TypeReg}); err != nil {
			return nil, err
		}
		if _, err := archive.Write(entry.body); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	if err := compressed.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// PublishFixtureSite creates a real immutable release before a fixture applies
// settings that pin it. Production validation must not accept a made-up digest.
func PublishFixtureSite(ctx context.Context, siteDataRoot, siteID string, archive []byte) (string, error) {
	if err := os.MkdirAll(filepath.Clean(siteDataRoot), 0o700); err != nil {
		return "", fmt.Errorf("create fixture site root: %w", err)
	}
	digest := sha256.Sum256(archive)
	revision := "sha256:" + hex.EncodeToString(digest[:])
	metadata, err := json.Marshal(map[string]any{
		"version":  1,
		"artifact": map[string]any{"mediaType": "application/gzip", "byteLength": len(archive), "sha256": revision},
		"payload":  map[string]any{"siteId": siteID},
	})
	if err != nil {
		return "", fmt.Errorf("encode fixture metadata: %w", err)
	}
	repository, err := site.NewReleaseStore(filepath.Clean(siteDataRoot))
	if err != nil {
		return "", fmt.Errorf("load fixture release store: %w", err)
	}
	operation, err := repository.AcceptSitePublish(ctx, sitemodel.SitePublishInput{
		Body: io.NopCloser(bytes.NewReader(archive)), ContentType: "application/gzip",
		IdempotencyKey: fmt.Sprintf("fixture-%s", siteID), Metadata: metadata,
	})
	if err != nil {
		return "", fmt.Errorf("accept fixture release: %w", err)
	}
	if err := repository.ProcessPendingSitePublishes(ctx); err != nil {
		return "", fmt.Errorf("process fixture release: %w", err)
	}
	return operation.RevisionID, nil
}

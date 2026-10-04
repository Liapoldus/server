package shared

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
)

// RegisterSiteDirectoryReader keeps legacy Caddy behavior fixtures independent
// from the product release-store fixture. Product acceptance uses ReleaseStore.
func RegisterSiteDirectoryReader() error {
	return caddyruntime.SetSiteDocumentReader(siteDirectoryReader{})
}

type siteDirectoryReader struct{}

func (siteDirectoryReader) OpenSiteDocument(_ context.Context, siteID, requestPath string) (io.ReadCloser, error) {
	root, err := caddyruntime.SiteRoot(siteID)
	if err != nil || !strings.HasPrefix(requestPath, "/") || strings.Contains(requestPath, `\`) {
		return nil, errors.New("")
	}
	clean := strings.TrimPrefix(path.Clean(requestPath), "/")
	if clean == "" || clean == "." {
		clean = "index.html"
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return nil, errors.New("")
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	file, err := rootHandle.Open(filepath.FromSlash(clean))
	if err == nil {
		if info, statErr := file.Stat(); statErr == nil && info.IsDir() {
			_ = file.Close()
			file, err = rootHandle.Open(filepath.Join(filepath.FromSlash(clean), "index.html"))
		}
	}
	_ = rootHandle.Close()
	if err != nil {
		return nil, errors.New("")
	}
	if info, statErr := file.Stat(); statErr != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("")
	}
	return file, nil
}

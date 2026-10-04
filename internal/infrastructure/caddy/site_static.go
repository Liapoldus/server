package caddy

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"

	caddycore "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"liapoldus.local/server-plugin/internal/domain/models"
)

const siteFileServerModule = "liapoldus_site_file_server"

type SiteDocumentReader interface {
	OpenSiteDocument(context.Context, string, string) (io.ReadCloser, error)
}

type siteDocumentFile interface {
	io.ReadCloser
	io.ReadSeeker
	Stat() (os.FileInfo, error)
}

type siteDocumentReaderHolder struct {
	reader SiteDocumentReader
}

var activeSiteDocumentReader atomic.Pointer[siteDocumentReaderHolder]

// SetSiteDocumentReader connects Caddy's static handler to the product-owned
// immutable release store. It is called by the Server composition root before
// any configuration containing a static handler is activated.
func SetSiteDocumentReader(reader SiteDocumentReader) error {
	if reader == nil {
		return models.ErrInvalidSitePublisher
	}
	activeSiteDocumentReader.Store(&siteDocumentReaderHolder{reader: reader})
	return nil
}

type siteFileServer struct {
	SiteID string `json:"siteId"`
}

func init() {
	caddycore.RegisterModule(siteFileServer{})
}

func (siteFileServer) CaddyModule() caddycore.ModuleInfo {
	return caddycore.ModuleInfo{
		ID:  caddycore.ModuleID("http.handlers." + siteFileServerModule),
		New: func() caddycore.Module { return new(siteFileServer) },
	}
}

func (handler *siteFileServer) Provision(caddycore.Context) error {
	if _, err := SiteRoot(handler.SiteID); err != nil {
		return errUnsupportedSettings
	}
	return nil
}

func (handler *siteFileServer) ServeHTTP(writer http.ResponseWriter, request *http.Request, _ caddyhttp.Handler) error {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return caddyhttp.Error(http.StatusMethodNotAllowed, errors.New(""))
	}
	holder := activeSiteDocumentReader.Load()
	if holder == nil || holder.reader == nil {
		return caddyhttp.Error(http.StatusServiceUnavailable, errors.New(""))
	}
	file, err := holder.reader.OpenSiteDocument(request.Context(), handler.SiteID, request.URL.Path)
	if err != nil {
		return caddyhttp.Error(http.StatusNotFound, errors.New(""))
	}
	defer file.Close()
	staticFile, ok := file.(siteDocumentFile)
	if !ok {
		return caddyhttp.Error(http.StatusInternalServerError, errors.New(""))
	}
	info, err := staticFile.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return caddyhttp.Error(http.StatusNotFound, errors.New(""))
	}
	if mediaType := mime.TypeByExtension(filepath.Ext(request.URL.Path)); mediaType != "" {
		writer.Header().Set("Content-Type", mediaType)
	}
	http.ServeContent(writer, request, filepath.Base(request.URL.Path), info.ModTime(), staticFile)
	return nil
}

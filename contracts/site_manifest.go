package contracts

import (
	"embed"
	"encoding/json"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed v1/site-publish-manifest.json v1/site-manifest.schema.json
var siteManifestFiles embed.FS

var (
	siteManifestSchemaOnce sync.Once
	siteManifestSchema     *jsonschema.Schema
	siteManifestSchemaErr  error
)

type sitePublishManifestLink struct {
	ArchiveEntry string `json:"archiveEntry"`
	Schema       string `json:"schema"`
}

func SiteManifestArchiveEntry() (string, error) {
	link, err := loadSitePublishManifestLink()
	if err != nil || !fs.ValidPath(link.ArchiveEntry) || path.Base(link.ArchiveEntry) != link.ArchiveEntry {
		return "", ErrInvalidAssets
	}
	return link.ArchiveEntry, nil
}

func ValidateSiteManifest(contents []byte) error {
	schema, err := compiledSiteManifestSchema()
	if err != nil {
		return ErrInvalidAssets
	}
	if schema == nil {
		return ErrInvalidAssets
	}
	var candidate any
	if err := json.Unmarshal(contents, &candidate); err != nil {
		return ErrInvalidAssets
	}
	if err := schema.Validate(candidate); err != nil {
		return ErrInvalidAssets
	}
	return nil
}

func compiledSiteManifestSchema() (*jsonschema.Schema, error) {
	siteManifestSchemaOnce.Do(func() {
		link, err := loadSitePublishManifestLink()
		schemaPath := strings.TrimPrefix(link.Schema, "contracts/")
		if err != nil || schemaPath == link.Schema || !fs.ValidPath(schemaPath) {
			siteManifestSchemaErr = ErrInvalidAssets
			return
		}
		contents, err := fs.ReadFile(siteManifestFiles, schemaPath)
		if err != nil {
			siteManifestSchemaErr = ErrInvalidAssets
			return
		}
		var document any
		if err := json.Unmarshal(contents, &document); err != nil {
			siteManifestSchemaErr = ErrInvalidAssets
			return
		}
		var metadata struct {
			ID string `json:"$id"`
		}
		if err := json.Unmarshal(contents, &metadata); err != nil || metadata.ID == "" {
			siteManifestSchemaErr = ErrInvalidAssets
			return
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		if err := compiler.AddResource(metadata.ID, document); err != nil {
			siteManifestSchemaErr = ErrInvalidAssets
			return
		}
		siteManifestSchema, siteManifestSchemaErr = compiler.Compile(metadata.ID)
		if siteManifestSchemaErr != nil {
			siteManifestSchemaErr = ErrInvalidAssets
		}
	})
	return siteManifestSchema, siteManifestSchemaErr
}

func loadSitePublishManifestLink() (sitePublishManifestLink, error) {
	contents, err := fs.ReadFile(siteManifestFiles, "v1/site-publish-manifest.json")
	if err != nil {
		return sitePublishManifestLink{}, ErrInvalidAssets
	}
	var link sitePublishManifestLink
	if err := json.Unmarshal(contents, &link); err != nil || link.ArchiveEntry == "" || link.Schema == "" {
		return sitePublishManifestLink{}, ErrInvalidAssets
	}
	return link, nil
}

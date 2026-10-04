package contracts

import (
	"embed"
	"encoding/json"
	"io/fs"
)

//go:embed v1/admin-actions.json
var siteArchiveFiles embed.FS

type SiteArchiveLimits struct {
	MinArtifactBytes    int64 `json:"minArtifactBytes"`
	ArtifactBytes       int64 `json:"artifactBytes"`
	MetadataBytes       int64 `json:"metadataBytes"`
	MultipartOverhead   int64 `json:"multipartOverheadBytes"`
	ExpandedBytes       int64 `json:"expandedBytes"`
	MaxEntries          int64 `json:"maxFiles"`
	MaxCompressionRatio int64 `json:"maxCompressionRatio"`
}

func LoadSiteArchiveLimits() (SiteArchiveLimits, error) {
	contents, err := fs.ReadFile(siteArchiveFiles, "v1/admin-actions.json")
	if err != nil {
		return SiteArchiveLimits{}, ErrInvalidAssets
	}
	var catalog struct {
		Archive SiteArchiveLimits `json:"archive"`
	}
	if err := json.Unmarshal(contents, &catalog); err != nil {
		return SiteArchiveLimits{}, ErrInvalidAssets
	}
	limits := catalog.Archive
	if limits.MinArtifactBytes < 1 || limits.ArtifactBytes < limits.MinArtifactBytes || limits.MetadataBytes < 1 || limits.MultipartOverhead < 0 || limits.ExpandedBytes < 1 || limits.MaxEntries < 1 || limits.MaxCompressionRatio < 1 {
		return SiteArchiveLimits{}, ErrInvalidAssets
	}
	return limits, nil
}

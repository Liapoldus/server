package contracts

import (
	admindef "liapoldus.local/server-plugin/contracts/definitions/admin"
)

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
	v := admindef.AdminActions().Archive
	return SiteArchiveLimits{MinArtifactBytes: int64(v.MinArtifactBytes), ArtifactBytes: int64(v.ArtifactBytes), MetadataBytes: int64(v.MetadataBytes), MultipartOverhead: int64(v.MultipartOverheadBytes), ExpandedBytes: int64(v.ExpandedBytes), MaxEntries: int64(v.MaxFiles), MaxCompressionRatio: int64(v.MaxCompressionRatio)}, nil
}

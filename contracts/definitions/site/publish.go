package site

type SitePublishManifestDocument struct {
	Plugin             string `json:"plugin"`
	PublishCapability  string `json:"publishCapability"`
	RollbackCapability string `json:"rollbackCapability"`
	ArchiveEntry       string `json:"archiveEntry"`
	Schema             string `json:"schema"`
	Semantics          string `json:"semantics"`
	Vectors            string `json:"vectors"`
	Version            int    `json:"version"`
}

func SitePublishManifest() SitePublishManifestDocument {
	return SitePublishManifestDocument{
		Version:            int(1),
		Plugin:             "server",
		PublishCapability:  "server.sites.publish",
		RollbackCapability: "server.sites.rollback",
		ArchiveEntry:       "site-manifest.json",
		Schema:             "contracts/v1/site-manifest.schema.json",
		Semantics:          "contracts/v1/site-manifest-semantics.json",
		Vectors:            "contracts/v1/site-manifest-vectors.json",
	}
}

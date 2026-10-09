package site

import "liapoldus.local/server-plugin/contracts/schema"

type SiteManifestVectorsDocumentScenariosItemInputEntriesItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type SiteManifestVectorsDocumentScenariosItemInputManifest struct {
	SiteID        string `json:"siteId"`
	DocumentRoot  string `json:"documentRoot"`
	IndexDocument string `json:"indexDocument"`
	SchemaVersion int    `json:"schemaVersion"`
}

type SiteManifestVectorsDocumentScenariosItemInput struct {
	PayloadSiteID *string                                                     `json:"payloadSiteId,omitempty"`
	Entries       *[]SiteManifestVectorsDocumentScenariosItemInputEntriesItem `json:"entries,omitempty"`
	Manifest      *SiteManifestVectorsDocumentScenariosItemInputManifest      `json:"manifest,omitempty"`
	Requests      *[]string                                                   `json:"requests,omitempty"`
}

type SiteManifestVectorsDocumentScenariosItemExpected struct {
	Resolved        *map[string]string `json:"resolved,omitempty"`
	Responses       *map[string]int    `json:"responses,omitempty"`
	PublicFiles     *[]string          `json:"publicFiles,omitempty"`
	CurrentPrevious *string            `json:"currentPrevious,omitempty"`
	Reason          *string            `json:"reason,omitempty"`
	HTTPStatus      *int               `json:"httpStatus,omitempty"`
	Accepted        bool               `json:"accepted"`
}

type SiteManifestVectorsDocumentScenariosItem struct {
	Expected SiteManifestVectorsDocumentScenariosItemExpected `json:"expected"`
	Input    SiteManifestVectorsDocumentScenariosItemInput    `json:"input"`
	ID       string                                           `json:"id"`
}

type SiteManifestVectorsDocument struct {
	ManifestSchema string                                     `json:"manifestSchema"`
	Semantics      string                                     `json:"semantics"`
	Scenarios      []SiteManifestVectorsDocumentScenariosItem `json:"scenarios"`
	Version        int                                        `json:"version"`
}

func SiteManifestVectors() SiteManifestVectorsDocument {
	return SiteManifestVectorsDocument{
		Version:        int(1),
		ManifestSchema: "site-manifest.schema.json",
		Semantics:      "site-manifest-semantics.json",
		Scenarios: []SiteManifestVectorsDocumentScenariosItem{{
			ID: "site-manifest-valid-archive-root",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("docs"),
				Entries: schema.Value([]SiteManifestVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "site-manifest.json",
					Type: "regular-file",
				}, {
					Path: "index.html",
					Type: "regular-file",
				}}),
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  ".",
					IndexDocument: "index.html",
				}),
				Requests: schema.Value([]string{"/", "/site-manifest.json"}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted: true,
				Resolved: schema.Value(map[string]string{
					"/": "index.html",
				}),
				Responses: schema.Value(map[string]int{
					"/":                   int(200),
					"/site-manifest.json": int(404),
				}),
				PublicFiles:     schema.Value([]string{"index.html"}),
				CurrentPrevious: schema.Value("activate-atomically-after-operation-success"),
			},
		}, {
			ID: "site-manifest-valid-subdirectory-root",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("portal"),
				Entries: schema.Value([]SiteManifestVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "site-manifest.json",
					Type: "regular-file",
				}, {
					Path: "dist/index.html",
					Type: "regular-file",
				}, {
					Path: "dist/docs/index.html",
					Type: "regular-file",
				}}),
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "portal",
					DocumentRoot:  "dist",
					IndexDocument: "index.html",
				}),
				Requests: schema.Value([]string{"/", "/docs/"}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted: true,
				Resolved: schema.Value(map[string]string{
					"/":      "dist/index.html",
					"/docs/": "dist/docs/index.html",
				}),
			},
		}, {
			ID: "site-manifest-rejects-site-id-mismatch",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("docs"),
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "Docs",
					DocumentRoot:  ".",
					IndexDocument: "index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("site-id-mismatch"),
			},
		}, {
			ID: "site-manifest-rejects-site-id-unicode-normalization-mismatch",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("café"),
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "café",
					DocumentRoot:  ".",
					IndexDocument: "index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("site-id-mismatch"),
			},
		}, {
			ID: "site-manifest-rejects-missing-entry",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("docs"),
				Entries: schema.Value([]SiteManifestVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "index.html",
					Type: "regular-file",
				}}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("manifest-missing"),
			},
		}, {
			ID: "site-manifest-rejects-nested-entry",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("docs"),
				Entries: schema.Value([]SiteManifestVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "nested/site-manifest.json",
					Type: "regular-file",
				}}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("manifest-missing"),
			},
		}, {
			ID: "site-manifest-rejects-duplicate-entry",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("docs"),
				Entries: schema.Value([]SiteManifestVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "site-manifest.json",
					Type: "regular-file",
				}, {
					Path: "site-manifest.json",
					Type: "regular-file",
				}}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("manifest-duplicate"),
			},
		}, {
			ID: "site-manifest-rejects-unsupported-version",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("docs"),
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(2),
					SiteID:        "docs",
					DocumentRoot:  ".",
					IndexDocument: "index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("unsupported-schema-version"),
			},
		}, {
			ID: "site-manifest-rejects-missing-index-document",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("docs"),
				Entries: schema.Value([]SiteManifestVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "site-manifest.json",
					Type: "regular-file",
				}}),
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  ".",
					IndexDocument: "index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("index-document-not-found-as-regular-file"),
			},
		}, {
			ID: "site-manifest-rejects-traversal-root",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  "../outside",
					IndexDocument: "index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("unsafe-document-root"),
			},
		}, {
			ID: "site-manifest-rejects-absolute-root",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  "/srv/site",
					IndexDocument: "index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("unsafe-document-root"),
			},
		}, {
			ID: "site-manifest-rejects-backslash-root",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  "dist\\public",
					IndexDocument: "index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("unsafe-document-root"),
			},
		}, {
			ID: "site-manifest-rejects-traversal-index",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  "dist",
					IndexDocument: "../index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("unsafe-index-document"),
			},
		}, {
			ID: "site-manifest-rejects-absolute-index",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  "dist",
					IndexDocument: "/index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("unsafe-index-document"),
			},
		}, {
			ID: "site-manifest-rejects-backslash-index",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  "dist",
					IndexDocument: "dist\\index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("unsafe-index-document"),
			},
		}, {
			ID: "site-manifest-rejects-control-root",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  "dist\u0000public",
					IndexDocument: "index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("unsafe-document-root"),
			},
		}, {
			ID: "site-manifest-rejects-control-index",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  "dist",
					IndexDocument: "bad\u001findex.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("unsafe-index-document"),
			},
		}, {
			ID: "site-manifest-rejects-delete-index",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  "dist",
					IndexDocument: "badindex.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("unsafe-index-document"),
			},
		}, {
			ID: "site-manifest-rejects-reserved-index-document",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("docs"),
				Entries: schema.Value([]SiteManifestVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "site-manifest.json",
					Type: "regular-file",
				}}),
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  ".",
					IndexDocument: "site-manifest.json",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("reserved-manifest-as-index"),
			},
		}, {
			ID: "site-manifest-rejects-symlink-root",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("docs"),
				Entries: schema.Value([]SiteManifestVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "site-manifest.json",
					Type: "regular-file",
				}, {
					Path: "dist",
					Type: "symbolic-link",
				}}),
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  "dist",
					IndexDocument: "index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("document-root-resolves-through-link"),
			},
		}, {
			ID: "site-manifest-rejects-symlink-index",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("docs"),
				Entries: schema.Value([]SiteManifestVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "site-manifest.json",
					Type: "regular-file",
				}, {
					Path: "dist/index.html",
					Type: "symbolic-link",
				}}),
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  "dist",
					IndexDocument: "index.html",
				}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted:        false,
				CurrentPrevious: schema.Value("unchanged"),
				Reason:          schema.Value("index-document-is-not-regular-file"),
			},
		}, {
			ID: "site-manifest-never-served",
			Input: SiteManifestVectorsDocumentScenariosItemInput{
				PayloadSiteID: schema.Value("docs"),
				Entries: schema.Value([]SiteManifestVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "site-manifest.json",
					Type: "regular-file",
				}, {
					Path: "index.html",
					Type: "regular-file",
				}}),
				Manifest: schema.Value(SiteManifestVectorsDocumentScenariosItemInputManifest{
					SchemaVersion: int(1),
					SiteID:        "docs",
					DocumentRoot:  ".",
					IndexDocument: "index.html",
				}),
				Requests: schema.Value([]string{"/site-manifest.json"}),
			},
			Expected: SiteManifestVectorsDocumentScenariosItemExpected{
				Accepted: true,
				Responses: schema.Value(map[string]int{
					"/site-manifest.json": int(404),
				}),
				PublicFiles:     schema.Value([]string{"index.html"}),
				CurrentPrevious: schema.Value("unchanged"),
				HTTPStatus:      schema.Value(int(404)),
			},
		}},
	}
}

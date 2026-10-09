package site

type SiteManifestSemanticsDocumentArchiveEntry struct {
	Path                string `json:"path"`
	Location            string `json:"location"`
	Type                string `json:"type"`
	Encoding            string `json:"encoding"`
	DuplicateObjectKeys string `json:"duplicateObjectKeys"`
	UnknownFields       string `json:"unknownFields"`
	Count               int    `json:"count"`
}

type SiteManifestSemanticsDocumentFieldsDocumentRoot struct {
	Format        string   `json:"format"`
	Dot           string   `json:"dot"`
	MustResolveTo string   `json:"mustResolveTo"`
	Reject        []string `json:"reject"`
}

type SiteManifestSemanticsDocumentFieldsIndexDocument struct {
	Format        string   `json:"format"`
	MustResolveTo string   `json:"mustResolveTo"`
	Reject        []string `json:"reject"`
}

type SiteManifestSemanticsDocumentFields struct {
	SchemaVersion string                                           `json:"schemaVersion"`
	SiteID        string                                           `json:"siteId"`
	DocumentRoot  SiteManifestSemanticsDocumentFieldsDocumentRoot  `json:"documentRoot"`
	IndexDocument SiteManifestSemanticsDocumentFieldsIndexDocument `json:"indexDocument"`
}

type SiteManifestSemanticsDocumentArchiveValidation struct {
	ManifestSiteIdMismatch                              string `json:"manifestSiteIdMismatch"`
	MissingOrDuplicateManifest                          string `json:"missingOrDuplicateManifest"`
	UnsupportedSchemaVersion                            string `json:"unsupportedSchemaVersion"`
	MissingDocumentRootOrIndexDocument                  string `json:"missingDocumentRootOrIndexDocument"`
	ManifestAsIndexDocumentWhenDocumentRootIsDot        string `json:"manifestAsIndexDocumentWhenDocumentRootIsDot"`
	SymlinkOrHardlinkUsedForDocumentRootOrIndexDocument string `json:"symlinkOrHardlinkUsedForDocumentRootOrIndexDocument"`
	CurrentPreviousOnAnyRejection                       string `json:"currentPreviousOnAnyRejection"`
}

type SiteManifestSemanticsDocumentStaticServing struct {
	RootRequest      string `json:"rootRequest"`
	DirectoryRequest string `json:"directoryRequest"`
	MissingPath      string `json:"missingPath"`
	PublicNamespace  string `json:"publicNamespace"`
	ManifestExposure string `json:"manifestExposure"`
	SpaFallback      bool   `json:"spaFallback"`
}

type SiteManifestSemanticsDocument struct {
	Fields            SiteManifestSemanticsDocumentFields            `json:"fields"`
	ArchiveValidation SiteManifestSemanticsDocumentArchiveValidation `json:"archiveValidation"`
	ArchiveEntry      SiteManifestSemanticsDocumentArchiveEntry      `json:"archiveEntry"`
	StaticServing     SiteManifestSemanticsDocumentStaticServing     `json:"staticServing"`
	ManifestSchema    string                                         `json:"manifestSchema"`
	ReleaseLifecycle  string                                         `json:"releaseLifecycle"`
	Secrets           string                                         `json:"secrets"`
	Version           int                                            `json:"version"`
}

func SiteManifestSemantics() SiteManifestSemanticsDocument {
	return SiteManifestSemanticsDocument{
		Version:        int(1),
		ManifestSchema: "site-manifest.schema.json",
		ArchiveEntry: SiteManifestSemanticsDocumentArchiveEntry{
			Path:                "site-manifest.json",
			Location:            "exactly-at-tar-root",
			Count:               int(1),
			Type:                "regular-file",
			Encoding:            "UTF-8 JSON",
			DuplicateObjectKeys: "reject",
			UnknownFields:       "reject",
		},
		Fields: SiteManifestSemanticsDocumentFields{
			SchemaVersion: "must-equal-1",
			SiteID:        "must-equal-the-UTF-8-byte-sequence-of-the-decoded-publish-metadata-payload-siteId-string-without-case-folding-or-Unicode-normalization",
			DocumentRoot: SiteManifestSemanticsDocumentFieldsDocumentRoot{
				Format:        "POSIX relative directory path using forward-slash separators",
				Dot:           "the archive root",
				Reject:        []string{"absolute-path", "backslash", "empty-segment", "dot-segment", "parent-segment", "trailing-slash", "NUL-C0-control-DEL"},
				MustResolveTo: "directory-containing-indexDocument-within-the-staged-archive",
			},
			IndexDocument: SiteManifestSemanticsDocumentFieldsIndexDocument{
				Format:        "POSIX relative regular-file path using forward-slash separators, relative to documentRoot",
				Reject:        []string{"absolute-path", "backslash", "empty-segment", "dot-segment", "parent-segment", "trailing-slash", "NUL-C0-control-DEL"},
				MustResolveTo: "regular-file-within-documentRoot",
			},
		},
		ArchiveValidation: SiteManifestSemanticsDocumentArchiveValidation{
			ManifestSiteIdMismatch:                              "reject-entire-candidate",
			MissingOrDuplicateManifest:                          "reject-entire-candidate",
			UnsupportedSchemaVersion:                            "reject-entire-candidate",
			MissingDocumentRootOrIndexDocument:                  "reject-entire-candidate",
			ManifestAsIndexDocumentWhenDocumentRootIsDot:        "reject-with-reserved-manifest-as-index",
			SymlinkOrHardlinkUsedForDocumentRootOrIndexDocument: "reject-entire-candidate",
			CurrentPreviousOnAnyRejection:                       "unchanged",
		},
		StaticServing: SiteManifestSemanticsDocumentStaticServing{
			RootRequest:      "resolve-documentRoot-then-append-indexDocument",
			DirectoryRequest: "resolve-requested-directory-under-documentRoot-then-append-indexDocument",
			MissingPath:      "404",
			SpaFallback:      false,
			PublicNamespace:  "exclude-the-archive-root-site-manifest.json-before-static-path-resolution",
			ManifestExposure: "site-manifest.json-is-never-served-as-public-static-content-even-when-documentRoot-is-dot",
		},
		ReleaseLifecycle: "successful-publish-or-rollback-atomically-controls-current-previous",
		Secrets:          "manifest-contains-no-secret-values-or-secret-references",
	}
}

package admin

import "liapoldus.local/server-plugin/contracts/schema"

type AdminSurfaceVectorsDocumentScenariosItemInputEntriesItem struct {
	Type *string `json:"type,omitempty"`
	Path string  `json:"path"`
}

type AdminSurfaceVectorsDocumentScenariosItemInput struct {
	Source                    *string                                                     `json:"source,omitempty"`
	Remove                    *string                                                     `json:"remove,omitempty"`
	MultipartParts            *[]string                                                   `json:"multipartParts,omitempty"`
	ArtifactBytes             *int                                                        `json:"artifactBytes,omitempty"`
	DeclaredDigest            *string                                                     `json:"declaredDigest,omitempty"`
	ActualDigest              *string                                                     `json:"actualDigest,omitempty"`
	ExpandedBytes             *int                                                        `json:"expandedBytes,omitempty"`
	FileCount                 *int                                                        `json:"fileCount,omitempty"`
	RegularFileEntries        *int                                                        `json:"regularFileEntries,omitempty"`
	ExplicitDirectoryEntries  *int                                                        `json:"explicitDirectoryEntries,omitempty"`
	ImplicitParentDirectories *int                                                        `json:"implicitParentDirectories,omitempty"`
	CompressedBytes           *int                                                        `json:"compressedBytes,omitempty"`
	GzipMembers               *int                                                        `json:"gzipMembers,omitempty"`
	ExpandedStream            *string                                                     `json:"expandedStream,omitempty"`
	GzipMemberIntegrity       *string                                                     `json:"gzipMemberIntegrity,omitempty"`
	GzipMember                *string                                                     `json:"gzipMember,omitempty"`
	TrailingCompressedData    *string                                                     `json:"trailingCompressedData,omitempty"`
	TarArchives               *int                                                        `json:"tarArchives,omitempty"`
	PostTerminatorBytes       *string                                                     `json:"postTerminatorBytes,omitempty"`
	Entries                   *[]AdminSurfaceVectorsDocumentScenariosItemInputEntriesItem `json:"entries,omitempty"`
	SameKey                   *bool                                                       `json:"sameKey,omitempty"`
	SameMetadata              *bool                                                       `json:"sameMetadata,omitempty"`
	SameArtifactDigest        *bool                                                       `json:"sameArtifactDigest,omitempty"`
	SettingsPage              *bool                                                       `json:"settingsPage,omitempty"`
}

type AdminSurfaceVectorsDocumentScenariosItemExpected struct {
	SurfaceValid        *bool   `json:"surfaceValid,omitempty"`
	Core                *string `json:"core,omitempty"`
	Plugin              *string `json:"plugin,omitempty"`
	EffectiveTarEntries *int    `json:"effectiveTarEntries,omitempty"`
}

type AdminSurfaceVectorsDocumentScenariosItem struct {
	Input    AdminSurfaceVectorsDocumentScenariosItemInput    `json:"input"`
	Expected AdminSurfaceVectorsDocumentScenariosItemExpected `json:"expected"`
	ID       string                                           `json:"id"`
}

type AdminSurfaceVectorsDocument struct {
	Contract  string                                     `json:"contract"`
	Scenarios []AdminSurfaceVectorsDocumentScenariosItem `json:"scenarios"`
	Version   int                                        `json:"version"`
}

func AdminSurfaceVectors() AdminSurfaceVectorsDocument {
	return AdminSurfaceVectorsDocument{
		Version:  int(1),
		Contract: "admin-surface.schema.json",
		Scenarios: []AdminSurfaceVectorsDocumentScenariosItem{{
			ID: "admin-surface-valid",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				Source: schema.Value("contracts/v1/admin-surface.json"),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				SurfaceValid: schema.Value(true),
			},
		}, {
			ID: "admin-surface-unknown-field",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				Remove: schema.Value("additionalProperties:false"),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				SurfaceValid: schema.Value(false),
			},
		}, {
			ID: "admin-publish-rejects-extra-part",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				MultipartParts: schema.Value([]string{"metadata", "artifact", "extra"}),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Core: schema.Value("reject-before-stream-open"),
			},
		}, {
			ID: "admin-publish-rejects-oversized-artifact",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				ArtifactBytes: schema.Value(int(134217729)),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Core: schema.Value("reject-with-resource-exhausted"),
			},
		}, {
			ID: "admin-publish-rejects-digest-mismatch",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				DeclaredDigest: schema.Value("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
				ActualDigest:   schema.Value("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Core: schema.Value("reject-before-forwarding-success-receipt"),
			},
		}, {
			ID: "admin-publish-rejects-empty-artifact",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				ArtifactBytes: schema.Value(int(0)),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("reject-before-durable-acceptance"),
			},
		}, {
			ID: "admin-publish-rejects-expanded-size-over-limit",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				ExpandedBytes: schema.Value(int(536870913)),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-rejects-file-count-over-limit",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				FileCount: schema.Value(int(10001)),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-counts-explicit-directories-toward-entry-limit",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				RegularFileEntries:        schema.Value(int(2)),
				ExplicitDirectoryEntries:  schema.Value(int(9999)),
				ImplicitParentDirectories: schema.Value(int(0)),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin:              schema.Value("accept-then-fail-operation-before-current-pointer-change"),
				EffectiveTarEntries: schema.Value(int(10001)),
			},
		}, {
			ID: "admin-publish-rejects-compression-ratio-over-limit",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				ExpandedBytes:   schema.Value(int(102401)),
				CompressedBytes: schema.Value(int(1024)),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-allows-concatenated-gzip-members",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				GzipMembers:    schema.Value(int(2)),
				ExpandedStream: schema.Value("one-valid-tar"),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept"),
			},
		}, {
			ID: "admin-publish-rejects-corrupt-gzip-member",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				GzipMemberIntegrity: schema.Value("invalid-crc-or-size"),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-rejects-truncated-gzip-member",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				GzipMember: schema.Value("truncated-before-trailer"),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-rejects-non-gzip-trailing-bytes",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				GzipMembers:            schema.Value(int(1)),
				TrailingCompressedData: schema.Value("not-a-gzip-member"),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-allows-zero-tar-padding",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				TarArchives:         schema.Value(int(1)),
				PostTerminatorBytes: schema.Value("zero-only"),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept"),
			},
		}, {
			ID: "admin-publish-rejects-nonzero-tar-trailing-data",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				TarArchives:         schema.Value(int(1)),
				PostTerminatorBytes: schema.Value("contains-nonzero-data"),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-rejects-unsafe-archive-entry",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				Entries: schema.Value([]AdminSurfaceVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "../outside",
					Type: schema.Value("regular"),
				}}),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-rejects-duplicate-path",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				Entries: schema.Value([]AdminSurfaceVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "assets/app.js",
				}, {
					Path: "assets/app.js",
				}}),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-rejects-case-collision",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				Entries: schema.Value([]AdminSurfaceVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "assets/App.js",
				}, {
					Path: "assets/app.js",
				}}),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-rejects-unicode-nfc-collision",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				Entries: schema.Value([]AdminSurfaceVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "café/index.html",
				}, {
					Path: "café/index.html",
				}}),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-rejects-link-entry",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				Entries: schema.Value([]AdminSurfaceVectorsDocumentScenariosItemInputEntriesItem{{
					Path: "current",
					Type: schema.Value("symbolic-link"),
				}}),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("accept-then-fail-operation-before-current-pointer-change"),
			},
		}, {
			ID: "admin-publish-reuses-idempotent-operation",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				SameKey:            schema.Value(true),
				SameMetadata:       schema.Value(true),
				SameArtifactDigest: schema.Value(true),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				Plugin: schema.Value("return-existing-operation-without-duplicate-effect"),
			},
		}, {
			ID: "admin-common-settings-live-in-core",
			Input: AdminSurfaceVectorsDocumentScenariosItemInput{
				SettingsPage: schema.Value(true),
			},
			Expected: AdminSurfaceVectorsDocumentScenariosItemExpected{
				SurfaceValid: schema.Value(false),
			},
		}},
	}
}

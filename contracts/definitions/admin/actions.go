package admin

import "liapoldus.local/server-plugin/contracts/schema"

type AdminActionsDocumentArchiveEntryCountSemantics struct {
	Unit                      string   `json:"unit"`
	ImplicitParentDirectories string   `json:"implicitParentDirectories"`
	CountedKinds              []string `json:"countedKinds"`
}

type AdminActionsDocumentArchiveGzip struct {
	Members                 string `json:"members"`
	Integrity               string `json:"integrity"`
	TrailingCompressedBytes string `json:"trailingCompressedBytes"`
}

type AdminActionsDocumentArchiveTar struct {
	ArchiveCount        string `json:"archiveCount"`
	Terminator          string `json:"terminator"`
	PostTerminatorBytes string `json:"postTerminatorBytes"`
}

type AdminActionsDocumentArchive struct {
	EntryCountSemantics    AdminActionsDocumentArchiveEntryCountSemantics `json:"entryCountSemantics"`
	Tar                    AdminActionsDocumentArchiveTar                 `json:"tar"`
	Gzip                   AdminActionsDocumentArchiveGzip                `json:"gzip"`
	MediaType              string                                         `json:"mediaType"`
	RejectedEntries        []string                                       `json:"rejectedEntries"`
	ValidationChecks       []string                                       `json:"validationChecks"`
	ExpandedBytes          int                                            `json:"expandedBytes"`
	MaxFiles               int                                            `json:"maxFiles"`
	RequestEnvelopeBytes   int                                            `json:"requestEnvelopeBytes"`
	MaxCompressionRatio    int                                            `json:"maxCompressionRatio"`
	MultipartOverheadBytes int                                            `json:"multipartOverheadBytes"`
	MetadataBytes          int                                            `json:"metadataBytes"`
	ArtifactBytes          int                                            `json:"artifactBytes"`
	MinArtifactBytes       int                                            `json:"minArtifactBytes"`
}

type AdminActionsDocumentReleaseLifecycle struct {
	Storage            string `json:"storage"`
	CurrentPrevious    string `json:"currentPrevious"`
	Staging            string `json:"staging"`
	Activation         string `json:"activation"`
	Failure            string `json:"failure"`
	PublishStatus      int    `json:"publishStatus"`
	ImmutableRevisions bool   `json:"immutableRevisions"`
}

type AdminActionsDocumentPublishOperation struct {
	Acceptance               string `json:"acceptance"`
	ArchiveValidation        string `json:"archiveValidation"`
	ArchiveValidationFailure string `json:"archiveValidationFailure"`
	CurrentPointer           string `json:"currentPointer"`
}

type AdminActionsDocumentOperationStatus struct {
	StateMapping                 map[string]string `json:"stateMapping"`
	CoreResource                 string            `json:"coreResource"`
	PluginStatusCapability       string            `json:"pluginStatusCapability"`
	PluginStates                 []string          `json:"pluginStates"`
	CoreStates                   []string          `json:"coreStates"`
	WorkerPollMilliseconds       int               `json:"workerPollMilliseconds"`
	CoreControlUsesPluginSdkRest bool              `json:"coreControlUsesPluginSdkRest"`
}

type AdminActionsDocumentOperationsEntryErrorsEntry struct {
	HTTP      int  `json:"http"`
	Retryable bool `json:"retryable"`
}

type AdminActionsDocumentOperationsEntrySurfaceBinding struct {
	PageID    string `json:"page"`
	SectionID string `json:"section"`
	ActionID  string `json:"action"`
}

type AdminActionsDocumentOperationsEntryIdempotency struct {
	Semantics      *string  `json:"semantics,omitempty"`
	Repeat         *string  `json:"repeat,omitempty"`
	SameInput      *string  `json:"sameInput,omitempty"`
	DifferentInput *string  `json:"differentInput,omitempty"`
	Scope          []string `json:"scope"`
	Required       bool     `json:"required"`
}

type AdminActionsDocumentOperationsEntryMultipart struct {
	Parts                     []string `json:"parts"`
	Order                     []string `json:"order"`
	ArtifactFilenameForwarded bool     `json:"artifactFilenameForwarded"`
}

type AdminActionsDocumentOperationsEntryArchiveLimitsEntryCountSemantics struct {
	Unit                      string   `json:"unit"`
	ImplicitParentDirectories string   `json:"implicitParentDirectories"`
	CountedKinds              []string `json:"countedKinds"`
}

type AdminActionsDocumentOperationsEntryArchiveLimitsGzip struct {
	Members                 string `json:"members"`
	Integrity               string `json:"integrity"`
	TrailingCompressedBytes string `json:"trailingCompressedBytes"`
}

type AdminActionsDocumentOperationsEntryArchiveLimitsTar struct {
	ArchiveCount        string `json:"archiveCount"`
	Terminator          string `json:"terminator"`
	PostTerminatorBytes string `json:"postTerminatorBytes"`
}

type AdminActionsDocumentOperationsEntryArchiveLimits struct {
	EntryCountSemantics    AdminActionsDocumentOperationsEntryArchiveLimitsEntryCountSemantics `json:"entryCountSemantics"`
	Tar                    AdminActionsDocumentOperationsEntryArchiveLimitsTar                 `json:"tar"`
	Gzip                   AdminActionsDocumentOperationsEntryArchiveLimitsGzip                `json:"gzip"`
	MediaType              string                                                              `json:"mediaType"`
	RejectedEntries        []string                                                            `json:"rejectedEntries"`
	ValidationChecks       []string                                                            `json:"validationChecks"`
	ExpandedBytes          int                                                                 `json:"expandedBytes"`
	MaxFiles               int                                                                 `json:"maxFiles"`
	RequestEnvelopeBytes   int                                                                 `json:"requestEnvelopeBytes"`
	MaxCompressionRatio    int                                                                 `json:"maxCompressionRatio"`
	MultipartOverheadBytes int                                                                 `json:"multipartOverheadBytes"`
	MetadataBytes          int                                                                 `json:"metadataBytes"`
	ArtifactBytes          int                                                                 `json:"artifactBytes"`
	MinArtifactBytes       int                                                                 `json:"minArtifactBytes"`
}

type AdminActionsDocumentOperationsEntry struct {
	SurfaceBinding       *AdminActionsDocumentOperationsEntrySurfaceBinding        `json:"surfaceBinding,omitempty"`
	ArchiveLimits        *AdminActionsDocumentOperationsEntryArchiveLimits         `json:"archiveLimits,omitempty"`
	RequestSchema        *schema.Definition                                        `json:"requestSchema,omitempty"`
	ResponseSchema       *schema.Definition                                        `json:"responseSchema,omitempty"`
	HTTPStatus           *int                                                      `json:"httpStatus,omitempty"`
	Errors               map[string]AdminActionsDocumentOperationsEntryErrorsEntry `json:"errors"`
	Transport            *string                                                   `json:"transport,omitempty"`
	RevisionPrecondition *string                                                   `json:"revisionPrecondition,omitempty"`
	Idempotency          *AdminActionsDocumentOperationsEntryIdempotency           `json:"idempotency,omitempty"`
	Multipart            *AdminActionsDocumentOperationsEntryMultipart             `json:"multipart,omitempty"`
	AcceptedHTTPStatus   *int                                                      `json:"acceptedHttpStatus,omitempty"`
	ReceiptSchema        *string                                                   `json:"receiptSchema,omitempty"`
	Ownership            string                                                    `json:"ownership"`
	Kind                 string                                                    `json:"kind"`
}

type AdminActionsDocument struct {
	Operations       map[string]AdminActionsDocumentOperationsEntry `json:"operations"`
	OperationStatus  AdminActionsDocumentOperationStatus            `json:"operationStatus"`
	PublishOperation AdminActionsDocumentPublishOperation           `json:"publishOperation"`
	Plugin           string                                         `json:"plugin"`
	Archive          AdminActionsDocumentArchive                    `json:"archive"`
	ReleaseLifecycle AdminActionsDocumentReleaseLifecycle           `json:"releaseLifecycle"`
	Version          int                                            `json:"version"`
}

func AdminActions() AdminActionsDocument {
	return AdminActionsDocument{
		Version: int(1),
		Plugin:  "server",
		Archive: AdminActionsDocumentArchive{
			MediaType:              "application/gzip",
			MinArtifactBytes:       int(1),
			ArtifactBytes:          int(134217728),
			MetadataBytes:          int(65536),
			MultipartOverheadBytes: int(65536),
			RequestEnvelopeBytes:   int(134348800),
			ExpandedBytes:          int(536870912),
			MaxFiles:               int(10000),
			EntryCountSemantics: AdminActionsDocumentArchiveEntryCountSemantics{
				Unit:                      "logical-tar-member-after-extension-processing",
				CountedKinds:              []string{"regular-file", "directory"},
				ImplicitParentDirectories: "not-counted",
			},
			MaxCompressionRatio: int(100),
			Gzip: AdminActionsDocumentArchiveGzip{
				Members:                 "concatenated-members-allowed",
				Integrity:               "drain-all-members-through-eof",
				TrailingCompressedBytes: "reject-unless-part-of-valid-gzip-member",
			},
			Tar: AdminActionsDocumentArchiveTar{
				ArchiveCount:        "one-tar-stream",
				Terminator:          "two-consecutive-512-byte-zero-blocks",
				PostTerminatorBytes: "zero-padding-only",
			},
			ValidationChecks: []string{"gzip-integrity", "tar-integrity", "streamed-sha256", "metadata-artifact-descriptor-equality", "aggregate-expanded-byte-limit", "file-count-limit", "compression-ratio-limit"},
			RejectedEntries:  []string{"absolute-path", "path-traversal", "symbolic-link", "hard-link", "duplicate-path", "case-collision", "unicode-nfc-collision", "special-file"},
		},
		ReleaseLifecycle: AdminActionsDocumentReleaseLifecycle{
			ImmutableRevisions: true,
			Storage:            "plugin-owned-persistent-filesystem",
			CurrentPrevious:    "successful-publish-or-rollback-atomically-swaps-current-and-previous",
			Staging:            "validate-complete-staged-release-before-activation",
			Activation:         "atomic-current-previous-pointer-swap-only-after-successful-terminal-operation",
			Failure:            "keep-current-and-previous-unchanged",
			PublishStatus:      int(202),
		},
		PublishOperation: AdminActionsDocumentPublishOperation{
			Acceptance:               "after-validated-metadata-and-bounded-artifact-bytes-are-durably-stored-and-sha256-matches",
			ArchiveValidation:        "asynchronous-durable-operation-before-release-activation",
			ArchiveValidationFailure: "operation-failed-with-stable-error-code-current-and-previous-unchanged",
			CurrentPointer:           "changes-only-after-all-archive-and-manifest-validation-succeeds",
		},
		OperationStatus: AdminActionsDocumentOperationStatus{
			CoreResource:           "GET /api/operations/{id}",
			WorkerPollMilliseconds: int(100),
			PluginStatusCapability: "server.operations.get",
			PluginStates:           []string{"accepted", "running", "completed", "failed"},
			CoreStates:             []string{"pending", "running", "succeeded", "failed", "degraded"},
			StateMapping: map[string]string{
				"accepted":  "pending",
				"running":   "running",
				"completed": "succeeded",
				"failed":    "failed",
			},
			CoreControlUsesPluginSdkRest: true,
		},
		Operations: map[string]AdminActionsDocumentOperationsEntry{
			"server.sites.list": {
				Kind:      "query",
				Ownership: "plugin-admin-surface-capability",
				RequestSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"cursor": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
						MaxLength: schema.Value(int(1024)),
					}, "limit": {
						Type:    "integer",
						Minimum: schema.Value(int(1)),
						Maximum: schema.Value(int(100)),
					}},
					Required:             schema.Value([]string{}),
					AdditionalProperties: false,
				}),
				ResponseSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"items": {
						Type: "array",
						Items: schema.Value(schema.Definition{
							Type: "object",
							Properties: map[string]schema.Definition{"siteId": {
								Type:      "string",
								MinLength: schema.Value(int(1)),
								MaxLength: schema.Value(int(128)),
							}, "currentRevision": {
								Type: []any{"string", "null"},
							}, "previousRevision": {
								Type: []any{"string", "null"},
							}, "updatedAt": {
								Type:      "string",
								MinLength: schema.Value(int(1)),
							}},
							Required:             schema.Value([]string{"siteId", "currentRevision", "previousRevision", "updatedAt"}),
							AdditionalProperties: false,
						}),
					}, "nextCursor": {
						Type: []any{"string", "null"},
					}},
					Required:             schema.Value([]string{"items", "nextCursor"}),
					AdditionalProperties: false,
				}),
				HTTPStatus: schema.Value(int(200)),
				Errors: map[string]AdminActionsDocumentOperationsEntryErrorsEntry{
					"invalid_input": {
						HTTP:      int(400),
						Retryable: false,
					},
					"not_found": {
						HTTP:      int(404),
						Retryable: false,
					},
					"conflict": {
						HTTP:      int(409),
						Retryable: false,
					},
					"unavailable": {
						HTTP:      int(503),
						Retryable: true,
					},
				},
			},
			"server.sites.releases.list": {
				Kind:      "query",
				Ownership: "plugin-admin-surface-capability",
				RequestSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"siteId": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
						MaxLength: schema.Value(int(128)),
					}, "cursor": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
						MaxLength: schema.Value(int(1024)),
					}, "limit": {
						Type:    "integer",
						Minimum: schema.Value(int(1)),
						Maximum: schema.Value(int(100)),
					}},
					Required:             schema.Value([]string{"siteId"}),
					AdditionalProperties: false,
				}),
				ResponseSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"items": {
						Type: "array",
						Items: schema.Value(schema.Definition{
							Type: "object",
							Properties: map[string]schema.Definition{"siteId": {
								Type:      "string",
								MinLength: schema.Value(int(1)),
							}, "revisionId": {
								Type:      "string",
								MinLength: schema.Value(int(1)),
							}, "state": {
								Enum: []any{"current", "previous", "available"},
							}, "sha256": {
								Type:    "string",
								Pattern: "^sha256:[0-9a-f]{64}$",
							}, "createdAt": {
								Type:      "string",
								MinLength: schema.Value(int(1)),
							}},
							Required:             schema.Value([]string{"siteId", "revisionId", "state", "sha256", "createdAt"}),
							AdditionalProperties: false,
						}),
					}, "nextCursor": {
						Type: []any{"string", "null"},
					}},
					Required:             schema.Value([]string{"items", "nextCursor"}),
					AdditionalProperties: false,
				}),
				HTTPStatus: schema.Value(int(200)),
				Errors: map[string]AdminActionsDocumentOperationsEntryErrorsEntry{
					"invalid_input": {
						HTTP:      int(400),
						Retryable: false,
					},
					"not_found": {
						HTTP:      int(404),
						Retryable: false,
					},
					"conflict": {
						HTTP:      int(409),
						Retryable: false,
					},
					"unavailable": {
						HTTP:      int(503),
						Retryable: true,
					},
				},
			},
			"server.sites.rollback": {
				Kind:      "action",
				Ownership: "plugin-admin-surface-action",
				RequestSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"siteId": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
						MaxLength: schema.Value(int(128)),
					}, "expectedCurrentRevision": {
						Type:    "string",
						Pattern: "^sha256:[a-f0-9]{64}$",
					}, "targetRevision": {
						Type:    "string",
						Pattern: "^sha256:[a-f0-9]{64}$",
					}},
					Required:             schema.Value([]string{"siteId", "expectedCurrentRevision", "targetRevision"}),
					AdditionalProperties: false,
				}),
				ResponseSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"siteId": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
					}, "currentRevision": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
					}, "previousRevision": {
						Type: []any{"string", "null"},
					}},
					Required:             schema.Value([]string{"siteId", "currentRevision", "previousRevision"}),
					AdditionalProperties: false,
				}),
				HTTPStatus: schema.Value(int(200)),
				Errors: map[string]AdminActionsDocumentOperationsEntryErrorsEntry{
					"invalid_input": {
						HTTP:      int(400),
						Retryable: false,
					},
					"not_found": {
						HTTP:      int(404),
						Retryable: false,
					},
					"conflict": {
						HTTP:      int(409),
						Retryable: false,
					},
					"unavailable": {
						HTTP:      int(503),
						Retryable: true,
					},
				},
				SurfaceBinding: schema.Value(AdminActionsDocumentOperationsEntrySurfaceBinding{
					PageID:    "sites",
					SectionID: "sites",
					ActionID:  "rollback",
				}),
				Idempotency: schema.Value(AdminActionsDocumentOperationsEntryIdempotency{
					Required:  true,
					Scope:     []string{"plugin-instance", "capability", "idempotency-key"},
					Semantics: schema.Value("correlation-only"),
					Repeat:    schema.Value("invoke-again"),
				}),
			},
			"server.sites.publish": {
				Kind:      "artifact-action",
				Ownership: "plugin-admin-surface-action",
				Errors: map[string]AdminActionsDocumentOperationsEntryErrorsEntry{
					"invalid_input": {
						HTTP:      int(400),
						Retryable: false,
					},
					"not_found": {
						HTTP:      int(404),
						Retryable: false,
					},
					"conflict": {
						HTTP:      int(409),
						Retryable: false,
					},
					"unavailable": {
						HTTP:      int(503),
						Retryable: true,
					},
					"operation_failed": {
						HTTP:      int(500),
						Retryable: false,
					},
				},
				SurfaceBinding: schema.Value(AdminActionsDocumentOperationsEntrySurfaceBinding{
					PageID:    "sites",
					SectionID: "sites",
					ActionID:  "publish",
				}),
				Idempotency: schema.Value(AdminActionsDocumentOperationsEntryIdempotency{
					Required:       true,
					Scope:          []string{"plugin-instance", "capability", "idempotency-key"},
					SameInput:      schema.Value("return-existing-operation-without-duplicate-effect"),
					DifferentInput: schema.Value("reject-before-acceptance-with-ALREADY_EXISTS"),
				}),
				Transport: schema.Value("plugin-sdk.rest.artifact-stream"),
				Multipart: schema.Value(AdminActionsDocumentOperationsEntryMultipart{
					Parts:                     []string{"metadata", "artifact"},
					Order:                     []string{"metadata", "artifact"},
					ArtifactFilenameForwarded: false,
				}),
				AcceptedHTTPStatus: schema.Value(int(202)),
				ReceiptSchema:      schema.Value("contracts/v1/artifact-operation-result.schema.json"),
				ArchiveLimits: schema.Value(AdminActionsDocumentOperationsEntryArchiveLimits{
					MediaType:              "application/gzip",
					MinArtifactBytes:       int(1),
					ArtifactBytes:          int(134217728),
					MetadataBytes:          int(65536),
					MultipartOverheadBytes: int(65536),
					RequestEnvelopeBytes:   int(134348800),
					ExpandedBytes:          int(536870912),
					MaxFiles:               int(10000),
					EntryCountSemantics: AdminActionsDocumentOperationsEntryArchiveLimitsEntryCountSemantics{
						Unit:                      "logical-tar-member-after-extension-processing",
						CountedKinds:              []string{"regular-file", "directory"},
						ImplicitParentDirectories: "not-counted",
					},
					MaxCompressionRatio: int(100),
					Gzip: AdminActionsDocumentOperationsEntryArchiveLimitsGzip{
						Members:                 "concatenated-members-allowed",
						Integrity:               "drain-all-members-through-eof",
						TrailingCompressedBytes: "reject-unless-part-of-valid-gzip-member",
					},
					Tar: AdminActionsDocumentOperationsEntryArchiveLimitsTar{
						ArchiveCount:        "one-tar-stream",
						Terminator:          "two-consecutive-512-byte-zero-blocks",
						PostTerminatorBytes: "zero-padding-only",
					},
					ValidationChecks: []string{"gzip-integrity", "tar-integrity", "streamed-sha256", "metadata-artifact-descriptor-equality", "aggregate-expanded-byte-limit", "file-count-limit", "compression-ratio-limit"},
					RejectedEntries:  []string{"absolute-path", "path-traversal", "symbolic-link", "hard-link", "duplicate-path", "case-collision", "unicode-nfc-collision", "special-file"},
				}),
				RevisionPrecondition: schema.Value("if current exists, expectedCurrentRevision is required and must match; for a new site, expectedCurrentRevision must be omitted"),
			},
			"server.operations.list": {
				Kind:      "query",
				Ownership: "plugin-admin-surface-capability",
				RequestSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"cursor": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
						MaxLength: schema.Value(int(1024)),
					}, "limit": {
						Type:    "integer",
						Minimum: schema.Value(int(1)),
						Maximum: schema.Value(int(100)),
					}, "state": {
						Enum: []any{"accepted", "running", "completed", "failed"},
					}},
					Required:             schema.Value([]string{}),
					AdditionalProperties: false,
				}),
				ResponseSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"items": {
						Type: "array",
						Items: schema.Value(schema.Definition{
							Type: "object",
							Properties: map[string]schema.Definition{"operationId": {
								Type:      "string",
								MinLength: schema.Value(int(1)),
								MaxLength: schema.Value(int(256)),
							}, "siteId": {
								Type: []any{"string", "null"},
							}, "kind": {
								Type:      "string",
								MinLength: schema.Value(int(1)),
							}, "state": {
								Enum: []any{"accepted", "running", "completed", "failed"},
							}, "progressPercent": {
								Type:    []any{"integer", "null"},
								Minimum: schema.Value(int(0)),
								Maximum: schema.Value(int(100)),
							}, "updatedAt": {
								Type:      "string",
								MinLength: schema.Value(int(1)),
							}},
							Required:             schema.Value([]string{"operationId", "siteId", "kind", "state", "progressPercent", "updatedAt"}),
							AdditionalProperties: false,
						}),
					}, "nextCursor": {
						Type: []any{"string", "null"},
					}},
					Required:             schema.Value([]string{"items", "nextCursor"}),
					AdditionalProperties: false,
				}),
				HTTPStatus: schema.Value(int(200)),
				Errors: map[string]AdminActionsDocumentOperationsEntryErrorsEntry{
					"invalid_input": {
						HTTP:      int(400),
						Retryable: false,
					},
					"not_found": {
						HTTP:      int(404),
						Retryable: false,
					},
					"conflict": {
						HTTP:      int(409),
						Retryable: false,
					},
					"unavailable": {
						HTTP:      int(503),
						Retryable: true,
					},
				},
			},
			"server.operations.get": {
				Kind:      "action",
				Ownership: "plugin-admin-surface-action",
				RequestSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"operationId": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
						MaxLength: schema.Value(int(256)),
					}},
					Required:             schema.Value([]string{"operationId"}),
					AdditionalProperties: false,
				}),
				ResponseSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"operationId": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
						MaxLength: schema.Value(int(256)),
					}, "siteId": {
						Type: []any{"string", "null"},
					}, "kind": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
					}, "state": {
						Enum: []any{"accepted", "running", "completed", "failed"},
					}, "progressPercent": {
						Type:    []any{"integer", "null"},
						Minimum: schema.Value(int(0)),
						Maximum: schema.Value(int(100)),
					}, "updatedAt": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
					}, "errorCode": {
						Type:      []any{"string", "null"},
						MaxLength: schema.Value(int(128)),
					}},
					Required:             schema.Value([]string{"operationId", "siteId", "kind", "state", "progressPercent", "updatedAt", "errorCode"}),
					AdditionalProperties: false,
				}),
				HTTPStatus: schema.Value(int(200)),
				Errors: map[string]AdminActionsDocumentOperationsEntryErrorsEntry{
					"invalid_input": {
						HTTP:      int(400),
						Retryable: false,
					},
					"not_found": {
						HTTP:      int(404),
						Retryable: false,
					},
					"conflict": {
						HTTP:      int(409),
						Retryable: false,
					},
					"unavailable": {
						HTTP:      int(503),
						Retryable: true,
					},
				},
				SurfaceBinding: schema.Value(AdminActionsDocumentOperationsEntrySurfaceBinding{
					PageID:    "sites",
					SectionID: "operations",
					ActionID:  "status",
				}),
			},
			"server.certificates.list": {
				Kind:      "query",
				Ownership: "plugin-admin-surface-capability",
				RequestSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"domain": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
						MaxLength: schema.Value(int(253)),
					}, "cursor": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
						MaxLength: schema.Value(int(1024)),
					}, "limit": {
						Type:    "integer",
						Minimum: schema.Value(int(1)),
						Maximum: schema.Value(int(100)),
					}},
					Required:             schema.Value([]string{}),
					AdditionalProperties: false,
				}),
				ResponseSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"items": {
						Type: "array",
						Items: schema.Value(schema.Definition{
							Ref: "#/$defs/certificateSummary",
						}),
					}, "nextCursor": {
						Type: []any{"string", "null"},
					}},
					Required:             schema.Value([]string{"items", "nextCursor"}),
					AdditionalProperties: false,
					Defs: map[string]schema.Definition{"certificateSummary": {
						Type: "object",
						Properties: map[string]schema.Definition{"domain": {
							Type:      "string",
							MinLength: schema.Value(int(1)),
							MaxLength: schema.Value(int(253)),
						}, "source": {
							Enum: []any{"acme", "custom"},
						}, "readiness": {
							Enum: []any{"pending", "ready", "failed", "unknown"},
						}, "notAfter": {
							Type: []any{"string", "null"},
						}, "serial": {
							Type: []any{"string", "null"},
						}},
						Required:             schema.Value([]string{"domain", "source", "readiness", "notAfter", "serial"}),
						AdditionalProperties: false,
					}},
				}),
				HTTPStatus: schema.Value(int(200)),
				Errors: map[string]AdminActionsDocumentOperationsEntryErrorsEntry{
					"invalid_input": {
						HTTP:      int(400),
						Retryable: false,
					},
					"not_found": {
						HTTP:      int(404),
						Retryable: false,
					},
					"conflict": {
						HTTP:      int(409),
						Retryable: false,
					},
					"unavailable": {
						HTTP:      int(503),
						Retryable: true,
					},
				},
			},
			"server.certificates.get": {
				Kind:      "action",
				Ownership: "plugin-admin-surface-action",
				RequestSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"domain": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
						MaxLength: schema.Value(int(253)),
					}},
					Required:             schema.Value([]string{"domain"}),
					AdditionalProperties: false,
				}),
				ResponseSchema: schema.Value(schema.Definition{
					Definition: "https://json-schema.org/draft/2020-12/schema",
					Type:       "object",
					Properties: map[string]schema.Definition{"domain": {
						Type:      "string",
						MinLength: schema.Value(int(1)),
						MaxLength: schema.Value(int(253)),
					}, "source": {
						Enum: []any{"acme", "custom"},
					}, "readiness": {
						Enum: []any{"pending", "ready", "failed", "unknown"},
					}, "notBefore": {
						Type: []any{"string", "null"},
					}, "notAfter": {
						Type: []any{"string", "null"},
					}, "serial": {
						Type: []any{"string", "null"},
					}, "lastErrorCode": {
						Type:      []any{"string", "null"},
						MaxLength: schema.Value(int(128)),
					}},
					Required:             schema.Value([]string{"domain", "source", "readiness", "notBefore", "notAfter", "serial", "lastErrorCode"}),
					AdditionalProperties: false,
				}),
				HTTPStatus: schema.Value(int(200)),
				Errors: map[string]AdminActionsDocumentOperationsEntryErrorsEntry{
					"invalid_input": {
						HTTP:      int(400),
						Retryable: false,
					},
					"not_found": {
						HTTP:      int(404),
						Retryable: false,
					},
					"conflict": {
						HTTP:      int(409),
						Retryable: false,
					},
					"unavailable": {
						HTTP:      int(503),
						Retryable: true,
					},
				},
				SurfaceBinding: schema.Value(AdminActionsDocumentOperationsEntrySurfaceBinding{
					PageID:    "certificates",
					SectionID: "domains",
					ActionID:  "status",
				}),
			},
		},
	}
}

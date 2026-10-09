package admin

import "liapoldus.local/server-plugin/contracts/schema"

func AdminActionsSchema() schema.Definition {
	return schema.Definition{
		Definition:           "https://json-schema.org/draft/2020-12/schema",
		ID:                   "https://liapoldus.github.io/plugins/server/admin-ui/v1/actions.schema.json",
		Title:                "Server plugin Admin Surface action catalog v1",
		Type:                 "object",
		AdditionalProperties: false,
		Required:             schema.Value([]string{"version", "plugin", "archive", "releaseLifecycle", "publishOperation", "operationStatus", "operations"}),
		Properties: map[string]schema.Definition{"version": {
			Const: 1,
		}, "plugin": {
			Const: "server",
		}, "archive": {
			Ref: "#/$defs/archive",
		}, "releaseLifecycle": {
			Ref: "#/$defs/releaseLifecycle",
		}, "publishOperation": {
			Ref: "#/$defs/publishOperation",
		}, "operationStatus": {
			Ref: "#/$defs/operationStatus",
		}, "operations": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"server.sites.list", "server.sites.releases.list", "server.sites.rollback", "server.sites.publish", "server.operations.list", "server.operations.get", "server.certificates.list", "server.certificates.get"}),
			Properties: map[string]schema.Definition{"server.sites.list": {
				Ref: "#/$defs/queryOperation",
			}, "server.sites.releases.list": {
				Ref: "#/$defs/queryOperation",
			}, "server.sites.rollback": {
				Ref: "#/$defs/actionOperation",
			}, "server.sites.publish": {
				Ref: "#/$defs/artifactOperation",
			}, "server.operations.list": {
				Ref: "#/$defs/queryOperation",
			}, "server.operations.get": {
				Ref: "#/$defs/actionOperation",
			}, "server.certificates.list": {
				Ref: "#/$defs/queryOperation",
			}, "server.certificates.get": {
				Ref: "#/$defs/actionOperation",
			}},
		}},
		Defs: map[string]schema.Definition{"nonEmptyString": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
		}, "jsonSchema": {
			Type:     "object",
			Required: schema.Value([]string{"type", "properties", "required", "additionalProperties"}),
			Properties: map[string]schema.Definition{"$schema": {
				Type: "string",
			}, "$id": {
				Type: "string",
			}, "title": {
				Type: "string",
			}, "description": {
				Type: "string",
			}, "type": {
				Const: "object",
			}, "additionalProperties": {
				Const: false,
			}, "required": {
				Type:        "array",
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Type: "string",
				}),
			}, "properties": {
				Type: "object",
				AdditionalProperties: schema.Definition{
					Type: "object",
				},
			}, "$defs": {
				Type: "object",
			}},
		}, "queryOperation": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"kind", "ownership", "requestSchema", "responseSchema", "httpStatus", "errors"}),
			Properties: map[string]schema.Definition{"kind": {
				Const: "query",
			}, "ownership": {
				Const: "plugin-admin-surface-capability",
			}, "requestSchema": {
				Ref: "#/$defs/jsonSchema",
			}, "responseSchema": {
				Ref: "#/$defs/jsonSchema",
			}, "httpStatus": {
				Const: 200,
			}, "errors": {
				Ref: "#/$defs/errors",
			}},
		}, "actionOperation": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"kind", "ownership", "surfaceBinding", "requestSchema", "responseSchema", "httpStatus", "errors"}),
			Properties: map[string]schema.Definition{"kind": {
				Const: "action",
			}, "ownership": {
				Const: "plugin-admin-surface-action",
			}, "surfaceBinding": {
				Ref: "#/$defs/surfaceBinding",
			}, "requestSchema": {
				Ref: "#/$defs/jsonSchema",
			}, "responseSchema": {
				Ref: "#/$defs/jsonSchema",
			}, "httpStatus": {
				Enum: []any{200, 202},
			}, "acceptedHttpStatus": {
				Const: 202,
			}, "preconditions": {
				Type:        "array",
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Type: "string",
				}),
			}, "idempotency": {
				Type:                 "object",
				AdditionalProperties: false,
				Required:             schema.Value([]string{"required", "scope", "semantics", "repeat"}),
				Properties: map[string]schema.Definition{"required": {
					Const: true,
				}, "scope": {
					Type:        "array",
					UniqueItems: schema.Value(true),
					Items: schema.Value(schema.Definition{
						Type: "string",
					}),
				}, "semantics": {
					Const: "correlation-only",
				}, "repeat": {
					Const: "invoke-again",
				}},
			}, "errors": {
				Ref: "#/$defs/errors",
			}},
		}, "artifactOperation": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"kind", "ownership", "surfaceBinding", "transport", "multipart", "acceptedHttpStatus", "receiptSchema", "archiveLimits", "revisionPrecondition", "idempotency", "errors"}),
			Properties: map[string]schema.Definition{"kind": {
				Const: "artifact-action",
			}, "ownership": {
				Const: "plugin-admin-surface-action",
			}, "surfaceBinding": {
				Ref: "#/$defs/surfaceBinding",
			}, "transport": {
				Const: "plugin-sdk.rest.artifact-stream",
			}, "multipart": {
				Type:                 "object",
				AdditionalProperties: false,
				Required:             schema.Value([]string{"parts", "order", "artifactFilenameForwarded"}),
				Properties: map[string]schema.Definition{"parts": {
					Const: []any{"metadata", "artifact"},
				}, "order": {
					Const: []any{"metadata", "artifact"},
				}, "artifactFilenameForwarded": {
					Const: false,
				}},
			}, "acceptedHttpStatus": {
				Const: 202,
			}, "receiptSchema": {
				Const: "contracts/v1/artifact-operation-result.schema.json",
			}, "archiveLimits": {
				Ref: "#/$defs/archive",
			}, "revisionPrecondition": {
				Const: "if current exists, expectedCurrentRevision is required and must match; for a new site, expectedCurrentRevision must be omitted",
			}, "idempotency": {
				Type:                 "object",
				AdditionalProperties: false,
				Required:             schema.Value([]string{"required", "scope", "sameInput", "differentInput"}),
				Properties: map[string]schema.Definition{"required": {
					Const: true,
				}, "scope": {
					Type:        "array",
					UniqueItems: schema.Value(true),
					Items: schema.Value(schema.Definition{
						Type: "string",
					}),
				}, "sameInput": {
					Type: "string",
				}, "differentInput": {
					Type: "string",
				}},
			}, "errors": {
				Ref: "#/$defs/errors",
			}},
		}, "surfaceBinding": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"page", "section", "action"}),
			Properties: map[string]schema.Definition{"page": {
				Ref: "#/$defs/nonEmptyString",
			}, "section": {
				Ref: "#/$defs/nonEmptyString",
			}, "action": {
				Ref: "#/$defs/nonEmptyString",
			}},
		}, "errors": {
			Type:                 "object",
			MinProperties:        schema.Value(int(1)),
			AdditionalProperties: false,
			Required:             schema.Value([]string{"invalid_input", "not_found", "conflict", "unavailable"}),
			Properties: map[string]schema.Definition{"invalid_input": {
				Ref: "#/$defs/httpError",
			}, "not_found": {
				Ref: "#/$defs/httpError",
			}, "conflict": {
				Ref: "#/$defs/httpError",
			}, "unavailable": {
				Ref: "#/$defs/httpError",
			}, "operation_failed": {
				Ref: "#/$defs/httpError",
			}},
		}, "httpError": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"http", "retryable"}),
			Properties: map[string]schema.Definition{"http": {
				Type:    "integer",
				Minimum: schema.Value(int(400)),
				Maximum: schema.Value(int(599)),
			}, "retryable": {
				Type: "boolean",
			}},
		}, "archive": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"mediaType", "minArtifactBytes", "artifactBytes", "metadataBytes", "multipartOverheadBytes", "requestEnvelopeBytes", "expandedBytes", "maxFiles", "entryCountSemantics", "maxCompressionRatio", "gzip", "tar", "validationChecks", "rejectedEntries"}),
			Properties: map[string]schema.Definition{"mediaType": {
				Const: "application/gzip",
			}, "minArtifactBytes": {
				Const: 1,
			}, "artifactBytes": {
				Const: 134217728,
			}, "metadataBytes": {
				Const: 65536,
			}, "multipartOverheadBytes": {
				Const: 65536,
			}, "requestEnvelopeBytes": {
				Const: 134348800,
			}, "expandedBytes": {
				Const: 536870912,
			}, "maxFiles": {
				Const: 10000,
			}, "entryCountSemantics": {
				Type:                 "object",
				AdditionalProperties: false,
				Required:             schema.Value([]string{"unit", "countedKinds", "implicitParentDirectories"}),
				Properties: map[string]schema.Definition{"unit": {
					Const: "logical-tar-member-after-extension-processing",
				}, "countedKinds": {
					Const: []any{"regular-file", "directory"},
				}, "implicitParentDirectories": {
					Const: "not-counted",
				}},
			}, "maxCompressionRatio": {
				Const: 100,
			}, "gzip": {
				Ref: "#/$defs/gzipSemantics",
			}, "tar": {
				Ref: "#/$defs/tarSemantics",
			}, "validationChecks": {
				Const: []any{"gzip-integrity", "tar-integrity", "streamed-sha256", "metadata-artifact-descriptor-equality", "aggregate-expanded-byte-limit", "file-count-limit", "compression-ratio-limit"},
			}, "rejectedEntries": {
				Const: []any{"absolute-path", "path-traversal", "symbolic-link", "hard-link", "duplicate-path", "case-collision", "unicode-nfc-collision", "special-file"},
			}},
		}, "gzipSemantics": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"members", "integrity", "trailingCompressedBytes"}),
			Properties: map[string]schema.Definition{"members": {
				Const: "concatenated-members-allowed",
			}, "integrity": {
				Const: "drain-all-members-through-eof",
			}, "trailingCompressedBytes": {
				Const: "reject-unless-part-of-valid-gzip-member",
			}},
		}, "tarSemantics": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"archiveCount", "terminator", "postTerminatorBytes"}),
			Properties: map[string]schema.Definition{"archiveCount": {
				Const: "one-tar-stream",
			}, "terminator": {
				Const: "two-consecutive-512-byte-zero-blocks",
			}, "postTerminatorBytes": {
				Const: "zero-padding-only",
			}},
		}, "releaseLifecycle": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"immutableRevisions", "storage", "currentPrevious", "staging", "activation", "failure", "publishStatus"}),
			Properties: map[string]schema.Definition{"immutableRevisions": {
				Const: true,
			}, "storage": {
				Const: "plugin-owned-persistent-filesystem",
			}, "currentPrevious": {
				Const: "successful-publish-or-rollback-atomically-swaps-current-and-previous",
			}, "staging": {
				Const: "validate-complete-staged-release-before-activation",
			}, "activation": {
				Const: "atomic-current-previous-pointer-swap-only-after-successful-terminal-operation",
			}, "failure": {
				Const: "keep-current-and-previous-unchanged",
			}, "publishStatus": {
				Const: 202,
			}},
		}, "publishOperation": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"acceptance", "archiveValidation", "archiveValidationFailure", "currentPointer"}),
			Properties: map[string]schema.Definition{"acceptance": {
				Const: "after-validated-metadata-and-bounded-artifact-bytes-are-durably-stored-and-sha256-matches",
			}, "archiveValidation": {
				Const: "asynchronous-durable-operation-before-release-activation",
			}, "archiveValidationFailure": {
				Const: "operation-failed-with-stable-error-code-current-and-previous-unchanged",
			}, "currentPointer": {
				Const: "changes-only-after-all-archive-and-manifest-validation-succeeds",
			}},
		}, "operationStatus": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"coreResource", "workerPollMilliseconds", "pluginStatusCapability", "pluginStates", "coreStates", "stateMapping", "coreControlUsesPluginSdkRest"}),
			Properties: map[string]schema.Definition{"coreResource": {
				Const: "GET /api/operations/{id}",
			}, "workerPollMilliseconds": {
				Type:    "integer",
				Minimum: schema.Value(int(25)),
				Maximum: schema.Value(int(60000)),
			}, "pluginStatusCapability": {
				Const: "server.operations.get",
			}, "pluginStates": {
				Const: []any{"accepted", "running", "completed", "failed"},
			}, "coreStates": {
				Const: []any{"pending", "running", "succeeded", "failed", "degraded"},
			}, "stateMapping": {
				Const: map[string]any{"accepted": "pending", "running": "running", "completed": "succeeded", "failed": "failed"},
			}, "coreControlUsesPluginSdkRest": {
				Const: true,
			}},
		}},
	}
}

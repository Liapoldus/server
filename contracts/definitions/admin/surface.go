package admin

import "liapoldus.local/server-plugin/contracts/schema"

type AdminSurfaceDocumentPagesItemSectionsItemActionsItemArtifactInput struct {
	MediaTypes                []string `json:"mediaTypes"`
	MaxBytes                  int      `json:"maxBytes"`
	MaxMetadataBytes          int      `json:"maxMetadataBytes"`
	MaxMultipartOverheadBytes int      `json:"maxMultipartOverheadBytes"`
}

type AdminSurfaceDocumentPagesItemSectionsItemActionsItem struct {
	Confirmation  *string                                                            `json:"confirmation,omitempty"`
	ArtifactInput *AdminSurfaceDocumentPagesItemSectionsItemActionsItemArtifactInput `json:"artifactInput,omitempty"`
	RowInput      map[string]string                                                  `json:"rowInput"`
	Dangerous     *bool                                                              `json:"dangerous,omitempty"`
	ID            string                                                             `json:"id"`
	Title         string                                                             `json:"title"`
	Capability    string                                                             `json:"capability"`
	InputSchema   schema.Definition                                                  `json:"inputSchema"`
}

type AdminSurfaceDocumentPagesItemSectionsItem struct {
	ID             string                                                 `json:"id"`
	Kind           string                                                 `json:"kind"`
	Title          string                                                 `json:"title"`
	DataCapability string                                                 `json:"dataCapability"`
	Columns        []string                                               `json:"columns"`
	Actions        []AdminSurfaceDocumentPagesItemSectionsItemActionsItem `json:"actions"`
}

type AdminSurfaceDocumentPagesItem struct {
	ID          string                                      `json:"id"`
	Title       string                                      `json:"title"`
	Capability  string                                      `json:"capability"`
	Permissions []string                                    `json:"permissions"`
	Sections    []AdminSurfaceDocumentPagesItemSectionsItem `json:"sections"`
}

type AdminSurfaceDocument struct {
	Plugin               string                          `json:"plugin"`
	RequiredCapabilities []string                        `json:"requiredCapabilities"`
	Pages                []AdminSurfaceDocumentPagesItem `json:"pages"`
	Version              int                             `json:"version"`
}

func AdminSurface() AdminSurfaceDocument {
	return AdminSurfaceDocument{
		Version:              int(1),
		Plugin:               "server",
		RequiredCapabilities: []string{"admin.surface.get", "server.sites.list", "server.sites.releases.list", "server.sites.rollback", "server.sites.publish", "server.operations.list", "server.operations.get", "server.certificates.list", "server.certificates.get"},
		Pages: []AdminSurfaceDocumentPagesItem{{
			ID:          "sites",
			Title:       "Сайты и релизы",
			Capability:  "server.sites.list",
			Permissions: []string{"plugins.server.read"},
			Sections: []AdminSurfaceDocumentPagesItemSectionsItem{{
				ID:             "sites",
				Kind:           "table",
				Title:          "Сайты",
				DataCapability: "server.sites.list",
				Columns:        []string{"siteId", "currentRevision", "previousRevision", "updatedAt"},
				Actions: []AdminSurfaceDocumentPagesItemSectionsItemActionsItem{{
					ID:           "publish",
					Title:        "Опубликовать .tar.gz",
					Capability:   "server.sites.publish",
					Confirmation: schema.Value("Проверить архив и опубликовать новую неизменяемую ревизию сайта?"),
					InputSchema: schema.Definition{
						Type: "object",
						Properties: map[string]schema.Definition{"siteId": {
							Type:      "string",
							MinLength: schema.Value(int(1)),
							MaxLength: schema.Value(int(128)),
						}, "expectedCurrentRevision": {
							Type:      "string",
							MinLength: schema.Value(int(1)),
							MaxLength: schema.Value(int(256)),
						}},
						Required:             schema.Value([]string{"siteId"}),
						AdditionalProperties: false,
					},
					ArtifactInput: schema.Value(AdminSurfaceDocumentPagesItemSectionsItemActionsItemArtifactInput{
						MediaTypes:                []string{"application/gzip"},
						MaxBytes:                  int(134217728),
						MaxMetadataBytes:          int(65536),
						MaxMultipartOverheadBytes: int(65536),
					}),
					RowInput: map[string]string{
						"siteId": "siteId",
					},
				}, {
					ID:           "rollback",
					Title:        "Откатить релиз",
					Capability:   "server.sites.rollback",
					Confirmation: schema.Value("Атомарно переключить current и previous на выбранную неизменяемую ревизию?"),
					InputSchema: schema.Definition{
						Type: "object",
						Properties: map[string]schema.Definition{"siteId": {
							Type:      "string",
							MinLength: schema.Value(int(1)),
							MaxLength: schema.Value(int(128)),
						}, "expectedCurrentRevision": {
							Type:      "string",
							MinLength: schema.Value(int(1)),
							MaxLength: schema.Value(int(256)),
						}, "targetRevision": {
							Type:      "string",
							MinLength: schema.Value(int(1)),
							MaxLength: schema.Value(int(256)),
						}},
						Required:             schema.Value([]string{"siteId", "expectedCurrentRevision", "targetRevision"}),
						AdditionalProperties: false,
					},
					RowInput: map[string]string{
						"siteId":                  "siteId",
						"expectedCurrentRevision": "currentRevision",
						"targetRevision":          "previousRevision",
					},
					Dangerous: schema.Value(true),
				}},
			}, {
				ID:             "releases",
				Kind:           "table",
				Title:          "Неизменяемые ревизии",
				DataCapability: "server.sites.releases.list",
				Columns:        []string{"siteId", "revisionId", "state", "sha256", "createdAt"},
				Actions:        []AdminSurfaceDocumentPagesItemSectionsItemActionsItem{},
			}, {
				ID:             "operations",
				Kind:           "table",
				Title:          "Операции публикации",
				DataCapability: "server.operations.list",
				Columns:        []string{"operationId", "siteId", "kind", "state", "progressPercent", "updatedAt"},
				Actions: []AdminSurfaceDocumentPagesItemSectionsItemActionsItem{{
					ID:         "status",
					Title:      "Обновить состояние",
					Capability: "server.operations.get",
					InputSchema: schema.Definition{
						Type: "object",
						Properties: map[string]schema.Definition{"operationId": {
							Type:      "string",
							MinLength: schema.Value(int(1)),
							MaxLength: schema.Value(int(256)),
						}},
						Required:             schema.Value([]string{"operationId"}),
						AdditionalProperties: false,
					},
					RowInput: map[string]string{
						"operationId": "operationId",
					},
				}},
			}},
		}, {
			ID:          "certificates",
			Title:       "Сертификаты",
			Capability:  "server.certificates.list",
			Permissions: []string{"plugins.server.tls.read"},
			Sections: []AdminSurfaceDocumentPagesItemSectionsItem{{
				ID:             "domains",
				Kind:           "table",
				Title:          "Домены и готовность TLS",
				DataCapability: "server.certificates.list",
				Columns:        []string{"domain", "source", "readiness", "notAfter", "serial"},
				Actions: []AdminSurfaceDocumentPagesItemSectionsItemActionsItem{{
					ID:         "status",
					Title:      "Проверить домен",
					Capability: "server.certificates.get",
					InputSchema: schema.Definition{
						Type: "object",
						Properties: map[string]schema.Definition{"domain": {
							Type:      "string",
							MinLength: schema.Value(int(1)),
							MaxLength: schema.Value(int(253)),
						}},
						Required:             schema.Value([]string{"domain"}),
						AdditionalProperties: false,
					},
					RowInput: map[string]string{
						"domain": "domain",
					},
				}},
			}},
		}},
	}
}

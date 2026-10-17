// Package definitions owns Server v1 product declarations. JSON files are exports.
package definitions

import (
	"encoding/json"
	admindef "liapoldus.local/server-service/contracts/definitions/admin"
	httpdef "liapoldus.local/server-service/contracts/definitions/http"
	servicedef "liapoldus.local/server-service/contracts/definitions/service"
	settingsdef "liapoldus.local/server-service/contracts/definitions/settings"
	sitedef "liapoldus.local/server-service/contracts/definitions/site"
)

func Documents() map[string]any {
	return map[string]any{
		"admin-actions.json":                    admindef.AdminActions(),
		"admin-actions.schema.json":             admindef.AdminActionsSchema(),
		"admin-query.json":                      admindef.AdminQuery(),
		"admin-query.schema.json":               admindef.AdminQuerySchema(),
		"admin-surface-vectors.json":            admindef.AdminSurfaceVectors(),
		"admin-surface.json":                    admindef.AdminSurface(),
		"admin-surface.schema.json":             admindef.AdminSurfaceSchema(),
		"artifact-metadata.schema.json":         sitedef.ArtifactMetadataSchema(),
		"artifact-operation-result.schema.json": sitedef.ArtifactOperationResultSchema(),
		"http-dispatch.json":                    httpdef.HTTPDispatch(),
		"http-request-parameters.schema.json":   httpdef.HTTPRequestParametersSchema(),
		"http-response-action.schema.json":      httpdef.HTTPResponseActionSchema(),
		"http-stream-vectors.json":              httpdef.HTTPStreamVectors(),
		"http-stream.json":                      httpdef.HTTPStream(),
		"http-stream.schema.json":               httpdef.HTTPStreamSchema(),
		"service.json":                          servicedef.Service(),
		"settings-semantics.json":               settingsdef.SettingsSemantics(),
		"settings-vectors.json":                 settingsdef.SettingsVectors(),
		"settings.schema.json":                  settingsdef.SettingsSchema(),
		"site-manifest-semantics.json":          sitedef.SiteManifestSemantics(),
		"site-manifest-vectors.json":            sitedef.SiteManifestVectors(),
		"site-manifest.schema.json":             sitedef.SiteManifestSchema(),
		"site-publish-manifest.json":            sitedef.SitePublishManifest(),
	}
}

func Bytes(name string) ([]byte, error) {
	value, ok := Documents()[name]
	if !ok {
		return nil, &UnknownDocument{Name: name}
	}
	return json.Marshal(value)
}

type UnknownDocument struct{ Name string }

func (err *UnknownDocument) Error() string { return "unknown Server contract: " + err.Name }

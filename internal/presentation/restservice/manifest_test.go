package restservice

import (
	"context"
	"encoding/json"
	"testing"

	sdkmodels "github.com/Liapoldus/service-sdk/v2/domain/models"
	sdkpresentation "github.com/Liapoldus/service-sdk/v2/presentation"
	"liapoldus.local/server-service/contracts"
	admindef "liapoldus.local/server-service/contracts/definitions/admin"
)

func TestServerManifestPublishesValidatedConfigurationUI(t *testing.T) {
	schema, err := contracts.SettingsSchema()
	if err != nil {
		t.Fatalf("load Server settings schema: %v", err)
	}
	provider, err := sdkpresentation.NewManifestProvider(sdkmodels.ManifestDescriptor{
		ID: "server", Version: ServiceVersion, SecretPurposes: []string{
			string(SettingsHTTPAuthPurpose), string(SettingsCertificatePurpose), string(SettingsPrivateKeyPurpose), string(SettingsUpstreamCAPurpose),
		}, ConfigurationUI: serverConfigurationUI(), Resources: serverResources(),
		EditorButtons: &sdkmodels.ServiceEditorButtonsDescriptor{Version: sdkmodels.ServiceEditorButtonsVersion, Buttons: []sdkmodels.ServiceEditorButton{
			sdkmodels.NewServiceEditorButton("server", "endpoints", sdkmodels.ConfigurationUIText{RU: "Редактор эндпоинтов", EN: "Endpoint editor"}, sdkmodels.ConfigurationUIText{RU: "Открыть редактор эндпоинтов Server", EN: "Open the Server endpoint editor"}),
		}},
	}, schema)
	if err != nil {
		t.Fatalf("validate Server configuration UI against its schema: %v", err)
	}
	manifest, err := provider.Manifest(context.Background())
	if err != nil {
		t.Fatalf("publish Server manifest: %v", err)
	}
	var received sdkmodels.ManifestDescriptor
	if err := json.Unmarshal(manifest, &received); err != nil {
		t.Fatalf("decode published manifest: %v", err)
	}
	if received.Version != ServiceVersion {
		t.Fatalf("manifest release version = %q, want %q", received.Version, ServiceVersion)
	}
	if received.EditorButtons == nil || received.EditorButtons.Version != sdkmodels.ServiceEditorButtonsVersion || len(received.EditorButtons.Buttons) != 1 || received.EditorButtons.Buttons[0].ID != "server.endpoints" {
		t.Fatalf("Server manifest omitted its stable editor button: %#v", received.EditorButtons)
	}
	if received.ConfigurationUI == nil || received.ConfigurationUI.Version != sdkmodels.ConfigurationUIVersion || len(received.ConfigurationUI.Groups) != 4 {
		t.Fatalf("Server manifest omitted its validated configuration UI: %#v", received.ConfigurationUI)
	}
	if got := received.ConfigurationUI.Groups[0].ID; got != "listeners" {
		t.Fatalf("manifest groups are not canonicalized by order: first group is %q", got)
	}
	if len(received.Resources) != 2 || received.Resources[0].Provider != sdkmodels.ResourceProviderServiceAdmin {
		t.Fatalf("manifest omitted the Core-backed site catalog: %#v", received.Resources)
	}
	resourceByID := map[string]sdkmodels.ResourceDescriptor{}
	for _, resource := range received.Resources {
		resourceByID[resource.ID] = resource
	}
	if resourceByID["site-releases"].QueryResource != "releases" || resourceByID["site-releases"].ValuePointer != "/revisionId" {
		t.Fatalf("manifest omitted the immutable release catalog: %#v", resourceByID["site-releases"])
	}
	publication := resourceByID["sites"].Publication
	if publication == nil || publication.PageID != "sites" || publication.ActionID != "publish" {
		t.Fatalf("manifest omitted the explicit site artifact-action binding: %#v", publication)
	}
	surface := admindef.AdminSurface()
	artifactActionFound := false
	for _, page := range surface.Pages {
		if page.ID != publication.PageID {
			continue
		}
		for _, section := range page.Sections {
			for _, action := range section.Actions {
				if action.ID == publication.ActionID && action.ArtifactInput != nil {
					artifactActionFound = true
				}
			}
		}
	}
	if !artifactActionFound {
		t.Fatal("site resource publication does not resolve to a declared artifact action")
	}
	foundSitePicker := false
	foundReleasePicker := false
	for _, group := range received.ConfigurationUI.Groups {
		for _, field := range group.Fields {
			if field.Path == "/config/routes/*/handler/siteId" {
				foundSitePicker = field.Widget == "resource" && field.ResourceKind == "site" && field.ResourceID == "sites"
			}
			if field.Path == "/config/routes/*/handler/releaseId" {
				foundReleasePicker = field.Widget == "resource" && field.ResourceKind == "site-release" && field.ResourceID == "site-releases"
			}
		}
	}
	if !foundSitePicker {
		t.Fatal("static-site handler does not bind its siteId field to the declared resource catalog")
	}
	if !foundReleasePicker {
		t.Fatal("static-site handler does not bind its releaseId field to the immutable release catalog")
	}
	secretPurposes := map[string]bool{}
	for _, purpose := range received.SecretPurposes {
		secretPurposes[purpose] = true
	}
	for _, purpose := range []string{string(SettingsHTTPAuthPurpose), string(SettingsCertificatePurpose), string(SettingsPrivateKeyPurpose), string(SettingsUpstreamCAPurpose)} {
		if !secretPurposes[purpose] {
			t.Fatalf("manifest omitted declared secret purpose %q", purpose)
		}
	}
	secretFields := map[string]string{}
	for _, group := range received.ConfigurationUI.Groups {
		for _, field := range group.Fields {
			if field.Widget == "secret" {
				secretFields[field.Path] = field.SecretPurpose
			}
		}
	}
	wantSecretFields := map[string]string{
		"/config/routes/*/features/auth/secretRef":     string(SettingsHTTPAuthPurpose),
		"/config/defaults/service/auth/secretRef":      string(SettingsHTTPAuthPurpose),
		"/config/defaults/static/auth/secretRef":       string(SettingsHTTPAuthPurpose),
		"/config/defaults/reverseProxy/auth/secretRef": string(SettingsHTTPAuthPurpose),
		"/config/defaults/redirect/auth/secretRef":     string(SettingsHTTPAuthPurpose),
		"/config/listeners/*/tls/certificateRef":       string(SettingsCertificatePurpose),
		"/config/listeners/*/tls/privateKeyRef":        string(SettingsPrivateKeyPurpose),
		"/config/routes/*/handler/upstreams/*/caRef":   string(SettingsUpstreamCAPurpose),
	}
	for path, purpose := range wantSecretFields {
		if secretFields[path] != purpose {
			t.Fatalf("secret UI field %s purpose = %q, want %q", path, secretFields[path], purpose)
		}
	}
}

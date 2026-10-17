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
		ID: "server", Version: ServiceVersion, ConfigurationUI: serverConfigurationUI(), Resources: serverResources(),
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
	if received.ConfigurationUI == nil || received.ConfigurationUI.Version != sdkmodels.ConfigurationUIVersion || len(received.ConfigurationUI.Groups) != 3 {
		t.Fatalf("Server manifest omitted its validated configuration UI: %#v", received.ConfigurationUI)
	}
	if got := received.ConfigurationUI.Groups[0].ID; got != "listeners" {
		t.Fatalf("manifest groups are not canonicalized by order: first group is %q", got)
	}
	if len(received.Resources) != 1 || received.Resources[0].ID != "sites" || received.Resources[0].Provider != sdkmodels.ResourceProviderServiceAdmin {
		t.Fatalf("manifest omitted the Core-backed site catalog: %#v", received.Resources)
	}
	publication := received.Resources[0].Publication
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
	for _, group := range received.ConfigurationUI.Groups {
		for _, field := range group.Fields {
			if field.Path == "/config/routes/*/handler/siteId" {
				foundSitePicker = field.Widget == "resource" && field.ResourceKind == "site" && field.ResourceID == "sites"
			}
		}
	}
	if !foundSitePicker {
		t.Fatal("static-site handler does not bind its siteId field to the declared resource catalog")
	}
}

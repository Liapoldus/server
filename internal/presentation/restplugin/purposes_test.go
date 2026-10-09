package restplugin

import (
	"context"
	"testing"

	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	settingsapp "liapoldus.local/server-plugin/internal/application/settings"
	settingsmodel "liapoldus.local/server-plugin/internal/domain/models/settings"
)

func TestSettingsSecretPurposesPreservation(t *testing.T) {
	if SettingsCertificatePurpose != "server.tls.certificate" || SettingsPrivateKeyPurpose != "server.tls.private-key" || SettingsUpstreamCAPurpose != "server.upstream.ca" {
		t.Fatal("settings grant purposes changed")
	}
}

type purposeTestRuntime struct{ active bool }

func (*purposeTestRuntime) Validate([]byte) error                               { return nil }
func (*purposeTestRuntime) Activate([]byte) error                               { return nil }
func (*purposeTestRuntime) Stop() error                                         { return nil }
func (*purposeTestRuntime) ValidateWithSecrets([]byte, map[string][]byte) error { return nil }
func (runtime *purposeTestRuntime) ActivateWithSecrets([]byte, map[string][]byte) error {
	runtime.active = true
	return nil
}

type purposeTestProvider struct{ purposes map[string]string }

func (provider *purposeTestProvider) SecretProvider(_ context.Context, reference, purpose string) (sdkmodels.SecretValue, error) {
	provider.purposes[reference] = purpose
	return sdkmodels.SecretValue{}, nil
}

func TestSettingsGrantPurposeDelivery(t *testing.T) {
	runtime := &purposeTestRuntime{}
	configuration, err := settingsapp.NewConfiguration(runtime)
	if err != nil {
		t.Fatal(err)
	}
	provider := &purposeTestProvider{purposes: make(map[string]string)}
	adapter := &Adapter{configuration: configuration, secrets: provider}
	settings := settingsmodel.Settings{SchemaVersion: 1, RuntimeConfig: []byte(`{"listeners":[{"tls":{"mode":"custom","certificateRef":"cert","privateKeyRef":"key"}}],"routes":[{"handler":{"type":"reverseProxy","upstreams":[{"caRef":"ca"}]}}]}`)}
	if err := adapter.applyWithSecrets(context.Background(), settings, "generation"); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"cert": "server.tls.certificate", "key": "server.tls.private-key", "ca": "server.upstream.ca"}
	if len(provider.purposes) != len(want) || !runtime.active || configuration.Revision() != "generation" {
		t.Fatal("secret-backed settings activation changed")
	}
	for reference, purpose := range want {
		if provider.purposes[reference] != purpose {
			t.Fatal("wrong settings grant purpose delivered")
		}
	}
}

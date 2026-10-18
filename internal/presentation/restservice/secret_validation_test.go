package restservice

import (
	"bytes"
	"context"
	"errors"
	"testing"

	sdkmodels "github.com/Liapoldus/service-sdk/v2/domain/models"
	settingsapp "liapoldus.local/server-service/internal/application/settings"
	settingsmodel "liapoldus.local/server-service/internal/domain/models/settings"
)

const authCandidateSettings = `{"schemaVersion":2,"config":{"listeners":[{"id":"public","kind":"http","address":"127.0.0.1:19910","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[{"id":"auth","listenerId":"public","handler":{"type":"reverseProxy","upstreams":[{"origin":"http://127.0.0.1:19911","weight":1}]},"features":{"auth":{"mode":"enabled","scheme":"bearer","secretRef":"ssr:v1:00112233445566778899aabbccddeeff"}}}]}}`

const authCandidateReference = "ssr:v1:00112233445566778899aabbccddeeff"

type authCandidateRuntime struct {
	validateCalls           int
	validateWithSecretsCall int
	activateCalls           int
	lastSecret              []byte
}

func (runtime *authCandidateRuntime) Validate([]byte) error {
	runtime.validateCalls++
	return errors.New("secret material is required")
}

func (runtime *authCandidateRuntime) ValidateWithSecrets(_ []byte, secrets map[string][]byte) error {
	runtime.validateWithSecretsCall++
	runtime.lastSecret = secrets[authCandidateReference]
	if !bytes.Equal(runtime.lastSecret, []byte("local-test-only")) {
		return errors.New("scoped secret was not provided")
	}
	return nil
}

func (runtime *authCandidateRuntime) Activate([]byte) error {
	runtime.activateCalls++
	return nil
}

func (runtime *authCandidateRuntime) ActivateWithSecrets([]byte, map[string][]byte) error {
	runtime.activateCalls++
	return nil
}

func (*authCandidateRuntime) Stop() error { return nil }

type authCandidateSecretProvider struct {
	value   []byte
	err     error
	calls   int
	ref     string
	purpose string
}

func (provider *authCandidateSecretProvider) SecretProvider(_ context.Context, reference, purpose string) (sdkmodels.SecretValue, error) {
	provider.calls++
	provider.ref = reference
	provider.purpose = purpose
	if provider.err != nil {
		return sdkmodels.SecretValue{}, provider.err
	}
	return sdkmodels.NewSecretValue(provider.value), nil
}

func TestValidateAndApplyConfigurationRedeemScopedSecretsBeforeActivation(t *testing.T) {
	runtime := &authCandidateRuntime{}
	configuration, err := settingsapp.NewConfiguration(runtime)
	if err != nil {
		t.Fatal(err)
	}
	provider := &authCandidateSecretProvider{value: []byte("local-test-only")}
	adapter := &Adapter{configuration: configuration, secrets: provider}

	if err := adapter.ValidateConfiguration(context.Background(), []byte(authCandidateSettings)); err != nil {
		t.Fatalf("validate auth candidate with a scoped secret: %v", err)
	}
	if !bytes.Equal(runtime.lastSecret, make([]byte, len("local-test-only"))) {
		t.Fatal("preflight did not clear its redeemed secret bytes")
	}
	raw := []byte(authCandidateSettings)
	incoming, err := sdkmodels.NewConfiguration("generation-auth-smoke", "1", sdkmodels.Digest(raw), raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Apply(context.Background(), incoming); err != nil {
		t.Fatalf("apply auth candidate with a scoped secret: %v", err)
	}
	if provider.calls != 2 || provider.ref != authCandidateReference || provider.purpose != string(SettingsHTTPAuthPurpose) {
		t.Fatalf("secret grant requests = calls:%d ref:%q purpose:%q", provider.calls, provider.ref, provider.purpose)
	}
	if runtime.validateCalls != 0 || runtime.validateWithSecretsCall != 2 || runtime.activateCalls != 1 {
		t.Fatalf("preflight runtime calls = validate:%d validateWithSecrets:%d activate:%d", runtime.validateCalls, runtime.validateWithSecretsCall, runtime.activateCalls)
	}
	if !bytes.Equal(runtime.lastSecret, make([]byte, len("local-test-only"))) {
		t.Fatal("apply did not clear its redeemed secret bytes")
	}
}

func TestValidateConfigurationRejectsSecretGrantFailureSafely(t *testing.T) {
	runtime := &authCandidateRuntime{}
	configuration, err := settingsapp.NewConfiguration(runtime)
	if err != nil {
		t.Fatal(err)
	}
	provider := &authCandidateSecretProvider{err: errors.New("grant unavailable")}
	adapter := &Adapter{configuration: configuration, secrets: provider}

	if err := adapter.ValidateConfiguration(context.Background(), []byte(authCandidateSettings)); !errors.Is(err, settingsmodel.ErrInvalidSettings) {
		t.Fatalf("secret grant failure was not mapped to a safe settings error: %v", err)
	}
	if provider.calls != 1 || runtime.validateWithSecretsCall != 0 || runtime.activateCalls != 0 {
		t.Fatalf("failed grant reached runtime validation or activation: calls:%d validate:%d activate:%d", provider.calls, runtime.validateWithSecretsCall, runtime.activateCalls)
	}
}

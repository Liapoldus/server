package settings

import (
	"errors"
	"testing"

	settingsmodel "liapoldus.local/server-service/internal/domain/models/settings"
)

type settingsTestRuntime struct {
	validateErr error
	activateErr error
	validations int
	activations int
}

func (runtime *settingsTestRuntime) Validate([]byte) error {
	runtime.validations++
	return runtime.validateErr
}

func (runtime *settingsTestRuntime) Activate([]byte) error {
	runtime.activations++
	return runtime.activateErr
}

func (*settingsTestRuntime) Stop() error { return nil }

func TestCandidateValidationDoesNotActivateOrChangeRevision(t *testing.T) {
	runtime := &settingsTestRuntime{}
	configuration, err := NewConfiguration(runtime)
	if err != nil {
		t.Fatal(err)
	}
	settings := settingsmodel.Settings{SchemaVersion: 2, RuntimeConfig: []byte(`{"listeners":[],"routes":[]}`)}
	if err := configuration.Apply(settings, "1"); err != nil {
		t.Fatal(err)
	}
	runtime.validateErr = errors.New("refused")
	if !errors.Is(configuration.Validate(settings), ErrCandidateRejected) {
		t.Fatal("candidate was accepted")
	}
	if configuration.revision != "1" || runtime.activations != 1 {
		t.Fatal("validation changed the active runtime")
	}
}

func TestPreflightChecksEnvelopeWithoutCompilingCandidate(t *testing.T) {
	runtime := &settingsTestRuntime{validateErr: errors.New("runtime compile must be deferred")}
	configuration, err := NewConfiguration(runtime)
	if err != nil {
		t.Fatal(err)
	}
	settings := settingsmodel.Settings{SchemaVersion: settingsmodel.SettingsSchemaVersion, RuntimeConfig: []byte(`{"listeners":[],"routes":[]}`)}
	if err := configuration.ValidatePreflight(settings); err != nil {
		t.Fatalf("valid current envelope rejected: %v", err)
	}
	if runtime.validations != 0 || runtime.activations != 0 {
		t.Fatalf("preflight compiled or activated the candidate: validations:%d activations:%d", runtime.validations, runtime.activations)
	}
	settings.SchemaVersion++
	if !errors.Is(configuration.ValidatePreflight(settings), ErrCandidateRejected) {
		t.Fatal("unknown settings schema version passed preflight")
	}
	settings = settingsmodel.Settings{SchemaVersion: settingsmodel.SettingsSchemaVersion}
	if !errors.Is(configuration.ValidatePreflight(settings), ErrCandidateRejected) {
		t.Fatal("empty runtime candidate passed preflight")
	}
}

func TestPreviousSettingsVersionIsBootstrapOnlyAndThenRequiresMigration(t *testing.T) {
	runtime := &settingsTestRuntime{}
	configuration, err := NewConfiguration(runtime)
	if err != nil {
		t.Fatal(err)
	}
	previous := settingsmodel.Settings{SchemaVersion: settingsmodel.BootstrapSettingsVersion, RuntimeConfig: []byte(`{"listeners":[],"routes":[]}`)}
	if !configuration.CanApplyLegacyBootstrap("generation-1") || configuration.Validate(previous) != nil {
		t.Fatal("first activation did not allow the previous settings envelope")
	}
	if err := configuration.Apply(previous, "generation-1"); err != nil {
		t.Fatalf("initial previous-version activation failed: %v", err)
	}
	if configuration.CanApplyLegacyBootstrap("generation-2") || !configuration.CanApplyLegacyBootstrap("generation-1") {
		t.Fatal("previous settings envelope was not limited to the initial generation and exact replay")
	}
	if err := configuration.Apply(previous, "generation-1"); err != nil {
		t.Fatalf("exact previous-version generation replay failed: %v", err)
	}
	changed := previous
	changed.RuntimeConfig = []byte(`{"listeners":[{"id":"changed"}],"routes":[]}`)
	if !errors.Is(configuration.Validate(changed), ErrCandidateRejected) {
		t.Fatal("changed previous-version settings passed candidate validation")
	}
	if !errors.Is(configuration.Apply(previous, "generation-2"), ErrCandidateRejected) || configuration.Revision() != "generation-1" || runtime.activations != 1 {
		t.Fatal("a new previous-version generation replaced active configuration")
	}
	current := settingsmodel.Settings{SchemaVersion: settingsmodel.SettingsSchemaVersion, RuntimeConfig: previous.RuntimeConfig}
	if err := configuration.Apply(current, "generation-2"); err != nil || configuration.Revision() != "generation-2" || runtime.activations != 2 {
		t.Fatalf("current settings version did not complete the reviewed migration: err=%v", err)
	}
}

func TestConfigurationErrorsPreservation(t *testing.T) {
	cases := []struct {
		err     error
		message string
	}{
		{ErrInvalidRevision, "invalid Caddy settings revision"},
		{ErrRevisionConflict, "Caddy settings revision conflict"},
		{ErrCandidateRejected, "Caddy candidate configuration rejected"},
		{ErrActivationFailed, "Caddy configuration activation failed"},
	}
	for _, tc := range cases {
		if tc.err.Error() != tc.message {
			t.Fatal("configuration error message changed")
		}
	}
	runtime := &settingsTestRuntime{}
	configuration, err := NewConfiguration(runtime)
	if err != nil {
		t.Fatal(err)
	}
	settings := settingsmodel.Settings{SchemaVersion: 2, RuntimeConfig: []byte(`{"listeners":[],"routes":[]}`)}
	if !errors.Is(configuration.Apply(settings, ""), ErrInvalidRevision) {
		t.Fatal("empty revision accepted")
	}
	if err := configuration.Apply(settings, "active"); err != nil {
		t.Fatal(err)
	}
	if err := configuration.Apply(settings, "active"); err != nil || runtime.validations != 1 || runtime.activations != 1 {
		t.Fatal("revision replay changed")
	}
	changed := settings
	changed.RuntimeConfig = []byte(`{"routes":[],"listeners":[]}`)
	if !errors.Is(configuration.Apply(changed, "active"), ErrRevisionConflict) {
		t.Fatal("raw-byte revision conflict changed")
	}
	runtime.validateErr = errors.New("candidate details must stay private")
	if !errors.Is(configuration.Apply(settings, "candidate"), ErrCandidateRejected) || configuration.Revision() != "active" || runtime.activations != 1 {
		t.Fatal("candidate rejection replaced active revision")
	}
	runtime.validateErr = nil
	runtime.activateErr = errors.New("runtime details must stay private")
	if !errors.Is(configuration.Apply(settings, "candidate"), ErrActivationFailed) || configuration.Revision() != "active" {
		t.Fatal("activation failure replaced active revision")
	}
}

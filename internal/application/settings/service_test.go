package settings

import (
	"errors"
	"testing"

	settingsmodel "liapoldus.local/server-plugin/internal/domain/models/settings"
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
	settings := settingsmodel.Settings{SchemaVersion: 1, RuntimeConfig: []byte(`{"listeners":[],"routes":[]}`)}
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

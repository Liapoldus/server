package restplugin

import (
	"errors"
	"testing"

	"liapoldus.local/server-plugin/contracts"
)

func TestSettingsAdapterErrorsPreservation(t *testing.T) {
	if ErrSecretReferencesRequireCoreGrantAPI.Error() != "Server configuration contains secret references but Core grant delivery is unavailable" {
		t.Fatal("grant-delivery error message changed")
	}
	_, err := New(nil, nil)
	if !errors.Is(err, contracts.ErrInvalidAssets) || err.Error() != "" {
		t.Fatal("invalid adapter error changed")
	}
}

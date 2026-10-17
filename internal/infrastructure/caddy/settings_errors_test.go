package caddy

import (
	"errors"
	"testing"
)

func TestSettingsErrorMessagesPreservation(t *testing.T) {
	if errUnsupportedSettings.Error() != "unsupported Caddy settings" || errInvalidRequestPath.Error() != "" {
		t.Fatal("settings or request-path error message changed")
	}
	_, err := compileHandler("route", settingsHandler{Type: "unknown"}, nil, nil, nil, nil, nil)
	if !errors.Is(err, errUnsupportedSettings) || err.Error() != "unsupported Caddy settings: handler" {
		t.Fatal("unsupported handler error changed")
	}
}

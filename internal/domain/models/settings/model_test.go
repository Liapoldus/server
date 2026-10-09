package settings

import (
	"errors"
	"testing"
)

func TestDecodeSettingsPreservation(t *testing.T) {
	if ErrInvalidSettings.Error() != "invalid Caddy plugin settings" {
		t.Fatal("settings error message changed")
	}
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty object", `{"schemaVersion":1,"config":{}}`, `{}`},
		{"raw bytes", `{"config":{ "routes" : [], "listeners": [] }, "schemaVersion":1}`, `{ "routes" : [], "listeners": [] }`},
		{"invalid JSON", `{"schemaVersion":1`, ""},
		{"null envelope", `null`, ""},
		{"array envelope", `[]`, ""},
		{"missing version", `{"config":{}}`, ""},
		{"missing config", `{"schemaVersion":1}`, ""},
		{"unknown field", `{"schemaVersion":1,"config":{},"extra":true}`, ""},
		{"wrong version", `{"schemaVersion":2,"config":{}}`, ""},
		{"string version", `{"schemaVersion":"1","config":{}}`, ""},
		{"fractional version", `{"schemaVersion":1.5,"config":{}}`, ""},
		{"null config", `{"schemaVersion":1,"config":null}`, ""},
		{"array config", `{"schemaVersion":1,"config":[]}`, ""},
		{"string config", `{"schemaVersion":1,"config":"{}"}`, ""},
		{"native Caddy envelope", `{"apps":{},"admin":{}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings, err := DecodeSettings([]byte(tc.input))
			if tc.want == "" {
				if !errors.Is(err, ErrInvalidSettings) || settings.SchemaVersion != 0 || settings.RuntimeConfig != nil {
					t.Fatal("invalid envelope did not return the existing settings sentinel and zero value")
				}
				return
			}
			if err != nil || settings.SchemaVersion != 1 || string(settings.RuntimeConfig) != tc.want {
				t.Fatal("decoded version or exact runtime config bytes changed")
			}
		})
	}
}

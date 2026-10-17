package contracts

import (
	"encoding/json"
	"testing"
)

func TestSettingsSchemaValidatesSharedFeatureDefaultsAndSavedOverrides(t *testing.T) {
	document := map[string]any{
		"schemaVersion": 2,
		"config": map[string]any{
			"listeners": []any{map[string]any{
				"id": "public-http", "kind": "http", "address": ":9080", "hostnames": []any{}, "protocols": []any{"http1"}, "tls": map[string]any{"mode": "disabled"},
			}},
			"routes": []any{map[string]any{
				"id": "static-docs", "listenerId": "public-http",
				"handler": map[string]any{"type": "static", "siteId": "docs", "releaseId": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "mountPrefix": "/docs"},
				"features": map[string]any{
					"compression": map[string]any{"mode": "disabled", "algorithms": []any{"gzip"}},
					"cache":       map[string]any{"mode": "disabled", "visibility": "private", "maxAgeSeconds": 120},
				},
			}},
			"defaults": map[string]any{
				"static": map[string]any{
					"compression": map[string]any{"mode": "enabled", "algorithms": []any{"zstd", "gzip"}},
					"cache":       map[string]any{"mode": "enabled", "visibility": "public", "maxAgeSeconds": 3600},
				},
			},
		},
	}
	assertValid := func(t *testing.T, value any, wantValid bool) {
		t.Helper()
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		err = ValidateSettings(encoded)
		if wantValid && err != nil {
			t.Fatalf("expected settings to satisfy the contract: %v", err)
		}
		if !wantValid && err == nil {
			t.Fatal("invalid feature configuration satisfied the settings contract")
		}
	}

	assertValid(t, document, true)

	invalidDefaults := cloneSettingsTestMap(t, document)
	invalidDefaults["config"].(map[string]any)["defaults"].(map[string]any)["static"].(map[string]any)["compression"].(map[string]any)["mode"] = "inherit"
	assertValid(t, invalidDefaults, false)

	invalidAlgorithm := cloneSettingsTestMap(t, document)
	invalidAlgorithm["config"].(map[string]any)["routes"].([]any)[0].(map[string]any)["features"].(map[string]any)["compression"].(map[string]any)["algorithms"] = []any{"brotli"}
	assertValid(t, invalidAlgorithm, false)

	invalidAge := cloneSettingsTestMap(t, document)
	invalidAge["config"].(map[string]any)["routes"].([]any)[0].(map[string]any)["features"].(map[string]any)["cache"].(map[string]any)["maxAgeSeconds"] = 31536001
	assertValid(t, invalidAge, false)
}

func cloneSettingsTestMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

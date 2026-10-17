package restservice

import (
	"bytes"
	"context"
	"errors"
	"testing"

	sdkmodels "github.com/Liapoldus/service-sdk/v2/domain/models"
	settingsapp "liapoldus.local/server-service/internal/application/settings"
)

type generationRuntime struct{ active []byte }

func (*generationRuntime) Validate([]byte) error { return nil }
func (runtime *generationRuntime) Activate(contents []byte) error {
	runtime.active = bytes.Clone(contents)
	return nil
}
func (*generationRuntime) Stop() error { return nil }

func TestApplyPreservesOpaqueGeneration(t *testing.T) {
	// Names inside opaque IDs are application data, never migration targets.
	config := []byte(`{ "routes": [], "listeners": [{"id":"plugin-marker","kind":"http","address":":8080","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}] }`)
	raw := append([]byte(" \n{\"config\":"), config...)
	raw = append(raw, []byte(",\"schemaVersion\":2}\n")...)
	digest := sdkmodels.Digest(raw)
	incoming, err := sdkmodels.NewConfiguration("generation-opaque", "1", digest, raw)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &generationRuntime{}
	configuration, err := settingsapp.NewConfiguration(runtime)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &Adapter{configuration: configuration}
	if err := adapter.Apply(context.Background(), incoming); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(incoming.Bytes(), raw) || incoming.SHA256 != digest || !bytes.Equal(runtime.active, config) {
		t.Fatal("immutable bytes, digest or opaque application ID changed")
	}
	changed := bytes.Replace(raw, []byte(`{ "routes"`), []byte(`{"routes"`), 1)
	conflict, err := sdkmodels.NewConfiguration(incoming.Generation, "1", sdkmodels.Digest(changed), changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Apply(context.Background(), conflict); !errors.Is(err, settingsapp.ErrRevisionConflict) {
		t.Fatalf("changed bytes reused an immutable generation: %v", err)
	}
	if !bytes.Equal(runtime.active, config) {
		t.Fatal("rejected generation replaced active bytes")
	}
}

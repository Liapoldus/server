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

type generationRuntime struct {
	active      []byte
	activations int
}

func (*generationRuntime) Validate([]byte) error { return nil }
func (runtime *generationRuntime) Activate(contents []byte) error {
	runtime.active = bytes.Clone(contents)
	runtime.activations++
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

func TestAdapterAllowsPreviousSettingsOnlyForInitialActiveGeneration(t *testing.T) {
	previous := []byte(`{"schemaVersion":1,"config":{"listeners":[{"id":"public-http","kind":"http","address":":9080","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[]}}`)
	runtime := &generationRuntime{}
	configuration, err := settingsapp.NewConfiguration(runtime)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &Adapter{configuration: configuration}
	if err := adapter.ValidateConfiguration(context.Background(), previous); err != nil {
		t.Fatalf("validate bootstrap settings: %v", err)
	}
	incoming, err := sdkmodels.NewConfiguration("generation-1", "1", sdkmodels.Digest(previous), previous)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Apply(context.Background(), incoming); err != nil {
		t.Fatalf("apply existing active generation: %v", err)
	}
	if !bytes.Equal(runtime.active, []byte(`{"listeners":[{"id":"public-http","kind":"http","address":":9080","hostnames":[],"protocols":["http1"],"tls":{"mode":"disabled"}}],"routes":[]}`)) || runtime.activations != 1 {
		t.Fatal("bootstrap changed runtime configuration bytes or activated more than once")
	}
	if err := adapter.ValidateConfiguration(context.Background(), previous); err != nil {
		t.Fatalf("validate exact active generation replay: %v", err)
	}
	changedPrevious := bytes.Replace(previous, []byte(":9080"), []byte(":9081"), 1)
	if err := adapter.ValidateConfiguration(context.Background(), changedPrevious); !errors.Is(err, settingsmodel.ErrInvalidSettings) {
		t.Fatal("changed previous-version settings passed validation after bootstrap")
	}
	newPrevious, err := sdkmodels.NewConfiguration("generation-2", "1", sdkmodels.Digest(changedPrevious), changedPrevious)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Apply(context.Background(), newPrevious); !errors.Is(err, settingsmodel.ErrInvalidSettings) || runtime.activations != 1 || configuration.Revision() != "generation-1" {
		t.Fatal("new previous-version generation replaced the active runtime")
	}
	current := bytes.Replace(previous, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":2`), 1)
	newCurrent, err := sdkmodels.NewConfiguration("generation-2", "1", sdkmodels.Digest(current), current)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Apply(context.Background(), newCurrent); err != nil || runtime.activations != 2 || configuration.Revision() != "generation-2" {
		t.Fatalf("current-version migration did not apply: err=%v", err)
	}
}

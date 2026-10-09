package main

import (
	"encoding/json"
	"io"
	"os"

	settingsapp "liapoldus.local/server-plugin/internal/application/settings"
	settingsmodel "liapoldus.local/server-plugin/internal/domain/models/settings"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

type observingRuntime struct {
	active        json.RawMessage
	lastValidated json.RawMessage
	validateCalls int
	activateCalls int
}

func (runtime *observingRuntime) Validate(contents []byte) error {
	runtime.validateCalls++
	runtime.lastValidated = append(runtime.lastValidated[:0], contents...)
	return nil
}

func (runtime *observingRuntime) Activate(contents []byte) error {
	runtime.activateCalls++
	runtime.active = append(runtime.active[:0], contents...)
	return nil
}

func (*observingRuntime) Stop() error { return nil }

type call struct {
	Settings map[string]any `json:"settings"`
	Revision string         `json:"revision"`
}

type request struct {
	Calls []call `json:"calls"`
}

type callResult struct {
	Revision string `json:"revision"`
	Code     string `json:"code"`
	Applied  bool   `json:"applied"`
}

func main() {
	contents, err := io.ReadAll(os.Stdin)
	check(err)
	var input request
	check(json.Unmarshal(contents, &input))

	runtime := &observingRuntime{}
	configuration, err := settingsapp.NewConfiguration(runtime)
	check(err)

	results := make([]callResult, 0, len(input.Calls))
	for _, item := range input.Calls {
		settings, marshalErr := json.Marshal(item.Settings)
		check(marshalErr)
		applyErr := shared.Apply(configuration, settings, item.Revision, nil)
		if applyErr != nil {
			results = append(results, callResult{Code: "InvalidArgument"})
			continue
		}
		results = append(results, callResult{
			Applied: true, Revision: item.Revision, Code: "OK",
		})
	}

	var active any
	if len(runtime.active) > 0 {
		check(json.Unmarshal(runtime.active, &active))
	}
	var lastValidated any
	if len(runtime.lastValidated) > 0 {
		check(json.Unmarshal(runtime.lastValidated, &lastValidated))
	}
	check(json.NewEncoder(os.Stdout).Encode(map[string]any{
		"calls":          results,
		"activeRevision": configuration.Revision(),
		"activeConfig":   active,
		"runtime": map[string]any{
			"validateCalls": runtime.validateCalls,
			"activateCalls": runtime.activateCalls,
			"lastValidated": lastValidated,
		},
	}))
}

func check(err error) {
	if err != nil {
		panic(settingsmodel.ErrInvalidSettings)
	}
}

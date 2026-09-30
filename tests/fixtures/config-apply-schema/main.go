package main

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"liapoldus.local/server-plugin/internal/application"
	"liapoldus.local/server-plugin/internal/domain/models"
	pluginadapter "liapoldus.local/server-plugin/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type observingRuntime struct {
	validateCalls int
	activateCalls int
	active        json.RawMessage
	lastValidated json.RawMessage
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
	Revision string         `json:"revision"`
	Settings map[string]any `json:"settings"`
}

type request struct {
	Calls []call `json:"calls"`
}

type callResult struct {
	Applied  bool   `json:"applied"`
	Revision string `json:"revision"`
	Code     string `json:"code"`
}

func main() {
	contents, err := io.ReadAll(os.Stdin)
	check(err)
	var input request
	check(json.Unmarshal(contents, &input))

	runtime := &observingRuntime{}
	configuration, err := application.NewConfiguration(runtime)
	check(err)
	service, err := pluginadapter.New(configuration, func() {})
	check(err)

	results := make([]callResult, 0, len(input.Calls))
	for _, item := range input.Calls {
		settings, marshalErr := json.Marshal(item.Settings)
		check(marshalErr)
		response, applyErr := service.ConfigApply(context.Background(), &pluginv1.ConfigApplyRequest{
			Config: settings, SettingsRevision: item.Revision,
		})
		if applyErr != nil {
			results = append(results, callResult{Code: status.Code(applyErr).String()})
			continue
		}
		results = append(results, callResult{
			Applied: response.GetApplied(), Revision: response.GetSettingsRevision(), Code: codes.OK.String(),
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
		panic(models.ErrInvalidSettings)
	}
}

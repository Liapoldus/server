package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"liapoldus.local/server-plugin/internal/application"
	"liapoldus.local/server-plugin/internal/domain/models"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

type fakeRuntime struct{ active []byte }

func (*fakeRuntime) Validate(contents []byte) error {
	var config struct {
		Listeners []struct {
			ID string `json:"id"`
		} `json:"listeners"`
	}
	if json.Unmarshal(contents, &config) != nil {
		return models.ErrInvalidSettings
	}
	if len(config.Listeners) > 0 && config.Listeners[0].ID == "reject" {
		return errors.New("runtime rejected candidate")
	}
	return nil
}

func (runtime *fakeRuntime) Activate(contents []byte) error {
	if err := runtime.Validate(contents); err != nil {
		return err
	}
	runtime.active = append([]byte(nil), contents...)
	return nil
}

func (*fakeRuntime) Stop() error { return nil }

type call struct {
	Revision string         `json:"revision"`
	Settings map[string]any `json:"settings"`
}

type request struct {
	Calls []call `json:"calls"`
}

type result struct {
	Applied  bool   `json:"applied"`
	Revision string `json:"revision"`
	Code     string `json:"code"`
}

func main() {
	contents, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}
	var input request
	if err := json.Unmarshal(contents, &input); err != nil {
		panic(err)
	}
	runtime := &fakeRuntime{}
	configuration, err := application.NewConfiguration(runtime)
	if err != nil {
		panic(err)
	}
	results := make([]result, 0, len(input.Calls))
	for _, item := range input.Calls {
		payload, err := json.Marshal(item.Settings)
		if err != nil {
			panic(err)
		}
		err = shared.Apply(configuration, payload, item.Revision, nil)
		if err != nil {
			code := "InvalidArgument"
			if errors.Is(err, application.ErrRevisionConflict) || errors.Is(err, application.ErrInvalidRevision) {
				code = "FailedPrecondition"
			}
			results = append(results, result{Code: code})
			continue
		}
		results = append(results, result{Applied: true, Revision: item.Revision, Code: "OK"})
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"calls": results, "activeRevision": configuration.Revision(), "activeConfig": string(runtime.active)}); err != nil {
		panic(err)
	}
}

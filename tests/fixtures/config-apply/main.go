package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/Liapoldus/caddy-plugin/internal/application"
	"github.com/Liapoldus/caddy-plugin/internal/domain/models"
	pluginadapter "github.com/Liapoldus/caddy-plugin/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeRuntime struct{ active []byte }

func (*fakeRuntime) Validate(contents []byte) error {
	var config map[string]any
	if json.Unmarshal(contents, &config) != nil {
		return models.ErrInvalidSettings
	}
	if _, rejected := config["reject"]; rejected {
		return errors.New("candidate rejected")
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
	service, err := pluginadapter.New(configuration, func() {})
	if err != nil {
		panic(err)
	}
	results := make([]result, 0, len(input.Calls))
	for _, item := range input.Calls {
		payload, err := json.Marshal(item.Settings)
		if err != nil {
			panic(err)
		}
		response, err := service.ConfigApply(context.Background(), &pluginv1.ConfigApplyRequest{Config: payload, SettingsRevision: item.Revision})
		if err != nil {
			results = append(results, result{Code: status.Code(err).String()})
			continue
		}
		results = append(results, result{Applied: response.GetApplied(), Revision: response.GetSettingsRevision(), Code: codes.OK.String()})
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"calls": results, "activeRevision": configuration.Revision(), "activeConfig": string(runtime.active)}); err != nil {
		panic(err)
	}
}

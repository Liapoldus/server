package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Liapoldus/caddy-plugin/internal/application"
	"github.com/Liapoldus/caddy-plugin/internal/domain/models"
	caddyruntime "github.com/Liapoldus/caddy-plugin/internal/infrastructure/caddy"
	pluginadapter "github.com/Liapoldus/caddy-plugin/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
)

type input struct {
	Port int `json:"port"`
}

func main() {
	contents, err := io.ReadAll(os.Stdin)
	check(err)
	var request input
	check(json.Unmarshal(contents, &request))
	runtime := caddyruntime.New()
	configuration, err := application.NewConfiguration(runtime)
	check(err)
	service, err := pluginadapter.New(configuration, func() {})
	check(err)
	settings, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"admin": map[string]any{"disabled": true},
			"apps": map[string]any{"http": map[string]any{"servers": map[string]any{"test": map[string]any{
				"listen": []string{fmt.Sprintf("127.0.0.1:%d", request.Port)},
				"routes": []any{map[string]any{"handle": []any{map[string]any{"handler": "static_response", "body": "plugin-owned"}}}},
			}}}},
		},
	})
	check(err)
	result, err := service.ConfigApply(context.Background(), &pluginv1.ConfigApplyRequest{Config: settings, SettingsRevision: "revision-1"})
	check(err)
	defer func() { _ = configuration.Stop() }()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", request.Port))
	check(err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	check(err)
	check(json.NewEncoder(os.Stdout).Encode(map[string]any{
		"revision": result.GetSettingsRevision(), "status": response.StatusCode, "body": string(body),
	}))
}

func check(err error) {
	if err != nil {
		panic(models.ErrInvalidSettings)
	}
}

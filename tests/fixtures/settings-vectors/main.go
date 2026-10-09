package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	settingsmodel "liapoldus.local/server-plugin/internal/domain/models/settings"
	caddyruntime "liapoldus.local/server-plugin/internal/infrastructure/caddy"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

type fixtureInput struct {
	Request struct {
		Method string `json:"method"`
		Host   string `json:"host"`
		Target string `json:"target"`
	} `json:"request"`
	Mode     string          `json:"mode"`
	Settings json.RawMessage `json:"settings"`
	Port     int             `json:"port"`
}

func main() {
	_, cleanup, err := shared.IsolateCaddyDataHome()
	check(err)
	defer cleanup()

	contents, err := io.ReadAll(os.Stdin)
	check(err)
	var input fixtureInput
	check(json.Unmarshal(contents, &input))
	settings, err := settingsmodel.DecodeSettings(input.Settings)
	check(err)
	runtime := caddyruntime.New()
	if input.Mode == "validate" {
		err = runtime.Validate(settings.RuntimeConfig)
		check(json.NewEncoder(os.Stdout).Encode(map[string]any{"accepted": err == nil}))
		return
	}
	check(runtime.Activate(settings.RuntimeConfig))
	defer func() { _ = runtime.Stop() }()
	method := input.Request.Method
	if method == "" {
		method = http.MethodGet
	}
	request, err := http.NewRequest(method, "http://127.0.0.1:"+portString(input.Port)+input.Request.Target, nil)
	check(err)
	request.Host = input.Request.Host
	client := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	check(err)
	defer response.Body.Close()
	check(json.NewEncoder(os.Stdout).Encode(map[string]any{
		"status": response.StatusCode, "location": response.Header.Get("Location"),
	}))
}

func portString(port int) string {
	return strconv.Itoa(port)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

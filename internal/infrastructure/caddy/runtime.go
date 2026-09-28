package caddy

import (
	"encoding/json"
	"errors"

	caddycore "github.com/caddyserver/caddy/v2"
	_ "github.com/caddyserver/caddy/v2/modules/standard"
	_ "github.com/mholt/caddy-l4"
)

type Runtime struct{}

func New() *Runtime { return &Runtime{} }

func (runtime *Runtime) Validate(configuration []byte) error {
	prepared, err := disableAutosave(configuration)
	if err != nil {
		return err
	}
	var parsed caddycore.Config
	if err := json.Unmarshal(prepared, &parsed); err != nil {
		return err
	}
	return caddycore.Validate(&parsed)
}

func (runtime *Runtime) Activate(configuration []byte) error {
	prepared, err := disableAutosave(configuration)
	if err != nil {
		return err
	}
	return caddycore.Load(prepared, true)
}

func (runtime *Runtime) Stop() error { return caddycore.Stop() }

func disableAutosave(configuration []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(configuration, &root); err != nil || root == nil {
		return nil, errors.New("")
	}
	var admin map[string]json.RawMessage
	if raw := root["admin"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &admin); err != nil || admin == nil {
			return nil, errors.New("")
		}
	}
	var settings map[string]json.RawMessage
	if raw := admin["config"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil || settings == nil {
			return nil, errors.New("")
		}
	}
	if settings == nil {
		settings = make(map[string]json.RawMessage)
	}
	settings["persist"] = json.RawMessage("false")
	adminConfig, err := json.Marshal(settings)
	if err != nil {
		return nil, errors.New("")
	}
	admin["config"] = adminConfig
	adminJSON, err := json.Marshal(admin)
	if err != nil {
		return nil, errors.New("")
	}
	root["admin"] = adminJSON
	prepared, err := json.Marshal(root)
	if err != nil {
		return nil, errors.New("")
	}
	return prepared, nil
}

package caddy

import (
	"encoding/json"
	"errors"
	"sync"

	caddycore "github.com/caddyserver/caddy/v2"
	_ "github.com/caddyserver/caddy/v2/modules/standard"
)

type Runtime struct {
	mu      sync.RWMutex
	targets []DispatchTarget
}

func New() *Runtime { return &Runtime{} }

func (runtime *Runtime) SetDispatchTargets(targets []DispatchTarget) error {
	if runtime == nil {
		return errUnsupportedSettings
	}
	copyTargets := append([]DispatchTarget(nil), targets...)
	seen := make(map[string]struct{}, len(copyTargets))
	for _, target := range copyTargets {
		if target.ID == "" || target.Endpoint == "" {
			return errUnsupportedSettings
		}
		if _, exists := seen[target.ID]; exists {
			return errUnsupportedSettings
		}
		seen[target.ID] = struct{}{}
	}
	runtime.mu.Lock()
	runtime.targets = copyTargets
	runtime.mu.Unlock()
	return nil
}

func (runtime *Runtime) Validate(configuration []byte) error {
	return runtime.ValidateWithSecrets(configuration, nil)
}

func (runtime *Runtime) ValidateWithSecrets(configuration []byte, secrets map[string][]byte) error {
	compiled, err := runtime.compile(configuration, secrets)
	if err != nil {
		return err
	}
	defer clear(compiled)
	prepared, err := disableAutosave(compiled)
	if err != nil {
		return err
	}
	defer clear(prepared)
	var parsed caddycore.Config
	if err := json.Unmarshal(prepared, &parsed); err != nil {
		return err
	}
	return caddycore.Validate(&parsed)
}

func (runtime *Runtime) Activate(configuration []byte) error {
	return runtime.ActivateWithSecrets(configuration, nil)
}

func (runtime *Runtime) ActivateWithSecrets(configuration []byte, secrets map[string][]byte) error {
	compiled, err := runtime.compile(configuration, secrets)
	if err != nil {
		return err
	}
	defer clear(compiled)
	prepared, err := disableAutosave(compiled)
	if err != nil {
		return err
	}
	defer clear(prepared)
	return caddycore.Load(prepared, true)
}

func (runtime *Runtime) compile(configuration []byte, secrets map[string][]byte) ([]byte, error) {
	if runtime == nil {
		return nil, errUnsupportedSettings
	}
	runtime.mu.RLock()
	targets := append([]DispatchTarget(nil), runtime.targets...)
	runtime.mu.RUnlock()
	return compileSettings(configuration, targets, secrets)
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

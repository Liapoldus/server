package caddy

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/caddyserver/caddy/v2/modules/caddytls"

	caddycore "github.com/caddyserver/caddy/v2"
	_ "github.com/caddyserver/caddy/v2/modules/standard"
	certificatemodel "liapoldus.local/server-plugin/internal/domain/models/certificate"
)

type Runtime struct {
	targets      []DispatchTarget
	certificates []certificateBinding
	id           uint64
	mu           sync.RWMutex
}

type certificateBinding struct {
	domain string
	source string
}

var nextRuntimeID atomic.Uint64

func New() *Runtime { return &Runtime{id: nextRuntimeID.Add(1)} }

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
		if target.Security.PlaintextLoopback {
			if !endpointIsLoopback(target.Endpoint) {
				return errUnsupportedSettings
			}
		} else if target.Security.Identity == "" || target.Security.PeerIdentity == "" ||
			target.Security.Certificate.PrivateKey == nil || target.Security.Roots == nil {
			return errUnsupportedSettings
		}
		if _, exists := seen[target.ID]; exists {
			return errUnsupportedSettings
		}
		seen[target.ID] = struct{}{}
	}
	runtime.mu.Lock()
	runtime.targets = copyTargets
	registerDispatchSecurity(runtime.id, copyTargets)
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
	var desired settingsConfig
	if err := json.Unmarshal(configuration, &desired); err != nil {
		return errUnsupportedSettings
	}
	if err := caddycore.Load(prepared, true); err != nil {
		return err
	}
	runtime.mu.Lock()
	runtime.certificates = configuredCertificateBindings(desired)
	runtime.mu.Unlock()
	return nil
}

func configuredCertificateBindings(desired settingsConfig) []certificateBinding {
	seen := make(map[string]struct{})
	bindings := make([]certificateBinding, 0)
	for _, listener := range desired.Listeners {
		source := ""
		switch listener.TLS.Mode {
		case "automatic":
			source = "acme"
		case "custom":
			source = "custom"
		default:
			continue
		}
		for _, domain := range listener.Hostnames {
			domain = strings.ToLower(domain)
			if domain == "" {
				continue
			}
			if _, exists := seen[domain]; exists {
				continue
			}
			seen[domain] = struct{}{}
			bindings = append(bindings, certificateBinding{domain: domain, source: source})
		}
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].domain < bindings[j].domain })
	return bindings
}

func (runtime *Runtime) ListCertificates(ctx context.Context, domain string, limit int, cursor string) (certificatemodel.CertificatePage, error) {
	if runtime == nil || ctx == nil || limit < 1 || limit > 100 || len(domain) > 253 {
		return certificatemodel.CertificatePage{}, certificatemodel.ErrInvalidCertificateQuery
	}
	if err := ctx.Err(); err != nil {
		return certificatemodel.CertificatePage{}, certificatemodel.ErrInvalidCertificateQuery
	}
	runtime.mu.RLock()
	bindings := append([]certificateBinding(nil), runtime.certificates...)
	runtime.mu.RUnlock()
	items := make([]certificatemodel.CertificateSummary, 0, len(bindings))
	for _, binding := range bindings {
		if domain != "" && binding.domain != strings.ToLower(domain) {
			continue
		}
		status := runtime.certificateStatus(binding)
		items = append(items, certificatemodel.CertificateSummary{Domain: status.Domain, Source: status.Source,
			Readiness: status.Readiness, NotAfter: status.NotAfter, Serial: status.Serial})
	}
	start := 0
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(decoded) == 0 || len(decoded) > 253 {
			return certificatemodel.CertificatePage{}, certificatemodel.ErrInvalidCertificateQuery
		}
		for start < len(items) && items[start].Domain <= string(decoded) {
			start++
		}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := certificatemodel.CertificatePage{Items: append([]certificatemodel.CertificateSummary(nil), items[start:end]...)}
	if end < len(items) && end > start {
		next := base64.RawURLEncoding.EncodeToString([]byte(items[end-1].Domain))
		page.NextCursor = &next
	}
	return page, nil
}

func (runtime *Runtime) CertificateStatus(ctx context.Context, domain string) (certificatemodel.CertificateStatus, error) {
	if runtime == nil || ctx == nil || domain == "" || len(domain) > 253 || ctx.Err() != nil {
		return certificatemodel.CertificateStatus{}, certificatemodel.ErrInvalidCertificateQuery
	}
	runtime.mu.RLock()
	bindings := append([]certificateBinding(nil), runtime.certificates...)
	runtime.mu.RUnlock()
	for _, binding := range bindings {
		if binding.domain == strings.ToLower(domain) {
			return runtime.certificateStatus(binding), nil
		}
	}
	return certificatemodel.CertificateStatus{}, certificatemodel.ErrCertificateNotFound
}

func (*Runtime) certificateStatus(binding certificateBinding) certificatemodel.CertificateStatus {
	status := certificatemodel.CertificateStatus{Domain: binding.domain, Source: binding.source, Readiness: "unknown"}
	if binding.source == "acme" {
		status.Readiness = "pending"
	}
	certificates := caddytls.AllMatchingCertificates(binding.domain)
	for _, certificate := range certificates {
		leaf := certificate.Leaf
		if leaf == nil && len(certificate.Certificate.Certificate) != 0 {
			leaf, _ = x509.ParseCertificate(certificate.Certificate.Certificate[0])
		}
		if leaf == nil {
			continue
		}
		status.Readiness = "ready"
		notBefore := leaf.NotBefore.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		notAfter := leaf.NotAfter.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		serial := hex.EncodeToString(leaf.SerialNumber.Bytes())
		status.NotBefore, status.NotAfter, status.Serial = &notBefore, &notAfter, &serial
		break
	}
	return status
}

func (runtime *Runtime) compile(configuration []byte, secrets map[string][]byte) ([]byte, error) {
	if runtime == nil {
		return nil, errUnsupportedSettings
	}
	runtime.mu.RLock()
	targets := append([]DispatchTarget(nil), runtime.targets...)
	runtime.mu.RUnlock()
	return compileSettings(configuration, targets, runtime.id, secrets)
}

func (runtime *Runtime) Stop() error {
	err := caddycore.Stop()
	if runtime != nil {
		unregisterDispatchSecurity(runtime.id)
	}
	return err
}

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

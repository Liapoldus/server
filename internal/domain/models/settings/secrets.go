package settings

import (
	"encoding/json"
	"sort"
)

type SecretReferenceKind uint8

const (
	SecretReferenceCertificate SecretReferenceKind = iota + 1
	SecretReferencePrivateKey
	SecretReferenceUpstreamCA
	SecretReferenceHTTPAuth
)

func (settings Settings) ConfigSecretReferences() ([]string, error) {
	kinds, err := settings.ConfigSecretReferenceKinds()
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(kinds))
	for reference := range kinds {
		result = append(result, reference)
	}
	sort.Strings(result)
	return result, nil
}

func (settings Settings) ConfigSecretReferenceKinds() (map[string]SecretReferenceKind, error) {
	upstreamReferences, err := settings.UpstreamCAReferences()
	if err != nil {
		return nil, err
	}
	var config struct {
		Listeners []struct {
			TLS struct {
				Mode           string `json:"mode"`
				CertificateRef string `json:"certificateRef"`
				PrivateKeyRef  string `json:"privateKeyRef"`
			} `json:"tls"`
		} `json:"listeners"`
		Routes []struct {
			Handler struct {
				Type string `json:"type"`
			} `json:"handler"`
			Features struct {
				Auth struct {
					Mode      string `json:"mode"`
					SecretRef string `json:"secretRef"`
				} `json:"auth"`
			} `json:"features"`
		} `json:"routes"`
		Defaults struct {
			Service struct {
				Auth struct {
					Mode      string `json:"mode"`
					SecretRef string `json:"secretRef"`
				} `json:"auth"`
			} `json:"service"`
			Static struct {
				Auth struct {
					Mode      string `json:"mode"`
					SecretRef string `json:"secretRef"`
				} `json:"auth"`
			} `json:"static"`
			ReverseProxy struct {
				Auth struct {
					Mode      string `json:"mode"`
					SecretRef string `json:"secretRef"`
				} `json:"auth"`
			} `json:"reverseProxy"`
			Redirect struct {
				Auth struct {
					Mode      string `json:"mode"`
					SecretRef string `json:"secretRef"`
				} `json:"auth"`
			} `json:"redirect"`
		} `json:"defaults"`
	}
	if err := json.Unmarshal(settings.RuntimeConfig, &config); err != nil {
		return nil, ErrInvalidSettings
	}
	references := make(map[string]SecretReferenceKind, len(upstreamReferences)+len(config.Listeners)*2)
	addReference := func(reference string, kind SecretReferenceKind) error {
		if reference == "" {
			return ErrInvalidSettings
		}
		if previous, exists := references[reference]; exists && previous != kind {
			return ErrInvalidSettings
		}
		references[reference] = kind
		return nil
	}
	for _, reference := range upstreamReferences {
		if err := addReference(reference, SecretReferenceUpstreamCA); err != nil {
			return nil, err
		}
	}
	for _, listener := range config.Listeners {
		if listener.TLS.Mode != "custom" {
			continue
		}
		if listener.TLS.CertificateRef == "" || listener.TLS.PrivateKeyRef == "" {
			return nil, ErrInvalidSettings
		}
		if err := addReference(listener.TLS.CertificateRef, SecretReferenceCertificate); err != nil {
			return nil, err
		}
		if err := addReference(listener.TLS.PrivateKeyRef, SecretReferencePrivateKey); err != nil {
			return nil, err
		}
	}
	defaultsForType := func(routeType string) struct {
		Mode      string `json:"mode"`
		SecretRef string `json:"secretRef"`
	} { switch routeType {
	case "service":
		return config.Defaults.Service.Auth
	case "static":
		return config.Defaults.Static.Auth
	case "reverseProxy":
		return config.Defaults.ReverseProxy.Auth
	case "redirect":
		return config.Defaults.Redirect.Auth
	default:
		return struct {
			Mode      string `json:"mode"`
			SecretRef string `json:"secretRef"`
		}{}
	} }
	for _, route := range config.Routes {
		defaults := defaultsForType(route.Handler.Type)
		mode := route.Features.Auth.Mode
		if mode == "" || mode == "inherit" {
			mode = defaults.Mode
		}
		if mode == "" {
			mode = "disabled"
		}
		if mode != "enabled" && mode != "disabled" {
			return nil, ErrInvalidSettings
		}
		if mode != "enabled" {
			continue
		}
		reference := route.Features.Auth.SecretRef
		if reference == "" {
			reference = defaults.SecretRef
		}
		if err := addReference(reference, SecretReferenceHTTPAuth); err != nil {
			return nil, err
		}
	}
	return references, nil
}

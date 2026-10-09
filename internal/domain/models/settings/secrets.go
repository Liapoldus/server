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
	}
	if err := json.Unmarshal(settings.RuntimeConfig, &config); err != nil {
		return nil, ErrInvalidSettings
	}
	references := make(map[string]SecretReferenceKind, len(upstreamReferences)+len(config.Listeners)*2)
	for _, reference := range upstreamReferences {
		if previous, exists := references[reference]; exists && previous != SecretReferenceUpstreamCA {
			return nil, ErrInvalidSettings
		}
		references[reference] = SecretReferenceUpstreamCA
	}
	for _, listener := range config.Listeners {
		if listener.TLS.Mode != "custom" {
			continue
		}
		if listener.TLS.CertificateRef == "" || listener.TLS.PrivateKeyRef == "" {
			return nil, ErrInvalidSettings
		}
		if previous, exists := references[listener.TLS.CertificateRef]; exists && previous != SecretReferenceCertificate {
			return nil, ErrInvalidSettings
		}
		if previous, exists := references[listener.TLS.PrivateKeyRef]; exists && previous != SecretReferencePrivateKey {
			return nil, ErrInvalidSettings
		}
		references[listener.TLS.CertificateRef] = SecretReferenceCertificate
		references[listener.TLS.PrivateKeyRef] = SecretReferencePrivateKey
	}
	return references, nil
}

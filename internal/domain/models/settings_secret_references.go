package models

import (
	"encoding/json"
	"sort"
)

func (settings Settings) ConfigSecretReferences() ([]string, error) {
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
	references := make(map[string]struct{}, len(upstreamReferences)+len(config.Listeners)*2)
	for _, reference := range upstreamReferences {
		references[reference] = struct{}{}
	}
	for _, listener := range config.Listeners {
		if listener.TLS.Mode != "custom" {
			continue
		}
		if listener.TLS.CertificateRef == "" || listener.TLS.PrivateKeyRef == "" {
			return nil, ErrInvalidSettings
		}
		references[listener.TLS.CertificateRef] = struct{}{}
		references[listener.TLS.PrivateKeyRef] = struct{}{}
	}
	result := make([]string, 0, len(references))
	for reference := range references {
		result = append(result, reference)
	}
	sort.Strings(result)
	return result, nil
}

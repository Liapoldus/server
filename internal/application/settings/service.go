package settings

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
	"sort"
	"sync"

	"liapoldus.local/server-plugin/internal/domain/interfaces"
	certificatemodel "liapoldus.local/server-plugin/internal/domain/models/certificate"
	settingsmodel "liapoldus.local/server-plugin/internal/domain/models/settings"
)

var (
	ErrInvalidRevision   = errors.New("invalid Caddy settings revision")
	ErrRevisionConflict  = errors.New("Caddy settings revision conflict")
	ErrCandidateRejected = errors.New("Caddy candidate configuration rejected")
	ErrActivationFailed  = errors.New("Caddy configuration activation failed")
)

type Runtime interface {
	Validate([]byte) error
	Activate([]byte) error
	Stop() error
}

type SecretAwareRuntime interface {
	ValidateWithSecrets([]byte, map[string][]byte) error
	ActivateWithSecrets([]byte, map[string][]byte) error
}

type Configuration struct {
	runtime  Runtime
	revision string
	mu       sync.Mutex
	digest   [sha256.Size]byte
}

func NewConfiguration(runtime Runtime) (*Configuration, error) {
	if runtime == nil {
		return nil, settingsmodel.ErrInvalidSettings
	}
	return &Configuration{runtime: runtime}, nil
}

func (configuration *Configuration) Apply(settings settingsmodel.Settings, revision string) error {
	return configuration.ApplyWithSecrets(settings, revision, nil)
}

func (configuration *Configuration) ApplyWithSecrets(settings settingsmodel.Settings, revision string, secrets map[string][]byte) error {
	if configuration == nil || revision == "" {
		return ErrInvalidRevision
	}
	digest := digestSettings(settings.RuntimeConfig, secrets)
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	if revision == configuration.revision {
		if digest != configuration.digest {
			return ErrRevisionConflict
		}
		return nil
	}
	secretRuntime, supportsSecrets := configuration.runtime.(SecretAwareRuntime)
	if len(secrets) != 0 && !supportsSecrets {
		return ErrCandidateRejected
	}
	if supportsSecrets {
		if err := secretRuntime.ValidateWithSecrets(settings.RuntimeConfig, secrets); err != nil {
			return ErrCandidateRejected
		}
	} else if err := configuration.runtime.Validate(settings.RuntimeConfig); err != nil {
		return ErrCandidateRejected
	}
	if supportsSecrets {
		if err := secretRuntime.ActivateWithSecrets(settings.RuntimeConfig, secrets); err != nil {
			return ErrActivationFailed
		}
	} else if err := configuration.runtime.Activate(settings.RuntimeConfig); err != nil {
		return ErrActivationFailed
	}
	configuration.revision = revision
	configuration.digest = digest
	return nil
}

func digestSettings(configuration []byte, secrets map[string][]byte) [sha256.Size]byte {
	hasher := sha256.New()
	var length [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(length[:], uint64(len(configuration)))
	writeDigestPart(hasher, length[:n])
	writeDigestPart(hasher, configuration)
	keys := make([]string, 0, len(secrets))
	for reference := range secrets {
		keys = append(keys, reference)
	}
	sort.Strings(keys)
	n = binary.PutUvarint(length[:], uint64(len(keys)))
	writeDigestPart(hasher, length[:n])
	for _, reference := range keys {
		n = binary.PutUvarint(length[:], uint64(len(reference)))
		writeDigestPart(hasher, length[:n])
		writeDigestPart(hasher, []byte(reference))
		n = binary.PutUvarint(length[:], uint64(len(secrets[reference])))
		writeDigestPart(hasher, length[:n])
		writeDigestPart(hasher, secrets[reference])
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return digest
}

func writeDigestPart(hasher hash.Hash, value []byte) {
	_, _ = hasher.Write(value)
}

func (configuration *Configuration) Stop() error {
	if configuration == nil {
		return nil
	}
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	return configuration.runtime.Stop()
}

func (configuration *Configuration) Revision() string {
	if configuration == nil {
		return ""
	}
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	return configuration.revision
}

func (configuration *Configuration) ListCertificates(ctx context.Context, domain string, limit int, cursor string) (certificatemodel.CertificatePage, error) {
	if configuration == nil || ctx == nil || limit < 1 || limit > 100 {
		return certificatemodel.CertificatePage{}, certificatemodel.ErrInvalidCertificateQuery
	}
	inventory, ok := configuration.runtime.(interfaces.CertificateInventory)
	if !ok {
		return certificatemodel.CertificatePage{}, certificatemodel.ErrCertificateInventoryUnavailable
	}
	return inventory.ListCertificates(ctx, domain, limit, cursor)
}

func (configuration *Configuration) CertificateStatus(ctx context.Context, domain string) (certificatemodel.CertificateStatus, error) {
	if configuration == nil || ctx == nil || domain == "" {
		return certificatemodel.CertificateStatus{}, certificatemodel.ErrInvalidCertificateQuery
	}
	inventory, ok := configuration.runtime.(interfaces.CertificateInventory)
	if !ok {
		return certificatemodel.CertificateStatus{}, certificatemodel.ErrCertificateInventoryUnavailable
	}
	return inventory.CertificateStatus(ctx, domain)
}

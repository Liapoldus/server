package application

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
	"sort"
	"sync"

	"liapoldus.local/server-plugin/internal/domain/models"
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
	mu       sync.Mutex
	runtime  Runtime
	revision string
	digest   [sha256.Size]byte
}

func NewConfiguration(runtime Runtime) (*Configuration, error) {
	if runtime == nil {
		return nil, models.ErrInvalidSettings
	}
	return &Configuration{runtime: runtime}, nil
}

func (configuration *Configuration) Apply(settings models.Settings, revision string) error {
	return configuration.ApplyWithSecrets(settings, revision, nil)
}

func (configuration *Configuration) ApplyWithSecrets(settings models.Settings, revision string, secrets map[string][]byte) error {
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

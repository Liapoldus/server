package application

import (
	"crypto/sha256"
	"errors"
	"sync"

	"github.com/Liapoldus/caddy-plugin/internal/domain/models"
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
	if configuration == nil || revision == "" {
		return ErrInvalidRevision
	}
	digest := sha256.Sum256(settings.RuntimeConfig)
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	if revision == configuration.revision {
		if digest != configuration.digest {
			return ErrRevisionConflict
		}
		return nil
	}
	if err := configuration.runtime.Validate(settings.RuntimeConfig); err != nil {
		return ErrCandidateRejected
	}
	if err := configuration.runtime.Activate(settings.RuntimeConfig); err != nil {
		return ErrActivationFailed
	}
	configuration.revision = revision
	configuration.digest = digest
	return nil
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

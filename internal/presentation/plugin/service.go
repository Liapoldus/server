package plugin

import (
	"context"
	"errors"
	"sync"

	"liapoldus.local/server-plugin/contracts"
	"liapoldus.local/server-plugin/internal/application"
	"liapoldus.local/server-plugin/internal/domain/models"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	pluginsdk "github.com/Liapoldus/pluginprotocol/presentation/sdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Service struct {
	pluginv1.UnimplementedPluginServiceServer
	configuration *application.Configuration
	contract      contracts.Plugin
	stop          func()
	stopOnce      sync.Once
	bootstrapMu   sync.RWMutex
	instanceID    string
	grantEndpoint string
}

func New(configuration *application.Configuration, stop func()) (*Service, error) {
	contract, err := contracts.Load()
	if err != nil || configuration == nil || stop == nil {
		return nil, contracts.ErrInvalidAssets
	}
	return &Service{configuration: configuration, contract: contract, stop: stop}, nil
}

func (service *Service) Manifest(context.Context, *pluginv1.ManifestRequest) (*pluginv1.Manifest, error) {
	return &pluginv1.Manifest{Name: service.contract.Name, ProtocolVersion: pluginsdk.ProtocolVersion}, nil
}

func (service *Service) ConfigSchema(context.Context, *pluginv1.ConfigSchemaRequest) (*pluginv1.ConfigSchema, error) {
	fields := make([]*pluginv1.ConfigField, 0, len(service.contract.ConfigSchema.Fields))
	for _, field := range service.contract.ConfigSchema.Fields {
		fields = append(fields, &pluginv1.ConfigField{Name: field.Name, Type: field.Type, Required: field.Required, Description: field.Description})
	}
	return &pluginv1.ConfigSchema{Fields: fields}, nil
}

func (service *Service) Bootstrap(_ context.Context, request *pluginv1.BootstrapRequest) (*pluginv1.BootstrapResult, error) {
	if request == nil || request.GetInstanceId() == "" {
		return nil, status.Error(codes.InvalidArgument, "")
	}
	service.bootstrapMu.Lock()
	defer service.bootstrapMu.Unlock()
	if service.instanceID != "" && service.instanceID != request.GetInstanceId() {
		return nil, status.Error(codes.PermissionDenied, "")
	}
	service.instanceID = request.GetInstanceId()
	service.grantEndpoint = request.GetGrantBrokerEndpoint()
	return &pluginv1.BootstrapResult{Accepted: true}, nil
}

func (service *Service) ConfigApply(ctx context.Context, request *pluginv1.ConfigApplyRequest) (*pluginv1.ConfigApplyResult, error) {
	if request == nil || request.GetSettingsRevision() == "" {
		return nil, status.Error(codes.InvalidArgument, "")
	}
	if err := contracts.ValidateSettings(request.GetConfig()); err != nil {
		return nil, status.Error(codes.InvalidArgument, "")
	}
	settings, err := models.DecodeSettings(request.GetConfig(), service.contract.Configuration.VersionField, service.contract.Configuration.RuntimeConfigField, service.contract.Configuration.SchemaVersion)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "")
	}
	references, err := settings.ConfigSecretReferences()
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "")
	}
	secrets, err := service.redeemConfigSecrets(ctx, request, references)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, "")
	}
	defer clearSecrets(secrets)
	if err := service.configuration.ApplyWithSecrets(settings, request.GetSettingsRevision(), secrets); err != nil {
		if errors.Is(err, application.ErrInvalidRevision) || errors.Is(err, application.ErrRevisionConflict) {
			return nil, status.Error(codes.FailedPrecondition, "")
		}
		if errors.Is(err, application.ErrCandidateRejected) || errors.Is(err, models.ErrInvalidSettings) {
			return nil, status.Error(codes.InvalidArgument, "")
		}
		return nil, status.Error(codes.Unavailable, "")
	}
	return &pluginv1.ConfigApplyResult{Applied: true, SettingsRevision: request.GetSettingsRevision()}, nil
}

func (service *Service) redeemConfigSecrets(ctx context.Context, request *pluginv1.ConfigApplyRequest, references []string) (map[string][]byte, error) {
	if len(references) == 0 {
		if len(request.GetGrants()) != 0 {
			return nil, application.ErrCandidateRejected
		}
		return nil, nil
	}
	service.bootstrapMu.RLock()
	instanceID, endpoint := service.instanceID, service.grantEndpoint
	service.bootstrapMu.RUnlock()
	if instanceID == "" || endpoint == "" {
		return nil, application.ErrCandidateRejected
	}
	expected := make(map[string]struct{}, len(references))
	for _, reference := range references {
		expected[reference] = struct{}{}
	}
	grants := make(map[string]*pluginv1.ActiveGrant, len(request.GetGrants()))
	for _, grant := range request.GetGrants() {
		if grant == nil || grant.GetHandle() == "" || grant.GetPurpose() == "" || grant.GetScope() != pluginv1.GrantScope_GRANT_SCOPE_CONFIG_APPLY ||
			grant.GetInstanceId() != instanceID || grant.GetSettingsRevision() != request.GetSettingsRevision() || grant.GetCapability() != "" || len(grant.GetDomains()) != 0 {
			return nil, application.ErrCandidateRejected
		}
		reference := grant.GetSecretReference()
		if _, exists := expected[reference]; !exists {
			return nil, application.ErrCandidateRejected
		}
		if _, duplicate := grants[reference]; duplicate {
			return nil, application.ErrCandidateRejected
		}
		grants[reference] = grant
	}
	if len(grants) != len(expected) {
		return nil, application.ErrCandidateRejected
	}
	client, err := pluginsdk.DialGrantBrokerContext(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	secrets := make(map[string][]byte, len(references))
	for _, reference := range references {
		secret, redeemErr := client.RedeemConfig(ctx, grants[reference])
		if redeemErr != nil {
			clearSecrets(secrets)
			return nil, redeemErr
		}
		secrets[reference] = secret
	}
	return secrets, nil
}

func clearSecrets(secrets map[string][]byte) {
	for _, secret := range secrets {
		clear(secret)
	}
}

func (service *Service) Shutdown(context.Context, *pluginv1.ShutdownRequest) (*pluginv1.ShutdownResult, error) {
	service.stopOnce.Do(func() { go service.stop() })
	return &pluginv1.ShutdownResult{Closed: true}, nil
}

func SettingsSchema() ([]byte, error) { return contracts.SettingsSchema() }

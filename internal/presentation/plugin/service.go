package plugin

import (
	"context"
	"errors"
	"sync"

	"github.com/Liapoldus/caddy-plugin/contracts"
	"github.com/Liapoldus/caddy-plugin/internal/application"
	"github.com/Liapoldus/caddy-plugin/internal/domain/models"
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

func (service *Service) ConfigApply(_ context.Context, request *pluginv1.ConfigApplyRequest) (*pluginv1.ConfigApplyResult, error) {
	if request == nil || request.GetSettingsRevision() == "" {
		return nil, status.Error(codes.InvalidArgument, "")
	}
	settings, err := models.DecodeSettings(request.GetConfig(), service.contract.Configuration.VersionField, service.contract.Configuration.RuntimeConfigField, service.contract.Configuration.SchemaVersion)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "")
	}
	if err := service.configuration.Apply(settings, request.GetSettingsRevision()); err != nil {
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

func (service *Service) Shutdown(context.Context, *pluginv1.ShutdownRequest) (*pluginv1.ShutdownResult, error) {
	service.stopOnce.Do(func() { go service.stop() })
	return &pluginv1.ShutdownResult{Closed: true}, nil
}

func SettingsSchema() ([]byte, error) { return contracts.SettingsSchema() }

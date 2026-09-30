package restplugin

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"

	"liapoldus.local/server-plugin/contracts"
	"liapoldus.local/server-plugin/internal/application"
	"liapoldus.local/server-plugin/internal/domain/models"
	sdkapp "liapoldus.local/plugin-sdk/application"
	sdkinterfaces "liapoldus.local/plugin-sdk/domain/interfaces"
	sdkmodels "liapoldus.local/plugin-sdk/domain/models"
	sdkinfra "liapoldus.local/plugin-sdk/infrastructure"
	sdkpresentation "liapoldus.local/plugin-sdk/presentation"
)

var ErrSecretReferencesRequireCoreGrantAPI = errors.New("Caddy configuration contains secret references but Core grant delivery is not available to the REST lifecycle")

type Adapter struct {
	configuration *application.Configuration
	contract      contracts.Plugin
}

func New(configuration *application.Configuration) (*Adapter, error) {
	contract, err := contracts.Load()
	if err != nil || configuration == nil {
		return nil, contracts.ErrInvalidAssets
	}
	return &Adapter{configuration: configuration, contract: contract}, nil
}

func (adapter *Adapter) Manifest(context.Context) ([]byte, error) {
	if adapter == nil {
		return nil, contracts.ErrInvalidAssets
	}
	return contracts.PluginManifest()
}

func (adapter *Adapter) ConfigurationSchema(context.Context) ([]byte, error) {
	if adapter == nil {
		return nil, contracts.ErrInvalidAssets
	}
	return contracts.SettingsSchema()
}

func (adapter *Adapter) Apply(ctx context.Context, incoming sdkmodels.Configuration) error {
	if adapter == nil || adapter.configuration == nil || ctx.Err() != nil || incoming.Validate() != nil {
		return models.ErrInvalidSettings
	}
	contents := incoming.Bytes()
	if err := contracts.ValidateSettings(contents); err != nil {
		return models.ErrInvalidSettings
	}
	settings, err := models.DecodeSettings(
		contents,
		adapter.contract.Configuration.VersionField,
		adapter.contract.Configuration.RuntimeConfigField,
		adapter.contract.Configuration.SchemaVersion,
	)
	if err != nil {
		return models.ErrInvalidSettings
	}
	references, err := settings.ConfigSecretReferences()
	if err != nil {
		return models.ErrInvalidSettings
	}
	if len(references) != 0 {
		return ErrSecretReferencesRequireCoreGrantAPI
	}
	return adapter.configuration.Apply(settings, incoming.Generation)
}


func serverConfiguration(configuration *application.Configuration, source sdkinterfaces.ConfigurationSource) (sdkpresentation.ServerConfiguration, error) {
	adapter, err := New(configuration)
	if err != nil || source == nil {
		return sdkpresentation.ServerConfiguration{}, contracts.ErrInvalidAssets
	}
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		return sdkpresentation.ServerConfiguration{}, err
	}
	lifecycle, err := sdkapp.NewLifecycle(source, adapter)
	if err != nil {
		return sdkpresentation.ServerConfiguration{}, err
	}
	metadata := sdkinterfaces.PluginMetadata(adapter)
	return sdkpresentation.ServerConfiguration{
		Manifest:           route(contract.Plugin.Endpoints.Manifest),
		ConfigSchema:       route(contract.Plugin.Endpoints.ConfigSchema),
		Health:             route(contract.Plugin.Endpoints.Health),
		Ready:              route(contract.Plugin.Endpoints.Ready),
		Reload:             route(contract.Plugin.Endpoints.Reload),
		Metrics:            route(contract.Plugin.Endpoints.Metrics),
		JSONMediaType:      contract.Plugin.Responses.ContentTypes.JSON,
		MetricsMediaType:   contract.Plugin.Responses.ContentTypes.Metrics,
		MaximumReloadBytes: contract.Plugin.ReloadRequest.MaximumBytes,
		ReadyMetricName:    contract.Plugin.Responses.Metrics.ReadyMetricName,
		ReadyMetricHelp:    contract.Plugin.Responses.Metrics.ReadyMetricHelp,
		HealthBody:         contract.Plugin.Responses.Health.Body,
		Errors: sdkpresentation.ErrorCatalog{
			InvalidRequest:           problem(contract, "invalidRequest"),
			NotReady:                 problem(contract, "notReady"),
			ConfigurationUnavailable: problem(contract, "configurationUnavailable"),
		},
		Lifecycle:       lifecycle,
		Metadata:        metadata,
	}, nil
}

// NewMutualTLSServer keeps workload identity and trust roots at the process
// composition boundary. This adapter never loads certificates or invents a
// bootstrap mechanism.
func NewMutualTLSServer(
	configuration *application.Configuration,
	source sdkinterfaces.ConfigurationSource,
	listener net.Listener,
	serverCertificate tls.Certificate,
	trustedCoreRoots *x509.CertPool,
	minimumTLSVersion uint16,
) (*http.Server, net.Listener, error) {
	if listener == nil {
		return nil, nil, sdkinfra.ErrInvalidMTLSConfiguration
	}
	serverOptions, err := serverConfiguration(configuration, source)
	if err != nil {
		return nil, nil, err
	}
	handler, err := sdkpresentation.NewHandler(serverOptions)
	if err != nil {
		return nil, nil, err
	}
	secureListener, err := sdkinfra.NewMutualTLSListener(listener, serverCertificate, trustedCoreRoots, minimumTLSVersion)
	if err != nil {
		return nil, nil, err
	}
	return &http.Server{Handler: handler}, secureListener, nil
}

func route(endpoint sdkinfra.Endpoint) sdkpresentation.Route {
	return sdkpresentation.Route{Method: endpoint.Method, Path: endpoint.Path}
}

func problem(contract sdkinfra.HTTPContract, name string) sdkpresentation.ProblemMapping {
	entry := contract.Errors[name]
	return sdkpresentation.ProblemMapping{Status: entry.Status, Code: entry.Code}
}

var _ sdkinterfaces.PluginMetadata = (*Adapter)(nil)
var _ sdkinterfaces.ConfigurationApplier = (*Adapter)(nil)

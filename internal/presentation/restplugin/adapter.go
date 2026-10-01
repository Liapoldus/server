package restplugin

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	sdkapp "github.com/Liapoldus/plugin-sdk/application"
	sdkinterfaces "github.com/Liapoldus/plugin-sdk/domain/interfaces"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkinfra "github.com/Liapoldus/plugin-sdk/infrastructure"
	sdkpresentation "github.com/Liapoldus/plugin-sdk/presentation"
	"liapoldus.local/server-plugin/contracts"
	"liapoldus.local/server-plugin/internal/application"
	"liapoldus.local/server-plugin/internal/domain/models"
)

var ErrSecretReferencesRequireCoreGrantAPI = errors.New("Server configuration contains secret references but Core grant delivery is unavailable")

// Adapter is the Server-owned metadata publisher and configuration applier.
// The Plugin SDK sees settings as opaque bytes until this adapter validates and
// applies them through the Server's own contracts and runtime.
type Adapter struct {
	configuration *application.Configuration
	contract      contracts.Plugin
}

// LifecycleOptions contains operational inputs supplied by the process
// composition root. Product configuration is deliberately absent: it arrives
// only through the SDK Reload/pull lifecycle.
type LifecycleOptions struct {
	Source      sdkinterfaces.ConfigurationSource
	Identity    sdkmodels.ReplicaIdentity
	Credentials sdkinfra.CredentialsProvider
	CorePeer    sdkmodels.PeerIdentity
	Revocation  sdkinterfaces.RevocationSource
	ErrorLog    *log.Logger
	LogOutput   io.Writer
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

// NewHandler creates the Plugin SDK REST lifecycle handler and its use case.
// It does not bind a socket or infer transport identity; callers must inject
// those separately through the SDK's transport constructors.
func NewHandler(
	configuration *application.Configuration,
	options LifecycleOptions,
) (http.Handler, *sdkapp.Lifecycle, *sdkinfra.ObserverPrometheusCollector, error) {
	adapter, err := New(configuration)
	if err != nil || options.Source == nil || !options.Identity.Valid() || options.LogOutput == nil {
		return nil, nil, nil, contracts.ErrInvalidAssets
	}
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		return nil, nil, nil, err
	}

	collector, err := sdkinfra.NewObserverPrometheusCollector(contract)
	if err != nil {
		return nil, nil, nil, err
	}
	logger, err := sdkinfra.NewJSONLogger(contract, options.LogOutput)
	if err != nil {
		return nil, nil, nil, err
	}
	logging, err := sdkapp.NewLoggingObserver(sdkapp.LoggingObserverConfiguration{
		Logger:              logger,
		RedactedKeys:        contract.Logging.RedactedKeys,
		AlwaysRedactedKeys:  contract.Logging.AlwaysRedactedKeys,
		RedactedPlaceholder: contract.Logging.RedactedPlaceholder,
		MaximumFields:       contract.Logging.MaximumFields,
		MaximumKeyLength:    contract.Logging.MaximumKeyLength,
		MaximumValueLength:  contract.Logging.MaximumValueLength,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	maximumKindLength := max(len(sdkapp.KindReload), len(sdkapp.KindSecretGrant), len(sdkapp.KindSecretRedemption))
	recorder, err := sdkapp.NewRecorder(sdkapp.RecorderConfiguration{
		Sink: collectorMetricsSink{collector: collector},
		AllowedKinds: []sdkapp.Kind{
			sdkapp.KindReload,
			sdkapp.KindSecretGrant,
			sdkapp.KindSecretRedemption,
		},
		MaximumKindLength: maximumKindLength,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	observer := sdkapp.Observers{recorder, logging}
	lifecycle, err := sdkapp.NewLifecycle(sdkapp.LifecycleConfiguration{
		Source:   options.Source,
		Applier:  adapter,
		Identity: options.Identity,
		Observer: observer,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	metadata := sdkinterfaces.PluginMetadata(adapter)
	handler, err := sdkpresentation.NewHandlerSet(sdkpresentation.HandlerConfiguration{
		Contracts:    presentationContracts(contract),
		Lifecycle:    lifecycle,
		Readiness:    sdkpresentation.WithoutContext(lifecycle.Readiness),
		Registration: lifecycle,
		Metadata:     metadata,
		Metrics:      collector,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return handler.Handler(), lifecycle, collector, nil
}

type collectorMetricsSink struct {
	collector *sdkinfra.ObserverPrometheusCollector
}

func (sink collectorMetricsSink) Lifecycle(kind string, outcome sdkmodels.Outcome) {
	sink.collector.Observe(context.Background(), kind, outcome)
}

func (sink collectorMetricsSink) PullFailure(sdkmodels.Outcome) {
	sink.collector.RecordConfigPullFailure()
}

func (sink collectorMetricsSink) SetReady(ready bool) {
	sink.collector.SetReady(ready)
}

// NewMutualTLSServer builds the SDK-owned Core↔plugin REST server. The Core
// identity, credential provider and revocation source are required inputs and
// are never discovered from request data or product settings.
func NewMutualTLSServer(
	configuration *application.Configuration,
	options LifecycleOptions,
) (*sdkinfra.MutualTLSServer, *sdkapp.Lifecycle, error) {
	handler, lifecycle, _, err := NewHandler(configuration, options)
	if err != nil {
		return nil, nil, err
	}
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		return nil, nil, err
	}
	if options.Credentials == nil || !options.CorePeer.Valid() || options.Revocation == nil {
		return nil, nil, sdkinfra.ErrInvalidServerTLS
	}
	server, err := sdkinfra.NewMutualTLSServer(contract, sdkinfra.MutualTLSServerConfig{
		Handler:    handler,
		Provider:   options.Credentials,
		Peer:       options.CorePeer,
		Revocation: options.Revocation,
		ErrorLog:   options.ErrorLog,
	})
	if err != nil {
		return nil, nil, err
	}
	return server, lifecycle, nil
}

func presentationContracts(contract sdkinfra.HTTPContract) sdkpresentation.Contracts {
	endpoint := func(name string) sdkpresentation.Endpoint {
		value, _ := contract.Endpoint(name)
		return sdkpresentation.Endpoint{Method: value.Method, Path: value.Path}
	}
	document := func(value sdkinfra.DocumentContract) sdkpresentation.DocumentContract {
		return sdkpresentation.DocumentContract{MediaType: value.MediaType, MaximumBytes: value.MaximumBytes, Required: append([]string(nil), value.Required...)}
	}
	problems := make(map[string]sdkpresentation.Problem, len(contract.Problems))
	for name, value := range contract.Problems {
		problems[name] = sdkpresentation.Problem{Status: value.Status, Code: value.Code}
	}
	errors := make(map[string]sdkpresentation.Problem, len(contract.Errors))
	for name, value := range contract.Errors {
		errors[name] = sdkpresentation.Problem{Status: value.Status, Code: value.Code}
	}
	return sdkpresentation.Contracts{
		ContractVersion:  contract.ContractVersion,
		IdentityEndpoint: endpoint("identity"), ManifestEndpoint: endpoint("manifest"),
		ConfigSchemaEndpoint: endpoint("configSchema"), HealthEndpoint: endpoint("health"),
		ReadyEndpoint: endpoint("ready"), ReloadEndpoint: endpoint("reload"),
		MetricsEndpoint: endpoint("metrics"),
		HealthStatus:    contract.Plugin.Responses.Health.Status,
		HealthBody:      cloneStringMap(contract.Plugin.Responses.Health.Body),
		ContentTypes: sdkpresentation.ContentTypes{
			JSON: contract.Plugin.Responses.ContentTypes.JSON, Metrics: contract.Plugin.Responses.ContentTypes.Metrics,
		},
		ReloadRequest:         document(contract.Plugin.ReloadRequest),
		ReloadAcknowledgement: document(contract.Plugin.ReloadAcknowledgement),
		Readiness:             document(contract.Plugin.Readiness), Manifest: document(contract.Plugin.Manifest),
		ConfigurationSchema: document(contract.Plugin.ConfigurationSchema),
		Registration: document(sdkinfra.DocumentContract{
			MediaType:    contract.Identity.Registration.MediaType,
			MaximumBytes: contract.Identity.Registration.MaximumBytes,
			Required:     contract.Identity.Registration.Required,
		}),
		MaximumMetadataBytes: contract.Plugin.MaximumMetadataBytes,
		ReadinessDeadline:    time.Duration(contract.Deadlines.PluginReadinessSeconds) * time.Second,
		Problems:             problems, Errors: errors,
		OutcomeProblems: cloneStringMap(contract.OutcomeProblems),
		SuccessOutcomes: append([]string(nil), contract.SuccessOutcomes...),
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

var _ sdkinterfaces.PluginMetadata = (*Adapter)(nil)
var _ sdkinterfaces.ConfigurationApplier = (*Adapter)(nil)

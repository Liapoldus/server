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
	settingsapp "liapoldus.local/server-plugin/internal/application/settings"
	siteapp "liapoldus.local/server-plugin/internal/application/site"
	settingsmodel "liapoldus.local/server-plugin/internal/domain/models/settings"
)

var ErrSecretReferencesRequireCoreGrantAPI = errors.New("Server configuration contains secret references but Core grant delivery is unavailable")

// Adapter is the Server-owned metadata publisher and configuration applier.
// The Plugin SDK sees settings as opaque bytes until this adapter validates and
// applies them through the Server's own contracts and runtime.
type Adapter struct {
	configuration *settingsapp.Configuration
	publisher     *siteapp.SitePublisher
	secrets       SecretProvider
}

type SecretProvider interface {
	SecretProvider(context.Context, string, string) (sdkmodels.SecretValue, error)
}

// LifecycleOptions contains operational inputs supplied by the process
// composition root. Product configuration is deliberately absent: it arrives
// only through the SDK Reload/pull lifecycle.
type LifecycleOptions struct {
	Source      sdkinterfaces.ConfigurationSource
	Broker      sdkinterfaces.SecretBroker
	Credentials sdkinfra.CredentialsProvider
	Revocation  sdkinterfaces.RevocationSource
	LogOutput   io.Writer
	ErrorLog    *log.Logger
	Identity    sdkmodels.ReplicaIdentity
	CorePeer    sdkmodels.PeerIdentity
}

func New(configuration *settingsapp.Configuration, publisher *siteapp.SitePublisher) (*Adapter, error) {
	if configuration == nil || publisher == nil {
		return nil, contracts.ErrInvalidAssets
	}
	return &Adapter{configuration: configuration, publisher: publisher}, nil
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
		return settingsmodel.ErrInvalidSettings
	}
	contents := incoming.Bytes()
	if err := contracts.ValidateSettings(contents); err != nil {
		return settingsmodel.ErrInvalidSettings
	}
	settings, err := settingsmodel.DecodeSettings(contents)
	if err != nil {
		return settingsmodel.ErrInvalidSettings
	}
	references, err := settings.ConfigSecretReferences()
	if err != nil {
		return settingsmodel.ErrInvalidSettings
	}
	if len(references) != 0 {
		return adapter.applyWithSecrets(ctx, settings, incoming.Generation)
	}
	return adapter.configuration.Apply(settings, incoming.Generation)
}

func (adapter *Adapter) SetSecretProvider(provider SecretProvider) {
	if adapter != nil {
		adapter.secrets = provider
	}
}

func (adapter *Adapter) applyWithSecrets(ctx context.Context, settings settingsmodel.Settings, generation string) error {
	if adapter.secrets == nil {
		return ErrSecretReferencesRequireCoreGrantAPI
	}
	kinds, err := settings.ConfigSecretReferenceKinds()
	if err != nil {
		return settingsmodel.ErrInvalidSettings
	}
	values := make(map[string][]byte, len(kinds))
	defer func() {
		for _, value := range values {
			clear(value)
		}
	}()
	for reference, kind := range kinds {
		var purpose SettingsSecretPurpose
		switch kind {
		case settingsmodel.SecretReferenceCertificate:
			purpose = SettingsCertificatePurpose
		case settingsmodel.SecretReferencePrivateKey:
			purpose = SettingsPrivateKeyPurpose
		case settingsmodel.SecretReferenceUpstreamCA:
			purpose = SettingsUpstreamCAPurpose
		default:
			return settingsmodel.ErrInvalidSettings
		}
		value, err := adapter.secrets.SecretProvider(ctx, reference, string(purpose))
		if err != nil {
			return settingsmodel.ErrInvalidSettings
		}
		values[reference] = value.Bytes()
		value.Destroy()
	}
	return adapter.configuration.ApplyWithSecrets(settings, generation, values)
}

// NewHandler creates the Plugin SDK REST lifecycle handler and its use case.
// It does not bind a socket or infer transport identity; callers must inject
// those separately through the SDK's transport constructors.
func NewHandler(
	configuration *settingsapp.Configuration,
	publisher *siteapp.SitePublisher,
	options LifecycleOptions,
) (http.Handler, *sdkapp.Lifecycle, *sdkinfra.ObserverPrometheusCollector, error) {
	adapter, err := New(configuration, publisher)
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
	if options.Broker != nil {
		secrets, err := sdkapp.NewSecretManager(sdkapp.SecretManagerConfiguration{
			Broker: options.Broker, Clock: wallClock{}, Lifecycle: lifecycle,
			Observer: observer, MaximumTrackedGrants: 1024,
		})
		if err != nil {
			return nil, nil, nil, err
		}
		adapter.SetSecretProvider(secrets)
	}
	metadata := sdkinterfaces.PluginMetadata(adapter)
	handler, err := sdkpresentation.NewHandlerSet(sdkpresentation.HandlerConfiguration{
		Contracts:    presentationContracts(contract),
		Lifecycle:    lifecycle,
		Readiness:    sdkpresentation.WithoutContext(lifecycle.Readiness),
		Registration: lifecycle,
		Metadata:     metadata,
		Metrics:      collector,
		Artifacts:    adapter,
		AdminSurface: adapter,
		AdminActions: adapter,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return handler.Handler(), lifecycle, collector, nil
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

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
	configuration *settingsapp.Configuration,
	publisher *siteapp.SitePublisher,
	options LifecycleOptions,
) (*sdkinfra.MutualTLSServer, *sdkapp.Lifecycle, error) {
	handler, lifecycle, _, err := NewHandler(configuration, publisher, options)
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
		return sdkpresentation.DocumentContract{
			MediaType: value.MediaType, MaximumBytes: value.MaximumBytes,
			Required: append([]string(nil), value.Required...), DigestAlgorithm: value.DigestAlgorithm,
		}
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
		ConfigurationSchema:    document(contract.Plugin.ConfigurationSchema),
		ArtifactStreamEndpoint: endpoint("artifactStream"), AdminSurfaceEndpoint: endpoint("adminSurface"),
		AdminActionEndpoint: endpoint("adminAction"),
		ArtifactStream: sdkpresentation.ArtifactStreamContract{
			MediaType:                     contract.Plugin.ArtifactStream.MediaType,
			MetadataMediaType:             contract.Plugin.ArtifactStream.MetadataMediaType,
			Parts:                         append([]string(nil), contract.Plugin.ArtifactStream.Parts...),
			PartOrder:                     append([]string(nil), contract.Plugin.ArtifactStream.PartOrder...),
			MaximumArtifactBytes:          contract.Plugin.ArtifactStream.MaximumArtifactBytes,
			MinimumArtifactBytes:          contract.Plugin.ArtifactStream.MinimumArtifactBytes,
			MaximumMetadataBytes:          contract.Plugin.ArtifactStream.MaximumMetadataBytes,
			MaximumMultipartOverheadBytes: contract.Plugin.ArtifactStream.MaximumMultipartOverheadBytes,
			MaximumRequestBytes:           contract.Plugin.ArtifactStream.MaximumRequestBytes,
			MaximumReceiptBytes:           contract.Plugin.ArtifactStream.MaximumReceiptBytes,
			AcceptedStatus:                contract.Plugin.ArtifactStream.AcceptedStatus,
			Deadline:                      time.Duration(contract.Deadlines.ArtifactStreamSeconds) * time.Second,
			FilenameForwarded:             contract.Plugin.ArtifactStream.FilenameForwarded,
			InvocationContext: sdkpresentation.ArtifactInvocationContract{
				MaximumBytes: contract.Plugin.ArtifactStream.InvocationContext.MaximumBytes,
				Required:     append([]string(nil), contract.Plugin.ArtifactStream.InvocationContext.Required...),
				Optional:     append([]string(nil), contract.Plugin.ArtifactStream.InvocationContext.Optional...),
				Headers:      cloneStringMap(contract.Plugin.ArtifactStream.InvocationContext.Headers),
			},
		},
		AdminSurface: document(contract.Plugin.AdminSurface),
		AdminAction: sdkpresentation.AdminActionContract{
			MediaType:            contract.Plugin.AdminAction.MediaType,
			MaximumRequestBytes:  contract.Plugin.AdminAction.MaximumRequestBytes,
			MaximumResponseBytes: contract.Plugin.AdminAction.MaximumResponseBytes,
			MaximumPageIDBytes:   contract.Plugin.AdminAction.MaximumPageIDBytes,
			MaximumActionIDBytes: contract.Plugin.AdminAction.MaximumActionIDBytes,
			PathSegmentPattern:   contract.Plugin.AdminAction.PathSegmentPattern,
			ResponseStatus: sdkpresentation.StatusRangeContract{
				Minimum: contract.Plugin.AdminAction.ResponseStatus.Minimum,
				Maximum: contract.Plugin.AdminAction.ResponseStatus.Maximum,
			},
			Deadline: time.Duration(contract.Plugin.AdminAction.DeadlineSeconds) * time.Second,
			InvocationContext: sdkpresentation.AdminInvocationContract{
				MaximumBytes:        contract.Plugin.AdminAction.InvocationContext.MaximumBytes,
				UnknownHeaderPrefix: contract.Plugin.AdminAction.InvocationContext.UnknownHeaderPrefix,
				Required:            append([]string(nil), contract.Plugin.AdminAction.InvocationContext.Required...),
				Optional:            append([]string(nil), contract.Plugin.AdminAction.InvocationContext.Optional...),
				Headers:             cloneStringMap(contract.Plugin.AdminAction.InvocationContext.Headers),
			},
		},
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

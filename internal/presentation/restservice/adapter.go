package restservice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	sdkapp "github.com/Liapoldus/service-sdk/v2/application"
	sdkinterfaces "github.com/Liapoldus/service-sdk/v2/domain/interfaces"
	sdkmodels "github.com/Liapoldus/service-sdk/v2/domain/models"
	sdkinfra "github.com/Liapoldus/service-sdk/v2/infrastructure"
	sdkpresentation "github.com/Liapoldus/service-sdk/v2/presentation"
	"liapoldus.local/server-service/contracts"
	settingsapp "liapoldus.local/server-service/internal/application/settings"
	siteapp "liapoldus.local/server-service/internal/application/site"
	settingsmodel "liapoldus.local/server-service/internal/domain/models/settings"
)

const ServiceVersion = "1.1.9"

var ErrSecretReferencesRequireCoreGrantAPI = errors.New("Server configuration contains secret references but Core grant delivery is unavailable")

// Adapter is the Server-owned metadata publisher and configuration applier.
// The Service SDK sees settings as opaque bytes until this adapter validates and
// applies them through the Server's own contracts and runtime.
type Adapter struct {
	configuration *settingsapp.Configuration
	publisher     *siteapp.SitePublisher
	secrets       SecretProvider
	manifest      *sdkpresentation.ManifestProvider
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
	schema, err := contracts.SettingsSchema()
	if err != nil {
		return nil, err
	}
	manifest, err := sdkpresentation.NewManifestProvider(sdkmodels.ManifestDescriptor{
		ID: "server", Version: ServiceVersion, SecretPurposes: []string{
			string(SettingsHTTPAuthPurpose), string(SettingsCertificatePurpose), string(SettingsPrivateKeyPurpose), string(SettingsUpstreamCAPurpose),
		}, ConfigurationUI: serverConfigurationUI(), Resources: serverResources(),
		EditorButtons: &sdkmodels.ServiceEditorButtonsDescriptor{Version: sdkmodels.ServiceEditorButtonsVersion, Buttons: []sdkmodels.ServiceEditorButton{
			sdkmodels.NewServiceEditorButton("server", "endpoints", sdkmodels.ConfigurationUIText{RU: "Редактор эндпоинтов", EN: "Endpoint editor"}, sdkmodels.ConfigurationUIText{RU: "Открыть редактор эндпоинтов Server", EN: "Open the Server endpoint editor"}),
		}},
	}, schema)
	if err != nil {
		return nil, err
	}
	return &Adapter{configuration: configuration, publisher: publisher, manifest: manifest}, nil
}

func serverResources() []sdkmodels.ResourceDescriptor {
	return []sdkmodels.ResourceDescriptor{{
		ID: "sites", Kind: "site", Provider: sdkmodels.ResourceProviderServiceAdmin,
		PageID: "sites", QueryResource: "sites", ValuePointer: "/siteId", LabelPointer: "/siteId",
		Publication: &sdkmodels.ResourcePublicationDescriptor{PageID: "sites", ActionID: "publish"},
	}, {
		ID: "site-releases", Kind: "site-release", Provider: sdkmodels.ResourceProviderServiceAdmin,
		PageID: "sites", QueryResource: "releases", ValuePointer: "/revisionId", LabelPointer: "/revisionId",
	}}
}

func serverConfigurationUI() *sdkmodels.ConfigurationUIDescriptor {
	text := func(ru, en string) sdkmodels.ConfigurationUIText {
		return sdkmodels.ConfigurationUIText{RU: ru, EN: en}
	}
	field := func(path, ru, en, widget string, order int) sdkmodels.ConfigurationUIField {
		return sdkmodels.ConfigurationUIField{Path: path, Label: text(ru, en), Widget: widget, Order: order}
	}
	siteResourceField := field("/config/routes/*/handler/siteId", "Сайт", "Site", "resource", 100)
	siteResourceField.ResourceKind = "site"
	siteResourceField.ResourceID = "sites"
	releaseResourceField := field("/config/routes/*/handler/releaseId", "Неизменяемый релиз", "Immutable release", "resource", 130)
	releaseResourceField.ResourceKind = "site-release"
	releaseResourceField.ResourceID = "site-releases"
	authSecretField := field("/config/routes/*/features/auth/secretRef", "Секрет HTTP-аутентификации", "HTTP authentication secret", "secret", 140)
	authSecretField.SecretPurpose = string(SettingsHTTPAuthPurpose)
	authDefaultSecretFields := make([]sdkmodels.ConfigurationUIField, 0, 4)
	for _, routeType := range []string{"service", "static", "reverseProxy", "redirect"} {
		secretField := field("/config/defaults/"+routeType+"/auth/secretRef", "Секрет HTTP-аутентификации", "HTTP authentication secret", "secret", 140)
		secretField.SecretPurpose = string(SettingsHTTPAuthPurpose)
		authDefaultSecretFields = append(authDefaultSecretFields, secretField)
	}
	certificateSecretField := field("/config/listeners/*/tls/certificateRef", "Сертификат TLS", "TLS certificate", "secret", 70)
	certificateSecretField.SecretPurpose = string(SettingsCertificatePurpose)
	privateKeySecretField := field("/config/listeners/*/tls/privateKeyRef", "Закрытый ключ TLS", "TLS private key", "secret", 80)
	privateKeySecretField.SecretPurpose = string(SettingsPrivateKeyPurpose)
	upstreamCASecretField := field("/config/routes/*/handler/upstreams/*/caRef", "Доверенный CA upstream", "Trusted upstream CA", "secret", 150)
	upstreamCASecretField.SecretPurpose = string(SettingsUpstreamCAPurpose)
	return &sdkmodels.ConfigurationUIDescriptor{
		Version: sdkmodels.ConfigurationUIVersion,
		Groups: []sdkmodels.ConfigurationUIGroup{
			{
				ID: "listeners", Title: text("Слушатели", "Listeners"),
				Description: &sdkmodels.ConfigurationUIText{RU: "HTTP-bind, протоколы и TLS. Studio показывает только подписи и opaque references; значения остаются в локальном системном хранилище.", EN: "HTTP binds, protocols and TLS. Studio shows only labels and opaque references; values remain in the local system vault."},
				Order:       10, Fields: []sdkmodels.ConfigurationUIField{
					field("/config/listeners/*/id", "ID слушателя", "Listener ID", "text", 10),
					field("/config/listeners/*/address", "Адрес bind", "Bind address", "text", 20),
					field("/config/listeners/*/hostnames", "Имена хостов", "Hostnames", "array", 30),
					field("/config/listeners/*/protocols", "HTTP-протоколы", "HTTP protocols", "array", 40),
					field("/config/listeners/*/tls/mode", "Режим TLS", "TLS mode", "select", 50),
					field("/config/listeners/*/redirectToListenerId", "Listener для redirect", "Redirect target listener", "text", 60),
					certificateSecretField, privateKeySecretField,
				},
			},
			{
				ID: "service-routes", Title: text("Маршруты сервисов", "Service routes"),
				Description: &sdkmodels.ConfigurationUIText{RU: "HTTP endpoints маршрутизируют запросы к независимому сервису по instance ID и capability.", EN: "HTTP endpoints route requests to an independent service by instance ID and capability."},
				Order:       20, Fields: []sdkmodels.ConfigurationUIField{
					field("/config/routes/*/id", "ID маршрута", "Route ID", "text", 10),
					field("/config/routes/*/listenerId", "Слушатель", "Listener", "text", 20),
					field("/config/routes/*/handler/type", "Тип обработчика", "Handler type", "select", 30),
					field("/config/routes/*/handler/instanceId", "ID экземпляра сервиса", "Service instance ID", "text", 40),
					field("/config/routes/*/handler/capability", "Capability", "Capability", "text", 50),
					field("/config/routes/*/handler/mode", "Режим вызова", "Invocation mode", "select", 60),
					field("/config/routes/*/handler/requestCookieNames", "Разрешённые Cookie", "Allowed cookies", "array", 70),
					siteResourceField,
					releaseResourceField,
					field("/config/routes/*/api/method", "HTTP method", "HTTP method", "text", 80),
					field("/config/routes/*/api/path", "Шаблон пути", "Path template", "text", 90),
					authSecretField, upstreamCASecretField,
				},
			},
			{
				ID: "route-defaults", Title: text("Общие значения маршрутов", "Route defaults"),
				Description: &sdkmodels.ConfigurationUIText{RU: "Общие значения по типам маршрутов. Параметры секретов содержат только opaque references.", EN: "Shared values by route type. Secret fields contain only opaque references."},
				Order:       15, Fields: authDefaultSecretFields,
			},
			{
				ID: "request-contract", Title: text("Контракт запроса", "Request contract"),
				Description: &sdkmodels.ConfigurationUIText{RU: "Типизированные параметры проверяются до вызова целевого сервиса.", EN: "Typed parameters are validated before the target service is called."},
				Order:       30, Fields: []sdkmodels.ConfigurationUIField{
					field("/config/routes/*/api/pathParameters/*/name", "Имя path-параметра", "Path parameter name", "text", 10),
					field("/config/routes/*/api/pathParameters/*/type", "Тип path-параметра", "Path parameter type", "select", 20),
					field("/config/routes/*/api/queryParameters/*/name", "Имя query-параметра", "Query parameter name", "text", 30),
					field("/config/routes/*/api/queryParameters/*/type", "Тип query-параметра", "Query parameter type", "select", 40),
					field("/config/routes/*/api/queryParameters/*/repeated", "Правило повторения", "Repetition policy", "select", 50),
					field("/config/routes/*/api/queryParameters/*/required", "Обязательный query", "Required query parameter", "toggle", 60),
					field("/config/routes/*/api/headerParameters/*/name", "Имя header", "Header name", "text", 70),
					field("/config/routes/*/api/headerParameters/*/type", "Тип header", "Header type", "select", 80),
					field("/config/routes/*/api/headerParameters/*/repeated", "Правило повторения header", "Header repetition policy", "select", 90),
					field("/config/routes/*/api/headerParameters/*/required", "Обязательный header", "Required header", "toggle", 100),
					field("/config/routes/*/api/allowUnknownQuery", "Разрешать неизвестные query", "Allow unknown query parameters", "toggle", 110),
					field("/config/routes/*/api/allowUnknownHeaders", "Разрешать неизвестные headers", "Allow unknown headers", "toggle", 120),
					field("/config/routes/*/api/body/required", "Обязательный JSON body", "Required JSON body", "toggle", 130),
					field("/config/routes/*/api/body/maxBytes", "Максимальный JSON body", "Maximum JSON body", "slider", 140),
					field("/config/routes/*/api/responses/*/status", "HTTP status ответа", "Response HTTP status", "slider", 150),
					field("/config/routes/*/api/responses/*/contentType", "Content-Type ответа", "Response Content-Type", "text", 160),
				},
			},
		},
	}
}

func (adapter *Adapter) Manifest(ctx context.Context) ([]byte, error) {
	if adapter == nil || adapter.manifest == nil {
		return nil, contracts.ErrInvalidAssets
	}
	return adapter.manifest.Manifest(ctx)
}

func (adapter *Adapter) ConfigurationSchema(context.Context) ([]byte, error) {
	if adapter == nil {
		return nil, contracts.ErrInvalidAssets
	}
	return contracts.SettingsSchema()
}

func (adapter *Adapter) ValidateConfiguration(ctx context.Context, contents []byte) error {
	if adapter == nil || adapter.configuration == nil || ctx.Err() != nil {
		return settingsmodel.ErrInvalidSettings
	}
	if err := contracts.ValidateSettings(contents); err != nil {
		return settingsmodel.ErrInvalidSettings
	}
	settings, err := settingsmodel.DecodeSettings(contents)
	if err != nil {
		return settingsmodel.ErrInvalidSettings
	}
	if err := adapter.validateStaticRouteReleases(ctx, settings.RuntimeConfig); err != nil {
		return settingsmodel.ErrInvalidSettings
	}
	return adapter.configuration.Validate(settings)
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
	if err := adapter.validateStaticRouteReleases(ctx, settings.RuntimeConfig); err != nil {
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

func (adapter *Adapter) validateStaticRouteReleases(ctx context.Context, runtimeConfig []byte) error {
	if adapter == nil || ctx == nil || ctx.Err() != nil {
		return settingsmodel.ErrInvalidSettings
	}
	var candidate struct {
		Routes []struct {
			Handler struct {
				Type      string `json:"type"`
				SiteID    string `json:"siteId"`
				ReleaseID string `json:"releaseId"`
			} `json:"handler"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(runtimeConfig, &candidate); err != nil {
		return settingsmodel.ErrInvalidSettings
	}
	for _, route := range candidate.Routes {
		if route.Handler.Type != "static" {
			continue
		}
		if adapter.publisher == nil {
			return settingsmodel.ErrInvalidSettings
		}
		if err := adapter.publisher.ValidatePinnedRelease(ctx, route.Handler.SiteID, route.Handler.ReleaseID); err != nil {
			return settingsmodel.ErrInvalidSettings
		}
	}
	return nil
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
		case settingsmodel.SecretReferenceHTTPAuth:
			purpose = SettingsHTTPAuthPurpose
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

// NewHandler creates the Service SDK REST lifecycle handler and its use case.
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
	metadata := sdkinterfaces.ServiceMetadata(adapter)
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

// NewMutualTLSServer builds the SDK-owned Core↔service REST server. The Core
// identity, credential provider and revocation source are required inputs and
// are never discovered from request data or product settings.
func NewMutualTLSServer(
	configuration *settingsapp.Configuration,
	publisher *siteapp.SitePublisher,
	options LifecycleOptions,
) (*sdkinfra.MutualTLSServer, *sdkapp.Lifecycle, *sdkinfra.ObserverPrometheusCollector, error) {
	handler, lifecycle, collector, err := NewHandler(configuration, publisher, options)
	if err != nil {
		return nil, nil, nil, err
	}
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		return nil, nil, nil, err
	}
	if options.Credentials == nil || !options.CorePeer.Valid() || options.Revocation == nil {
		return nil, nil, nil, sdkinfra.ErrInvalidServerTLS
	}
	server, err := sdkinfra.NewMutualTLSServer(contract, sdkinfra.MutualTLSServerConfig{
		Handler:    handler,
		Provider:   options.Credentials,
		Peer:       options.CorePeer,
		Revocation: options.Revocation,
		ErrorLog:   options.ErrorLog,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return server, lifecycle, collector, nil
}

// NewPublicObservabilityHandler returns the universal SDK healthz/metrics
// surface for Server's public listener. Server supplies only the versioned
// contract and SDK collector; it does not implement or customize either route.
func NewPublicObservabilityHandler(contract sdkinfra.HTTPContract, collector *sdkinfra.ObserverPrometheusCollector) (http.Handler, error) {
	contracts := presentationContracts(contract)
	return sdkpresentation.NewObservabilityHandler(sdkpresentation.ObservabilityConfiguration{
		HealthEndpoint:   contracts.HealthEndpoint,
		MetricsEndpoint:  contracts.MetricsEndpoint,
		HealthStatus:     contracts.HealthStatus,
		HealthBody:       contracts.HealthBody,
		ContentTypes:     contracts.ContentTypes,
		Metrics:          collector,
		NotFound:         contracts.Problems["notFound"],
		MethodNotAllowed: contracts.Problems["methodNotAllowed"],
		InternalError:    contracts.Problems["internalError"],
	})
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
		ConfigSchemaEndpoint: endpoint("configSchema"), ConfigValidationEndpoint: endpoint("configValidate"), HealthEndpoint: endpoint("health"),
		ReadyEndpoint: endpoint("ready"), ReloadEndpoint: endpoint("reload"),
		MetricsEndpoint: endpoint("metrics"),
		HealthStatus:    contract.Service.Responses.Health.Status,
		HealthBody:      cloneStringMap(contract.Service.Responses.Health.Body),
		ContentTypes: sdkpresentation.ContentTypes{
			JSON: contract.Service.Responses.ContentTypes.JSON, Metrics: contract.Service.Responses.ContentTypes.Metrics,
		},
		ReloadRequest:         document(contract.Service.ReloadRequest),
		ReloadAcknowledgement: document(contract.Service.ReloadAcknowledgement),
		Readiness:             document(contract.Service.Readiness), Manifest: document(contract.Service.Manifest),
		ConfigurationSchema:    document(contract.Service.ConfigurationSchema),
		ArtifactStreamEndpoint: endpoint("artifactStream"), AdminSurfaceEndpoint: endpoint("adminSurface"),
		AdminActionEndpoint: endpoint("adminAction"),
		ArtifactStream: sdkpresentation.ArtifactStreamContract{
			MediaType:                     contract.Service.ArtifactStream.MediaType,
			MetadataMediaType:             contract.Service.ArtifactStream.MetadataMediaType,
			Parts:                         append([]string(nil), contract.Service.ArtifactStream.Parts...),
			PartOrder:                     append([]string(nil), contract.Service.ArtifactStream.PartOrder...),
			MaximumArtifactBytes:          contract.Service.ArtifactStream.MaximumArtifactBytes,
			MinimumArtifactBytes:          contract.Service.ArtifactStream.MinimumArtifactBytes,
			MaximumMetadataBytes:          contract.Service.ArtifactStream.MaximumMetadataBytes,
			MaximumMultipartOverheadBytes: contract.Service.ArtifactStream.MaximumMultipartOverheadBytes,
			MaximumRequestBytes:           contract.Service.ArtifactStream.MaximumRequestBytes,
			MaximumReceiptBytes:           contract.Service.ArtifactStream.MaximumReceiptBytes,
			AcceptedStatus:                contract.Service.ArtifactStream.AcceptedStatus,
			Deadline:                      time.Duration(contract.Deadlines.ArtifactStreamSeconds) * time.Second,
			FilenameForwarded:             contract.Service.ArtifactStream.FilenameForwarded,
			InvocationContext: sdkpresentation.ArtifactInvocationContract{
				MaximumBytes: contract.Service.ArtifactStream.InvocationContext.MaximumBytes,
				Required:     append([]string(nil), contract.Service.ArtifactStream.InvocationContext.Required...),
				Optional:     append([]string(nil), contract.Service.ArtifactStream.InvocationContext.Optional...),
				Headers:      cloneStringMap(contract.Service.ArtifactStream.InvocationContext.Headers),
			},
		},
		AdminSurface: document(contract.Service.AdminSurface),
		AdminAction: sdkpresentation.AdminActionContract{
			MediaType:            contract.Service.AdminAction.MediaType,
			MaximumRequestBytes:  contract.Service.AdminAction.MaximumRequestBytes,
			MaximumResponseBytes: contract.Service.AdminAction.MaximumResponseBytes,
			MaximumPageIDBytes:   contract.Service.AdminAction.MaximumPageIDBytes,
			MaximumActionIDBytes: contract.Service.AdminAction.MaximumActionIDBytes,
			PathSegmentPattern:   contract.Service.AdminAction.PathSegmentPattern,
			ResponseStatus: sdkpresentation.StatusRangeContract{
				Minimum: contract.Service.AdminAction.ResponseStatus.Minimum,
				Maximum: contract.Service.AdminAction.ResponseStatus.Maximum,
			},
			Deadline: time.Duration(contract.Service.AdminAction.DeadlineSeconds) * time.Second,
			InvocationContext: sdkpresentation.AdminInvocationContract{
				MaximumBytes:        contract.Service.AdminAction.InvocationContext.MaximumBytes,
				UnknownHeaderPrefix: contract.Service.AdminAction.InvocationContext.UnknownHeaderPrefix,
				Required:            append([]string(nil), contract.Service.AdminAction.InvocationContext.Required...),
				Optional:            append([]string(nil), contract.Service.AdminAction.InvocationContext.Optional...),
				Headers:             cloneStringMap(contract.Service.AdminAction.InvocationContext.Headers),
			},
		},
		Registration: document(sdkinfra.DocumentContract{
			MediaType:    contract.Identity.Registration.MediaType,
			MaximumBytes: contract.Identity.Registration.MaximumBytes,
			Required:     contract.Identity.Registration.Required,
		}),
		MaximumMetadataBytes: contract.Service.MaximumMetadataBytes,
		ReadinessDeadline:    time.Duration(contract.Deadlines.ServiceReadinessSeconds) * time.Second,
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

var _ sdkinterfaces.ServiceMetadata = (*Adapter)(nil)
var _ sdkinterfaces.ConfigurationApplier = (*Adapter)(nil)

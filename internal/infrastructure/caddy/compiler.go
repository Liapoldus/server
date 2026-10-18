package caddy

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/textproto"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	caddycore "github.com/caddyserver/caddy/v2"
	"liapoldus.local/server-service/contracts"
)

var errUnsupportedSettings = errors.New("unsupported Caddy settings")

func sortedMethods(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

type settingsConfig struct {
	Listeners []settingsListener           `json:"listeners"`
	Routes    []settingsRoute              `json:"routes"`
	Defaults  settingsRouteFeatureDefaults `json:"defaults"`
}

type settingsRouteFeatureDefaults struct {
	Service      routeFeatureDefaults `json:"service"`
	Static       routeFeatureDefaults `json:"static"`
	ReverseProxy routeFeatureDefaults `json:"reverseProxy"`
	Redirect     routeFeatureDefaults `json:"redirect"`
}

type routeFeatureDefaults struct {
	Compression compressionFeature `json:"compression"`
	Cache       cacheFeature       `json:"cache"`
	Headers     headersFeature     `json:"headers"`
	CORS        corsFeature        `json:"cors"`
	Auth        authFeature        `json:"auth"`
	RateLimit   rateLimitFeature   `json:"rateLimit"`
	HealthCheck healthCheckFeature `json:"healthCheck"`
	Rewrite     rewriteFeature     `json:"rewrite"`
}

type routeFeatureOverrides struct {
	Compression compressionFeature `json:"compression"`
	Cache       cacheFeature       `json:"cache"`
	Headers     headersFeature     `json:"headers"`
	CORS        corsFeature        `json:"cors"`
	Auth        authFeature        `json:"auth"`
	RateLimit   rateLimitFeature   `json:"rateLimit"`
	HealthCheck healthCheckFeature `json:"healthCheck"`
	Rewrite     rewriteFeature     `json:"rewrite"`
}

type compressionFeature struct {
	Mode       string   `json:"mode"`
	Algorithms []string `json:"algorithms"`
}

type cacheFeature struct {
	Mode          string `json:"mode"`
	Visibility    string `json:"visibility"`
	MaxAgeSeconds *int   `json:"maxAgeSeconds"`
}

type headersFeature struct {
	Mode     string            `json:"mode"`
	Request  map[string]string `json:"request"`
	Response map[string]string `json:"response"`
}

type corsFeature struct {
	Mode             string   `json:"mode"`
	Origins          []string `json:"origins"`
	Methods          []string `json:"methods"`
	AllowedHeaders   []string `json:"allowedHeaders"`
	ExposedHeaders   []string `json:"exposedHeaders"`
	AllowCredentials *bool    `json:"allowCredentials"`
	MaxAgeSeconds    *int     `json:"maxAgeSeconds"`
}

type authFeature struct {
	Mode       string `json:"mode"`
	Scheme     string `json:"scheme"`
	SecretRef  string `json:"secretRef"`
	HeaderName string `json:"headerName"`
	Username   string `json:"username"`
	Realm      string `json:"realm"`
}

type rateLimitFeature struct {
	Mode              string   `json:"mode"`
	RequestsPerSecond int      `json:"requestsPerSecond"`
	Burst             int      `json:"burst"`
	TrustedProxies    []string `json:"trustedProxies"`
}

type healthCheckFeature struct {
	Mode            string `json:"mode"`
	Path            string `json:"path"`
	IntervalSeconds int    `json:"intervalSeconds"`
	TimeoutSeconds  int    `json:"timeoutSeconds"`
	ExpectedStatus  int    `json:"expectedStatus"`
}

type rewriteFeature struct {
	Mode string `json:"mode"`
	Path string `json:"path"`
}

type effectiveRouteFeatures struct {
	Compression compressionFeature
	Cache       cacheFeature
	Headers     headersFeature
	CORS        corsFeature
	Auth        authFeature
	RateLimit   rateLimitFeature
	HealthCheck healthCheckFeature
	Rewrite     rewriteFeature
}

type settingsListener struct {
	TLS struct {
		Mode           string `json:"mode"`
		CertificateRef string `json:"certificateRef"`
		PrivateKeyRef  string `json:"privateKeyRef"`
	} `json:"tls"`
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	Address              string   `json:"address"`
	RedirectToListenerID string   `json:"redirectToListenerId"`
	Hostnames            []string `json:"hostnames"`
	Protocols            []string `json:"protocols"`
}

type settingsRoute struct {
	ID         string                `json:"id"`
	ListenerID string                `json:"listenerId"`
	Match      *settingsRouteMatch   `json:"match"`
	API        *apiContract          `json:"api"`
	Handler    settingsHandler       `json:"handler"`
	Features   routeFeatureOverrides `json:"features"`
}

type settingsHandler struct {
	Type               string     `json:"type"`
	SiteID             string     `json:"siteId"`
	ReleaseID          string     `json:"releaseId"`
	MountPrefix        string     `json:"mountPrefix"`
	InstanceID         string     `json:"instanceId"`
	Capability         string     `json:"capability"`
	Mode               string     `json:"mode"`
	RequestCookieNames []string   `json:"requestCookieNames,omitempty"`
	Upstreams          []upstream `json:"upstreams"`
	Target             string     `json:"target"`
	StatusCode         int        `json:"statusCode"`
	StreamLimits       struct {
		MaxConcurrency    int `json:"maxConcurrency"`
		IdleTimeoutMillis int `json:"idleTimeoutMillis"`
		MaxDurationMillis int `json:"maxDurationMillis"`
	} `json:"streamLimits,omitempty"`
}

type upstream struct {
	Origin string `json:"origin"`
	CARef  string `json:"caRef"`
	Weight int    `json:"weight"`
}

func compileSettings(contents []byte, callerSetID uint64, secrets map[string][]byte) ([]byte, error) {
	var desired settingsConfig
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&desired); err != nil {
		return nil, errUnsupportedSettings
	}
	listeners := make(map[string]settingsListener, len(desired.Listeners))
	servers := make(map[string]any, len(desired.Listeners))
	httpLimits, err := contracts.LoadHTTPDispatch()
	if err != nil {
		return nil, err
	}
	tlsCertificates := make([]any, 0, len(desired.Listeners))
	usedSecretReferences := make(map[string]struct{})
	for _, listener := range desired.Listeners {
		protocols, alpn, err := compileHTTPProtocols(listener.Protocols, listener.TLS.Mode)
		if err != nil || listener.ID == "" || listener.Kind != "http" || validateSocketAddress(listener.Address) != nil {
			return nil, errUnsupportedSettings
		}
		switch listener.TLS.Mode {
		case "disabled":
			if len(listener.Hostnames) != 0 || len(listener.Protocols) != 1 || protocols[0] != "h1" || (listener.RedirectToListenerID != "" && len(listener.Hostnames) != 0) {
				return nil, errUnsupportedSettings
			}
		case "custom":
			if len(listener.Hostnames) == 0 || listener.TLS.CertificateRef == "" || listener.TLS.PrivateKeyRef == "" {
				return nil, errUnsupportedSettings
			}
			certificate, certificateExists := secrets[listener.TLS.CertificateRef]
			privateKey, privateKeyExists := secrets[listener.TLS.PrivateKeyRef]
			if !certificateExists || !privateKeyExists {
				return nil, errUnsupportedSettings
			}
			if _, err := tls.X509KeyPair(certificate, privateKey); err != nil {
				return nil, errUnsupportedSettings
			}
			tlsCertificates = append(tlsCertificates, map[string]any{
				"certificate": string(certificate),
				"key":         string(privateKey),
			})
			usedSecretReferences[listener.TLS.CertificateRef] = struct{}{}
			usedSecretReferences[listener.TLS.PrivateKeyRef] = struct{}{}
		case "automatic":
			if len(listener.Hostnames) == 0 {
				return nil, errUnsupportedSettings
			}
		default:
			return nil, errUnsupportedSettings
		}
		for _, hostname := range listener.Hostnames {
			if _, err := canonicalConfiguredHost(hostname); err != nil {
				return nil, errUnsupportedSettings
			}
		}
		if _, exists := listeners[listener.ID]; exists {
			return nil, errUnsupportedSettings
		}
		listeners[listener.ID] = listener
		server := map[string]any{"listen": []string{listener.Address}, "protocols": protocols, "routes": []any{}, "max_header_bytes": httpLimits.MaxRequestHeaderBytes}
		switch listener.TLS.Mode {
		case "disabled", "custom":
			server["automatic_https"] = map[string]any{"disable": true, "disable_redirects": true}
		case "automatic":
			server["automatic_https"] = map[string]any{"disable_redirects": true}
		}
		if listener.TLS.Mode == "custom" {
			server["tls_connection_policies"] = []any{map[string]any{
				"match":        map[string]any{"sni": listener.Hostnames},
				"alpn":         alpn,
				"protocol_min": "tls1.2",
				"protocol_max": "tls1.3",
			}}
		}
		servers[listener.ID] = server
	}

	routesByListener := make(map[string][]any, len(listeners))
	matcherIDsByListener := make(map[string]map[string]struct{}, len(listeners))
	usedTargets := make(map[string]map[string]struct{})
	upstreamPools := make([]upstreamPoolConfig, 0)
	routeIDs := make(map[string]struct{}, len(desired.Routes))
	for _, route := range desired.Routes {
		if route.ID == "" || route.ListenerID == "" {
			return nil, errUnsupportedSettings
		}
		if _, exists := routeIDs[route.ID]; exists {
			return nil, errUnsupportedSettings
		}
		routeIDs[route.ID] = struct{}{}
		if _, exists := listeners[route.ListenerID]; !exists {
			return nil, errUnsupportedSettings
		}
		matcherSource := route.Match
		if route.Handler.Type == "service" {
			if route.API == nil || (matcherSource != nil && (matcherSource.Path != nil || len(matcherSource.Methods) != 0)) {
				return nil, errUnsupportedSettings
			}
			if _, err := compileAPIContract(*route.API, route.ID, route.Handler.Mode); err != nil {
				return nil, errUnsupportedSettings
			}
			var hosts []string
			if matcherSource != nil {
				hosts = matcherSource.Hosts
			}
			matcherSource = &settingsRouteMatch{
				Hosts: hosts, Methods: []string{route.API.Method},
				Path: &pathMatcher{Type: "template", Value: route.API.Path},
			}
		} else if route.Handler.Type == "static" {
			if !validStaticMountPrefix(route.Handler.MountPrefix) {
				return nil, errUnsupportedSettings
			}
			var hosts []string
			var methods []string
			if matcherSource != nil {
				hosts = matcherSource.Hosts
				if matcherSource.Path != nil && (matcherSource.Path.Type != "prefix" || matcherSource.Path.Value != route.Handler.MountPrefix) {
					return nil, errUnsupportedSettings
				}
				for _, method := range matcherSource.Methods {
					if method != "GET" && method != "HEAD" {
						return nil, errUnsupportedSettings
					}
				}
				methods = matcherSource.Methods
			}
			matcherSource = &settingsRouteMatch{Hosts: hosts, Methods: methods, Path: &pathMatcher{Type: "prefix", Value: route.Handler.MountPrefix}}
		} else if route.API != nil || (matcherSource != nil && matcherSource.Path != nil && matcherSource.Path.Type == "template") {
			return nil, errUnsupportedSettings
		}
		matcher, identity, err := canonicalMatcher(matcherSource)
		if err != nil {
			return nil, errUnsupportedSettings
		}
		if matcherIDsByListener[route.ListenerID] == nil {
			matcherIDsByListener[route.ListenerID] = make(map[string]struct{})
		}
		if _, exists := matcherIDsByListener[route.ListenerID][identity]; exists {
			return nil, errUnsupportedSettings
		}
		matcherIDsByListener[route.ListenerID][identity] = struct{}{}
		defaults, err := routeDefaultsForType(route.Handler.Type, desired.Defaults)
		if err != nil {
			return nil, errUnsupportedSettings
		}
		features, err := resolveRouteFeatures(route.Handler.Type, defaults, route.Features)
		if err != nil || features.Compression.Mode == "enabled" && route.Handler.Type == "service" && route.Handler.Mode != "call" {
			return nil, errUnsupportedSettings
		}
		if route.Handler.Type == "reverseProxy" && features.HealthCheck.Mode == "enabled" && !uniqueUpstreamDialAddresses(route.Handler.Upstreams) {
			return nil, errUnsupportedSettings
		}
		handler, err := compileHandler(route.ID, route.Handler, route.API, usedTargets, &upstreamPools, secrets, usedSecretReferences)
		if err != nil {
			return nil, err
		}
		if route.Handler.Type == "reverseProxy" && features.HealthCheck.Mode == "enabled" {
			handler["health_checks"] = map[string]any{"active": activeHealthCheckConfig(features.HealthCheck)}
		}
		handlers := make([]any, 0, 10)
		if features.Compression.Mode == "enabled" {
			encodings := make(map[string]any, len(features.Compression.Algorithms))
			for _, algorithm := range features.Compression.Algorithms {
				encodings[algorithm] = map[string]any{}
			}
			handlers = append(handlers, map[string]any{"handler": "encode", "encodings": encodings, "prefer": features.Compression.Algorithms})
		}
		requestHeaders := make(map[string]any, len(features.Headers.Request))
		for name, value := range features.Headers.Request {
			requestHeaders[textproto.CanonicalMIMEHeaderKey(name)] = []string{value}
		}
		if features.Auth.Mode == "enabled" {
			authHeader := "Authorization"
			if features.Auth.Scheme == "apiKey" {
				authHeader = features.Auth.HeaderName
			}
			for name := range requestHeaders {
				if strings.EqualFold(name, authHeader) {
					return nil, errUnsupportedSettings
				}
			}
		}
		responseHeaders := make(map[string]any, len(features.Headers.Response)+1)
		for name, value := range features.Headers.Response {
			responseHeaders[textproto.CanonicalMIMEHeaderKey(name)] = []string{value}
		}
		if features.Cache.Mode == "enabled" {
			cacheControl := fmt.Sprintf("%s, max-age=%d", features.Cache.Visibility, *features.Cache.MaxAgeSeconds)
			for name := range responseHeaders {
				if strings.EqualFold(name, "Cache-Control") {
					return nil, errUnsupportedSettings
				}
			}
			responseHeaders["Cache-Control"] = []string{cacheControl}
		}
		if len(requestHeaders) != 0 || len(responseHeaders) != 0 {
			handlers = append(handlers, map[string]any{"handler": "headers", "request": map[string]any{"set": requestHeaders}, "response": map[string]any{"set": responseHeaders, "deferred": true}})
		}
		if features.CORS.Mode == "enabled" {
			corsConfig := map[string]any{
				"handler": "liapoldus_cors", "origins": features.CORS.Origins, "methods": features.CORS.Methods,
				"allowedHeaders": features.CORS.AllowedHeaders, "exposedHeaders": features.CORS.ExposedHeaders,
				"maxAgeSeconds": *features.CORS.MaxAgeSeconds,
			}
			if features.CORS.AllowCredentials != nil {
				corsConfig["allowCredentials"] = *features.CORS.AllowCredentials
			}
			handlers = append(handlers, corsConfig)
		}
		if features.RateLimit.Mode == "enabled" {
			handlers = append(handlers, rateLimitConfig(features.RateLimit))
		}
		if features.Auth.Mode == "enabled" {
			secret, exists := secrets[features.Auth.SecretRef]
			if !exists {
				return nil, errUnsupportedSettings
			}
			secretMAC, macErr := authSecretMAC(secret)
			if macErr != nil {
				return nil, errUnsupportedSettings
			}
			usedSecretReferences[features.Auth.SecretRef] = struct{}{}
			authConfig := map[string]any{"handler": authHandlerModule, "scheme": features.Auth.Scheme, "secretMac": secretMAC}
			if features.Auth.HeaderName != "" {
				authConfig["headerName"] = features.Auth.HeaderName
			}
			if features.Auth.Username != "" {
				authConfig["username"] = features.Auth.Username
			}
			if features.Auth.Realm != "" {
				authConfig["realm"] = features.Auth.Realm
			}
			handlers = append(handlers, authConfig)
		}
		if features.Rewrite.Mode == "enabled" {
			handlers = append(handlers, map[string]any{"handler": "rewrite", "uri": features.Rewrite.Path})
		}
		handlers = append(handlers, handler)
		matcherConfig := map[string]any{}
		if len(matcher.Hosts) != 0 {
			matcherConfig["hosts"] = matcher.Hosts
		}
		if len(matcher.Methods) != 0 {
			matcherConfig["methods"] = matcher.Methods
		}
		if matcher.Path != nil {
			matcherConfig["path"] = matcher.Path
		}
		if route.Handler.Mode == "http_stream" || route.Handler.Mode == "websocket" || route.Handler.Mode == "sse" {
			limits, limitsErr := contracts.LoadHTTPDispatch()
			if limitsErr != nil {
				return nil, limitsErr
			}
			if route.Handler.StreamLimits.MaxConcurrency == 0 {
				route.Handler.StreamLimits.MaxConcurrency = limits.MaxStreamConcurrencyPerInstance
			}
			if route.Handler.StreamLimits.IdleTimeoutMillis == 0 {
				route.Handler.StreamLimits.IdleTimeoutMillis = limits.DefaultStreamIdleTimeoutMillis
			}
			if route.Handler.StreamLimits.MaxDurationMillis == 0 {
				route.Handler.StreamLimits.MaxDurationMillis = limits.DefaultStreamMaxDurationMillis
			}
			if route.Handler.StreamLimits.MaxConcurrency > limits.MaxStreamConcurrencyPerInstance || route.Handler.StreamLimits.IdleTimeoutMillis > limits.DefaultStreamIdleTimeoutMillis || route.Handler.StreamLimits.MaxDurationMillis > limits.DefaultStreamMaxDurationMillis {
				return nil, errUnsupportedSettings
			}
			handler["maxConcurrentStreams"] = route.Handler.StreamLimits.MaxConcurrency
			handler["idleTimeoutMillis"] = route.Handler.StreamLimits.IdleTimeoutMillis
			handler["maxDurationMillis"] = route.Handler.StreamLimits.MaxDurationMillis
		}
		routesByListener[route.ListenerID] = append(routesByListener[route.ListenerID], map[string]any{
			"match":    []any{map[string]any{routeMatcherModule: matcherConfig}},
			"handle":   handlers,
			"terminal": true,
		})
	}
	for _, listener := range desired.Listeners {
		if listener.RedirectToListenerID == "" {
			continue
		}
		target, exists := listeners[listener.RedirectToListenerID]
		if !exists || target.TLS.Mode == "disabled" || len(routesByListener[listener.ID]) != 0 {
			return nil, errUnsupportedSettings
		}
		routesByListener[listener.ID] = []any{map[string]any{
			"handle": []any{map[string]any{
				"handler":         httpsRedirectModule,
				"targetHostnames": target.Hostnames,
			}},
			"terminal": true,
		}}
	}
	for listenerID, listener := range listeners {
		if listener.TLS.Mode != "automatic" {
			continue
		}
		serverRoutes := routesByListener[listenerID]
		for _, hostname := range listener.Hostnames {
			serverRoutes = append(serverRoutes, map[string]any{
				"match": []any{map[string]any{"host": []string{hostname}}},
				"handle": []any{map[string]any{
					"handler":     "static_response",
					"status_code": 404,
				}},
				"terminal": true,
			})
		}
		routesByListener[listenerID] = serverRoutes
	}
	for listenerID := range listeners {
		routesByListener[listenerID] = append(routesByListener[listenerID], map[string]any{
			"handle": []any{map[string]any{
				"handler":     "static_response",
				"status_code": 404,
			}},
			"terminal": true,
		})
	}
	for listenerID, server := range servers {
		serverConfig := server.(map[string]any)
		guard := map[string]any{"handle": []any{headerLimitHandler(httpLimits)}, "terminal": false}
		serverConfig["routes"] = append([]any{guard}, routesByListener[listenerID]...)
	}

	apps := map[string]any{
		"http": map[string]any{"servers": servers},
	}
	if len(tlsCertificates) != 0 {
		apps["tls"] = map[string]any{"certificates": map[string]any{"load_pem": tlsCertificates}}
	}
	if len(upstreamPools) != 0 {
		apps[upstreamPoolsAppID] = upstreamPoolAppConfig{Pools: upstreamPools}
	}
	if len(usedSecretReferences) != len(secrets) {
		return nil, errUnsupportedSettings
	}
	if len(usedTargets) != 0 {
		contract, err := contracts.LoadHTTPDispatch()
		if err != nil {
			return nil, err
		}
		instances := make([]any, 0, len(usedTargets))
		instanceIDs := make([]string, 0, len(usedTargets))
		for instanceID := range usedTargets {
			instanceIDs = append(instanceIDs, instanceID)
		}
		sort.Strings(instanceIDs)
		for _, instanceID := range instanceIDs {
			methods := usedTargets[instanceID]
			instances = append(instances, map[string]any{"id": instanceID, "methods": sortedMethods(methods)})
		}
		apps[contract.App] = map[string]any{"instances": instances, "callerSetId": callerSetID}
	}

	compiled, err := json.Marshal(map[string]any{
		"admin": map[string]any{"disabled": true, "config": map[string]any{"persist": false}},
		"apps":  apps,
	})
	if err != nil {
		return nil, errUnsupportedSettings
	}
	return compiled, nil
}

func routeDefaultsForType(routeType string, configured settingsRouteFeatureDefaults) (routeFeatureDefaults, error) {
	switch routeType {
	case "service":
		return configured.Service, nil
	case "static":
		return configured.Static, nil
	case "reverseProxy":
		return configured.ReverseProxy, nil
	case "redirect":
		return configured.Redirect, nil
	default:
		return routeFeatureDefaults{}, errUnsupportedSettings
	}
}

func resolveRouteFeatures(routeType string, defaults routeFeatureDefaults, overrides routeFeatureOverrides) (effectiveRouteFeatures, error) {
	implicit := routeFeatureDefaults{
		Compression: compressionFeature{Mode: "disabled", Algorithms: []string{"zstd", "gzip"}},
		Cache:       cacheFeature{Mode: "disabled", Visibility: "private", MaxAgeSeconds: featureIntPointer(0)},
		Headers:     headersFeature{Mode: "disabled"},
		CORS:        corsFeature{Mode: "disabled", MaxAgeSeconds: featureIntPointer(0)},
		Auth:        authFeature{Mode: "disabled"},
		RateLimit:   rateLimitFeature{Mode: "disabled"},
		HealthCheck: healthCheckFeature{Mode: "disabled", Path: "/healthz", IntervalSeconds: 30, TimeoutSeconds: 5, ExpectedStatus: 200},
		Rewrite:     rewriteFeature{Mode: "disabled"},
	}
	if routeType == "static" {
		implicit.Compression.Mode = "enabled"
		implicit.Cache = cacheFeature{Mode: "enabled", Visibility: "public", MaxAgeSeconds: featureIntPointer(3600)}
	}
	defaults = mergeFeatureDefaults(implicit, defaults)
	if !validDefaultFeature(defaults.Compression.Mode) || !validCompressionAlgorithms(defaults.Compression.Algorithms) ||
		!validDefaultFeature(defaults.Cache.Mode) || !validCacheFeature(defaults.Cache) ||
		!validDefaultFeature(defaults.Headers.Mode) || !validHeaderFeature(defaults.Headers) ||
		!validDefaultFeature(defaults.CORS.Mode) || !validCORSFeature(defaults.CORS, false) ||
		!validAuthFeature(defaults.Auth, false) || !validRateLimitFeature(defaults.RateLimit, false) ||
		!validHealthCheckFeature(defaults.HealthCheck, false) || !validRewriteFeature(defaults.Rewrite, false) {
		return effectiveRouteFeatures{}, errUnsupportedSettings
	}
	compression := defaults.Compression
	if overrides.Compression.Mode != "" {
		switch overrides.Compression.Mode {
		case "inherit":
		case "enabled", "disabled":
			compression.Mode = overrides.Compression.Mode
		default:
			return effectiveRouteFeatures{}, errUnsupportedSettings
		}
		if overrides.Compression.Mode == "enabled" && len(overrides.Compression.Algorithms) > 0 {
			if !validCompressionAlgorithms(overrides.Compression.Algorithms) {
				return effectiveRouteFeatures{}, errUnsupportedSettings
			}
			compression.Algorithms = append([]string(nil), overrides.Compression.Algorithms...)
		}
	}
	cache := defaults.Cache
	if overrides.Cache.Mode != "" {
		switch overrides.Cache.Mode {
		case "inherit":
		case "enabled", "disabled":
			cache.Mode = overrides.Cache.Mode
		default:
			return effectiveRouteFeatures{}, errUnsupportedSettings
		}
		if overrides.Cache.Mode == "enabled" {
			if overrides.Cache.Visibility != "" {
				cache.Visibility = overrides.Cache.Visibility
			}
			if overrides.Cache.MaxAgeSeconds != nil {
				cache.MaxAgeSeconds = featureIntPointer(*overrides.Cache.MaxAgeSeconds)
			}
		}
	}
	if !validCacheFeature(cache) {
		return effectiveRouteFeatures{}, errUnsupportedSettings
	}
	headers := defaults.Headers
	if overrides.Headers.Mode != "" {
		switch overrides.Headers.Mode {
		case "inherit":
		case "enabled", "disabled":
			headers.Mode = overrides.Headers.Mode
		default:
			return effectiveRouteFeatures{}, errUnsupportedSettings
		}
		if overrides.Headers.Mode == "enabled" {
			if overrides.Headers.Request != nil {
				headers.Request = mergeHeaderMaps(headers.Request, overrides.Headers.Request)
			}
			if overrides.Headers.Response != nil {
				headers.Response = mergeHeaderMaps(headers.Response, overrides.Headers.Response)
			}
		}
	}
	if !validHeaderFeature(headers) || !validHeaderFeature(overrides.Headers) {
		return effectiveRouteFeatures{}, errUnsupportedSettings
	}
	cors := defaults.CORS
	if overrides.CORS.Mode != "" {
		switch overrides.CORS.Mode {
		case "inherit":
		case "enabled", "disabled":
			cors.Mode = overrides.CORS.Mode
		default:
			return effectiveRouteFeatures{}, errUnsupportedSettings
		}
		if overrides.CORS.Mode == "enabled" {
			if len(overrides.CORS.Origins) > 0 {
				cors.Origins = append([]string(nil), overrides.CORS.Origins...)
			}
			if len(overrides.CORS.Methods) > 0 {
				cors.Methods = append([]string(nil), overrides.CORS.Methods...)
			}
			if len(overrides.CORS.AllowedHeaders) > 0 {
				cors.AllowedHeaders = append([]string(nil), overrides.CORS.AllowedHeaders...)
			}
			if len(overrides.CORS.ExposedHeaders) > 0 {
				cors.ExposedHeaders = append([]string(nil), overrides.CORS.ExposedHeaders...)
			}
			if overrides.CORS.AllowCredentials != nil {
				value := *overrides.CORS.AllowCredentials
				cors.AllowCredentials = &value
			}
			if overrides.CORS.MaxAgeSeconds != nil {
				cors.MaxAgeSeconds = featureIntPointer(*overrides.CORS.MaxAgeSeconds)
			}
		}
	}
	if !validCORSFeature(cors, false) || !validCORSFeature(overrides.CORS, true) {
		return effectiveRouteFeatures{}, errUnsupportedSettings
	}
	auth, err := resolveAuthFeature(defaults.Auth, overrides.Auth)
	if err != nil {
		return effectiveRouteFeatures{}, errUnsupportedSettings
	}
	rateLimit, err := resolveRateLimitFeature(defaults.RateLimit, overrides.RateLimit)
	if err != nil {
		return effectiveRouteFeatures{}, errUnsupportedSettings
	}
	healthCheck, err := resolveHealthCheckFeature(defaults.HealthCheck, overrides.HealthCheck)
	if err != nil || (healthCheck.Mode == "enabled" && routeType != "reverseProxy") {
		return effectiveRouteFeatures{}, errUnsupportedSettings
	}
	rewrite, err := resolveRewriteFeature(defaults.Rewrite, overrides.Rewrite)
	if err != nil || (rewrite.Mode == "enabled" && routeType == "redirect") {
		return effectiveRouteFeatures{}, errUnsupportedSettings
	}
	if auth.Mode == "enabled" && cache.Mode == "enabled" && cache.Visibility == "public" {
		// Shared static defaults must never turn authenticated responses into
		// publicly cacheable content. A route can opt in with an explicit,
		// private cache policy.
		if overrides.Cache.Mode != "enabled" || overrides.Cache.Visibility != "private" {
			return effectiveRouteFeatures{}, errUnsupportedSettings
		}
	}
	return effectiveRouteFeatures{Compression: compression, Cache: cache, Headers: headers, CORS: cors, Auth: auth, RateLimit: rateLimit, HealthCheck: healthCheck, Rewrite: rewrite}, nil
}

func mergeFeatureDefaults(implicit, configured routeFeatureDefaults) routeFeatureDefaults {
	if configured.Compression.Mode == "" {
		configured.Compression = implicit.Compression
	} else if len(configured.Compression.Algorithms) == 0 {
		configured.Compression.Algorithms = implicit.Compression.Algorithms
	}
	if configured.Cache.Mode == "" {
		configured.Cache = implicit.Cache
	} else {
		if configured.Cache.Visibility == "" {
			configured.Cache.Visibility = implicit.Cache.Visibility
		}
		if configured.Cache.MaxAgeSeconds == nil {
			configured.Cache.MaxAgeSeconds = featureIntPointer(*implicit.Cache.MaxAgeSeconds)
		}
	}
	if configured.Headers.Mode == "" {
		configured.Headers = implicit.Headers
	}
	if configured.CORS.Mode == "" {
		configured.CORS = implicit.CORS
	} else {
		if configured.CORS.MaxAgeSeconds == nil {
			configured.CORS.MaxAgeSeconds = featureIntPointer(*implicit.CORS.MaxAgeSeconds)
		}
	}
	if configured.Auth.Mode == "" {
		configured.Auth = implicit.Auth
	}
	if configured.RateLimit.Mode == "" {
		configured.RateLimit = implicit.RateLimit
	}
	if configured.HealthCheck.Mode == "" {
		configured.HealthCheck = implicit.HealthCheck
	} else {
		if configured.HealthCheck.Path == "" {
			configured.HealthCheck.Path = implicit.HealthCheck.Path
		}
		if configured.HealthCheck.IntervalSeconds == 0 {
			configured.HealthCheck.IntervalSeconds = implicit.HealthCheck.IntervalSeconds
		}
		if configured.HealthCheck.TimeoutSeconds == 0 {
			configured.HealthCheck.TimeoutSeconds = implicit.HealthCheck.TimeoutSeconds
		}
		if configured.HealthCheck.ExpectedStatus == 0 {
			configured.HealthCheck.ExpectedStatus = implicit.HealthCheck.ExpectedStatus
		}
	}
	if configured.Rewrite.Mode == "" {
		configured.Rewrite = implicit.Rewrite
	}
	return configured
}

func validDefaultFeature(mode string) bool { return mode == "enabled" || mode == "disabled" }

func resolveFeatureMode(defaultMode, overrideMode string) (string, error) {
	if overrideMode == "" || overrideMode == "inherit" {
		return defaultMode, nil
	}
	if overrideMode != "enabled" && overrideMode != "disabled" {
		return "", errUnsupportedSettings
	}
	return overrideMode, nil
}

func resolveAuthFeature(defaults, overrides authFeature) (authFeature, error) {
	mode, err := resolveFeatureMode(defaults.Mode, overrides.Mode)
	if err != nil {
		return authFeature{}, err
	}
	result := defaults
	result.Mode = mode
	if overrides.Scheme != "" {
		result.Scheme = overrides.Scheme
	}
	if overrides.SecretRef != "" {
		result.SecretRef = overrides.SecretRef
	}
	if overrides.HeaderName != "" {
		result.HeaderName = overrides.HeaderName
	}
	if overrides.Username != "" {
		result.Username = overrides.Username
	}
	if overrides.Realm != "" {
		result.Realm = overrides.Realm
	}
	if !validAuthFeature(result, false) || !validAuthFeature(overrides, true) {
		return authFeature{}, errUnsupportedSettings
	}
	return result, nil
}

func validAuthFeature(feature authFeature, allowInherit bool) bool {
	if feature.Mode != "enabled" && feature.Mode != "disabled" && (!allowInherit || feature.Mode != "" && feature.Mode != "inherit") {
		return false
	}
	if feature.Scheme != "" && feature.Scheme != "bearer" && feature.Scheme != "apiKey" && feature.Scheme != "basic" {
		return false
	}
	if feature.SecretRef != "" && !secretReferencePattern.MatchString(feature.SecretRef) {
		return false
	}
	if feature.HeaderName != "" && (!validFeatureHeaderName(feature.HeaderName) || strings.EqualFold(feature.HeaderName, "authorization")) {
		return false
	}
	if feature.Username != "" && (len(feature.Username) > 128 || strings.ContainsAny(feature.Username, "\r\n\x00")) {
		return false
	}
	if feature.Realm != "" && (len(feature.Realm) > 128 || strings.ContainsAny(feature.Realm, "\r\n\x00")) {
		return false
	}
	if feature.Mode == "enabled" && (feature.Scheme == "" || feature.SecretRef == "" || feature.Scheme == "basic" && feature.Username == "" || feature.Scheme == "apiKey" && feature.HeaderName == "") {
		return false
	}
	return true
}

func resolveRateLimitFeature(defaults, overrides rateLimitFeature) (rateLimitFeature, error) {
	mode, err := resolveFeatureMode(defaults.Mode, overrides.Mode)
	if err != nil {
		return rateLimitFeature{}, err
	}
	result := defaults
	result.Mode = mode
	if overrides.RequestsPerSecond != 0 {
		result.RequestsPerSecond = overrides.RequestsPerSecond
	}
	if overrides.Burst != 0 {
		result.Burst = overrides.Burst
	}
	if overrides.TrustedProxies != nil {
		result.TrustedProxies = append([]string(nil), overrides.TrustedProxies...)
	}
	if !validRateLimitFeature(result, false) || !validRateLimitFeature(overrides, true) {
		return rateLimitFeature{}, errUnsupportedSettings
	}
	return result, nil
}

func validRateLimitFeature(feature rateLimitFeature, allowInherit bool) bool {
	if feature.Mode != "enabled" && feature.Mode != "disabled" && (!allowInherit || feature.Mode != "" && feature.Mode != "inherit") {
		return false
	}
	if len(feature.TrustedProxies) > 32 {
		return false
	}
	if feature.Mode == "enabled" && (feature.RequestsPerSecond < 1 || feature.RequestsPerSecond > 10000 || feature.Burst < 1 || feature.Burst > 10000) {
		return false
	}
	for _, raw := range feature.TrustedProxies {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || prefix != prefix.Masked() {
			return false
		}
	}
	return true
}

func resolveHealthCheckFeature(defaults, overrides healthCheckFeature) (healthCheckFeature, error) {
	mode, err := resolveFeatureMode(defaults.Mode, overrides.Mode)
	if err != nil {
		return healthCheckFeature{}, err
	}
	result := defaults
	result.Mode = mode
	if overrides.Path != "" {
		result.Path = overrides.Path
	}
	if overrides.IntervalSeconds != 0 {
		result.IntervalSeconds = overrides.IntervalSeconds
	}
	if overrides.TimeoutSeconds != 0 {
		result.TimeoutSeconds = overrides.TimeoutSeconds
	}
	if overrides.ExpectedStatus != 0 {
		result.ExpectedStatus = overrides.ExpectedStatus
	}
	if !validHealthCheckFeature(result, false) || !validHealthCheckFeature(overrides, true) {
		return healthCheckFeature{}, errUnsupportedSettings
	}
	return result, nil
}

func validHealthCheckFeature(feature healthCheckFeature, allowInherit bool) bool {
	if feature.Mode != "enabled" && feature.Mode != "disabled" && (!allowInherit || feature.Mode != "" && feature.Mode != "inherit") {
		return false
	}
	if feature.Path != "" && !validConfiguredPath(feature.Path) {
		return false
	}
	if feature.IntervalSeconds != 0 && (feature.IntervalSeconds < 1 || feature.IntervalSeconds > 600) {
		return false
	}
	if feature.TimeoutSeconds != 0 && (feature.TimeoutSeconds < 1 || feature.TimeoutSeconds > 60) {
		return false
	}
	if feature.ExpectedStatus != 0 && (feature.ExpectedStatus < 200 || feature.ExpectedStatus > 599) {
		return false
	}
	if feature.Mode == "enabled" && (feature.Path == "" || feature.IntervalSeconds < 1 || feature.TimeoutSeconds < 1 || feature.ExpectedStatus < 200) {
		return false
	}
	return true
}

func resolveRewriteFeature(defaults, overrides rewriteFeature) (rewriteFeature, error) {
	mode, err := resolveFeatureMode(defaults.Mode, overrides.Mode)
	if err != nil {
		return rewriteFeature{}, err
	}
	result := defaults
	result.Mode = mode
	if overrides.Path != "" {
		result.Path = overrides.Path
	}
	if !validRewriteFeature(result, false) || !validRewriteFeature(overrides, true) {
		return rewriteFeature{}, errUnsupportedSettings
	}
	return result, nil
}

func validRewriteFeature(feature rewriteFeature, allowInherit bool) bool {
	if feature.Mode != "enabled" && feature.Mode != "disabled" && (!allowInherit || feature.Mode != "" && feature.Mode != "inherit") {
		return false
	}
	if feature.Path != "" && !validConfiguredPath(feature.Path) {
		return false
	}
	return feature.Mode != "enabled" || feature.Path != ""
}

func validConfiguredPath(value string) bool {
	if len(value) == 0 || len(value) > 2048 || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.ContainsAny(value, "\\\r\n\x00?#") {
		return false
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || !strings.HasPrefix(parsed.Path, "/") {
		return false
	}
	decoded, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return false
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return path.Clean(parsed.Path) == parsed.Path
}

func validFeatureHeaderName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune("!#$%&'*+-.^_`|~0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", char) {
			return false
		}
	}
	return true
}

var secretReferencePattern = regexp.MustCompile(`^ssr:v1:[a-f0-9]{32}$`)

func validCompressionAlgorithms(algorithms []string) bool {
	if len(algorithms) == 0 || len(algorithms) > 2 {
		return false
	}
	seen := make(map[string]struct{}, len(algorithms))
	for _, algorithm := range algorithms {
		if algorithm != "gzip" && algorithm != "zstd" {
			return false
		}
		if _, exists := seen[algorithm]; exists {
			return false
		}
		seen[algorithm] = struct{}{}
	}
	return true
}

func validCacheFeature(feature cacheFeature) bool {
	return (feature.Mode == "enabled" || feature.Mode == "disabled") && feature.MaxAgeSeconds != nil &&
		(feature.Visibility == "public" || feature.Visibility == "private") &&
		*feature.MaxAgeSeconds >= 0 && *feature.MaxAgeSeconds <= 31536000
}

func validHeaderFeature(feature headersFeature) bool {
	return (feature.Mode == "" || feature.Mode == "inherit" || feature.Mode == "enabled" || feature.Mode == "disabled") &&
		validHeaderMap(feature.Request) && validHeaderMap(feature.Response)
}

func validHeaderMap(headers map[string]string) bool {
	if len(headers) > 64 {
		return false
	}
	seen := make(map[string]struct{}, len(headers))
	for name, value := range headers {
		if len(name) == 0 || len(name) > 128 || !httpTokenRE.MatchString(name) || len(value) > 8192 || !utf8.ValidString(value) {
			return false
		}
		folded := strings.ToLower(name)
		if _, exists := seen[folded]; exists {
			return false
		}
		seen[folded] = struct{}{}
		for _, character := range value {
			if (character < 0x20 && character != '\t') || character == 0x7f {
				return false
			}
		}
	}
	return true
}

func validCORSFeature(feature corsFeature, allowInherit bool) bool {
	modeValid := feature.Mode == "enabled" || feature.Mode == "disabled" || allowInherit && (feature.Mode == "" || feature.Mode == "inherit")
	if !modeValid || len(feature.Origins) > 32 || len(feature.Methods) > 32 || len(feature.AllowedHeaders) > 64 || len(feature.ExposedHeaders) > 64 {
		return false
	}
	seenOrigins := make(map[string]struct{}, len(feature.Origins))
	for _, origin := range feature.Origins {
		if !validCORSOrigin(origin) {
			return false
		}
		if _, exists := seenOrigins[origin]; exists {
			return false
		}
		seenOrigins[origin] = struct{}{}
	}
	if _, wildcard := seenOrigins["*"]; wildcard && len(seenOrigins) != 1 {
		return false
	}
	for _, method := range feature.Methods {
		if !validHTTPMethod(method) {
			return false
		}
	}
	if !uniqueStrings(feature.Methods) || !validCORSHeaderNames(feature.AllowedHeaders) || !validCORSHeaderNames(feature.ExposedHeaders) {
		return false
	}
	if feature.AllowCredentials != nil && *feature.AllowCredentials {
		if _, wildcard := seenOrigins["*"]; wildcard {
			return false
		}
	}
	maxAge := 0
	if feature.MaxAgeSeconds != nil {
		maxAge = *feature.MaxAgeSeconds
	}
	if maxAge < 0 || maxAge > 86400 {
		return false
	}
	if feature.Mode == "enabled" && (len(feature.Origins) == 0 || len(feature.Methods) == 0) {
		return false
	}
	return true
}

func validCORSOrigin(value string) bool {
	if value == "*" {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	return value == parsed.Scheme+"://"+parsed.Host
}

func validCORSHeaderNames(values []string) bool {
	if !uniqueFoldedStrings(values) {
		return false
	}
	for _, value := range values {
		if len(value) == 0 || len(value) > 128 || !httpTokenRE.MatchString(value) {
			return false
		}
	}
	return true
}

func uniqueStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func uniqueFoldedStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}

func cloneHeaderMap(headers map[string]string) map[string]string {
	copy := make(map[string]string, len(headers))
	for key, value := range headers {
		copy[key] = value
	}
	return copy
}

func mergeHeaderMaps(defaults, overrides map[string]string) map[string]string {
	merged := cloneHeaderMap(defaults)
	if merged == nil {
		merged = make(map[string]string, len(overrides))
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

func featureIntPointer(value int) *int { return &value }

func validateSocketAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil || portText == "" || (len(portText) > 1 && portText[0] == '0') {
		return errUnsupportedSettings
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errUnsupportedSettings
	}
	if host == "" {
		return nil
	}
	addressValue, err := netip.ParseAddr(host)
	if err != nil || addressValue.Zone() != "" {
		return errUnsupportedSettings
	}
	return nil
}

func compileHTTPProtocols(configured []string, tlsMode string) ([]string, []string, error) {
	if len(configured) == 0 {
		return nil, nil, errUnsupportedSettings
	}
	protocolMap := map[string]string{"http1": "h1", "http2": "h2", "http3": "h3"}
	alpnMap := map[string]string{"http1": "http/1.1", "http2": "h2", "http3": "h3"}
	protocols := make([]string, 0, len(configured))
	alpn := make([]string, 0, len(configured))
	seen := make(map[string]struct{}, len(configured))
	for _, protocol := range configured {
		mapped, exists := protocolMap[protocol]
		if !exists {
			return nil, nil, errUnsupportedSettings
		}
		if _, duplicate := seen[protocol]; duplicate {
			return nil, nil, errUnsupportedSettings
		}
		seen[protocol] = struct{}{}
		if protocol == "http3" && tlsMode == "disabled" {
			return nil, nil, errUnsupportedSettings
		}
		protocols = append(protocols, mapped)
		alpn = append(alpn, alpnMap[protocol])
	}
	if _, h2 := seen["http2"]; h2 {
		if _, h1 := seen["http1"]; !h1 {
			return nil, nil, errUnsupportedSettings
		}
	}
	return protocols, alpn, nil
}

func compileHandler(routeID string, handler settingsHandler, api *apiContract, usedTargets map[string]map[string]struct{}, upstreamPools *[]upstreamPoolConfig, secrets map[string][]byte, usedSecretReferences map[string]struct{}) (map[string]any, error) {
	switch handler.Type {
	case "redirect":
		if !validRedirectTarget(handler.Target) ||
			(handler.StatusCode != 301 && handler.StatusCode != 302 && handler.StatusCode != 307 && handler.StatusCode != 308) {
			return nil, errUnsupportedSettings
		}
		return map[string]any{"handler": redirectHandlerModule, "target": handler.Target, "statusCode": handler.StatusCode}, nil
	case "static":
		if _, err := SiteRoot(handler.SiteID); err != nil || !validReleaseID(handler.ReleaseID) || !validStaticMountPrefix(handler.MountPrefix) {
			return nil, errUnsupportedSettings
		}
		return map[string]any{"handler": siteFileServerModule, "siteId": handler.SiteID, "releaseId": handler.ReleaseID, "mountPrefix": handler.MountPrefix}, nil
	case "reverseProxy":
		if routeID == "" || len(handler.Upstreams) == 0 || len(handler.Upstreams) > 32 || upstreamPools == nil {
			return nil, errUnsupportedSettings
		}
		pool := upstreamPoolConfig{ID: routeID, Origins: make([]upstreamOriginConfig, 0, len(handler.Upstreams))}
		caddyUpstreams := make([]any, 0, len(handler.Upstreams))
		for _, configuredOrigin := range handler.Upstreams {
			origin, err := parseUpstreamOrigin(configuredOrigin.Origin, configuredOrigin.Weight)
			if err != nil {
				return nil, errUnsupportedSettings
			}
			if configuredOrigin.CARef != "" {
				if origin.Scheme != "https" {
					return nil, errUnsupportedSettings
				}
				bundle, exists := secrets[configuredOrigin.CARef]
				if !exists || !validCertificateBundle(bundle) {
					return nil, errUnsupportedSettings
				}
				origin.CABundle = append([]byte(nil), bundle...)
				usedSecretReferences[configuredOrigin.CARef] = struct{}{}
			}
			pool.Origins = append(pool.Origins, origin)
			caddyUpstreams = append(caddyUpstreams, map[string]any{"dial": origin.Address})
		}
		*upstreamPools = append(*upstreamPools, pool)
		return map[string]any{
			"handler":   "reverse_proxy",
			"upstreams": caddyUpstreams,
			"load_balancing": map[string]any{
				"selection_policy": map[string]any{"policy": "liapoldus_smooth_weighted", "pool": routeID},
			},
			"transport": map[string]any{"protocol": "liapoldus_upstream_pool", "pool": routeID},
		}, nil
	case "service":
		if (handler.Mode != "call" && handler.Mode != "http_stream" && handler.Mode != "websocket" && handler.Mode != "sse") || handler.InstanceID == "" || handler.Capability == "" || api == nil {
			return nil, errUnsupportedSettings
		}
		contract, err := contracts.LoadHTTPDispatch()
		if err != nil {
			return nil, err
		}
		if usedTargets[handler.InstanceID] == nil {
			usedTargets[handler.InstanceID] = make(map[string]struct{})
		}
		usedTargets[handler.InstanceID][handler.Capability] = struct{}{}
		module := contract.Module
		if index := strings.LastIndexByte(module, '.'); index >= 0 {
			module = module[index+1:]
		}
		moduleConfig := map[string]any{"handler": module, "instance": handler.InstanceID, "capability": handler.Capability, "mode": handler.Mode, "api": api}
		if len(handler.RequestCookieNames) > 0 {
			moduleConfig["requestCookieNames"] = append([]string(nil), handler.RequestCookieNames...)
		}
		return moduleConfig, nil
	default:
		return nil, fmt.Errorf("%w: handler", errUnsupportedSettings)
	}
}

func uniqueUpstreamDialAddresses(upstreams []upstream) bool {
	seen := make(map[string]struct{}, len(upstreams))
	for _, configured := range upstreams {
		origin, err := parseUpstreamOrigin(configured.Origin, configured.Weight)
		if err != nil {
			return false
		}
		if _, exists := seen[origin.Address]; exists {
			return false
		}
		seen[origin.Address] = struct{}{}
	}
	return true
}

func validReleaseID(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range strings.TrimPrefix(value, "sha256:") {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func validStaticMountPrefix(value string) bool {
	if value == "/" {
		return true
	}
	if len(value) < 2 || len(value) > 2048 || value[0] != '/' || strings.ContainsAny(value, `%?#\\`) || strings.Contains(value, "//") {
		return false
	}
	for _, segment := range strings.Split(strings.Trim(value, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
		for _, character := range segment {
			if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && !strings.ContainsRune("._~-", character) {
				return false
			}
		}
	}
	return true
}

func SiteRoot(siteID string) (string, error) {
	if siteID == "" || len(siteID) > 128 || !utf8.ValidString(siteID) || siteID == "." || siteID == ".." || strings.ContainsAny(siteID, `/\\`) {
		return "", errUnsupportedSettings
	}
	for _, character := range siteID {
		if character < 0x20 || character == 0x7f {
			return "", errUnsupportedSettings
		}
	}
	root := filepath.Join(SiteDataRoot(), "sites", siteID, "active", "current")
	relative, err := filepath.Rel(filepath.Join(SiteDataRoot(), "sites"), root)
	if err != nil || !filepath.IsLocal(relative) {
		return "", errUnsupportedSettings
	}
	return root, nil
}

func SiteDataRoot() string {
	return filepath.Join(caddycore.AppDataDir(), "liapoldus")
}

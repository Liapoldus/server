package caddy

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
	caddycore "github.com/caddyserver/caddy/v2"
	"liapoldus.local/server-plugin/contracts"
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
	Listeners []settingsListener `json:"listeners"`
	Routes    []settingsRoute    `json:"routes"`
}

type settingsListener struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Address   string   `json:"address"`
	Hostnames []string `json:"hostnames"`
	Protocols []string `json:"protocols"`
	TLS       struct {
		Mode           string `json:"mode"`
		CertificateRef string `json:"certificateRef"`
		PrivateKeyRef  string `json:"privateKeyRef"`
	} `json:"tls"`
	RedirectToListenerID string `json:"redirectToListenerId"`
}

type settingsRoute struct {
	ID         string              `json:"id"`
	ListenerID string              `json:"listenerId"`
	Match      *settingsRouteMatch `json:"match"`
	Handler    settingsHandler     `json:"handler"`
}

type settingsHandler struct {
	Type               string   `json:"type"`
	SiteID             string   `json:"siteId"`
	InstanceID         string   `json:"instanceId"`
	Capability         string   `json:"capability"`
	Mode               string   `json:"mode"`
	RequestCookieNames []string `json:"requestCookieNames,omitempty"`
	StreamLimits       struct {
		MaxConcurrency    int `json:"maxConcurrency"`
		IdleTimeoutMillis int `json:"idleTimeoutMillis"`
		MaxDurationMillis int `json:"maxDurationMillis"`
	} `json:"streamLimits,omitempty"`
	Upstreams []upstream `json:"upstreams"`
}

type upstream struct {
	Origin string `json:"origin"`
	Weight int    `json:"weight"`
	CARef  string `json:"caRef"`
}

type DispatchTarget struct {
	ID            string
	Endpoint      string
	TimeoutMillis int
	Security      peer.SecurityConfig
}

func compileSettings(contents []byte, targets []DispatchTarget, targetSetID uint64, secrets map[string][]byte) ([]byte, error) {
	var desired settingsConfig
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&desired); err != nil {
		return nil, errUnsupportedSettings
	}
	targetsByID := make(map[string]DispatchTarget, len(targets))
	for _, target := range targets {
		if target.ID == "" || target.Endpoint == "" {
			return nil, errUnsupportedSettings
		}
		if _, exists := targetsByID[target.ID]; exists {
			return nil, errUnsupportedSettings
		}
		targetsByID[target.ID] = target
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
		if listener.TLS.Mode == "disabled" {
			if len(listener.Hostnames) != 0 || len(listener.Protocols) != 1 || protocols[0] != "h1" || (listener.RedirectToListenerID != "" && len(listener.Hostnames) != 0) {
				return nil, errUnsupportedSettings
			}
		} else if listener.TLS.Mode == "custom" {
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
		} else if listener.TLS.Mode == "automatic" {
			if len(listener.Hostnames) == 0 {
				return nil, errUnsupportedSettings
			}
		} else {
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
		matcher, identity, err := canonicalMatcher(route.Match)
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
		handler, err := compileHandler(route.ID, route.Handler, targetsByID, usedTargets, &upstreamPools, secrets, usedSecretReferences)
		if err != nil {
			return nil, err
		}
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
			"handle":   []any{handler},
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
		for _, target := range targets {
			if _, used := usedTargets[target.ID]; !used {
				continue
			}
			instance := map[string]any{"id": target.ID, "endpoint": target.Endpoint, "methods": sortedMethods(usedTargets[target.ID])}
			if target.TimeoutMillis > 0 {
				instance["timeoutMillis"] = target.TimeoutMillis
			}
			instances = append(instances, instance)
		}
		apps[contract.App] = map[string]any{"instances": instances, "targetSetId": targetSetID}
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

func compileHandler(routeID string, handler settingsHandler, targets map[string]DispatchTarget, usedTargets map[string]map[string]struct{}, upstreamPools *[]upstreamPoolConfig, secrets map[string][]byte, usedSecretReferences map[string]struct{}) (map[string]any, error) {
	switch handler.Type {
	case "static":
		if _, err := SiteRoot(handler.SiteID); err != nil {
			return nil, errUnsupportedSettings
		}
		return map[string]any{"handler": siteFileServerModule, "siteId": handler.SiteID}, nil
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
	case "plugin":
		if (handler.Mode != "call" && handler.Mode != "http_stream" && handler.Mode != "websocket" && handler.Mode != "sse") || handler.InstanceID == "" || handler.Capability == "" {
			return nil, errUnsupportedSettings
		}
		if _, exists := targets[handler.InstanceID]; !exists {
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
		moduleConfig := map[string]any{"handler": module, "instance": handler.InstanceID, "capability": handler.Capability, "mode": handler.Mode}
		if len(handler.RequestCookieNames) > 0 {
			moduleConfig["requestCookieNames"] = append([]string(nil), handler.RequestCookieNames...)
		}
		return moduleConfig, nil
	default:
		return nil, fmt.Errorf("%w: handler", errUnsupportedSettings)
	}
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

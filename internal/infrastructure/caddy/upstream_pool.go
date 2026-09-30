package caddy

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	caddycore "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"golang.org/x/net/idna"
)

const (
	upstreamPoolsAppID     = "liapoldus.upstream_pools"
	weightedPolicyModuleID = "http.reverse_proxy.selection_policies.liapoldus_smooth_weighted"
	poolTransportModuleID  = "http.reverse_proxy.transport.liapoldus_upstream_pool"
	upstreamRetryCooldown  = 30 * time.Second
)

type upstreamPoolAppConfig struct {
	Pools []upstreamPoolConfig `json:"pools"`
}

type upstreamPoolConfig struct {
	ID      string                 `json:"id"`
	Origins []upstreamOriginConfig `json:"origins"`
}

type upstreamOriginConfig struct {
	Address  string `json:"address"`
	Scheme   string `json:"scheme"`
	Weight   int    `json:"weight"`
	CABundle []byte `json:"caBundle,omitempty"`
}

type upstreamPoolApp struct {
	Pools []upstreamPoolConfig `json:"pools,omitempty"`
	pools map[string]*upstreamPoolState
}

type upstreamPoolState struct {
	mu            sync.Mutex
	config        upstreamPoolConfig
	currentWeight []int
	unhealthyTill []time.Time
	lastEligible  []bool
	selectedPool  reverseproxy.UpstreamPool
	originIndexes map[*reverseproxy.Upstream]int
}

type smoothWeightedSelection struct {
	Pool  string `json:"pool"`
	state *upstreamPoolState
}

type upstreamPoolTransport struct {
	Pool       string `json:"pool"`
	state      *upstreamPoolState
	transports []*reverseproxy.HTTPTransport
}

func init() {
	caddycore.RegisterModule(upstreamPoolApp{})
	caddycore.RegisterModule(smoothWeightedSelection{})
	caddycore.RegisterModule((*upstreamPoolTransport)(nil))
}

func (upstreamPoolApp) CaddyModule() caddycore.ModuleInfo {
	return caddycore.ModuleInfo{ID: caddycore.ModuleID(upstreamPoolsAppID), New: func() caddycore.Module { return new(upstreamPoolApp) }}
}

func (app *upstreamPoolApp) Provision(caddycore.Context) error {
	if app == nil {
		return errUnsupportedSettings
	}
	app.pools = make(map[string]*upstreamPoolState, len(app.Pools))
	for _, pool := range app.Pools {
		if pool.ID == "" || len(pool.Origins) < 1 || len(pool.Origins) > 32 {
			return errUnsupportedSettings
		}
		if _, duplicate := app.pools[pool.ID]; duplicate {
			return errUnsupportedSettings
		}
		weights := make([]int, len(pool.Origins))
		for index, origin := range pool.Origins {
			if origin.Address == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Weight < 1 || origin.Weight > 100 {
				return errUnsupportedSettings
			}
			weights[index] = origin.Weight
			if len(origin.CABundle) != 0 && (origin.Scheme != "https" || !validCertificateBundle(origin.CABundle)) {
				return errUnsupportedSettings
			}
		}
		app.pools[pool.ID] = &upstreamPoolState{
			config: pool, currentWeight: make([]int, len(weights)), unhealthyTill: make([]time.Time, len(weights)),
			lastEligible: make([]bool, len(weights)), originIndexes: make(map[*reverseproxy.Upstream]int, len(weights)),
		}
	}
	return nil
}

func (*upstreamPoolApp) Start() error { return nil }
func (*upstreamPoolApp) Stop() error  { return nil }

func (smoothWeightedSelection) CaddyModule() caddycore.ModuleInfo {
	return caddycore.ModuleInfo{ID: caddycore.ModuleID(weightedPolicyModuleID), New: func() caddycore.Module { return new(smoothWeightedSelection) }}
}

func (selection *smoothWeightedSelection) Provision(ctx caddycore.Context) error {
	state, err := upstreamPoolStateFromContext(ctx, selection.Pool)
	if err != nil {
		return err
	}
	selection.state = state
	return nil
}

func (selection *smoothWeightedSelection) Select(pool reverseproxy.UpstreamPool, _ *http.Request, _ http.ResponseWriter) *reverseproxy.Upstream {
	if selection == nil || selection.state == nil {
		return nil
	}
	upstream, _ := selection.state.selectUpstream(pool)
	return upstream
}

func (transport *upstreamPoolTransport) CaddyModule() caddycore.ModuleInfo {
	return caddycore.ModuleInfo{ID: caddycore.ModuleID(poolTransportModuleID), New: func() caddycore.Module { return new(upstreamPoolTransport) }}
}

func (transport *upstreamPoolTransport) Provision(ctx caddycore.Context) error {
	if transport == nil {
		return errUnsupportedSettings
	}
	state, err := upstreamPoolStateFromContext(ctx, transport.Pool)
	if err != nil {
		return err
	}
	transport.state = state
	transport.transports = make([]*reverseproxy.HTTPTransport, len(state.config.Origins))
	for index, origin := range state.config.Origins {
		settings := &reverseproxy.HTTPTransport{}
		if origin.Scheme == "https" {
			settings.TLS = &reverseproxy.TLSConfig{}
		}
		if err := settings.Provision(ctx); err != nil {
			closePoolTransports(transport.transports)
			return err
		}
		if origin.Scheme == "https" && len(origin.CABundle) != 0 {
			if err := extendSystemRoots(settings, origin.CABundle); err != nil {
				closePoolTransports(transport.transports)
				return err
			}
		}
		if settings.Transport != nil {
			// Reuse can trigger net/http's own replay of idempotent requests; the
			// product contract permits retries only in this module before any bytes.
			settings.Transport.DisableKeepAlives = true
			settings.Transport.Proxy = nil
			dialer := &net.Dialer{Timeout: time.Duration(settings.DialTimeout), FallbackDelay: time.Duration(settings.FallbackDelay)}
			settings.Transport.DialContext = dialer.DialContext
		}
		transport.transports[index] = settings
	}
	return nil
}

func (transport *upstreamPoolTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport == nil || transport.state == nil || request == nil || request.URL == nil {
		return nil, errUnsupportedSettings
	}
	dialInfo, hasDialInfo := reverseproxy.GetDialInfo(request.Context())
	if !hasDialInfo || dialInfo.Upstream == nil {
		return nil, errUnsupportedSettings
	}
	originIndex := transport.state.originIndex(dialInfo.Upstream)
	if originIndex < 0 {
		return nil, errUnsupportedSettings
	}
	response, err := transport.roundTripTo(request, originIndex)
	if err == nil || !eligibleConnectFailure(err) {
		return response, err
	}
	transport.state.markUnavailable(originIndex)
	pool := transport.state.currentPool()
	if len(pool) != len(transport.state.config.Origins) {
		return nil, err
	}
	retryUpstream, retryIndex := transport.state.selectUpstream(pool)
	if retryUpstream == nil || retryIndex == originIndex || retryIndex < 0 || retryIndex >= len(transport.state.config.Origins) {
		return nil, err
	}
	return transport.roundTripTo(request, retryIndex)
}

func (transport *upstreamPoolTransport) roundTripTo(request *http.Request, originIndex int) (*http.Response, error) {
	if originIndex < 0 || originIndex >= len(transport.state.config.Origins) || originIndex >= len(transport.transports) {
		return nil, errUnsupportedSettings
	}
	origin := transport.state.config.Origins[originIndex]
	inner := transport.transports[originIndex]
	if inner == nil || inner.Transport == nil {
		return nil, errUnsupportedSettings
	}
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	urlCopy.Scheme = origin.Scheme
	urlCopy.Host = origin.Address
	clone.URL = &urlCopy
	if origin.Scheme == "https" {
		clone.Host = origin.Address
	}
	return inner.RoundTrip(clone)
}

func (transport *upstreamPoolTransport) Cleanup() error {
	closePoolTransports(transport.transports)
	return nil
}

func (state *upstreamPoolState) selectUpstream(pool reverseproxy.UpstreamPool) (*reverseproxy.Upstream, int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(pool) != len(state.config.Origins) {
		return nil, -1
	}
	state.selectedPool = append(state.selectedPool[:0], pool...)
	now := time.Now()
	eligible := make([]bool, len(pool))
	totalWeight := 0
	eligibleCount := 0
	for index, upstream := range pool {
		if upstream == nil || upstream.Dial != state.config.Origins[index].Address {
			continue
		}
		state.originIndexes[upstream] = index
		if !upstream.Available() {
			continue
		}
		if now.Before(state.unhealthyTill[index]) {
			continue
		}
		state.unhealthyTill[index] = time.Time{}
		eligible[index] = true
		eligibleCount++
		totalWeight += state.config.Origins[index].Weight
	}
	if !sameEligibility(state.lastEligible, eligible) {
		clear(state.currentWeight)
		copy(state.lastEligible, eligible)
	}
	if eligibleCount == 0 || totalWeight == 0 {
		return nil, -1
	}
	selected := -1
	for index, available := range eligible {
		if !available {
			continue
		}
		state.currentWeight[index] += state.config.Origins[index].Weight
		if selected < 0 || state.currentWeight[index] > state.currentWeight[selected] {
			selected = index
		}
	}
	state.currentWeight[selected] -= totalWeight
	return pool[selected], selected
}

func (state *upstreamPoolState) originIndex(upstream *reverseproxy.Upstream) int {
	state.mu.Lock()
	defer state.mu.Unlock()
	if index, exists := state.originIndexes[upstream]; exists {
		return index
	}
	return -1
}

func (state *upstreamPoolState) markUnavailable(index int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if index >= 0 && index < len(state.unhealthyTill) {
		state.unhealthyTill[index] = time.Now().Add(upstreamRetryCooldown)
	}
}

func (state *upstreamPoolState) currentPool() reverseproxy.UpstreamPool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return append(reverseproxy.UpstreamPool(nil), state.selectedPool...)
}

func upstreamPoolStateFromContext(ctx caddycore.Context, poolID string) (*upstreamPoolState, error) {
	if poolID == "" {
		return nil, errUnsupportedSettings
	}
	value, err := ctx.App(upstreamPoolsAppID)
	if err != nil {
		return nil, errUnsupportedSettings
	}
	app, ok := value.(*upstreamPoolApp)
	if !ok || app == nil {
		return nil, errUnsupportedSettings
	}
	state := app.pools[poolID]
	if state == nil {
		return nil, errUnsupportedSettings
	}
	return state, nil
}

func eligibleConnectFailure(err error) bool {
	var operationError *net.OpError
	if !errors.As(err, &operationError) || operationError.Op != "dial" {
		return false
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}

func sameEligibility(previous, current []bool) bool {
	if len(previous) != len(current) {
		return false
	}
	for index := range previous {
		if previous[index] != current[index] {
			return false
		}
	}
	return true
}

func extendSystemRoots(transport *reverseproxy.HTTPTransport, bundle []byte) error {
	if transport == nil || transport.Transport == nil || transport.Transport.TLSClientConfig == nil || !validCertificateBundle(bundle) {
		return errUnsupportedSettings
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		return errUnsupportedSettings
	}
	if !roots.AppendCertsFromPEM(bundle) {
		return errUnsupportedSettings
	}
	transport.Transport.TLSClientConfig = transport.Transport.TLSClientConfig.Clone()
	transport.Transport.TLSClientConfig.RootCAs = roots
	return nil
}

func validCertificateBundle(contents []byte) bool {
	remaining := contents
	found := false
	for len(remaining) != 0 {
		remaining = bytes.TrimSpace(remaining)
		if len(remaining) == 0 {
			break
		}
		if !bytes.HasPrefix(remaining, []byte("-----BEGIN CERTIFICATE-----")) {
			return false
		}
		block, rest := pem.Decode(remaining)
		if block == nil || block.Type != "CERTIFICATE" {
			return false
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return false
		}
		found = true
		remaining = rest
	}
	return found
}

func closePoolTransports(transports []*reverseproxy.HTTPTransport) {
	for _, transport := range transports {
		if transport == nil || transport.Transport == nil {
			continue
		}
		transport.Transport.CloseIdleConnections()
	}
}

var _ caddycore.App = (*upstreamPoolApp)(nil)
var _ reverseproxy.Selector = (*smoothWeightedSelection)(nil)
var _ http.RoundTripper = (*upstreamPoolTransport)(nil)
var _ reverseproxy.TLSTransport = (*upstreamPoolTransport)(nil)

func (transport *upstreamPoolTransport) TLSEnabled() bool { return true }

func (transport *upstreamPoolTransport) EnableTLS(*reverseproxy.TLSConfig) error { return nil }

func parseUpstreamOrigin(value string, weight int) (upstreamOriginConfig, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return upstreamOriginConfig{}, errUnsupportedSettings
	}
	if weight == 0 {
		weight = 1
	}
	if weight < 1 || weight > 100 {
		return upstreamOriginConfig{}, errUnsupportedSettings
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	} else {
		numericPort, parseErr := strconv.ParseUint(port, 10, 16)
		if parseErr != nil || numericPort == 0 {
			return upstreamOriginConfig{}, errUnsupportedSettings
		}
		port = strconv.FormatUint(numericPort, 10)
	}
	if parsed.Hostname() == "" {
		return upstreamOriginConfig{}, errUnsupportedSettings
	}
	host := parsed.Hostname()
	if address := net.ParseIP(host); address == nil {
		host, err = idna.Lookup.ToASCII(host)
		if err != nil || host == "" {
			return upstreamOriginConfig{}, errUnsupportedSettings
		}
		host = strings.ToLower(host)
	}
	return upstreamOriginConfig{Address: net.JoinHostPort(host, port), Scheme: parsed.Scheme, Weight: weight}, nil
}

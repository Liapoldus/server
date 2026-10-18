package caddy

import (
	"testing"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
)

func TestOriginIndexResolvesActiveHealthProbeBeforeSelection(t *testing.T) {
	state := &upstreamPoolState{
		config:        upstreamPoolConfig{Origins: []upstreamOriginConfig{{Address: "127.0.0.1:8080", Scheme: "http", Weight: 1}}},
		originIndexes: make(map[*reverseproxy.Upstream]int),
	}
	probe := &reverseproxy.Upstream{Dial: "127.0.0.1:8080"}
	if index := state.originIndex(probe); index != 0 {
		t.Fatalf("initial active health probe should resolve its configured transport, got index %d", index)
	}
	if cached := state.originIndexes[probe]; cached != 0 {
		t.Fatalf("resolved upstream pointer should be cached, got index %d", cached)
	}
}

func TestOriginIndexRejectsAmbiguousProbeDialAddress(t *testing.T) {
	state := &upstreamPoolState{
		config: upstreamPoolConfig{Origins: []upstreamOriginConfig{
			{Address: "127.0.0.1:8080", Scheme: "http", Weight: 1},
			{Address: "127.0.0.1:8080", Scheme: "http", Weight: 2},
		}},
		originIndexes: make(map[*reverseproxy.Upstream]int),
	}
	if index := state.originIndex(&reverseproxy.Upstream{Dial: "127.0.0.1:8080"}); index != -1 {
		t.Fatalf("ambiguous active health probe dial must be rejected, got index %d", index)
	}
}

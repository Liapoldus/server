package caddy

import (
	"sync"

	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
)

// Caddy configuration contains only opaque target IDs. Private keys and trust
// roots stay in this process-local registry, scoped to the Runtime that prepared
// the immutable snapshot; Caddy's JSON config and autosave never carry them.
var dispatchSecurity = struct {
	sets map[uint64]map[string]DispatchTarget
	sync.RWMutex
}{sets: make(map[uint64]map[string]DispatchTarget)}

func registerDispatchSecurity(setID uint64, targets []DispatchTarget) {
	copyTargets := make(map[string]DispatchTarget, len(targets))
	for _, target := range targets {
		copyTargets[target.ID] = target
	}
	dispatchSecurity.Lock()
	dispatchSecurity.sets[setID] = copyTargets
	dispatchSecurity.Unlock()
}

func lookupDispatchSecurity(setID uint64, id, endpoint string) (peer.SecurityConfig, bool) {
	dispatchSecurity.RLock()
	target, ok := dispatchSecurity.sets[setID][id]
	dispatchSecurity.RUnlock()
	if !ok || target.Endpoint != endpoint {
		return peer.SecurityConfig{}, false
	}
	return target.Security, true
}

func unregisterDispatchSecurity(setID uint64) {
	dispatchSecurity.Lock()
	delete(dispatchSecurity.sets, setID)
	dispatchSecurity.Unlock()
}

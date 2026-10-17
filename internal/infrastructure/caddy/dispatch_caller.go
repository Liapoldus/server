package caddy

import (
	"context"
	"io"
	"sync"

	"github.com/Liapoldus/wool/v3/presentation/peer"
)

// DispatchCaller resolves every invocation through the service-owned Core peer
// directory. The Server runtime receives no peer endpoint or credential data.
type DispatchCaller interface {
	Call(context.Context, string, string, []byte) ([]byte, error)
	OpenStream(context.Context, string, string) (peer.Stream, io.Closer, error)
}

var dispatchCallers = struct {
	sets map[uint64]DispatchCaller
	sync.RWMutex
}{sets: make(map[uint64]DispatchCaller)}

func registerDispatchCaller(setID uint64, caller DispatchCaller) {
	dispatchCallers.Lock()
	dispatchCallers.sets[setID] = caller
	dispatchCallers.Unlock()
}

func lookupDispatchCaller(setID uint64) (DispatchCaller, bool) {
	dispatchCallers.RLock()
	caller, ok := dispatchCallers.sets[setID]
	dispatchCallers.RUnlock()
	return caller, ok
}

func unregisterDispatchCaller(setID uint64) {
	dispatchCallers.Lock()
	delete(dispatchCallers.sets, setID)
	dispatchCallers.Unlock()
}

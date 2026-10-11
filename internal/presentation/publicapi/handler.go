package publicapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type Caller interface {
	Call(context.Context, string, string, []byte) ([]byte, error)
}

// Handler exposes only a generic operator-facing HTTP boundary. Target and
// method are opaque contract data resolved by Core's peer directory inside the
// Server process.
func Handler(caller Caller, observability http.Handler) http.Handler {
	mux := http.NewServeMux()
	if observability == nil {
		observability = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			write(w, http.StatusServiceUnavailable, map[string]any{"error": "sdk_observability_unavailable"})
		})
	}
	// The SDK handler owns its paths. Mounting it at the public fallback keeps
	// healthz/metrics out of the product router and prevents path duplication.
	mux.Handle("/", observability)
	mux.HandleFunc("POST /api/peer", func(w http.ResponseWriter, r *http.Request) {
		if caller == nil {
			write(w, http.StatusServiceUnavailable, map[string]any{"error": "peer_directory_unavailable"})
			return
		}
		target := strings.TrimSpace(r.URL.Query().Get("target"))
		method := strings.TrimSpace(r.URL.Query().Get("method"))
		if target == "" || method == "" {
			write(w, http.StatusBadRequest, map[string]any{"error": "target_and_method_required"})
			return
		}
		payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil || !json.Valid(payload) {
			write(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
			return
		}
		result, err := caller.Call(r.Context(), target, method, payload)
		if err != nil {
			write(w, http.StatusBadGateway, map[string]any{"error": "peer_call_failed"})
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(result)
	})
	return mux
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

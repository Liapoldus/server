package contracts

import (
	"encoding/json"
	"testing"
)

func TestResolveAdminQueryAllowsUnfilteredReleaseTable(t *testing.T) {
	query, err := ResolveAdminQuery("sites", []byte(`{"resource":"releases","limit":50}`))
	if err != nil {
		t.Fatalf("unfiltered release query was rejected: %v", err)
	}
	if query.Resource != "releases" || query.Capability != "server.sites.releases.list" {
		t.Fatalf("resolved release query = %+v", query)
	}
	var payload map[string]any
	if err := json.Unmarshal(query.Payload, &payload); err != nil || payload["limit"] != float64(50) {
		t.Fatalf("release query payload = %s err=%v", query.Payload, err)
	}
}

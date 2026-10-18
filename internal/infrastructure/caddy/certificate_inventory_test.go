package caddy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestEmptyCertificateInventorySerializesItemsAsArray(t *testing.T) {
	page, err := (&Runtime{}).ListCertificates(context.Background(), "", 50, "")
	if err != nil || page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("empty certificate page = %+v err=%v", page, err)
	}
	encoded, err := json.Marshal(page)
	if err != nil || !strings.Contains(string(encoded), `"items":[]`) {
		t.Fatalf("empty certificate page JSON = %s err=%v", encoded, err)
	}
}

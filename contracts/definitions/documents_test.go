package definitions

import (
	"encoding/json"
	admin "liapoldus.local/server-plugin/contracts/definitions/admin"
	httpdef "liapoldus.local/server-plugin/contracts/definitions/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// This test was first run against the pre-migration files. Every exported
// declaration must retain the complete document, including false and null.
func TestDocumentsPreserveArtifacts(t *testing.T) {
	for name, document := range Documents() {
		t.Run(name, func(t *testing.T) {
			old, err := os.ReadFile(filepath.Join("..", "v1", name))
			if err != nil {
				t.Fatal(err)
			}
			actual, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			var before, after any
			if err = json.Unmarshal(old, &before); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(actual, &after); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("declaration changed contract\nbefore: %s\nafter: %s", old, actual)
			}
		})
	}
}

func TestDeclarationsAreIndependent(t *testing.T) {
	first := admin.AdminActions()
	delete(first.Operations, "server.sites.publish")
	if _, ok := admin.AdminActions().Operations["server.sites.publish"]; !ok {
		t.Fatal("mutable declarations are shared")
	}
	firstDispatch := httpdef.HTTPDispatch()
	firstDispatch.BlockedHeaders[0] = "changed"
	if httpdef.HTTPDispatch().BlockedHeaders[0] == "changed" {
		t.Fatal("dispatch declarations are shared")
	}
}

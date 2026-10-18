package site

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"liapoldus.local/server-service/contracts"
)

func TestListSiteReleasesWithoutSiteIDAggregatesAndPaginatesAcrossSites(t *testing.T) {
	store, err := NewReleaseStore(filepath.Join(t.TempDir(), "releases"))
	if err != nil {
		t.Fatal(err)
	}
	writeActiveSiteRelease(t, store.root, "site-b", strings.Repeat("b", 64))
	writeActiveSiteRelease(t, store.root, "site-a", strings.Repeat("a", 64))

	first, err := store.ListSiteReleases(context.Background(), "", 1, "")
	if err != nil || len(first.Items) != 1 || first.Items[0].SiteID != "site-a" || first.NextCursor == nil {
		t.Fatalf("first all-sites release page = %+v err=%v", first, err)
	}
	second, err := store.ListSiteReleases(context.Background(), "", 1, *first.NextCursor)
	if err != nil || len(second.Items) != 1 || second.Items[0].SiteID != "site-b" || second.NextCursor != nil {
		t.Fatalf("second all-sites release page = %+v err=%v", second, err)
	}
}

func TestListSiteReleasesWithoutSitesReturnsEmptyArray(t *testing.T) {
	store, err := NewReleaseStore(filepath.Join(t.TempDir(), "releases"))
	if err != nil {
		t.Fatal(err)
	}
	page, err := store.ListSiteReleases(context.Background(), "", 50, "")
	if err != nil || page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("empty all-sites release page = %+v err=%v", page, err)
	}
	encoded, err := json.Marshal(page)
	if err != nil || !strings.Contains(string(encoded), `"items":[]`) {
		t.Fatalf("empty release page JSON = %s err=%v", encoded, err)
	}
}

func TestPaginateEmptyResultsReturnsEmptyArray(t *testing.T) {
	page, err := paginate([]string(nil), 50, "", func(item string) string { return item })
	if err != nil || page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("empty page = %+v err=%v", page, err)
	}
}

func writeActiveSiteRelease(t *testing.T, root, siteID, digest string) {
	t.Helper()
	siteRoot := filepath.Join(root, "sites", siteID)
	releasePath := filepath.Join(siteRoot, "releases", digest)
	if err := os.MkdirAll(releasePath, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestName, err := contracts.SiteManifestArchiveEntry()
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"schemaVersion":1,"siteId":"` + siteID + `","documentRoot":".","indexDocument":"index.html"}`)
	if err := os.WriteFile(filepath.Join(releasePath, manifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(releasePath, "index.html"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	generationID := "generation-1"
	generationPath := filepath.Join(siteRoot, "generations", generationID)
	if err := os.MkdirAll(generationPath, 0o700); err != nil {
		t.Fatal(err)
	}
	generation, err := json.Marshal(persistedGeneration{GenerationID: generationID, Current: "sha256:" + digest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generationPath, "generation.json"), generation, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("generations", generationID), filepath.Join(siteRoot, "active")); err != nil {
		t.Fatal(err)
	}
}

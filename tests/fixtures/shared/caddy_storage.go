package shared

import (
	"os"
	"path/filepath"
	"time"

	caddycore "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/certmagic"
)

// IsolateCaddyDataHome keeps runtime fixtures from reading or changing the
// operator's normal Caddy data directory.
func IsolateCaddyDataHome() (string, func(), error) {
	directory, err := os.MkdirTemp("", "server-caddy-test-")
	if err != nil {
		return "", nil, err
	}
	if err := os.Setenv("XDG_DATA_HOME", directory); err != nil {
		_ = os.RemoveAll(directory)
		return "", nil, err
	}
	if err := os.Setenv("XDG_CONFIG_HOME", filepath.Join(directory, "config")); err != nil {
		_ = os.RemoveAll(directory)
		return "", nil, err
	}
	// Caddy initializes DefaultStorage during package initialization, before
	// fixture main can set XDG_DATA_HOME. Reset it after selecting the temp root.
	caddycore.DefaultStorage = &certmagic.FileStorage{Path: caddycore.AppDataDir()}
	// ConfigAutosavePath is also computed during package initialization.
	caddycore.ConfigAutosavePath = filepath.Join(caddycore.AppConfigDir(), "autosave.json")
	// Runtime fixtures must never contact a public ACME service. Use a local
	// unreachable authority so activation semantics stay asynchronous without
	// issuing external account or certificate requests.
	certmagic.DefaultACME.CA = "http://127.0.0.1:1/acme/directory"
	return directory, func() {
		// Caddy's storage cleanup is stopped asynchronously after Config.Stop.
		// Let the cleanup worker finish before removing the fixture root.
		time.Sleep(time.Second)
		_ = os.RemoveAll(directory)
	}, nil
}

package main

import (
	"encoding/json"
	"os"
	"strings"

	caddycore "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/certmagic"
	"liapoldus.local/server-plugin/tests/fixtures/shared"
)

func main() {
	_, cleanup, err := shared.IsolateCaddyDataHome()
	check(err)
	defer cleanup()

	dataHome := os.Getenv("XDG_DATA_HOME")
	result := map[string]any{
		"isolated": strings.Contains(dataHome, "server-caddy-test-") &&
			caddycore.DefaultStorage.Path == caddycore.AppDataDir(),
		"acmeTestAuthority": certmagic.DefaultACME.CA,
		"dataHome":          dataHome,
	}
	check(json.NewEncoder(os.Stdout).Encode(result))
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

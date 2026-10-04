package main

import (
	"encoding/json"
	"os"

	caddycore "github.com/caddyserver/caddy/v2"
	_ "liapoldus.local/server-plugin/internal/infrastructure/caddy"
)

func main() {
	modules := caddycore.GetModules("dns.providers")
	ids := make([]string, 0, len(modules))
	for _, module := range modules {
		ids = append(ids, string(module.ID))
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		DNSProviderModules []string `json:"dnsProviderModules"`
	}{DNSProviderModules: ids}); err != nil {
		os.Exit(1)
	}
}

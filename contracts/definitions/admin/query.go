package admin

type AdminQueryDocument struct {
	PageResources               map[string][]string `json:"pageResources"`
	Resources                   map[string]string   `json:"resources"`
	Plugin                      string              `json:"plugin"`
	FallbackCapability          string              `json:"fallbackCapability"`
	ActionID                    string              `json:"actionId"`
	CertificateStatusCapability string              `json:"certificateStatusCapability"`
	Version                     int                 `json:"version"`
	DefaultLimit                int                 `json:"defaultLimit"`
}

func AdminQuery() AdminQueryDocument {
	return AdminQueryDocument{
		Version:                     int(1),
		Plugin:                      "server",
		DefaultLimit:                int(50),
		FallbackCapability:          "server.sites.list",
		ActionID:                    "query",
		CertificateStatusCapability: "server.certificates.get",
		PageResources: map[string][]string{
			"sites":        {"sites", "releases", "operations"},
			"certificates": {"certificates"},
		},
		Resources: map[string]string{
			"sites":        "server.sites.list",
			"releases":     "server.sites.releases.list",
			"operations":   "server.operations.list",
			"certificates": "server.certificates.list",
		},
	}
}

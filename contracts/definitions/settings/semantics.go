package settings

type SettingsSemanticsDocumentPathRequestPath struct {
	Source          string   `json:"source"`
	PercentDecode   string   `json:"percentDecode"`
	DotSegments     string   `json:"dotSegments"`
	RepeatedSlashes string   `json:"repeatedSlashes"`
	TrailingSlash   string   `json:"trailingSlash"`
	Query           string   `json:"query"`
	Reject          []string `json:"reject"`
}

type SettingsSemanticsDocumentPathConfiguredPathExactPrefixGlob struct {
	Representation string `json:"representation"`
	LeadingSlash   string `json:"leadingSlash"`
	Validation     string `json:"validation"`
	PercentDecode  string `json:"percentDecode"`
	Normalization  string `json:"normalization"`
}

type SettingsSemanticsDocumentPathConfiguredPathRegex struct {
	Engine     string `json:"engine"`
	Input      string `json:"input"`
	Validation string `json:"validation"`
	Match      string `json:"match"`
}

type SettingsSemanticsDocumentPathConfiguredPath struct {
	ExactPrefixGlob SettingsSemanticsDocumentPathConfiguredPathExactPrefixGlob `json:"exactPrefixGlob"`
	Regex           SettingsSemanticsDocumentPathConfiguredPathRegex           `json:"regex"`
}

type SettingsSemanticsDocumentPathGlobTokens struct {
	Star       string `json:"*"`
	DoubleStar string `json:"**"`
	Question   string `json:"?"`
}

type SettingsSemanticsDocumentPathGlob struct {
	Tokens       SettingsSemanticsDocumentPathGlobTokens `json:"tokens"`
	Tokenization string                                  `json:"tokenization"`
	Match        string                                  `json:"match"`
	Unsupported  []string                                `json:"unsupported"`
}

type SettingsSemanticsDocumentPath struct {
	RequestPath    SettingsSemanticsDocumentPathRequestPath    `json:"requestPath"`
	ConfiguredPath SettingsSemanticsDocumentPathConfiguredPath `json:"configuredPath"`
	Glob           SettingsSemanticsDocumentPathGlob           `json:"glob"`
}

type SettingsSemanticsDocumentSocketAddress struct {
	Format string `json:"format"`
	Host   string `json:"host"`
	Ipv4   string `json:"ipv4"`
	Ipv6   string `json:"ipv6"`
	Port   string `json:"port"`
}

type SettingsSemanticsDocumentAutomaticHTTPsRedirect struct {
	AutomaticTlsListener string `json:"automaticTlsListener"`
	SourceListener       string `json:"sourceListener"`
	HostCoverage         string `json:"hostCoverage"`
	Location             string `json:"location"`
	Method               string `json:"method"`
	Status               int    `json:"status"`
}

type SettingsSemanticsDocumentUpstreamPool struct {
	Scope              string   `json:"scope"`
	EligibilityChange  string   `json:"eligibilityChange"`
	PassiveFailure     string   `json:"passiveFailure"`
	Retry              string   `json:"retry"`
	NoRetryOrExclusion string   `json:"noRetryOrExclusion"`
	Selection          []string `json:"selection"`
	ActiveHealthChecks bool     `json:"activeHealthChecks"`
}

type SettingsSemanticsDocument struct {
	Path                   SettingsSemanticsDocumentPath                   `json:"path"`
	AutomaticHTTPsRedirect SettingsSemanticsDocumentAutomaticHTTPsRedirect `json:"automaticHttpsRedirect"`
	SocketAddress          SettingsSemanticsDocumentSocketAddress          `json:"socketAddress"`
	SettingsSchema         string                                          `json:"settingsSchema"`
	UpstreamPool           SettingsSemanticsDocumentUpstreamPool           `json:"upstreamPool"`
	Version                int                                             `json:"version"`
}

func SettingsSemantics() SettingsSemanticsDocument {
	return SettingsSemanticsDocument{
		Version:        int(1),
		SettingsSchema: "settings.schema.json",
		Path: SettingsSemanticsDocumentPath{
			RequestPath: SettingsSemanticsDocumentPathRequestPath{
				Source:          "HTTP request-target path before the first query delimiter; the query is kept separately",
				PercentDecode:   "strictly once, after splitting the query",
				Reject:          []string{"malformed-percent-encoding", "NUL", "C0-or-DEL-control", "invalid-UTF-8-after-decoding"},
				DotSegments:     "remove RFC 3986 dot segments after percent decoding; parent segments above root stay at root",
				RepeatedSlashes: "collapse to one slash after percent decoding",
				TrailingSlash:   "preserve when the decoded path ends with slash, slash-dot, or slash-dot-dot",
				Query:           "excluded from route matching and preserved unchanged for handlers",
			},
			ConfiguredPath: SettingsSemanticsDocumentPathConfiguredPath{
				ExactPrefixGlob: SettingsSemanticsDocumentPathConfiguredPathExactPrefixGlob{
					Representation: "already-decoded canonical absolute path text, not a URL or request-target",
					LeadingSlash:   "required",
					Validation:     "reject NUL/control, invalid UTF-8, repeated slashes, and literal . or .. path segments; preserve trailing slash",
					PercentDecode:  "never; percent is an ordinary literal character",
					Normalization:  "reject non-canonical input instead of rewriting it",
				},
				Regex: SettingsSemanticsDocumentPathConfiguredPathRegex{
					Engine:     "Go RE2",
					Input:      "regex source is not path-normalized",
					Validation: "compile during SDK Reload candidate apply; reject invalid UTF-8 and NUL/control",
					Match:      "the regex match must cover the complete canonical request path",
				},
			},
			Glob: SettingsSemanticsDocumentPathGlob{
				Tokens: SettingsSemanticsDocumentPathGlobTokens{
					Star:       "zero or more Unicode code points other than slash",
					DoubleStar: "zero or more Unicode code points including slash",
					Question:   "exactly one Unicode code point other than slash",
				},
				Tokenization: "scan left to right and choose the longest token; ** is recognized before *; *** is ** followed by *",
				Unsupported:  []string{"character classes", "escaping"},
				Match:        "the glob must match the complete canonical request path",
			},
		},
		SocketAddress: SettingsSemanticsDocumentSocketAddress{
			Format: "host:port; IPv6 host must be bracketed",
			Host:   "empty wildcard or IP literal only; DNS names are not bind addresses",
			Ipv4:   "validated as a complete IPv4 literal",
			Ipv6:   "validated with Go net/netip ParseAddr; zone identifiers are rejected",
			Port:   "ASCII decimal integer 1 through 65535; leading zeroes are rejected",
		},
		AutomaticHTTPsRedirect: SettingsSemanticsDocumentAutomaticHTTPsRedirect{
			AutomaticTlsListener: "ACME-managed certificates and HTTPS only; it does not create a separate HTTP socket implicitly",
			SourceListener:       "an explicit HTTP listener with tls.mode=disabled, protocols=[http1], redirectToListenerId targeting a TLS-enabled HTTP listener, and no routes",
			HostCoverage:         "the canonical request host must be covered by a hostname declared on the target listener; otherwise return 421 Misdirected Request without Location",
			Status:               int(308),
			Location:             "https scheme + canonical request host + canonical request path + original query",
			Method:               "preserved by the 308 response",
		},
		UpstreamPool: SettingsSemanticsDocumentUpstreamPool{
			Scope:              "independent smooth-weight state per route",
			Selection:          []string{"For each eligible origin, add its configured weight to its current weight.", "Select the origin with the greatest current weight; ties use original array order.", "Subtract the sum of eligible weights from the selected origin's current weight."},
			EligibilityChange:  "reset current weights for all origins in the new eligible set",
			PassiveFailure:     "TCP connection refusal or timeout before request bytes; exclude only that origin for 30 seconds",
			Retry:              "at most one next eligible origin, only after TCP connect refusal or timeout before any request byte",
			NoRetryOrExclusion: "TLS handshake/verification failure and any failure after TCP connect",
			ActiveHealthChecks: false,
		},
	}
}

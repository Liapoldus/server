package settings

import "liapoldus.local/server-plugin/contracts/schema"

type SettingsVectorsDocumentScenariosItemInputSource struct {
	Tls                  string   `json:"tls"`
	Protocols            []string `json:"protocols"`
	RedirectToListenerID string   `json:"redirectToListenerId"`
	Routes               []any    `json:"routes"`
}

type SettingsVectorsDocumentScenariosItemInputTarget struct {
	ID        *string  `json:"id,omitempty"`
	Tls       string   `json:"tls"`
	Hostnames []string `json:"hostnames"`
}

type SettingsVectorsDocumentScenariosItemInputRequest struct {
	Method        *string `json:"method,omitempty"`
	Host          string  `json:"host"`
	RequestTarget string  `json:"requestTarget"`
}

type SettingsVectorsDocumentScenariosItemInputUpstreamsItem struct {
	ID     string `json:"id"`
	Weight int    `json:"weight"`
}

type SettingsVectorsDocumentScenariosItemInput struct {
	RequestTarget    *string                                                   `json:"requestTarget,omitempty"`
	Type             *string                                                   `json:"type,omitempty"`
	Value            *string                                                   `json:"value,omitempty"`
	Pattern          *string                                                   `json:"pattern,omitempty"`
	Path             *string                                                   `json:"path,omitempty"`
	Address          *string                                                   `json:"address,omitempty"`
	Source           *SettingsVectorsDocumentScenariosItemInputSource          `json:"source,omitempty"`
	Target           *SettingsVectorsDocumentScenariosItemInputTarget          `json:"target,omitempty"`
	Request          *SettingsVectorsDocumentScenariosItemInputRequest         `json:"request,omitempty"`
	Upstreams        *[]SettingsVectorsDocumentScenariosItemInputUpstreamsItem `json:"upstreams,omitempty"`
	Selections       *int                                                      `json:"selections,omitempty"`
	Failure          *string                                                   `json:"failure,omitempty"`
	RequestBytesSent *int                                                      `json:"requestBytesSent,omitempty"`
	AttemptedOrigins *int                                                      `json:"attemptedOrigins,omitempty"`
}

type SettingsVectorsDocumentScenariosItemExpected struct {
	Accepted                    *bool     `json:"accepted,omitempty"`
	CanonicalPath               *string   `json:"canonicalPath,omitempty"`
	Query                       *string   `json:"query,omitempty"`
	Matches                     *bool     `json:"matches,omitempty"`
	Tokens                      *[]string `json:"tokens,omitempty"`
	Status                      *int      `json:"status,omitempty"`
	Location                    *any      `json:"location,omitempty"`
	MethodPreserved             *bool     `json:"methodPreserved,omitempty"`
	Sequence                    *[]string `json:"sequence,omitempty"`
	RetryAnotherOrigin          *bool     `json:"retryAnotherOrigin,omitempty"`
	MaxAdditionalAttempts       *int      `json:"maxAdditionalAttempts,omitempty"`
	FailedOriginCooldownSeconds *int      `json:"failedOriginCooldownSeconds,omitempty"`
}

type SettingsVectorsDocumentScenariosItem struct {
	Input    SettingsVectorsDocumentScenariosItemInput    `json:"input"`
	Expected SettingsVectorsDocumentScenariosItemExpected `json:"expected"`
	ID       string                                       `json:"id"`
}

type SettingsVectorsDocument struct {
	Semantics string                                 `json:"semantics"`
	Scenarios []SettingsVectorsDocumentScenariosItem `json:"scenarios"`
	Version   int                                    `json:"version"`
}

func SettingsVectors() SettingsVectorsDocument {
	return SettingsVectorsDocument{
		Version:   int(1),
		Semantics: "settings-semantics.json",
		Scenarios: []SettingsVectorsDocumentScenariosItem{{
			ID: "request-path-decodes-once-and-normalizes-dot-segments",
			Input: SettingsVectorsDocumentScenariosItemInput{
				RequestTarget: schema.Value("/a//b/%2e/c/../?page=1"),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Accepted:      schema.Value(true),
				CanonicalPath: schema.Value("/a/b/"),
				Query:         schema.Value("page=1"),
			},
		}, {
			ID: "request-path-percent-decodes-only-once",
			Input: SettingsVectorsDocumentScenariosItemInput{
				RequestTarget: schema.Value("/a/%252e%252e/b"),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Accepted:      schema.Value(true),
				CanonicalPath: schema.Value("/a/%2e%2e/b"),
				Query:         schema.Value(""),
			},
		}, {
			ID: "request-path-rejects-malformed-percent-encoding",
			Input: SettingsVectorsDocumentScenariosItemInput{
				RequestTarget: schema.Value("/bad%2"),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Accepted: schema.Value(false),
			},
		}, {
			ID: "request-path-preserves-query-separately",
			Input: SettingsVectorsDocumentScenariosItemInput{
				RequestTarget: schema.Value("/search?q=a%2Fb"),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Accepted:      schema.Value(true),
				CanonicalPath: schema.Value("/search"),
				Query:         schema.Value("q=a%2Fb"),
			},
		}, {
			ID: "configured-exact-path-rejects-dot-segments",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Type:  schema.Value("exact"),
				Value: schema.Value("/a/../b"),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Accepted: schema.Value(false),
			},
		}, {
			ID: "configured-prefix-path-rejects-repeated-slashes",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Type:  schema.Value("prefix"),
				Value: schema.Value("/a//b"),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Accepted: schema.Value(false),
			},
		}, {
			ID: "glob-starstar-crosses-path-separators",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Pattern: schema.Value("/assets/**"),
				Path:    schema.Value("/assets/a/b"),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Matches: schema.Value(true),
			},
		}, {
			ID: "glob-star-does-not-cross-path-separators",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Pattern: schema.Value("/assets/*"),
				Path:    schema.Value("/assets/a/b"),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Matches: schema.Value(false),
			},
		}, {
			ID: "glob-triple-star-uses-longest-tokenization",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Pattern: schema.Value("/x/***"),
				Path:    schema.Value("/x/a/b"),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Matches: schema.Value(true),
				Tokens:  schema.Value([]string{"**", "*"}),
			},
		}, {
			ID: "ipv6-bind-valid-literal",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Address: schema.Value("[2001:db8::1]:8443"),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Accepted: schema.Value(true),
			},
		}, {
			ID: "ipv6-bind-rejects-invalid-literal",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Address: schema.Value("[::::]:8443"),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Accepted: schema.Value(false),
			},
		}, {
			ID: "automatic-https-redirect-preserves-method-and-query",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Source: schema.Value(SettingsVectorsDocumentScenariosItemInputSource{
					Tls:                  "disabled",
					Protocols:            []string{"http1"},
					RedirectToListenerID: "https",
					Routes:               []any{},
				}),
				Target: schema.Value(SettingsVectorsDocumentScenariosItemInputTarget{
					ID:        schema.Value("https"),
					Tls:       "automatic",
					Hostnames: []string{"example.test"},
				}),
				Request: schema.Value(SettingsVectorsDocumentScenariosItemInputRequest{
					Method:        schema.Value("POST"),
					Host:          "example.test",
					RequestTarget: "/upload?q=1",
				}),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Status:          schema.Value(int(308)),
				Location:        schema.Value[any]("https://example.test/upload?q=1"),
				MethodPreserved: schema.Value(true),
			},
		}, {
			ID: "automatic-https-rejects-host-outside-target-coverage",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Target: schema.Value(SettingsVectorsDocumentScenariosItemInputTarget{
					Tls:       "automatic",
					Hostnames: []string{"example.test"},
				}),
				Request: schema.Value(SettingsVectorsDocumentScenariosItemInputRequest{
					Host:          "attacker.test",
					RequestTarget: "/",
				}),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Status:   schema.Value(int(421)),
				Location: schema.Value[any](nil),
			},
		}, {
			ID: "smooth-weighted-round-robin-honors-configured-weights",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Upstreams: schema.Value([]SettingsVectorsDocumentScenariosItemInputUpstreamsItem{{
					ID:     "a",
					Weight: int(5),
				}, {
					ID:     "b",
					Weight: int(1),
				}}),
				Selections: schema.Value(int(6)),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				Sequence: schema.Value([]string{"a", "a", "a", "b", "a", "a"}),
			},
		}, {
			ID: "upstream-retry-is-limited-to-one-pre-bytes-connect-failure",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Failure:          schema.Value("tcp_connect_timeout"),
				RequestBytesSent: schema.Value(int(0)),
				AttemptedOrigins: schema.Value(int(1)),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				RetryAnotherOrigin:          schema.Value(true),
				MaxAdditionalAttempts:       schema.Value(int(1)),
				FailedOriginCooldownSeconds: schema.Value(int(30)),
			},
		}, {
			ID: "upstream-does-not-retry-after-request-bytes",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Failure:          schema.Value("connection_reset"),
				RequestBytesSent: schema.Value(int(1)),
				AttemptedOrigins: schema.Value(int(1)),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				RetryAnotherOrigin:    schema.Value(false),
				MaxAdditionalAttempts: schema.Value(int(0)),
			},
		}, {
			ID: "upstream-tls-verification-failure-does-not-retry-or-exclude",
			Input: SettingsVectorsDocumentScenariosItemInput{
				Failure:          schema.Value("tls_verification"),
				RequestBytesSent: schema.Value(int(0)),
				AttemptedOrigins: schema.Value(int(1)),
			},
			Expected: SettingsVectorsDocumentScenariosItemExpected{
				RetryAnotherOrigin:          schema.Value(false),
				MaxAdditionalAttempts:       schema.Value(int(0)),
				FailedOriginCooldownSeconds: schema.Value(int(0)),
			},
		}},
	}
}

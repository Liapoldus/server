package settings

import "liapoldus.local/server-plugin/contracts/schema"

func SettingsSchema() schema.Definition {
	return schema.Definition{
		Definition:           "https://json-schema.org/draft/2020-12/schema",
		ID:                   "https://liapoldus.github.io/plugins/server/settings/v1/schema.json",
		Title:                "Server plugin settings v1",
		Description:          "Strict Liapoldus configuration compiled privately into the Caddy runtime. Native Caddy JSON and Caddyfile input are not accepted.",
		Type:                 "object",
		AdditionalProperties: false,
		Required:             schema.Value([]string{"schemaVersion", "config"}),
		Properties: map[string]schema.Definition{"schemaVersion": {
			Const: 1,
		}, "config": {
			Ref: "#/$defs/config",
		}},
		Defs: map[string]schema.Definition{"nonEmptyString": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
		}, "socketAddress": {
			Type:        "string",
			Description: "An HTTP listener bind address in host:port form. Host is empty wildcard, IPv4 literal, or bracketed IPv6 literal; port is ASCII decimal 1 through 65535 without leading zero. Reload additionally validates IP literals with netip.ParseAddr and rejects IPv6 zone identifiers.",
			Pattern:     "^(?::(?:[1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])|(?:(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\\.){3}(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9]):(?:[1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])|\\[[0-9A-Fa-f:.]+\\]:(?:[1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]))$",
		}, "hostname": {
			Type:        "string",
			MinLength:   schema.Value(int(1)),
			MaxLength:   schema.Value(int(255)),
			Description: "DNS hostname or a leftmost wildcard. Reload canonicalizes IDNA to a lowercase A-label without a port; a wildcard matches exactly one DNS label and never the apex.",
			Pattern:     "^(?:\\*\\.)?[^.*:/?#\\s]+(?:\\.[^.*:/?#\\s]+)*$",
		}, "httpProtocol": {
			Type: "string",
			Enum: []any{"http1", "http2", "http3"},
		}, "tlsAutomatic": {
			Type:                 "object",
			Description:          "Uses Caddy-managed ACME certificates for declared hostnames. HTTP redirects to HTTPS only through a separately declared plaintext redirect-only listener; no HTTP socket is opened implicitly. Effective TLS bounds are TLS 1.2 minimum and TLS 1.3 maximum; HTTP/3 is available only over TLS. HTTP-01 and TLS-ALPN-01 use built modules; DNS-01 is available only for provider modules compiled into this binary. The provider list is a build concern, not a settings field.",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"mode"}),
			Properties: map[string]schema.Definition{"mode": {
				Const: "automatic",
			}},
		}, "tlsCustom": {
			Type:                 "object",
			Description:          "Uses the referenced certificate/key pair. The plugin resolves references only through the scoped secret-grant boundary and validates the pair before activation. Effective TLS bounds are TLS 1.2 minimum and TLS 1.3 maximum.",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"mode", "certificateRef", "privateKeyRef"}),
			Properties: map[string]schema.Definition{"mode": {
				Const: "custom",
			}, "certificateRef": {
				Ref:         "#/$defs/nonEmptyString",
				Description: "Opaque Core-generated secret reference; never PEM bytes or a path.",
			}, "privateKeyRef": {
				Ref:         "#/$defs/nonEmptyString",
				Description: "Opaque Core-generated secret reference; never PEM bytes or a path.",
			}},
		}, "tlsDisabled": {
			Type:                 "object",
			Description:          "Explicit HTTP/1.1 plaintext listener. It cannot declare hostnames, routes with host matchers, or ACME behavior.",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"mode"}),
			Properties: map[string]schema.Definition{"mode": {
				Const: "disabled",
			}},
		}, "tls": {
			OneOf: []schema.Definition{{
				Ref: "#/$defs/tlsAutomatic",
			}, {
				Ref: "#/$defs/tlsCustom",
			}, {
				Ref: "#/$defs/tlsDisabled",
			}},
		}, "httpListener": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"id", "kind", "address", "hostnames", "protocols", "tls"}),
			Properties: map[string]schema.Definition{"id": {
				Ref: "#/$defs/nonEmptyString",
			}, "kind": {
				Const: "http",
			}, "address": {
				Ref: "#/$defs/socketAddress",
			}, "hostnames": {
				Type:        "array",
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/hostname",
				}),
				Description: "Explicit hostnames owned by this listener. IDNA/case-equivalent duplicates are rejected during SDK Reload candidate apply.",
			}, "protocols": {
				Type:        "array",
				MinItems:    schema.Value(int(1)),
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/httpProtocol",
				}),
				Description: "Explicitly enabled protocols; HTTP/3 is permitted only for automatic or custom TLS. No protocol is enabled implicitly.",
			}, "tls": {
				Ref: "#/$defs/tls",
			}, "redirectToListenerId": {
				Ref:         "#/$defs/nonEmptyString",
				Description: "Optional explicit redirect-only listener target. The target must be a TLS-enabled HTTP listener with declared hostname coverage; the source listener must have no routes. The redirect source never binds implicitly.",
			}},
			AllOf: []schema.Definition{{
				If: schema.Value(schema.Definition{
					Type: "object",
					Properties: map[string]schema.Definition{"tls": {
						Type: "object",
						Properties: map[string]schema.Definition{"mode": {
							Const: "disabled",
						}},
					}},
				}),
				Then: schema.Value(schema.Definition{
					Properties: map[string]schema.Definition{"hostnames": {
						Type:     "array",
						MaxItems: schema.Value(int(0)),
					}, "protocols": {
						Const: []any{"http1"},
					}},
				}),
				Else: schema.Value(schema.Definition{
					Properties: map[string]schema.Definition{"hostnames": {
						Type:     "array",
						MinItems: schema.Value(int(1)),
					}},
				}),
			}, {
				If: schema.Value(schema.Definition{
					Type:     "object",
					Required: schema.Value([]string{"redirectToListenerId"}),
				}),
				Then: schema.Value(schema.Definition{
					Properties: map[string]schema.Definition{"tls": {
						Type: "object",
						Properties: map[string]schema.Definition{"mode": {
							Const: "disabled",
						}},
					}, "protocols": {
						Const: []any{"http1"},
					}},
				}),
			}},
		}, "listener": {
			Ref: "#/$defs/httpListener",
		}, "pathMatcher": {
			Description: "Exactly one path mode compares against the canonical request path without query. Before matching, the request path rejects malformed percent-encoding, NUL/control and invalid UTF-8, percent-decodes exactly once, removes dot segments, collapses repeated slashes and preserves a trailing slash (including paths ending in /. or /..). exact is equality and prefix is case-sensitive prefix matching; glob and regex are full-match. exact/prefix/glob configured values are already-decoded canonical absolute paths: Reload rejects repeated slashes and literal dot segments and never percent-decodes settings. Glob tokens are read left-to-right using the longest token (** before *), so *** means ** followed by *; * matches any number of non-slash characters, ** any characters including slash, and ? exactly one non-slash character; character classes and escaping are not supported. Regex uses Go RE2 full-match over the canonical request path; its source is compiled but not path-normalized. Reload validates configured paths and compiles regex before activation.",
			OneOf: []schema.Definition{{
				Type:                 "object",
				AdditionalProperties: false,
				Required:             schema.Value([]string{"type", "value"}),
				Properties: map[string]schema.Definition{"type": {
					Const: "exact",
				}, "value": {
					Ref: "#/$defs/nonEmptyString",
				}},
			}, {
				Type:                 "object",
				AdditionalProperties: false,
				Required:             schema.Value([]string{"type", "value"}),
				Properties: map[string]schema.Definition{"type": {
					Const: "prefix",
				}, "value": {
					Ref: "#/$defs/nonEmptyString",
				}},
			}, {
				Type:                 "object",
				AdditionalProperties: false,
				Required:             schema.Value([]string{"type", "value"}),
				Properties: map[string]schema.Definition{"type": {
					Const: "glob",
				}, "value": {
					Ref: "#/$defs/nonEmptyString",
				}},
				AllOf: []schema.Definition{{
					Properties: map[string]schema.Definition{"value": {
						Not: schema.Value(schema.Definition{
							Type:    "string",
							Pattern: "\\[|\\]|\\\\",
						}),
					}},
				}},
			}, {
				Type:                 "object",
				AdditionalProperties: false,
				Required:             schema.Value([]string{"type", "value"}),
				Properties: map[string]schema.Definition{"type": {
					Const: "regex",
				}, "value": {
					Ref: "#/$defs/nonEmptyString",
				}},
			}},
		}, "method": {
			Type:      "string",
			MinLength: schema.Value(int(1)),
			Pattern:   "^[!#$%&'*+.^_`|~0-9A-Za-z-]+$",
		}, "httpMatch": {
			Type:                 "object",
			AdditionalProperties: false,
			Description:          "All supplied predicates are combined with AND; omitted predicates are wildcards. An absent match object matches every request for the listener. Hosts are exact or a leftmost one-label wildcard, canonicalized during SDK Reload candidate apply as lowercase IDNA A-labels without ports; methods are exact case-sensitive HTTP method tokens.",
			Properties: map[string]schema.Definition{"hosts": {
				Type:        "array",
				MinItems:    schema.Value(int(1)),
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/hostname",
				}),
			}, "methods": {
				Type:        "array",
				MinItems:    schema.Value(int(1)),
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/method",
				}),
			}, "path": {
				Ref: "#/$defs/pathMatcher",
			}},
		}, "staticHandler": {
			Type:                 "object",
			Description:          "Serves the active current release for siteId. A route cannot pin an immutable release or select previous; publishing or rollback changes the current pointer without changing the route.",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"type", "siteId"}),
			Properties: map[string]schema.Definition{"type": {
				Const: "static",
			}, "siteId": {
				Ref: "#/$defs/nonEmptyString",
			}},
		}, "upstream": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"origin"}),
			Properties: map[string]schema.Definition{"origin": {
				Type:        "string",
				Pattern:     "^https?://[^/@?#\\s]+$",
				Description: "HTTP(S) origin authority only: no userinfo, path, query, or fragment. Full URL and port parsing is checked during SDK Reload candidate apply.",
			}, "weight": {
				Type:        "integer",
				Minimum:     schema.Value(int(1)),
				Maximum:     schema.Value(int(100)),
				Default:     1,
				Description: "Smooth weighted round-robin weight. An omitted value is interpreted as 1.",
			}, "caRef": {
				Ref:         "#/$defs/nonEmptyString",
				Description: "Optional opaque private-CA secret reference; accepted only for HTTPS origins. It extends the system trust chain and never disables hostname verification.",
			}},
			AllOf: []schema.Definition{{
				If: schema.Value(schema.Definition{
					Type: "object",
					Properties: map[string]schema.Definition{"origin": {
						Type:    "string",
						Pattern: "^http://",
					}},
					Required: schema.Value([]string{"origin"}),
				}),
				Then: schema.Value(schema.Definition{
					Not: schema.Value(schema.Definition{
						Required: schema.Value([]string{"caRef"}),
					}),
				}),
			}},
		}, "reverseProxyHandler": {
			Type:                 "object",
			Description:          "Uses per-route smooth weighted round-robin with 1–32 origins and weights 1–100 (default 1): add each eligible weight to its current weight, choose the largest (ties use array order), then subtract the eligible-weight sum from the selected origin. If the eligible set changes, current weights reset for that set. Only TCP refusal/timeout before request bytes passively excludes an origin for 30 seconds and permits at most one next-origin retry. TLS handshake/verification failure and failures after TCP connect are terminal and neither retried nor passively excluded; active health probes are not used. Once request bytes are sent, the request is never replayed.",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"type", "upstreams"}),
			Properties: map[string]schema.Definition{"type": {
				Const: "reverseProxy",
			}, "upstreams": {
				Type:     "array",
				MinItems: schema.Value(int(1)),
				MaxItems: schema.Value(int(32)),
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/upstream",
				}),
			}},
		}, "pluginHandler": {
			Type:                 "object",
			AdditionalProperties: false,
			Required:             schema.Value([]string{"type", "instanceId", "capability", "mode"}),
			Properties: map[string]schema.Definition{"type": {
				Const: "plugin",
			}, "instanceId": {
				Ref: "#/$defs/nonEmptyString",
			}, "capability": {
				Ref: "#/$defs/nonEmptyString",
			}, "requestCookieNames": {
				Type:        "array",
				UniqueItems: schema.Value(true),
				Items: schema.Value(schema.Definition{
					Type:      "string",
					MinLength: schema.Value(int(1)),
					Pattern:   "^[!#$%&'*+.^_`|~0-9A-Za-z-]+$",
				}),
				Description: "Only these request Cookie names are included in the plugin request context; all other cookies are omitted.",
			}, "mode": {
				Enum:        []any{"call", "http_stream", "websocket", "sse"},
				Description: "Selects a Server-supported invocation mode. Target method availability is determined by the generic peer call at invocation time; Core does not inspect product capabilities.",
			}, "streamLimits": {
				Type:                 "object",
				AdditionalProperties: false,
				Description:          "Optional per-route bounds for long-lived streaming modes; all limits remain subject to the per-instance ceiling. Omitted values use the versioned defaults: concurrency 128 per route and instance, idle 60000 ms, maximum duration 3600000 ms.",
				Properties: map[string]schema.Definition{"maxConcurrency": {
					Type:    "integer",
					Minimum: schema.Value(int(1)),
					Maximum: schema.Value(int(128)),
				}, "idleTimeoutMillis": {
					Type:    "integer",
					Minimum: schema.Value(int(1000)),
					Maximum: schema.Value(int(60000)),
				}, "maxDurationMillis": {
					Type:    "integer",
					Minimum: schema.Value(int(1000)),
					Maximum: schema.Value(int(3600000)),
				}},
			}},
			AllOf: []schema.Definition{{
				If: schema.Value(schema.Definition{
					Properties: map[string]schema.Definition{"mode": {
						Const: "call",
					}},
					Required: schema.Value([]string{"mode"}),
				}),
				Then: schema.Value(schema.Definition{
					Not: schema.Value(schema.Definition{
						Required: schema.Value([]string{"streamLimits"}),
					}),
				}),
			}},
		}, "httpHandler": {
			OneOf: []schema.Definition{{
				Ref: "#/$defs/staticHandler",
			}, {
				Ref: "#/$defs/reverseProxyHandler",
			}, {
				Ref: "#/$defs/pluginHandler",
			}},
		}, "httpRoute": {
			Type:                 "object",
			AdditionalProperties: false,
			Description:          "Routes are evaluated in array order for their listener; the first matching route is terminal and executes exactly one handler. Supplied matcher predicates are ANDed. SDK Reload candidate apply rejects unresolved listener references, duplicate IDs/binds, canonical-equivalent matchers, invalid normalized paths/hosts, TLS certificate coverage errors and mode values outside the Server-supported enum before activation. Target peer method availability is checked only when invoked.",
			Required:             schema.Value([]string{"id", "listenerId", "handler"}),
			Properties: map[string]schema.Definition{"id": {
				Ref: "#/$defs/nonEmptyString",
			}, "listenerId": {
				Ref: "#/$defs/nonEmptyString",
			}, "match": {
				Ref: "#/$defs/httpMatch",
			}, "handler": {
				Ref: "#/$defs/httpHandler",
			}},
		}, "route": {
			Ref: "#/$defs/httpRoute",
		}, "config": {
			Type:                 "object",
			AdditionalProperties: false,
			Description:          "Strict Liapoldus desired settings, not native Caddy JSON or Caddyfile. Before compiling a candidate runtime configuration, SDK Reload candidate apply rejects duplicate IDs/binds, unresolved listener/site references, route/listener kind mismatches, canonical-equivalent matchers, invalid normalized paths/hosts, TLS certificate coverage errors and mode values outside the Server-supported enum. It does not preflight product methods on remote peers.",
			Required:             schema.Value([]string{"listeners", "routes"}),
			Properties: map[string]schema.Definition{"listeners": {
				Type: "array",
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/listener",
				}),
			}, "routes": {
				Type: "array",
				Items: schema.Value(schema.Definition{
					Ref: "#/$defs/route",
				}),
			}},
		}},
	}
}

import { readContract } from '../support/contracts.js';
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { settingsAjv } from "../support/settings-ajv.js";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const schema = readContract('settings.schema.json');
const serviceContract = readContract('service.json');
const validate = settingsAjv().compile(schema);

const httpListener = {
  id: "web",
  kind: "http",
  address: ":443",
  hostnames: ["api.example.test"],
  protocols: ["http1", "http2", "http3"],
  tls: { mode: "automatic" },
};
const httpRoute = {
  id: "api",
  listenerId: "web",
  match: { hosts: ["api.example.test"], methods: ["GET", "POST"], path: { type: "prefix", value: "/v1" } },
  handler: { type: "reverseProxy", upstreams: [{ origin: "https://origin.example.test" }] },
};
const serviceAPI = (method = "GET", routePath = "/v1/echo") => ({
  version: "server-http/v1",
  method,
  path: routePath,
  responses: [{ status: 200, contentType: "application/json", schema: {}, example: null }],
});

function settings(config: Record<string, unknown>) {
  return { schemaVersion: 2, config };
}

function accepts(candidate: unknown): boolean {
  return validate(candidate) as boolean;
}

describe("Server service strict settings schema v2", () => {
  it("advertises the service-owned listener and route fields", () => {
    expect(serviceContract.configuration).toMatchObject({ schemaVersion: 2, versionField: "schemaVersion", runtimeConfigField: "config" });
    expect(serviceContract.configSchema.fields).toEqual([
      expect.objectContaining({ name: "listeners", type: "array", required: true }) as unknown,
      expect.objectContaining({ name: "routes", type: "array", required: true }) as unknown,
    ]);
    expect(schema.$defs.config.description).toContain("SDK Reload candidate apply");
    expect(schema.$defs.httpRoute.description).toContain("canonical-equivalent matchers");
  });

  it("accepts empty configuration and valid HTTP and redirect listeners", () => {
    expect(accepts(settings({ listeners: [], routes: [] }))).toBe(true);
    expect(accepts(settings({
      listeners: [
        httpListener,
        { id: "redirect", kind: "http", address: ":80", hostnames: [], protocols: ["http1"], tls: { mode: "disabled" }, redirectToListenerId: "web" },
      ],
      routes: [],
    }))).toBe(true);
    expect(accepts(settings({ listeners: [{ id: "tcp-db", kind: "l4", network: "tcp", address: ":5432" }], routes: [] }))).toBe(false);
  });

  it("accepts custom-certificate and explicit plaintext listener variants", () => {
    expect(accepts(settings({
      listeners: [
        { id: "custom", kind: "http", address: "192.0.2.10:8443", hostnames: ["custom.example.test"], protocols: ["http1", "http2"], tls: { mode: "custom", certificateRef: "ssr:v1:00000000000000000000000000000001", privateKeyRef: "ssr:v1:00000000000000000000000000000002" } },
        { id: "plain", kind: "http", address: ":8080", hostnames: [], protocols: ["http1"], tls: { mode: "disabled" } },
      ],
      routes: [],
    }))).toBe(true);
    expect(schema.$defs.tlsAutomatic.description).toContain("no HTTP socket is opened implicitly");
    expect(schema.$defs.tlsAutomatic.description).toContain("TLS 1.2 minimum and TLS 1.3 maximum");
    expect(schema.$defs.tlsCustom.description).toContain("validates the pair before activation");
  });

	it("leaves IDNA hostname canonicalization to semantic settings validation", () => {
    expect(accepts(settings({
      listeners: [{ ...httpListener, hostnames: ["münich.example.test"] }],
      routes: [{ ...httpRoute, match: { hosts: ["münich.example.test"] } }],
    }))).toBe(true);
    expect(accepts(settings({
      listeners: [{ ...httpListener, hostnames: ["*.example.test"] }],
      routes: [{ ...httpRoute, match: { hosts: ["*.example.test"] } }],
    }))).toBe(true);
    expect(accepts(settings({
      listeners: [{ ...httpListener, hostnames: ["API.Example.Test"] }],
      routes: [{ ...httpRoute, match: { hosts: ["API.Example.Test"] } }],
    }))).toBe(true);
    expect(schema.$defs.hostname.description).toContain("exactly one DNS label");
    expect(schema.$defs.hostname.description).toContain("lowercase A-label");
  });

  it("accepts request matchers for reverse proxies and mounts static releases by URL prefix", () => {
    const paths = [
      { type: "exact", value: "/exact" },
      { type: "prefix", value: "/prefix" },
      { type: "glob", value: "/assets/*" },
      { type: "glob", value: "/assets/**" },
      { type: "glob", value: "/files/file-?.js" },
      { type: "regex", value: "/v[0-9]+/items" },
    ];
    const handlers = [{ type: "reverseProxy", upstreams: [{ origin: "https://origin.example.test" }] }];
    for (const path of paths) for (const handler of handlers) {
      expect(accepts(settings({ listeners: [httpListener], routes: [{ ...httpRoute, match: { path }, handler }] }))).toBe(true);
    }
    expect(accepts(settings({ listeners: [httpListener], routes: [{ id: "static", listenerId: "web", handler: { type: "static", siteId: "frontend", releaseId: `sha256:${"a".repeat(64)}`, mountPrefix: "/assets" } }] }))).toBe(true);
    const serviceHandlers = [
      { type: "service", instanceId: "identity", capability: "identity.login", mode: "call" },
      { type: "service", instanceId: "forms", capability: "forms.submit", mode: "http_stream" },
      { type: "service", instanceId: "socket", capability: "events.socket", mode: "websocket" },
      { type: "service", instanceId: "events", capability: "events.subscribe", mode: "sse" },
    ];
    for (const [index, handler] of serviceHandlers.entries()) {
      expect(accepts(settings({ listeners: [httpListener], routes: [{
        id: `service-${index}`,
        listenerId: "web",
        match: { hosts: ["api.example.test"] },
        api: serviceAPI(index === 0 ? "POST" : "GET", `/v1/service-${index}`),
        handler,
      }] }))).toBe(true);
    }
    expect(schema.$defs.pathMatcher.description).toContain("percent-decodes exactly once");
    expect(schema.$defs.pathMatcher.description).toContain("Go RE2 full-match");
    expect(schema.$defs.httpRoute.description).toContain("first matching route is terminal");
    expect(schema.$defs.serviceAPIContract.description).toContain("validated before service dispatch");
    expect(schema.$defs.apiParameter.properties.repeated.enum).toEqual(["single", "first", "last", "repeat", "commaDelimited"]);
    expect(schema.$defs.staticHandler.description).toContain("immutable site release");
  });

  it("accepts service dispatch and rejects the removed runtime handler name", () => {
    const handler = { type: "service", instanceId: "forms", capability: "forms.submit", mode: "call" };
    const candidate = (type: string) => settings({
      listeners: [httpListener],
      routes: [{ id: "typed-api", listenerId: "web", api: serviceAPI("POST", "/submit"), handler: { ...handler, type } }],
    });
    expect(accepts(candidate("service"))).toBe(true);
    expect(accepts(candidate("plugin"))).toBe(false);
    expect(readContract('http-dispatch.json').module).toBe("http.handlers.liapoldus_service");
    expect(serviceContract.name).toBe("server");
  });

  it("rejects legacy untyped service endpoints and ambiguous matchers", () => {
    const handler = { type: "service", instanceId: "echo", capability: "echo.handle", mode: "call" };
    expect(accepts(settings({ listeners: [httpListener], routes: [{
      id: "legacy", listenerId: "web", match: { methods: ["GET"], path: { type: "exact", value: "/echo" } }, handler,
    }] }))).toBe(false);
    expect(accepts(settings({ listeners: [httpListener], routes: [{
      id: "duplicate-path-source", listenerId: "web", match: { path: { type: "exact", value: "/ignored" } }, api: serviceAPI(), handler,
    }] }))).toBe(false);
  });

  it("publishes exact path, glob, address, and explicit redirect semantics", () => {
    const semantics = readContract('settings-semantics.json');
    expect(serviceContract.configSchema.semantics).toBe("contracts/v1/settings-semantics.json");
    expect(semantics.path).toMatchObject({
      requestPath: {
        percentDecode: "strictly once, after splitting the query",
        dotSegments: "remove RFC 3986 dot segments after percent decoding; parent segments above root stay at root",
        repeatedSlashes: "collapse to one slash after percent decoding",
        trailingSlash: "preserve when the decoded path ends with slash, slash-dot, or slash-dot-dot",
        query: "excluded from route matching and preserved unchanged for handlers",
      },
      configuredPath: {
        exactPrefixGlob: {
          representation: "already-decoded canonical absolute path text, not a URL or request-target",
          percentDecode: "never; percent is an ordinary literal character",
        },
        regex: {
          engine: "Go RE2",
          match: "the regex match must cover the complete canonical request path",
        },
      },
      glob: {
        tokenization: "scan left to right and choose the longest token; ** is recognized before *; *** is ** followed by *",
      },
    });
    expect(semantics.socketAddress).toMatchObject({
      ipv6: "validated with Go net/netip ParseAddr; zone identifiers are rejected",
      host: "empty wildcard or IP literal only; DNS names are not bind addresses",
      port: "ASCII decimal integer 1 through 65535; leading zeroes are rejected",
    });
    expect(semantics.automaticHttpsRedirect).toMatchObject({
      automaticTlsListener: "ACME-managed certificates and HTTPS only; it does not create a separate HTTP socket implicitly",
      status: 308,
      hostCoverage: "the canonical request host must be covered by a hostname declared on the target listener; otherwise return 421 Misdirected Request without Location",
    });
    expect(semantics.upstreamPool).toMatchObject({
      eligibilityChange: "reset current weights for all origins in the new eligible set",
      passiveFailure: "TCP connection refusal or timeout before request bytes; exclude only that origin for 30 seconds",
      retry: "at most one next eligible origin, only after TCP connect refusal or timeout before any request byte",
      noRetryOrExclusion: "TLS handshake/verification failure and any failure after TCP connect",
      activeHealthChecks: true,
    });
    expect(semantics.automaticHttpsRedirect).toMatchObject({
      sourceListener: "an explicit HTTP listener with tls.mode=disabled, protocols=[http1], redirectToListenerId targeting a TLS-enabled HTTP listener, and no routes",
      location: "https scheme + canonical request host + canonical request path + original query",
      method: "preserved by the 308 response",
    });
  });

  it("publishes conformance vectors for Caddy settings runtime semantics", () => {
    const vectors = readContract('settings-vectors.json');
    expect(serviceContract.configSchema.conformanceVectors).toBe("contracts/v1/settings-vectors.json");
    expect(vectors).toMatchObject({ version: 1, semantics: "settings-semantics.json" });
    expect(vectors.scenarios.map((scenario: { id: string }) => scenario.id)).toEqual(expect.arrayContaining([
      "request-path-decodes-once-and-normalizes-dot-segments",
      "request-path-rejects-malformed-percent-encoding",
      "configured-exact-path-rejects-dot-segments",
      "glob-starstar-crosses-path-separators",
      "glob-star-does-not-cross-path-separators",
      "glob-triple-star-uses-longest-tokenization",
      "ipv6-bind-valid-literal",
      "ipv6-bind-rejects-invalid-literal",
      "automatic-https-redirect-preserves-method-and-query",
      "automatic-https-rejects-host-outside-target-coverage",
      "smooth-weighted-round-robin-honors-configured-weights",
      "upstream-retry-is-limited-to-one-pre-bytes-connect-failure",
    ]));
    expect(new Set(vectors.scenarios.map((scenario: { id: string }) => scenario.id)).size).toBe(vectors.scenarios.length);
  });

  it("accepts weighted HTTPS pools, private CA refs, and the weight default", () => {
    const maxPool = Array.from({ length: 32 }, (_, index) => ({ origin: `https://origin-${index}.example.test` }));
    expect(accepts(settings({
      listeners: [httpListener],
      routes: [{ ...httpRoute, handler: { type: "reverseProxy", upstreams: [
        { origin: "https://one.internal:8443", weight: 2, caRef: "ssr:v1:00000000000000000000000000000003" },
        { origin: "http://192.0.2.25:8080", weight: 100 },
        { origin: "http://a" },
      ] } }],
    }))).toBe(true);
    expect(accepts(settings({ listeners: [httpListener], routes: [{ ...httpRoute, handler: { type: "reverseProxy", upstreams: maxPool } }] }))).toBe(true);
    expect(schema.$defs.upstream.properties.weight.default).toBe(1);
    expect(schema.$defs.upstream.properties.caRef.description).toContain("never disables hostname verification");
    expect(schema.$defs.reverseProxyHandler.description).toContain("Configure active GET health probes with route.features.healthCheck");
    expect(schema.$defs.reverseProxyHandler.description).toContain("before request bytes passively excludes");
  });

  it("rejects native Caddy JSON and unknown keys at all schema levels", () => {
    const cases = [
      settings({ apps: {} }),
      settings({ listeners: [], routes: [], app: {} }),
      settings({ listeners: [] }),
      { schemaVersion: 2, config: { listeners: [], routes: [] }, extra: true },
      settings({ listeners: [{ ...httpListener, extra: true }], routes: [] }),
      settings({ listeners: [{ ...httpListener, tls: { mode: "automatic", extra: true } }], routes: [] }),
      settings({ listeners: [httpListener], routes: [{ ...httpRoute, extra: true }] }),
      settings({ listeners: [httpListener], routes: [{ ...httpRoute, match: { path: { type: "exact", value: "/", extra: true } } }] }),
      settings({ listeners: [httpListener], routes: [{ ...httpRoute, handler: { type: "static", siteId: "x", releaseId: "previous", mountPrefix: "/" } }] }),
      settings({ listeners: [httpListener], routes: [{ ...httpRoute, handler: { type: "reverseProxy", upstreams: [{ origin: "https://origin.example.test", extra: true }] } }] }),
    ];
    for (const candidate of cases) expect(accepts(candidate)).toBe(false);
  });

  it("rejects malformed binds, protocols, listener variants, and TLS combinations", () => {
    const listeners = [
      { ...httpListener, id: "" },
      { ...httpListener, address: "gateway.example.test:443" },
      { ...httpListener, address: ":0" },
      { ...httpListener, address: ":65536" },
      { ...httpListener, protocols: [] },
      { ...httpListener, protocols: ["http1", "http1"] },
      { ...httpListener, protocols: ["h2"] },
      { ...httpListener, tls: { mode: "unknown" } },
      { ...httpListener, hostnames: [], tls: { mode: "automatic" } },
      { ...httpListener, tls: { mode: "custom", certificateRef: "cert-ref" } },
      { ...httpListener, tls: { mode: "custom", certificateRef: "cert-ref", privateKeyRef: "key-ref", privateKey: "secret" } },
      { ...httpListener, tls: { mode: "automatic", dnsProvider: "provider-from-user-settings" } },
      { ...httpListener, hostnames: [], protocols: ["http1", "http2"], tls: { mode: "disabled" } },
      { ...httpListener, hostnames: [], protocols: ["http1", "http3"], tls: { mode: "disabled" } },
      { ...httpListener, tls: { mode: "disabled" }, redirectToListenerId: "" },
      { ...httpListener, redirectToListenerId: "tls-target" },
    ];
    for (const listener of listeners) expect(accepts(settings({ listeners: [listener], routes: [] }))).toBe(false);
  });

  it("rejects malformed matchers and unsupported glob syntax", () => {
    const matchers = [
      { hosts: [] },
      { hosts: ["**.example.test"] },
      { hosts: ["api.example.test:443"] },
      { methods: [] },
      { methods: ["GET /admin"] },
      { path: { type: "substring", value: "/a" } },
      { path: { type: "exact" } },
      { path: { type: "exact", value: "" } },
      { path: { type: "glob", value: "/assets/[ab]" } },
      { path: { type: "glob", value: "/assets/\\*" } },
      { path: { type: "regex", value: "" } },
    ];
    for (const match of matchers) expect(accepts(settings({ listeners: [httpListener], routes: [{ ...httpRoute, match }] }))).toBe(false);
  });

  it("rejects malformed route and handler variants and invalid service modes", () => {
    const cases = [
      { ...httpRoute, handler: { type: "static" } },
      { ...httpRoute, handler: { type: "redirect", location: "https://example.test" } },
      { ...httpRoute, handler: { type: "service", instanceId: "x", capability: "x", mode: "unknown" } },
      { ...httpRoute, handler: { type: "service", instanceId: "x", capability: "x", mode: "tcp" } },
      { ...httpRoute, handler: { type: "l4Relay", target: { host: "db.internal", port: 5432 } } },
    ];
    for (const route of cases) {
      expect(accepts(settings({ listeners: [httpListener], routes: [route] }))).toBe(false);
    }
  });

  it("rejects invalid pools, origins, weights, and HTTP caRef", () => {
    const invalidPool = (upstreams: unknown[]) => settings({
      listeners: [httpListener],
      routes: [{ ...httpRoute, handler: { type: "reverseProxy", upstreams } }],
    });
    const tooMany = Array.from({ length: 33 }, (_, index) => ({ origin: `https://origin-${index}.example.test` }));
    const cases = [
      invalidPool([]), invalidPool(tooMany),
      invalidPool([{ origin: "ftp://origin.example.test" }]),
      invalidPool([{ origin: "https://user:pass@origin.example.test" }]),
      invalidPool([{ origin: "https://origin.example.test/path" }]),
      invalidPool([{ origin: "https://origin.example.test?query=1" }]),
      invalidPool([{ origin: "http://origin.example.test", caRef: "private-ca" }]),
      invalidPool([{ origin: "https://origin.example.test", weight: 0 }]),
      invalidPool([{ origin: "https://origin.example.test", weight: 101 }]),
      invalidPool([{ origin: "https://origin.example.test", weight: 1.5 }]),
      invalidPool([{ weight: 1 }]),
    ];
    for (const candidate of cases) expect(accepts(candidate)).toBe(false);
  });
});

import { required } from '../support/models.js';
import { parseJSON } from '../support/json.js';
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

type Case = {
  name: string;
  payloadSiteId: string;
  manifest?: unknown;
  manifestJSON?: string;
  entries: Array<{ path: string; kind: "file" | "directory" | "symlink"; content?: string; target?: string }>;
};

function validCase(name: string, overrides: Partial<Case> = {}): Case {
  return {
    name,
    payloadSiteId: "docs",
    manifest: { schemaVersion: 1, siteId: "docs", documentRoot: "public", indexDocument: "index.html" },
    entries: [
      { path: "public", kind: "directory" },
      { path: "public/index.html", kind: "file", content: "index" },
    ],
    ...overrides,
  };
}

describe("site manifest validation against a staged release", () => {
  it("validates manifest identity, strict JSON, paths and resolved file types in production Go", () => {
    const cases: Case[] = [
      validCase("valid-subdirectory"),
      validCase("valid-root", {
        manifest: { schemaVersion: 1, siteId: "docs", documentRoot: ".", indexDocument: "index.html" },
        entries: [{ path: "index.html", kind: "file", content: "index" }],
      }),
      validCase("reject-site-id-mismatch", { manifest: { schemaVersion: 1, siteId: "other", documentRoot: "public", indexDocument: "index.html" } }),
      validCase("reject-unknown-field", { manifest: { schemaVersion: 1, siteId: "docs", documentRoot: "public", indexDocument: "index.html", extra: true } }),
      validCase("reject-unsupported-version", { manifest: { schemaVersion: 2, siteId: "docs", documentRoot: "public", indexDocument: "index.html" } }),
      validCase("reject-non-integer-version", { manifest: { schemaVersion: 1.5, siteId: "docs", documentRoot: "public", indexDocument: "index.html" } }),
      validCase("reject-duplicate-json-key", { manifest: undefined, manifestJSON: '{"schemaVersion":1,"siteId":"docs","siteId":"other","documentRoot":"public","indexDocument":"index.html"}' }),
      validCase("reject-malformed-json", { manifest: undefined, manifestJSON: '{"schemaVersion":1,"siteId":' }),
      validCase("reject-trailing-json", { manifest: undefined, manifestJSON: '{"schemaVersion":1,"siteId":"docs","documentRoot":"public","indexDocument":"index.html"} {}' }),
      validCase("reject-traversal-root", { manifest: { schemaVersion: 1, siteId: "docs", documentRoot: "../public", indexDocument: "index.html" } }),
      validCase("reject-dot-segment-root", { manifest: { schemaVersion: 1, siteId: "docs", documentRoot: "public/./nested", indexDocument: "index.html" } }),
      validCase("reject-empty-segment-root", { manifest: { schemaVersion: 1, siteId: "docs", documentRoot: "public//nested", indexDocument: "index.html" } }),
      validCase("reject-backslash-index", { manifest: { schemaVersion: 1, siteId: "docs", documentRoot: "public", indexDocument: "assets\\index.html" } }),
      validCase("reject-control-character", { manifest: { schemaVersion: 1, siteId: "docs", documentRoot: "public", indexDocument: "bad\u007f.html" } }),
      validCase("reject-unicode-normalization-mismatch", { payloadSiteId: "café", manifest: { schemaVersion: 1, siteId: "cafe\u0301", documentRoot: "public", indexDocument: "index.html" } }),
      validCase("reject-manifest-as-root-index", { manifest: { schemaVersion: 1, siteId: "docs", documentRoot: ".", indexDocument: "site-manifest.json" }, entries: [] }),
      validCase("reject-missing-manifest", { manifest: undefined, entries: [{ path: "public", kind: "directory" }, { path: "public/index.html", kind: "file" }] }),
      validCase("reject-missing-index", { entries: [{ path: "public", kind: "directory" }] }),
      validCase("reject-file-as-root", { entries: [{ path: "public", kind: "file" }] }),
      validCase("reject-symlinked-root", { entries: [{ path: "public", kind: "symlink", target: "elsewhere" }, { path: "elsewhere/index.html", kind: "file" }] }),
      validCase("reject-symlinked-index", { entries: [{ path: "public", kind: "directory" }, { path: "public/index.html", kind: "symlink", target: "elsewhere" }, { path: "public/elsewhere", kind: "file" }] }),
    ];

    const output = execFileSync("go", ["run", "./tests/fixtures/site-manifest"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      input: JSON.stringify({ cases }),
      encoding: "utf8",
    });
    const results = parseJSON<{ name: string; accepted: boolean; manifest?: unknown }[]>(output);

    expect(results).toHaveLength(cases.length);
    expect(required(results.find((result) => result.name === "valid-subdirectory"))).toMatchObject({
      accepted: true,
      manifest: { schemaVersion: 1, siteId: "docs", documentRoot: "public", indexDocument: "index.html" },
    });
    expect(required(results.find((result) => result.name === "valid-root"))).toMatchObject({
      accepted: true,
      manifest: { documentRoot: ".", indexDocument: "index.html" },
    });
    for (const candidate of cases.filter((item) => item.name.startsWith("reject-"))) {
      expect(required(results.find((result) => result.name === candidate.name))?.accepted, candidate.name).toBe(false);
    }
  }, 60_000);
});

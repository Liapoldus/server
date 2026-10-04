import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import Ajv2020 from "ajv/dist/2020.js";
import { describe, expect, it } from "vitest";

const root = fileURLToPath(new URL("..", import.meta.url));

async function json(path: string): Promise<any> {
  return JSON.parse(await readFile(path, "utf8"));
}

describe("Caddy site archive manifest v1", () => {
  it("publishes the strict manifest schema and archive linkage", async () => {
    const link = await json(`${root}/contracts/v1/site-publish-manifest.json`);
    const schema = await json(`${root}/contracts/v1/site-manifest.schema.json`);
    const semantics = await json(`${root}/contracts/v1/site-manifest-semantics.json`);
    const ajv = new Ajv2020({ allErrors: true, strict: false });
    const validate = ajv.compile(schema);

    expect(link).toEqual({
      version: 1,
      plugin: "server",
      publishCapability: "server.sites.publish",
      rollbackCapability: "server.sites.rollback",
      archiveEntry: "site-manifest.json",
      schema: "contracts/v1/site-manifest.schema.json",
      semantics: "contracts/v1/site-manifest-semantics.json",
      vectors: "contracts/v1/site-manifest-vectors.json",
    });
    expect(semantics).toMatchObject({
      version: 1,
      archiveEntry: { path: "site-manifest.json", location: "exactly-at-tar-root", count: 1, type: "regular-file" },
      archiveValidation: {
        manifestAsIndexDocumentWhenDocumentRootIsDot: "reject-with-reserved-manifest-as-index",
        currentPreviousOnAnyRejection: "unchanged",
      },
      fields: {
        siteId: "must-equal-the-UTF-8-byte-sequence-of-the-decoded-publish-metadata-payload-siteId-string-without-case-folding-or-Unicode-normalization",
        documentRoot: { dot: "the archive root", reject: expect.arrayContaining(["absolute-path", "backslash", "parent-segment"]) },
        indexDocument: { format: "POSIX relative regular-file path using forward-slash separators, relative to documentRoot" },
      },
      staticServing: {
        rootRequest: "resolve-documentRoot-then-append-indexDocument",
        directoryRequest: "resolve-requested-directory-under-documentRoot-then-append-indexDocument",
        spaFallback: false,
        manifestExposure: "site-manifest.json-is-never-served-as-public-static-content-even-when-documentRoot-is-dot",
        publicNamespace: "exclude-the-archive-root-site-manifest.json-before-static-path-resolution",
      },
      releaseLifecycle: "successful-publish-or-rollback-atomically-controls-current-previous",
    });

    const valid = { schemaVersion: 1, siteId: "docs", documentRoot: "public", indexDocument: "index.html" };
    expect(validate(valid)).toBe(true);
    expect(validate({ ...valid, documentRoot: "." })).toBe(true);
    for (const invalid of [
      { ...valid, schemaVersion: 2 },
      { ...valid, documentRoot: "../outside" },
      { ...valid, documentRoot: "/public" },
      { ...valid, documentRoot: "public\\assets" },
      { ...valid, indexDocument: "../outside.html" },
      { ...valid, indexDocument: "/index.html" },
      { ...valid, indexDocument: "public\\index.html" },
      { ...valid, documentRoot: ".", indexDocument: "site-manifest.json" },
      { ...valid, documentRoot: "bad\0root" },
      { ...valid, indexDocument: "bad\u001findex.html" },
      { ...valid, secretRef: "must-not-be-accepted" },
    ]) {
      expect(validate(invalid)).toBe(false);
    }
  });

  it("declares publish acceptance and rejection vectors for archive and static-root semantics", async () => {
    const vectors = await json(`${root}/contracts/v1/site-manifest-vectors.json`);
    const schema = await json(`${root}/contracts/v1/site-manifest.schema.json`);
    const validate = new Ajv2020({ allErrors: true, strict: false }).compile(schema);
    const ids = vectors.scenarios.map((scenario: any) => scenario.id);

    expect(ids).toEqual(expect.arrayContaining([
      "site-manifest-valid-archive-root",
      "site-manifest-valid-subdirectory-root",
      "site-manifest-rejects-site-id-mismatch",
      "site-manifest-rejects-site-id-unicode-normalization-mismatch",
      "site-manifest-rejects-missing-entry",
      "site-manifest-rejects-nested-entry",
      "site-manifest-rejects-duplicate-entry",
      "site-manifest-rejects-unsupported-version",
      "site-manifest-rejects-missing-index-document",
      "site-manifest-rejects-traversal-root",
      "site-manifest-rejects-absolute-root",
      "site-manifest-rejects-backslash-root",
      "site-manifest-rejects-traversal-index",
      "site-manifest-rejects-absolute-index",
      "site-manifest-rejects-backslash-index",
      "site-manifest-rejects-control-root",
      "site-manifest-rejects-control-index",
      "site-manifest-rejects-delete-index",
      "site-manifest-rejects-reserved-index-document",
      "site-manifest-rejects-symlink-root",
      "site-manifest-rejects-symlink-index",
      "site-manifest-never-served",
    ]));
    expect(vectors.scenarios).toHaveLength(ids.length);
    const evaluate = (input: any): { accepted: boolean; reason?: string; publicFiles?: string[]; responses?: Record<string, number> } => {
      const entries = input.entries ?? [
        { path: "site-manifest.json", type: "regular-file" },
        { path: "index.html", type: "regular-file" },
        { path: "dist/index.html", type: "regular-file" },
      ];
      const manifestEntries = entries.filter((entry: any) => entry.path === "site-manifest.json");
      if (manifestEntries.length === 0) return { accepted: false, reason: "manifest-missing" };
      if (manifestEntries.length !== 1) return { accepted: false, reason: "manifest-duplicate" };
      if (manifestEntries[0].type !== "regular-file") return { accepted: false, reason: "manifest-not-regular-file" };
      if (!input.manifest) return { accepted: false, reason: "manifest-missing" };
      if (!validate(input.manifest)) {
        if (input.manifest.schemaVersion !== 1) return { accepted: false, reason: "unsupported-schema-version" };
        if (input.manifest.documentRoot === "." && input.manifest.indexDocument === "site-manifest.json") return { accepted: false, reason: "reserved-manifest-as-index" };
        if (validate.errors?.some((error: any) => error.instancePath === "/documentRoot")) return { accepted: false, reason: "unsafe-document-root" };
        if (validate.errors?.some((error: any) => error.instancePath === "/indexDocument")) return { accepted: false, reason: "unsafe-index-document" };
        return { accepted: false, reason: "invalid-manifest" };
      }
      if (input.payloadSiteId !== undefined && input.manifest.siteId !== input.payloadSiteId) return { accepted: false, reason: "site-id-mismatch" };
      const root = input.manifest.documentRoot === "." ? "" : `${input.manifest.documentRoot}/`;
      const rootEntry = input.manifest.documentRoot === "." ? undefined : entries.find((entry: any) => entry.path === input.manifest.documentRoot);
      if (rootEntry && rootEntry.type !== "directory") return { accepted: false, reason: "document-root-resolves-through-link" };
      const indexPath = `${root}${input.manifest.indexDocument}`;
      const indexEntry = entries.find((entry: any) => entry.path === indexPath);
      if (!indexEntry) return { accepted: false, reason: "index-document-not-found-as-regular-file" };
      if (indexEntry.type !== "regular-file") return { accepted: false, reason: "index-document-is-not-regular-file" };
      const publicPrefix = input.manifest.documentRoot === "." ? "" : `${input.manifest.documentRoot}/`;
      const publicFiles = entries
        .filter((entry: any) => entry.type === "regular-file" && entry.path.startsWith(publicPrefix) && entry.path !== "site-manifest.json")
        .map((entry: any) => entry.path.slice(publicPrefix.length));
      const responses: Record<string, number> = {};
      const resolved: Record<string, string> = {};
      for (const requestPath of input.requests ?? []) {
        const manifestRequest = input.manifest.documentRoot === "." && requestPath === "/site-manifest.json";
        const requestRelativePath = requestPath.replace(/^\//, "");
        const staticRelativePath = requestPath === "/"
          ? input.manifest.indexDocument
          : requestPath.endsWith("/")
            ? `${requestRelativePath}${input.manifest.indexDocument}`
            : requestRelativePath;
        const archivePath = `${root}${staticRelativePath}`;
        const entry = entries.find((candidate: any) => candidate.path === archivePath);
        const served = !manifestRequest && archivePath !== "site-manifest.json" && entry?.type === "regular-file" && publicFiles.includes(staticRelativePath);
        responses[requestPath] = served ? 200 : 404;
        if (served) resolved[requestPath] = archivePath;
      }
      return { accepted: true, publicFiles, responses, resolved };
    };

    for (const scenario of vectors.scenarios) {
      const actual = evaluate(scenario.input);
      expect(actual.accepted, scenario.id).toBe(scenario.expected.accepted);
      if (scenario.expected.reason) expect(actual.reason, scenario.id).toBe(scenario.expected.reason);
      if (!actual.accepted) expect(scenario.expected.currentPrevious, scenario.id).toBe("unchanged");
      if (scenario.expected.responses) expect(actual.responses, scenario.id).toEqual(scenario.expected.responses);
      if (scenario.expected.resolved) expect(actual.resolved, scenario.id).toEqual(scenario.expected.resolved);
      if (scenario.expected.publicFiles) expect(actual.publicFiles, scenario.id).toEqual(scenario.expected.publicFiles);
    }
    expect(vectors.scenarios.find((scenario: any) => scenario.id === "site-manifest-never-served").expected)
      .toMatchObject({ accepted: true, responses: { "/site-manifest.json": 404 }, httpStatus: 404, currentPrevious: "unchanged" });
    const archiveRoot = vectors.scenarios.find((scenario: any) => scenario.id === "site-manifest-valid-archive-root");
    const rootEvaluation = evaluate(archiveRoot.input);
    expect(rootEvaluation.publicFiles).not.toContain("site-manifest.json");
    expect(rootEvaluation.responses).toMatchObject({ "/": 200, "/site-manifest.json": 404 });
  });
});

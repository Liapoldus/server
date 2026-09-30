import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import Ajv2020 from "ajv/dist/2020.js";
import { describe, expect, it } from "vitest";

const root = fileURLToPath(new URL("..", import.meta.url));
const protocolRoot = fileURLToPath(new URL("../../../pluginprotocol", import.meta.url));

async function json(path: string): Promise<any> {
  return JSON.parse(await readFile(path, "utf8"));
}

describe("Server plugin-owned Admin Surface v1", () => {
  it("publishes a strict surface fixture and action catalog through the plugin contract", async () => {
    const plugin = await json(`${root}/contracts/v1/plugin.json`);
    const schema = await json(`${root}/contracts/v1/admin-surface.schema.json`);
    const surface = await json(`${root}/contracts/v1/admin-surface.json`);
    const actionsSchema = await json(`${root}/contracts/v1/admin-actions.schema.json`);
    const actions = await json(`${root}/contracts/v1/admin-actions.json`);
    const generic = await json(`${protocolRoot}/contracts/admin-ui/v1/schema.json`);
    const ajv = new Ajv2020({ allErrors: true, strict: false });

    expect(plugin.adminSurface).toMatchObject({
      version: 1,
      descriptor: "contracts/v1/admin-surface.json",
      schema: "contracts/v1/admin-surface.schema.json",
      actions: "contracts/v1/admin-actions.json",
    });
    expect(ajv.compile(schema)(surface)).toBe(true);
    expect(ajv.compile(actionsSchema)(actions)).toBe(true);
    expect(surface.requiredCapabilities).toContain("admin.surface.get");
    expect(Object.keys(actions.operations).sort()).toEqual(surface.requiredCapabilities.filter((capability: string) => capability !== "admin.surface.get").sort());
    const pageProperties = new Set(Object.keys(generic.page.properties));
    const sectionProperties = new Set(Object.keys(generic.section.properties));
    const actionProperties = new Set(Object.keys(generic.action.properties));
    for (const page of surface.pages) {
      expect(Object.keys(page).every((key) => pageProperties.has(key))).toBe(true);
      for (const section of page.sections) {
        expect(generic.section.kinds).toContain(section.kind);
        expect(Object.keys(section).every((key) => key === "kind" || sectionProperties.has(key))).toBe(true);
        for (const action of section.actions ?? []) expect(Object.keys(action).every((key) => actionProperties.has(key))).toBe(true);
      }
    }
  });

  it("keeps common settings lifecycle outside the product Admin Surface", async () => {
    const surface = await json(`${root}/contracts/v1/admin-surface.json`);
    const settingsPage = surface.pages.find((page: any) => page.id === "settings");

    expect(settingsPage).toBeUndefined();
    expect(JSON.stringify(surface)).not.toMatch(/ConfigSchema|ConfigApply|settingsSchemaRpc|settingsApplyRpc/);
  });

  it("declares every UI capability and validates each action input and row binding explicitly", async () => {
    const surface = await json(`${root}/contracts/v1/admin-surface.json`);
    const actions = await json(`${root}/contracts/v1/admin-actions.json`);
    const protocolAdmin = await json(`${protocolRoot}/contracts/admin-ui/v1/schema.json`);
    const properties = protocolAdmin.action.properties;
    const allowedKeywords = new Set<string>(properties.inputSchema.allowedKeywords);
    const allowedTypes = new Set<string>(properties.inputSchema.allowedTypeValues);

    const validInputSchema = (value: unknown, topLevel = true): boolean => {
      if (value === null || typeof value !== "object" || Array.isArray(value)) return false;
      const candidate = value as Record<string, unknown>;
      if (Object.keys(candidate).some((key) => !allowedKeywords.has(key))) return false;
      if (topLevel && properties.inputSchema.requiredTopLevelKeywords.some((key: string) => !Object.hasOwn(candidate, key))) return false;
      if (topLevel && (candidate.type !== "object" || candidate.additionalProperties !== false || !Array.isArray(candidate.required) || candidate.properties === null || typeof candidate.properties !== "object" || Array.isArray(candidate.properties))) return false;
      if (typeof candidate.type !== "string" || !allowedTypes.has(candidate.type)) return false;
      if (candidate.type === "object") {
        if (candidate.additionalProperties !== false || candidate.properties === null || typeof candidate.properties !== "object" || Array.isArray(candidate.properties)) return false;
        const fields = candidate.properties as Record<string, unknown>;
        if (Array.isArray(candidate.required) && (new Set(candidate.required).size !== candidate.required.length || candidate.required.some((key) => typeof key !== "string" || !Object.hasOwn(fields, key)))) return false;
        return Object.values(fields).every((field) => validInputSchema(field, false));
      }
      if (candidate.properties !== undefined || candidate.required !== undefined || candidate.additionalProperties !== undefined) return false;
      if ((candidate.minLength !== undefined || candidate.maxLength !== undefined) && candidate.type !== "string") return false;
      if ((candidate.minimum !== undefined || candidate.maximum !== undefined) && candidate.type !== "number" && candidate.type !== "integer") return false;
      if (candidate.enum !== undefined && (!Array.isArray(candidate.enum) || candidate.enum.length === 0)) return false;
      return true;
    };

    const refs = new Set<string>(["admin.surface.get"]);
    for (const page of surface.pages) {
      if (page.capability) refs.add(page.capability);
      for (const section of page.sections) {
        if (section.dataCapability) refs.add(section.dataCapability);
        if (section.kind !== "table") continue;
        const columns = new Set<string>(section.columns);
        for (const action of section.actions) {
          refs.add(action.capability);
          expect(validInputSchema(action.inputSchema)).toBe(true);
          expect(actions.operations[action.capability].surfaceBinding).toEqual({ page: page.id, section: section.id, action: action.id });
          if (action.rowInput) {
            const inputKeys = new Set(Object.keys(action.inputSchema.properties));
            for (const [property, column] of Object.entries(action.rowInput)) {
              expect(inputKeys.has(property)).toBe(true);
              expect(columns.has(column as string)).toBe(true);
            }
          }
        }
      }
    }
    expect([...refs].sort()).toEqual([...surface.requiredCapabilities].sort());

    const publish = surface.pages.find((page: any) => page.id === "sites").sections[0].actions.find((action: any) => action.id === "publish");
    expect(validInputSchema({ ...publish.inputSchema, properties: { siteId: { type: "string", pattern: "^forbidden$" } } })).toBe(false);
    expect(validInputSchema({ ...publish.inputSchema, required: ["undeclared"] })).toBe(false);
  });

  it("validates all declared query and response schemas with no open object payloads", async () => {
    const catalog = await json(`${root}/contracts/v1/admin-actions.json`);
    const ajv = new Ajv2020({ allErrors: true, strict: false });
    const validateAllObjectSchemas = (value: unknown): boolean => {
      if (Array.isArray(value)) return value.every(validateAllObjectSchemas);
      if (value === null || typeof value !== "object") return true;
      const candidate = value as Record<string, unknown>;
      if (candidate.type === "object" && candidate.additionalProperties !== false) return false;
      return Object.values(candidate).every(validateAllObjectSchemas);
    };

    for (const operation of Object.values(catalog.operations) as any[]) {
      if (operation.requestSchema) expect(() => ajv.compile(operation.requestSchema)).not.toThrow();
      if (operation.responseSchema) expect(() => ajv.compile(operation.responseSchema)).not.toThrow();
      if (operation.requestSchema) expect(validateAllObjectSchemas(operation.requestSchema)).toBe(true);
      if (operation.responseSchema) expect(validateAllObjectSchemas(operation.responseSchema)).toBe(true);
    }
    expect(catalog.operations["server.sites.publish"].receiptSchema).toContain("artifact-operation-result.schema.json");

    const sitesList = catalog.operations["server.sites.list"];
    expect(ajv.compile(sitesList.requestSchema)({ limit: 100 })).toBe(true);
    expect(ajv.compile(sitesList.requestSchema)({ limit: 101 })).toBe(false);
    expect(ajv.compile(sitesList.requestSchema)({ path: "/tmp" })).toBe(false);

    const certList = catalog.operations["server.certificates.list"];
    const validateCertificates = ajv.compile(certList.responseSchema);
    expect(validateCertificates({ items: [{ domain: "api.example.test", source: "acme", readiness: "ready", notAfter: "2030-01-01T00:00:00Z", serial: "0a" }], nextCursor: null })).toBe(true);
    expect(validateCertificates({ items: [{ domain: "api.example.test", source: "acme", readiness: "secret", notAfter: null, serial: null }], nextCursor: null })).toBe(false);
    expect(validateCertificates({ items: [{ domain: "api.example.test", source: "acme", readiness: "ready", notAfter: null, serial: "0a", privateKey: "forbidden" }], nextCursor: null })).toBe(false);

    const operationGet = catalog.operations["server.operations.get"];
    expect(ajv.compile(operationGet.responseSchema)({ operationId: "op-1", siteId: "docs", kind: "publish", state: "running", progressPercent: 35, updatedAt: "2030-01-01T00:00:00Z", errorCode: null })).toBe(true);
    expect(ajv.compile(operationGet.responseSchema)({ operationId: "op-1", siteId: "docs", kind: "publish", state: "running", progressPercent: 35, updatedAt: "2030-01-01T00:00:00Z", errorCode: null, secret: "must-not-be-exposed" })).toBe(false);
  });

  it("declares site list, release status, publish, rollback, and operation progress actions", async () => {
    const surface = await json(`${root}/contracts/v1/admin-surface.json`);
    const actions = await json(`${root}/contracts/v1/admin-actions.json`);
    const sitesPage = surface.pages.find((page: any) => page.id === "sites");
    const siteTable = sitesPage.sections.find((section: any) => section.id === "sites");
    const releaseTable = sitesPage.sections.find((section: any) => section.id === "releases");
    const operationTable = sitesPage.sections.find((section: any) => section.id === "operations");
    const publish = siteTable.actions.find((action: any) => action.id === "publish");
    const rollback = siteTable.actions.find((action: any) => action.id === "rollback");

    expect(siteTable.dataCapability).toBe("server.sites.list");
    expect(publish).toMatchObject({ capability: "server.sites.publish", rowInput: { siteId: "siteId" } });
    expect(publish.artifactInput).toEqual({
      mediaTypes: ["application/gzip"],
      maxBytes: 134217728,
      maxMetadataBytes: 65536,
      maxMultipartOverheadBytes: 65536,
    });
    expect(rollback).toMatchObject({
      capability: "server.sites.rollback",
      rowInput: {
        siteId: "siteId",
        expectedCurrentRevision: "currentRevision",
        targetRevision: "previousRevision",
      },
    });
    expect(releaseTable.dataCapability).toBe("server.sites.releases.list");
    expect(operationTable.dataCapability).toBe("server.operations.list");
    expect(operationTable.actions).toContainEqual(expect.objectContaining({
      capability: "server.operations.get",
      rowInput: { operationId: "operationId" },
    }));
    expect(actions.operations["server.sites.publish"]).toMatchObject({
      transport: "pluginprotocol.v1.artifact-stream",
      multipart: { parts: ["metadata", "artifact"], artifactFilenameForwarded: false },
      acceptedHttpStatus: 202,
      receiptSchema: "pluginprotocol/contracts/protocol/v1/artifact-operation-result.schema.json",
    });
    expect(actions.operations["server.sites.publish"].archiveLimits).toEqual(actions.archive);
    expect(actions.archive).toMatchObject({
      minArtifactBytes: 1,
      artifactBytes: 134217728,
      metadataBytes: 65536,
      multipartOverheadBytes: 65536,
      requestEnvelopeBytes: 134348800,
      expandedBytes: 536870912,
      maxFiles: 10000,
      maxCompressionRatio: 100,
      entryCountSemantics: {
        unit: "logical-tar-member-after-extension-processing",
        countedKinds: ["regular-file", "directory"],
        implicitParentDirectories: "not-counted",
      },
      gzip: {
        members: "concatenated-members-allowed",
        integrity: "drain-all-members-through-eof",
        trailingCompressedBytes: "reject-unless-part-of-valid-gzip-member",
      },
      tar: {
        archiveCount: "one-tar-stream",
        terminator: "two-consecutive-512-byte-zero-blocks",
        postTerminatorBytes: "zero-padding-only",
      },
    });
    const archiveVectors = await json(`${root}/contracts/v1/admin-surface-vectors.json`);
    expect(archiveVectors.scenarios).toContainEqual(expect.objectContaining({
      id: "admin-publish-counts-explicit-directories-toward-entry-limit",
      input: { regularFileEntries: 2, explicitDirectoryEntries: 9999, implicitParentDirectories: 0 },
      expected: { effectiveTarEntries: 10001, plugin: "reject-before-durable-acceptance" },
    }));
    expect(actions.releaseLifecycle).toMatchObject({
      storage: "plugin-owned-persistent-filesystem",
      activation: "atomic-current-previous-pointer-swap-only-after-successful-terminal-operation",
      failure: "keep-current-and-previous-unchanged",
    });
    expect(actions.operationStatus).toMatchObject({
      pluginStatusCapability: "server.operations.get",
      coreResource: "GET /api/operations/{id}",
      coreControlUsesPluginSdkRest: true,
      stateMapping: { accepted: "pending", running: "running", completed: "succeeded", failed: "failed" },
    });
    expect(actions.operationStatus.coreStates).toContain("degraded");
    expect(actions.operations["server.operations.get"].ownership).toBe("plugin-admin-surface-action");
  });

  it("declares per-domain certificate readiness and guarded renew/revoke actions", async () => {
    const surface = await json(`${root}/contracts/v1/admin-surface.json`);
    const actions = await json(`${root}/contracts/v1/admin-actions.json`);
    const page = surface.pages.find((candidate: any) => candidate.id === "certificates");
    const table = page.sections.find((section: any) => section.id === "domains");

    expect(table.dataCapability).toBe("server.certificates.list");
    expect(table.columns).toEqual(expect.arrayContaining(["domain", "source", "readiness", "serial"]));
    expect(table.actions).toEqual(expect.arrayContaining([
      expect.objectContaining({ capability: "server.certificates.get", rowInput: { domain: "domain" } }),
      expect.objectContaining({ capability: "server.certificates.renew", rowInput: { domain: "domain" } }),
      expect.objectContaining({ capability: "server.certificates.revoke", rowInput: { domain: "domain", serial: "serial" } }),
    ]));
    expect(actions.operations["server.certificates.renew"].acceptedHttpStatus).toBe(202);
    expect(actions.operations["server.certificates.revoke"].acceptedHttpStatus).toBe(202);
    expect(actions.operations["server.certificates.revoke"].preconditions).toEqual([
      "certificate-is-caddy-managed",
      "serial-matches-current-certificate-for-domain",
      "non-empty-reason",
    ]);
    expect(actions.operations["server.certificates.list"].responseSchema.$defs.certificateSummary.properties.readiness.enum)
      .toEqual(["pending", "ready", "failed", "unknown"]);
    const revokeAction = table.actions.find((action: any) => action.capability === "server.certificates.revoke");
    expect(revokeAction.inputSchema.required).toEqual(["domain", "serial", "reason"]);
    expect(revokeAction.inputSchema.properties.reason.minLength).toBe(1);
  });

  it("aligns artifact action inputs and archive safety limits with generic protocol v1", async () => {
    const surface = await json(`${root}/contracts/v1/admin-surface.json`);
    const actionSchema = await json(`${root}/contracts/v1/admin-surface.schema.json`);
    const protocolAdmin = await json(`${protocolRoot}/contracts/admin-ui/v1/schema.json`);
    const artifactTransport = await json(`${protocolRoot}/contracts/protocol/v1/artifact-stream.json`);
    const metadataSchema = await json(`${protocolRoot}/contracts/protocol/v1/artifact-metadata.schema.json`);
    const receiptSchema = await json(`${protocolRoot}/contracts/protocol/v1/artifact-operation-result.schema.json`);
    const sites = surface.pages.find((page: any) => page.id === "sites");
    const publish = sites.sections.find((section: any) => section.id === "sites").actions.find((action: any) => action.id === "publish");
    const protocolAction = protocolAdmin.action.properties;
    const ajv = new Ajv2020({ allErrors: true, strict: false });
    const validateSurface = ajv.compile(actionSchema);
    const validateInputSchema = (candidate: unknown) => {
      const wrapper = { ...publish, inputSchema: candidate };
      return validateSurface({ ...surface, pages: surface.pages.map((page: any) => ({
        ...page,
        sections: page.sections.map((section: any) => ({
          ...section,
          actions: section.actions?.map((action: any) => action.id === "publish" ? wrapper : action),
        })),
      })) });
    };

    expect(protocolAction.inputSchema).toBeDefined();
    expect(protocolAction.artifactInput.required).toEqual(["mediaTypes", "maxBytes", "maxMetadataBytes", "maxMultipartOverheadBytes"]);
    expect(publish.artifactInput.maxBytes).toBeLessThanOrEqual(artifactTransport.limits.artifactBytes);
    expect(publish.artifactInput.maxMetadataBytes).toBeLessThanOrEqual(artifactTransport.limits.metadataBytes);
    expect(publish.artifactInput.maxMultipartOverheadBytes).toBeLessThanOrEqual(artifactTransport.limits.multipartHeadersAndFramingBytes);
    expect(artifactTransport.limits.multipartEnvelopeBytes).toBe(134348800);
    const catalog = await json(`${root}/contracts/v1/admin-actions.json`);
    expect(catalog.archive.artifactBytes).toBe(artifactTransport.limits.artifactBytes);
    expect(catalog.archive.metadataBytes).toBe(artifactTransport.limits.metadataBytes);
    expect(catalog.archive.multipartOverheadBytes).toBe(artifactTransport.limits.multipartHeadersAndFramingBytes);
    expect(catalog.archive.requestEnvelopeBytes).toBe(artifactTransport.limits.multipartEnvelopeBytes);
    expect(catalog.operations["server.sites.publish"].archiveLimits).toEqual(catalog.archive);
    expect(metadataSchema.properties.payload.description).toContain("capability validates its own schema");
    expect(ajv.compile(metadataSchema)({
      version: 1,
      artifact: { mediaType: "application/gzip", byteLength: 4, sha256: `sha256:${"a".repeat(64)}` },
      payload: { siteId: "portal" },
    })).toBe(true);
    expect(ajv.compile(receiptSchema)({ version: 1, operationId: "operation-1", state: "accepted" })).toBe(true);
    expect(ajv.compile(receiptSchema)({ version: 1, operationId: "operation-1", state: "failed" })).toBe(false);
    expect(ajv.compile(publish.inputSchema)({ siteId: "portal" })).toBe(true);
    expect(ajv.compile(publish.inputSchema)({ siteId: "portal", unexpected: true })).toBe(false);
    const validatePayload = ajv.compile(publish.inputSchema);
    expect(validatePayload({ siteId: "portal", unlisted: "value" })).toBe(false);
    const validateMetadata = ajv.compile(metadataSchema);
    expect(validateMetadata({ version: 1, artifact: { mediaType: "application/gzip", byteLength: 4, sha256: `sha256:${"a".repeat(64)}` }, payload: { siteId: "portal" } })).toBe(true);
    expect(validatePayload({ siteId: "portal" })).toBe(true);
    expect(validateInputSchema({
      type: "object",
      properties: { siteId: { type: "string", pattern: "^bad$" } },
      required: ["siteId"],
      additionalProperties: false,
    })).toBe(false);
    const invalidArtifactSurfaces = [
      { ...publish.artifactInput, maxBytes: 134217729 },
      { ...publish.artifactInput, maxMetadataBytes: 65537 },
      { ...publish.artifactInput, mediaTypes: ["application/gzip", "application/gzip"] },
      { ...publish.artifactInput, mediaTypes: ["Application/Gzip"] },
    ];
    for (const artifactInput of invalidArtifactSurfaces) {
      const candidate = { ...publish, artifactInput };
      expect(validateSurface({ ...surface, pages: surface.pages.map((page: any) => ({
        ...page,
        sections: page.sections.map((section: any) => ({
          ...section,
          actions: section.actions?.map((action: any) => action.id === "publish" ? candidate : action),
        })),
      })) })).toBe(false);
    }
  });

  it("uses only the generic plugin admin API and declares negative vectors for malformed contracts", async () => {
    const protocolAdmin = await json(`${protocolRoot}/contracts/admin-ui/v1/schema.json`);
    const vectors = await json(`${root}/contracts/v1/admin-surface-vectors.json`);
    const serialized = JSON.stringify(await json(`${root}/contracts/v1/admin-surface.json`));

    expect(protocolAdmin.coreApi.base).toBe("/api/plugins/{instance}/admin");
    expect(serialized).not.toMatch(/\/api\/(?:caddy|sites|certificates)(?:\/|"|$)/);
    expect(vectors.scenarios.map((scenario: any) => scenario.id)).toEqual(expect.arrayContaining([
      "admin-surface-valid",
      "admin-surface-unknown-field",
      "admin-publish-rejects-extra-part",
      "admin-publish-rejects-oversized-artifact",
      "admin-publish-rejects-digest-mismatch",
      "admin-publish-rejects-empty-artifact",
      "admin-publish-rejects-expanded-size-over-limit",
      "admin-publish-rejects-file-count-over-limit",
      "admin-publish-rejects-compression-ratio-over-limit",
      "admin-publish-allows-concatenated-gzip-members",
      "admin-publish-rejects-non-gzip-trailing-bytes",
      "admin-publish-allows-zero-tar-padding",
      "admin-publish-rejects-nonzero-tar-trailing-data",
      "admin-publish-rejects-unsafe-archive-entry",
      "admin-publish-rejects-duplicate-path",
      "admin-publish-rejects-case-collision",
      "admin-publish-rejects-unicode-nfc-collision",
      "admin-publish-rejects-link-entry",
      "admin-publish-reuses-idempotent-operation",
      "admin-revoke-rejects-custom-certificate",
      "admin-common-settings-live-in-core",
    ]));
  });
});

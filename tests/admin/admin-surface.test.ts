import { required } from '../support/models.js';
import { readContract } from '../support/contracts.js';
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { Ajv2020 } from "ajv/dist/2020.js";
import { describe, expect, it } from "vitest";

const root = fileURLToPath(new URL("../..", import.meta.url));


describe("Server plugin-owned Admin Surface v1", () => {
	it("treats synchronous admin-action idempotency keys as correlation only", async () => {
		const actions = readContract('admin-actions.json');
		const rollback = actions.operations["server.sites.rollback"];

		expect(rollback.idempotency).toMatchObject({
			required: true,
			semantics: "correlation-only",
			repeat: "invoke-again",
		});
		expect(rollback.idempotency).not.toHaveProperty("sameInput", "return-the-existing-completed-result");
		expect(actions.operations["server.sites.publish"].idempotency.sameInput).toBe("return-existing-operation-without-duplicate-effect");
	});

	it("leaves certificate renewal automatic and exposes no manual renew or revoke actions", async () => {
		const surface = readContract('admin-surface.json');
		const actions = readContract('admin-actions.json');

		expect(surface.requiredCapabilities).not.toContain("server.certificates.renew");
		expect(surface.requiredCapabilities).not.toContain("server.certificates.revoke");
		expect(actions.operations).not.toHaveProperty("server.certificates.renew");
		expect(actions.operations).not.toHaveProperty("server.certificates.revoke");
		expect(surface.requiredCapabilities).toContain("server.certificates.list");
		expect(surface.requiredCapabilities).toContain("server.certificates.get");
	});

	it("publishes a strict surface fixture and action catalog through the plugin contract", async () => {
    const plugin = readContract('plugin.json');
    const schema = readContract('admin-surface.schema.json');
    const surface = readContract('admin-surface.json');
    const actionsSchema = readContract('admin-actions.schema.json');
    const actions = readContract('admin-actions.json');
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
    const pageProperties = new Set(Object.keys(schema.$defs.page.properties));
    const sectionProperties = new Set(Object.keys({ ...schema.$defs.tableSection.properties, ...schema.$defs.detailSection.properties }));
    const actionProperties = new Set(Object.keys(schema.$defs.action.properties));
    for (const page of surface.pages) {
      expect(Object.keys(page).every((key) => pageProperties.has(key))).toBe(true);
      for (const section of page.sections) {
        expect([schema.$defs.tableSection.properties.kind.const, schema.$defs.detailSection.properties.kind.const]).toContain(section.kind);
        expect(Object.keys(section).every((key) => key === "kind" || sectionProperties.has(key))).toBe(true);
        for (const action of section.actions ?? []) expect(Object.keys(action).every((key) => actionProperties.has(key))).toBe(true);
      }
    }
  });

  it("keeps common settings lifecycle outside the product Admin Surface", async () => {
    const surface = readContract('admin-surface.json');
    const schema = readContract('admin-surface.schema.json');
    const settingsPage = surface.pages.find((page) => page.id === "settings");

    expect(settingsPage).toBeUndefined();
    expect(JSON.stringify(surface)).not.toMatch(/ConfigSchema|ConfigApply|settingsSchemaRpc|settingsApplyRpc/);
    expect(JSON.stringify(schema)).not.toMatch(/ConfigSchema|ConfigApply|fieldsFromControlRpc/);
  });

  it("declares every UI capability and validates each action input and row binding explicitly", async () => {
    const surface = readContract('admin-surface.json');
    const actions = readContract('admin-actions.json');
    const surfaceSchema = readContract('admin-surface.schema.json');
    const allowedKeywords = new Set<string>([
      ...Object.keys(surfaceSchema.$defs.inputField.properties),
      ...Object.keys(surfaceSchema.$defs.actionInputSchema.properties),
    ]);
    const allowedTypes = new Set<string>(["object", ...surfaceSchema.$defs.inputField.properties.type.enum]);

    const validInputSchema = (value: unknown, topLevel = true): boolean => {
      if (value === null || typeof value !== "object" || Array.isArray(value)) return false;
      const candidate = value as Record<string, unknown>;
      if (Object.keys(candidate).some((key) => !allowedKeywords.has(key))) return false;
      if (topLevel && surfaceSchema.$defs.actionInputSchema.required.some((key: string) => !Object.hasOwn(candidate, key))) return false;
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

    const publish = required(surface.pages.find((page) => page.id === "sites").sections[0].actions.find((action) => action.id === "publish"));
    expect(validInputSchema({ ...publish.inputSchema, properties: { siteId: { type: "string", pattern: "^forbidden$" } } })).toBe(false);
    expect(validInputSchema({ ...publish.inputSchema, required: ["undeclared"] })).toBe(false);
  });

  it("validates all declared query and response schemas with no open object payloads", async () => {
    const catalog = readContract('admin-actions.json');
    const ajv = new Ajv2020({ allErrors: true, strict: false });
    const validateAllObjectSchemas = (value: unknown): boolean => {
      if (Array.isArray(value)) return value.every(validateAllObjectSchemas);
      if (value === null || typeof value !== "object") return true;
      const candidate = value as Record<string, unknown>;
      if (candidate.type === "object" && candidate.additionalProperties !== false) return false;
      return Object.values(candidate).every(validateAllObjectSchemas);
    };

    for (const operation of Object.values(catalog.operations)) {
      if (operation.requestSchema) expect(() => ajv.compile(operation.requestSchema)).not.toThrow();
      if (operation.responseSchema) expect(() => ajv.compile(operation.responseSchema)).not.toThrow();
      if (operation.requestSchema) expect(validateAllObjectSchemas(operation.requestSchema)).toBe(true);
      if (operation.responseSchema) expect(validateAllObjectSchemas(operation.responseSchema)).toBe(true);
      if (operation.kind === "action") expect(operation.requestSchema).toBeDefined();
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
    const surface = readContract('admin-surface.json');
    const actions = readContract('admin-actions.json');
    const sitesPage = required(surface.pages.find((page) => page.id === "sites"));
    const siteTable = required(sitesPage.sections.find((section) => section.id === "sites"));
    const releaseTable = required(sitesPage.sections.find((section) => section.id === "releases"));
    const operationTable = required(sitesPage.sections.find((section) => section.id === "operations"));
    const publish = required(siteTable.actions.find((action) => action.id === "publish"));
    const rollback = required(siteTable.actions.find((action) => action.id === "rollback"));

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
      transport: "plugin-sdk.rest.artifact-stream",
      multipart: { parts: ["metadata", "artifact"], artifactFilenameForwarded: false },
      acceptedHttpStatus: 202,
      receiptSchema: "contracts/v1/artifact-operation-result.schema.json",
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
    const archiveVectors = readContract('admin-surface-vectors.json');
    expect(archiveVectors.scenarios).toContainEqual(expect.objectContaining({
      id: "admin-publish-counts-explicit-directories-toward-entry-limit",
      input: { regularFileEntries: 2, explicitDirectoryEntries: 9999, implicitParentDirectories: 0 },
      expected: { effectiveTarEntries: 10001, plugin: "accept-then-fail-operation-before-current-pointer-change" },
    }));
    expect(actions.releaseLifecycle).toMatchObject({
      storage: "plugin-owned-persistent-filesystem",
      activation: "atomic-current-previous-pointer-swap-only-after-successful-terminal-operation",
      failure: "keep-current-and-previous-unchanged",
    });
    expect(actions.publishOperation).toEqual({
      acceptance: "after-validated-metadata-and-bounded-artifact-bytes-are-durably-stored-and-sha256-matches",
      archiveValidation: "asynchronous-durable-operation-before-release-activation",
      archiveValidationFailure: "operation-failed-with-stable-error-code-current-and-previous-unchanged",
      currentPointer: "changes-only-after-all-archive-and-manifest-validation-succeeds",
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

	it("declares read-only per-domain certificate readiness without manual renew or revoke", async () => {
    const surface = readContract('admin-surface.json');
    const actions = readContract('admin-actions.json');
    const page = required(surface.pages.find((candidate) => candidate.id === "certificates"));
    const table = required(page.sections.find((section) => section.id === "domains"));

    expect(table.dataCapability).toBe("server.certificates.list");
    expect(table.columns).toEqual(expect.arrayContaining(["domain", "source", "readiness", "serial"]));
		expect(table.actions).toEqual(expect.arrayContaining([
			expect.objectContaining({ capability: "server.certificates.get", rowInput: { domain: "domain" } }) as unknown,
		]));
		expect(table.actions.map((action) => action.capability)).not.toContain("server.certificates.renew");
		expect(table.actions.map((action) => action.capability)).not.toContain("server.certificates.revoke");
    expect(actions.operations).not.toHaveProperty("server.certificates.renew");
    expect(actions.operations).not.toHaveProperty("server.certificates.revoke");
    expect(actions.operations["server.certificates.list"].responseSchema.$defs.certificateSummary.properties.readiness.enum)
      .toEqual(["pending", "ready", "failed", "unknown"]);
  });

  it("aligns artifact action inputs and archive safety limits with Server-owned contracts", async () => {
    const surface = readContract('admin-surface.json');
    const actionSchema = readContract('admin-surface.schema.json');
    const metadataSchema = readContract('artifact-metadata.schema.json');
    const receiptSchema = readContract('artifact-operation-result.schema.json');
    const sites = required(surface.pages.find((page) => page.id === "sites"));
    const publish = required(sites.sections.find((section) => section.id === "sites").actions.find((action) => action.id === "publish"));
    const actionProperties = actionSchema.$defs.action.properties;
    const ajv = new Ajv2020({ allErrors: true, strict: false });
    const validateSurface = ajv.compile(actionSchema);
    const validateInputSchema = (candidate: unknown) => {
      const wrapper = { ...publish, inputSchema: candidate };
      return validateSurface({ ...surface, pages: surface.pages.map((page) => ({
        ...page,
        sections: page.sections.map((section) => ({
          ...section,
          actions: section.actions?.map((action) => action.id === "publish" ? wrapper : action),
        })),
      })) });
    };

    expect(actionProperties.inputSchema).toBeDefined();
    expect(actionSchema.$defs.artifactInput.required).toEqual(["mediaTypes", "maxBytes", "maxMetadataBytes", "maxMultipartOverheadBytes"]);
    const catalog = readContract('admin-actions.json');
    expect(required(publish.artifactInput).maxBytes).toBeLessThanOrEqual(catalog.archive.artifactBytes);
    expect(required(publish.artifactInput).maxMetadataBytes).toBeLessThanOrEqual(catalog.archive.metadataBytes);
    expect(required(publish.artifactInput).maxMultipartOverheadBytes).toBeLessThanOrEqual(catalog.archive.multipartOverheadBytes);
    expect(catalog.archive.requestEnvelopeBytes).toBe(134348800);
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
      expect(validateSurface({ ...surface, pages: surface.pages.map((page) => ({
        ...page,
        sections: page.sections.map((section) => ({
          ...section,
          actions: section.actions?.map((action) => action.id === "publish" ? candidate : action),
        })),
      })) })).toBe(false);
    }
  });

  it("declares negative vectors for malformed product contracts without a peer-protocol admin API", async () => {
    const vectors = readContract('admin-surface-vectors.json');
    const serialized = JSON.stringify(readContract('admin-surface.json'));

    expect(serialized).not.toMatch(/ConfigSchema|ConfigApply|fieldsFromControlRpc/);
    expect(serialized).not.toMatch(/\/api\/(?:caddy|sites|certificates)(?:\/|"|$)/);
    expect(vectors.scenarios.map((scenario) => scenario.id)).toEqual(expect.arrayContaining([
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
      "admin-common-settings-live-in-core",
    ]));
  });
});

import { parseJSON } from '../support/json.js';
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { gzipSync } from "node:zlib";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

type Entry = { name: string; body?: Buffer; directory?: boolean };

function octal(header: Buffer, offset: number, width: number, value: number): void {
  header.write(`${value.toString(8).padStart(width - 1, "0")}\0`, offset, width, "ascii");
}

function archive(siteId: string, body: string): Buffer {
  const entries: Entry[] = [
    { name: "site-manifest.json", body: Buffer.from(JSON.stringify({ schemaVersion: 1, siteId, documentRoot: "public/nested", indexDocument: "home.htm" })) },
    { name: "public/", directory: true },
    { name: "public/nested/", directory: true },
    { name: "public/nested/home.htm", body: Buffer.from(body) },
    { name: "public/nested/assets/home.htm", body: Buffer.from("directory-index") },
    { name: "public/nested/assets/logo.txt", body: Buffer.from("asset") },
  ];
  const blocks: Buffer[] = [];
  for (const entry of entries) {
    const header = Buffer.alloc(512);
    header.write(entry.name, 0, 100, "utf8");
    octal(header, 100, 8, entry.directory ? 0o755 : 0o644);
    octal(header, 108, 8, 0);
    octal(header, 116, 8, 0);
    const content = entry.body ?? Buffer.alloc(0);
    octal(header, 124, 12, content.length);
    octal(header, 136, 12, 0);
    header.fill(0x20, 148, 156);
    header[156] = entry.directory ? 0x35 : 0x30;
    header.write("ustar\0", 257, "ascii");
    header.write("00", 263, "ascii");
    header.write(`${header.reduce((sum, byte) => sum + byte, 0).toString(8).padStart(6, "0")}\0 `, 148, 8, "ascii");
    blocks.push(header);
    if (content.length) {
      blocks.push(content);
      const padding = (512 - content.length % 512) % 512;
      if (padding) blocks.push(Buffer.alloc(padding));
    }
  }
  blocks.push(Buffer.alloc(1024));
  return gzipSync(Buffer.concat(blocks));
}

function request(artifact: Buffer, siteId: string, idempotencyKey: string, expectedCurrentRevision?: string) {
  return {
    metadata: JSON.stringify({
      version: 1,
      artifact: { mediaType: "application/gzip", byteLength: artifact.length, sha256: `sha256:${createHash("sha256").update(artifact).digest("hex")}` },
      payload: { siteId, ...(expectedCurrentRevision ? { expectedCurrentRevision } : {}) },
    }),
    contentType: "application/gzip",
    idempotencyKey,
    artifact: artifact.toString("base64"),
  };
}

describe("Server site publish product runtime", () => {
  it("streams a validated artifact into durable 202 operations, enforces CAS/idempotency and recovers current/previous", async () => {
    const firstArtifact = archive("docs", "first-release");
    const secondArtifact = archive("docs", "second-release");
    const invalidArchive = gzipSync(Buffer.from("not a tar archive"));
    const corruptDigest = request(firstArtifact, "docs", "bad-digest", `sha256:${createHash("sha256").update(secondArtifact).digest("hex")}`);
    const corruptDocument = parseJSON<Result>(corruptDigest.metadata);
    corruptDocument.artifact.sha256 = `sha256:${"0".repeat(64)}`;
    corruptDigest.metadata = JSON.stringify(corruptDocument);
    const duplicateMetadata = request(firstArtifact, "docs", "duplicate-metadata", `sha256:${createHash("sha256").update(secondArtifact).digest("hex")}`);
    duplicateMetadata.metadata = duplicateMetadata.metadata.replace('"siteId":"docs"', '"siteId":"docs","siteId":"docs"');

    const port = await freePort();
    const input = {
      port,
      requests: [
        request(firstArtifact, "docs", "publish-one"),
        request(firstArtifact, "docs", "publish-one"),
        request(secondArtifact, "docs", "publish-one"),
        request(secondArtifact, "docs", "publish-two", `sha256:${createHash("sha256").update(firstArtifact).digest("hex")}`),
        request(firstArtifact, "docs", "stale-cas", `sha256:${createHash("sha256").update(firstArtifact).digest("hex")}`),
        corruptDigest,
        duplicateMetadata,
        request(invalidArchive, "docs", "invalid-archive", `sha256:${createHash("sha256").update(secondArtifact).digest("hex")}`),
      ],
    };
    const output = execFileSync("go", ["run", "./tests/fixtures/site-publish"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      input: JSON.stringify(input),
      encoding: "utf8",
      timeout: 120_000,
    });
    const result = parseJSON<Result>(output);

    expect(result.first).toMatchObject({ accepted: true, state: "accepted" });
    expect(result.afterRestart).toMatchObject({ state: "completed", currentRevision: `sha256:${createHash("sha256").update(firstArtifact).digest("hex")}`, previousRevision: null });
    expect(result.orphanStagingRemoved).toBe(true);
    expect(result.postSwitchRecovery).toMatchObject({ state: "completed", currentRevision: `sha256:${createHash("sha256").update(firstArtifact).digest("hex")}` });
    expect(result.duplicate.operationId).toBe(result.first.operationId);
    expect(result.duplicateEffectCount).toBe(1);
    expect(result.idempotencyConflict.code).toBe("conflict");
    expect(result.second).toMatchObject({ accepted: true, state: "accepted" });
    expect(result.afterSecondPublish).toMatchObject({ state: "completed", previousRevision: `sha256:${createHash("sha256").update(firstArtifact).digest("hex")}` });
    expect(result.staleCAS.code).toBe("conflict");
    expect(result.badDigest.code).toBe("invalid_input");
    expect(result.duplicateMetadata.code).toBe("invalid_input");
    expect(result.invalidArchiveAccepted).toMatchObject({ accepted: true, state: "accepted" });
    expect(result.invalidArchiveAfterProcessing).toMatchObject({ state: "failed", code: "operation_failed" });
    expect(result.stateAfterInvalidArchive).toBe(`sha256:${createHash("sha256").update(secondArtifact).digest("hex")}`);
    expect(result.finalState).toMatchObject({
      currentRevision: `sha256:${createHash("sha256").update(secondArtifact).digest("hex")}`,
      previousRevision: `sha256:${createHash("sha256").update(firstArtifact).digest("hex")}`,
      rootDocument: "second-release",
      nestedAsset: "asset",
      directoryIndex: "directory-index",
      manifestPublic: false,
      caddyRootDocument: "second-release",
      caddyNestedAsset: "asset",
      caddyDirectoryIndex: "directory-index",
      caddyManifestStatus: 404,
    });
    expect(result.rollback).toMatchObject({
      state: "completed",
      currentRevision: `sha256:${createHash("sha256").update(firstArtifact).digest("hex")}`,
      previousRevision: `sha256:${createHash("sha256").update(secondArtifact).digest("hex")}`,
      rootDocument: "first-release",
    });
    expect(result.rollbackRepeat).toMatchObject({ code: "conflict" });
    expect(result.rollbackRepeat.operationId).not.toBe(result.rollback.operationId);
  }, 120_000);
});

async function freePort(): Promise<number> {
  const server = createServer();
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => resolve());
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("could not allocate fixture port");
  const { port } = address;
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error instanceof Error ? error : new Error(String(error))) : resolve()));
  return port;
}

interface Result { duplicate: { operationId: string }; first: { operationId: string }; idempotencyConflict: { code: string }; staleCAS: { code: string }; badDigest: { code: string }; [key: string]: unknown; accepted: { operationId: string }; replay: { operationId: string }; conflict: { code: string }; digestMismatch: { code: string }; duplicateMetadata: { code: string }; rollback: { operationId: string }; rollbackRepeat: { operationId: string } }

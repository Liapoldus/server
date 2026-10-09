import { required } from '../support/models.js';
import { parseJSON } from '../support/json.js';
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { gzipSync } from "node:zlib";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const manifest = JSON.stringify({ schemaVersion: 1, siteId: "docs", documentRoot: "public", indexDocument: "index.html" });

type Entry = { name: string; type?: "file" | "directory" | "symlink" | "hardlink" | "device"; body?: Buffer; target?: string };
type ArchiveCase = { name: string; artifact: Buffer; siteId?: string; preexisting?: string[] };

function octal(header: Buffer, offset: number, width: number, value: number): void {
  header.write(`${value.toString(8).padStart(width - 1, "0")}\0`, offset, width, "ascii");
}

function tar(entries: Entry[], postTerminator: Buffer = Buffer.alloc(0)): Buffer {
  const blocks: Buffer[] = [];
  for (const entry of entries) {
    const header = Buffer.alloc(512);
    header.write(entry.name, 0, 100, "utf8");
    octal(header, 100, 8, entry.type === "directory" ? 0o755 : 0o644);
    octal(header, 108, 8, 0);
    octal(header, 116, 8, 0);
    const body = entry.type === "file" || entry.type === undefined ? entry.body ?? Buffer.alloc(0) : Buffer.alloc(0);
    octal(header, 124, 12, body.length);
    octal(header, 136, 12, 0);
    header.fill(0x20, 148, 156);
    header[156] = entry.type === "directory" ? 0x35 : entry.type === "symlink" ? 0x32 : entry.type === "hardlink" ? 0x31 : entry.type === "device" ? 0x33 : 0x30;
    if (entry.target) header.write(entry.target, 157, 100, "utf8");
    header.write("ustar\0", 257, "ascii");
    header.write("00", 263, "ascii");
    const checksum = header.reduce((sum, byte) => sum + byte, 0);
    header.write(`${checksum.toString(8).padStart(6, "0")}\0 `, 148, 8, "ascii");
    blocks.push(header);
    if (body.length > 0) {
      blocks.push(body);
      const padding = (512 - (body.length % 512)) % 512;
      if (padding > 0) blocks.push(Buffer.alloc(padding));
    }
  }
  blocks.push(Buffer.alloc(1024));
  if (postTerminator.length > 0) blocks.push(postTerminator);
  return Buffer.concat(blocks);
}

function gzipTar(entries: Entry[], postTerminator?: Buffer): Buffer {
  return gzipSync(tar(entries, postTerminator));
}

function validEntries(): Entry[] {
  return [
    { name: "site-manifest.json", body: Buffer.from(manifest) },
    { name: "public/", type: "directory" },
    { name: "public/index.html", body: Buffer.from("site-content") },
  ];
}

function archiveCase(name: string, entries: Entry[], options: Partial<ArchiveCase> = {}): ArchiveCase {
  return { name, artifact: gzipTar(entries), ...options };
}

describe("site archive staging helper", () => {
  it("extracts only a safe single site archive and rejects unsafe/integrity failures without leaving staged files", () => {
    const corruptGzip = gzipTar(validEntries());
    corruptGzip[corruptGzip.length - 1] ^= 0xff;
    const corruptTar = tar(validEntries());
    corruptTar[0] ^= 1;
    const corruptTarGzip = gzipSync(corruptTar);
    const validGzip = gzipTar(validEntries());
    const concatenatedMember = Buffer.concat([validGzip, gzipSync(Buffer.alloc(0))]);
    const trailingCompressedByte = Buffer.concat([validGzip, Buffer.from([0x41])]);
    const tooManyEntries: Entry[] = [
      { name: "site-manifest.json", body: Buffer.from(manifest) },
      { name: "public/", type: "directory" },
      { name: "public/index.html", body: Buffer.from("site-content") },
      ...Array.from({ length: 4_998 }, (_, index) => ({
        name: `directory-${index}/`,
        type: "directory" as const,
      })),
      ...Array.from({ length: 5_000 }, (_, index) => ({
        name: `assets/file-${index}.bin`,
        body: createHash("sha256").update(String(index)).digest(),
      })),
    ];

    const cases: ArchiveCase[] = [
      archiveCase("valid", validEntries()),
      { name: "valid-zero-tar-padding", artifact: gzipTar(validEntries(), Buffer.alloc(512)) },
      { name: "valid-concatenated-gzip-members", artifact: concatenatedMember },
      archiveCase("reject-parent-traversal", [{ name: "../outside", body: Buffer.from("no") }]),
      archiveCase("reject-absolute-path", [{ name: "/outside", body: Buffer.from("no") }]),
      archiveCase("reject-duplicate-path", [...validEntries(), { name: "public/index.html", body: Buffer.from("duplicate") }]),
      archiveCase("reject-case-collision", [...validEntries(), { name: "public/INDEX.html", body: Buffer.from("collision") }]),
      archiveCase("reject-nfc-collision", [
        { name: "site-manifest.json", body: Buffer.from(manifest) },
        { name: "café.txt", body: Buffer.from("one") },
        { name: "cafe\u0301.txt", body: Buffer.from("two") },
      ]),
      archiveCase("reject-symlink", [...validEntries(), { name: "public/link", type: "symlink", target: "index.html" }]),
      archiveCase("reject-hardlink", [...validEntries(), { name: "public/link", type: "hardlink", target: "public/index.html" }]),
      archiveCase("reject-device", [...validEntries(), { name: "public/device", type: "device" }]),
      { name: "reject-corrupt-gzip", artifact: corruptGzip },
      { name: "reject-truncated-gzip", artifact: validGzip.subarray(0, validGzip.length - 5) },
      { name: "reject-corrupt-tar-header", artifact: corruptTarGzip },
      { name: "reject-tar-without-terminator", artifact: gzipSync(tar(validEntries()).subarray(0, -1024)) },
      { name: "reject-non-gzip-trailing-bytes", artifact: trailingCompressedByte },
      { name: "reject-nonzero-tar-trailing-data", artifact: gzipTar(validEntries(), Buffer.from("hidden-record")) },
      archiveCase("reject-ratio-over-limit", [...validEntries(), { name: "public/repetitive.txt", body: Buffer.alloc(128 * 1024, 0x41) }]),
      archiveCase("reject-entry-count-over-limit", tooManyEntries),
      archiveCase("reject-nonempty-staging", validEntries(), { preexisting: ["keep.txt"] }),
      archiveCase("reject-site-id-mismatch", [{ name: "site-manifest.json", body: Buffer.from(manifest.replace('"docs"', '"other"')) }, ...validEntries().slice(1)]),
    ];

    const output = execFileSync("go", ["run", "./tests/fixtures/site-archive"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      input: JSON.stringify({ cases: cases.map((candidate) => ({ ...candidate, siteId: candidate.siteId ?? "docs", artifact: candidate.artifact.toString("base64") })) }),
      encoding: "utf8",
    });
    const results = parseJSON<{ name: string; accepted: boolean; digest: string; compressedBytes: number; expandedBytes: number; files: string[] }[]>(output);

    const valid = required(results.find((result) => result.name === "valid"));
    expect(valid).toMatchObject({ accepted: true, files: ["public", "public/index.html", "site-manifest.json"] });
    expect(valid?.digest).toBe(`sha256:${createHash("sha256").update(cases[0].artifact).digest("hex")}`);
    expect(valid?.compressedBytes).toBe(cases[0].artifact.length);
    expect(valid?.expandedBytes).toBe(tar(validEntries()).length);
    expect(required(results.find((result) => result.name === "valid-concatenated-gzip-members"))?.accepted).toBe(true);
    expect(required(results.find((result) => result.name === "valid-zero-tar-padding"))?.accepted).toBe(true);

    for (const candidate of cases.filter((item) => item.name.startsWith("reject-"))) {
      const result = required(results.find((item) => item.name === candidate.name));
      expect(result?.accepted, candidate.name).toBe(false);
      if (!candidate.preexisting) expect(result?.files, candidate.name).toEqual([]);
      else expect(result?.files, candidate.name).toEqual(candidate.preexisting);
    }
  }, 120_000);
});

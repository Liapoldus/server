import { parseJSON } from '../support/json.js';
import { execFileSync } from "node:child_process";
import { createServer } from "node:net";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

describe("custom TLS settings", () => {
	it("verifies hostnames and preserves the active revision when a certificate/key pair is invalid", async () => {
    const httpPort = await freePort();
    const httpsPort = await freePort();
    const output = execFileSync("go", ["run", "./tests/fixtures/custom-tls"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      input: JSON.stringify({ httpPort, httpsPort }),
      encoding: "utf8",
    });
    const result = parseJSON<Record<string, unknown>>(output);

    expect(result).toMatchObject({
      initialRevision: "revision-1",
      invalidPairRejected: true,
      revisionAfterInvalidPair: "revision-1",
      oldRevisionStatus: 200,
      oldRevisionBody: "old-active",
      validCode: "OK",
      validRevision: "revision-3",
      tls12: { negotiatedProtocol: "http/1.1", status: 200, body: "custom-tls" },
      tls13: { negotiatedProtocol: "http/1.1", status: 200, body: "custom-tls" },
      wrongHostnameRejected: true,
    });
    expect(result.certificateList).toMatchObject({
      status: 200,
      body: { items: [{ domain: "custom.example.test", source: "custom", readiness: "ready", notAfter: expect.any(String), serial: expect.any(String) }] },
    });
    expect(result.certificateStatus).toMatchObject({
      status: 200,
      body: { domain: "custom.example.test", source: "custom", readiness: "ready", notBefore: expect.any(String), notAfter: expect.any(String), serial: expect.any(String), lastErrorCode: null },
    });
    expect(result.certificateMissing).toMatchObject({ status: 404, body: { code: "not_found" } });
    expect(result.certificateInvalidCursor).toMatchObject({ status: 400, body: { code: "invalid_input" } });
    expect(JSON.stringify(result)).not.toMatch(/PRIVATE KEY|certificatePem|privateKeyPem/i);
  }, 90_000);
});

async function freePort(): Promise<number> {
  const server = createServer();
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => resolve());
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("could not allocate a test port");
  const { port } = address;
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error instanceof Error ? error : new Error(String(error))) : resolve()));
  return port;
}

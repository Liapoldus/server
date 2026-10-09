import { parseJSON } from '../support/json.js';
import { execFileSync } from "node:child_process";
import { createServer } from "node:net";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

describe("custom TLS HTTP protocol conformance", () => {
  it("negotiates HTTP/2 over TLS and serves HTTP/3 over QUIC with plugin-owned certificate settings", async () => {
    const httpsPort = await freePort();
    const output = execFileSync("go", ["run", "./tests/fixtures/http2-http3"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      input: JSON.stringify({ httpsPort }),
      encoding: "utf8",
    });
    const result = parseJSON<Record<string, unknown>>(output);

    expect(result).toMatchObject({
      configApplied: true,
      http2: { protocol: "HTTP/2.0", alpn: "h2", status: 200, body: "secure-site", tlsVersion: "1.3" },
      http3: { protocol: "HTTP/3.0", alpn: "h3", status: 200, body: "secure-site", tlsVersion: "1.3" },
      tls12: { status: 200, tlsVersion: "1.2" },
      tls13: { status: 200, tlsVersion: "1.3" },
      tls11Rejected: true,
    });
    expect(JSON.stringify(result)).not.toMatch(/PRIVATE KEY|certificatePem|privateKeyPem/i);
  }, 120_000);
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

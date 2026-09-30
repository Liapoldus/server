import { execFileSync } from "node:child_process";
import { createServer } from "node:net";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

describe("Caddy Plugin SDK Reload adapter", () => {
  it("preserves the serving generation when a schema-valid Caddy activation fails", async () => {
    const port = await freePort();
    const output = execFileSync("go", ["run", "./tests/fixtures/plugin-sdk-reload"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off", GOTOOLCHAIN: "go1.26.0" },
      input: JSON.stringify({ port }),
      encoding: "utf8",
      timeout: 120_000,
    });

    const result = JSON.parse(output);
    expect(result).toEqual({
      acknowledgement: { generation: "generation-1", sha256: result.expectedDigest, applied: true },
      expectedDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      readyAfterApply: { ready: true, generation: "generation-1" },
      firstResponse: { status: 200, body: "sdk-caddy-active" },
      rejectedCandidate: true,
      candidateFailureStage: "runtime-activation",
      revisionAfterReject: "generation-1",
      readyAfterReject: { ready: true, generation: "generation-1" },
      responseAfterReject: { status: 200, body: "sdk-caddy-active" },
    });
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
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
  return port;
}

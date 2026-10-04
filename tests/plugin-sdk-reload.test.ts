import { execFileSync } from "node:child_process";
import { createServer } from "node:net";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

describe("Caddy Plugin SDK Reload adapter", () => {
  it("preserves the last-good Caddy generation and reports the replica not-ready after activation failure", async () => {
    const port = await freePort();
    const adminProbe = createServer();
    await new Promise<void>((resolve, reject) => {
      adminProbe.once("error", reject);
      adminProbe.listen(0, "127.0.0.1", resolve);
    });
    const adminAddress = adminProbe.address();
    if (!adminAddress || typeof adminAddress === "string") throw new Error("could not reserve Caddy admin port");

    let output: string;
    try {
      // Caddy must start while its configured default admin address is owned
      // by this probe; the Server runtime must not bind or expose that API.
      output = execFileSync("go", ["run", "./tests/fixtures/plugin-sdk-reload"], {
        cwd: root,
        env: {
          ...process.env,
          GOWORK: "off",
          GOTOOLCHAIN: "go1.26.0",
          CADDY_ADMIN: `127.0.0.1:${adminAddress.port}`,
        },
        input: JSON.stringify({ port }),
        encoding: "utf8",
        timeout: 120_000,
      });
    } finally {
      await new Promise<void>((resolve, reject) => adminProbe.close((error) => error ? reject(error) : resolve()));
    }

    const result = JSON.parse(output);
    expect(result).toEqual({
      acknowledgement: { generation: "generation-1", sha256: result.expectedDigest, applied: true },
      expectedDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      readyAfterApply: { ready: true, generation: "generation-1" },
      registration: { instanceId: "server", replicaId: "replica-1", ready: true, appliedGeneration: "generation-1" },
      manifestName: "server",
      configurationSchemaValid: true,
      metricsHasReadinessGauge: true,
      firstResponse: { status: 200, body: "sdk-caddy-active" },
      rejectedCandidate: true,
      candidateFailureStage: "runtime-activation",
      revisionAfterReject: "generation-1",
      readyAfterReject: { ready: false, generation: "generation-1" },
      responseAfterReject: { status: 200, body: "sdk-caddy-active" },
      secretsRedeemed: 2,
      secretPurposesValidated: true,
      redactionPassed: true,
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

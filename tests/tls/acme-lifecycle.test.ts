import { parseJSON } from '../support/json.js';
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { request as httpsRequest } from "node:https";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const pebbleImage = "ghcr.io/letsencrypt/pebble@sha256:ddf230642b1a584f519f32e347de1b05a6e4c1f6c35c1863b33effeab5f78199";

describe("automatic TLS lifecycle", () => {
  it("obtains and force-renews a certificate through Caddy and the ACME protocol", async () => {
    const acmePort = await freePort();
    const managementPort = await freePort();
    const listenerPort = await freePort();
    const challengePort = await freePort();
    const containerName = `liapoldus-pebble-${process.pid}-${Date.now()}`;
    const containerID = execFileSync("docker", [
      "run", "--detach", "--rm", "--name", containerName,
      "--publish", `127.0.0.1:${acmePort}:14000`,
      "--publish", `127.0.0.1:${managementPort}:15000`,
      "--env", "PEBBLE_VA_ALWAYS_VALID=1",
      "--env", "PEBBLE_VA_NOSLEEP=1",
      "--env", "PEBBLE_WFE_NONCEREJECT=0",
      pebbleImage,
    ], { encoding: "utf8" }).trim();

    try {
      await waitForPebble(acmePort);
      const rootPEM = await getLocalTLS(`https://127.0.0.1:${managementPort}/roots/0`);
      const trustDirectory = mkdtempSync(path.join(tmpdir(), "liapoldus-pebble-root-"));
      const trustFile = path.join(trustDirectory, "root.pem");
      execFileSync("docker", ["cp", `${containerID}:/test/certs/pebble.minica.pem`, trustFile]);
      const transportRootPEM = readFileSync(trustFile, "utf8");
      const port = listenerPort;
      const host = "acme-lifecycle.example.test";
      try {
        const output = execFileSync("go", ["run", "./tests/fixtures/caddy-activation"], {
          cwd: root,
          env: { ...process.env, GOWORK: "off", SSL_CERT_FILE: trustFile },
          input: JSON.stringify({
          port,
          skipRequest: true,
          certificateStatusHost: host,
          acmeDirectory: `https://127.0.0.1:${acmePort}/dir`,
          acmeRootPEM: rootPEM,
          acmeTransportRootPEM: transportRootPEM,
          httpChallengePort: challengePort,
          forceRenew: true,
          settings: {
            schemaVersion: 1,
            config: {
              listeners: [{
                id: "automatic-tls",
                kind: "http",
                address: `127.0.0.1:${port}`,
                hostnames: [host],
                protocols: ["http1"],
                tls: { mode: "automatic" },
              }],
              routes: [{
                id: "site",
                listenerId: "automatic-tls",
                handler: { type: "static", siteId: "frontend" },
              }],
            },
          },
          }),
          encoding: "utf8",
          timeout: 120_000,
        });

        const result = parseJSON<Result>(output);
        expect(result.revision).toBe("revision-1");
        expect(result.certificateStatus).toMatchObject({ domain: host, source: "acme", readiness: "ready" });
        expect(result.certificateStatus.serial).toEqual(expect.any(String));
        expect(result.tlsProbe).toMatchObject({ status: 200, body: "plugin-owned", serverName: host });
        expect(result.renewal).toMatchObject({ completed: true, changedCertificate: true });
      } finally {
        rmSync(trustDirectory, { recursive: true, force: true });
      }
    } finally {
      execFileSync("docker", ["rm", "--force", containerID], { stdio: "ignore" });
    }
  }, 150_000);
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

async function waitForPebble(port: number): Promise<void> {
  const deadline = Date.now() + 30_000;
  let lastError: unknown;
  while (Date.now() < deadline) {
    try {
      await getLocalTLS(`https://127.0.0.1:${port}/dir`);
      return;
    } catch (error) {
      lastError = error;
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
  }
  throw new Error(`Pebble did not become ready: ${String(lastError)}`);
}

async function getLocalTLS(url: string): Promise<string> {
  return await new Promise((resolve, reject) => {
    const request = httpsRequest(url, {
      rejectUnauthorized: false,
      headers: { "user-agent": "liapoldus-acme-test" },
    }, (response) => {
      const chunks: Buffer[] = [];
      response.on("data", (chunk: Buffer) => chunks.push(chunk));
      response.on("end", () => {
        const body = Buffer.concat(chunks).toString("utf8");
        if ((response.statusCode ?? 500) >= 400) reject(new Error(`local test endpoint returned ${response.statusCode}: ${body}`));
        else resolve(body);
      });
    });
    request.on("error", reject);
    request.end();
  });
}

interface Result { revision: string; certificateStatus: { serial: string }; tlsProbe: unknown; renewal: unknown }

import { parseJSON } from '../support/json.js';
import { readContract } from '../support/contracts.js';
import { createServer } from "node:http";
import { spawn } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { Ajv2020 } from "ajv/dist/2020.js";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

describe("strict settings to Caddy runtime compilation", () => {
	it("serves the current site release from a schema-validated settings revision", async () => {
    const port = await freePort();
    const settings = httpSettings(port, { type: "static", siteId: "frontend" });
    const result = await runActivation({ port, settings, siteContent: "compiled-static" });

    expect(result).toEqual({ revision: "revision-1", status: 200, body: "compiled-static" });
  }, 60_000);

	it("proxies to one HTTP origin from a schema-validated settings revision", async () => {
    const upstream = createServer((_request, response) => {
      response.writeHead(202, { "Content-Type": "text/plain" });
      response.end("compiled-proxy");
    });
    await new Promise<void>((resolve, reject) => {
      upstream.once("error", reject);
      upstream.listen(0, "127.0.0.1", () => resolve());
    });
    const address = upstream.address();
    if (!address || typeof address === "string") throw new Error("could not allocate upstream port");
    const port = await freePort();

    try {
      const settings = httpSettings(port, {
        type: "reverseProxy",
        upstreams: [{ origin: `http://127.0.0.1:${address.port}` }],
      });
      const result = await runActivation({ port, settings });

      expect(result).toEqual({ revision: "revision-1", status: 202, body: "compiled-proxy" });
    } finally {
      await new Promise<void>((resolve, reject) => upstream.close((error) => error ? reject(error instanceof Error ? error : new Error(String(error))) : resolve()));
    }
  }, 60_000);

  it("rejects a schema-valid unsupported candidate without replacing the active Caddy revision", async () => {
    const port = await freePort();
    const candidateSettings = httpSettings(port, {
      type: "plugin",
      instanceId: "unconfigured-plugin",
      capability: "forms.submit",
      mode: "http_stream",
    });
    const schema = readContract('settings.schema.json');
    expect(new Ajv2020({ allErrors: true }).compile(schema)(candidateSettings)).toBe(true);
    const result = await runActivation({
      port,
      settings: httpSettings(port, { type: "static", siteId: "frontend" }),
      siteContent: "active-before-reject",
      candidateSettings,
    });

    expect(result).toEqual({
      revision: "revision-1",
      candidateCode: "InvalidArgument",
      revisionAfterCandidate: "revision-1",
      status: 200,
      body: "active-before-reject",
    });
  }, 60_000);
});

function httpSettings(port: number, handler: Record<string, unknown>) {
  return {
    schemaVersion: 1,
    config: {
      listeners: [{
        id: "web",
        kind: "http",
        address: `127.0.0.1:${port}`,
        hostnames: [],
        protocols: ["http1"],
        tls: { mode: "disabled" },
      }],
      routes: [{ id: "route", listenerId: "web", handler }],
    },
  };
}

function runActivation(input: Record<string, unknown>): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const child = spawn("go", ["run", "./tests/fixtures/caddy-activation"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
    });
    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];
    child.stdout.on("data", (chunk: Buffer) => stdout.push(chunk));
    child.stderr.on("data", (chunk: Buffer) => stderr.push(chunk));
    child.once("error", reject);
    child.once("close", (code) => {
      if (code !== 0) {
        reject(new Error(Buffer.concat(stderr).toString("utf8")));
        return;
      }
      try {
        resolve(parseJSON<Record<string, unknown>>(Buffer.concat(stdout).toString("utf8")));
      } catch (error) {
        reject(error instanceof Error ? error : new Error(String(error)));
      }
    });
    child.stdin.end(JSON.stringify(input));
  });
}

async function freePort(): Promise<number> {
  const server = createServer();
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => resolve());
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("could not allocate test port");
  const { port } = address;
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error instanceof Error ? error : new Error(String(error))) : resolve()));
  return port;
}

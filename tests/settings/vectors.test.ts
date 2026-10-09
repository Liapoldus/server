import { required } from '../support/models.js';
import { parseJSON } from '../support/json.js';
import { readContract } from '../support/contracts.js';
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { createServer } from "node:http";
import { Ajv2020 } from "ajv/dist/2020.js";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const vectors = readContract('settings-vectors.json');
const schema = readContract('settings.schema.json');
const validateSchema = new Ajv2020({ allErrors: true }).compile(schema);
const selected = new Map(vectors.scenarios.map((scenario) => [scenario.id, scenario] as const));

describe("executable settings vectors", () => {
  it("accepts the valid IPv6 bind literal in the settings compiler", () => {
    const vector = requiredVector("ipv6-bind-valid-literal");
    const settings = settingsForAddress(required(vector.input.address));
    expect(validateSchema(settings)).toBe(true);
    const result = validateSettings(settings);
    expect(result.accepted).toBe(vector.expected.accepted);
  }, 60_000);

  it("rejects a schema-shaped malformed IPv6 bind literal semantically", () => {
    const vector = requiredVector("ipv6-bind-rejects-invalid-literal");
    const settings = settingsForAddress(vector.input.address);
    expect(validateSchema(settings)).toBe(true);
    const result = validateSettings(settings);
    expect(result.accepted).toBe(vector.expected.accepted);
  }, 60_000);

  it("preserves method and query in an automatic HTTPS redirect", async () => {
    const vector = requiredVector("automatic-https-redirect-preserves-method-and-query");
    const sourcePort = await freePort();
    const targetPort = await freePort();
    const result = runRuntime(redirectSettings(sourcePort, targetPort), {
	      method: required(vector.input.request).method,
	      host: required(vector.input.request).host,
	      target: required(vector.input.request).requestTarget,
    }, sourcePort);

    expect(result).toMatchObject({
      status: vector.expected.status,
      location: vector.expected.location,
    });
    expect(vector.expected.methodPreserved).toBe(true);
    expect(required(vector.input.request).method).toBe("POST");
  }, 60_000);

  it("rejects a redirect host outside automatic TLS target coverage", async () => {
    const vector = requiredVector("automatic-https-rejects-host-outside-target-coverage");
    const sourcePort = await freePort();
    const targetPort = await freePort();
    const result = runRuntime(redirectSettings(sourcePort, targetPort), {
      method: "GET",
      host: required(vector.input.request).host,
      target: required(vector.input.request).requestTarget,
    }, sourcePort);

    expect(result.status).toBe(vector.expected.status);
    expect(result.location || null).toBe(vector.expected.location);
  }, 60_000);
});

function requiredVector(id: string) {
  const vector = selected.get(id);
  if (!vector) throw new Error(`settings vector is missing: ${id}`);
  return vector;
}

function settingsForAddress(address: string) {
  return {
    schemaVersion: 1,
    config: {
      listeners: [{
        id: "web", kind: "http", address, hostnames: [], protocols: ["http1"], tls: { mode: "disabled" },
      }],
      routes: [],
    },
  };
}

function redirectSettings(sourcePort: number, targetPort: number) {
  const vector = requiredVector("automatic-https-redirect-preserves-method-and-query");
  const settings = {
    schemaVersion: 1,
    config: {
      listeners: [
        {
          id: "http", kind: "http", address: `127.0.0.1:${sourcePort}`, hostnames: [],
          protocols: required(vector.input.source).protocols, tls: { mode: required(vector.input.source).tls },
          redirectToListenerId: required(vector.input.source).redirectToListenerId,
        },
        {
          id: required(vector.input.target).id, kind: "http", address: `127.0.0.1:${targetPort}`,
          hostnames: required(vector.input.target).hostnames, protocols: ["http1"], tls: { mode: required(vector.input.target).tls },
        },
      ],
      routes: [],
    },
  };
  expect(validateSchema(settings)).toBe(true);
  return settings;
}

function validateSettings(settings: unknown): { accepted: boolean } {
  return runFixture({ mode: "validate", settings });
}

function runRuntime(settings: unknown, request: unknown, port: number): Result {
  return runFixture({ mode: "serve", settings, request, port });
}

function runFixture(input: unknown): Result {
  const output = execFileSync("go", ["run", "./tests/fixtures/settings-vectors"], {
    cwd: root,
    env: { ...process.env, GOWORK: "off" },
    input: JSON.stringify(input),
    encoding: "utf8",
    timeout: 60_000,
  });
  return parseJSON<Result>(output);
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

interface Result { accepted: boolean; status: number; location: string }

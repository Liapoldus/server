import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import Ajv2020 from "ajv/dist/2020.js";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const schema = JSON.parse(readFileSync(path.join(root, "contracts/v1/settings.schema.json"), "utf8"));

function streamSettings(streamLimits: object) {
  return {
    schemaVersion: 1,
    config: {
      listeners: [{ id: "web", kind: "http", address: "127.0.0.1:8080", hostnames: [], protocols: ["http1"], tls: { mode: "disabled" } }],
      routes: [{ id: "events", listenerId: "web", handler: { type: "plugin", instanceId: "forms", capability: "forms.events", mode: "http_stream", streamLimits } }],
    },
  };
}

describe("HTTP stream limits contract", () => {
  it("accepts bounded route limits independently of unary call timeout", () => {
    const validate = new Ajv2020({ allErrors: true }).compile(schema);
    expect(validate(streamSettings({ maxConcurrency: 1, idleTimeoutMillis: 1_000, maxDurationMillis: 2_000 }))).toBe(true);
    expect(validate(streamSettings({ maxConcurrency: 129, idleTimeoutMillis: 1_000, maxDurationMillis: 2_000 }))).toBe(false);
    expect(validate(streamSettings({ maxConcurrency: 1, idleTimeoutMillis: 0, maxDurationMillis: 2_000 }))).toBe(false);
    expect(validate(streamSettings({ maxConcurrency: 1, idleTimeoutMillis: 1_000, maxDurationMillis: 3_600_001 }))).toBe(false);
  });
});

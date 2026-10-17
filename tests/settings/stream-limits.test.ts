import { readContract } from '../support/contracts.js';
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { settingsAjv } from "../support/settings-ajv.js";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const schema = readContract('settings.schema.json');

function streamSettings(streamLimits: object) {
  return {
    schemaVersion: 1,
    config: {
      listeners: [{ id: "web", kind: "http", address: "127.0.0.1:8080", hostnames: [], protocols: ["http1"], tls: { mode: "disabled" } }],
      routes: [{
        id: "events",
        listenerId: "web",
        handler: { type: "service", instanceId: "forms", capability: "forms.events", mode: "http_stream", streamLimits },
        api: {
          version: "server-http/v1",
          method: "GET",
          path: "/events",
          responses: [{ status: 200, contentType: "application/json", schema: {}, example: null }],
        },
      }],
    },
  };
}

describe("HTTP stream limits contract", () => {
  it("accepts bounded route limits independently of unary call timeout", () => {
    const validate = settingsAjv().compile(schema);
    expect(validate(streamSettings({ maxConcurrency: 1, idleTimeoutMillis: 1_000, maxDurationMillis: 2_000 }))).toBe(true);
    expect(validate(streamSettings({ maxConcurrency: 129, idleTimeoutMillis: 1_000, maxDurationMillis: 2_000 }))).toBe(false);
    expect(validate(streamSettings({ maxConcurrency: 1, idleTimeoutMillis: 0, maxDurationMillis: 2_000 }))).toBe(false);
    expect(validate(streamSettings({ maxConcurrency: 1, idleTimeoutMillis: 1_000, maxDurationMillis: 3_600_001 }))).toBe(false);
  });
});

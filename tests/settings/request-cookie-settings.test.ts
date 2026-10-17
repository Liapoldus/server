import { readContract } from '../support/contracts.js';
import { settingsAjv } from "../support/settings-ajv.js";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const schema = readContract('settings.schema.json');
const validate = settingsAjv().compile(schema);

function settings(requestCookieNames: string[]) {
  return {
    schemaVersion: 2,
    config: {
      listeners: [{ id: "web", kind: "http", address: "127.0.0.1:8080", hostnames: [], protocols: ["http1"], tls: { mode: "disabled" } }],
      routes: [{
        id: "forms",
        listenerId: "web",
        handler: { type: "service", instanceId: "forms", capability: "forms.session", mode: "call", requestCookieNames },
        api: {
          version: "server-http/v1",
          method: "GET",
          path: "/forms",
          responses: [{ status: 200, contentType: "application/json", schema: {}, example: null }],
        },
      }],
    },
  };
}

describe("Server request-cookie allow-list settings", () => {
  it("accepts unique cookie token names and rejects duplicates or invalid names", () => {
    expect(validate(settings(["session", "csrf-token"]))).toBe(true);
    expect(validate(settings(["session", "session"]))).toBe(false);
    expect(validate(settings(["bad;name"]))).toBe(false);
  });
});

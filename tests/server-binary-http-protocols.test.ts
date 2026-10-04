import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

describe("real Server binary HTTP protocol gate", () => {
  it("serves the same active release through HTTP/1.1, HTTP/2, and HTTP/3", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/caddy-rest-process"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off", GOTOOLCHAIN: "go1.26.0" },
      encoding: "utf8",
      timeout: 180_000,
    });
    expect(JSON.parse(output).protocols).toEqual({
      http1: { protocol: "HTTP/1.1", alpn: "http/1.1", status: 200, body: "published-site" },
      http2: { protocol: "HTTP/2.0", alpn: "h2", status: 200, body: "published-site" },
      http3: { protocol: "HTTP/3.0", alpn: "h3", status: 200, body: "published-site" },
    });
    expect(output).not.toMatch(/PRIVATE KEY|BEGIN CERTIFICATE|grant-fixture-/);
  }, 180_000);
});

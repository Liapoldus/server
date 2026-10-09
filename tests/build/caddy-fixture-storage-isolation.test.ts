import { parseJSON } from '../support/json.js';
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const runtimeFixtures = [
  "caddy-activation",
  "custom-tls",
  "http2-http3",
  "http-dispatch",
  "http-stream-limits",
  "plugin-sdk-reload",
  "site-publish",
  "settings-vectors",
  "upstream-ca",
  "websocket-dispatch",
];

describe("Caddy runtime fixture storage isolation", () => {
  it.each(runtimeFixtures)("isolates %s from the operator's Caddy data directory", (fixture) => {
    const source = readFileSync(`${root}/tests/fixtures/${fixture}/main.go`, "utf8");
    const isolation = source.indexOf("shared.IsolateCaddyDataHome()");
    const runtime = source.indexOf("caddyruntime.New()");

    expect(isolation, `${fixture} must set XDG_DATA_HOME to a temporary directory`).toBeGreaterThanOrEqual(0);
    expect(runtime, `${fixture} must construct the runtime`).toBeGreaterThan(isolation);
  });

  it("resets Caddy's package-initialized default storage to the isolated data directory", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/caddy-storage-isolation"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      encoding: "utf8",
    });
    const result = parseJSON<{ isolated: boolean; acmeTestAuthority: string; dataHome: string }>(output);

    expect(result.isolated).toBe(true);
    expect(result.acmeTestAuthority).toBe("http://127.0.0.1:1/acme/directory");
    expect(result.dataHome).toBeTypeOf("string");
    expect(existsSync(result.dataHome)).toBe(false);
  }, 120_000);
});

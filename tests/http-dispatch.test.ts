import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

describe("Caddy plugin unary HTTP dispatch", () => {
  it("forwards bounded HTTP context to a plugin and applies its response action", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/http-dispatch"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      encoding: "utf8",
    });

    expect(JSON.parse(output)).toEqual({
      revision: "revision-1",
      status: 202,
      contentType: "text/plain",
      body: "plugin-response",
      method: "POST",
      path: "/submit",
    });
  }, 60_000);
});

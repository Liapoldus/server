import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

describe("Server request header limit", () => {
  it("bounds aggregate parsed request headers at the approved 64 KiB and serves the exact boundary", () => {
    const result = JSON.parse(execFileSync("go", ["run", "./tests/fixtures/http-header-limits"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      encoding: "utf8",
      timeout: 120_000,
    }));

    expect(result).toEqual({
      maxRequestHeaderBytes: 65_536,
      atLimitStatus: 200,
      overLimitStatus: 431,
    });
  }, 120_000);
});

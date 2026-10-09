import { parseJSON } from '../support/json.js';
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

describe("Server site publish cross-process coordination", () => {
  it("allows only one concurrent publish for a site across independent processes", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/site-publish-concurrency"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off", GOTOOLCHAIN: "go1.26.0" },
      encoding: "utf8",
      timeout: 60_000,
    });
    const result = parseJSON<Result>(output);

    expect(result.outcomes).toHaveLength(2);
    expect(result.outcomes.filter((outcome: { accepted: boolean }) => outcome.accepted)).toHaveLength(1);
    expect(result.outcomes.filter((outcome: { code: string }) => outcome.code === "conflict")).toHaveLength(1);
  }, 90_000);
});

interface Result { outcomes: { accepted: boolean; code: string }[] }

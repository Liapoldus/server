import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

describe("Caddy production process Plugin SDK lifecycle", () => {
  it("pulls config, publishes a streamed site artifact, and serves its immutable release", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/caddy-rest-process"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off", GOTOOLCHAIN: "go1.26.0" },
      encoding: "utf8",
      timeout: 180_000,
    });

    expect(JSON.parse(output)).toMatchObject({
      acknowledgement: { generation: "generation-rest-1", applied: true },
      readiness: { ready: true, generation: "generation-rest-1" },
      response: { status: 200, body: "published-site" },
      adminSurface: { status: 200, hasPublishAction: true },
      artifact: { status: 202, state: "accepted" },
      crossPageQuery: { status: 400, body: { code: "invalid_input" } },
      beforeCompletion: { status: 404, body: "" },
      manifestResponse: { status: 404 },
      childExitCode: 0,
    });
  }, 180_000);
});

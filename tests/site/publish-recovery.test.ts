import { parseJSON } from '../support/json.js';
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

describe("Server site publish recovery across independent processes", () => {
  it("recovers an accepted immutable release after the accepting process is killed", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/site-publish-recovery"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off", GOTOOLCHAIN: "go1.26.0" },
      encoding: "utf8",
      timeout: 60_000,
    });
    const result = parseJSON<Result>(output);

    expect(result.acceptingProcessKilled).toBe(true);
    expect(result.accepted).toMatchObject({ state: "accepted", operationId: expect.any(String) });
    expect(result.recoveredBy).toHaveLength(2);
    for (const replica of result.recoveredBy) {
      expect(replica).toMatchObject({
        operationState: "completed",
        currentRevision: `sha256:${result.expectedDigest}`,
        servedBody: "recovered-after-process-exit",
      });
    }
  }, 90_000);
});

interface Result { acceptingProcessKilled: boolean; accepted: unknown; recoveredBy: unknown[]; expectedDigest: string }

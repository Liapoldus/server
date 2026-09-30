import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

describe("production ConfigApply settings-schema boundary", () => {
  it("validates strict Liapoldus settings before the runtime adapter and preserves the active revision on rejection", () => {
    const validSettings = { schemaVersion: 1, config: { listeners: [], routes: [] } };
    const input = {
      calls: [
        { revision: "revision-1", settings: validSettings },
        { revision: "revision-2", settings: { ...validSettings, config: { listeners: [], routes: [], unknown: true } } },
        { revision: "revision-3", settings: { schemaVersion: 1, config: { listeners: [] } } },
      ],
    };
    const output = execFileSync("go", ["run", "./tests/fixtures/config-apply-schema"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      input: JSON.stringify(input),
      encoding: "utf8",
    });
    const result = JSON.parse(output);

    expect(result.calls).toEqual([
      { applied: true, revision: "revision-1", code: "OK" },
      { applied: false, revision: "", code: "InvalidArgument" },
      { applied: false, revision: "", code: "InvalidArgument" },
    ]);
    expect(result.runtime).toMatchObject({ validateCalls: 1, activateCalls: 1 });
    expect(result.activeRevision).toBe("revision-1");
    expect(result.activeConfig).toEqual({ listeners: [], routes: [] });
    expect(result.runtime.lastValidated).toEqual({ listeners: [], routes: [] });
  }, 60_000);
});

import { execFileSync } from "node:child_process";
import { createServer } from "node:net";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import Ajv2020 from "ajv/dist/2020.js";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const schema = JSON.parse(readFileSync(path.join(root, "contracts/v1/settings.schema.json"), "utf8"));

describe("Caddy plugin ConfigApply contract", () => {
  it("accepts versioned native Caddy JSON and rejects malformed settings", () => {
    const validate = new Ajv2020({ allErrors: true }).compile(schema);

    expect(validate({ schemaVersion: 1, config: { apps: {} } })).toBe(true);
    expect(validate({ schemaVersion: 2, config: { apps: {} } })).toBe(false);
    expect(validate({ schemaVersion: 1, config: "not-an-object" })).toBe(false);
    expect(validate({ schemaVersion: 1, config: {}, extra: true })).toBe(false);
  });

  it("acknowledges a valid revision and retains active settings when a candidate is rejected", () => {
    const input = JSON.stringify({
      calls: [
        { revision: "revision-1", settings: { schemaVersion: 1, config: { apps: { marker: "active" } } } },
        { revision: "revision-2", settings: { schemaVersion: 1, config: { reject: true } } },
        { revision: "revision-1", settings: { schemaVersion: 1, config: { apps: { marker: "changed" } } } },
      ],
    });
    const output = execFileSync("go", ["run", "./tests/fixtures/config-apply"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      input,
      encoding: "utf8",
    });
    const result = JSON.parse(output);

    expect(result.calls[0]).toMatchObject({ applied: true, revision: "revision-1", code: "OK" });
    expect(result.calls[1]).toMatchObject({ applied: false, revision: "", code: "InvalidArgument" });
    expect(result.calls[2]).toMatchObject({ applied: false, revision: "", code: "FailedPrecondition" });
    expect(result.activeRevision).toBe("revision-1");
    expect(result.activeConfig).toContain('"marker":"active"');
  });

  it("activates native Caddy JSON delivered through ConfigApply", async () => {
    const port = await freePort();
    const output = execFileSync("go", ["run", "./tests/fixtures/caddy-activation"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      input: JSON.stringify({ port }),
      encoding: "utf8",
    });

    expect(JSON.parse(output)).toEqual({ revision: "revision-1", status: 200, body: "plugin-owned" });
  }, 60_000);
});

async function freePort(): Promise<number> {
  const server = createServer();
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => resolve());
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("could not allocate a test port");
  const { port } = address;
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
  return port;
}

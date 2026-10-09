import { readFile } from "node:fs/promises";
import { describe, expect, it } from "vitest";

describe("Server plugin v1 excludes public L4", () => {
  it("does not ship Caddy-L4 or declare TCP/UDP listener contracts", async () => {
    const [moduleFile, runtimeFile, schemaFile] = await Promise.all([
      readFile(new URL("../go.mod", import.meta.url), "utf8"),
      readFile(new URL("../internal/infrastructure/caddy/runtime.go", import.meta.url), "utf8"),
      readFile(new URL("../contracts/v1/settings.schema.json", import.meta.url), "utf8"),
    ]);

    expect(moduleFile).not.toContain("github.com/mholt/caddy-l4");
    expect(runtimeFile).not.toContain("caddy-l4");
    expect(schemaFile).not.toMatch(/l4Listener|l4Route|l4Relay|\"kind\":\s*\"l4\"/);
  });
});

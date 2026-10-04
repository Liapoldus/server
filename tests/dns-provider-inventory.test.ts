import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

describe("Server v1 DNS challenge inventory", () => {
	it("ships no DNS provider modules in the production Caddy module registry", () => {
		const output = execFileSync("go", ["run", "./tests/fixtures/dns-provider-inventory"], {
			cwd: root,
			env: { ...process.env, GOWORK: "off", GOTOOLCHAIN: "go1.26.0" },
			encoding: "utf8",
		});

		expect(JSON.parse(output).dnsProviderModules).toEqual([]);
	}, 180_000);
});

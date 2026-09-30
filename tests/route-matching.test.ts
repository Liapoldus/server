import { createServer } from "node:http";
import { spawn } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import Ajv2020 from "ajv/dist/2020.js";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const vectors = JSON.parse(readFileSync(path.join(root, "contracts/v1/settings-vectors.json"), "utf8"));
const settingsSchema = JSON.parse(readFileSync(path.join(root, "contracts/v1/settings.schema.json"), "utf8"));
const validateSettings = new Ajv2020({ allErrors: true }).compile(settingsSchema);

describe("Caddy strict HTTP route matching", () => {
  it("matches canonical paths, hosts and methods in order and forwards the canonical path", async () => {
    const originA = await startOrigin("a");
    const originB = await startOrigin("b");
    const port = await freePort();
    try {
      const settings = {
        schemaVersion: 1,
        config: {
          listeners: [listener(port)],
          routes: [
            route("exact-idna", {
              hosts: ["BÜCHER.example"], methods: ["GET"], path: { type: "exact", value: "/exact" },
            }, proxy(originA.origin)),
            route("normalized-prefix", { path: { type: "prefix", value: "/normalized" } }, proxy(originA.origin)),
            route("wildcard-globstar", {
              hosts: ["*.example.test"], methods: ["GET"], path: { type: "glob", value: "/assets/**" },
            }, proxy(originA.origin)),
            route("shadowed-exact", {
              hosts: ["*.example.test"], path: { type: "exact", value: "/assets/special" },
            }, proxy(originB.origin)),
            route("single-segment-glob", { path: { type: "glob", value: "/single/*" } }, proxy(originB.origin)),
            route("re2-full-match", { path: { type: "regex", value: "^/regex/[0-9]+$" } }, proxy(originA.origin)),
            route("percent-decoded-once", {
              path: { type: "exact", value: vector("request-path-percent-decodes-only-once").expected.canonicalPath },
            }, proxy(originA.origin)),
            route("glob-triple-star", {
              path: { type: "glob", value: vector("glob-triple-star-uses-longest-tokenization").input.pattern },
            }, proxy(originA.origin)),
            route("normalized-static-path", {
              path: { type: "exact", value: "/docs/file.txt" },
            }, { type: "static", siteId: "frontend" }),
            route("fallback", undefined, proxy(originB.origin)),
          ],
        },
      };
      const result = await runFixture({
        port,
        settings,
        requests: [
          { target: "/exact", host: "xn--bcher-kva.example:8443" },
          { target: "/exact", host: "other.example" },
          { target: "/exact", host: "xn--bcher-kva.example", method: "POST" },
          { target: "/normalized//a/../one?value=a%2Fb" },
          { target: "/assets/a/b", host: "one.example.test" },
          { target: "/assets/special", host: "one.example.test" },
          { target: "/assets/a/b", host: "example.test" },
          { target: "/assets/a/b", host: "two.one.example.test" },
          { target: "/single/a" },
          { target: "/single/a/b" },
          { target: "/regex/123" },
          { target: "/regex/abc" },
          { target: vector("request-path-decodes-once-and-normalizes-dot-segments").input.requestTarget },
          { target: vector("request-path-percent-decodes-only-once").input.requestTarget },
          { target: vector("request-path-preserves-query-separately").input.requestTarget },
          { target: vector("glob-triple-star-uses-longest-tokenization").input.path },
          { target: "/docs//dir/../file.txt" },
          { target: vector("request-path-rejects-malformed-percent-encoding").input.requestTarget, raw: true },
          { target: "/%00", raw: true },
        ],
      });
      expect(result.responses.map((response: { status: number }) => response.status)).toEqual([
        200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 200, 400, 400,
      ]);
      expect(result.responses.filter((response: { status: number }) => response.status === 200).map((response: { body: string }) => response.body)).toEqual([
        "a:/exact",
        "b:/exact",
        "b:/exact",
        "a:/normalized/one?value=a%2Fb",
        "a:/assets/a/b",
        "a:/assets/special",
        "b:/assets/a/b",
        "b:/assets/a/b",
        "b:/single/a",
        "b:/single/a/b",
        "a:/regex/123",
        "b:/regex/abc",
        "b:/a/b/?page=1",
        "a:/a/%252e%252e/b",
        "b:/search?q=a%2Fb",
        "a:/x/a/b",
        "static-canonical",
      ]);
    } finally {
      await Promise.all([originA.close(), originB.close()]);
    }
  }, 60_000);

  it("rejects noncanonical and canonical-equivalent matchers without replacing active settings", async () => {
    const port = await freePort();
    const settings = {
      schemaVersion: 1,
      config: {
        listeners: [listener(port)],
        routes: [route("active", undefined, { type: "static", siteId: "frontend" })],
      },
    };
    const noncanonical = {
      schemaVersion: 1,
      config: {
        listeners: [listener(port)],
        routes: [route("bad-path", { path: { type: "exact", value: "/a//b" } }, proxy("http://127.0.0.1:18081"))],
      },
    };
    const equivalentMatchers = {
      schemaVersion: 1,
      config: {
        listeners: [listener(port)],
        routes: [
          route("upper-host", { hosts: ["BÜCHER.example"], path: { type: "exact", value: "/same" } }, proxy("http://127.0.0.1:18081")),
          route("ascii-host", { hosts: ["xn--bcher-kva.example"], path: { type: "exact", value: "/same" } }, proxy("http://127.0.0.1:18082")),
        ],
      },
    };
    const exactDotSegments = {
      schemaVersion: 1,
      config: {
        listeners: [listener(port)],
        routes: [route("dot-segments", {
          path: vector("configured-exact-path-rejects-dot-segments").input,
        }, proxy("http://127.0.0.1:18081"))],
      },
    };
    expect(validateSettings(noncanonical)).toBe(true);
    expect(validateSettings(equivalentMatchers)).toBe(true);
    expect(validateSettings(exactDotSegments)).toBe(true);
    const result = await runFixture({
      port,
      settings,
      siteContent: "active-before-invalid-candidates",
      candidates: [noncanonical, exactDotSegments, equivalentMatchers],
      requests: [{ target: "/", host: "localhost" }],
    });

    expect(result.candidateCodes).toEqual(["InvalidArgument", "InvalidArgument", "InvalidArgument"]);
    expect(result.revisionAfterCandidates).toBe("revision-1");
    expect(result.responses).toEqual([{ status: 200, body: "active-before-invalid-candidates" }]);
  }, 60_000);
});

function listener(port: number) {
  return {
    id: "web",
    kind: "http",
    address: `127.0.0.1:${port}`,
    hostnames: [],
    protocols: ["http1"],
    tls: { mode: "disabled" },
  };
}

function route(id: string, match: unknown, handler: unknown) {
  return { id, listenerId: "web", ...(match ? { match } : {}), handler };
}

function proxy(origin: string) {
  return { type: "reverseProxy", upstreams: [{ origin }] };
}

function vector(id: string): any {
  const found = vectors.scenarios.find((scenario: { id: string }) => scenario.id === id);
  if (!found) throw new Error(`missing settings vector: ${id}`);
  return found;
}

function startOrigin(id: string): Promise<{ origin: string; close: () => Promise<void> }> {
  const server = createServer((request, response) => {
    response.writeHead(200, { "Content-Type": "text/plain" });
    response.end(`${id}:${request.url}`);
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("could not allocate upstream port"));
        return;
      }
      resolve({
        origin: `http://127.0.0.1:${address.port}`,
        close: () => new Promise((done, fail) => server.close((error) => error ? fail(error) : done())),
      });
    });
  });
}

function runFixture(input: Record<string, unknown>): Promise<Record<string, any>> {
  return new Promise((resolve, reject) => {
    const child = spawn("go", ["run", "./tests/fixtures/caddy-activation"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
    });
    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];
    child.stdout.on("data", (chunk: Buffer) => stdout.push(chunk));
    child.stderr.on("data", (chunk: Buffer) => stderr.push(chunk));
    child.once("error", reject);
    child.once("close", (code) => {
      if (code !== 0) {
        reject(new Error(Buffer.concat(stderr).toString("utf8")));
        return;
      }
      try {
        resolve(JSON.parse(Buffer.concat(stdout).toString("utf8")));
      } catch (error) {
        reject(error);
      }
    });
    child.stdin.end(JSON.stringify(input));
  });
}

async function freePort(): Promise<number> {
  const server = createServer();
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("could not allocate test port");
  const { port } = address;
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
  return port;
}

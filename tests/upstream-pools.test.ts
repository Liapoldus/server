import { createServer, type Server } from "node:http";
import { spawn } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const settingsVectors = JSON.parse(readFileSync(path.join(root, "contracts/v1/settings-vectors.json"), "utf8"));

describe("Caddy upstream pool conformance", () => {
  it("executes the settings vector's smooth weighted round-robin sequence", async () => {
    const vector = settingsVectors.scenarios.find((scenario: { id: string }) => scenario.id === "smooth-weighted-round-robin-honors-configured-weights");
    expect(vector).toBeDefined();
    const originA = await startOrigin("a");
    const originB = await startOrigin("b");
    const port = await freePort();

    try {
      const origins = { a: originA.origin, b: originB.origin };
      const expected = vector.expected.sequence as Array<keyof typeof origins>;
      const settings = httpSettings(port, vector.input.upstreams.map((upstream: { id: keyof typeof origins; weight: number }) => ({
        origin: origins[upstream.id], weight: upstream.weight,
      })));
      const result = await runActivation({
        port,
        settings,
        requests: Array.from({ length: vector.input.selections }, () => ({ target: "/" })),
      });

      expect(result.responses.map((response: { body: string }) => response.body)).toEqual(expected);
    } finally {
      await Promise.all([originA.close(), originB.close()]);
    }
  }, 60_000);

  it("retries one refused connection and keeps that origin out of the pool during its cooldown", async () => {
    const failedAddress = await closedAddress();
    const port = await freePort();
    let announceHealthyRequest!: () => void;
    const healthyWasUsed = new Promise<void>((resolve) => { announceHealthyRequest = resolve; });
    let releaseHealthyResponse!: () => void;
    const healthyResponseGate = new Promise<void>((resolve) => { releaseHealthyResponse = resolve; });
    let healthyRequests = 0;
    const healthyServer = createServer(async (_request, response) => {
      healthyRequests++;
      if (healthyRequests === 1) {
        announceHealthyRequest();
        await healthyResponseGate;
      }
      response.end("healthy");
    });
    await listen(healthyServer);
    const healthyOrigin = `http://${serverAddress(healthyServer)}`;
    let reboundRequests = 0;
    const rebound = createServer((_request, response) => {
      reboundRequests++;
      response.end("rebound-after-refusal");
    });

    try {
      const settings = httpSettings(port, [
        { origin: `http://${failedAddress}`, weight: 100 },
        { origin: healthyOrigin, weight: 1 },
      ]);
      const resultPromise = runActivation({
        port,
        settings,
        requests: [{ target: "/first" }, { target: "/second" }],
      });
      await Promise.race([
        healthyWasUsed,
        resultPromise.then(() => { throw new Error("activation completed without selecting the healthy origin"); }),
      ]);
      await new Promise<void>((resolve, reject) => {
        rebound.once("error", reject);
        rebound.listen(Number(failedAddress.split(":").at(-1)), "127.0.0.1", resolve);
      });
      releaseHealthyResponse();
      const result = await resultPromise;

      expect(result.responses).toEqual([
        { status: 200, body: "healthy" },
        { status: 200, body: "healthy" },
      ]);
      expect(reboundRequests).toBe(0);
      expect(healthyRequests).toBe(2);
    } finally {
      releaseHealthyResponse();
      await Promise.all([close(healthyServer), close(rebound)]);
    }
  }, 60_000);

  it("limits connection-refusal failover to one additional origin", async () => {
    const failedA = await closedAddress();
    const failedB = await closedAddress();
    const third = await startOrigin("third-must-not-be-tried");
    const port = await freePort();

    try {
      const settings = httpSettings(port, [
        { origin: `http://${failedA}`, weight: 100 },
        { origin: `http://${failedB}`, weight: 99 },
        { origin: third.origin, weight: 1 },
      ]);
      const result = await runActivation({ port, settings, requests: [{ target: "/" }] });

      expect(result.responses[0].status).toBe(502);
      expect(third.requests).toBe(0);
    } finally {
      await third.close();
    }
  }, 60_000);

  it("does not retry an upstream failure after request bytes were sent", async () => {
    let failedOriginRequests = 0;
    const failedOrigin = createServer((_request, socket) => {
      failedOriginRequests++;
      socket.destroy();
    });
    await listen(failedOrigin);
    const failedAddress = serverAddress(failedOrigin);
    const fallback = await startOrigin("must-not-replay");
    const port = await freePort();

    try {
      const settings = httpSettings(port, [
        { origin: `http://${failedAddress}`, weight: 100 },
        { origin: fallback.origin, weight: 1 },
      ]);
      const result = await runActivation({ port, settings, requests: [{ target: "/", method: "POST" }] });

      expect(failedOriginRequests).toBe(1);
      expect(result.responses[0].status).toBe(502);
      expect(fallback.requests).toBe(0);
    } finally {
      await close(failedOrigin);
      await fallback.close();
    }
  }, 60_000);

  it("extends system trust with a scoped caRef and preserves hostname verification", async () => {
    const trustedPort = await freePort();
    const trusted = await runCAActivation({ port: trustedPort, trustCA: true });
    expect(trusted).toMatchObject({ statuses: [200], bodies: ["trusted-origin"], fallbackRequests: 0 });

    const untrustedPort = await freePort();
    const untrusted = await runCAActivation({ port: untrustedPort, trustCA: false });
    expect(untrusted.statuses).toEqual([502, 502]);
    expect(untrusted.bodies).toEqual(["", ""]);
    expect(untrusted.fallbackRequests).toBe(0);

    const mismatchedHostnamePort = await freePort();
    const mismatchedHostname = await runCAActivation({ port: mismatchedHostnamePort, trustCA: true, wrongHostname: true });
    expect(mismatchedHostname.statuses).toEqual([502, 502]);
    expect(mismatchedHostname.bodies).toEqual(["", ""]);
    expect(mismatchedHostname.fallbackRequests).toBe(0);
  }, 60_000);
});

function httpSettings(port: number, upstreams: Array<{ origin: string; weight: number }>) {
  return {
    schemaVersion: 1,
    config: {
      listeners: [{
        id: "web",
        kind: "http",
        address: `127.0.0.1:${port}`,
        hostnames: [],
        protocols: ["http1"],
        tls: { mode: "disabled" },
      }],
      routes: [{
        id: "weighted-pool",
        listenerId: "web",
        handler: { type: "reverseProxy", upstreams },
      }],
    },
  };
}

function runActivation(input: Record<string, unknown>): Promise<any> {
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

function runCAActivation(input: { port: number; trustCA: boolean; wrongHostname?: boolean }): Promise<any> {
  return new Promise((resolve, reject) => {
    const child = spawn("go", ["run", "./tests/fixtures/upstream-ca"], {
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

async function startOrigin(body: string, onRequest: () => void = () => {}): Promise<{
  origin: string;
  requests: number;
  close: () => Promise<void>;
}> {
  let requests = 0;
  const server = createServer((_request, response) => {
    requests++;
    onRequest();
    response.end(body);
  });
  await listen(server);
  const address = serverAddress(server);
  return {
    origin: `http://${address}`,
    get requests() { return requests; },
    close: () => close(server),
  };
}

async function closedAddress(): Promise<string> {
  const server = createServer();
  await listen(server);
  const address = serverAddress(server);
  await close(server);
  return address;
}

async function freePort(): Promise<number> {
  const server = createServer();
  await listen(server);
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("could not allocate listener port");
  const { port } = address;
  await close(server);
  return port;
}

function listen(server: Server): Promise<void> {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
}

function serverAddress(server: Server): string {
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("server has no TCP address");
  return `127.0.0.1:${address.port}`;
}

function close(server: Server): Promise<void> {
	if (!server.listening) return Promise.resolve();
	return new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
}

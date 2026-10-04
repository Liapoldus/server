import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

describe("HTTP stream runtime limits", () => {
  it("uses stream-specific duration, idle and concurrency bounds rather than the unary timeout", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/http-stream-limits"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      encoding: "utf8",
      timeout: 30_000,
    });
    const result = JSON.parse(output);

    expect(result.slow).toMatchObject({ status: 200, body: "stream-ok" });
    expect(result.slowElapsedMillis).toBeGreaterThanOrEqual(700);
    expect(result.slowElapsedMillis).toBeLessThan(1_500);
    expect(result.concurrent.status).toBe(503);
    expect(result.idle).toMatchObject({ status: 200, body: "" });
    expect(result.idleElapsedMillis).toBeGreaterThanOrEqual(900);
    expect(result.idleElapsedMillis).toBeLessThan(2_000);
    expect(result.maximum.status).toBe(200);
    expect(result.maximum.body).toContain("started");
    expect(result.maximumElapsedMillis).toBeGreaterThanOrEqual(1_700);
    expect(result.maximumElapsedMillis).toBeLessThan(3_000);
    expect(result.sse).toEqual({ status: 200, contentType: "text/event-stream", body: "event: update\ndata: first\ndata: second\nid: 17\nretry: 1000\n\n" });
    expect(result.invalidSSE).toEqual({ status: 200, body: "" });
    expect(result.afterStartOverflow).toEqual({ status: 200, body: "prefix" });
    expect(result.backpressure).toMatchObject({
      status: 200,
      completed: false,
      queueFull: true,
      pausedSentChunks: expect.any(Number),
      totalChunks: 4096,
    });
    expect(result.backpressure.pausedSentChunks).toBeLessThan(result.backpressure.totalChunks);
    expect(result.backpressure.bodyBytes).toBeGreaterThan(0);
    expect(result.backpressure.bodyBytes).toBeLessThan(result.backpressure.totalChunks * 24000);
  }, 35_000);
});

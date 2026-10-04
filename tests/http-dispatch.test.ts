import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

describe("Caddy plugin HTTP dispatch", () => {
  it("forwards unary requests and streams request/response bodies through the generic peer stream", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/http-dispatch"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      encoding: "utf8",
    });

    const result = JSON.parse(output);
    expect(result).toEqual({
      revision: "revision-1",
      status: 202,
      contentType: "text/plain",
      body: "plugin-response",
      setCookie: expect.arrayContaining([
        expect.stringContaining("session=ordinary-value"),
        expect.stringContaining("auth=http-only-value"),
      ]),
      method: "POST",
      path: "/submit",
      cookieForwarded: true,
      receivedCookies: [
        { name: "session", value: "allowed-value" },
        { name: "csrf", value: "csrf-value" },
        { name: "session", value: "second-value" },
      ],
      privateCookieForwarded: false,
      mutualTLS: true,
      invalidAction: { status: 502, setCookie: [], body: "" },
      streamStatus: 201,
      streamSetCookie: expect.arrayContaining([
        expect.stringContaining("stream-session=stream-value"),
        expect.stringContaining("HttpOnly"),
      ]),
      streamContentType: "text/plain",
      streamBody: "accepted:alpha-beta",
      streamReceived: "alpha-beta",
      streamMethod: "POST",
      streamPath: "/stream",
      oversizedStatus: 413,
      streamBytesRead: expect.any(Number),
    });
    expect(result.streamBytesRead).toBeGreaterThan(0);
    expect(result.streamBytesRead).toBeLessThanOrEqual(1_048_576 + 10);
  }, 60_000);
});

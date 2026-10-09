import { parseJSON } from '../support/json.js';
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

describe("WebSocket plugin dispatch runtime", () => {
  it("lets the plugin accept or reject before upgrade and preserves text/binary message boundaries", () => {
    const output = execFileSync("go", ["run", "./tests/fixtures/websocket-dispatch"], {
      cwd: root,
      env: { ...process.env, GOWORK: "off" },
      encoding: "utf8",
      timeout: 45_000,
    });
    const result = parseJSON<Record<string, unknown>>(output);

    expect(result.accepted).toEqual({
      status: 101,
      subprotocol: "forms.v1",
      setCookie: expect.arrayContaining([
        expect.stringContaining("session=ordinary") as unknown,
        expect.stringContaining("auth=opaque;") as unknown,
        expect.stringContaining("HttpOnly") as unknown,
      ]),
      received: [
        { type: "text", data: "hello" },
        { type: "binary", data: "AAEC/w==" },
      ],
      replies: [
        { type: "text", data: "accepted" },
        { type: "binary", data: "AQID" },
      ],
    });
    expect(result.rejected).toEqual({
      status: 403,
      setCookie: expect.arrayContaining([expect.stringContaining("csrf=ordinary")]) as unknown,
      upgraded: false,
    });
    expect(result.invalidSubprotocol).toEqual({ status: 502, upgraded: false });
    expect(result.invalidCookie).toEqual({ status: 502, setCookie: [], upgraded: false });
    expect(result.oversizedMessage).toEqual({ closeCode: 1009, pluginReceivesOversizedMessage: false });
  }, 60_000);
});

import Ajv2020 from "ajv/dist/2020.js";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const dispatch = JSON.parse(readFileSync(path.join(root, "contracts/v1/http-dispatch.json"), "utf8"));
const schema = JSON.parse(readFileSync(path.join(root, "contracts/v1", dispatch.responseCookies.schema), "utf8"));
const validate = new Ajv2020({ allErrors: true, strict: false }).compile(schema);

describe("HTTP response cookie contract", () => {
  it("allows ordinary and HttpOnly typed cookies in one response action", () => {
    expect(validate({
      status: 200,
      headers: { "Content-Type": "text/plain" },
      cookies: [
        { name: "preference", value: "compact", path: "/", secure: true, sameSite: "lax" },
        { name: "session", value: "opaque", path: "/", secure: true, httpOnly: true, sameSite: "strict" },
      ],
      body: "ok",
    })).toBe(true);
  });

  it("rejects generic Set-Cookie headers and insecure SameSite=None cookies", () => {
    expect(validate({ status: 200, headers: { "Set-Cookie": "session=opaque; HttpOnly" } })).toBe(false);
    expect(validate({ status: 200, cookies: [{ name: "session", value: "opaque", sameSite: "none" }] })).toBe(false);
  });
});

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const dispatch = JSON.parse(readFileSync(path.join(root, "contracts/v1/http-dispatch.json"), "utf8"));
const vectors = JSON.parse(readFileSync(path.join(root, "contracts/v1/http-stream-vectors.json"), "utf8"));

describe("HTTP_STREAM request-body contract", () => {
  it("counts decoded body octets, applies the existing request limit, and defines commit-aware overflow", () => {
    expect(dispatch.maxRequestBytes).toBe(1_048_576);
    expect(dispatch.requestBodySemantics).toEqual({
      count: "body-octets-after-transfer-framing",
      chunkedIncluded: true,
      modes: ["call", "http_stream"],
      overflowBeforeResponseStart: "requestTooLargeStatus-and-cancel-stream",
      overflowAfterResponseStart: "abort-stream-without-replacing-response",
    });
  });

  it("defines exact-limit, chunked overflow before commit, and overflow after commit vectors", () => {
    expect(vectors.contract).toBe("contracts/v1/http-dispatch.json");
    expect(vectors.scenarios).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: "http-stream-chunked-at-limit",
        mode: "http_stream",
        bodyLengthDeltaFromLimit: 0,
        responseStarted: false,
        expected: { forwardedBytes: "maxRequestBytes", requestEndSent: true, response: "plugin-response" },
      }),
      expect.objectContaining({
        id: "http-stream-chunked-over-limit-before-response-start",
        mode: "http_stream",
        bodyLengthDeltaFromLimit: 1,
        responseStarted: false,
        expected: { forwardedBytes: "maxRequestBytes", requestEndSent: false, response: "requestTooLargeStatus", cancelStream: true },
      }),
      expect.objectContaining({
        id: "http-stream-chunked-over-limit-after-response-start",
        mode: "http_stream",
        bodyLengthDeltaFromLimit: 1,
        responseStarted: true,
        expected: { forwardedBytes: "maxRequestBytes", response: "abort-stream", replaceResponse: false, cancelStream: true },
      }),
    ]));
  });
});

import { readContract } from '../support/contracts.js';
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { Ajv2020 } from "ajv/dist/2020.js";
import { describe, expect, it } from "vitest";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const dispatch = readContract('http-dispatch.json');
const streamContract = readContract('http-stream.json');
const streamSchema = readContract('http-stream.schema.json');
const vectors = readContract('http-stream-vectors.json');
const validateStreamContract = new Ajv2020({ allErrors: true }).compile(streamSchema);

describe("HTTP_STREAM request-body contract", () => {
  it("validates the versioned stream envelope used by the runtime", () => {
    expect(validateStreamContract(streamContract)).toBe(true);
    expect(streamContract.maxChunkBytes).toBeLessThan(streamContract.maxFrameBytes);
    expect(streamContract.websocket.maxMessageBytes).toBe(1_048_576);
    expect(streamContract.sse).toMatchObject({ eventKind: "sse_event", contentType: "text/event-stream", dataField: "data", maxRetryMillis: 2_147_483_647 });
    expect(validateStreamContract({ ...streamContract, requestEndKind: "" })).toBe(false);
  });

  it("counts decoded body octets, applies the existing request limit, and defines commit-aware overflow", () => {
    expect(dispatch.maxRequestBytes).toBe(1_048_576);
    expect(dispatch.maxStreamConcurrencyPerInstance).toBe(128);
    expect(dispatch.defaultStreamIdleTimeoutMillis).toBe(60_000);
    expect(dispatch.defaultStreamMaxDurationMillis).toBe(3_600_000);
    expect(dispatch.streamTimeoutStatus).toBe(504);
    expect(dispatch.requestBodySemantics).toEqual({
      count: "body-octets-after-transfer-framing",
      chunkedIncluded: true,
      modes: ["call", "http_stream", "sse"],
      overflowBeforeResponseStart: "requestTooLargeStatus-and-cancel-stream",
      overflowAfterResponseStart: "abort-stream-without-replacing-response",
    });
  });

  it("defines exact-limit, chunked overflow before commit, and overflow after commit vectors", () => {
    expect(vectors.contract).toBe("contracts/v1/http-dispatch.json");
    expect(vectors.scenarios.map((scenario) => scenario.id)).toEqual(expect.arrayContaining([
      "http-stream-limits-defaults",
      "http-stream-route-concurrency",
      "http-stream-idle-timeout",
      "http-stream-max-duration",
      "sse-structured-event-serialization",
      "sse-invalid-event-after-response-start",
      "websocket-plugin-accepts-before-upgrade-and-selects-offered-subprotocol",
      "websocket-plugin-rejects-before-upgrade",
      "websocket-unoffered-subprotocol-is-rejected-before-upgrade",
      "websocket-text-binary-message-boundaries",
      "websocket-message-over-limit-closes-with-1009",
    ]));
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

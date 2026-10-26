import { describe, expect, it } from "vitest";
import type { ClientError } from "./dto";
import { retryServerProbe } from "./queries";

function clientError(kind: ClientError["kind"], httpStatus: number | null = null): ClientError {
  return {
    kind,
    httpStatus,
    code: null,
    message: kind,
    requestId: null,
    details: {},
  };
}

describe("retryServerProbe", () => {
  it.each(["connection", "timeout"] as const)("retries %s failures up to three total attempts", (kind) => {
    const error = clientError(kind);
    expect(retryServerProbe(0, error)).toBe(true);
    expect(retryServerProbe(1, error)).toBe(true);
    expect(retryServerProbe(2, error)).toBe(false);
  });

  it.each([
    clientError("authentication", 401),
    clientError("api", 401),
    clientError("protocol"),
    clientError("configuration"),
    clientError("response_too_large"),
  ])("does not retry non-connection errors ($kind)", (error) => {
    expect(retryServerProbe(0, error)).toBe(false);
  });
});

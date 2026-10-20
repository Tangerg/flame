import { describe, expect, it } from "vitest";
import { normalizeRuntimeEndpoint, requireRuntimeEndpoint } from "./endpoint";
import { createHttpTransport } from "./transports/http";
import { createSidecarClient } from "./sidecar";

describe("Runtime address ownership", () => {
  it("canonicalizes a path prefix and rejects ambiguous or credential-bearing addresses", () => {
    expect(normalizeRuntimeEndpoint(" HTTPS://EXAMPLE.COM:443/runtime/ ")).toBe(
      "https://example.com/runtime",
    );
    expect(normalizeRuntimeEndpoint("https://example.com/%3F%23")).toBe(
      "https://example.com/%3F%23",
    );
    for (const input of [
      "/runtime",
      "file:///runtime",
      "https://user:secret@example.com",
      "https://example.com?token=secret",
      "https://example.com#token",
      "https://example.com/?",
      "https://example.com/#",
    ]) {
      expect(normalizeRuntimeEndpoint(input)).toBeNull();
      expect(() => requireRuntimeEndpoint(input)).toThrow("without credentials");
      expect(() => createHttpTransport({ baseUrl: input })).toThrow("without credentials");
      expect(() => createSidecarClient({ baseUrl: input })).toThrow("without credentials");
    }
  });
});

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { normalizeRuntimeEndpoint, requireRuntimeEndpoint } from "./endpoint";
import { createHttpTransport } from "./transports/http";
import { createSidecarClient } from "./sidecar";

describe("Runtime address ownership", () => {
  it("canonicalizes accepted addresses without reinterpreting escaped delimiters", () => {
    expect(normalizeRuntimeEndpoint(" HTTPS://EXAMPLE.COM:443/runtime/ ")).toBe(
      "https://example.com/runtime",
    );
    expect(normalizeRuntimeEndpoint("https://example.com/%3F%23")).toBe(
      "https://example.com/%3F%23",
    );
  });

  const fixtures: { input: string; accepted: boolean }[] = JSON.parse(
    readFileSync(
      new URL("../../go-sdk/httporigin/testdata/runtime-endpoints.json", import.meta.url),
      "utf8",
    ),
  );

  it.each(fixtures.filter(({ accepted }) => accepted))("accepts $input", ({ input }) => {
    expect(normalizeRuntimeEndpoint(input)).not.toBeNull();
  });

  it.each(fixtures.filter(({ accepted }) => !accepted))("rejects $input", ({ input }) => {
    expect(normalizeRuntimeEndpoint(input)).toBeNull();
    expect(() => requireRuntimeEndpoint(input)).toThrow("without credentials");
    expect(() => createHttpTransport({ baseUrl: input })).toThrow("without credentials");
    expect(() => createSidecarClient({ baseUrl: input })).toThrow("without credentials");
  });
});

it("refuses to attach a bearer to a non-loopback cleartext endpoint", () => {
  expect(() =>
    createHttpTransport({ baseUrl: "http://example.test", localToken: "example" }),
  ).toThrow("HTTPS");
  expect(() =>
    createHttpTransport({ baseUrl: "https://example.test", localToken: "example" }),
  ).not.toThrow();
  expect(() =>
    createHttpTransport({ baseUrl: "http://127.0.0.1:8080", localToken: "example" }),
  ).not.toThrow();
});

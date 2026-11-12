import { expect, it, vi } from "vitest";
import { HTTP_ENDPOINTS } from "@flame/runtime-client/wire";
import { RpcConnectionError } from "../errors";
import { createHttpTransport, type HttpTransportConfig } from "./http";

it.each(["token\rnext", "token\nnext"])(
  "refuses line breaks in credentials before creating a transport",
  (localToken) => {
    const fetchStub = vi.fn<typeof fetch>();
    expect(() =>
      createHttpTransport({ baseUrl: "https://runtime.example", localToken, fetch: fetchStub }),
    ).toThrow("Bearer credentials must not contain line breaks");
    expect(fetchStub).not.toHaveBeenCalled();
  },
);

it.each([
  { baseUrl: "http://runtime.example", localToken: undefined },
  { baseUrl: "https://runtime.example", localToken: "original-token" },
])("captures credentials with the admitted endpoint $baseUrl", async ({ baseUrl, localToken }) => {
  const failure = new TypeError("connection refused");
  const fetchStub = vi.fn<typeof fetch>(async () => {
    throw failure;
  });
  const config: HttpTransportConfig = { baseUrl, localToken, fetch: fetchStub };
  const transport = createHttpTransport(config);
  config.localToken = "replacement-token";
  config.baseUrl = "https://replacement.example";

  try {
    await expect(
      transport.send({ jsonrpc: "2.0", id: "1", method: "runtime.discover", params: {} }),
    ).rejects.toBeInstanceOf(RpcConnectionError);
    const [url, options] = fetchStub.mock.calls[0]!;
    expect(url).toBe(`${baseUrl}${HTTP_ENDPOINTS.rpc.path}`);
    expect(new Headers(options?.headers).get("Authorization")).toBe(
      localToken === undefined ? null : `Bearer ${localToken}`,
    );
  } finally {
    await transport.close();
  }
});

it("retains the admitted SSE byte capacity after the caller edits its configuration", async () => {
  const response = `data: ${JSON.stringify({ jsonrpc: "2.0", id: "1", result: "界".repeat(40) })}\n\n`;
  const config: HttpTransportConfig = {
    baseUrl: "http://runtime.example",
    maximumEventStreamFrameBytes: 100,
    fetch: async () => new Response(response, { headers: { "Content-Type": "text/event-stream" } }),
  };
  const transport = createHttpTransport(config);
  config.maximumEventStreamFrameBytes = 1000;
  const events = transport.recv()[Symbol.asyncIterator]();

  try {
    await transport.send({ jsonrpc: "2.0", id: "1", method: "runs.start", params: {} });
    await expect(events.next()).resolves.toMatchObject({
      value: {
        type: "requestError",
        error: { message: "event-stream frame exceeds its byte capacity" },
      },
    });
  } finally {
    await transport.close();
  }
});

it("preserves the original fetch failure at the transport boundary", async () => {
  const failure = new TypeError("connection refused");
  const transport = createHttpTransport({
    baseUrl: "http://runtime.example",
    fetch: async () => {
      throw failure;
    },
  });
  try {
    await expect(
      transport.send({ jsonrpc: "2.0", id: "1", method: "runtime.discover", params: {} }),
    ).rejects.toMatchObject({ name: "RpcConnectionError", cause: failure });
  } finally {
    await transport.close();
  }
});

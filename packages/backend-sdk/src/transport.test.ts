import { PassThrough } from "node:stream";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { z } from "zod";
import { expect, it } from "vitest";
import { ExactNumber, stringifyJSON } from "@flame/runtime-client";
import { StdioTransport } from "./transport";

it.each([
  Buffer.from(
    'dependency wrote directly to stdout\n{"jsonrpc":"2.0","method":"ignored"}\n',
  ),
  Buffer.from([0xff, 10]),
  Buffer.from('{"jsonrpc":"2.0"'),
])(
  "closes malformed stdout or incomplete EOF without parsing around it",
  async (bytes) => {
    const input = new PassThrough();
    const transport = new StdioTransport(input, new PassThrough());
    const closed = Promise.withResolvers<void>();
    let messages = 0,
      failures = 0;
    transport.onmessage = () => {
      messages++;
    };
    transport.onerror = () => {
      failures++;
    };
    transport.onclose = () => closed.resolve();
    await transport.start();
    input.end(bytes);
    await closed.promise;
    expect(messages).toBe(0);
    expect(failures).toBe(1);
    await transport.close();
  },
);

it("uses the maintained MCP protocol for exact custom messages and reentrant calls", async () => {
  const a = new PassThrough(),
    b = new PassThrough();
  const client = new Client(
    { name: "conformance", version: "1" },
    { capabilities: {} },
  );
  const server = new Server(
    { name: "conformance", version: "1" },
    { capabilities: {} },
  );
  const nested = z.object({
    method: z.literal("flame.test/nested"),
    params: z.object({ value: z.unknown() }),
  });
  client.setRequestHandler(nested, (request) =>
    Promise.resolve({ value: request.params.value }),
  );
  server.setRequestHandler(
    z.object({
      method: z.literal("flame.test/exact"),
      params: z.object({
        value: z.unknown(),
        _meta: z.record(z.string(), z.unknown()),
      }),
    }),
    async (request, context) => {
      const reply = await server.request(
        {
          method: "flame.test/nested",
          params: { value: request.params.value },
        },
        z.object({ value: z.unknown() }),
        { signal: context.signal },
      );
      return { value: reply.value, metadata: request.params._meta };
    },
  );
  await server.connect(new StdioTransport(a, b));
  await client.connect(new StdioTransport(b, a));
  try {
    const value = {
      integer: 9007199254740993n,
      decimal: new ExactNumber("1.0"),
      exponent: new ExactNumber("1e99999"),
    };
    const result = await client.request(
      {
        method: "flame.test/exact",
        params: { value, _meta: { original: value } },
      },
      z.object({ value: z.unknown(), metadata: z.unknown() }),
    );
    expect(stringifyJSON(result.value)).toBe(stringifyJSON(value));
    expect(stringifyJSON(result.metadata)).toBe(
      stringifyJSON({ original: value }),
    );
  } finally {
    await client.close();
    await server.close();
  }
});

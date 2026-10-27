import { PassThrough } from "node:stream";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { z } from "zod";
import { expect, it } from "vitest";
import { ExactNumber, stringifyJSON } from "@qimu/sdk/json";
import { StdioTransport } from "./transport.ts";

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
    const transport = new StdioTransport(input, new PassThrough(), 1024 * 1024);
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
  await server.connect(new StdioTransport(a, b, 1024 * 1024));
  await client.connect(new StdioTransport(b, a, 1024 * 1024));
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

it("bounds individual frames without rejecting a chunk containing several small frames", async () => {
  const input = new PassThrough();
  const output = new PassThrough();
  const frame = '{"jsonrpc":"2.0","method":"notice"}';
  const transport = new StdioTransport(input, output, Buffer.byteLength(frame));
  let received = 0;
  let failures = 0;
  transport.onmessage = () => {
    received++;
  };
  transport.onerror = () => {
    failures++;
  };
  await transport.start();
  input.write(`${frame}\n${frame}\n`);
  expect(received).toBe(2);
  input.write("x".repeat(Buffer.byteLength(frame) + 1));
  expect(failures).toBe(1);
  expect(received).toBe(2);
  await transport.close();
});

it("refuses oversized outgoing frames before writing", async () => {
  const output = new PassThrough();
  const transport = new StdioTransport(new PassThrough(), output, 32);
  let bytes = 0;
  output.on("data", (chunk: Buffer) => {
    bytes += chunk.length;
  });
  await transport.start();
  await expect(
    transport.send({
      jsonrpc: "2.0",
      method: "notice",
      params: { value: "x".repeat(32) },
    }),
  ).rejects.toThrow("capacity");
  expect(bytes).toBe(0);
  await transport.close();
});

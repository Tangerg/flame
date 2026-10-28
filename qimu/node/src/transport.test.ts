import { PassThrough, Writable } from "node:stream";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { z } from "zod";
import { expect, it } from "vitest";
import { ExactNumber, stringifyJSON } from "@qimu/sdk/json";
import { StdioTransport } from "./transport.ts";

it.each([true, false])(
  "every close joins the original pending write and keeps its failure listener (autoDestroy=%s)",
  async (autoDestroy) => {
    const writing = Promise.withResolvers<(error?: Error | null) => void>();
    const output = new Writable({
      autoDestroy,
      write(_chunk, _encoding, complete) {
        writing.resolve(complete);
      },
    });
    const input = new PassThrough();
    const transport = new StdioTransport(input, output, 1024);
    let failures = 0;
    let notifications = 0;
    transport.onerror = () => {
      failures++;
    };
    transport.onclose = () => {
      notifications++;
    };
    await transport.start();
    const sent = transport
      .send({ jsonrpc: "2.0", method: "notice" })
      .catch((error: unknown) => error);
    const complete = await writing.promise;
    let closed = false;
    const first = transport.close();
    const second = transport.close().then(() => {
      closed = true;
    });
    await Promise.resolve();
    expect(closed).toBe(false);
    expect(notifications).toBe(0);
    complete(new Error("original write failed"));
    expect(await sent).toBeInstanceOf(Error);
    await Promise.all([first, second]);
    expect(closed).toBe(true);
    expect(failures).toBe(1);
    expect(notifications).toBe(1);
    expect(output.listenerCount("error")).toBe(0);
  },
);

it.each([true, false])(
  "joins delayed stream destruction before reporting close (emitClose=%s)",
  async (emitClose) => {
    const writing = Promise.withResolvers<(error?: Error | null) => void>();
    const destroying = Promise.withResolvers<(error?: Error | null) => void>();
    const failure = new Error("original write failed");
    const output = new Writable({
      emitClose,
      write(_chunk, _encoding, complete) {
        writing.resolve(complete);
      },
      destroy(error, complete) {
        expect(error).toBe(failure);
        destroying.resolve(complete);
      },
    });
    const transport = new StdioTransport(new PassThrough(), output, 1024);
    const failures: Error[] = [];
    let notifications = 0;
    transport.onerror = (error) => failures.push(error);
    transport.onclose = () => {
      notifications++;
    };
    await transport.start();
    const sent = transport
      .send({ jsonrpc: "2.0", method: "notice" })
      .catch((error: unknown) => error);
    const completeWrite = await writing.promise;
    let closed = false;
    const closing = transport.close().then(() => {
      closed = true;
    });
    completeWrite(failure);
    const completeDestroy = await destroying.promise;
    await Promise.resolve();
    expect(closed).toBe(false);
    expect(notifications).toBe(0);
    expect(failures).toHaveLength(0);
    completeDestroy(failure);
    expect(await sent).toBe(failure);
    await closing;
    expect(closed).toBe(true);
    expect(notifications).toBe(1);
    expect(failures).toHaveLength(1);
    expect(failures[0]?.cause).toBe(failure);
    expect(output.listenerCount("error")).toBe(0);
  },
);

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

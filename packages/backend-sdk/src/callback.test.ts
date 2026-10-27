import { createServer, type IncomingMessage, type Server } from "node:http";
import { once } from "node:events";
import { afterEach, expect, it } from "vitest";
import { parseJSON, stringifyJSON } from "@flame/runtime-client";
import { callHost, decodeReply, Refusal, type Invocation } from "./callback";
import { Method } from "./contract";
import wire from "./protocol.generated.json";

const servers: Server[] = [];
afterEach(async () => {
  for (const server of servers.splice(0)) {
    server.closeAllConnections();
    await new Promise<void>((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    );
  }
});
const method = new Method({
  name: "fixture.callback",
  kind: "query",
  target: "installation",
  description: "Read through the captured invocation.",
  input: { type: "object" },
  output: { type: "boolean" },
});
async function host(reply: unknown) {
  const received = Promise.withResolvers<{
    header: IncomingMessage["headers"][string];
    body: string;
  }>();
  const server = createServer((request, response) => {
    const chunks: Buffer[] = [];
    request.on("data", (chunk: Buffer) => chunks.push(chunk));
    request.on("error", received.reject);
    request.on("end", () => {
      received.resolve({
        header: request.headers[wire.constants.InvocationHeader!.toLowerCase()],
        body: Buffer.concat(chunks).toString("utf8"),
      });
      response.setHeader("Content-Type", "application/json");
      response.end(stringifyJSON(reply));
    });
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  servers.push(server);
  const address = server.address();
  if (!address || typeof address === "string")
    throw new Error("callback fixture requires an IP listener");
  return {
    ref: {
      endpoint: `http://127.0.0.1:${address.port}/`,
      credential: "1".repeat(64),
      call: "2".repeat(64),
      maxResponseBytes: 1 << 20,
    },
    received: received.promise,
  };
}

it.each(["plain", "café", "工作区", "😀", 'literal-\\u4e2d/"quoted"'])(
  "preserves %s in the original invocation through HTTP",
  async (text) => {
    const fixture = await host({ value: true });
    const invocation: Invocation = Object.freeze({
      id: `call-${text}`,
      origin: "model",
      parent: `parent-${text}`,
      target: Object.freeze({
        scope: "installation",
        reference: `/workspace/${text}`,
      }),
    });
    const input = parseJSON(
      '{"wide":9007199254740993,"decimal":1.0,"text":"工作区"}',
    );
    expect(
      await callHost(
        fixture.ref,
        method,
        input,
        new AbortController().signal,
        invocation,
      ),
    ).toBe(true);
    const received = await fixture.received;
    expect(typeof received.header).toBe("string");
    const header = received.header as string;
    expect(header).toMatch(/^[\x20-\x7e]+$/);
    expect(parseJSON(header)).toEqual(invocation);
    const body = parseJSON(received.body) as { input: unknown };
    expect(stringifyJSON(body.input)).toBe(stringifyJSON(input));
  },
);

it("does not invent invocation metadata when none was captured", async () => {
  const fixture = await host({ value: true });
  await callHost(fixture.ref, method, {}, new AbortController().signal);
  expect((await fixture.received).header).toBeUndefined();
});

it.each(["", "known refusal", "拒绝 😀"])(
  "keeps a known refusal with message %j as the original business outcome",
  (message) => {
    let received: unknown;
    try {
      decodeReply({ failure: { code: "denied", message } });
    } catch (error) {
      received = error;
    }
    expect(received).toBeInstanceOf(Refusal);
    expect(received).toMatchObject({ code: "denied", message });
    expect(new Refusal("denied", message).message).toBe(message);
  },
);

it("preserves an empty-message refusal received through the host transport", async () => {
  const fixture = await host({ failure: { code: "denied", message: "" } });
  await expect(
    callHost(fixture.ref, method, {}, new AbortController().signal),
  ).rejects.toMatchObject({ name: "Refusal", code: "denied", message: "" });
});

it.each([
  { code: "", message: "known" },
  { code: "denied" },
  { code: "denied", message: null },
  { code: 1, message: "known" },
  { code: "\ud800", message: "known" },
  { code: "denied", message: "\udfff" },
  { code: "denied", message: "known", extra: true },
])("does not classify malformed refusal %j as a business result", (failure) => {
  let received: unknown;
  try {
    decodeReply({ failure });
  } catch (error) {
    received = error;
  }
  expect(received).toBeInstanceOf(Error);
  expect(received).not.toBeInstanceOf(Refusal);
});

it("does not reinterpret an ambiguous reply as a refusal", () => {
  expect(() =>
    decodeReply({ value: true, failure: { code: "denied", message: "" } }),
  ).toThrow();
  expect(decodeReply({ value: null })).toBeNull();
});

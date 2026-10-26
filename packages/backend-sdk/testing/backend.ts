import { readFileSync, writeFileSync } from "node:fs";
import "./logging";
import {
  Backend,
  Capability,
  Method,
  Refusal,
  callHost,
  parseJSON,
  stringifyJSON,
  type CapabilityDescriptor,
  type MethodDescriptor,
  type CallContext,
} from "@flame/backend-sdk";

console.info("conformance module loaded");
const contract = new Capability(
  parseJSON(
    readFileSync(new URL("./contract.json", import.meta.url), "utf8"),
  ) as CapabilityDescriptor,
);
const host = new Method(
  parseJSON(
    readFileSync(new URL("./host.json", import.meta.url), "utf8"),
  ) as MethodDescriptor,
);
const backend = new Backend({
  name: "flame.typescript.conformance",
  version: "1",
});
const receipts = new Map<
  string,
  { method: string; digest: string; reply: unknown }
>();
type Input = { mode: string; value: unknown };
const decode = (value: unknown) => value as Input;
const result = async (context: CallContext, input: Input) => {
  if (input.mode === "refuse")
    throw new Refusal("fixture_refused", "fixture refused the operation");
  const value =
    input.mode === "host"
      ? (
          (await callHost(
            context.callback!,
            host,
            { value: input.value },
            context.signal,
            context.invocation,
          )) as { value: unknown }
        ).value
      : input.value;
  return { value, source: context.requestInput };
};
backend.implement(contract.method("fixture.read"), decode, result);
backend.implement(
  contract.method("fixture.write"),
  decode,
  async (context, input) => {
    const previous = receipts.get(context.commandId!);
    if (previous) {
      if (
        previous.method !== "fixture.write" ||
        previous.digest !== context.inputDigest
      )
        throw new Refusal("command_conflict", "original command input differs");
      return (previous.reply as { value: unknown }).value;
    }
    const output = await result(context, input);
    receipts.set(context.commandId!, {
      method: "fixture.write",
      digest: context.inputDigest,
      reply: { value: output },
    });
    if (input.mode === "invalid") return { source: context.requestInput };
    return output;
  },
);
for (const name of ["fixture.query-stream", "fixture.command-stream"])
  backend.implementStream(
    contract.method(name),
    decode,
    async function* (context, input) {
      try {
        yield { kind: "chunk", value: input.value };
        if (input.mode === "missing") return;
        if (input.mode === "wait") {
          context.signal.throwIfAborted();
          await new Promise<void>((resolve) =>
            context.signal.addEventListener("abort", () => resolve(), {
              once: true,
            }),
          );
          context.signal.throwIfAborted();
        }
        const output = await result(context, input);
        if (name === "fixture.command-stream")
          receipts.set(context.commandId!, {
            method: name,
            digest: context.inputDigest,
            reply: { value: output },
          });
        yield { kind: "result", value: output };
        if (input.mode === "mutated-final")
          output.source = "changed after yielding the final result";
        if (input.mode === "late") yield { kind: "chunk", value: "late" };
      } finally {
        if (input.mode === "wait")
          writeFileSync(new URL("./cancelled.txt", import.meta.url), "joined");
      }
    },
  );
backend.receipts((_context, input) => {
  const receipt = receipts.get(input.id);
  if (!receipt) return { found: false };
  if (receipt.method !== input.method || receipt.digest !== input.inputDigest)
    throw new Refusal("command_conflict", "receipt identity differs");
  return { found: true, reply: receipt.reply };
});
backend.toolReceipts((_context, input) => {
  const receipt = receipts.get(input.id);
  if (!receipt) return { found: false };
  if (receipt.method !== input.method || receipt.digest !== input.inputDigest)
    throw new Refusal("command_conflict", "Tool receipt identity differs");
  return { found: true, output: (receipt.reply as { value: string }).value };
});
backend.bindCapability("fixture", contract);
backend.onSetup(() => {});
backend.tool(
  {
    name: "host_echo",
    description: "Use a captured host collaborator.",
    inputSchema: {
      type: "object",
      properties: { value: {} },
      required: ["value"],
      additionalProperties: false,
    },
  },
  async (context, input) => {
    const output = await result(context, {
      mode: "host",
      value: (input as Input).value,
    });
    receipts.set(context.toolCommandId(), {
      method: "host_echo",
      digest: context.inputDigest,
      reply: {
        value: stringifyJSON({
          content: [{ kind: "text", text: output.source }],
        }),
      },
    });
    return { content: [{ type: "text", text: output.source }] };
  },
);
export default backend;

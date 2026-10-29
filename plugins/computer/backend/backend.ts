import { METHODS } from "../contracts/methods";
import { mkdirSync } from "node:fs";
import { isAbsolute, join } from "node:path";
import { toolReceipts } from "@flame/backend-sdk";
import { Backend, Capability, canonicalJSON } from "@qimu/node";
import { projectNumber } from "@qimu/sdk/json";
import { z } from "zod";
import descriptor, { schemas, type Snapshot } from "../contracts/capability";
import { LIMITS } from "../contracts/limits";
import manifest from "../package/plugin.json";
import { configuration } from "./configuration";
import { Control, type ConnectEngine } from "./control";
import { Receipts } from "./receipts";

export function createBackend(
  environment: NodeJS.ProcessEnv = process.env,
  connect?: ConnectEngine,
): Backend {
  const data = environment.PLUGIN_DATA;
  if (!data || !isAbsolute(data))
    throw new Error("computer: absolute plugin data directory is required");
  const config = configuration(environment, data);
  const contract = new Capability(descriptor);
  const backend = new Backend({
    name: manifest.name,
    version: manifest.version,
  });
  let control: Control;
  let receipts: Receipts;
  backend.onSetup((context) => {
    mkdirSync(data, { recursive: true, mode: 0o700 });
    receipts = new Receipts(join(data, "computer.db"), LIMITS);
    context.defer(() => receipts.close());
    control = new Control(config, receipts, context.signal, connect);
    return { close: () => control.close() };
  });
  backend.implement(
    contract.method(METHODS.read),
    (value) => schemas.empty.parse(value),
    () => control.snapshot(),
  );
  backend.implementStream<z.infer<typeof schemas.empty>, Snapshot, boolean>(
    contract.method(METHODS.watch),
    (value) => schemas.empty.parse(value),
    (context) => control.watch(context.signal),
  );
  backend.implement(
    contract.method(METHODS.context),
    (value) => schemas.empty.parse(value),
    () => control.context(),
  );
  backend.implement(
    contract.method(METHODS.open),
    (value) =>
      schemas.open.parse({
        ...(value as object),
        leaseSeconds: projectNumber(
          (value as { leaseSeconds: unknown }).leaseSeconds,
        ),
      }),
    (context, input) => control.open(context, input.leaseSeconds),
  );
  backend.implement(
    contract.method(METHODS.close),
    (value) => schemas.sessionInput.parse(value),
    (context, input) => control.closeSession(context, input.sessionId),
  );
  backend.implement(
    contract.method(METHODS.stop),
    (value) => schemas.stop.parse(value),
    (context, input) =>
      control.stop(context, input.sessionId, input.operationId),
  );
  backend.implement(
    contract.method(METHODS.execute),
    (value) => schemas.execute.parse(value),
    (context, input) => control.execute(context, input),
  );
  backend.implement(
    contract.method(METHODS.reply),
    (value) => schemas.reply.parse(value),
    (context, input) => control.reply(context, input),
  );
  backend.implement(
    contract.method(METHODS.history),
    (value) =>
      schemas.history.parse({
        ...(value as object),
        limit: projectNumber((value as { limit: unknown }).limit),
      }),
    (_context, input) => control.history(input.before, input.limit),
  );
  backend.implement(
    contract.method(METHODS.result),
    (value) => schemas.resultInput.parse(value),
    (_context, input) => control.result(input.id),
  );
  backend.receipts((_context, request) => receipts.receipt(request));
  toolReceipts(backend, (_context, request) => receipts.toolReceipt(request));
  backend.bindCapability("control", contract);

  backend.tool(
    {
      name: "context",
      title: "Read computer-use instructions and current targets",
      description:
        "Read the current computer-control session, original CUA engine instructions, and complete js/js_reset argument schemas before using computer_js or computer_reset. Follow those original instructions and pass the exact returned sessionId, workerId and surfaceDigest. The human operator must first open a time-limited control session. The computer is the Runtime host, which may differ from the user's browser device. This only reads context; it never opens a session or grants permissions.",
      inputSchema: z.toJSONSchema(schemas.empty) as { type: "object" },
      annotations: {
        readOnlyHint: true,
        destructiveHint: false,
        openWorldHint: false,
      },
    },
    (context) => {
      context.signal.throwIfAborted();
      const surface = control.context();
      return {
        structuredContent: surface,
        content: [{ type: "text", text: canonicalJSON(surface) }],
      };
    },
  );
  const input = schemas.execute.omit({ tool: true });
  for (const [name, tool] of [
    ["js", "js"],
    ["reset", "js_reset"],
  ] as const) {
    backend.tool(
      {
        name,
        title:
          tool === "js"
            ? "Use the controlled computer"
            : "Reset the computer-use JavaScript worker",
        description:
          tool === "js"
            ? "Execute the original CUA js tool using its full captured argument schema. First read computer_context and follow the runtime's instructions. Encode the original js arguments as argumentsJSON, including code and any declared title/timeout fields. This supports desktop apps, browser control, screenshots and other APIs actually advertised by that runtime. Original text/images/audio/resources are returned intact. One control turn is ended after this operation; the JavaScript heap remains until reset or session retirement. Never replay an unconfirmed action after transport loss. Permissions must be answered by the human operator."
            : "Explicitly invoke the original js_reset tool on the accepted computer session. Encode its original arguments as argumentsJSON. This discards JavaScript variables and references. Read computer_context again afterward for the new accepted worker identity. Never use this to recover or retry an unconfirmed earlier action.",
        inputSchema: z.toJSONSchema(input) as { type: "object" },
        annotations: {
          readOnlyHint: false,
          destructiveHint: true,
          idempotentHint: false,
          openWorldHint: true,
        },
      },
      (context, value) =>
        control.tool(context, name, { ...input.parse(value), tool }),
    );
  }
  return backend;
}

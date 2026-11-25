import type { Transport } from "@modelcontextprotocol/sdk/shared/transport.js";
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import type { RequestHandlerExtra } from "@modelcontextprotocol/sdk/shared/protocol.js";
import type {
  Request,
  Notification,
  Tool,
  CallToolResult,
} from "@modelcontextprotocol/sdk/types.js";
import {
  CallToolResultSchema,
  ToolSchema,
} from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";
import { parseJSON } from "@frame/sdk/json";
import { Lifetime, FrameError, type CloseOutcome } from "@frame/sdk";
import {
  Capability,
  assertProtocol,
  declareMethod,
  type Method,
  PROTOCOL,
  canonicalJSON,
  compareTokens,
  exactFields,
  freezeJSON,
  validName,
} from "./contract.ts";
import { callbackRef, Refusal } from "./callback.ts";
import wire from "./protocol.generated.json";
import {
  type CallContext,
  type ReceiptRequest,
  callContext,
  callMetadata,
  decodeReceipt,
} from "./invocation.ts";
import { deliverStream, type StreamFrame } from "./stream.ts";
import {
  type ActivationResource,
  type SetupContribution,
  type SetupHandler,
  assertBackground,
  validateRequirements,
} from "./setup.ts";

type Decoder<P> = (value: unknown, source: string) => P;

interface BoundMethod {
  method: Method;
  decode: Decoder<unknown>;
  handler?: (
    context: CallContext,
    input: unknown,
  ) => unknown | Promise<unknown>;
  stream?: (
    context: CallContext,
    input: unknown,
  ) => AsyncIterable<StreamFrame<unknown, unknown>>;
}

type Extra = RequestHandlerExtra<Request, Notification>;

function decodeAdmitted(bound: BoundMethod, admitted: unknown): unknown {
  return bound.decode(admitted, canonicalJSON(admitted));
}

export class Backend {
  readonly #server: Server;
  readonly #methods = new Map<string, BoundMethod>();
  readonly #capabilities = new Map<string, Capability>();
  readonly #tools = new Map<
    string,
    {
      descriptor: Tool;
      admitInput: (input: unknown) => unknown;
      admitOutput?: (output: unknown) => unknown;
      handler: (
        context: CallContext,
        input: unknown,
      ) => CallToolResult | Promise<CallToolResult>;
    }
  >();
  readonly #lifetime = new Lifetime("backend");
  readonly #activation = this.#lifetime.branch("activation");
  #published = false;
  #setup?: SetupHandler;
  #setupResult?: Promise<ActivationResource | void>;
  #start?: Promise<void>;

  constructor(info: { name: string; version: string }) {
    this.#server = new Server(info, { capabilities: { tools: {} } });
    this.#lifetime.cleanup(() => this.#server.close(), "mcp-transport");
    this.#lifetime.observeExit((outcome) => {
      if (outcome.status === "released") {
        this.#methods.clear();
        this.#capabilities.clear();
        this.#tools.clear();
        this.#setup = undefined;
        this.#setupResult = undefined;
        this.#start = undefined;
      }
    });
    this.#receive("tools/list", (params) => this.#listTools(params));
    this.#receive("tools/call", (params, extra) =>
      this.#callTool(params, extra),
    );
    this.#receive(wire.constants.describeMethod!, (params) =>
      this.#describe(params),
    );
    this.#receive(wire.constants.validateMethod!, (params) =>
      this.#validate(params),
    );
    this.#receive(wire.constants.invokeMethod!, (params, extra) =>
      this.#invoke(params, extra),
    );
    this.#receive(wire.constants.setupMethod!, (params, extra) =>
      this.#supplySetup(params, extra),
    );
    this.#receive(wire.constants.activateMethod!, (params) =>
      this.#activate(params),
    );
  }

  #listTools(params: Record<string, unknown>) {
    exactFields(params, [], ["cursor", "_meta"]);
    if (params.cursor !== undefined)
      throw new Error("plugin: unsupported tool cursor");
    return {
      tools: [...this.#tools.values()].map((tool) => tool.descriptor),
    };
  }
  async #callTool(params: Record<string, unknown>, extra: Extra) {
    exactFields(params, ["name"], ["arguments", "_meta"]);
    const tool = this.#tools.get(params.name as string);
    if (!tool) throw new Error("plugin: tool is not published");
    const input = tool.admitInput(params.arguments ?? {});
    const call = this.#call(extra.signal);
    try {
      const context = callContext({ input, _meta: params._meta }, call);
      context.signal.throwIfAborted();
      let output: CallToolResult;
      try {
        output = await call.run(() => tool.handler(context, input));
      } catch (cause) {
        if (cause instanceof Refusal)
          return {
            isError: true,
            content: [
              { type: "text", text: cause.code + ": " + cause.message },
            ],
          };
        throw new Error("plugin: tool outcome could not be confirmed", {
          cause,
        });
      }
      const captured = CallToolResultSchema.parse(freezeJSON(output));
      if (tool.admitOutput && !captured.isError)
        return CallToolResultSchema.parse({
          ...captured,
          structuredContent: tool.admitOutput(captured.structuredContent),
        });
      return captured;
    } finally {
      await closeOwned(call);
    }
  }
  #describe(params: Record<string, unknown>) {
    exactFields(params, ["protocol"], ["_meta"]);
    assertProtocol(params);
    return {
      value: {
        protocol: PROTOCOL,
        methods: [...this.#methods.values()]
          .map(({ method }) => method.descriptor)
          .sort((a, b) => compareTokens(a.name, b.name)),
        capabilities: [...this.#capabilities]
          .map(([name, contract]) => ({
            name,
            contract: contract.descriptor,
          }))
          .sort((a, b) => compareTokens(a.name, b.name)),
      },
    };
  }
  #validate(params: Record<string, unknown>) {
    const bound = this.#resolve(params);
    if (params.commandId !== undefined)
      throw new Error("plugin: validation cannot carry a command identity");
    try {
      decodeAdmitted(bound, bound.method.admitInput(params.input));
      return { value: "" };
    } catch {
      return { value: "plugin: input is not structurally admitted" };
    }
  }
  async #invoke(params: Record<string, unknown>, extra: Extra) {
    const bound = this.#resolve(params);
    const input = decodeAdmitted(
      bound,
      bound.method.admitInvocation(params.input, params.commandId),
    );
    const call = this.#call(extra.signal);
    try {
      const context = callContext(
        params,
        call,
        bound.method.descriptor.name === wire.constants.receiptMethod!
          ? undefined
          : bound.method.descriptor.target,
      );
      context.signal.throwIfAborted();
      let value: unknown;
      try {
        value = await call.run(() =>
          bound.stream
            ? deliverStream(
                () => bound.stream!(context, input),
                bound.method,
                context,
                callMetadata(params)[wire.constants.streamMetadataKey!],
              )
            : bound.handler!(context, input),
        );
      } catch (error) {
        if (error instanceof Refusal)
          return { failure: { code: error.code, message: error.message } };
        throw new Error("plugin: operation could not be confirmed", {
          cause: error,
        });
      }
      return { value: bound.method.admitOutput(value) };
    } finally {
      await closeOwned(call);
    }
  }
  async #supplySetup(params: Record<string, unknown>, extra: Extra) {
    exactFields(params, ["protocol", "contributions"], ["_meta"]);
    assertProtocol(params);
    if (this.#setupResult || !Array.isArray(params.contributions))
      throw new Error("plugin: setup is invalid or already supplied");
    const contributions: unknown = freezeJSON(params.contributions);
    this.#validateSetup(contributions);
    const metadata = callMetadata(params);
    const ref =
      metadata[wire.constants.callbackMetadataKey!] === undefined
        ? undefined
        : callbackRef(metadata[wire.constants.callbackMetadataKey!]);
    const abort = () => this.#activation.beginClose();
    extra.signal.throwIfAborted();
    extra.signal.addEventListener("abort", abort, { once: true });
    let resource: ActivationResource | undefined;
    this.#activation.cleanup(() => resource?.close(), "activation-resource");
    this.#setupResult = this.#activation.run(async () => {
      resource =
        (await this.#setup?.({
          signal: this.#activation.signal,
          lifetime: this.#activation,
          contributions,
          callback: ref,
          defer: (cleanup) => this.#activation.cleanup(cleanup),
        })) || undefined;
      this.#activation.signal.throwIfAborted();
      if (contributions.some((value) => value.background) && !resource?.start)
        throw new Error(
          "plugin: background setup requires an activation resource",
        );
      return resource;
    });
    try {
      await this.#setupResult;
      return { value: true };
    } catch (error) {
      await closeOwned(this.#activation);
      throw new Error("plugin: setup failed", { cause: error });
    } finally {
      extra.signal.removeEventListener("abort", abort);
    }
  }
  async #activate(params: Record<string, unknown>) {
    exactFields(params, ["protocol"], ["_meta"]);
    assertProtocol(params);
    if (!this.#setupResult)
      throw new Error("plugin: setup is required before activation");
    const resource = await this.#setupResult;
    this.#lifetime.signal.throwIfAborted();
    this.#start ??= this.#activation.run(() => resource?.start?.());
    await this.#start;
    return { value: true };
  }
  #receive(
    name: string,
    handler: (
      params: Record<string, unknown>,
      extra: Extra,
    ) => unknown | Promise<unknown>,
  ): void {
    this.#server.setRequestHandler(
      z.object({
        method: z.literal(name),
        params: z.record(z.string(), z.unknown()).optional(),
      }),
      async (request, extra) => {
        if (
          !this.#published ||
          this.#lifetime.closing ||
          this.#activation.closing
        )
          throw new Error("plugin: backend is unavailable");
        return await this.#lifetime.run(
          async () =>
            (await handler(request.params ?? {}, extra)) as Record<
              string,
              unknown
            >,
        );
      },
    );
  }
  #resolve(params: Record<string, unknown>): BoundMethod {
    exactFields(
      params,
      ["protocol", "method", "contract", "input"],
      ["export", "capability", "commandId", "_meta"],
    );
    assertProtocol(params);
    const bound =
      typeof params.method === "string"
        ? this.#methods.get(params.method)
        : undefined;
    if (!bound || params.contract !== bound.method.identity)
      throw new Error("plugin: method contract mismatch");
    if ((params.export === undefined) !== (params.capability === undefined))
      throw new Error(
        "plugin: capability export and identity are required together",
      );
    let member = bound.method.descriptor.name;
    if (params.method === wire.constants.receiptMethod!) {
      exactFields(params.input, ["id", "method", "inputDigest"]);
      member = params.input.method as string;
    }
    if (params.export !== undefined) {
      const capability = this.#capabilities.get(params.export as string);
      if (!capability || capability.identity !== params.capability)
        throw new Error("plugin: capability contract mismatch");
      if (params.method === wire.constants.receiptMethod!)
        capability.portable.method(member);
      else if (
        capability.portable.method(member).identity !== bound.method.identity
      )
        throw new Error("plugin: method is not a capability member");
    } else if (
      [...this.#capabilities.values()].some((value) =>
        value.descriptor.methods.some((method) => method.name === member),
      )
    )
      throw new Error("plugin: capability member requires its export");
    return bound;
  }
  #validateSetup(
    contributions: unknown,
  ): asserts contributions is readonly SetupContribution[] {
    if (!Array.isArray(contributions))
      throw new Error("plugin: setup contributions must be an array");
    const ids = new Set<string>();
    for (const value of contributions) {
      exactFields(value, ["id", "export", "requires"], ["background"]);
      if (
        typeof value.id !== "string" ||
        !validName(value.id) ||
        ids.has(value.id) ||
        typeof value.export !== "string" ||
        !this.#capabilities.has(value.export) ||
        !Array.isArray(value.requires)
      )
        throw new Error("plugin: invalid setup contribution");
      ids.add(value.id);
      if (value.background !== undefined) assertBackground(value.background);
      if ((value.requires.length || value.background) && !this.#setup)
        throw new Error("plugin: declared collaborators require setup");
      validateRequirements(value.requires);
    }
  }
  #register(bound: BoundMethod): void {
    if (
      this.#published ||
      this.#lifetime.signal.aborted ||
      this.#methods.has(bound.method.descriptor.name)
    )
      throw new Error("plugin: duplicate or frozen method registration");
    this.#methods.set(bound.method.descriptor.name, bound);
  }
  implement<P, R>(
    method: Method,
    decode: Decoder<P>,
    handler: (context: CallContext, input: P) => R | Promise<R>,
  ): void {
    if (method.admitChunk)
      throw new Error(
        "plugin: streaming method requires streaming implementation",
      );
    this.#register({
      method,
      decode,
      handler: handler as BoundMethod["handler"],
    });
  }
  implementStream<P, C, R>(
    method: Method,
    decode: Decoder<P>,
    handler: (
      context: CallContext,
      input: P,
    ) => AsyncIterable<StreamFrame<C, R>>,
  ): void {
    if (!method.admitChunk)
      throw new Error("plugin: unary method requires unary implementation");
    this.#register({
      method,
      decode,
      stream: handler as BoundMethod["stream"],
    });
  }
  bindCapability(name: string, contract: Capability): void {
    if (this.#published || !validName(name) || this.#capabilities.has(name))
      throw new Error("plugin: invalid or duplicate capability export");
    this.#capabilities.set(name, contract);
  }
  receipts(
    handler: (
      context: CallContext,
      request: ReceiptRequest,
    ) => unknown | Promise<unknown>,
  ): void {
    const method = declareMethod(parseJSON(wire.receipt));
    this.#register({
      method,
      decode: decodeReceipt,
      handler: handler as BoundMethod["handler"],
    });
  }
  onSetup(handler: SetupHandler): void {
    if (this.#setup || this.#published)
      throw new Error("plugin: setup registration is frozen");
    this.#setup = handler;
  }
  tool(
    descriptor: Tool,
    handler: (
      context: CallContext,
      input: unknown,
    ) => CallToolResult | Promise<CallToolResult>,
  ): void {
    const captured = ToolSchema.parse(freezeJSON(descriptor));
    if (this.#published || !captured.name || this.#tools.has(captured.name))
      throw new Error("plugin: duplicate or frozen tool registration");
    const validation = declareMethod({
      name: captured.name,
      description: captured.description ?? "",
      kind: "query",
      input: captured.inputSchema,
      output: captured.outputSchema ?? true,
    });
    this.#tools.set(captured.name, {
      descriptor: captured,
      admitInput: validation.admitInput,
      admitOutput: captured.outputSchema ? validation.admitOutput : undefined,
      handler,
    });
  }
  #publish(): void {
    if (this.#published || this.#lifetime.signal.aborted)
      throw new Error("plugin: backend cannot publish");
    for (const contract of this.#capabilities.values())
      for (const descriptor of contract.descriptor.methods)
        if (
          this.#methods.get(descriptor.name)?.method.identity !==
          contract.portable.method(descriptor.name).identity
        )
          throw new Error("plugin: capability implementation is incomplete");
    this.#published = true;
  }
  serve(transport: Transport): Promise<void> {
    this.#publish();
    const closed = Promise.withResolvers<void>();
    const failures: unknown[] = [];
    this.#server.onclose = () => closed.resolve();
    this.#server.onerror = (error) => failures.push(error);
    return (async () => {
      try {
        await this.#server.connect(transport);
        await closed.promise;
      } catch (error) {
        failures.push(error);
      } finally {
        try {
          await this.close();
        } catch (error) {
          failures.push(error);
        }
      }
      if (failures.length) {
        throw new AggregateError(failures, "plugin: backend service failed");
      }
    })();
  }
  #call(signal: AbortSignal): Lifetime {
    const owner = this.#lifetime.branch("call");
    owner.dependsOn(this.#activation);
    const cancel = () => owner.beginClose();
    owner.cleanup(() => {
      signal.removeEventListener("abort", cancel);
    }, "request-cancellation");
    if (signal.aborted) cancel();
    else signal.addEventListener("abort", cancel, { once: true });
    return owner;
  }

  close(): Promise<void> {
    return closeOwned(this.#lifetime);
  }
  wait(signal?: AbortSignal): Promise<CloseOutcome> {
    return this.#lifetime.wait(signal);
  }
}

async function closeOwned(owner: Lifetime): Promise<void> {
  const outcome = await owner.close();
  if (outcome.status !== "released")
    throw new FrameError(
      "close_unconfirmed",
      "backend resource release remains unconfirmed",
    );
  if (outcome.failures.some((failure) => failure.phase === "cleanup"))
    throw new FrameError(
      "unavailable",
      "backend exit completed with recorded cleanup failures",
    );
}

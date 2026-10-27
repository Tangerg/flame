import { randomUUID } from "node:crypto";
import type { Transport } from "@modelcontextprotocol/sdk/shared/transport.js";
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import type { RequestHandlerExtra } from "@modelcontextprotocol/sdk/shared/protocol.js";
import type {
  Request,
  Notification,
  Tool,
  CallToolResult,
} from "@modelcontextprotocol/sdk/types.js";
import { CallToolResultSchema } from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";
import { parseJSON } from "@qimu/sdk/json";
import {
  Lifetime,
  QimuError,
  type Cleanup,
  type Release,
  type CloseOutcome,
} from "@qimu/sdk";
import {
  Capability,
  Method,
  PROTOCOL,
  canonicalJSON,
  compareTokens,
  digestJSON,
  exactFields,
  freezeJSON,
  validCommand,
  validToken,
  type MethodDescriptor,
} from "./contract.ts";
import {
  callbackRef,
  callHost,
  Refusal,
  streamDelivery,
  type CallbackRef,
  type Invocation,
} from "./callback.ts";
import wire from "./protocol.generated.json";
import { compileSchema } from "./schema.ts";

export interface CallContext {
  readonly signal: AbortSignal;
  readonly commandId?: string;
  readonly invocation?: Invocation;
  readonly requestInput: string;
  readonly inputDigest: string;
  readonly callback?: CallbackRef;
  readonly lifetime: Lifetime;
  defer(cleanup: () => void | Release | Promise<void | Release>): Cleanup;
}
export interface SetupBinding {
  installationId: string;
  digest: string;
  contribution: string;
  contract: import("./contract.ts").CapabilityDescriptor;
}
export interface SetupContribution {
  id: string;
  export: string;
  background?: Invocation["target"];
  requires: Array<{
    id: string;
    contract: import("./contract.ts").CapabilityDescriptor;
    optional: boolean;
    bindings: SetupBinding[];
  }>;
}
export interface ActivationResource {
  close(): void | Promise<void>;
  start?(): void | Promise<void>;
}
export interface ReceiptRequest {
  id: string;
  method: string;
  inputDigest: string;
}
export interface SetupContext {
  readonly signal: AbortSignal;
  readonly contributions: readonly SetupContribution[];
  readonly callback?: CallbackRef;
  readonly lifetime: Lifetime;
  defer(cleanup: () => void | Release | Promise<void | Release>): Cleanup;
}
export type StreamFrame<C, R> =
  { kind: "chunk"; value: C } | { kind: "result"; value: R };
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
type SetupHandler = (
  context: SetupContext,
) => void | ActivationResource | Promise<void | ActivationResource>;

export class Backend {
  readonly #server: Server;
  readonly #methods = new Map<string, BoundMethod>();
  readonly #capabilities = new Map<string, Capability>();
  readonly #tools = new Map<
    string,
    {
      descriptor: Tool;
      validate: (input: unknown) => void;
      validateOutput?: (output: unknown) => void;
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
    this.#receive("tools/list", (params) => {
      exactFields(params, [], ["cursor", "_meta"]);
      if (params.cursor !== undefined)
        throw new Error("plugin: unsupported tool cursor");
      return {
        tools: [...this.#tools.values()].map((tool) => tool.descriptor),
      };
    });
    this.#receive("tools/call", async (params, extra) => {
      exactFields(params, ["name"], ["arguments", "_meta"]);
      const tool = this.#tools.get(params.name as string);
      if (!tool) throw new Error("plugin: tool is not published");
      const input = freezeJSON(params.arguments ?? {});
      tool.validate(input);
      const call = this.#call(extra.signal);
      const context = this.#context({ input, _meta: params._meta }, call);
      try {
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
        CallToolResultSchema.parse(output);
        if (tool.validateOutput) tool.validateOutput(output.structuredContent);
        return output;
      } finally {
        await closeOwned(call);
      }
    });
    this.#receive(wire.constants.describeMethod!, (params) => {
      exactFields(params, ["protocol"], ["_meta"]);
      this.#protocol(params);
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
    });
    this.#receive(wire.constants.validateMethod!, (params) => {
      const bound = this.#resolve(params);
      if (params.commandId !== undefined)
        throw new Error("plugin: validation cannot carry a command identity");
      try {
        this.#decode(bound, params.input);
        return { value: "" };
      } catch {
        return { value: "plugin: input is not structurally admitted" };
      }
    });
    this.#receive(wire.constants.invokeMethod!, async (params, extra) => {
      const bound = this.#resolve(params);
      if (
        bound.method.descriptor.kind === "command"
          ? !validCommand(params.commandId)
          : params.commandId !== undefined
      )
        throw new Error("plugin: invalid command identity for method kind");
      const input = this.#decode(bound, params.input);
      const call = this.#call(extra.signal);
      const context = this.#context(
        params,
        call,
        bound.method.descriptor.name === wire.constants.ReceiptMethod!
          ? undefined
          : bound.method.descriptor.target,
      );
      try {
        context.signal.throwIfAborted();
        let value: unknown;
        try {
          value = await call.run(() =>
            bound.stream
              ? this.#deliver(
                  bound,
                  context,
                  input,
                  this.#metadata(params)[wire.constants.streamMetadataKey!],
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
        bound.method.validateOutput(value);
        return { value };
      } finally {
        await closeOwned(call);
      }
    });
    this.#receive(wire.constants.setupMethod!, async (params, extra) => {
      exactFields(params, ["protocol", "contributions"], ["_meta"]);
      this.#protocol(params);
      if (this.#setupResult || !Array.isArray(params.contributions))
        throw new Error("plugin: setup is invalid or already supplied");
      const contributions = freezeJSON(
        params.contributions,
      ) as SetupContribution[];
      this.#validateSetup(contributions);
      const metadata = this.#metadata(params);
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
    });
    this.#receive(wire.constants.activateMethod!, async (params) => {
      exactFields(params, ["protocol"], ["_meta"]);
      this.#protocol(params);
      if (!this.#setupResult)
        throw new Error("plugin: setup is required before activation");
      const resource = await this.#setupResult;
      this.#lifetime.signal.throwIfAborted();
      this.#start ??= this.#activation.run(() => resource?.start?.());
      await this.#start;
      return { value: true };
    });
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
  #protocol(params: Record<string, unknown>): void {
    if (params.protocol !== PROTOCOL)
      throw new Error("plugin: current backend protocol is required");
  }
  #resolve(params: Record<string, unknown>): BoundMethod {
    exactFields(
      params,
      ["protocol", "method", "contract", "input"],
      ["export", "capability", "commandId", "_meta"],
    );
    this.#protocol(params);
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
    if (params.method === wire.constants.ReceiptMethod!) {
      exactFields(params.input, ["id", "method", "inputDigest"]);
      member = params.input.method as string;
    }
    if (params.export !== undefined) {
      const capability = this.#capabilities.get(params.export as string);
      if (!capability || capability.identity !== params.capability)
        throw new Error("plugin: capability contract mismatch");
      if (params.method === wire.constants.ReceiptMethod!)
        capability.method(member);
      else if (capability.method(member).identity !== bound.method.identity)
        throw new Error("plugin: method is not a capability member");
    } else if (
      [...this.#capabilities.values()].some((value) =>
        value.descriptor.methods.some((method) => method.name === member),
      )
    )
      throw new Error("plugin: capability member requires its export");
    return bound;
  }
  #decode(bound: BoundMethod, input: unknown): unknown {
    const frozen = freezeJSON(input);
    bound.method.validateInput(frozen);
    return bound.decode(frozen, canonicalJSON(frozen));
  }
  #metadata(params: Record<string, unknown>): Record<string, unknown> {
    const meta = params._meta ?? {};
    if (meta === null || typeof meta !== "object" || Array.isArray(meta))
      throw new Error("plugin: metadata must be an object");
    return freezeJSON(meta as Record<string, unknown>);
  }
  #context(
    params: Record<string, unknown>,
    owner: Lifetime,
    target?: string,
  ): CallContext {
    const meta = this.#metadata(params);
    const invocation = meta[wire.constants.InvocationMetadataKey!] as
      Invocation | undefined;
    if (invocation) {
      exactFields(invocation, ["id", "origin", "target"], ["parent"]);
      exactFields(invocation.target, ["scope", "reference"]);
      if (
        !validToken(invocation.id) ||
        !validToken(invocation.origin) ||
        (invocation.parent !== undefined &&
          (!validToken(invocation.parent) ||
            invocation.parent === invocation.id)) ||
        !validToken(invocation.target.scope) ||
        typeof invocation.target.reference !== "string" ||
        !invocation.target.reference ||
        /\p{Cc}/u.test(invocation.target.reference) ||
        (target !== undefined && invocation.target.scope !== target)
      )
        throw new Error("plugin: invalid calling-owner invocation");
    }
    const ref =
      meta[wire.constants.callbackMetadataKey!] === undefined
        ? undefined
        : callbackRef(meta[wire.constants.callbackMetadataKey!]);
    const lifetime = owner.signal;
    return Object.freeze({
      signal: lifetime,
      commandId: params.commandId as string | undefined,
      invocation,
      callback: ref,
      requestInput: canonicalJSON(params.input),
      inputDigest: digestJSON(params.input),
      lifetime: owner,
      defer: (cleanup: () => void | Release | Promise<void | Release>) =>
        owner.cleanup(cleanup),
    });
  }
  #validateSetup(contributions: SetupContribution[]): void {
    const ids = new Set<string>();
    for (const value of contributions) {
      exactFields(value, ["id", "export", "requires"], ["background"]);
      if (
        typeof value.id !== "string" ||
        !validToken(value.id) ||
        ids.has(value.id) ||
        !this.#capabilities.has(value.export) ||
        !Array.isArray(value.requires)
      )
        throw new Error("plugin: invalid setup contribution");
      ids.add(value.id);
      if (value.background !== undefined) {
        exactFields(value.background, ["scope", "reference"]);
        if (
          value.background.scope !== "installation" ||
          typeof value.background.reference !== "string" ||
          !validToken(value.background.reference)
        )
          throw new Error("plugin: background must identify its installation");
      }
      if ((value.requires.length || value.background) && !this.#setup)
        throw new Error("plugin: declared collaborators require setup");
      const requirements = new Set<string>();
      for (const dependency of value.requires) {
        exactFields(dependency, ["id", "contract", "optional", "bindings"]);
        if (
          typeof dependency.id !== "string" ||
          !validToken(dependency.id) ||
          requirements.has(dependency.id) ||
          typeof dependency.optional !== "boolean" ||
          !Array.isArray(dependency.bindings)
        )
          throw new Error("plugin: invalid setup dependency");
        requirements.add(dependency.id);
        const contract = new Capability(dependency.contract);
        if (
          (!dependency.optional && !dependency.bindings.length) ||
          (contract.descriptor.composition === "selection" &&
            dependency.bindings.length > 1)
        )
          throw new Error("plugin: invalid dependency cardinality");
        for (const binding of dependency.bindings) {
          exactFields(binding, [
            "installationId",
            "digest",
            "contribution",
            "contract",
          ]);
          if (
            typeof binding.installationId !== "string" ||
            !validToken(binding.installationId) ||
            typeof binding.contribution !== "string" ||
            !validToken(binding.contribution) ||
            typeof binding.digest !== "string" ||
            !/^[a-f0-9]{64}$/.test(binding.digest) ||
            new Capability(binding.contract).identity !== contract.identity
          )
            throw new Error("plugin: invalid captured dependency binding");
        }
      }
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
    if (method.validateChunk)
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
    if (!method.validateChunk)
      throw new Error("plugin: unary method requires unary implementation");
    this.#register({
      method,
      decode,
      stream: handler as BoundMethod["stream"],
    });
  }
  bindCapability(name: string, contract: Capability): void {
    if (this.#published || !validToken(name) || this.#capabilities.has(name))
      throw new Error("plugin: invalid or duplicate capability export");
    this.#capabilities.set(name, contract);
  }
  receipts(
    handler: (
      context: CallContext,
      request: ReceiptRequest,
    ) => unknown | Promise<unknown>,
  ): void {
    const method = new Method(parseJSON(wire.receipt) as MethodDescriptor);
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
    if (this.#published || !descriptor.name || this.#tools.has(descriptor.name))
      throw new Error("plugin: duplicate or frozen tool registration");
    this.#tools.set(descriptor.name, {
      descriptor: freezeJSON(descriptor),
      validate: compileSchema(descriptor.inputSchema),
      validateOutput: descriptor.outputSchema
        ? compileSchema(descriptor.outputSchema)
        : undefined,
      handler,
    });
  }
  #publish(): void {
    if (
      this.#published ||
      (!this.#methods.size && !this.#tools.size) ||
      this.#lifetime.signal.aborted
    )
      throw new Error("plugin: backend cannot publish");
    for (const contract of this.#capabilities.values())
      for (const descriptor of contract.descriptor.methods)
        if (
          this.#methods.get(descriptor.name)?.method.identity !==
          contract.method(descriptor.name).identity
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
  async #deliver(
    bound: BoundMethod,
    context: CallContext,
    input: unknown,
    reference: unknown,
  ): Promise<unknown> {
    const ref = callbackRef(reference);
    let final: { value: unknown } | undefined;
    let sequence = 0n;
    for await (const frame of bound.stream!(context, input)) {
      context.signal.throwIfAborted();
      exactFields(frame, ["kind", "value"]);
      if (final)
        throw new Error("plugin: stream continued after its final result");
      if (frame.kind === "result") {
        bound.method.validateOutput(frame.value);
        final = { value: freezeJSON(frame.value) };
      } else if (frame.kind === "chunk") {
        bound.method.validateChunk!(frame.value);
        if (sequence === 18446744073709551615n)
          throw new Error("plugin: stream sequence exhausted");
        sequence++;
        const acknowledged = await callHost(
          ref,
          streamDelivery,
          { sequence, value: frame.value },
          context.signal,
          context.invocation,
          randomUUID(),
        );
        if (acknowledged !== true)
          throw new Error(
            "plugin: stream consumer did not acknowledge its chunk",
          );
      } else throw new Error("plugin: invalid stream frame kind");
    }
    if (!final) throw new Error("plugin: stream final result is unconfirmed");
    return final.value;
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

export function decodeReceipt(value: unknown): ReceiptRequest {
  const request = value as ReceiptRequest;
  if (
    !validCommand(request.id) ||
    !validCommand(request.method) ||
    !/^[a-f0-9]{64}$/.test(request.inputDigest)
  )
    throw new Error("plugin: invalid receipt identity");
  return request;
}

async function closeOwned(owner: Lifetime): Promise<void> {
  const outcome = await owner.close();
  if (outcome.status !== "released")
    throw new QimuError(
      "close_unconfirmed",
      "backend resource release remains unconfirmed",
    );
  if (outcome.failures.some((failure) => failure.phase === "cleanup"))
    throw new QimuError(
      "unavailable",
      "backend exit completed with recorded cleanup failures",
    );
}

import { METHODS } from "../contracts/methods";
import { toolCommandId } from "@flame/backend-sdk";
import { randomUUID } from "node:crypto";
import { hostname } from "node:os";
import {
  Method,
  Refusal,
  canonicalJSON,
  decodeReply,
  digestJSON,
  parseJSON,
  type CallContext,
  type Invocation,
} from "@qimu/node";
import { isJSONNumber, projectNumber } from "@qimu/sdk/json";
import type {
  CallToolResult,
  ElicitRequestParams,
  ElicitResult,
} from "@modelcontextprotocol/sdk/types.js";
import {
  schemas,
  type Approval,
  type ControlSession,
  type ExecuteInput,
  type ExecutionResult,
  type HistoryPage,
  type OperationRecord,
  type ReplyInput,
  type Snapshot,
  type Surface,
} from "../contracts/capability";
import { InvalidPermission, PermissionRequest } from "../contracts/elicitation";
import { LIMITS } from "../contracts/limits";
import {
  type Engine,
  EngineStartupFailure,
  type EngineHooks,
  type EngineResult,
  type EngineSurface,
} from "./engine";
import { Receipts, type OriginalCommand, type ReceiptRecord } from "./receipts";
import { Session } from "./session";
import type { Configuration } from "./configuration";
import { Cua } from "./cua";

export type EngineConnection = Pick<Engine, "surface" | "call" | "close">;
export type ConnectEngine = (
  config: Configuration,
  hooks: EngineHooks,
) => Promise<EngineConnection>;
type Reason = NonNullable<ControlSession["reason"]>;
type PendingPermission = {
  readonly snapshot: Approval;
  readonly request: PermissionRequest;
  readonly answer: (response: ElicitResult) => void;
};
type Resource = {
  readonly session: Session;
  readonly lifetime: AbortController;
  readonly timer: ReturnType<typeof setTimeout>;
  readonly opening: Promise<EngineConnection>;
  engine?: EngineConnection;
  surface?: Readonly<EngineSurface>;
  inputs?: ReadonlyMap<string, Method>;
  permission?: PendingPermission;
  work?: Promise<EngineResult>;
  disposal?: Promise<void>;
};

function trusted(context: CallContext, human = false): Invocation {
  if (
    !context.invocation ||
    context.invocation.target.scope !== "installation" ||
    (human
      ? context.invocation.origin !== "human"
      : context.invocation.origin === "automatic")
  )
    throw new Refusal(
      human ? "human_required" : "invocation_required",
      human
        ? "computer control requires the original human caller"
        : "computer control requires a captured model or human caller",
    );
  return context.invocation;
}

function engineJSON(value: unknown): unknown {
  if (isJSONNumber(value)) return projectNumber(value);
  if (Array.isArray(value)) return value.map(engineJSON);
  if (value !== null && typeof value === "object")
    return Object.fromEntries(
      Object.entries(value).map(([name, child]) => [name, engineJSON(child)]),
    );
  return value;
}

function toolFailure(error: Refusal): CallToolResult {
  return {
    isError: true,
    content: [{ type: "text", text: `${error.code}: ${error.message}` }],
  };
}

export class Control {
  readonly #config?: Configuration;
  readonly #receipts: Receipts;
  readonly #signal: AbortSignal;
  readonly #connect: ConnectEngine;
  readonly #host = {
    hostname: hostname(),
    platform: process.platform,
    arch: process.arch,
  };
  readonly #listeners = new Set<() => void>();
  #resource?: Resource;
  #closing?: Promise<void>;
  #failure?: unknown;

  constructor(
    config: Configuration | undefined,
    receipts: Receipts,
    signal: AbortSignal,
    connect: ConnectEngine = Cua.connect,
  ) {
    this.#config = config;
    this.#receipts = receipts;
    this.#signal = signal;
    this.#connect = connect;
    signal.throwIfAborted();
    signal.addEventListener("abort", this.#shutdown, { once: true });
  }

  readonly #shutdown = (): void => {
    void this.close().catch((cause: unknown) => {
      this.#failure = cause;
    });
  };

  #changed(): void {
    for (const listener of this.#listeners) listener();
  }

  snapshot(): Snapshot {
    const resource = this.#resource;
    return {
      host: { ...this.#host },
      configured: this.#config !== undefined,
      ...(resource
        ? {
            session: {
              ...resource.session.snapshot(),
              ...(resource.permission
                ? { approval: structuredClone(resource.permission.snapshot) }
                : {}),
            },
          }
        : {}),
    };
  }

  async *watch(
    signal: AbortSignal,
  ): AsyncGenerator<
    { kind: "chunk"; value: Snapshot } | { kind: "result"; value: boolean }
  > {
    let dirty = true;
    let resolve: (() => void) | undefined;
    const wake = () => {
      dirty = true;
      resolve?.();
    };
    this.#listeners.add(wake);
    signal.addEventListener("abort", wake, { once: true });
    try {
      for (;;) {
        signal.throwIfAborted();
        if (!dirty)
          await new Promise<void>((done) => {
            resolve = done;
            if (dirty || signal.aborted) done();
          });
        signal.throwIfAborted();
        resolve = undefined;
        dirty = false;
        yield { kind: "chunk", value: this.snapshot() };
      }
    } finally {
      this.#listeners.delete(wake);
      signal.removeEventListener("abort", wake);
    }
  }

  #current(id?: string): Resource {
    this.#signal.throwIfAborted();
    if (this.#closing)
      throw new Refusal("computer_closed", "computer backend is closing");
    const resource = this.#resource;
    if (!resource)
      throw new Refusal(
        "session_required",
        "open a computer-control session from the human console first",
      );
    if (id !== undefined) resource.session.assertCurrent(id);
    if (
      resource.session.expired(Date.now()) &&
      !resource.lifetime.signal.aborted
    )
      this.#retire(resource, "expired");
    return resource;
  }

  context(): Surface {
    const resource = this.#current();
    const session = resource.session.snapshot();
    if (
      !resource.surface ||
      !session.surfaceDigest ||
      !["ready", "executing"].includes(session.state) ||
      resource.lifetime.signal.aborted
    )
      throw new Refusal(
        "computer_unavailable",
        "computer engine has no current usable surface",
      );
    return {
      sessionId: session.id,
      workerId: session.workerId,
      surfaceDigest: session.surfaceDigest,
      expiresAt: session.expiresAt,
      instructions: resource.surface.instructions,
      toolsJSON: canonicalJSON(resource.surface.tools),
      serverJSON: canonicalJSON({
        info: resource.surface.serverInfo,
        capabilities: resource.surface.capabilities,
      }),
      host: { ...this.#host },
    };
  }

  #original(
    context: CallContext,
    kind: "tool" | "command",
    method: string,
  ): OriginalCommand {
    const invocation = trusted(context);
    const id = kind === "tool" ? toolCommandId(context) : context.commandId;
    if (!id) throw new Error("computer: original command identity is required");
    return {
      id,
      kind,
      method,
      input: context.requestInput,
      inputDigest: context.inputDigest,
      invocation,
      admittedAt: new Date().toISOString(),
    };
  }

  async #command<T>(
    context: CallContext,
    method: string,
    action: (id: string) => T | Promise<T>,
    admission: "ordinary" | "reserved" = "ordinary",
  ): Promise<T> {
    context.signal.throwIfAborted();
    const original = this.#original(context, "command", method);
    const admitted = this.#receipts.admit(original, admission);
    if (!admitted.fresh) {
      if (admitted.record.state !== "settled")
        throw new Error(
          "computer: original command outcome remains unconfirmed",
        );
      return decodeReply(admitted.record.outcome) as T;
    }
    try {
      const value = await action(original.id);
      this.#receipts.settleCommand(original.id, { value });
      return value;
    } catch (cause) {
      if (cause instanceof Refusal)
        this.#receipts.settleCommand(original.id, {
          failure: { code: cause.code, message: cause.message },
        });
      throw cause;
    } finally {
      this.#changed();
    }
  }

  #revocationAdmission(
    context: CallContext,
    sessionId: string,
    operationId?: string,
  ): "ordinary" | "reserved" {
    const resource = this.#resource;
    return context.invocation?.origin === "human" &&
      resource &&
      !resource.disposal &&
      resource.session.canRevoke(sessionId, operationId)
      ? "reserved"
      : "ordinary";
  }

  open(context: CallContext, leaseSeconds: number): Promise<ControlSession> {
    return this.#command(context, METHODS.open, async (id) => {
      trusted(context, true);
      this.#signal.throwIfAborted();
      if (this.#closing)
        throw new Refusal("computer_closed", "computer backend is closing");
      if (!this.#config)
        throw new Refusal(
          "engine_unconfigured",
          "select an installed CUA application on the Runtime host first",
        );
      if (
        this.#resource &&
        this.#resource.session.snapshot().state !== "closed"
      )
        throw new Refusal(
          "session_active",
          "close the current computer-control session before opening another",
        );
      if (!schemas.open.safeParse({ leaseSeconds }).success)
        throw new Refusal(
          "invalid_lease",
          "computer lease is outside its declared bound",
        );
      const now = Date.now();
      const session = new Session(
        randomUUID(),
        randomUUID(),
        new Date(now).toISOString(),
        new Date(now + leaseSeconds * 1000).toISOString(),
      );
      const lifetime = new AbortController();
      const opening = Promise.withResolvers<EngineConnection>();
      this.#receipts.markDispatched(id);
      const resource: Resource = {
        session,
        lifetime,
        timer: setTimeout(
          () => this.#retire(resource, "expired"),
          leaseSeconds * 1000,
        ),
        opening: opening.promise,
      };
      this.#resource = resource;
      this.#changed();
      const abort = () => this.#retire(resource, "cancelled");
      context.signal.addEventListener("abort", abort, { once: true });
      void Promise.resolve()
        .then(() =>
          this.#connect(this.#config!, {
            signal: lifetime.signal,
            onElicitation: (params, call) =>
              this.#permission(resource, params, call.signal),
            onRetired: () => this.#retire(resource, "engine"),
          }),
        )
        .then(opening.resolve, opening.reject);
      try {
        resource.engine = await resource.opening;
        lifetime.signal.throwIfAborted();
        const surface = resource.engine.surface();
        if (Buffer.byteLength(canonicalJSON(surface)) > LIMITS.maxSurfaceBytes)
          throw new Error("computer: engine surface exceeds its bound");
        resource.inputs = new Map(
          surface.tools.map((tool) => [
            tool.name,
            new Method({
              name: `flame.computer/upstream.${tool.name}`,
              kind: "command",
              target: "installation",
              description: tool.description ?? "",
              input: tool.inputSchema,
              output: {},
            }),
          ]),
        );
        resource.surface = surface;
        session.connected(digestJSON(surface));
        this.#changed();
        return session.snapshot();
      } catch (cause) {
        this.#retire(
          resource,
          lifetime.signal.aborted ? "cancelled" : "startup",
        );
        await resource.disposal;
        const refusal = new Refusal(
          "engine_unavailable",
          cause instanceof EngineStartupFailure
            ? cause.message
            : "computer engine could not start; inspect the configured installation and desktop prerequisites",
        );
        refusal.cause = cause;
        throw refusal;
      } finally {
        context.signal.removeEventListener("abort", abort);
      }
    });
  }

  #retire(resource: Resource, reason: Reason): void {
    if (resource.disposal) return;
    resource.session.stopping(reason);
    clearTimeout(resource.timer);
    // Publish revocation before aborting callbacks: their synchronous observers
    // must never admit another operation into the retiring worker.
    resource.disposal = Promise.resolve().then(async () => {
      try {
        let engine: EngineConnection | undefined;
        try {
          engine = await resource.opening;
        } catch (cause) {
          if (
            !(cause instanceof EngineStartupFailure) ||
            cause.cleanup !== "closed"
          )
            throw cause;
        }
        await engine?.close();
        await resource.work?.catch(() => {});
        resource.session.retired(reason, true);
      } catch (cause) {
        resource.session.retired("cleanup", false);
        this.#failure = cause;
        throw cause;
      } finally {
        this.#changed();
      }
    });
    resource.lifetime.abort(new Error(`computer: control session ${reason}`));
    void resource.disposal.catch(() => {});
    this.#changed();
  }

  closeSession(
    context: CallContext,
    sessionId: string,
  ): Promise<ControlSession> {
    return this.#command(
      context,
      METHODS.close,
      async (id) => {
        trusted(context, true);
        const resource = this.#current(sessionId);
        this.#receipts.markDispatched(id);
        this.#retire(resource, "user");
        await resource.disposal;
        return resource.session.snapshot();
      },
      this.#revocationAdmission(context, sessionId),
    );
  }

  stop(
    context: CallContext,
    sessionId: string,
    operationId: string,
  ): Promise<ControlSession> {
    return this.#command(
      context,
      METHODS.stop,
      async (id) => {
        trusted(context, true);
        const resource = this.#current(sessionId);
        resource.session.assertOperation(operationId);
        this.#receipts.markDispatched(id);
        this.#retire(resource, "user");
        await resource.disposal;
        return resource.session.snapshot();
      },
      this.#revocationAdmission(context, sessionId, operationId),
    );
  }

  async #permission(
    resource: Resource,
    params: ElicitRequestParams,
    signal: AbortSignal,
  ): Promise<ElicitResult> {
    const session = resource.session.snapshot();
    if (
      resource !== this.#resource ||
      session.state !== "executing" ||
      !session.operation ||
      resource.permission ||
      signal.aborted ||
      resource.lifetime.signal.aborted
    )
      return { action: "cancel" };
    const requestJSON = canonicalJSON(params);
    const request = new PermissionRequest(requestJSON);
    if (request.native?.app.toLowerCase() === "com.tangerg.flame")
      return { action: "decline" };
    const lifetime = AbortSignal.any([signal, resource.lifetime.signal]);
    const pending = Promise.withResolvers<ElicitResult>();
    const approval: PendingPermission = {
      snapshot: {
        id: randomUUID(),
        sessionId: session.id,
        operationId: session.operation.id,
        requestJSON,
      },
      request,
      answer: pending.resolve,
    };
    const abort = () => pending.resolve({ action: "cancel" });
    lifetime.addEventListener("abort", abort, { once: true });
    resource.permission = approval;
    this.#changed();
    try {
      if (lifetime.aborted) abort();
      return await pending.promise;
    } finally {
      lifetime.removeEventListener("abort", abort);
      if (resource.permission === approval) resource.permission = undefined;
      this.#changed();
    }
  }

  reply(context: CallContext, input: ReplyInput): Promise<boolean> {
    return this.#command(context, METHODS.reply, (id) => {
      trusted(context, true);
      const resource = this.#current(input.sessionId);
      const pending = resource.permission;
      if (
        !pending ||
        pending.snapshot.id !== input.requestId ||
        resource.lifetime.signal.aborted
      )
        throw new Refusal(
          "permission_retired",
          "computer permission request is no longer pending",
        );
      let response: ElicitResult;
      try {
        response = pending.request.reply(input.responseJSON);
      } catch (cause) {
        if (!(cause instanceof InvalidPermission)) throw cause;
        const refusal = new Refusal("invalid_permission", cause.message);
        refusal.cause = cause;
        throw refusal;
      }
      this.#receipts.markDispatched(id);
      resource.permission = undefined;
      pending.answer(response);
      this.#changed();
      return true;
    });
  }

  async #execute(
    context: CallContext,
    input: ExecuteInput,
    id: string,
  ): Promise<EngineResult> {
    const resource = this.#current(input.sessionId);
    const engine = resource.engine;
    let args: Record<string, unknown>;
    try {
      const exact = parseJSON(input.argumentsJSON);
      const method = resource.inputs?.get(input.tool);
      if (!method)
        throw new Error("computer: unavailable original tool schema");
      method.validateInput(exact);
      args = engineJSON(exact) as Record<string, unknown>;
      if (!args || typeof args !== "object" || Array.isArray(args))
        throw new Error("computer: upstream tool arguments must be an object");
    } catch (cause) {
      const refusal = new Refusal(
        "invalid_arguments",
        "computer arguments do not satisfy the captured upstream tool contract",
      );
      refusal.cause = cause;
      throw refusal;
    }
    if (!engine || resource.lifetime.signal.aborted)
      throw new Refusal(
        "computer_unavailable",
        "computer engine is not available",
      );
    context.signal.throwIfAborted();
    resource.session.begin(input, {
      id,
      tool: input.tool,
      startedAt: new Date().toISOString(),
    });
    this.#changed();
    const work = Promise.resolve().then(() => {
      if (resource.session.expired(Date.now())) {
        this.#retire(resource, "expired");
        throw new Refusal(
          "lease_expired",
          "computer control lease expired before dispatch",
        );
      }
      context.signal.throwIfAborted();
      resource.lifetime.signal.throwIfAborted();
      this.#receipts.markDispatched(id);
      return engine.call(input.tool, args, {
        sessionId: resource.session.id,
        turnId: randomUUID(),
        callId: id,
        signal: AbortSignal.any([context.signal, resource.lifetime.signal]),
      });
    });
    resource.work = work;
    try {
      const result = await work;
      if (result.cleanup === "unconfirmed") this.#retire(resource, "cleanup");
      else
        resource.session.complete(
          id,
          input.tool === "js_reset" && !result.result.isError
            ? randomUUID()
            : undefined,
        );
      return result;
    } catch (cause) {
      this.#retire(resource, "engine");
      if (cause instanceof Refusal && !this.#receipts.read(id)?.dispatched)
        throw cause;
      throw new Error(
        "computer: original external operation outcome remains unconfirmed",
        { cause },
      );
    } finally {
      this.#changed();
    }
  }

  execute(context: CallContext, input: ExecuteInput): Promise<ExecutionResult> {
    return this.#command(context, METHODS.execute, async (id) => {
      const result = await this.#execute(context, input, id);
      return { commandId: id, ...result };
    });
  }

  async tool(
    context: CallContext,
    name: "js" | "reset",
    input: ExecuteInput,
  ): Promise<CallToolResult> {
    context.signal.throwIfAborted();
    const original = this.#original(context, "tool", name);
    const admitted = this.#receipts.admit(original);
    if (!admitted.fresh) {
      if (admitted.record.state !== "settled")
        throw new Error("computer: original tool outcome remains unconfirmed");
      return admitted.record.outcome as CallToolResult;
    }
    try {
      const result = await this.#execute(context, input, original.id);
      this.#receipts.settleTool(original.id, result.result);
      return result.result;
    } catch (cause) {
      if (!(cause instanceof Refusal)) throw cause;
      const result = toolFailure(cause);
      this.#receipts.settleTool(original.id, result);
      return result;
    } finally {
      this.#changed();
    }
  }

  history(before: string | undefined, limit: number): HistoryPage {
    const page = this.#receipts.list({ before, limit });
    return {
      entries: page.records.map((record) => ({
        id: record.id,
        kind: record.kind,
        method: record.method,
        admittedAt: record.admittedAt,
        dispatched: record.dispatched,
        settled: record.state === "settled",
      })),
      ...(page.nextCursor ? { next: page.nextCursor } : {}),
    };
  }

  result(id: string): OperationRecord {
    const record = this.#receipts.read(id);
    if (!record)
      throw new Refusal(
        "operation_not_found",
        "computer operation was not found",
      );
    return this.#record(record);
  }

  #record(record: ReceiptRecord): OperationRecord {
    return schemas.record.parse({
      id: record.id,
      kind: record.kind,
      method: record.method,
      admittedAt: record.admittedAt,
      dispatched: record.dispatched,
      settled: record.state === "settled",
      inputJSON: record.input,
      invocationJSON: canonicalJSON(record.invocation),
      ...(record.state === "settled"
        ? { outcomeJSON: canonicalJSON(record.outcome) }
        : {}),
    });
  }

  close(): Promise<void> {
    this.#closing ??= Promise.resolve().then(async () => {
      this.#signal.removeEventListener("abort", this.#shutdown);
      if (this.#resource) {
        this.#retire(this.#resource, "shutdown");
        await this.#resource.disposal;
      }
      if (this.#failure) throw this.#failure;
    });
    return this.#closing;
  }
}

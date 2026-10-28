import { performance } from "node:perf_hooks";
import { Method } from "@qimu/node";
import { cloneJSON, parseJSON, stringifyJSON } from "@qimu/sdk/json";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import type { RequestOptions } from "@modelcontextprotocol/sdk/shared/protocol.js";
import {
  CallToolResultSchema,
  ElicitRequestParamsSchema,
  ListToolsResultSchema,
  ToolListChangedNotificationSchema,
  type CallToolRequest,
  type CallToolResult,
  type ElicitRequest,
  type ElicitResult,
  type Implementation,
  type ListToolsResult,
  type ServerCapabilities,
  type Tool,
} from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";
import { EngineProcess, type EngineConfig } from "./engineProcess";
import { LIMITS } from "../contracts/limits";
import manifest from "../package/plugin.json";

export type { EngineConfig } from "./engineProcess";

export interface EngineSurface {
  instructions: string;
  tools: readonly Tool[];
  serverInfo: Implementation;
  capabilities: ServerCapabilities;
}

export interface EngineHooks {
  signal: AbortSignal;
  onElicitation(
    params: ElicitRequest["params"],
    context: { signal: AbortSignal },
  ): Promise<ElicitResult>;
  onRetired(reason: Error): void;
}

export interface EngineCallScope {
  sessionId: string;
  turnId: string;
  callId: string;
  signal: AbortSignal;
}

export interface EngineResult {
  result: CallToolResult;
  cleanup: "ended" | "unconfirmed";
}

export class EngineStartupFailure extends Error {
  readonly cleanup: "closed" | "unconfirmed";

  constructor(
    cleanup: "closed" | "unconfirmed",
    cause: unknown,
    diagnostics: readonly string[] = [],
  ) {
    super(
      "computer: engine connection could not be established" +
        (cleanup === "unconfirmed"
          ? "; inspect its ownership record before retrying"
          : "") +
        (diagnostics.length ? ` (${diagnostics.join(", ")})` : ""),
      { cause },
    );
    this.name = "EngineStartupFailure";
    this.cleanup = cleanup;
  }
}

const CONNECT_MS = 120_000;
const CLEANUP_MS = 120_000;
const CALL_MS = 120_000;
const RESULT_MARGIN_MS = 30_000;
const MAX_TIMER_MS = 2 ** 31 - 1;

// SDK schemas admit the protocol; their parsed projections must not rewrite evidence.
const originalToolResult = z.custom<CallToolResult>(
  (value) => CallToolResultSchema.safeParse(value).success,
);
const originalTools = z.custom<ListToolsResult>(
  (value) => ListToolsResultSchema.safeParse(value).success,
);
const originalElicitation = z.object({
  method: z.literal("elicitation/create"),
  params: z.record(z.string(), z.unknown()),
});

function immutable<T>(value: T): T {
  if (value && typeof value === "object" && !Object.isFrozen(value)) {
    for (const child of Object.values(value)) immutable(child);
    Object.freeze(value);
  }
  return value;
}

function ownedIdentity(value: string): boolean {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    !Array.from(value).some((character) => {
      const code = character.codePointAt(0)!;
      return code < 32 || code === 127;
    })
  );
}

class ExecutionDeadline {
  readonly controller = new AbortController();
  readonly #parent: AbortSignal;
  #remaining: number;
  #started = performance.now();
  #timer?: ReturnType<typeof setTimeout>;
  #paused = 0;
  #closed = false;

  constructor(milliseconds: number, parent: AbortSignal) {
    this.#remaining = milliseconds;
    this.#parent = parent;
    if (parent.aborted) this.#abort();
    else {
      parent.addEventListener("abort", this.#abort, { once: true });
      this.#resume();
    }
  }

  readonly #abort = (): void => {
    this.controller.abort(this.#parent.reason);
    this.close();
  };

  #resume(): void {
    if (this.#closed || this.#paused || this.controller.signal.aborted) return;
    this.#started = performance.now();
    this.#timer = setTimeout(
      () => {
        this.controller.abort(
          new Error("computer: engine execution deadline elapsed"),
        );
        this.close();
      },
      Math.max(0, this.#remaining),
    );
  }

  pause(): () => void {
    if (this.#closed) return () => {};
    if (this.#paused++ === 0) {
      clearTimeout(this.#timer);
      this.#remaining -= performance.now() - this.#started;
    }
    let resumed = false;
    return () => {
      if (resumed || this.#closed) return;
      resumed = true;
      this.#paused--;
      this.#resume();
    };
  }

  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    clearTimeout(this.#timer);
    this.#parent.removeEventListener("abort", this.#abort);
  }
}

interface ActiveCall {
  scope: EngineCallScope;
  deadline: ExecutionDeadline;
  approvals: AbortController;
  phase: "execution" | "cleanup";
  settled: Promise<EngineResult>;
}

export class Engine {
  readonly #process: EngineProcess;
  readonly #client: Client;
  readonly #hooks: EngineHooks;
  readonly #requestMeta: Readonly<Record<string, unknown>>;
  readonly #lifetime = new AbortController();
  readonly #outputs = new Map<string, (value: unknown) => void>();
  #surface?: Readonly<EngineSurface>;
  #active?: ActiveCall;
  #closing?: Promise<void>;
  #shutdown?: Promise<void>;
  #observerFailure?: Error;

  private constructor(config: EngineConfig, hooks: EngineHooks) {
    const metadata = parseJSON(config.env.NODE_REPL_REQUEST_META ?? "{}");
    if (!metadata || typeof metadata !== "object" || Array.isArray(metadata))
      throw new Error("computer: engine request metadata must be an object");
    this.#requestMeta = immutable(metadata as Record<string, unknown>);
    this.#hooks = hooks;
    this.#process = new EngineProcess(config);
    this.#client = new Client(
      { name: manifest.name, version: manifest.version },
      { capabilities: { elicitation: { form: {}, url: {} } } },
    );
    this.#client.setRequestHandler(originalElicitation, (request, extra) => {
      ElicitRequestParamsSchema.parse(request.params);
      return this.#elicit(
        request.params as ElicitRequest["params"],
        extra.signal,
      );
    });
    this.#client.setNotificationHandler(
      ToolListChangedNotificationSchema,
      () => {
        this.#retire(new Error("computer: engine tool surface changed"));
      },
    );
    this.#client.onerror = (cause) => {
      this.#retire(new Error("computer: engine protocol failed", { cause }));
    };
    this.#client.onclose = () => {
      this.#retire(new Error("computer: engine connection ended"));
    };
    hooks.signal.addEventListener("abort", this.#leaseEnded, { once: true });
  }

  static async connect(
    config: EngineConfig,
    hooks: EngineHooks,
  ): Promise<Engine> {
    let engine: Engine | undefined;
    try {
      hooks.signal.throwIfAborted();
      engine = new Engine(config, hooks);
      await engine.#client.connect(engine.#process, {
        signal: engine.#lifetime.signal,
        timeout: CONNECT_MS,
      });
      const listed = await engine.#client.request(
        { method: "tools/list" },
        originalTools,
        { signal: engine.#lifetime.signal, timeout: CONNECT_MS },
      );
      const instructions = engine.#client.getInstructions();
      const serverInfo = engine.#client.getServerVersion();
      const capabilities = engine.#client.getServerCapabilities();
      if (
        listed.nextCursor ||
        !instructions?.trim() ||
        !serverInfo ||
        !capabilities ||
        ["js", "js_reset", "turn_ended"].some(
          (name) =>
            listed.tools.filter((tool) => tool.name === name).length !== 1,
        )
      )
        throw new Error(
          "computer: engine instructions or lifecycle tools are incomplete",
        );
      for (const tool of listed.tools.filter((tool) =>
        ["js", "js_reset", "turn_ended"].includes(tool.name),
      )) {
        if (tool.execution?.taskSupport === "required")
          throw new Error(
            "computer: engine controls require synchronous invocation",
          );
        if (tool.outputSchema)
          engine.#outputs.set(
            tool.name,
            new Method({
              name: tool.name,
              kind: "command",
              target: "call",
              description: tool.description ?? "",
              input: tool.inputSchema,
              output: tool.outputSchema,
            }).validateOutput,
          );
      }
      engine.#surface = immutable(
        cloneJSON({
          instructions,
          tools: listed.tools.filter(
            (tool) => tool.name === "js" || tool.name === "js_reset",
          ),
          serverInfo,
          capabilities,
        }),
      );
      engine.#lifetime.signal.throwIfAborted();
      return engine;
    } catch (cause) {
      if (!engine) throw new EngineStartupFailure("closed", cause);
      const diagnostics = engine.#process.diagnostics;
      const failure = new EngineStartupFailure("closed", cause, diagnostics);
      engine.#retire(failure);
      try {
        await engine.close();
      } catch (cleanup) {
        throw new EngineStartupFailure(
          "unconfirmed",
          new AggregateError(
            [failure, cleanup],
            "computer: engine startup and cleanup failed",
          ),
          diagnostics,
        );
      }
      throw failure;
    }
  }

  readonly #leaseEnded = (): void => {
    this.#retire(
      new Error("computer: control lease ended", {
        cause: this.#hooks.signal.reason,
      }),
    );
  };

  #retire(reason: Error): void {
    if (this.#lifetime.signal.aborted) return;
    this.#lifetime.abort(reason);
    try {
      this.#hooks.onRetired(reason);
    } catch (cause) {
      this.#observerFailure = new Error(
        "computer: engine retirement observer failed",
        { cause },
      );
    }
    if (!this.#active) {
      void this.close().catch(() => {});
    }
  }

  surface(): Readonly<EngineSurface> {
    if (!this.#surface)
      throw new Error("computer: engine has no captured surface");
    return this.#surface;
  }

  async #elicit(
    params: ElicitRequest["params"],
    requestSignal: AbortSignal,
  ): Promise<ElicitResult> {
    const active = this.#active;
    if (!active || active.phase !== "execution") return { action: "cancel" };
    const signal = AbortSignal.any([
      requestSignal,
      active.deadline.controller.signal,
      active.approvals.signal,
    ]);
    if (signal.aborted) return { action: "cancel" };
    const resume = active.deadline.pause();
    const cancelled = Promise.withResolvers<ElicitResult>();
    const abort = () => cancelled.resolve({ action: "cancel" });
    signal.addEventListener("abort", abort, { once: true });
    try {
      return await Promise.race([
        Promise.resolve().then(() =>
          this.#hooks.onElicitation(immutable(cloneJSON(params)), {
            signal,
          }),
        ),
        cancelled.promise,
      ]);
    } catch (cause) {
      this.#retire(new Error("computer: engine elicitation failed", { cause }));
      return { action: "cancel" };
    } finally {
      signal.removeEventListener("abort", abort);
      resume();
    }
  }

  call(
    tool: "js" | "js_reset",
    args: Record<string, unknown>,
    scope: EngineCallScope,
  ): Promise<EngineResult> {
    this.#lifetime.signal.throwIfAborted();
    scope.signal.throwIfAborted();
    if (!this.#surface || this.#active)
      throw new Error(
        "computer: engine already has an active control operation",
      );
    if (
      !["js", "js_reset"].includes(tool) ||
      ![scope.sessionId, scope.turnId, scope.callId].every(ownedIdentity)
    )
      throw new Error(
        "computer: engine call requires its owned control identities",
      );
    const requested =
      tool === "js" && typeof args.timeout_ms === "number"
        ? args.timeout_ms
        : 0;
    if (
      !Number.isFinite(requested) ||
      requested > MAX_TIMER_MS - RESULT_MARGIN_MS
    )
      throw new Error("computer: engine timeout exceeds timer capacity");
    const input = immutable(cloneJSON(args));
    this.#process.beginTurn(scope);
    const deadline = new ExecutionDeadline(
      Math.max(CALL_MS, requested + RESULT_MARGIN_MS),
      AbortSignal.any([scope.signal, this.#lifetime.signal]),
    );
    const completion = Promise.withResolvers<EngineResult>();
    const active: ActiveCall = {
      scope: { ...scope },
      deadline,
      approvals: new AbortController(),
      phase: "execution",
      settled: completion.promise,
    };
    this.#active = active;
    void this.#execute(active, tool, input).then(
      completion.resolve,
      completion.reject,
    );
    return completion.promise;
  }

  async #execute(
    active: ActiveCall,
    tool: "js" | "js_reset",
    args: Record<string, unknown>,
  ): Promise<EngineResult> {
    let result: CallToolResult | undefined;
    let failure: unknown;
    let cleanup: EngineResult["cleanup"] = "ended";
    try {
      const received = await this.#callTool(
        {
          name: tool,
          arguments: args,
          _meta: this.#metadata(active.scope),
        },
        {
          signal: active.deadline.controller.signal,
          timeout: MAX_TIMER_MS,
        },
      );
      if (
        Buffer.byteLength(stringifyJSON(received)) >
        LIMITS.maxResultBytes - 1024
      )
        throw new Error("computer: engine result exceeds receipt capacity");
      result = received;
    } catch (cause) {
      failure = new Error("computer: engine action outcome is unconfirmed", {
        cause,
      });
      this.#retire(failure as Error);
    } finally {
      active.phase = "cleanup";
      active.deadline.close();
      active.approvals.abort(new Error("computer: control operation ended"));
      try {
        await this.#endTurn(active, result !== undefined);
      } catch (cause) {
        cleanup = "unconfirmed";
        const cleanupFailure = new Error(
          "computer: engine turn cleanup is unconfirmed",
          { cause },
        );
        this.#retire(cleanupFailure);
        if (failure)
          failure = new AggregateError(
            [failure, cleanupFailure],
            "computer: engine action and cleanup are unconfirmed",
          );
        else failure = cleanupFailure;
      }
      if (active.scope.signal.aborted && !this.#lifetime.signal.aborted)
        this.#retire(
          new Error("computer: control operation was cancelled", {
            cause: active.scope.signal.reason,
          }),
        );
      if (this.#lifetime.signal.aborted) {
        try {
          await this.#stopProcess();
        } catch (cause) {
          cleanup = "unconfirmed";
          if (!failure)
            failure = new Error(
              "computer: engine process cleanup is unconfirmed",
              { cause },
            );
        }
      }
      this.#active = undefined;
    }
    // A later cleanup failure cannot revoke an already observed action result.
    if (result !== undefined) return { result, cleanup };
    throw failure;
  }

  async #callTool(
    params: CallToolRequest["params"],
    options: RequestOptions,
  ): Promise<CallToolResult> {
    const result = await this.#client.request(
      { method: "tools/call", params },
      originalToolResult,
      options,
    );
    const validate = this.#outputs.get(params.name);
    if (validate) {
      if (!result.structuredContent && !result.isError)
        throw new Error("computer: engine omitted declared structured output");
      if (result.structuredContent) validate(result.structuredContent);
    }
    return result;
  }

  #metadata(scope: EngineCallScope): Record<string, unknown> {
    return {
      ...this.#requestMeta,
      "x-codex-turn-metadata": {
        session_id: scope.sessionId,
        turn_id: scope.turnId,
        call_id: scope.callId,
      },
    };
  }

  async #endTurn(active: ActiveCall, observed: boolean): Promise<void> {
    this.#process.endAdmission(active.scope);
    const failures: unknown[] = [];
    try {
      const ended = await this.#callTool(
        {
          name: "turn_ended",
          arguments: {
            hook_event_name:
              observed &&
              !active.scope.signal.aborted &&
              !this.#lifetime.signal.aborted
                ? "Stop"
                : "Interrupt",
            session_id: active.scope.sessionId,
            turn_id: active.scope.turnId,
          },
          _meta: this.#metadata(active.scope),
        },
        { timeout: CLEANUP_MS },
      );
      if (ended.isError)
        throw new Error("computer: engine refused turn cleanup");
    } catch (cause) {
      failures.push(cause);
    }
    try {
      await this.#process.finishTurn(active.scope);
    } catch (cause) {
      failures.push(cause);
    }
    if (failures.length)
      throw new AggregateError(
        failures,
        "computer: engine turn cleanup is unconfirmed",
      );
  }

  #stopProcess(): Promise<void> {
    this.#shutdown ??= Promise.resolve().then(async () => {
      try {
        await this.#client.close();
      } finally {
        try {
          await this.#process.close();
        } finally {
          this.#hooks.signal.removeEventListener("abort", this.#leaseEnded);
        }
      }
      if (this.#observerFailure) throw this.#observerFailure;
    });
    return this.#shutdown;
  }

  close(): Promise<void> {
    this.#closing ??= Promise.resolve().then(async () => {
      this.#retire(new Error("computer: engine closed"));
      await this.#active?.settled.catch(() => {});
      await this.#stopProcess();
    });
    return this.#closing;
  }
}
